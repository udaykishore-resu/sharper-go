package buildinfo

import (
	"runtime"
	"testing"
)

func TestBuildInfo(t *testing.T) {
	b := BuildInfo()
	if b.Go != runtime.Version() {
		t.Fatalf("Go = %q, want %q", b.Go, runtime.Version())
	}
	// A test binary's main module has no tag-based version,
	// so the stamp falls back to "(devel)" or stays "dev".
	if b.Version == "" {
		t.Fatal("empty version")
	}
	if BuildInfo() != b {
		t.Fatal("OnceValue returned a different value")
	}
}
