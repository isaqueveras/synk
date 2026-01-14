// Copyright (c) 2025 Isaque Veras
// Licensed under the MIT License.
// See LICENSE file in the project root for full license information.

package synk

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Client represents a Client that manages the configuration,
// context, and producers for a specific task.
type Client struct {
	id  ulid.ULID
	cfg *config
	wg  sync.WaitGroup

	producers map[string]*producer

	ctx        context.Context
	cancel     context.CancelFunc
	workCancel context.CancelFunc
}

type config struct {
	queues  map[string]*QueueConfig
	workers map[string]*workerInfo
	cleaner *CleanerConfig
	storage Storage
	logger  *slog.Logger
}

// QueueConfigDefault is the default configuration for the queue system.
// It sets the maximum number of workers to 100, the time interval to fetch jobs to 200 milliseconds,
// and the timeout for each job to 1 minute.
var QueueConfigDefault = &QueueConfig{
	MaxWorkers: 100,
	TimeFetch:  time.Millisecond * 200,
	JobTimeout: time.Minute,
}

// QueueConfig holds the configuration settings for a job queue.
// It includes the maximum number of workers, the time interval for fetching jobs,
// and the timeout duration for each job.
type QueueConfig struct {
	MaxWorkers uint16
	TimeFetch  time.Duration
	JobTimeout time.Duration
}

// NewClient creates a new instance of worker with the provided context and options.
// It initializes the client's configuration, queues, and workers. If no queues or workers are
// configured, it panics. It also generates a unique client ID and sets up producers for each queue.
func NewClient(ctx context.Context, opts ...Option) *Client {
	clt := &Client{
		ctx:       ctx,
		producers: make(map[string]*producer),
		cfg: &config{
			queues:  make(map[string]*QueueConfig),
			workers: make(map[string]*workerInfo),
			logger:  slog.Default(),
		},
	}

	for _, opt := range opts {
		opt(clt.cfg)
	}

	clientID, err := ulid.New(ulid.Now(), rand.Reader)
	if err != nil {
		clt.cfg.logger.ErrorContext(ctx, "failed to create client ID: "+err.Error())
		return nil
	}

	clt.id = clientID
	clt.cfg.logger = clt.cfg.logger.WithGroup("client").With(slog.String("id", clt.id.String()))

	if clt.cfg.storage == nil {
		clt.cfg.logger.ErrorContext(ctx, "no storage configured")
		return nil
	}

	if err := clt.cfg.storage.Ping(); err != nil {
		clt.cfg.logger.ErrorContext(ctx, "failed to ping storage: "+err.Error())
		return nil
	}

	if len(clt.cfg.queues) == 0 || clt.cfg.workers == nil {
		clt.cfg.logger.DebugContext(ctx, "no queues or workers configured")
		return clt
	}

	if clt.cfg.cleaner != nil {
		clt.wg.Add(1)
		go func() {
			defer clt.wg.Done()
			clt.cleaner(ctx, clt.cfg.cleaner)
		}()
	}

	for queue, config := range clt.cfg.queues {
		logger := clt.cfg.logger.WithGroup("producer").With(slog.String("queue", queue))
		clt.producers[queue] = &producer{
			clientID:    &clt.id,
			logger:      logger,
			workers:     clt.cfg.workers,
			storage:     clt.cfg.storage,
			jobTimeout:  config.JobTimeout,
			jobsChannel: make(chan *JobRow, config.MaxWorkers),
			config: &producerConfig{
				maxWorkerCount: config.MaxWorkers,
				timeFetch:      config.TimeFetch,
				queueName:      queue,
				workID:         clt.id.String(),
				workers:        clt.cfg.workers,
				jobTimeout:     config.JobTimeout,
			},
		}
	}

	return clt
}

// Shutdown cancels the client's context and stops any ongoing work.
// It calls the cancel functions associated with the client to gracefully shut down any operations.
func (c *Client) Shutdown() {
	c.wg.Wait()

	c.cfg.logger.Debug("Stopping client")
	if c.cancel != nil {
		c.cfg.logger.Debug("Stopping client context")
		c.cancel()
	}
	if c.workCancel != nil {
		c.cfg.logger.Debug("Stopping work cancel function")
		c.workCancel()
	}
}

// Insert add a job into the queue to be processed.
func (c *Client) Insert(queue string, params JobArgs, options ...*InsertOptions) (*int64, error) {
	state, option, err := getOptionsOrDefault(options...)
	if err != nil {
		return nil, err
	}

	args, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	row := &JobRow{
		Kind:    params.Kind(),
		Queue:   queue,
		Args:    args,
		State:   state,
		Options: option,
	}

	var jobID *int64
	if jobID, err = c.cfg.storage.Insert(row); err != nil {
		c.cfg.logger.DebugContext(c.ctx, "failed to insert job into queue", slog.String("error", err.Error()),
			slog.String("queue", queue), slog.String("kind", params.Kind()), slog.Any("args", params))
		return nil, err
	}

	c.cfg.logger.DebugContext(c.ctx, "job inserted into queue", slog.String("queue", queue),
		slog.Int64("job_id", *jobID), slog.String("kind", params.Kind()), slog.Any("args", params))

	return jobID, nil
}

// InsertTx adds a job into the specified queue within the context of the provided
// transaction, allowing the operation to be part of an atomic database transaction.
func (c *Client) InsertTx(tx *sql.Tx, queue string, params JobArgs, options ...*InsertOptions) (id *int64, err error) {
	state, option, err := getOptionsOrDefault(options...)
	if err != nil {
		return nil, err
	}

	row := &JobRow{
		Kind:    params.Kind(),
		Queue:   queue,
		State:   state,
		Options: option,
	}

	if row.Args, err = json.Marshal(params); err != nil {
		return nil, err
	}

	if id, err = c.cfg.storage.InsertTx(tx, row); err != nil {
		c.cfg.logger.DebugContext(c.ctx, "failed to insert job into queue within transaction", slog.String("error", err.Error()),
			slog.String("queue", queue), slog.String("kind", params.Kind()), slog.Any("args", params))
		return nil, err
	}

	c.cfg.logger.DebugContext(c.ctx, "job inserted into queue within transaction", slog.String("queue", queue),
		slog.Int64("job_id", *id), slog.String("kind", params.Kind()), slog.Any("args", params))

	return id, nil
}

// Start it initializes the client's context and starts the producers for each queue.
// Each producer runs in a separate goroutine, fetching and processing jobs according to its configuration.
// The method waits for all producers to complete their work before returning.
// It also sets up a heartbeat mechanism to log the total number of completed jobs at regular intervals.
func (c *Client) Start() {
	c.ctx, c.cancel = context.WithCancel(c.ctx)

	ctx, cancel := context.WithCancel(c.ctx)
	c.workCancel = cancel

	c.wg.Add(len(c.producers))
	for _, producer := range c.producers {
		go func() {
			defer c.wg.Done()

			// starts a heartbeat goroutine for each producer to monitor their status.
			go producer.heartbeat(c.ctx)

			jobs := make(chan []*JobRow)
			for {
				select {
				case <-c.ctx.Done():
					producer.logger.DebugContext(c.ctx, "Producer context done: "+c.ctx.Err().Error())
					return
				case <-time.NewTicker(producer.config.timeFetch).C:
					producer.process(ctx, jobs)
					select {
					case <-c.ctx.Done():
						producer.logger.DebugContext(c.ctx, "Producer context done: "+c.ctx.Err().Error())
						return
					default:
					}
				case <-producer.jobsChannel:
					producer.numJobsActive.Add(-1)
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

func getOptionsOrDefault(options ...*InsertOptions) (JobState, *InsertOptions, error) {
	opts := &InsertOptions{}
	if len(options) > 0 {
		opts = options[0]
	}

	if opts.Priority == 0 {
		opts.Priority = PriorityMedium
	}

	state := JobStateAvailable
	if !opts.ScheduledAt.IsZero() {
		state = JobStateScheduled
	}

	if (opts.Priority > PriorityLow) || (opts.Priority < PriorityCritical) {
		return state, nil, errors.New("priority must be between 1 and 4")
	}

	if opts.ScheduledAt.IsZero() || opts.ScheduledAt.Before(time.Now()) {
		opts.ScheduledAt = time.Now().UTC()
	}

	if opts.MaxRetries == 0 {
		opts.MaxRetries = 7
	}

	if opts.Pending {
		state = JobStatePending
	}

	return state, opts, nil
}

func (c *Client) cleaner(ctx context.Context, clear *CleanerConfig) {
	if c.cfg.cleaner.CleanInterval == 0 {
		c.cfg.logger.ErrorContext(ctx, "cleaner interval is required")
		return
	}

	if c.cfg.cleaner.ByStatus == nil {
		c.cfg.logger.ErrorContext(ctx, "cleaner by status is required")
		return
	}

	ticker := time.NewTicker(clear.CleanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.cfg.logger.ErrorContext(ctx, "Heartbeat context done: "+ctx.Err().Error())
			return
		case <-ticker.C:
			if err := c.cfg.storage.Cleaner(clear); err != nil {
				c.cfg.logger.ErrorContext(ctx, "failed to clean jobs", slog.String("error", err.Error()))
			}
		}
	}
}
