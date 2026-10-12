//go:build cgo && !purego

package crc

/*
#cgo noescape fast_sum
#cgo nocallback fast_sum
#include <stddef.h>
#include <stdint.h>
static uint32_t fast_sum(const uint8_t *p, size_t n) {
    uint32_t s = 0;
    for (size_t i = 0; i < n; i++) s = s*31 + p[i];
    return s;
}
*/
import "C"
import "unsafe"

func sum(b []byte) uint32 {
	p := (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(b)))
	return uint32(C.fast_sum(p, C.size_t(len(b))))
}

const impl = "cgo"
