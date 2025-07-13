// Package worker provides a sample implementation of a Synk worker for processing biometry jobs.
//
// This file defines the arguments for a biometry job, the worker implementation, and the logic for processing jobs.
// The worker simulates random processing time and randomly fails some jobs to demonstrate error handling in the queue system.

package worker

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/isaqueveras/synk"
)

// BiometryArgs defines the arguments required to process a biometry job.
// It includes the biometry ID and customer ID.
type BiometryArgs struct {
	BiometryID string `json:"biometry_id"`
	CustomerID string `json:"customer_id"`
}

// Kind returns the job kind identifier for biometry jobs.
func (BiometryArgs) Kind() string {
	return "biometry"
}

// NewBiometry returns a new instance of a biometry worker.
// This worker will process jobs of type BiometryArgs.
func NewBiometry() synk.Worker[BiometryArgs] {
	return &biometryWorker{}
}

// biometryWorker implements the Synk Worker interface for biometry jobs.
type biometryWorker struct {
	synk.WorkerDefaults[BiometryArgs]
}

// Work processes a biometry job.
// It simulates a random processing time between 0 and 9 seconds.
// If the random time is less than 3 seconds, it returns an error to simulate a failure.
// Otherwise, it sleeps for the random duration and returns success.
func (biometryWorker) Work(_ context.Context, job *synk.Job[BiometryArgs]) error {
	random := time.Duration(rand.Intn(10))
	if random < 3 {
		return fmt.Errorf("error processing biometry job: %s", job.Args.BiometryID)
	}
	time.Sleep(time.Second * random)
	return nil
}
