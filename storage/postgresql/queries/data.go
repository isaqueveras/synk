package queries

import (
	"context"
	"database/sql"
	"time"

	"github.com/isaqueveras/synk/types"
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
  ORDER BY priority ASC, scheduled_at ASC, id ASC
  LIMIT $2::integer
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
func (q *Queries) GetJobAvailable(ctx context.Context, tx *sql.Tx, queue string, limit int32) ([]*types.JobRow, error) {
	rows, err := tx.QueryContext(ctx, getJobAvailableSQL, queue, limit, "01JK7753BK0C8PY75K0JVXFYY0")
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

const insertSQL = "INSERT INTO synk.job (queue, kind, args, max_attempts) VALUES ($1, $2, $3::jsonb, 3) RETURNING id"

// Insert inserts a new job into the database with the specified queue, kind, and arguments.
func (q *Queries) Insert(ctx context.Context, tx *sql.Tx, queue, kind string, args []byte) (id *int64, err error) {
	err = tx.QueryRowContext(ctx, insertSQL, queue, kind, args).Scan(&id)
	return id, err
}

const updateJobStateSQL = `
UPDATE synk.job SET 
	state = $1, 
	finalized_at = $2, 
	errors = CASE WHEN $3 IS NOT NULL THEN array_append(errors, $3::jsonb) ELSE errors END 
WHERE id = $4`

// UpdateJobState updates the state of a job identified by its ID in the database.
func (q *Queries) UpdateJobState(ctx context.Context, tx *sql.Tx, jobID int64, newState string, finalizedAt *time.Time, errorMsg *string) error {
	var errMsgJSON any = nil
	if errorMsg != nil {
		errMsgJSON = *errorMsg
	}
	_, err := tx.ExecContext(ctx, updateJobStateSQL, newState, finalizedAt, errMsgJSON, jobID)
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
