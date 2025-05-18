package worker

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/isaqueveras/synk"
)

// BiometryArgs ...
type BiometryArgs struct {
	BiometryID string `json:"biometry_id"`
	CustomerID string `json:"customer_id"`
}

// Kind ...
func (BiometryArgs) Kind() string {
	return "biometry"
}

// NewBiometry ...
func NewBiometry() synk.Worker[BiometryArgs] {
	return &biometryWorker{}
}

// biometryWorker ...
type biometryWorker struct {
	synk.WorkerDefaults[BiometryArgs]
}

func (biometryWorker) Work(_ context.Context, job *synk.Job[BiometryArgs]) error {
	now := time.Now()
	time.Sleep(time.Second * time.Duration(rand.Intn(10)))
	fmt.Printf("Kind: %v - BiometryID: %v - CustomerID: %v | latency: %f\n",
		job.Args.Kind(), job.Args.BiometryID, job.Args.CustomerID, time.Since(now).Seconds())
	return nil
}
