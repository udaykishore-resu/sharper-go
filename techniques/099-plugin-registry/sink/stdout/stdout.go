// Package stdout registers the "stdout" sink from its init.
// Register only a cheap constructor here: connect in New.
package stdout

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/udaykishore-resu/sharper-go/techniques/099-plugin-registry/sink"
)

func init() {
	sink.Register("stdout", func(cfg map[string]string) (sink.Sink, error) {
		for k := range cfg {
			if k != "prefix" {
				return nil, fmt.Errorf("sink stdout: unknown setting %q", k)
			}
		}
		return &out{w: os.Stdout, prefix: cfg["prefix"]}, nil
	})
}

type out struct {
	w      io.Writer
	prefix string
}

func (o *out) Publish(_ context.Context, topic string, p []byte) error {
	_, err := fmt.Fprintf(o.w, "%s%s %s\n", o.prefix, topic, p)
	return err
}
