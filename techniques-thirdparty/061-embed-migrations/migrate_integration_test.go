//go:build integration

package store

import (
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
