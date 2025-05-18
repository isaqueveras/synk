package worker

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/isaqueveras/synk"
)

// ContractArgs ...
type ContractArgs struct {
	CustomerID   string `json:"customer_id"`
	CustomerName string `json:"customer_name"`
}

// Kind ...
func (ContractArgs) Kind() string {
	return "contract"
}

// NewContract ...
func NewContract() synk.Worker[ContractArgs] {
	return &contractWorker{}
}

type contractWorker struct {
	synk.WorkerDefaults[ContractArgs]
}

func (w contractWorker) Work(_ context.Context, job *synk.Job[ContractArgs]) error {
	now := time.Now()
	time.Sleep(time.Second * time.Duration(rand.Intn(10)))
	fmt.Printf("Kind: %v - CustomerID: %v - CustomerName: %v | latency: %f\n",
		job.Args.Kind(), job.Args.CustomerID, job.Args.CustomerName, time.Since(now).Seconds())
	return nil
}
