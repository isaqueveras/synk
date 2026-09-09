package synk

import (
	"database/sql"
	"time"
)

// Storage is an interface that defines methods for interacting with job storage.
type Storage interface {
	// GetJobAvailable retrieves a list of available jobs from the specified queue.
	// It takes the name of the queue and a limit on the number of jobs to retrieve.
	// It returns a slice of pointers to JobRow and an error if the operation fails.
	GetJobAvailable(queue string, limit int32, nodeID *string) ([]*JobRow, error)

	// Insert adds a new job to the specified queue with the given kind and arguments
	// within the context of the provided transaction. This allows the operation to be
	// part of an atomic database transaction. It returns the ID of the inserted job
	// and an error if the operation fails.
	Insert(tx *sql.Tx, params *JobRow) (*int64, error)

	// Cancel cancels a job by its ID and returns an error if the operation fails.
	Cancel(jobID *int64) error

	// Retry retries a job by its ID and returns an error if the operation fails.
	Retry(jobID *int64) error

	// Delete deletes a job by its ID and returns an error if the operation fails.
	Delete(jobID *int64) error

	// UpdateJobState updates the state of a job identified by its ID.
	// It takes the job ID, the new state, an optional finalized time, and an
	// optional error message. It returns an error if the update fails.
	UpdateJobState(jobID *int64, newState JobState, finalizedAt time.Time, e *AttemptError) error

	// Cleaner is a method for cleaning up expired jobs based on their state and age.
	// It takes a CleanerConfig struct as input and returns an error if the cleanup fails.
	Cleaner(*CleanerConfig) (int64, error)

	// Heartbeat updates the heartbeat timestamp for a node in the database,
	// indicating that it is still active and processing jobs. It takes the node ID
	// and a list of queues as input, and returns an error if the operation fails.
	Heartbeat(nodeID string, queues []string) error

	// Ping checks the connection to the storage system.
	// It returns an error if the connection is not successful.
	Ping() error
}
