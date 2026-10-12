package store

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strings"
	"testing"
)

// logPath is the printed errors.As / errors.AsType pair.
func logPath(err error) {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		log.Print(pe.Path)
	}
	// Go 1.26+: the same, without the variable
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		log.Print(pe.Path)
	}
}

func TestAsAndAsTypeAgree(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(os.Stderr); log.SetFlags(log.LstdFlags) })

	_, err := os.Open("/no/such/ledger.conf")
	logPath(fmt.Errorf("load config: %w", err))
	if got := strings.Count(buf.String(), "/no/such/ledger.conf\n"); got != 2 {
		t.Fatalf("want the path from As and from AsType, got %q", buf.String())
	}
	buf.Reset()
	logPath(errors.New("no path here"))
	if buf.Len() != 0 {
		t.Fatalf("matched a non-PathError: %q", buf.String())
	}
}
