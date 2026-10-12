// Package clamp is Technique 15: min, max and clear.
package clamp

import "time"

const minPage, maxPage = 1, 500

func pageSize(asked int) int {
	return max(minPage, min(asked, maxPage))
}

func backoff(attempt int) time.Duration {
	d := 100 * time.Millisecond << min(attempt, 6)
	return min(d, 5*time.Second)
}

// unclamped is the bug from the first Breaks when: the shift
// wraps before min ever sees the value.
func unclamped(attempt int) time.Duration {
	return min(100*time.Millisecond<<attempt, 5*time.Second)
}
