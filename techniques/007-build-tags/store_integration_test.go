//go:build integration

package store

import (
	"os"
	"testing"
)

// This file is not in the test binary unless you pass
// -tags=integration. Plain `go vet ./...` doesn't read it either;
// run `go vet -tags=integration ./...` in CI.
//
// Inside the tag, a missing database is a failure, never a skip:
// a job that sets the tag but forgot to start Postgres goes red.
func TestPingIntegration(t *testing.T) {
	url := os.Getenv("LEDGERD_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("LEDGERD_TEST_DATABASE_URL is not set; " +
			"the integration job must start Postgres")
	}
	if Ping() != "pong" {
		t.Fatal("integration test failed")
	}
}
