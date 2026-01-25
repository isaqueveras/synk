package postgresql

import (
	"context"
	"database/sql"
	"time"

	"github.com/isaqueveras/synk"
	"github.com/isaqueveras/synk/storage/postgresql/queries"

	"github.com/oklog/ulid/v2"
)

// New creates a new instance of the storage repository using the provided
// database connection. It returns an implementation of the storage.Storage interface.
func New(db *sql.DB, timeouts ...time.Duration) synk.Storage {
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
func (pg *postgres) GetJobAvailable(queue string, limit int32, clientID *ulid.ULID) (items []*synk.JobRow, err error) {
	ctx, cancel := context.WithTimeout(pg.ctx, pg.timeout)
	defer cancel()

	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() // nolint

	if items, err = pg.queries.GetJobAvailable(ctx, tx, queue, limit, clientID); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return items, nil
}

// Insert inserts a new job into the specified queue with the given kind and arguments
// within the context of the provided transaction.
// This allows the operation to be part of an atomic database transaction.
func (pg *postgres) Insert(tx *sql.Tx, params *synk.JobRow) (id *int64, err error) {
	ctx, cancel := context.WithTimeout(pg.ctx, pg.timeout)
	defer cancel()

	var newTx = tx
	if tx == nil {
		if newTx, err = pg.db.BeginTx(ctx, nil); err != nil {
			return nil, err
		}
		defer newTx.Rollback()
	}

	if id, err = pg.queries.Insert(ctx, newTx, params); err != nil {
		return nil, err
	}

	if tx == nil {
		if err = newTx.Commit(); err != nil {
			return nil, err
		}
	}

	return id, nil
}

// UpdateJobState updates the state, finalized_at, and error message of a job.
func (pg *postgres) UpdateJobState(jobID *int64, newState synk.JobState, finalizedAt time.Time, e *synk.AttemptError) error {
	ctx, cancel := context.WithTimeout(pg.ctx, pg.timeout)
	defer cancel()

	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err = pg.queries.UpdateJobState(ctx, tx, jobID, newState, finalizedAt, e); err != nil {
		return err
	}

	return tx.Commit()
}

// Cleaner is a method for cleaning up expired jobs based on their state and age.
func (pg *postgres) Cleaner(clear *synk.CleanerConfig) (int64, error) {
	ctx, cancel := context.WithTimeout(pg.ctx, pg.timeout)
	defer cancel()

	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var rows int64
	if rows, err = pg.queries.Cleaner(ctx, tx, clear); err != nil {
		return 0, err
	}

	return rows, tx.Commit()
}

// Retry retries a job by its ID and returns an error if the operation fails.
func (pg *postgres) Retry(jobID *int64) error {
	ctx, cancel := context.WithTimeout(pg.ctx, pg.timeout)
	defer cancel()

	tx, err := pg.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: false})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err = pg.queries.Retry(ctx, tx, jobID); err != nil {
		return err
	}

	return tx.Commit()
}

func (pg *postgres) Cancel(*int64) error {
	return nil
}

// Delete deletes a job by its ID and returns an error if the operation fails.
func (pg *postgres) Delete(jobID *int64) error {
	ctx, cancel := context.WithTimeout(pg.ctx, pg.timeout)
	defer cancel()

	tx, err := pg.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: false})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err = pg.queries.Delete(ctx, tx, jobID); err != nil {
		return err
	}

	return tx.Commit()
}
