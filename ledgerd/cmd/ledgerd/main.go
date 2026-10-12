// Command ledgerd is a small double-entry ledger API.
//
//	ledgerd [serve]   run the API (the default)
//	ledgerd migrate   apply embedded migrations and exit
//
// main only wires: run returns an error, main prints it with the
// build version and sets the exit code, so every deferred call in
// run executes first (Technique 101).
package main

import (
	"fmt"
	"os"

	"example.com/ledgerd/internal/obs"
	_ "example.com/ledgerd/internal/sink/logsink"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr,
			"ledgerd", obs.Version+":", err)
		os.Exit(1)
	}
}
