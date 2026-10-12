//go:build integration

package pgtest

import (
	"strings"
	"testing"
)

// Technique 68's real-server test: a unique index the mock store
// never knew about (transfer IDs) raises SQLSTATE 23505 here.
func TestUniqueViolationIsReal(t *testing.T) {
	pool := StartPG(t)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `CREATE TABLE transfers (id text PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO transfers VALUES ('t1')`); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO transfers VALUES ('t1')`)
	if err == nil || !strings.Contains(err.Error(), "23505") {
		t.Fatalf("err = %v", err)
	}
}
