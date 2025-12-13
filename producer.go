package synk

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"
)

type producer struct {
	clientID *ulid.ULID

	logger      *slog.Logger
	jobsChannel chan *JobRow
	config      *producerConfig
	storage     Storage
	workers     map[string]*workerInfo

	jobTimeout    time.Duration
	numJobsActive atomic.Int32

	done func(*JobRow)
}

type producerConfig struct {
	maxWorkerCount uint16
	workers        map[string]*workerInfo
	queueName      string
	workID         string
	jobTimeout     time.Duration
	timeFetch      time.Duration
}

func (p *producer) process(ctx context.Context, jobs chan []*JobRow) {
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
			p.logger.InfoContext(ctx, "Heartbeat: total completed jobs", slog.Int64("active_jobs", int64(p.numJobsActive.Load())))
		}
	}
}

func (p *producer) start(ctx context.Context, jobs []*JobRow) {
	for _, job := range jobs {
		var work work
		if info, ok := p.workers[job.Kind]; ok {
			work = info.work.makeWork(job)
		}

		if work == nil {
			p.logger.ErrorContext(ctx, "Worker not defined for this type", slog.Int64("job_id", job.ID), slog.String("kind", job.Kind))
			return
		}

		p.done = p.handleWorkerDone
		p.numJobsActive.Add(1)

		go p.startWork(ctx, job, work)
	}
}

func (p *producer) startWork(ctx context.Context, job *JobRow, work work) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.ErrorContext(ctx, string(debug.Stack()))
		}
	}()

	if err := work.unmarshal(); err != nil {
		p.logger.ErrorContext(ctx, "Failed to unmarshal job args", slog.Int64("job_id", job.ID), slog.String("error", err.Error()))
		return
	}

	var (
		timeout = work.timeout()
		state   = JobStateCompleted
		attempt *AttemptError
	)

	if timeout == 0 {
		timeout = p.jobTimeout
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if err := work.work(ctx); err != nil {
		msg := err.Error()
		attempt = &AttemptError{
			At:      time.Now(),
			Attempt: job.Attempt,
			Error:   msg,
			Trace:   string(debug.Stack()),
		}

		state = JobStateAvailable
		if job.Attempt >= job.Options.MaxRetries {
			state = JobStateCancelled
		}

		p.logger.DebugContext(ctx, "Job failed", slog.Int64("job_id", job.ID), slog.String("kind", job.Kind),
			slog.String("args", string(job.Args)), slog.String("error", msg))
	}

	if err := p.storage.UpdateJobState(job.ID, state, time.Now(), attempt); err != nil {
		p.logger.DebugContext(ctx, fmt.Sprintf("Failed to update job %d: %v", job.ID, err))
	}

	p.logger.DebugContext(ctx, "Job completed",
		slog.Int64("job_id", job.ID),
		slog.String("kind", job.Kind),
		slog.String("args", string(job.Args)),
	)

	select {
	case <-ctx.Done():
		p.logger.DebugContext(ctx, "Context done: "+ctx.Err().Error())
		return
	default:
	}

	p.handleWorkerDone(job)
}

func (p *producer) handleWorkerDone(job *JobRow) {
	p.jobsChannel <- job
}

func (p *producer) getJobAvailable(jobs chan<- []*JobRow, limit int32, clientID *ulid.ULID) {
	items, err := p.storage.GetJobAvailable(p.config.queueName, limit, clientID)
	if err != nil {
		panic(err)
	}

	fmt.Printf("len(items): %v\n", len(items))

	jobs <- items
}
