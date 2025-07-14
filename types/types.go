package types

import "time"

// JobRow represents a row in the job table, containing information about a specific job.
// It includes details such as the job ID, the number of attempts, the time of the last attempt,
// the type of job, the queue it belongs to, the encoded arguments, the current state of the job,
// and any errors that occurred during attempts.
type JobRow struct {
	Id        int64
	Attempt   int
	AttemptAt *time.Time
	Kind      string
	Queue     string
	Args      []byte
	State     JobState
	Errors    []AttemptError
	Options   *InsertOptions
}

// InsertOptions represents options for inserting a job into the queue.
type InsertOptions struct {
	// ScheduledAt is the time at which the job should be scheduled to run.
	ScheduledAt time.Time

	// Priority is the priority of the job, which can be used to determine the order
	// in which jobs are processed. Higher values indicate higher priority.
	Priority int

	// Pending indicates whether the job is pending execution.
	// If true, the job is considered pending and will not be executed until it is marked
	// as ready. If false, the job is ready to be executed.
	Pending bool

	// Retryable indicates whether the job can be retried if it fails.
	Retryable bool

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
	JobStateRetryable JobState = "retryable"
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
