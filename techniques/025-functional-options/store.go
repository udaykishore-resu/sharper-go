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

func New(dsn string, opts ...Option) (*Store, error) {
	s := &Store{dsn: dsn, maxConn: 10,
		timeout: 5 * time.Second}
	for _, opt := range opts {
		opt(s)
	}
	if s.maxConn < 1 {
		return nil, errors.New("store: maxConn < 1")
	}
	return s, nil
}
