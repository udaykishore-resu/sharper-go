// Package fx is Technique 66's subject: an HTTP client for a
// rates service, built to take an injected *http.Client.
package fx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
	// Backoff between the three attempts on a 503.
	Backoff []time.Duration
}

// Rate returns parts per million of from in to.
func (c *Client) Rate(ctx context.Context, from, to string) (int64, error) {
	var lastErr error
	for attempt := range 3 {
		ppm, retry, err := c.once(ctx, from, to)
		if err == nil || !retry {
			return ppm, err
		}
		lastErr = err
		if attempt < 2 && attempt < len(c.Backoff) {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(c.Backoff[attempt]):
			}
		}
	}
	return 0, lastErr
}

func (c *Client) once(ctx context.Context, from, to string) (int64, bool, error) {
	u := c.BaseURL + "/rates?" + url.Values{"from": {from}, "to": {to}}.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return 0, false, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		return 0, true, fmt.Errorf("fx: %s", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("fx: %s", resp.Status)
	}
	var body struct {
		PPM int64 `json:"ppm"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, false, fmt.Errorf("fx: bad body: %w", err)
	}
	return body.PPM, false, nil
}
