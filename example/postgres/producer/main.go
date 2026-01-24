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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := synk.NewClient(ctx, synk.WithStorage(postgresql.New(db)))

	opts := &synk.InsertOptions{
		MaxRetries:  2,
		Queue:       "ownership",
		Priority:    synk.PriorityCritical,
		ScheduledAt: time.Now().Add(time.Minute),
	}

	criarbiometriaID, err := client.Insert("CriarBiometria", worker.BiometryArgs{}, opts)
	if err != nil {
		panic(err)
	}

	opts.DependsOn = []*int64{criarbiometriaID}
	criarContratoAtualTitularID, err := client.Insert("CriarContratoAtualTitular", worker.ContractArgs{}, opts)
	if err != nil {
		panic(err)
	}

	criarContratoNovoTitularID, err := client.Insert("CriarContratoNovoTitular", worker.ContractArgs{}, opts)
	if err != nil {
		panic(err)
	}

	opts.DependsOn = []*int64{criarContratoAtualTitularID, criarContratoNovoTitularID}
	if _, err = client.Insert("CriarTermoCessão", worker.ContractArgs{}, opts); err != nil {
		panic(err)
	}
}
