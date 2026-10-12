// Package sink is Technique 99: a registry of compiled-in
// backends, the way database/sql drivers work.
package sink

import (
	"context"
	"fmt"
	"maps"
	"slices"
)

type Sink interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

type Factory func(cfg map[string]string) (Sink, error)

// Written only from init, which runs on one goroutine.
var factories = map[string]Factory{}

func Register(name string, f Factory) {
	if _, dup := factories[name]; dup {
		panic("sink: duplicate " + name)
	}
	factories[name] = f
}

func New(name string, cfg map[string]string) (Sink, error) {
	f, ok := factories[name]
	if !ok {
		have := slices.Sorted(maps.Keys(factories))
		return nil, fmt.Errorf(
			"sink %q not built in (have %v)", name, have)
	}
	return f(cfg)
}
