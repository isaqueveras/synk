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

// BiometryWorker implements the Worker interface for biometry jobs.
type BiometryWorker[T synk.JobArgs] struct{}

// NewBiometry returns a new instance of a biometry worker.
// This worker will process jobs of type BiometryArgs.
func NewBiometry() synk.Worker[BiometryArgs] {
	return &BiometryWorker[BiometryArgs]{}
}

// Work processes a biometry job.
// It simulates a random processing time between 0 and 9 seconds.
// If the random time is less than 3 seconds, it returns an error to simulate a failure.
// Otherwise, it sleeps for the random duration and returns success.
func (BiometryWorker[T]) Work(ctx context.Context, in *synk.Job[BiometryArgs]) error {
	random := time.Duration(rand.Intn(10))
	if random < 3 {
		return fmt.Errorf("error processing biometry job: %d", in.Job.ID)
	}

	time.Sleep(time.Second * random)

	if random == 2 {
		client, err := synk.ClientFromContext(ctx)
		if err != nil {
			return err
		}

		args := BiometryArgs{
			BiometryID: "asdasdas",
			CustomerID: "sdfsdfds",
		}

		_, err = client.Enqueue(ctx, "MinhaTarefa", args, in.Job.Options)
		return err
	}

	return nil
}

// NextRetry returns the time at which the biometry job should be retried.
// In this implementation, it returns a time 5 minutes from the current time.
func (BiometryWorker[T]) NextRetry(*synk.Job[T]) time.Time {
	return time.Now().Add(time.Minute * 5)
}

// Timeout returns the duration after which the biometry job should be considered timed out.
func (BiometryWorker[T]) Timeout(*synk.Job[T]) time.Duration {
	return time.Minute * 2
}
