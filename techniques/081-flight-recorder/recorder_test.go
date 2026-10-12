package flight

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshot(t *testing.T) {
	fr, err := StartRecorder()
	if err != nil {
		t.Fatal(err)
	}
	defer fr.Stop()
	if _, err := StartRecorder(); err == nil {
		t.Fatal("a second recorder started")
	}
	for range 1000 { // some activity to record
		go func() {}()
	}
	path := filepath.Join(t.TempDir(), "slow.trace")
	snapshot(fr, path)
	snapshot(fr, path+".2") // guarded: a write is already running
	// Shutdown waits for busy to clear, so the file is complete.
	for deadline := time.Now().Add(5 * time.Second); busy.Load(); {
		if time.Now().After(deadline) {
			t.Fatal("snapshot never finished")
		}
		time.Sleep(10 * time.Millisecond)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("snapshot: %v size=%v", err, fi)
	}
}
