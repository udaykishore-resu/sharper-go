package store

import "testing"

func TestPingUnit(t *testing.T) {
	if Ping() != "pong" {
		t.Fatal("unit test failed")
	}
}
