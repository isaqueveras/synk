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
	"github.com/oklog/ulid/v2"
)

func main() {
	stdlib.RegisterConnConfig(&pgx.ConnConfig{})

	db, err := sql.Open("pgx", os.Getenv("SYNK_DATABASE_POSTGRES"))
	if err != nil {
		panic(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	logg := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client := synk.NewClient(ctx, synk.WithStorage(postgresql.New(db)), synk.WithLogger(logg))
	defer client.Shutdown()

	for {
		{ // Insert a job into the queue to be processed later
			arg := worker.BiometryArgs{
				BiometryID: ulid.Make().String(),
				CustomerID: ulid.Make().String(),
			}

			if _, err = client.Insert("ownership", arg, &synk.InsertOptions{
				MaxRetries: 3,
				Priority:   synk.PriorityMedium,
			}); err != nil {
				panic(err)
			}
		}

		{
			arg := worker.ContractArgs{
				CustomerID:   ulid.Make().String(),
				CustomerName: "John Doe",
			}

			if _, err := client.Insert("default", arg, &synk.InsertOptions{
				MaxRetries: 7,
				Priority:   synk.PriorityCritical,
			}); err != nil {
				panic(err)
			}
		}

		time.Sleep(time.Second)
	}
}
