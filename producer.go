package synk

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/isaqueveras/synk/storage"
	"github.com/isaqueveras/synk/types"
)

type producer struct {
	jobsChannel chan *types.JobRow
	config      *producerConfig
	storage     storage.Storage
	workers     map[string]*workerInfo

	jobTimeout         time.Duration
	numJobsActive      atomic.Int32
	totalWorkPerformed atomic.Uint64

	done func(*types.JobRow)
}

type producerConfig struct {
	maxWorkerCount uint16
	workers        map[string]*workerInfo
	queueName      string
	workID         string
	jobTimeout     time.Duration
	timeFetch      time.Duration
}

func (p *producer) process(ctx context.Context, jobs chan []*types.JobRow) {
	limit := int32(p.config.maxWorkerCount) - p.numJobsActive.Load()
	go p.getJobAvailable(jobs, limit)
	for {
		select {
		case jobs := <-jobs:
			if len(jobs) != 0 {
				p.start(ctx, jobs)
			}
			return
		case <-p.jobsChannel:
			p.numJobsActive.Add(-1)
			p.totalWorkPerformed.Add(1)
		}
	}
}

func (p *producer) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("ctx.Err(): %v\n", ctx.Err())
			return
		case <-ticker.C:
			log.Println("Heartbeat: total completed jobs", p.config.queueName, p.totalWorkPerformed.Load())
		}
	}
}

func (p *producer) start(ctx context.Context, jobs []*types.JobRow) {
	for _, job := range jobs {
		var work work
		if info, ok := p.workers[job.Kind]; ok {
			work = info.work.makeWork(job)
		}

		if work == nil {
			log.Printf("[ERROR] Worker not defined for this type: JobID: %d, Kind: %s", job.Id, job.Kind)
			return
		}

		p.done = p.handleWorkerDone
		p.numJobsActive.Add(1)

		go p.startWork(ctx, job, work)
	}
}

func (p *producer) startWork(ctx context.Context, job *types.JobRow, work work) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("PANIC:", r, fmt.Sprintf("\n%s", string(debug.Stack())))
		}
	}()

	if err := work.unmarshal(); err != nil {
		panic(err)
	}

	jobTimeout := work.timeout()
	if jobTimeout == 0 {
		jobTimeout = p.jobTimeout
	}

	if jobTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, jobTimeout)
		defer cancel()
	}

	if err := work.work(ctx); err != nil {
		panic(err)
	}

	select {
	case <-ctx.Done():
		fmt.Printf("ctx.Err(): %v\n", ctx.Err())
		return
	default:
	}

	p.handleWorkerDone(job)
}

func (p *producer) handleWorkerDone(job *types.JobRow) {
	p.jobsChannel <- job
}

func (p *producer) getJobAvailable(jobs chan<- []*types.JobRow, limit int32) {
	items, err := p.storage.GetJobAvailable(p.config.queueName, limit)
	if err != nil {
		panic(err)
	}
	jobs <- items
}
