package synk

import (
	"context"
	"database/sql"
	"time"
)

// Storage is an interface that defines methods for interacting with job storage.
type Storage interface {
	// GetJobAvailable retrieves a list of available jobs from the specified queue.
	// It takes the name of the queue and a limit on the number of jobs to retrieve.
	GetJobAvailable(nodeID *NodeID, queue string, limit int32) ([]*JobRow, error)
	// Enqueue adds a new job to the specified queue with the given kind and arguments
	// within the context of the provided transaction. This allows the operation to be
	// part of an atomic database transaction.
	Enqueue(ctx context.Context, tx *sql.Tx, params *JobRow) (*JobID, error)
	// Cancel cancels a job by its ID
	Cancel(jobID *JobID) error
	// Retry retries a job by its ID
	Retry(jobID *JobID) error
	// Delete deletes a job by its ID
	Delete(jobID *JobID) error
	// UpdateJobState updates the state of a job identified by its ID.
	UpdateJobState(jobID *JobID, newState JobState, finalizedAt time.Time, e *AttemptError) error
	// Cleaner is a method for cleaning up expired jobs based on their state and age.
	// It takes a CleanerConfig struct as input
	Cleaner(*CleanerConfig) (int64, error)
	// Heartbeat updates the heartbeat timestamp for a node in the database,
	// indicating that it is still active and processing jobs.
	Heartbeat(nodeID *NodeID, queues []string) error
	// Ping checks the connection to the storage system.
	Ping() error
}
