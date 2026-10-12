//go:build !cgo || purego

package crc

// sum is the pure Go fallback with the same signature. CI runs
// the tests twice: once as is, once with CGO_ENABLED=0.
func sum(b []byte) uint32 {
	var s uint32
	for _, c := range b {
		s = s*31 + uint32(c)
	}
	return s
}

const impl = "purego"
