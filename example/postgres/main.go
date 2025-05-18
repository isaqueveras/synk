package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/isaqueveras/synk"
	"github.com/isaqueveras/synk/example/worker"
	"github.com/isaqueveras/synk/storage/postgresql"
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
		synk.WithWorker(worker.NewContract()),
		synk.WithWorker(worker.NewBiometry()),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	client := synk.NewClient(ctx, opts...)
	defer client.Stop()

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		client.Exec()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		var i int
		for range time.NewTicker(time.Second / 1000).C {
			i++
			if err := client.Insert("ownership", worker.ContractArgs{
				CustomerID:   fmt.Sprintf("%d", i),
				CustomerName: fmt.Sprintf("Nome do Cliente %d", i),
			}); err != nil {
				panic(err)
			}

			if err := client.Insert("default", worker.ContractArgs{
				CustomerID:   fmt.Sprintf("%d", i+1),
				CustomerName: fmt.Sprintf("Nome do Cliente %d", i+1),
			}); err != nil {
				panic(err)
			}

			if err := client.Insert("ownership", worker.BiometryArgs{
				CustomerID: fmt.Sprintf("%d", i*6),
				BiometryID: fmt.Sprintf("%d", i*3),
			}); err != nil {
				panic(err)
			}
		}
	}()

	wg.Wait()
}
