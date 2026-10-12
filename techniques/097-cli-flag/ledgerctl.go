// Package ledgerctl is Technique 97: a two-command CLI on the
// standard flag package.
package ledgerctl

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

type cmd struct {
	name, help string
	run        func(context.Context, []string) error
}

var cmds = []cmd{
	{"replay", "re-post transfers from a file", replay},
	{"verify", "check balances sum to zero", verify},
}

func dispatch(ctx context.Context, args []string) error {
	for _, c := range cmds {
		if len(args) > 0 && c.name == args[0] {
			return c.run(ctx, args[1:])
		}
	}
	usage(os.Stderr)
	return errors.New("missing or unknown command")
}

// usage loops over the same table, so help can't drift.
func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: ledgerctl <command> [flags]")
	for _, c := range cmds {
		fmt.Fprintf(w, "  %-8s %s\n", c.name, c.help)
	}
}

// lastReplay records what replay parsed, for the tests.
var lastReplay struct {
	file   string
	dryRun bool
}

func replay(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dry := fs.Bool("dry-run", true, "print, don't post")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		// flag stops at the first non-flag: show the leftovers.
		return fmt.Errorf("replay: want one file, got %q", fs.Args())
	}
	lastReplay.file, lastReplay.dryRun = fs.Arg(0), *dry
	return ctx.Err()
}

func verify(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs.Parse(args)
}
