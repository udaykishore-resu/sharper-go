package sink_test

import (
	"strings"
	"testing"

	"example.com/ledgerd/internal/sink"
	_ "example.com/ledgerd/internal/sink/logsink"
)

func TestBuiltIn(t *testing.T) {
	if _, err := sink.New("log", nil); err != nil {
		t.Fatal(err)
	}
	_, err := sink.New("kafka", nil)
	if err == nil || !strings.Contains(err.Error(), "have [log]") {
		t.Fatalf("missing sink error = %v", err)
	}
	if _, err := sink.New("log", map[string]string{"ulr": "x"}); err == nil {
		t.Fatal("unknown setting accepted")
	}
}
