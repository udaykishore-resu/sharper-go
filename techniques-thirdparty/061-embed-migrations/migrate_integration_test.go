//go:build integration

package store

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	pgtest "github.com/udaykishore-resu/sharper-go/techniques-thirdparty/068-testcontainers"
)

func TestMigrateTwice(t *testing.T) {
	pool := pgtest.StartPG(t)
	db := stdlib.OpenDBFromPool(pool) // leave it untuned
	defer db.Close()
	for range 2 {
		if err := Migrate(t.Context(), db); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM accounts WHERE id = 'house'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("house account: %d, %v", n, err)
	}
}

// Two overlapping migration Jobs take turns on goose's session
// advisory lock: both succeed and each version is recorded once.
func TestOverlappingMigrations(t *testing.T) {
	pool := pgtest.StartPG(t)
	var errs [2]error
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 2 {
		wg.Go(func() {
			<-start
			db := stdlib.OpenDBFromPool(pool)
			defer db.Close()
			errs[i] = Migrate(context.Background(), db)
		})
	}
	close(start)
	wg.Wait()
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("%v / %v", errs[0], errs[1])
	}
	var dup int
	err := pool.QueryRow(t.Context(), `SELECT count(*) FROM (
        SELECT version_id FROM goose_db_version
        GROUP BY version_id HAVING count(*) > 1) d`).Scan(&dup)
	if err != nil || dup != 0 {
		t.Fatalf("%d versions applied twice (%v)", dup, err)
	}
}
