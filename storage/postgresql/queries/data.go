package queries

import (
	"context"
	"database/sql"

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
