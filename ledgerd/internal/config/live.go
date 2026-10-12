package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

// Live is config that reloads without a restart. A loaded
// *Live is read-only; reload builds a new one (Technique 39).
type Live struct {
	Version     string     `json:"version"`
	LogLevel    slog.Level `json:"log_level"`
	MaxTransfer int64      `json:"max_transfer_cents"`
	raw         []byte
}

func parse(b []byte) (*Live, error) {
	var c Live
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, err
	}
	var errs []error
	if c.Version == "" {
		errs = append(errs, errors.New("version is required"))
	}
	if c.MaxTransfer <= 0 {
		errs = append(errs, errors.New("max_transfer_cents must be > 0"))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	c.raw = bytes.Clone(b)
	return &c, nil
}

// Reloader holds the live config behind an atomic pointer
// (Technique 39) and reloads it from Path (Technique 91).
type Reloader struct {
	Path  string
	Log   *slog.Logger
	Level *slog.LevelVar
	// OnApply runs after each successful swap, e.g. to publish
	// the version through expvar (Technique 89).
	OnApply func(*Live)

	cur   atomic.Pointer[Live]
	fails atomic.Int64
}

// Get returns the current snapshot, or nil before the first
// successful Reload. Load it once per request.
func (r *Reloader) Get() *Live { return r.cur.Load() }

// MaxTransfer is the current limit, or 0 for none.
func (r *Reloader) MaxTransfer() int64 {
	if c := r.Get(); c != nil {
		return c.MaxTransfer
	}
	return 0
}

// Fails counts rejected reloads, for a metric.
func (r *Reloader) Fails() int64 { return r.fails.Load() }

// Reload reads, validates and swaps in the file. A bad file is
// rejected whole and readers keep the old config.
func (r *Reloader) Reload() error {
	b, err := os.ReadFile(r.Path)
	if err != nil {
		return err
	}
	if old := r.cur.Load(); old != nil &&
		bytes.Equal(old.raw, b) {
		return nil // unchanged
	}
	c, err := parse(b) // decode + validate
	if err != nil {
		r.fails.Add(1)
		return fmt.Errorf("reload %s: %w", r.Path, err)
	}
	r.cur.Store(c)
	r.Level.Set(c.LogLevel)
	if r.OnApply != nil {
		r.OnApply(c)
	}
	r.Log.Info("config applied", "version", c.Version)
	return nil
}

// Watch polls every interval and reloads at once on SIGHUP.
func (r *Reloader) Watch(ctx context.Context,
	every time.Duration) {
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-hup:
		case <-t.C:
		}
		if err := r.Reload(); err != nil {
			r.Log.Error("config rejected", "err", err)
		}
	}
}
