// Command svc runs intcover.Handler until SIGTERM. main returns
// normally on SIGTERM, which is what lets a -cover build write its
// counters to GOCOVERDIR (Technique 72, Technique 45).
package main

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	intcover "github.com/udaykishore-resu/sharper-go/techniques/072-integration-cover"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "svc:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	// SVC_NO_SIGNALS=1 is a test seam: it reproduces a service that
	// never catches SIGTERM, so the default action kills it.
	if os.Getenv("SVC_NO_SIGNALS") != "1" {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
		defer stop()
	}
	srv := &http.Server{
		Addr:              cmp.Or(os.Getenv("SVC_ADDR"), "127.0.0.1:8072"),
		Handler:           intcover.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(sctx) // then main returns: counters flush
}
