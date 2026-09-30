package synk

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"
)

type producer struct {
	nodeID        *NodeID
	logger        *slog.Logger
	config        *producerConfig
	storage       Storage
	workers       map[string]*workerInfo
	jobTimeout    time.Duration
	numJobsActive atomic.Int32
}

type producerConfig struct {
	maxWorkerCount uint64
	workers        map[string]*workerInfo
	queueName      string
	jobTimeout     time.Duration
	timeFetch      time.Duration
}

func (p *producer) process(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		return
	}

	limit := int32(p.config.maxWorkerCount) - p.numJobsActive.Load()
	if limit <= 0 {
		return
	}

	jobs, err := p.storage.GetJobAvailable(p.nodeID, p.config.queueName, limit)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			p.logger.ErrorContext(ctx, "failed to get available jobs",
				slog.String("error", err.Error()),
				slog.String("queue", p.config.queueName),
				slog.String("node_id", p.nodeID.String()),
			)
		}
		return
	}

	if len(jobs) == 0 {
		return
	}

	p.logger.Debug("Fetched available jobs",
		slog.Int("count", len(jobs)),
		slog.String("queue", p.config.queueName),
		slog.String("node_id", p.nodeID.String()),
	)

	p.start(ctx, jobs)
}

func (p *producer) heartbeat(ctx context.Context, queues StringArray) {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.ErrorContext(ctx, "Heartbeat context done: "+ctx.Err().Error())
			return
		case <-ticker.C:
			if err := p.storage.Heartbeat(p.nodeID, queues); err != nil {
				p.logger.ErrorContext(ctx, "Failed to update heartbeat",
					slog.String("error", err.Error()),
					slog.String("queue", p.config.queueName),
					slog.String("node_id", p.nodeID.String()))
			}
			p.logger.InfoContext(ctx, "Heartbeat: total completed jobs",
				slog.Int64("active_jobs", int64(p.numJobsActive.Load())),
			)
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
			p.logger.ErrorContext(ctx, "Worker not defined for this type",
				slog.Int64("job_id", int64(job.ID)), slog.String("kind", job.Kind))
			return
		}

		jobCtx, jobCancel := context.WithCancelCause(ctx)

		go p.startWork(jobCtx, jobCancel, job, work)
	}
}

func (p *producer) startWork(ctx context.Context, cancel context.CancelCauseFunc, job *JobRow, work work) {
	p.numJobsActive.Add(1)
	defer p.numJobsActive.Add(-1)

	defer func() {
		cancel(errors.New("context cancelled as executor finished"))
		if r := recover(); r != nil {
			p.logger.ErrorContext(ctx, "worker panic",
				slog.Any("panic", r),
				slog.Int64("job_id", int64(job.ID)),
				slog.String("stack", string(debug.Stack())),
			)
		}
	}()

	if err := work.unmarshal(); err != nil {
		panic(fmt.Sprintf("failed to unmarshal job args: %v", err))
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
			NodeID:  p.nodeID.String(),
			At:      time.Now(),
			Attempt: job.Attempt,
			Error:   msg,
			Trace:   string(debug.Stack()),
		}

		state = JobStateAvailable
		if job.Attempt >= job.Options.MaxRetries {
			state = JobStateCancelled
		}

		p.logger.DebugContext(ctx, "Job failed",
			slog.Int64("job_id", int64(job.ID)),
			slog.String("kind", job.Kind),
			slog.String("args", string(job.Args)),
			slog.String("error", msg),
		)
	}

	if err := p.storage.UpdateJobState(&job.ID, state, time.Now(), attempt); err != nil {
		p.logger.DebugContext(ctx, fmt.Sprintf("Failed to update job %d: %v", job.ID, err))
	}

	p.logger.DebugContext(ctx, "Job completed",
		slog.Int64("job_id", int64(job.ID)),
		slog.String("kind", job.Kind),
		slog.String("args", string(job.Args)),
	)

}
