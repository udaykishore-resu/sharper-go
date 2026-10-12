package crc

import "testing"

func reference(b []byte) uint32 {
	var s uint32
	for _, c := range b {
		s = s*31 + uint32(c)
	}
	return s
}

func TestSum(t *testing.T) {
	for _, in := range []string{"", "a", "ledgerd", "transfer t_88 500"} {
		if got, want := Sum([]byte(in)), reference([]byte(in)); got != want {
			t.Errorf("%s Sum(%q) = %d, want %d", impl, in, got, want)
		}
	}
	t.Logf("implementation: %s", impl)
}

// Benchmark the call alone: each cgo call has a fixed overhead.
func BenchmarkSum(b *testing.B) {
	data := make([]byte, 64)
	for b.Loop() {
		Sum(data)
	}
}
