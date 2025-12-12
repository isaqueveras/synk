package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"time"

	"github.com/isaqueveras/synk"
	"github.com/isaqueveras/synk/example/worker"
	"github.com/isaqueveras/synk/storage/postgresql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func main() {
	stdlib.RegisterConnConfig(&pgx.ConnConfig{})

	// Open a connection to the PostgreSQL database using the connection string from the environment variable.
	db, err := sql.Open("pgx", os.Getenv("SYNK_DATABASE_POSTGRES"))
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// Create a logger instance.
	logg := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var opts = []synk.Option{
		// Sets the configuration for the queues to be used.
		synk.WithQueue("default", synk.QueueConfigDefault),
		synk.WithQueue("ownership", &synk.QueueConfig{
			MaxWorkers: 10,
			TimeFetch:  time.Second,
			JobTimeout: time.Minute * 10,
		}),

		// Set storage configuration using PostgreSQL.
		synk.WithStorage(postgresql.New(db)),

		// Sets the logger to be used.
		synk.WithLogger(logg),

		// Sets the workers to be used.
		synk.WithWorker(worker.NewContract()),
		synk.WithWorker(worker.NewBiometry()),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	client := synk.NewClient(ctx, opts...)
	defer client.Shutdown()

	client.ProcessJobs()
}
