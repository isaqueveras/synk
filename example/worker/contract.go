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
	sleep := time.Duration(rand.Intn(10))
	if sleep < 3 {
		return fmt.Errorf("error processing contract job: %s", job.Args.CustomerID)
	}
	time.Sleep(time.Second * sleep)
	return nil
}
