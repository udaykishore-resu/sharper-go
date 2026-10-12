package httpapi

import (
	"net/http"
	"time"
)

// NewServer sets all four timeouts (Technique 44). WriteTimeout
// cuts a connection but never stops a handler; the Deadline
// middleware (REQUEST_TIMEOUT, 10s) bounds the work. The shutdown
// grace (20s) covers the request deadline, and the idempotency
// lease (store.Lease, 30s) outlasts both and the 25s
// ClaimBudget (TestIdemClaimBudgetInsideLease).
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
