// Copyright (c) 2025 Isaque Veras
// Licensed under the MIT License.
// See LICENSE file in the project root for full license information.

package synk

import (
	"fmt"
	"log/slog"
	"time"
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

// CleanerConfigDefault provides default settings for the job cleaner.
var CleanerConfigDefault = &CleanerConfig{
	CleanInterval: time.Hour * 24, // every 24 hours
	ByStatus: map[JobState]time.Duration{
		JobStateCompleted: time.Hour * 24 * 30, // 30 days
		JobStateCancelled: time.Hour * 24 * 90, // 90 days
	},
}

// CleanerConfig represents the configuration settings for the job cleaner.
type CleanerConfig struct {
	// CleanInterval is the time interval at which the cleaner will run to remove old jobs.
	CleanInterval time.Duration

	// ByStatus is a map that defines the retention duration for jobs based on their status.
	// The key is the JobState, and the value is the duration after which jobs in that state
	// should be cleaned up.
	ByStatus map[JobState]time.Duration
}

// WithCleaner is an option function that sets the cleaner configuration for the synk client.
// It takes a CleanerConfig struct as an argument and returns an Option function that updates
// the Config with the provided cleaner configuration.
func WithCleaner(cleaner *CleanerConfig) Option {
	return func(cfg *config) {
		cfg.cleaner = cleaner
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
func WithStorage(storage Storage) Option {
	return func(cfg *config) {
		cfg.storage = storage
	}
}

// WithLogger sets the logger for the synk client.
func WithLogger(logger *slog.Logger) Option {
	return func(cfg *config) {
		cfg.logger = logger
	}
}
