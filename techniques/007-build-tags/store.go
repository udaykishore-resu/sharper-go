// Package store shows Technique 7: integration tests live behind
// the integration tag, so `go test ./...` never silently skips
// them and `go test -tags=integration ./...` brings them in.
package store

// Ping is the function the tagged test exercises.
func Ping() string { return "pong" }
