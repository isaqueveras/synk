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
