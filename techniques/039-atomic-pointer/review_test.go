package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const good = `{"max_transfer":500,"frozen":{"a1":true}}`

// Every bad file is rejected and the previous config stays live.
func TestBadReloadKeepsOld(t *testing.T) {
	cases := map[string]string{
		"malformed":       `{"max_transfer":500,`,
		"missing limit":   `{"frozen":{}}`,
		"missing frozen":  `{"max_transfer":500}`,
		"zero limit":      `{"max_transfer":0,"frozen":{}}`,
		"negative limit":  `{"max_transfer":-5,"frozen":{}}`,
		"unknown field":   `{"max_transfer":500,"frozn":{"a1":true}}`,
		"empty frozen id": `{"max_transfer":500,"frozen":{"":true}}`,
		"wrong type":      `{"max_transfer":"500","frozen":{}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var h Holder
			if err := h.Reload(write(t, good)); err != nil {
				t.Fatal(err)
			}
			old := h.Get()
			if err := h.Reload(write(t, body)); err == nil {
				t.Fatalf("accepted %s", body)
			}
			if h.Get() != old {
				t.Fatal("a rejected file replaced the live config")
			}
		})
	}
	var h Holder
	if err := h.Reload(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("missing file accepted")
	}
}

// validate reports every rule at once.
func TestValidateJoinsErrors(t *testing.T) {
	c := Config{Frozen: map[string]bool{"": true}}
	err := c.validate()
	for _, want := range []string{"max_transfer", "empty account"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to mention %q", err, want)
		}
	}
}

// Readers on many goroutines while one writer reloads good and bad
// files: under -race nothing races, and every snapshot a reader
// sees is a whole, valid config.
func TestConcurrentReadersDuringReload(t *testing.T) {
	var h Holder
	if err := h.Reload(write(t, good)); err != nil {
		t.Fatal(err)
	}
	files := make([]string, 20)
	for i := range files {
		body := fmt.Sprintf(`{"max_transfer":%d,"frozen":{"a%d":true}}`, 100+i, i)
		if i%3 == 0 {
			body = `{"max_transfer":0,"frozen":{}}`
		}
		files[i] = write(t, body)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				c := h.Get()
				if c.MaxTransfer <= 0 || len(c.Frozen) != 1 {
					t.Errorf("reader saw %+v", c)
					return
				}
			}
		})
	}
	for range 50 {
		for _, f := range files {
			_ = h.Reload(f)
		}
	}
	close(stop)
	wg.Wait()
}
