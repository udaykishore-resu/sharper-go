// Package pool is Technique 54: the four database/sql pool
// settings that matter, sized from a formula.
package pool

import (
	"database/sql"
	"time"
)

// Pod has 8 workers; Postgres max_connections is 200,
// shared by 12 replicas plus admin headroom.
func tunePool(db *sql.DB) {
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(12)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
}

// perPod is the formula in the comment above: max_connections
// minus headroom, divided by the autoscaler's max replicas.
func perPod(maxConnections, headroom, maxReplicas int) int {
	return (maxConnections - headroom) / maxReplicas
}

// waitDelta is what you log every 15 seconds: if waits climb,
// the pool is the queue.
func waitDelta(prev, now sql.DBStats) (int64, time.Duration) {
	return now.WaitCount - prev.WaitCount,
		now.WaitDuration - prev.WaitDuration
}
