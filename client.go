// Copyright (c) 2025 Isaque Veras
// Licensed under the MIT License.
// See LICENSE file in the project root for full license information.

package synk

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/isaqueveras/synk/storage"
	"github.com/isaqueveras/synk/types"

	"github.com/oklog/ulid/v2"
)

// Client represents a client that manages the configuration,
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
	storage storage.Storage
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

// client defines the interface for a client that can start and stop processing jobs.
// It includes methods to start and stop the client, which manages job queues and workers.
type client interface {
	// Exec begins the processing of jobs by the client.
	// It initializes the necessary context and starts the producers for each queue.
	Exec()

	// Stop halts the processing of jobs by the client.
	// It cancels the context and stops all producers.
	Stop()

	// Insert add a job into the queue to be processed.
	Insert(queue string, params JobArgs) (*int64, error)
}

// NewClient creates a new instance of worker with the provided context and options.
// It initializes the client's configuration, queues, and workers. If no queues or workers are
// configured, it panics. It also generates a unique client ID and sets up producers for each queue.
func NewClient(ctx context.Context, opts ...Option) client {
	clt := &Client{
		ctx:       ctx,
		producers: make(map[string]*producer),
		cfg: &config{
			queues:  make(map[string]*QueueConfig),
			workers: make(map[string]*workerInfo),
		},
	}

	for _, opt := range opts {
		opt(clt.cfg)
	}

	if clt.cfg.storage == nil {
		panic("no storage configured")
	}

	if err := clt.cfg.storage.Ping(); err != nil {
		panic("failed to ping storage: " + err.Error())
	}

	clientID, err := ulid.New(ulid.Now(), rand.Reader)
	if err != nil {
		panic(err)
	}

	clt.id = clientID
	if len(clt.cfg.queues) == 0 || clt.cfg.workers == nil {
		return clt
	}

	for queue, config := range clt.cfg.queues {
		clt.producers[queue] = &producer{
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
func (c *Client) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.workCancel != nil {
		c.workCancel()
	}
}

// Insert add a job into the queue to be processed.
func (c *Client) Insert(queue string, params JobArgs) (*int64, error) {
	args, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return c.cfg.storage.Insert(queue, params.Kind(), args)
}

// Exec it initializes the client's context and starts the producers for each queue.
// Each producer runs in a separate goroutine, fetching and processing jobs according to its configuration.
// The method waits for all producers to complete their work before returning.
// It also sets up a heartbeat mechanism to log the total number of completed jobs at regular intervals.
func (c *Client) Exec() {
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
					return
				case <-time.NewTicker(producer.config.timeFetch).C:
					producer.process(ctx, jobs)
					select {
					case <-c.ctx.Done():
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

	log.Println("Client successfully started:", c.id.String())
	c.wg.Wait()
}
