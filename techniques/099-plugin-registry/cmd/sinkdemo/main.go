// Command sinkdemo picks a sink by name, as cmd/ledgerd does.
// Which sinks exist is decided by the blank imports in the
// sinks_*.go files, each behind its own build tag.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/udaykishore-resu/sharper-go/techniques/099-plugin-registry/sink"
)

func main() {
	s, err := sink.New(os.Getenv("SINK"), nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	s.Publish(context.Background(), "transfer.created", []byte(`{}`))
}
