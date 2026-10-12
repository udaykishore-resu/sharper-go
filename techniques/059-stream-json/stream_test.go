package stream

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
)

// gen writes a JSON array of n fake transfers without ever
// holding it: test with a generator, not a fixture file.
func gen(n int) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte("["))
		for i := range n {
			if i > 0 {
				pw.Write([]byte(","))
			}
			fmt.Fprintf(pw, `{"id":"t_%d","from":"a","to":"b","amount":%d}`, i, i%1000+1)
		}
		pw.Write([]byte("]"))
		pw.Close()
	}()
	return pr
}

func TestStreamsInFlatMemory(t *testing.T) {
	var sum int64
	var peak uint64
	var ms runtime.MemStats
	n, err := EachTransfer(gen(50_000), func(tr Transfer) error {
		sum += tr.Amount
		if tr.Amount == 1 { // sample now and then
			runtime.ReadMemStats(&ms)
			peak = max(peak, ms.HeapInuse)
		}
		return nil
	})
	if err != nil || n != 50_000 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if peak > 64<<20 {
		t.Fatalf("heap in use peaked at %d MiB", peak>>20)
	}
}

func TestBadItemNamesIndex(t *testing.T) {
	_, err := EachTransfer(strings.NewReader(`[{"amount":1},{"amount":"x"}]`),
		func(Transfer) error { return nil })
	if err == nil || !strings.HasPrefix(err.Error(), "item 1:") {
		t.Fatalf("err = %v", err)
	}
}

func BenchmarkEachTransfer(b *testing.B) {
	for b.Loop() {
		EachTransfer(gen(10_000), func(Transfer) error { return nil })
	}
}
