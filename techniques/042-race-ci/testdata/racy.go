//go:build ignore

// racy is a counter with its mutex deleted: the Try it of
// Technique 42.
package main

import (
	"fmt"
	"sync"
)

func main() {
	m := map[string]int64{}
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() { m[fmt.Sprint("acc_", i%5)]++ })
	}
	wg.Wait()
	fmt.Println(len(m))
}
