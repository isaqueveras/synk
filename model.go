package synk

import (
	"database/sql"
	"time"

	"github.com/oklog/ulid/v2"
)

// JobRow represents a row in the job table, containing information about a specific job.
// It includes details such as the job ID, the number of attempts, the time of the last attempt,
// the type of job, the queue it belongs to, the encoded arguments, the current state of the job,
// and any errors that occurred during attempts.
type JobRow struct {
	ID        int64
	Attempt   int
	AttemptAt *time.Time
	Kind      string
	Queue     string
	Args      []byte
	State     JobState
	Errors    []AttemptError
	Options   *InsertOptions
}

// Priority represents the priority of a job.
type Priority int

const (
	PriorityCritical Priority = 1
	PriorityHigh     Priority = 2
	PriorityMedium   Priority = 3
	PriorityLow      Priority = 4
)

// InsertOptions represents options for inserting a job into the queue.
type InsertOptions struct {
	// ScheduledAt is the time at which the job should be scheduled to run.
	ScheduledAt time.Time

	// Priority is the priority of the job, which can be used to determine the order
	// in which jobs are processed. Higher values indicate higher priority.
	Priority Priority

	// Pending indicates whether the job is pending execution.
	// If true, the job is considered pending and will not be executed until it is marked
	// as ready. If false, the job is ready to be executed.
	Pending bool

	// MaxRetries is the maximum number of times the job can be retried if it fails.
	MaxRetries int
}

// JobState represents the status of a job.
type JobState string

const (
	JobStateAvailable JobState = "available"
	JobStateCancelled JobState = "cancelled"
	JobStateCompleted JobState = "completed"
	JobStateDiscarded JobState = "discarded"
	JobStateRunning   JobState = "running"
	JobStateScheduled JobState = "scheduled"
	JobStatePending   JobState = "pending"
)

// AttemptError represents an error that occurred during a job attempt.
// It contains details about the time of the error, the attempt number,
// the error message, and a stack trace if the job panicked.
type AttemptError struct {
	// At is the time at which the error occurred.
	At time.Time `json:"at"`

	// Attempt is the attempt number on which the error occurred (maps to
	// Attempt on a job row).
	Attempt int `json:"attempt"`

	// Error contains the stringified error of an error returned from a job or a
	// panic value in case of a panic.
	Error string `json:"error"`

	// Trace contains a stack trace from a job that panicked. The trace is
	// produced by invoking `debug.Trace()`.
	Trace string `json:"trace"`
}

// Storage is an interface that defines methods for interacting with job storage.
// It provides a method to retrieve available jobs from a specified queue.
type Storage interface {
	// Ping checks the connection to the storage system.
	// It returns an error if the connection is not successful.
	Ping() error

	// GetJobAvailable retrieves a list of available jobs from the specified queue.
	// It takes the name of the queue and a limit on the number of jobs to retrieve.
	// It returns a slice of pointers to JobRow and an error if the operation fails.
	GetJobAvailable(queue string, limit int32, clientID *ulid.ULID) ([]*JobRow, error)

	// GetJobsScheduled retrieves a list of jobs that are scheduled to run.
	// GetJobsScheduled() ([]*JobRow, error)

	// Insert adds a new job to the specified queue with the given kind and arguments.
	// It takes the name of the queue, the kind of job, and the arguments as a byte slice.
	// It returns a pointer to the job ID and an error if the insertion fails.
	Insert(params *JobRow) (*int64, error)

	// InsertTx adds a new job to the specified queue with the given kind and arguments
	// within the context of the provided transaction.
	InsertTx(tx *sql.Tx, params *JobRow) (*int64, error)

	// UpdateJobState updates the state of a job identified by its ID.
	// It takes the job ID, the new state, an optional finalized time, and an
	// optional error message. It returns an error if the update fails.
	UpdateJobState(jobID int64, newState JobState, finalizedAt time.Time, e *AttemptError) error
}
