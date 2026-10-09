package synk

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// JobID represents a unique identifier for a job in the queue system.
// It is defined as an unsigned 64-bit integer.
type JobID uint64

// String returns the string representation of the JobID.
func (id JobID) String() string {
	return fmt.Sprintf("%d", id)
}

// NodeID represents a unique identifier for a node in the queue system.
type NodeID string

// String returns the string representation of the NodeID.
func (n NodeID) String() string {
	return string(n)
}

var (
	// ErrJobNameRequired is returned when a job name is not provided during job enqueueing.
	ErrJobNameRequired = errors.New("job name is required")
	// ErrJobKindRequired is returned when a job kind is not provided during job enqueueing.
	ErrJobKindRequired = errors.New("job kind is required")
)

// JobRow represents a row in the job table, containing information about a specific job.
// It includes details such as the job ID, the number of attempts, the time of the last attempt,
// the type of job, the queue it belongs to, the encoded arguments, the current state of the job,
// and any errors that occurred during attempts.
type JobRow struct {
	ID        JobID           `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Attempt   int             `json:"attempt,omitempty"`
	AttemptAt *time.Time      `json:"attempt_at,omitempty"`
	Kind      string          `json:"kind,omitempty"`
	Queue     string          `json:"queue,omitempty"`
	DependsOn []int64         `json:"depends_on,omitempty"`
	Args      []byte          `json:"args,omitempty"`
	State     JobState        `json:"state,omitempty"`
	Errors    []AttemptError  `json:"errors,omitempty"`
	Options   *EnqueueOptions `json:"options,omitempty"`
}

// Priority represents the priority of a job.
type Priority int

const (
	// PriorityCritical represents the highest priority level for a job.
	PriorityCritical Priority = 1
	// PriorityHigh represents a high priority level for a job.
	PriorityHigh Priority = 2
	// PriorityMedium represents a medium priority level for a job.
	PriorityMedium Priority = 3
	// PriorityLow represents the lowest priority level for a job.
	PriorityLow Priority = 4
)

// EnqueueOptions represents options for enqueueing a job into the queue.
type EnqueueOptions struct {
	// Transaction is an optional database transaction that can be used to insert the job into the queue.
	// If provided, the job will be inserted within the context of this transaction.
	Transaction *sql.Tx

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
	DependsOn []JobID
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

// Storage is an interface that defines methods for interacting with job storage.
type Storage interface {
	// GetJobAvailable retrieves a list of available jobs from the specified queue.
	// It takes the name of the queue and a limit on the number of jobs to retrieve.
	GetJobAvailable(nodeID *NodeID, queue string, limit int64) ([]*JobRow, error)

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

	// UpdateHeartbeat updates the heartbeat timestamp for a node in the database,
	// indicating that it is still active and processing jobs.
	UpdateHeartbeat(nodeID *NodeID, queues []string, queueName string, metrics *Metrics) error

	// Ping checks the connection to the storage system.
	Ping() error
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

// Queues represents a collection of queue configurations, where each queue
// is identified by its name and associated with a QueueConfig.
type Queues map[string]*QueueConfig

// Names returns a StringArray containing the names of all queues in the Queues map.
func (q Queues) Names() StringArray {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	return StringArray(keys)
}

// Job represents a job to be processed by a worker. It is a generic type that
// takes a type parameter T which must satisfy the JobArgs interface.
// The Job struct embeds a JobRow from the types package and includes the arguments
// required to process the job.
type Job[T JobArgs] struct {
	// A pointer to a JobRow struct from the types package,
	// which contains metadata about the job.
	Job *JobRow

	// Args arguments required to process the job, of type T.
	Args T
}

// JobArgs represents an interface that defines a method for retrieving the kind of job.
// Any type that implements this interface must provide a Kind method that returns a string
// indicating the type or category of the job.
type JobArgs interface {
	Kind() string
}

// Worker represents a generic worker interface that processes jobs of type T.
// T must satisfy the JobArgs constraint. The Worker interface defines methods for executing a job,
// determining the timeout for a job, and calculating the next retry time for a job.
type Worker[T JobArgs] interface {
	// Work method processes the given job within the provided context and returns an error if the job fails.
	Work(context.Context, *Job[T]) error
	// Timeout method returns the duration after which the job should be considered timed out.
	Timeout(*Job[T]) time.Duration
	// NextRetry method returns the time at which the job should be retried.
	NextRetry(*Job[T]) time.Time
}

// work represents a unit of work that can be processed.
// It provides methods to unmarshal a job, perform the work,
// retrieve the timeout duration, and determine the next retry time.
type work interface {
	unmarshal() error
	nextRetry() time.Time
	timeout() time.Duration
	work(context.Context) error
}

// workUnit is an interface that defines the creation of workUnit instances.
type workUnit interface {
	makeWork(*JobRow) work
}

// workerInfo holds information about a worker's job.
// It contains the arguments for the job and the work unit to be processed.
type workerInfo struct {
	args JobArgs
	work workUnit
}

// WorkerDefaults is a generic struct that can be used to define default
// settings or configurations for a worker. The generic type parameter T
// represents the type of job arguments that the worker will handle.
type WorkerDefaults[T JobArgs] struct{}

// NextRetry calculates the next retry time for a given job.
// It returns a zero value of time.Time, indicating no retry is scheduled.
func (w WorkerDefaults[T]) NextRetry(*Job[T]) time.Time { return time.Time{} }

// Timeout returns the duration for which the worker should wait before timing out a job.
// This method can be overridden to provide custom timeout logic for different jobs.
func (w WorkerDefaults[T]) Timeout(*Job[T]) time.Duration { return 0 }

// workWrapper is a generic struct that wraps a Worker instance.
// It is parameterized by the type T, which must implement the JobArgs interface.
type workWrapper[T JobArgs] struct{ worker Worker[T] }

// newWorkWrapper creates a new instance of workWrapper for the given Worker.
func newWorkWrapper[T JobArgs](w Worker[T]) workUnit {
	return &workWrapper[T]{worker: w}
}

// makeWork initializes and returns a new work unit for the given job row.
// It wraps the job row and associates it with the worker.
func (w *workWrapper[T]) makeWork(row *JobRow) work {
	return &wrapperWorkUnit[T]{row: row, worker: w.worker}
}

// wrapperWorkUnit is a generic struct that encapsulates a work unit for a job.
// It contains a job of type JobArgs, a row of type JobRow, and a worker of type Worker.
// T represents the type parameter that must satisfy the JobArgs constraint.
type wrapperWorkUnit[T JobArgs] struct {
	job    *Job[T]
	row    *JobRow
	worker Worker[T]
}

// work executes the work unit within the provided context.
// It performs the necessary operations defined for the work unit
// and returns an error if any issues occur during execution.
func (w *wrapperWorkUnit[T]) work(ctx context.Context) error {
	return w.worker.Work(ctx, w.job)
}

// nextRetry calculates the next retry time for the current job.
// It uses the worker's NextRetry method to determine the appropriate time
// based on the job's properties and retry logic.
func (w *wrapperWorkUnit[T]) nextRetry() time.Time {
	return w.worker.NextRetry(w.job)
}

// timeout returns the duration for which the work unit should wait before timing out.
// The duration is determined based on the specific implementation of the wrapperWorkUnit.
func (w *wrapperWorkUnit[T]) timeout() time.Duration {
	return w.worker.Timeout(w.job)
}

// unmarshal deserializes the data contained in the wrapperWorkUnit into the appropriate
// type T. It returns an error if the unmarshalling process fails.
func (w *wrapperWorkUnit[T]) unmarshal() error {
	w.job = &Job[T]{Job: w.row}
	if w.row != nil && w.row.Args == nil {
		return fmt.Errorf("args is nil for job %d", w.row.ID)
	}
	return json.Unmarshal(w.row.Args, &w.job.Args)
}
