package export

import (
	"strings"
	"testing"
)

func TestLineZeroAllocs(t *testing.T) {
	if raceOn {
		t.Skip("allocation counts are not meaningful under -race")
	}
	var c CSVWriter
	tr := Transfer{"t_1", "acc_1", "acc_2", 12345}
	if got := string(c.Line(tr)); got != "t_1,acc_1,acc_2,12345\n" {
		t.Fatalf("got %q", got)
	}
	if n := testing.AllocsPerRun(100, func() { c.Line(tr) }); n != 0 {
		t.Fatalf("Line: %v allocs after warm-up, want 0", n)
	}
}

// Bytes() is the buffer's own memory: the next Line overwrites it.
func TestCallerMustCopy(t *testing.T) {
	var c CSVWriter
	first := c.Line(Transfer{"t_1", "a", "b", 1})
	c.Line(Transfer{"t_2", "c", "d", 2})
	if string(first) != "t_2,c,d,2\n" {
		t.Fatalf("expected the old slice to see the new line, got %q", first)
	}
}

// A Builder's Reset throws the memory away, so reuse allocates.
func TestBuilderResetDropsMemory(t *testing.T) {
	if raceOn {
		t.Skip("allocation counts are not meaningful under -race")
	}
	var b strings.Builder
	n := testing.AllocsPerRun(100, func() {
		b.Reset()
		b.WriteString("t_1,acc_1,acc_2,12345\n")
	})
	if n < 1 {
		t.Fatalf("Builder reuse: %v allocs", n)
	}
}

// Copying a Builder is fine; writing to the copy panics.
func TestBuilderCopyPanicsOnWrite(t *testing.T) {
	var b strings.Builder
	b.WriteString("x")
	c := b
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(r.(string), "copied by value") {
			t.Fatalf("recover() = %v", r)
		}
	}()
	c.WriteString("y")
}
