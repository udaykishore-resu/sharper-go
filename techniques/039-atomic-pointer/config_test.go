package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "limits.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReload(t *testing.T) {
	var h Holder
	if h.Get() != nil {
		t.Fatal("a zero atomic.Pointer holds nil until the first Store")
	}
	if err := h.Reload(write(t, `{"max_transfer":500,"frozen":{"a1":true}}`)); err != nil {
		t.Fatal(err)
	}
	if err := h.Reload(write(t, `{"max_transfer":0}`)); err == nil {
		t.Fatal("bad file accepted")
	}
	if c := h.Get(); c.MaxTransfer != 500 || !c.Frozen["a1"] {
		t.Fatalf("old config not kept: %+v", c)
	}
}

// Run with -cpu=1,4,16 and compare how each scales.
func BenchmarkRWMutexGet(b *testing.B) {
	h := &rwHolder{c: &Config{MaxTransfer: 1}}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = h.Get().MaxTransfer
		}
	})
}

func BenchmarkAtomicGet(b *testing.B) {
	var h Holder
	h.p.Store(&Config{MaxTransfer: 1})
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = h.Get().MaxTransfer
		}
	})
}
