// A stand-in ledgerd for Technique 74: a CPU-bound hot path behind
// an interface, and the loopback pprof server from Technique 73.
package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"

	"example.com/ledgerd/internal/ledger"
)

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	go http.Serve(ln, mux)
	fmt.Println(ln.Addr())
	var s ledger.Store = ledger.MemStore{}
	for {
		ledger.Work(s)
	}
}
