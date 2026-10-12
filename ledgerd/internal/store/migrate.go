package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// The schema ships inside the binary (Technique 61).
//
//go:embed migrations/*.sql
var embedded embed.FS

// Migrate applies every pending migration under a Postgres
// session advisory lock, so two overlapping Jobs take turns
// instead of migrating at once (TestConcurrentMigrate). A Job
// that waits polls every 5s and gives up after goose's default
// of 60 tries. ledgerd runs it as `ledgerd migrate`, once per deploy,
// from a Kubernetes Job.
func (s *Store) Migrate(ctx context.Context, log *slog.Logger) error {
	// Leave this handle untuned: pool sizing lives on pgxpool,
	// and OpenDBFromPool keeps no idle connections (Technique 54).
	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()
	return migrate(ctx, db, log)
}

func migrate(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	p, err := provider(db)
	if err != nil {
		return err
	}
	res, err := p.Up(ctx)
	for _, r := range res {
		log.Info("migration applied",
			"migration", r.Source.Version, "dur_ms", r.Duration.Milliseconds())
	}
	return err
}

func provider(db *sql.DB) (*goose.Provider, error) {
	fsys, err := fs.Sub(embedded, "migrations")
	if err != nil {
		return nil, err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	p, err := goose.NewProvider(goose.DialectPostgres,
		db, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}
	return p, nil
}

// CheckSchema reports an error if the database is behind the
// newest migration embedded in this binary. Embedding ties the
// files to the build; it does not prove the Job ran them, so a
// server should call this before it reports ready. A database
// that is ahead is fine: that is an old pod during an
// expand-and-contract rollout (Technique 61).
func (s *Store) CheckSchema(ctx context.Context) error {
	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()
	p, err := provider(db)
	if err != nil {
		return err
	}
	have, want, err := p.GetVersions(ctx)
	if err != nil {
		return fmt.Errorf("schema version: %w", err)
	}
	if have < want {
		return fmt.Errorf("schema at version %d, binary needs %d",
			have, want)
	}
	return nil
}
