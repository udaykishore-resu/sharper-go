package report

import (
	"testing"
	"unsafe"
)

// Lock the size in: this fails if someone reorders the fields.
func TestSizes(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("sizes are for 64-bit platforms")
	}
	if got := unsafe.Sizeof(Posting{}); got != 48 {
		t.Errorf("Posting = %d bytes, want 48", got)
	}
	if got := unsafe.Sizeof(PostingPacked{}); got != 40 {
		t.Errorf("PostingPacked = %d bytes, want 40", got)
	}
	// The saving is real where structs sit side by side.
	if a, b := unsafe.Sizeof([1000]Posting{}), unsafe.Sizeof([1000]PostingPacked{}); a-b != 8000 {
		t.Errorf("slice of 1000 saves %d bytes", a-b)
	}
}
