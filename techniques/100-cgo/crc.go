// Package crc is Technique 100: one package uses cgo, behind
// build tags, with a pure Go fallback. No C type leaves it.
package crc

// Sum is the only exported function.
func Sum(b []byte) uint32 { return sum(b) }
