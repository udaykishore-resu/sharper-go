package validate

import (
	"errors"
	"strings"
	"testing"
)

func TestTwoRulesAtOnce(t *testing.T) {
	err := Transfer{From: "a", To: "a", Amount: -5}.Validate()
	for _, want := range []error{ErrSameAccount, ErrBadAmount, ErrInvalid} {
		if !errors.Is(err, want) {
			t.Errorf("errors.Is(err, %v) = false", want)
		}
	}
	if strings.Count(err.Error(), "\n") != 1 {
		t.Errorf("want one message per line, got %q", err)
	}
	if errors.Unwrap(err) != nil {
		t.Error("Unwrap on a joined error returns nil")
	}
	if (Transfer{From: "a", To: "b", Amount: 1}).Validate() != nil {
		t.Error("valid transfer must give nil")
	}
}

type failClose struct{ strings.Builder }

func (*failClose) Close() error { return errors.New("close: disk full") }

func TestCloseErrorKept(t *testing.T) {
	err := writeAll(&failClose{}, []byte("data"))
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("close error lost: %v", err)
	}
}
