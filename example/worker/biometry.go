package worker

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/isaqueveras/synk"
)

// BiometryArgs define struct for args biometry
type BiometryArgs struct {
	BiometryID string `json:"biometry_id"`
	CustomerID string `json:"customer_id"`
}

// Kind define biometry kind
func (BiometryArgs) Kind() string {
	return "biometry"
}

// NewBiometry create a new worker for biometry
func NewBiometry() synk.Worker[BiometryArgs] {
	return &biometryWorker{}
}

// biometryWorker ...
type biometryWorker struct {
	synk.WorkerDefaults[BiometryArgs]
}

func (biometryWorker) Work(_ context.Context, job *synk.Job[BiometryArgs]) error {
	sleep := time.Duration(rand.Intn(10))
	if sleep < 3 {
		return fmt.Errorf("error processing biometry job: %s", job.Args.BiometryID)
	}
	time.Sleep(time.Second * sleep)
	return nil
}
