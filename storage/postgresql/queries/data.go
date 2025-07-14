package queries

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/isaqueveras/synk/types"

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
  SELECT id, args, kind
  FROM synk.job
  WHERE state = 'available' AND queue = $1::TEXT
		AND scheduled_at <= COALESCE($4::TIMESTAMPTZ, NOW())
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
RETURNING job.id, job.args, job.kind`

// GetJobAvailable retrieves available jobs from the database and updates their state to 'running'.
func (q *Queries) GetJobAvailable(ctx context.Context, tx *sql.Tx, queue string, limit int32, clientID *ulid.ULID) ([]*types.JobRow, error) {
	rows, err := tx.QueryContext(ctx, getJobAvailableSQL, queue, limit, clientID.String(), nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]*types.JobRow, 0)
	for rows.Next() {
		var job = new(types.JobRow)
		job.Queue = queue
		if err = rows.Scan(&job.Id, &job.Args, &job.Kind); err != nil {
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
INSERT INTO synk.job (queue, kind, args, max_attempts, state, scheduled_at) 
VALUES ($1, $2, $3::jsonb, $4, $5, $6) RETURNING id`

// Insert inserts a new job into the database with the specified queue, kind, and arguments.
func (q *Queries) Insert(ctx context.Context, tx *sql.Tx, params *types.JobRow) (id *int64, err error) {
	if err = tx.
		QueryRowContext(ctx, insertSQL, params.Queue, params.Kind, params.Args, params.Options.MaxRetries,
			params.State, params.Options.ScheduledAt).
		Scan(&id); err != nil {
		return nil, err
	}
	return id, nil
}

const updateJobStateSQLNoError = `UPDATE synk.job SET state = $1, finalized_at = $2 WHERE id = $3`
const updateJobStateSQLWithError = `UPDATE synk.job SET state = $1, errors = array_append(errors, $2::jsonb) WHERE id = $3`

// UpdateJobState updates the state of a job identified by its ID in the database.
func (q *Queries) UpdateJobState(ctx context.Context, tx *sql.Tx, jobID int64, newState types.JobState, finalizedAt time.Time, e *types.AttemptError) error {
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

const rescheduleJobSQL = "UPDATE synk.job SET scheduled_at = $1, attempt = $2 WHERE id = $3"

// RescheduleJob updates the scheduled_at and attempt fields for a job in the database.
func (q *Queries) RescheduleJob(ctx context.Context, tx *sql.Tx, jobID int64, scheduledAt time.Time, attempt int) error {
	_, err := tx.ExecContext(ctx, rescheduleJobSQL, scheduledAt, attempt, jobID)
	return err
}

const listJobsByStateSQL = `SELECT id, attempt, attempted_at, kind, queue, args, state FROM synk.job WHERE state = $1`

// ListJobsByState retrieves all jobs with a given state from the database.
func (q *Queries) ListJobsByState(ctx context.Context, tx *sql.Tx, state string) (jobs []*types.JobRow, err error) {
	rows, err := tx.QueryContext(ctx, listJobsByStateSQL, state)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var job types.JobRow
		if err = rows.Scan(&job.Id, &job.Attempt, &job.AttemptAt, &job.Kind, &job.Queue, &job.Args, &job.State); err != nil {
			return nil, err
		}
		jobs = append(jobs, &job)
	}

	return jobs, rows.Err()
}
