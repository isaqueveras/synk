package queries

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/isaqueveras/synk"
)

// Queries represents a collection of methods to interact with the PostgreSQL database.
// This struct is intended to encapsulate all the database queries related to the application.
type Queries struct{}

// New creates a new instance of Queries.
func New() *Queries {
	return &Queries{}
}

const getJobAvailableSQL = `
WITH jobs AS (
	SELECT j.id, j.args, j.kind, j.attempt, j.max_attempts
  FROM job AS j
  INNER JOIN queue AS q ON j.queue = q.name
  WHERE j.state IN ('available', 'scheduled') 
		AND j.queue = $1::TEXT 
		AND j.scheduled_at <= COALESCE($4::TIMESTAMPTZ, NOW())
		AND j.attempt < j.max_attempts
		AND q.is_paused = false
  ORDER BY j.priority ASC, j.scheduled_at ASC, j.id ASC
  LIMIT $2::INTEGER
  FOR UPDATE OF j SKIP LOCKED
) 
UPDATE job SET
  state = 'running',
	locked_by = $3::TEXT,
  attempt = job.attempt + 1,
  attempted_at = NOW(),
  attempted_by = array_append(job.attempted_by, $3::TEXT)
FROM jobs
WHERE job.id = jobs.id
RETURNING job.id, job.args, job.kind, job.attempt, job.max_attempts;`

// GetJobAvailable retrieves available jobs from the database and updates their state to 'running'.
func (q *Queries) GetJobAvailable(ctx context.Context, tx *sql.Tx, queue string, limit int32, nodeID *synk.NodeID) ([]*synk.JobRow, error) {
	rows, err := tx.QueryContext(ctx, getJobAvailableSQL, queue, limit, nodeID.String(), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	jobs := make([]*synk.JobRow, 0)
	for rows.Next() {
		var job = &synk.JobRow{Options: &synk.EnqueueOptions{}, Queue: queue}
		if err = rows.Scan(&job.ID, &job.Args, &job.Kind, &job.Attempt, &job.Options.MaxRetries); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

const enqueueSQL = `
INSERT INTO job (queue, kind, args, max_attempts, state, scheduled_at, priority, name, depends_on, remaining_dependencies)
VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7, $8, $9::bigint[], COALESCE(array_length($9::bigint[], 1), 0))
RETURNING id;`

// Enqueue inserts a new job into the database with the specified queue, kind, and arguments.
func (q *Queries) Enqueue(ctx context.Context, tx *sql.Tx, job *synk.JobRow) (id *synk.JobID, err error) {
	err = tx.QueryRowContext(ctx, enqueueSQL, job.Queue, job.Kind, job.Args, job.Options.MaxRetries,
		job.State, job.Options.ScheduledAt, job.Options.Priority, job.Name, job.Options.DependsOn,
	).Scan(&id)
	return id, err
}

const updateJobStateSQLNoError = `UPDATE job SET state = $1, finalized_at = $2 WHERE id = $3`
const updateJobStateSQLWithError = `UPDATE job SET state = $1, errors = array_append(errors, $2::jsonb) WHERE id = $3`

// UpdateJobState updates the state of a job identified by its ID in the database
func (q *Queries) UpdateJobState(ctx context.Context, tx *sql.Tx, jobID *synk.JobID, newState synk.JobState, finalizedAt time.Time, e *synk.AttemptError) error {
	if e != nil {
		errorJSON, err := json.Marshal(e)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, updateJobStateSQLWithError, newState, errorJSON, jobID)
		return err
	}
	_, err := tx.ExecContext(ctx, updateJobStateSQLNoError, newState, finalizedAt, jobID)
	return err
}

const resolveDependenciesSQL = `
UPDATE job
SET
	remaining_dependencies = remaining_dependencies - 1,
	state = CASE 
		WHEN remaining_dependencies - 1 <= 0 AND scheduled_at > NOW() THEN 'scheduled'::job_state 
		WHEN remaining_dependencies - 1 <= 0 THEN 'available'::job_state 
		ELSE state 
	END
WHERE $1 = ANY(depends_on) AND state = 'pending';`

// ResolveDependencies resolves dependencies for a job identified by its ID in the database
func (q *Queries) ResolveDependencies(ctx context.Context, tx *sql.Tx, jobID *synk.JobID) error {
	_, err := tx.ExecContext(ctx, resolveDependenciesSQL, jobID)
	return err
}

const cleanerBatchSQL = `
WITH cleaner_batch AS (
	SELECT id
	FROM job
	WHERE state = $1 AND finalized_at < $2
	LIMIT $3
	FOR UPDATE SKIP LOCKED
)
DELETE FROM job j 
USING cleaner_batch c 
WHERE j.id = c.id;`

// Cleaner is a method for cleaning up expired jobs based on their state and age.
// Deletion is performed in batches to avoid table locks and I/O spikes on the database
func (q *Queries) Cleaner(ctx context.Context, tx *sql.Tx, clear *synk.CleanerConfig) (int64, error) {
	var totalDeleted int64
	for status, retentionDuration := range clear.ByStatus {
		cutoffTime := time.Now().Add(-retentionDuration)
		for {
			result, err := tx.ExecContext(ctx, cleanerBatchSQL, status, cutoffTime, clear.BatchSize)
			if err != nil {
				return totalDeleted, fmt.Errorf("failed to clean status %q: %w", status, err)
			}

			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return totalDeleted, err
			}

			totalDeleted += rowsAffected
			if rowsAffected < int64(clear.BatchSize) {
				break
			}
		}
	}
	return totalDeleted, nil
}

const retrySQL = `
WITH job_locked AS (
	SELECT id FROM job
	WHERE id = $1
	FOR UPDATE SKIP LOCKED
) UPDATE job J SET 
	state = 'available', 
	attempt = 0, 
	attempted_at = NULL,
	finalized_at = NULL, 
	scheduled_at = now()
FROM job_locked JL
WHERE J.id = JL.id AND J.state != 'running'
AND NOT (J.state = 'available' AND J.scheduled_at < now());`

// Retry retries a job by its ID and returns an error if the operation fails.
func (q *Queries) Retry(ctx context.Context, tx *sql.Tx, jobID *synk.JobID) error {
	_, err := tx.ExecContext(ctx, retrySQL, jobID)
	return err
}

const deleteSQL = `DELETE FROM job WHERE id = $1;`

// Delete deletes a job by its ID and returns an error if the operation fails.
func (q *Queries) Delete(ctx context.Context, tx *sql.Tx, jobID *synk.JobID) error {
	_, err := tx.ExecContext(ctx, deleteSQL, jobID)
	return err
}

const cancelSQL = `
WITH job_locked AS (
	SELECT id FROM job 
	WHERE id = $1
	FOR UPDATE SKIP LOCKED
)
UPDATE job J 
SET state = 'cancelled', finalized_at = now()
FROM job_locked JL
WHERE J.id = JL.id AND J.state not in ('running', 'cancelled', 'completed');`

// Cancel cancels a job by its ID and returns an error if the operation fails.
func (q *Queries) Cancel(ctx context.Context, tx *sql.Tx, jobID *synk.JobID) error {
	_, err := tx.ExecContext(ctx, cancelSQL, jobID)
	return err
}

const heartbeatSQL = `
INSERT INTO node (id, hostname, pid, queues, started_at, last_heartbeat_at)
VALUES ($1, $2, $3, $4, NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET last_heartbeat_at = NOW(), 
	queues = EXCLUDED.queues, pid = EXCLUDED.pid, hostname = EXCLUDED.hostname;`

// Heartbeat updates the heartbeat timestamp for a node in the database,
// indicating that it is still active and processing jobs.
func (q *Queries) Heartbeat(ctx context.Context, tx *sql.Tx, nodeID *synk.NodeID, queues synk.StringArray) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, heartbeatSQL, nodeID.String(), hostname, os.Getpid(), queues)
	return err
}
