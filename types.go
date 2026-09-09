package synk

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// JobRow represents a row in the job table, containing information about a specific job.
// It includes details such as the job ID, the number of attempts, the time of the last attempt,
// the type of job, the queue it belongs to, the encoded arguments, the current state of the job,
// and any errors that occurred during attempts.
type JobRow struct {
	ID        int64          `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Attempt   int            `json:"attempt,omitempty"`
	AttemptAt *time.Time     `json:"attempt_at,omitempty"`
	Kind      string         `json:"kind,omitempty"`
	Queue     string         `json:"queue,omitempty"`
	DependsOn []int64        `json:"depends_on,omitempty"`
	Args      []byte         `json:"args,omitempty"`
	State     JobState       `json:"state,omitempty"`
	Errors    []AttemptError `json:"errors,omitempty"`
	Options   *InsertOptions `json:"options,omitempty"`
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
	// If not specified, the current time is used.
	ScheduledAt time.Time

	// Priority is the priority of the job, which can be used to determine the order
	// in which jobs are processed. Higher values indicate higher priority.
	Priority Priority

	// Pending indicates whether the job is pending execution.
	// If true, the job is considered pending and will not be executed until it is marked
	// as ready. If false, the job is ready to be executed.
	Pending bool

	// Queue is the name of the queue to which the job belongs.
	// If not specified, the default queue is used.
	Queue string

	// MaxRetries is the maximum number of times the job can be retried if it fails.
	// If not specified, the default value is used.
	MaxRetries int

	// DependsOn is a slice of job IDs that the current job depends on.
	// If not specified, the current job does not depend on any other jobs.
	// If any of the dependent jobs fail, the current job will not be executed.
	// This is useful for ensuring that jobs are executed in a specific order.
	// For example, if job A depends on job B, and job B fails, job A will not be executed.
	// If job B succeeds, job A will be executed.
	DependsOn []*int64
}

// JobState represents the status of a job.
type JobState string

const (
	JobStateAvailable JobState = "available"
	JobStateCancelled JobState = "cancelled"
	JobStateCompleted JobState = "completed"
	JobStateRunning   JobState = "running"
	JobStateScheduled JobState = "scheduled"
	JobStatePending   JobState = "pending"
)

// AttemptError represents an error that occurred during a job attempt.
// It contains details about the time of the error, the attempt number,
// the error message, and a stack trace if the job panicked.
type AttemptError struct {
	// NodeID is the ID of the node who used the job.
	NodeID string `json:"node_id"`
	// At is the time at which the error occurred.
	At time.Time `json:"at"`
	// Attempt is the attempt number on which the error occurred (maps to Attempt on a job row).
	Attempt int `json:"attempt"`
	// Error contains the stringified error of an error returned from a job or a panic value in case of a panic.
	Error string `json:"error"`
	// Trace contains a stack trace from a job that panicked. The trace is produced by invoking `debug.Trace()`.
	Trace string `json:"trace"`
}

// StringArray is a custom type that represents an array of strings.
type StringArray []string

// Value implements the driver.Valuer interface for StringArray.
func (a StringArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "{}", nil
	}
	quoted := make([]string, len(a))
	for i, v := range a {
		quoted[i] = fmt.Sprintf(`"%s"`, strings.ReplaceAll(v, `"`, `\"`))
	}
	return "{" + strings.Join(quoted, ",") + "}", nil
}

// Scan implements the sql.Scanner interface for StringArray.
func (a *StringArray) Scan(src interface{}) error {
	if src == nil {
		*a = nil
		return nil
	}

	var source string
	switch t := src.(type) {
	case string:
		source = t
	case []byte:
		source = string(t)
	default:
		return fmt.Errorf("invalid type for StringArray: %T", src)
	}

	str := strings.Trim(source, "{}")
	if str == "" {
		*a = []string{}
		return nil
	}

	elements := strings.Split(str, ",")
	res := make([]string, len(elements))
	for i, elem := range elements {
		res[i] = strings.Trim(elem, `"`)
	}

	*a = res
	return nil
}
