// Package ledgerctl is Technique 97: a two-command CLI on the
// standard flag package.
package ledgerctl

import (
	"cmp"
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

// Main is what cmd/ledgerctl calls. It exits 0 on success and on
// -h (flag.ErrHelp), and 1 on any other error, after one line on
// stderr.
func Main(ctx context.Context, args []string) int {
	err := dispatch(ctx, args)
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return 0
	}
	fmt.Fprintln(os.Stderr, "ledgerctl:", err)
	return 1
}

// lastReplay records what replay parsed, for the tests.
var lastReplay struct {
	file   string
	dryRun bool
	token  string
}

func replay(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	dry := fs.Bool("dry-run", true, "print, don't post")
	// A secret never becomes a flag default: -h prints defaults.
	// Read the environment after parsing instead.
	tok := fs.String("token", "",
		"API token (default: $LEDGERCTL_TOKEN)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		// flag stops at the first non-flag: show the leftovers.
		return fmt.Errorf("replay: want one file, got %q", fs.Args())
	}
	lastReplay.file, lastReplay.dryRun = fs.Arg(0), *dry
	lastReplay.token = cmp.Or(*tok, os.Getenv("LEDGERCTL_TOKEN"))
	if lastReplay.token == "" {
		return errors.New("replay: no token: set -token or LEDGERCTL_TOKEN")
	}
	return ctx.Err()
}

func verify(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	return fs.Parse(args)
}
