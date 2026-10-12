// Package page is where Page[T] lives after the move.
package page

// Page is one page of results and the cursor for the next.
type Page[T any] struct {
	Items []T
	Next  string
}
