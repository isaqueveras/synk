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

	{ // Insert a job into the queue to be processed later
		arg := worker.BiometryArgs{
			BiometryID: "31GDRF8K0C8PY75K0JVXFYY0",
			CustomerID: "01JK7753BK0C8PY75K0JVXFYY0",
		}

		opts := &synk.InsertOptions{
			ScheduledAt: time.Now().Add(time.Minute * 2),
			MaxRetries:  2,
			// Pending:     true,
			Priority: synk.PriorityMedium,
		}

		if _, err = client.Insert("ownership", arg, opts); err != nil {
			panic(err)
		}
	}
}
