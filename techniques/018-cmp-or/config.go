// Package config is Technique 18: cmp.Or for defaults, in
// precedence order.
package config

import (
	"cmp"
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
