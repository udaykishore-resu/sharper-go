// Package config is Technique 91: reload live config without a
// restart, on a poll and on SIGHUP.
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

type Config struct {
	Version     string     `json:"version"`
	LogLevel    slog.Level `json:"log_level"`
	MaxTransfer int64      `json:"max_transfer_cents"`
	raw         []byte
}

// parse decodes and validates.
func parse(b []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.MaxTransfer <= 0 {
		return nil, errors.New("max_transfer_cents must be > 0")
	}
	c.raw = bytes.Clone(b)
	return &c, nil
}

type Reloader struct {
	Path  string
	Log   *slog.Logger
	Level *slog.LevelVar
	cur   atomic.Pointer[Config]
	fails atomic.Int64
}

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
	r.Log.Info("config applied", "version", c.Version)
	return nil
}

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
