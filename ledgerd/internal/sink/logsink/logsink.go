// Package logsink is the default event sink: it writes each
// outbox event as one structured log line. Swap it for a broker
// sink in production by adding a package and a blank import.
package logsink

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"example.com/ledgerd/internal/sink"
)

func init() {
	sink.Register("log", func(cfg map[string]string) (sink.Sink, error) {
		for k := range cfg {
			return nil, fmt.Errorf("sink log: unknown setting %q", k)
		}
		return logSink{}, nil
	})
}

type logSink struct{}

func (logSink) Publish(ctx context.Context, topic string, payload []byte) error {
	slog.InfoContext(ctx, "event", "topic", topic,
		"payload", json.RawMessage(payload))
	return nil
}
