// Package sum is Technique 16: constraints with ~ by default.
package sum

import "slices"

type Integer interface {
	~int | ~int32 | ~int64
}

func Sum[T Integer](xs []T) T {
	var total T
	for _, x := range xs {
		total += x
	}
	return total
}

func Positive[S ~[]E, E Integer](s S) S {
	return slices.DeleteFunc(s, func(x E) bool {
		return x <= 0
	})
}

// Cents is the named money type that broke int | int64.
type Cents int64

// Batch is a named slice that Positive must hand back as a Batch.
type Batch []Cents

// Set shows how comparable lets a panic through with any.
type Set[T comparable] map[T]struct{}

func (s Set[T]) Add(v T) { s[v] = struct{}{} }
