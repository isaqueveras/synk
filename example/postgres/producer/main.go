package main

import (
	"context"
	"database/sql"
	"fmt"
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	client := synk.NewClient(ctx, synk.WithStorage(postgresql.New(db)))
	defer client.Stop()

	var i int
	for range time.NewTicker(time.Second).C {
		i++
		if _, err := client.Insert("ownership", worker.ContractArgs{
			CustomerID:   fmt.Sprintf("%d", i),
			CustomerName: fmt.Sprintf("Nome do Cliente %d", i),
		}); err != nil {
			panic(err)
		}
	}
}
