package main

import (
	"context"
	"database/sql"
	"log"
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

	client := synk.NewClient(ctx, synk.WithClientID("produtor01"), synk.WithStorage(postgresql.New(db)))

	opts := &synk.InsertOptions{
		MaxRetries:  15,
		Queue:       "ownership",
		Priority:    synk.PriorityCritical,
		ScheduledAt: time.Now().Add(time.Minute),
	}

	criarbiometriaID, err := client.Insert("CriarBiometria", worker.BiometryArgs{}, opts)
	if err != nil {
		panic(err)
	}

	opts.DependsOn = []*int64{criarbiometriaID}
	criarContratoAtualTitularID, err := client.Insert("CriarContratoAtualTitular", worker.BiometryArgs{}, opts)
	if err != nil {
		panic(err)
	}

	criarContratoNovoTitularID, err := client.Insert("CriarContratoNovoTitular", worker.BiometryArgs{}, opts)
	if err != nil {
		panic(err)
	}

	opts.DependsOn = []*int64{criarContratoAtualTitularID, criarContratoNovoTitularID}
	if _, err = client.Insert("CriarTermoCessão", worker.BiometryArgs{}, opts); err != nil {
		panic(err)
	}

	time.Sleep(time.Minute)

	log.Print("retrying job")
	if err := client.Retry(ctx, criarbiometriaID); err != nil {
		panic(err)
	}

	time.Sleep(time.Minute)
}
