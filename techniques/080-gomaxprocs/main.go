// Command gomaxprocs is Technique 80. A module whose go line is
// older than 1.25 opts in to container-aware GOMAXPROCS with
// these two settings; a go 1.25+ line, like this one, has them
// on by default. Setting GOMAXPROCS yourself turns both off.
//
//go:debug containermaxprocs=1
//go:debug updatemaxprocs=1
package main

import (
	"fmt"
	"runtime"
	"runtime/metrics"
)

func gomaxprocsMetric() uint64 {
	s := []metrics.Sample{{Name: "/sched/gomaxprocs:threads"}}
	metrics.Read(s)
	return s[0].Value.Uint64()
}

func main() {
	fmt.Println("GOMAXPROCS", runtime.GOMAXPROCS(0),
		"metric", gomaxprocsMetric(), "NumCPU", runtime.NumCPU())
}
