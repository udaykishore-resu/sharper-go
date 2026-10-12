// Package debug is Technique 73: pprof on its own mux, on a
// loopback port the load balancer never sees. Importing
// net/http/pprof, blank or not, registers its handlers on
// http.DefaultServeMux, so no server may serve the default mux.
package debug

import (
	"log"
	"net/http"
	"net/http/pprof"
	"time"
)

func debugMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile",
		pprof.Profile)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}

func serveDebug() *http.Server {
	srv := &http.Server{
		Addr:              "127.0.0.1:6060",
		Handler:           debugMux(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { log.Println(srv.ListenAndServe()) }()
	return srv
}
