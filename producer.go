package synk

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/isaqueveras/synk/storage"
	"github.com/isaqueveras/synk/types"
	"github.com/oklog/ulid/v2"
)

type producer struct {
	clientID *ulid.ULID

	logger      *slog.Logger
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
	go p.getJobAvailable(jobs, limit, p.clientID)
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
			p.logger.ErrorContext(ctx, "Heartbeat context done: "+ctx.Err().Error())
			return
		case <-ticker.C:
			p.logger.InfoContext(ctx, "Heartbeat: total completed jobs",
				slog.Uint64("total_completed_jobs", p.totalWorkPerformed.Load()),
				slog.Int64("active_jobs", int64(p.numJobsActive.Load())),
			)
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
			p.logger.ErrorContext(ctx, "Worker not defined for this type",
				slog.Int64("job_id", job.Id), slog.String("kind", job.Kind))
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
			p.logger.ErrorContext(ctx, "PANIC:", r, fmt.Sprintf("\n%s", string(debug.Stack())))
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
		errMsg := err.Error()
		if err := p.storage.UpdateJobState(job.Id, types.JobStateRetryable, time.Now(), &types.AttemptError{
			At:      time.Now(),
			Attempt: job.Attempt,
			Error:   errMsg,
			Trace:   string(debug.Stack()),
		}); err != nil {
			p.logger.ErrorContext(ctx, fmt.Sprintf("Failed to update job %d to retryable: %v", job.Id, err))
		}

		p.logger.ErrorContext(ctx, "Job failed",
			slog.Int64("job_id", job.Id),
			slog.String("kind", job.Kind),
			slog.String("args", string(job.Args)),
			slog.String("error", errMsg),
		)
		return
	}

	if err := p.storage.UpdateJobState(job.Id, types.JobStateCompleted, time.Now(), nil); err != nil {
		p.logger.ErrorContext(ctx, fmt.Sprintf("Failed to update job %d to completed: %v", job.Id, err))
	}

	p.logger.DebugContext(ctx, "Job completed",
		slog.Int64("job_id", job.Id),
		slog.String("kind", job.Kind),
		slog.String("args", string(job.Args)),
	)

	select {
	case <-ctx.Done():
		p.logger.ErrorContext(ctx, "Context done: "+ctx.Err().Error())
		return
	default:
	}

	p.handleWorkerDone(job)
}

func (p *producer) handleWorkerDone(job *types.JobRow) {
	p.jobsChannel <- job
}

func (p *producer) getJobAvailable(jobs chan<- []*types.JobRow, limit int32, clientID *ulid.ULID) {
	items, err := p.storage.GetJobAvailable(p.config.queueName, limit, clientID)
	if err != nil {
		panic(err)
	}
	jobs <- items
}
