// Package config is Technique 39: atomic.Pointer for config
// that many goroutines read and one goroutine swaps.
package config

import (
	"bytes"
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
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields() // a typo is an error
	if err := d.Decode(&c); err != nil {
		return err
	}
	if err := c.validate(); err != nil {
		return err
	}
	h.p.Store(&c)
	return nil
}

// validate reports every broken rule at once. A missing
// max_transfer decodes to 0 and fails here; a missing frozen
// decodes to nil and fails too, so "none frozen" must be {}.
func (c *Config) validate() error {
	var errs []error
	if c.MaxTransfer <= 0 {
		errs = append(errs, errors.New("max_transfer must be > 0"))
	}
	if c.Frozen == nil {
		errs = append(errs, errors.New("frozen is required; use {} for none"))
	}
	if _, ok := c.Frozen[""]; ok {
		errs = append(errs, errors.New("frozen has an empty account ID"))
	}
	return errors.Join(errs...)
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
