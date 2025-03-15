package contract

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/isaqueveras/synk"
)

type ContractArgs struct {
	CustomerID   string `json:"customer_id"`
	CustomerName string `json:"customer_name"`
}

func (ContractArgs) Kind() string {
	return "contract"
}

func NewWorker() synk.Worker[ContractArgs] {
	return &worker{}
}

type worker struct {
	synk.WorkerDefaults[ContractArgs]
}

func (w worker) Work(ctx context.Context, job *synk.Job[ContractArgs]) error {
	now := time.Now()
	time.Sleep(time.Second * time.Duration(rand.Intn(10)))
	fmt.Printf("Kind: %v - CustomerID: %v - CustomerName: %v | latency: %f\n",
		job.Args.Kind(), job.Args.CustomerID, job.Args.CustomerName, time.Since(now).Seconds())
	return nil
}
