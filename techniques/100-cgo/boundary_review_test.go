//go:build cgo && !purego && linux

package crc

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Review checks for Technique 100's Breaks when, on the cgo build.

// A Go struct holding an unpinned Go pointer, passed to C, panics
// at the call; runtime.Pinner makes the same call legal.
func TestUnpinnedGoPointerPanics(t *testing.T) {
	got := func() (msg string) {
		defer func() { msg = fmt.Sprint(recover()) }()
		nameLen("ledgerd", false)
		return ""
	}()
	if !strings.Contains(got, "Go pointer to unpinned Go pointer") {
		t.Fatalf("panic = %q", got)
	}
	if n := nameLen("ledgerd", true); n != 7 {
		t.Fatalf("pinned call = %d", n)
	}
}

// Every goroutine blocked in C holds an OS thread, so unbounded
// blocking calls need one thread each; a semaphore caps them.
// The Go runtime keeps idle threads, so each burst runs in a
// fresh process (this test binary, re-run) and reads the thread
// count from /proc mid-burst.
func TestBlockedCallsHoldThreads(t *testing.T) {
	if lim := os.Getenv("CRC_BURST_LIMIT"); lim != "" {
		n, _ := strconv.Atoi(lim)
		fmt.Printf("growth=%d\n", burst(t, n))
		return
	}
	growth := func(limit int) int {
		cmd := exec.Command(os.Args[0], "-test.run=^TestBlockedCallsHoldThreads$",
			"-test.count=1")
		cmd.Env = append(os.Environ(), fmt.Sprintf("CRC_BURST_LIMIT=%d", limit))
		out, err := cmd.CombinedOutput()
		_, v, ok := strings.Cut(string(out), "growth=")
		if err != nil || !ok {
			t.Fatalf("child: %v\n%s", err, out)
		}
		n, _ := strconv.Atoi(strings.Fields(v)[0])
		return n
	}
	bounded, unbounded := growth(4), growth(32)
	t.Logf("extra threads mid-burst: %d with a semaphore of 4, %d without",
		bounded, unbounded)
	if unbounded < 28 || bounded > 8 {
		t.Fatal("want one thread per blocked call, and the semaphore to cap them")
	}
}

// burst blocks 32 goroutines in C, at most limit at a time, and
// returns how many threads the process gained mid-burst.
func burst(t *testing.T, limit int) int {
	threads := func() int {
		b, err := os.ReadFile("/proc/self/status")
		if err != nil {
			t.Fatal(err)
		}
		_, v, _ := strings.Cut(string(b), "Threads:")
		n, _ := strconv.Atoi(strings.Fields(v)[0])
		return n
	}
	base := threads()
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			sem <- struct{}{}
			blockInC(200)
			<-sem
		})
	}
	time.Sleep(100 * time.Millisecond) // mid-burst
	n := threads()
	wg.Wait()
	return n - base
}

func TestSumNilSlice(t *testing.T) {
	if Sum(nil) != 0 {
		t.Fatal("Sum(nil) != 0")
	}
}
