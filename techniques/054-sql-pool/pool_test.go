package pool

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"
)

// A connector that never connects: sql.OpenDB is lazy, so the
// pool settings can be checked without a database.
type nopConnector struct{}

func (nopConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, errors.New("no database in this test")
}
func (nopConnector) Driver() driver.Driver { return nil }

func TestTunePool(t *testing.T) {
	db := sql.OpenDB(nopConnector{})
	defer db.Close()
	tunePool(db)
	if got := db.Stats().MaxOpenConnections; got != 12 {
		t.Fatalf("MaxOpenConnections = %d", got)
	}
	if perPod(200, 30, 12) != 14 {
		t.Fatal("170 over 12 replicas is 14, so 12 is safe")
	}
	if n, d := waitDelta(sql.DBStats{WaitCount: 3}, sql.DBStats{WaitCount: 10,
		WaitDuration: time.Second}); n != 7 || d != time.Second {
		t.Fatalf("delta %d %v", n, d)
	}
}
