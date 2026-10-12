// Command ledgerctl: replay and verify, on the standard flag
// package (Technique 97).
package main

import (
	"context"
	"os"
	"os/signal"

	ledgerctl "github.com/udaykishore-resu/sharper-go/techniques/097-cli-flag"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := ledgerctl.Main(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}
