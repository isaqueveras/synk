package storage

import (
	"database/sql"
	"time"

	"github.com/isaqueveras/synk/types"

	"github.com/oklog/ulid/v2"
)

// Storage is an interface that defines methods for interacting with job storage.
// It provides a method to retrieve available jobs from a specified queue.
type Storage interface {
	// Ping checks the connection to the storage system.
	// It returns an error if the connection is not successful.
	Ping() error

	// GetJobAvailable retrieves a list of available jobs from the specified queue.
	// It takes the name of the queue and a limit on the number of jobs to retrieve.
	// It returns a slice of pointers to JobRow and an error if the operation fails.
	GetJobAvailable(queue string, limit int32) ([]*types.JobRow, error)

	// Insert adds a new job to the specified queue with the given kind and arguments.
	// It takes the name of the queue, the kind of job, and the arguments as a byte slice.
	// It returns a pointer to the job ID and an error if the insertion fails.
	Insert(params *types.JobRow) (*int64, error)

	// InsertTx adds a new job to the specified queue with the given kind and arguments
	// within the context of the provided transaction.
	InsertTx(tx *sql.Tx, params *types.JobRow) (*int64, error)

	// UpdateJobState updates the state of a job identified by its ID.
	// It takes the job ID, the new state, an optional finalized time, and an
	// optional error message. It returns an error if the update fails.
	UpdateJobState(jobID int64, newState types.JobState, finalizedAt time.Time, e *types.AttemptError) error

	// RescheduleJob reschedules a job identified by its ID to be executed at a specified time.
	// It takes the job ID, the time to reschedule to, and the attempt number
	// It returns an error if the rescheduling fails.
	RescheduleJob(jobID int64, scheduledAt time.Time, attempt int) error

	// ListJobsByState retrieves a list of jobs filtered by their state.
	// It takes the state as a string and returns a slice of pointers to JobRow
	// and an error if the operation fails.
	ListJobsByState(state string) ([]*types.JobRow, error)
}
