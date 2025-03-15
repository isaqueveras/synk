// Copyright (c) 2025 Isaque Veras
// Licensed under the MIT License.
// See LICENSE file in the project root for full license information.

package synk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/isaqueveras/synk/types"
)

// Job represents a job to be processed by a worker. It is a generic type that
// takes a type parameter T which must satisfy the JobArgs interface.
// The Job struct embeds a JobRow from the types package and includes the arguments
// required to process the job.
type Job[T JobArgs] struct {
	// A pointer to a JobRow struct from the types package,
	// which contains metadata about the job.
	*types.JobRow

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
	makeWork(*types.JobRow) work
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
func (w *workWrapper[T]) makeWork(row *types.JobRow) work {
	return &wrapperWorkUnit[T]{row: row, worker: w.worker}
}

// wrapperWorkUnit is a generic struct that encapsulates a work unit for a job.
// It contains a job of type JobArgs, a row of type JobRow, and a worker of type Worker.
// T represents the type parameter that must satisfy the JobArgs constraint.
type wrapperWorkUnit[T JobArgs] struct {
	job    *Job[T]
	row    *types.JobRow
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
	w.job = &Job[T]{JobRow: w.row}
	if w.row != nil && w.row.Args == nil {
		return fmt.Errorf("Args is nil for job %d", w.row.Id)
	}
	return json.Unmarshal(w.row.Args, &w.job.Args)
}
