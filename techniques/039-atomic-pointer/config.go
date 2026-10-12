// Package config is Technique 39: atomic.Pointer for config
// that many goroutines read and one goroutine swaps.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"sync/atomic"
)

// Config is read-only once loaded; reload builds a new one.
type Config struct {
	MaxTransfer int64           `json:"max_transfer"`
	Frozen      map[string]bool `json:"frozen"`
}

type Holder struct{ p atomic.Pointer[Config] }

func (h *Holder) Get() *Config { return h.p.Load() }

func (h *Holder) Reload(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return err
	}
	if c.MaxTransfer <= 0 {
		return errors.New("max_transfer must be > 0")
	}
	h.p.Store(&c)
	return nil
}

// rwHolder is the RWMutex version, for the benchmark.
type rwHolder struct {
	mu sync.RWMutex
	c  *Config
}

func (h *rwHolder) Get() *Config {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.c
}
