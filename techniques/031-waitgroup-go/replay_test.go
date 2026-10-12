package replay

import "testing"

func TestReplayAll(t *testing.T) {
	batch := make([]Transfer, 200)
	var c counter
	replayAll(batch, c.replay)
	if got := c.n.Load(); got != 200 {
		t.Fatalf("replayed %d, want 200: Wait returned early", got)
	}
}
