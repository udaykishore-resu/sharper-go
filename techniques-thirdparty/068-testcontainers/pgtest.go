// Package pgtest is Technique 68: real Postgres in tests with
// testcontainers-go. Other techniques' integration tests use it.
//
// Set PGTEST_DATABASE_URL to reuse a running server instead: each
// call then creates and drops a fresh database on it.
package pgtest

import (
	"context"
	"crypto/rand"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Pin the major version you run in production.
const pgImage = "postgres:17-alpine"

// StartPG returns a pool on a fresh, empty database.
func StartPG(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if url := os.Getenv("PGTEST_DATABASE_URL"); url != "" {
		return freshDB(t, url)
	}
	ctx := t.Context()
	ctr, err := postgres.Run(ctx, pgImage,
		postgres.WithDatabase("ledgerd"),
		postgres.BasicWaitStrategies(),
	)
	tc.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatal(err)
	}
	dsn := ctr.MustConnectionString(ctx,
		"sslmode=disable")
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func freshDB(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background() // cleanup must outlive t.Context()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	name := "t_" + strings.ToLower(rand.Text()[:12])
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		admin.Exec(ctx, "DROP DATABASE "+name)
		admin.Close(ctx)
	})
	return pool
}
