package main

import (
	"context"
	"database/sql"
	"fmt"
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	logg := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client := synk.NewClient(ctx, synk.WithStorage(postgresql.New(db)), synk.WithLogger(logg))
	defer client.Stop()

	if err := withTransaction(client, db); err != nil {
		panic(err)
	}

	var i int
	for range time.NewTicker(time.Second).C {
		i++

		client.Insert("default", worker.ContractArgs{
			CustomerID:   fmt.Sprintf("%d", i),
			CustomerName: fmt.Sprintf("Customer Name %d-tx", i),
		})

		client.Insert("default", worker.BiometryArgs{
			BiometryID: fmt.Sprintf("%d", i),
			CustomerID: fmt.Sprintf("%d", i),
		})
	}
}

func withTransaction(client synk.Client, db *sql.DB) error {
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = client.InsertTx(tx, "ownership", worker.ContractArgs{
		CustomerID:   "01JK7753BK0C8PY75K0JVXFYY0",
		CustomerName: "John Doe",
	}); err != nil {
		return err
	}

	if _, err = client.InsertTx(tx, "ownership", worker.BiometryArgs{
		BiometryID: "31GDRF8K0C8PY75K0JVXFYY0",
		CustomerID: "01JK7753BK0C8PY75K0JVXFYY0",
	}); err != nil {
		return err
	}

	return tx.Commit()
}
