package biometry

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/isaqueveras/synk"
)

type BiometryArgs struct {
	BiometryID string `json:"biometry_id"`
	CustomerID string `json:"customer_id"`
}

func (BiometryArgs) Kind() string {
	return "biometry"
}

// NewWorker ...
func NewWorker() synk.Worker[BiometryArgs] {
	return &Worker{}
}

type Worker struct {
	synk.WorkerDefaults[BiometryArgs]
}

func (Worker) Work(ctx context.Context, job *synk.Job[BiometryArgs]) error {
	now := time.Now()
	time.Sleep(time.Second * time.Duration(rand.Intn(10)))
	fmt.Printf("Kind: %v - BiometryID: %v - CustomerID: %v | latency: %f\n",
		job.Args.Kind(), job.Args.BiometryID, job.Args.CustomerID, time.Since(now).Seconds())
	return nil
}
