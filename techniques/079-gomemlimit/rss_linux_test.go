package memlimit

import (
	"os"
	"runtime/metrics"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func goTotal() int64 {
	s := []metrics.Sample{
		{Name: "/memory/classes/total:bytes"},
		{Name: "/memory/classes/heap/released:bytes"},
	}
	metrics.Read(s)
	return int64(s[0].Value.Uint64() - s[1].Value.Uint64())
}

func rss(t *testing.T) int64 {
	b, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		t.Skip("no /proc/self/statm")
	}
	f := strings.Fields(string(b))
	pages, _ := strconv.ParseInt(f[1], 10, 64)
	return pages * int64(os.Getpagesize())
}

// The soft limit tracks what the runtime manages, not the pod's
// RSS: 64 MiB from syscall.Mmap raises RSS and leaves the
// runtime's total (the number GOMEMLIMIT is compared with) flat.
func TestMmapIsOutsideTheLimit(t *testing.T) {
	const size = 64 << 20
	r0, g0 := rss(t), goTotal()
	m, err := syscall.Mmap(-1, 0, size,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		t.Skip(err)
	}
	defer syscall.Munmap(m)
	for i := 0; i < size; i += 4096 {
		m[i] = 1 // touch every page so it counts in RSS
	}
	r1, g1 := rss(t), goTotal()
	if r1-r0 < size*9/10 {
		t.Fatalf("RSS grew %d, want about %d", r1-r0, size)
	}
	if g1-g0 > size/4 {
		t.Fatalf("runtime total grew %d: mmap counted", g1-g0)
	}
	t.Logf("RSS +%d MiB, runtime total +%d MiB", (r1-r0)>>20, (g1-g0)>>20)
}
