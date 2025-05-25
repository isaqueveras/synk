package main

import (
	"context"
	"database/sql"
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

	db, err := sql.Open("pgx", os.Getenv("SYNK_DATABASE_POSTGRES"))
	if err != nil {
		panic(err)
	}
	defer db.Close()

	var opts = []synk.Option{
		// Sets the configuration for the queues to be used.
		synk.WithQueue("default", synk.QueueConfigDefault),
		synk.WithQueue("ownership", &synk.QueueConfig{
			MaxWorkers: 100,
			TimeFetch:  time.Second / 10,
			JobTimeout: time.Minute,
		}),

		// Set storage configuration using PostgreSQL.
		synk.WithStorage(postgresql.New(db)),

		// Sets the workers to be used.
		synk.WithWorker(worker.NewContract()),
		synk.WithWorker(worker.NewBiometry()),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	client := synk.NewClient(ctx, opts...)
	defer client.Stop()

	client.Exec()
}
