// Package server is Technique 44: the four http.Server timeouts.
package server

import (
	"net/http"
	"time"
)

func newServer(h http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":8080",
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
}

// export streams for minutes: it extends its own deadline
// instead of bending the global WriteTimeout.
func export(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Now().Add(10 * time.Minute)); err != nil {
		http.Error(w, "cannot extend deadline", 500)
		return
	}
	w.Write([]byte("id,amount\n"))
}
