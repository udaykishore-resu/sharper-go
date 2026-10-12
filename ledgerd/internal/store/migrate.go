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
// advisory lock, so two overlapping deploys never migrate at
// once. ledgerd runs it as `ledgerd migrate`, once per deploy,
// from a Kubernetes Job.
func (s *Store) Migrate(ctx context.Context, log *slog.Logger) error {
	// Leave this handle untuned: pool sizing lives on pgxpool,
	// and OpenDBFromPool keeps no idle connections (Technique 54).
	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()
	return migrate(ctx, db, log)
}

func migrate(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	fsys, err := fs.Sub(embedded, "migrations")
	if err != nil {
		return err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres,
		db, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	res, err := p.Up(ctx)
	for _, r := range res {
		log.Info("migration applied",
			"migration", r.Source.Version, "dur_ms", r.Duration.Milliseconds())
	}
	return err
}
