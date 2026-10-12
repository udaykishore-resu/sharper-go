//go:build integration

package store

import "testing"

// This file is not in the binary unless you pass
// -tags=integration. Plain `go vet ./...` doesn't read it either;
// run `go vet -tags=integration ./...` in CI.
func TestPingIntegration(t *testing.T) {
	if Ping() != "pong" {
		t.Fatal("integration test failed")
	}
}
