package postgresql

import (
	"context"
	"database/sql"
	"time"

	"github.com/isaqueveras/synk/storage"
	"github.com/isaqueveras/synk/storage/postgresql/queries"
	"github.com/isaqueveras/synk/types"
)

// New creates a new instance of the storage repository using the provided
// database connection. It returns an implementation of the storage.Storage interface.
func New(db *sql.DB, timeouts ...time.Duration) storage.Storage {
	timeout := time.Second * 5
	if len(timeouts) != 0 {
		timeout = timeouts[0]
	}
	return &postgres{
		ctx:     context.Background(),
		db:      db,
		timeout: timeout,
		queries: queries.New(),
	}
}

type postgres struct {
	db      *sql.DB
	ctx     context.Context
	queries *queries.Queries
	timeout time.Duration
}

// Ping checks the connection to the PostgreSQL database by sending a ping.
func (pg *postgres) Ping() error {
	ctx, cancel := context.WithTimeout(pg.ctx, pg.timeout)
	defer cancel()
	return pg.db.PingContext(ctx)
}

// GetJobAvailable retrieves a list of available jobs from the specified queue with a limit on the number of jobs.
func (pg *postgres) GetJobAvailable(queue string, limit int32) (items []*types.JobRow, err error) {
	ctx, cancel := context.WithTimeout(pg.ctx, pg.timeout)
	defer cancel()

	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() // nolint

	if items, err = pg.queries.GetJobAvailable(ctx, tx, queue, limit); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return items, nil
}

// Insert inserts a new job into the specified queue with the given kind and arguments.
func (pg *postgres) Insert(queue, kind string, args []byte) (*int64, error) {
	ctx, cancel := context.WithTimeout(pg.ctx, pg.timeout)
	defer cancel()

	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var id *int64
	if id, err = pg.queries.Insert(ctx, tx, queue, kind, args); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return id, nil
}

// UpdateJobState updates the state, finalized_at, and error message of a job.
func (pg *postgres) UpdateJobState(jobID int64, newState types.JobState, finalizedAt time.Time, errorMsg *string) error {
	ctx, cancel := context.WithTimeout(pg.ctx, pg.timeout)
	defer cancel()

	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err = pg.queries.UpdateJobState(ctx, tx, jobID, newState, finalizedAt, errorMsg); err != nil {
		return err
	}

	return tx.Commit()
}

// RescheduleJob updates the scheduled_at and attempt fields for a job.
func (pg *postgres) RescheduleJob(jobID int64, scheduledAt time.Time, attempt int) error {
	ctx, cancel := context.WithTimeout(pg.ctx, pg.timeout)
	defer cancel()

	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err = pg.queries.RescheduleJob(ctx, tx, jobID, scheduledAt, attempt); err != nil {
		return err
	}

	return tx.Commit()
}

// ListJobsByState returns all jobs with a given state.
func (pg *postgres) ListJobsByState(state string) (jobs []*types.JobRow, err error) {
	ctx, cancel := context.WithTimeout(pg.ctx, pg.timeout)
	defer cancel()

	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	return pg.queries.ListJobsByState(ctx, tx, state)
}
