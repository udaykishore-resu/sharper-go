package transfer

import (
	"strings"
	"testing"
	"time"

	"github.com/udaykishore-resu/sharper-go/techniques/internal/gotool"
)

// The cases the reviewer named, one at a time. Run this test also
// with GOEXPERIMENT=nojsonv2 to cover the older encoder.
func TestOmitzeroCases(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want string
	}{
		{"zero time", TransferOut{ID: "t1"}, `{"id":"t1","amount":0}`},
		{"set time", TransferOut{ID: "t1",
			PostedAt: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)},
			`{"id":"t1","amount":0,"posted_at":"2026-09-30T00:00:00Z"}`},
		{"zero Money, currency set",
			TransferOut{Fee: Money{Currency: "USD"}},
			`{"id":"","amount":0}`},
		{"non-zero Money", TransferOut{Fee: Money{Cents: 25}},
			`{"id":"","amount":0,"fee":{"Cents":25,"Currency":""}}`},
		{"nil slice, both options", TransferOut{}, `{"id":"","amount":0}`},
		{"empty slice, both options", TransferOut{Tags: []string{}},
			`{"id":"","amount":0}`},
		{"nil slice, omitzero only", OnlyZero{}, `{}`},
		{"empty slice, omitzero only", OnlyZero{Tags: []string{}},
			`{"tags":[]}`},
	}
	for _, c := range cases {
		if got := enc(t, c.v); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

// encoding/json skips tag options it does not know, with no error.
// Before Go 1.24, omitzero was such an option. Older toolchains
// are not installed here, so this checks the mechanism on both
// encoders Go 1.27.2 ships (run again with GOEXPERIMENT=nojsonv2).
func TestUnknownOptionIgnored(t *testing.T) {
	type T struct {
		//lint:ignore SA5008 the unknown option is the point
		N int `json:"n,omitnothing"`
	}
	if got := enc(t, T{}); got != `{"n":0}` {
		t.Fatalf("got %s", got)
	}
}

// The go fix omitzero analyzer finds omitempty on a struct field,
// where it has no effect. Its applied fix deletes the option;
// switching to omitzero is the behavior change you make by hand.
func TestGoFixOmitzero(t *testing.T) {
	src := "package m\n\nimport \"time\"\n\n" +
		"type T struct {\n" +
		"\tAt time.Time `json:\"at,omitempty\"`\n}\n"
	dir := gotool.Module(t, "1.26.0", map[string]string{"a.go": src})
	out, _ := gotool.Go(t, dir, "fix", "-omitzero", "-diff", ".")
	if !strings.Contains(out, "+\tAt time.Time `json:\"at\"`") ||
		!strings.Contains(out, "behavior change") {
		t.Fatalf("go fix -omitzero:\n%s", out)
	}
}
