// Package config is Technique 18: cmp.Or for defaults, in
// precedence order.
package config

import (
	"cmp"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Addr    string
	Timeout time.Duration
	Page    int
	Retries int
}

func load(flagAddr string, file Config) Config {
	return Config{
		Addr: cmp.Or(
			flagAddr,
			os.Getenv("LEDGERD_ADDR"),
			file.Addr,
			":8080",
		),
		Timeout: cmp.Or(file.Timeout, 5*time.Second),
		Page:    cmp.Or(file.Page, 100),
		// The trap: zero retries means "no retries".
		Retries: cmp.Or(file.Retries, 3),
	}
}

// closeBoth returns the first non-nil error; both steps run.
func closeBoth(a, b func() error) error {
	return cmp.Or(a(), b())
}

// envDuration reads a duration from the environment. Unset gives
// zero, so cmp.Or falls through to the next source; a set value
// that does not parse is an error, never a silent fallback.
func envDuration(name string) (time.Duration, error) {
	v := os.Getenv(name)
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: %w", name, v, err)
	}
	return d, nil
}
