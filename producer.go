package synk

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"
)

type producer struct {
	nodeID     *NodeID
	logger     *slog.Logger
	config     *producerConfig
	storage    Storage
	workers    map[string]*workerInfo
	jobTimeout time.Duration

	metrics metricsState
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

	limit := int64(p.config.maxWorkerCount) - p.metrics.activeJobs.Load()
	if limit <= 0 {
		return
	}

	jobs, err := p.storage.GetJobAvailable(p.nodeID, p.config.queueName, limit)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			p.logger.ErrorContext(ctx, "failed to get available jobs", slog.String("error", err.Error()),
				slog.String("queue", p.config.queueName), slog.String("node_id", p.nodeID.String()))
		}
		return
	}

	p.metrics.jobsFetched.Add(int64(len(jobs)))
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

		go p.executor(jobCtx, jobCancel, job, work)
	}
}

func (p *producer) heartbeat(ctx context.Context, queues StringArray) {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.storage.UpdateHeartbeat(p.nodeID, queues, p.config.queueName, p.metrics.snapshot()); err != nil {
				p.metrics.heartbeatErrors.Add(1)
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					p.logger.ErrorContext(ctx, "Failed to update heartbeat", slog.String("error", err.Error()))
				}
				return
			}

			p.metrics.heartbeatsSent.Add(1)
			p.metrics.lastHeartbeat.Store(time.Now().UnixNano())

			p.logger.InfoContext(ctx, "heartbeat", slog.Any("metrics", p.metrics.snapshot()))
		}
	}
}

func (p *producer) executor(ctx context.Context, cancel context.CancelCauseFunc, job *JobRow, work work) {
	p.metrics.addActiveJob(job.ID)
	p.metrics.jobsStarted.Add(1)
	p.metrics.lastJobStarted.Store(time.Now().UnixNano())

	defer func() {
		p.metrics.removeActiveJob(job.ID)
		p.metrics.lastJobCompleted.Store(time.Now().UnixNano())

		cancel(errors.New("context cancelled as executor finished"))

		if r := recover(); r != nil {
			p.logger.ErrorContext(ctx, "Executor panic", slog.Any("panic", r), slog.Int64("job_id", int64(job.ID)),
				slog.String("stack", string(debug.Stack())))
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
		p.metrics.jobsFailed.Add(1)
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

			p.metrics.jobsCancelled.Add(1)
			state = JobStateCancelled
		}

		if p.storage.UpdateJobState(&job.ID, state, time.Now(), attempt) != nil {
			return
		}

		p.logger.DebugContext(ctx, "Job failed", slog.Int64("job_id", int64(job.ID)), slog.String("kind", job.Kind),
			slog.String("args", string(job.Args)), slog.String("error", msg))
		return
	}

	p.metrics.jobsCompleted.Add(1)
	if err := p.storage.UpdateJobState(&job.ID, state, time.Now(), nil); err != nil {
		return
	}

	p.logger.DebugContext(ctx, "Job completed", slog.Int64("job_id", int64(job.ID)), slog.String("kind", job.Kind),
		slog.String("args", string(job.Args)))
}
