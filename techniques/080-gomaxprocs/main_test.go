package main

import (
	"runtime"
	"testing"
)

// Graph /sched/gomaxprocs:threads next to CFS throttling.
func TestMetricMatches(t *testing.T) {
	if got, want := gomaxprocsMetric(), uint64(runtime.GOMAXPROCS(0)); got != want {
		t.Fatalf("metric %d, GOMAXPROCS %d", got, want)
	}
}

// If code you can't change set the value, hand control back.
func TestSetDefault(t *testing.T) {
	old := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(old)
	runtime.SetDefaultGOMAXPROCS()
	if runtime.GOMAXPROCS(0) < 1 {
		t.Fatal("no default restored")
	}
}
