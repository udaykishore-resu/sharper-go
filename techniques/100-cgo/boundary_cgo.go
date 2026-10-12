//go:build cgo && !purego

package crc

// Helpers for the boundary tests (Technique 100's Breaks when):
// a C call that blocks, and a C struct that holds a Go pointer.
// Neither is exported; no C type leaves the package.

/*
#include <stddef.h>
#include <unistd.h>
typedef struct { const char *name; } holder;
static size_t name_len(holder *h) {
    size_t n = 0;
    while (h->name[n]) n++;
    return n;
}
static void block_ms(int ms) { usleep(ms * 1000); }
*/
import "C"

import (
	"runtime"
	"unsafe"
)

// blockInC holds its goroutine's OS thread for ms milliseconds.
func blockInC(ms int) { C.block_ms(C.int(ms)) }

// nameLen hands C a Go struct that holds a pointer to Go memory.
// The cgo pointer check panics unless that inner pointer is
// pinned for the call.
func nameLen(name string, pin bool) int {
	b := append([]byte(name), 0)
	h := &C.holder{name: (*C.char)(unsafe.Pointer(&b[0]))}
	if pin {
		var p runtime.Pinner
		p.Pin(&b[0])
		defer p.Unpin()
	}
	return int(C.name_len(h))
}
