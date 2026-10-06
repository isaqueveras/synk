// Copyright (c) 2025 Isaque Veras
// Licensed under the MIT License.
// See LICENSE file in the project root for full license information.

// Package synk provides a distributed job queue system for processing tasks in a distributed environment.
package synk

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"
)

type client struct {
	nodeID NodeID

	cfg *config
	wg  sync.WaitGroup

	producers map[string]*producer

	ctx    context.Context
	cancel context.CancelFunc

	workCtx    context.Context
	workCancel context.CancelFunc
}

type config struct {
	nodeID  NodeID
	queues  Queues
	workers map[string]*workerInfo
	cleaner *CleanerConfig
	storage Storage
	logger  *slog.Logger
}

// QueueConfig holds the configuration settings for a job queue.
// It includes the maximum number of workers, the time interval for fetching jobs,
// and the timeout duration for each job.
type QueueConfig struct {
	MaxWorkers uint64
	TimeFetch  time.Duration

	workCtx    context.Context
	JobTimeout time.Duration
}

// NewClient creates a new instance of worker with the provided context and options.
// It initializes the client's configuration, queues, and workers. If no queues or workers are
// configured, it panics. It also generates a unique client ID and sets up producers for each queue.
func NewClient(ctx context.Context, opts ...Option) *client {
	ctx, cancel := context.WithCancel(ctx)

	clt := &client{
		ctx: ctx, cancel: cancel,
		producers: make(map[string]*producer),
		cfg: &config{
			queues:  make(map[string]*QueueConfig),
			workers: make(map[string]*workerInfo),
			logger:  slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn})),
		},
	}

	for _, opt := range opts {
		opt(clt.cfg)
	}

	clt.nodeID = clt.cfg.nodeID
	if clt.nodeID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			clt.cfg.logger.Error("failed to get hostname: " + err.Error())
			hostname = "unknown"
		}
		clt.nodeID = NodeID(hostname + "_" + time.Now().Format(time.RFC3339))
	}

	clt.cfg.logger = clt.cfg.logger.With(slog.String("node_id", clt.nodeID.String()))
	if clt.cfg.storage == nil {
		clt.cfg.logger.Error("no storage configured")
		return clt
	}

	if err := clt.cfg.storage.Ping(); err != nil {
		clt.cfg.logger.Error("failed to ping storage: " + err.Error())
		return clt
	}

	clt.workCtx, clt.workCancel = context.WithCancel(context.WithValue(ctx, ContextKeyClient{}, clt))
	if len(clt.cfg.queues) == 0 || clt.cfg.workers == nil {
		clt.cfg.logger.Debug("no queues or workers configured")
		return clt
	}

	for queue, config := range clt.cfg.queues {
		logger := clt.cfg.logger.With(slog.String("queue", queue))
		clt.producers[queue] = &producer{
			nodeID:     &clt.nodeID,
			logger:     logger,
			workers:    clt.cfg.workers,
			storage:    clt.cfg.storage,
			jobTimeout: config.JobTimeout,
			config: &producerConfig{
				maxWorkerCount: uint64(config.MaxWorkers),
				timeFetch:      config.TimeFetch,
				queueName:      queue,
				workers:        clt.cfg.workers,
				jobTimeout:     config.JobTimeout,
			},
		}
	}

	return clt
}

// Enqueue adds a job into the specified queue within the context of the provided
// transaction, allowing the operation to be part of an atomic database transaction.
func (c *client) Enqueue(ctx context.Context, name string, args JobArgs, options ...*EnqueueOptions) (JobID, error) {
	state, option, err := getOptionsOrDefault(options...)
	if err != nil {
		return 0, err
	}

	if name == "" {
		return 0, ErrJobNameRequired
	}

	if args.Kind() == "" {
		return 0, ErrJobKindRequired
	}

	row := &JobRow{
		Name:    name,
		Kind:    args.Kind(),
		Queue:   option.Queue,
		State:   state,
		Options: option,
	}

	if row.Args, err = json.Marshal(args); err != nil {
		return 0, err
	}

	var jobID *JobID
	if jobID, err = c.cfg.storage.Enqueue(ctx, option.Transaction, row); err != nil {
		c.cfg.logger.Debug("failed to insert job into queue", slog.String("error", err.Error()),
			slog.String("queue", option.Queue), slog.String("kind", args.Kind()), slog.Any("args", args))
		return 0, err
	}

	c.cfg.logger.Debug("job inserted into queue", slog.String("queue", option.Queue),
		slog.Int64("job_id", int64(*jobID)), slog.String("kind", args.Kind()), slog.Any("args", args))

	return *jobID, nil
}

// InitializeProducers it initializes the client's context and starts the producers for each queue.
// Each producer runs in a separate goroutine, fetching and processing jobs according to its configuration.
// The method waits for all producers to complete their work before returning.
// It also sets up a heartbeat mechanism to log the total number of completed jobs at regular intervals.
func (c *client) InitializeProducers() {
	c.wg.Add(len(c.producers))
	for _, producer := range c.producers {
		pdc := producer

		go func() {
			defer c.wg.Done()

			ticker := time.NewTicker(pdc.config.timeFetch)
			defer ticker.Stop()

			go pdc.heartbeat(c.ctx, c.cfg.queues.Names())

			for {
				select {
				case <-c.ctx.Done():
					pdc.logger.DebugContext(c.ctx, "producer context done: "+c.ctx.Err().Error())
					return
				case <-ticker.C:
					pdc.process(c.workCtx)
				}
			}
		}()
	}

	c.cfg.logger.InfoContext(c.ctx, "Client started",
		slog.Int("num_producers", len(c.producers)),
		slog.Int("num_queues", len(c.cfg.queues)),
		slog.Int("num_workers", len(c.cfg.workers)),
	)

	c.wg.Wait()
}

// InitializeCleaner runs the cleaner function with the provided context and cleaner configuration.
func (c *client) InitializeCleaner() {
	if c.cfg.cleaner.CleanInterval == 0 {
		c.cfg.logger.Error("cleaner interval is required")
		return
	}

	if c.cfg.cleaner.ByStatus == nil {
		c.cfg.logger.Error("cleaner by status is required")
		return
	}

	ticker := time.NewTicker(c.cfg.cleaner.CleanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			c.cfg.logger.Error("Heartbeat context done: " + c.ctx.Err().Error())
			return
		case <-ticker.C:
			totalDeleted, err := c.cfg.storage.Cleaner(c.cfg.cleaner)
			if err != nil {
				c.cfg.logger.Error("failed to clean jobs", slog.String("error", err.Error()))
				continue
			}
			c.cfg.logger.Info("Total cleaned jobs", slog.Int64("jobs_cleaned", totalDeleted))
		}
	}
}

// CancelJob cancels a job by its ID.
func (c *client) CancelJob(ctx context.Context, jobID JobID) error {
	return c.cfg.storage.Cancel(&jobID)
}

// RetryJob retries a job by its ID.
func (c *client) RetryJob(ctx context.Context, jobID JobID) error {
	return c.cfg.storage.Retry(&jobID)
}

// DeleteJob deletes a job by its ID.
func (c *client) DeleteJob(ctx context.Context, jobID JobID) error {
	return c.cfg.storage.Delete(&jobID)
}

// ContextKeyClient is a context key used to store the client instance in the context.
type ContextKeyClient struct{}

// ClientFromContext returns the client instance from the context.
// If the client is not found in the context, it returns an error.
func ClientFromContext(ctx context.Context) (*client, error) {
	client, ok := ctx.Value(ContextKeyClient{}).(*client)
	if !ok || client == nil {
		return nil, errors.New("client not found in context")
	}
	return client, nil
}
