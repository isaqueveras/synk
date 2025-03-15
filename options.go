// Copyright (c) 2025 Isaque Veras
// Licensed under the MIT License.
// See LICENSE file in the project root for full license information.

package synk

import (
	"fmt"
	"time"

	"github.com/isaqueveras/synk/storage"
)

// Option represents a function that modifies the configuration settings
// of a Config object. It allows for flexible and customizable configuration
// of the application by applying various options.
type Option func(*config)

// WithWorker is an option function that sets the Workers field in the Config struct.
// It takes a pointer to a Workers struct and returns an Option function that assigns
// the provided Workers to the Config.
func WithWorker[T JobArgs](w Worker[T]) Option {
	return func(cfg *config) {
		var args T
		if _, ok := cfg.workers[args.Kind()]; ok {
			panic(fmt.Errorf("worker for kind %q is already registered", args.Kind()))
		}

		// Register a worker for a specific job kind.
		cfg.workers[args.Kind()] = &workerInfo{
			args: args, work: newWorkWrapper(w),
		}
	}
}

// WithQueue is an option function that configures a queue with the given name and optional QueueConfig.
// If no QueueConfig is provided, a default configuration with MaxWorkers set to 100 is used.
// The function returns an Option that updates the Config with the specified queue configuration.
func WithQueue(name string, queueCfg ...*QueueConfig) Option {
	q := &QueueConfig{
		MaxWorkers: 100,
		TimeFetch:  time.Millisecond * 200,
	}

	if len(queueCfg) >= 1 {
		q = queueCfg[0]
	}

	return func(cfg *config) {
		cfg.queues[name] = q
	}
}

// WithStorage sets the storage backend for the synk client.
// In this case, it uses a PostgreSQL storage implementation.
// This option is essential for persisting job data and ensuring
// reliable job processing.
func WithStorage(storage storage.Storage) Option {
	return func(cfg *config) {
		cfg.storage = storage
	}
}
