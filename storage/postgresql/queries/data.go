package queries

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/isaqueveras/synk"

	"github.com/oklog/ulid/v2"
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
  SELECT id, args, kind, attempt, max_attempts
  FROM synk.job
  WHERE state in ('available', 'scheduled') AND queue = $1::TEXT 
		AND scheduled_at <= COALESCE($4::TIMESTAMPTZ, NOW())
		AND attempt < max_attempts AND NOT EXISTS (
      SELECT 1 FROM unnest(depends_on) dep_id
      WHERE NOT EXISTS (
        SELECT 1 FROM synk.job
        WHERE id = dep_id AND state = 'completed'
      )
    )
  ORDER BY priority ASC, scheduled_at ASC, id ASC
  LIMIT $2::INTEGER
  FOR UPDATE SKIP LOCKED
) UPDATE synk.job SET
  state = 'running',
  attempt = job.attempt + 1,
  attempted_at = NOW(),
  attempted_by = array_append(job.attempted_by, $3::TEXT)
FROM jobs
WHERE job.id = jobs.id
RETURNING job.id, job.args, job.kind, job.attempt, job.max_attempts;`

// GetJobAvailable retrieves available jobs from the database and updates their state to 'running'.
func (q *Queries) GetJobAvailable(ctx context.Context, tx *sql.Tx, queue string, limit int32, clientID *ulid.ULID) ([]*synk.JobRow, error) {
	rows, err := tx.QueryContext(ctx, getJobAvailableSQL, queue, limit, clientID.String(), nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]*synk.JobRow, 0)
	for rows.Next() {
		var job = &synk.JobRow{Options: &synk.InsertOptions{}, Queue: queue}
		if err = rows.Scan(&job.ID, &job.Args, &job.Kind, &job.Attempt, &job.Options.MaxRetries); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}
		jobs = append(jobs, job)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

const insertSQL = `
INSERT INTO synk.job (queue, kind, args, max_attempts, state, scheduled_at, priority, name, depends_on) 
VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7, $8, $9::bigint[]) RETURNING id;`

// Insert inserts a new job into the database with the specified queue, kind, and arguments.
func (q *Queries) Insert(ctx context.Context, tx *sql.Tx, job *synk.JobRow) (id *int64, err error) {
	err = tx.QueryRowContext(ctx, insertSQL, job.Queue, job.Kind, job.Args, job.Options.MaxRetries,
		job.State, job.Options.ScheduledAt, job.Options.Priority, job.Name, job.Options.DependsOn).Scan(&id)
	return id, err
}

const updateJobStateSQLNoError = `UPDATE synk.job SET state = $1, finalized_at = $2 WHERE id = $3`
const updateJobStateSQLWithError = `UPDATE synk.job SET state = $1, errors = array_append(errors, $2::jsonb) WHERE id = $3`

// UpdateJobState updates the state of a job identified by its ID in the database.
func (q *Queries) UpdateJobState(ctx context.Context, tx *sql.Tx, jobID int64, newState synk.JobState, finalizedAt time.Time, e *synk.AttemptError) error {
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

// Cleaner is a method for cleaning up expired jobs based on their state and age.
func (q *Queries) Cleaner(ctx context.Context, tx *sql.Tx, clear *synk.CleanerConfig) (int64, error) {
	const stmt = "(state = '%s' AND attempted_at < now() - interval '%s')"

	var conditions []string
	for status, retention := range clear.ByStatus {
		conditions = append(conditions, fmt.Sprintf(stmt, status, formatInterval(retention)))
	}

	result, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM synk.job WHERE %s", strings.Join(conditions, " OR ")))
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func formatInterval(d time.Duration) string {
	seconds := int(d.Seconds())
	days := seconds / (24 * 3600)
	if days > 0 {
		return fmt.Sprintf("%d days", days)
	}
	hours := seconds / 3600
	if hours > 0 {
		return fmt.Sprintf("%d hours", hours)
	}
	minutes := seconds / 60
	if minutes > 0 {
		return fmt.Sprintf("%d minutes", minutes)
	}
	return fmt.Sprintf("%d seconds", seconds)
}
