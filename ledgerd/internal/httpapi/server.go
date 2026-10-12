package httpapi

import (
	"net/http"
	"time"
)

// NewServer sets all four timeouts (Technique 44). The
// shutdown grace (20s by default) covers WriteTimeout, and the
// idempotency lease (30s) outlasts both.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
}
