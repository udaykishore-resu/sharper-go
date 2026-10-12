// Package sink is the registry of outbox event sinks
// (Technique 99). Each backend lives in its own package and
// registers itself from init; cmd/ledgerd picks what ships with
// blank imports.
package sink

import (
	"context"
	"fmt"
	"maps"
	"slices"
)

// Sink publishes one event.
type Sink interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// Factory builds a sink from its settings. It must reject
// unknown keys, so a typo fails at startup.
type Factory func(cfg map[string]string) (Sink, error)

// Written only from init, which runs on one goroutine.
var factories = map[string]Factory{}

// Register adds a backend. A duplicate name panics on purpose:
// silent last-one-wins would depend on init order.
func Register(name string, f Factory) {
	if _, dup := factories[name]; dup {
		panic("sink: duplicate " + name)
	}
	factories[name] = f
}

// New builds the named sink, or lists what was built in.
func New(name string, cfg map[string]string) (Sink, error) {
	f, ok := factories[name]
	if !ok {
		have := slices.Sorted(maps.Keys(factories))
		return nil, fmt.Errorf(
			"sink %q not built in (have %v)", name, have)
	}
	return f(cfg)
}
