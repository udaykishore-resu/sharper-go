// Command kubectl-ledger: put it on PATH and run
// `kubectl ledger status -n ledgerd`.
package main

import (
	"context"
	"os"
	"os/signal"
	"time"

	plugin "github.com/udaykishore-resu/sharper-go/techniques-thirdparty/096-kubectl-plugin"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := plugin.NewRoot().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
