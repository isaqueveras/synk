// Package worker provides a sample implementation of a Synk worker for processing contract jobs.
//
// This file defines the arguments for a contract job, the worker implementation, and the logic for processing jobs.
// The worker simulates random processing time and randomly fails some jobs to demonstrate error handling in the queue system.

package worker

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/isaqueveras/synk"
)

// ContractArgs defines the arguments required to process a contract job.
// It includes the customer ID and customer name.
type ContractArgs struct {
	CustomerID   string `json:"customer_id"`
	CustomerName string `json:"customer_name"`
}

// Kind returns the job kind identifier for contract jobs.
func (ContractArgs) Kind() string {
	return "contract"
}

// NewContract returns a new instance of a contract worker.
// This worker will process jobs of type ContractArgs.
func NewContract() synk.Worker[ContractArgs] {
	return &contractWorker{}
}

// contractWorker implements the Synk Worker interface for contract jobs.
type contractWorker struct {
	synk.WorkerDefaults[ContractArgs]
}

// Work processes a contract job.
// It simulates a random processing time between 0 and 9 seconds.
// If the random time is less than 3 seconds, it returns an error to simulate a failure.
// Otherwise, it sleeps for the random duration and returns success.
func (w contractWorker) Work(_ context.Context, job *synk.Job[ContractArgs]) error {
	random := time.Duration(rand.Intn(10))
	if random < 3 {
		return fmt.Errorf("error processing contract job: %s", job.Args.CustomerID)
	}
	time.Sleep(time.Second * random)
	return nil
}
