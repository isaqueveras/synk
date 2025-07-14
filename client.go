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

	"github.com/isaqueveras/synk/storage"
	"github.com/isaqueveras/synk/types"

	"github.com/oklog/ulid/v2"
)

// IClient represents a client that manages the configuration,
// context, and producers for a specific task.
type IClient struct {
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
	storage storage.Storage
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

// Client defines the interface for a Client that can start and stop processing jobs.
// It includes methods to start and stop the Client, which manages job queues and workers.
type Client interface {
	// Exec begins the processing of jobs by the client.
	// It initializes the necessary context and starts the producers for each queue.
	Exec()

	// Stop halts the processing of jobs by the client.
	// It cancels the context and stops all producers.
	Stop()

	// Insert add a job into the queue to be processed.
	Insert(queue string, params JobArgs, opts ...*types.InsertOptions) (*int64, error)

	// InsertTx adds a job into the specified queue within the context of the provided
	// transaction, allowing the operation to be part of an atomic database transaction.
	InsertTx(tx *sql.Tx, queue string, params JobArgs, opts ...*types.InsertOptions) (*int64, error)
}

// NewClient creates a new instance of worker with the provided context and options.
// It initializes the client's configuration, queues, and workers. If no queues or workers are
// configured, it panics. It also generates a unique client ID and sets up producers for each queue.
func NewClient(ctx context.Context, opts ...Option) Client {
	clt := &IClient{
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

	for queue, config := range clt.cfg.queues {
		logger := clt.cfg.logger.WithGroup("producer").With(slog.String("queue", queue))
		clt.producers[queue] = &producer{
			clientID:    &clt.id,
			logger:      logger,
			workers:     clt.cfg.workers,
			storage:     clt.cfg.storage,
			jobTimeout:  config.JobTimeout,
			jobsChannel: make(chan *types.JobRow, config.MaxWorkers),
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

// Stop cancels the client's context and stops any ongoing work.
// It calls the cancel functions associated with the client to
// gracefully shut down any operations.
func (c *IClient) Stop() {
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
func (c *IClient) Insert(queue string, params JobArgs, options ...*types.InsertOptions) (*int64, error) {
	args, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	opts := &types.InsertOptions{}
	if len(options) > 0 {
		opts = options[0]
	}

	if opts.Priority == 0 {
		opts.Priority = 4
	}

	if opts.Priority > 4 {
		return nil, errors.New("priority must be between 1 and 4")
	}

	state := types.JobStateAvailable
	if !opts.ScheduledAt.IsZero() {
		state = types.JobStateScheduled
	}

	if opts.ScheduledAt.IsZero() || opts.ScheduledAt.Before(time.Now()) {
		opts.ScheduledAt = time.Now().UTC()
	}

	if opts.MaxRetries == 0 {
		opts.MaxRetries = 7
	}

	if opts.Pending {
		state = types.JobStatePending
	}

	job := &types.JobRow{
		Kind:    params.Kind(),
		Queue:   queue,
		Args:    args,
		State:   state,
		Options: opts,
	}

	var jobID *int64
	if jobID, err = c.cfg.storage.Insert(job); err != nil {
		c.cfg.logger.ErrorContext(c.ctx, "failed to insert job into queue", slog.String("error", err.Error()),
			slog.String("queue", queue), slog.String("kind", params.Kind()), slog.Any("args", params))
		return nil, err
	}

	c.cfg.logger.DebugContext(c.ctx, "job inserted into queue", slog.String("queue", queue),
		slog.Int64("job_id", *jobID), slog.String("kind", params.Kind()), slog.Any("args", params))

	return jobID, nil
}

// InsertTx adds a job into the specified queue within the context of the provided
// transaction, allowing the operation to be part of an atomic database transaction.
func (c *IClient) InsertTx(tx *sql.Tx, queue string, params JobArgs, options ...*types.InsertOptions) (*int64, error) {
	args, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	jobRow := &types.JobRow{
		Kind:  params.Kind(),
		Queue: queue,
		Args:  args,
		State: types.JobStateAvailable,
	}

	id, err := c.cfg.storage.InsertTx(tx, jobRow)
	if err != nil {
		c.cfg.logger.ErrorContext(c.ctx, "failed to insert job into queue within transaction", slog.String("error", err.Error()),
			slog.String("queue", queue), slog.String("kind", params.Kind()), slog.Any("args", params))
		return nil, err
	}

	c.cfg.logger.DebugContext(c.ctx, "job inserted into queue within transaction", slog.String("queue", queue),
		slog.Int64("job_id", *id), slog.String("kind", params.Kind()), slog.Any("args", params))

	return id, nil
}

// Exec it initializes the client's context and starts the producers for each queue.
// Each producer runs in a separate goroutine, fetching and processing jobs according to its configuration.
// The method waits for all producers to complete their work before returning.
// It also sets up a heartbeat mechanism to log the total number of completed jobs at regular intervals.
func (c *IClient) Exec() {
	c.ctx, c.cancel = context.WithCancel(c.ctx)

	ctx, cancel := context.WithCancel(c.ctx)
	c.workCancel = cancel

	c.wg.Add(len(c.producers))
	for _, producer := range c.producers {
		producer := producer

		go func() {
			defer c.wg.Done()

			// starts a heartbeat goroutine for each producer to monitor their status.
			go producer.heartbeat(c.ctx)

			jobs := make(chan []*types.JobRow)
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
					producer.totalWorkPerformed.Add(1)
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
