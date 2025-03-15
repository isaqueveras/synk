package main

import (
	"context"
	"database/sql"
	"os"
	"time"

	"github.com/isaqueveras/synk"
	"github.com/isaqueveras/synk/storage/postgresql"

	"github.com/isaqueveras/synk/example/domain/biometry"
	"github.com/isaqueveras/synk/example/domain/contract"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func main() {
	stdlib.RegisterConnConfig(&pgx.ConnConfig{})

	var database = os.Getenv("SYNK_DATABASE_POSTGRES")
	db, err := sql.Open("pgx", database)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	var opts = []synk.Option{
		// Sets the configuration for the queues to be used.
		synk.WithQueue("default", synk.QueueConfigDefault),
		synk.WithQueue("ownership", &synk.QueueConfig{
			MaxWorkers: 10,
			TimeFetch:  time.Second,
			JobTimeout: time.Minute,
		}),

		// Set storage configuration using PostgreSQL.
		synk.WithStorage(postgresql.New(db)),

		// Sets the workers to be used.
		synk.WithWorker(contract.NewWorker()),
		synk.WithWorker(biometry.NewWorker()),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	client := synk.NewClient(ctx, opts...)

	client.Start()
	defer client.Stop()
}
