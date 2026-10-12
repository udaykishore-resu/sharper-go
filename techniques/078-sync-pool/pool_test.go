package respond

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

type discard struct{ h http.Header }

func (d discard) Header() http.Header       { return d.h }
func (discard) Write(b []byte) (int, error) { return io.Discard.Write(b) }
func (discard) WriteHeader(int)             {}

var body = map[string]any{"id": "t_1", "amount": 500}

func TestOversizedNotPooled(t *testing.T) {
	w := discard{http.Header{}}
	big := strings.Repeat("x", 1<<20)
	if err := writeJSON(w, big); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if buf := bufPool.Get().(*bytes.Buffer); buf.Cap() > maxPooled {
			t.Fatalf("pool holds a %d-byte buffer", buf.Cap())
		}
	}
}

// Measure under parallel load and compare with benchstat; a
// single-threaded benchmark hides a pool's value.
func BenchmarkWriteJSONPooled(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		w := discard{http.Header{}}
		for pb.Next() {
			writeJSON(w, body)
		}
	})
}

func BenchmarkWriteJSONPlain(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		w := discard{http.Header{}}
		for pb.Next() {
			writeJSONPlain(w, body)
		}
	})
}
