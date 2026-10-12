// Package bloop is Technique 70: benchmarks with b.Loop. Run with
//
//	go test -run '^$' -bench . -benchmem
package bloop

import (
	"testing"

	amount "github.com/udaykishore-resu/sharper-go/techniques/067-native-fuzzing"
)

// Setup above the loop is not timed, the result needs no sink,
// and the compiler cannot delete the call inside the braces.
func BenchmarkParseAmount(b *testing.B) {
	inputs := []string{"0.01", "12.34", "99999.99"}
	for b.Loop() {
		for _, s := range inputs {
			amount.ParseAmount(s)
		}
	}
}

// To see whether a function is linear, wrap sizes in b.Run and
// call b.Loop inside each.
func BenchmarkFormatSizes(b *testing.B) {
	for _, n := range []int64{1, 1_000_00, 1_000_000_000_00} {
		b.Run(amount.FormatAmount(n), func(b *testing.B) {
			for b.Loop() {
				amount.FormatAmount(n)
			}
		})
	}
}
