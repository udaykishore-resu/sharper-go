// Package config loads ledgerd's boot config from the
// environment (Technique 101) and its live config from a file
// it can reload without a restart (Techniques 39 and 91).
package config

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Config is boot config: changing it needs a restart.
type Config struct {
	DatabaseURL string
	LogLevel    slog.Level
	// Keys maps an API token to the client it identifies.
	Keys       map[string]string
	ReqTimeout time.Duration
	Grace      time.Duration
	MaxConns   int32
	// LiveConfig is an optional path to the reloadable file.
	LiveConfig string
	// Sink names the outbox event sink (Technique 99).
	Sink string
}

// Load reads every setting through getenv, so tests pass a map.
// Defaults come from cmp.Or (Technique 18), and every problem
// comes back at once through errors.Join (Technique 22).
func Load(getenv func(string) string) (Config, error) {
	var errs []error
	c := Config{
		DatabaseURL: getenv("DATABASE_URL"),
		LiveConfig:  getenv("LEDGERD_LIVE_CONFIG"),
		Sink:        cmp.Or(getenv("EVENT_SINK"), "log"),
	}
	if c.DatabaseURL == "" {
		// No default on purpose: never connect to a local
		// database by accident in production.
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	lvl := cmp.Or(getenv("LOG_LEVEL"), "info")
	if err := c.LogLevel.UnmarshalText([]byte(lvl)); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	var err error
	c.Keys, err = parseKeys(getenv("API_KEYS"))
	errs = append(errs, err)
	c.ReqTimeout, err = duration(getenv, "REQUEST_TIMEOUT", 10*time.Second)
	errs = append(errs, err)
	c.Grace, err = duration(getenv, "SHUTDOWN_GRACE", 20*time.Second)
	errs = append(errs, err)
	n, err := strconv.ParseInt(cmp.Or(getenv("DB_MAX_CONNS"), "12"), 10, 32)
	if err != nil || n < 1 {
		errs = append(errs, fmt.Errorf("DB_MAX_CONNS: want a positive int, got %q",
			getenv("DB_MAX_CONNS")))
	}
	c.MaxConns = int32(n)
	return c, errors.Join(errs...)
}

func duration(getenv func(string) string, key string,
	def time.Duration) (time.Duration, error) {
	s := getenv(key)
	if s == "" {
		return def, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def, fmt.Errorf("%s: want a positive duration, got %q", key, s)
	}
	return d, nil
}

// parseKeys reads "client1:token1,client2:token2".
func parseKeys(s string) (map[string]string, error) {
	keys := map[string]string{}
	if s == "" {
		return keys, nil
	}
	var errs []error
	for item := range strings.SplitSeq(s, ",") {
		client, token, ok := strings.Cut(strings.TrimSpace(item), ":")
		if !ok || client == "" || len(token) < 16 {
			errs = append(errs, fmt.Errorf(
				"API_KEYS: entry %q needs client:token, token >= 16 chars",
				client))
			continue
		}
		// ledger.ScopedKey joins client and key with "/", so a
		// client name holding one could share another client's
		// idempotency keys (Technique 52).
		if strings.Contains(client, "/") {
			errs = append(errs, fmt.Errorf(
				"API_KEYS: client %q must not contain '/'", client))
			continue
		}
		keys[token] = client
	}
	return keys, errors.Join(errs...)
}
