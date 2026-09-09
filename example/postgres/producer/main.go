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
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := synk.NewClient(ctx,
		synk.WithNodeID("p_01M21PJSVMFYPBY0ZQWFQJXAKR"),
		synk.WithStorage(postgresql.New(db)))

	{ // Insert jobs with dependencies
		opts := &synk.EnqueueOptions{
			MaxRetries:  15,
			Queue:       "ownership",
			Priority:    synk.PriorityCritical,
			ScheduledAt: time.Now().Add(time.Minute),
		}

		criarbiometriaID, err := client.Enqueue(ctx, "CriarBiometria", worker.BiometryArgs{}, opts)
		if err != nil {
			panic(err)
		}

		opts.DependsOn = []synk.JobID{criarbiometriaID}
		criarContratoAtualTitularID, err := client.Enqueue(ctx, "CriarContratoAtualTitular", worker.BiometryArgs{}, opts)
		if err != nil {
			panic(err)
		}

		criarContratoNovoTitularID, err := client.Enqueue(ctx, "CriarContratoNovoTitular", worker.BiometryArgs{}, opts)
		if err != nil {
			panic(err)
		}

		opts.DependsOn = []synk.JobID{criarContratoAtualTitularID, criarContratoNovoTitularID}
		if _, err = client.Enqueue(ctx, "CriarTermoCessão", worker.BiometryArgs{}, opts); err != nil {
			panic(err)
		}
	}

	time.Sleep(time.Hour)
}
