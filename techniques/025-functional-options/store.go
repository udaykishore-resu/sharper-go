// Package store is Technique 25: functional options, kept small.
package store

import (
	"errors"
	"time"
)

type Store struct {
	dsn     string
	maxConn int
	timeout time.Duration
}

type Option func(*Store)

func WithMaxConns(n int) Option {
	return func(s *Store) { s.maxConn = n }
}

// WithTimeout has the same shape as WithMaxConns.
func WithTimeout(d time.Duration) Option {
	return func(s *Store) { s.timeout = d }
}

func New(dsn string, opts ...Option) (*Store, error) {
	s := &Store{dsn: dsn, maxConn: 10,
		timeout: 5 * time.Second}
	for _, opt := range opts {
		opt(s) // in order: the last one wins
	}
	if s.maxConn < 1 || s.timeout <= 0 {
		return nil, errors.New(
			"store: maxConn < 1 or timeout <= 0")
	}
	return s, nil
}
