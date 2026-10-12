package obs

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogValueHidesKey(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "v1.4.2")
	log.Info("transfer posted", slog.Any("transfer",
		Transfer{ID: "t_9f2", Amount: 500, Key: "secret-key"}))
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	tr := line["transfer"].(map[string]any)
	if tr["id"] != "t_9f2" || strings.Contains(buf.String(), "secret-key") {
		t.Fatalf("line = %s", &buf)
	}
}

func TestLevelVar(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "v1")
	log.Debug("hidden")
	Level.Set(slog.LevelDebug)
	defer Level.Set(slog.LevelInfo)
	log.Debug("shown")
	if strings.Contains(buf.String(), "hidden") || !strings.Contains(buf.String(), "shown") {
		t.Fatalf("got %s", &buf)
	}
}

// The JSON handler writes a Duration as nanoseconds: log dur_ms.
func TestDurationIsNanoseconds(t *testing.T) {
	var buf bytes.Buffer
	NewLogger(&buf, "v1").Info("req", "dur", 2*time.Millisecond,
		"dur_ms", (2 * time.Millisecond).Milliseconds())
	if !strings.Contains(buf.String(), `"dur":2000000`) ||
		!strings.Contains(buf.String(), `"dur_ms":2`) {
		t.Fatalf("got %s", &buf)
	}
}

// An odd number of arguments gives !BADKEY (go vet flags it too).
func TestBadKey(t *testing.T) {
	var buf bytes.Buffer
	args := []any{"acct"} // via a slice so vet lets the test build
	NewLogger(&buf, "v1").Info("odd", args...)
	if !strings.Contains(buf.String(), "!BADKEY") {
		t.Fatalf("got %s", &buf)
	}
}
