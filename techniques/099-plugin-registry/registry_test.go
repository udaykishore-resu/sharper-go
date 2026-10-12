// Package registry tests Technique 99 the way cmd/ledgerd would:
// it picks what ships with blank imports.
package registry

import (
	"strings"
	"testing"

	"github.com/udaykishore-resu/sharper-go/techniques/099-plugin-registry/sink"
	_ "github.com/udaykishore-resu/sharper-go/techniques/099-plugin-registry/sink/stdout"
)

// Assert the names production builds need.
func TestBuiltIn(t *testing.T) {
	if _, err := sink.New("stdout", map[string]string{"prefix": "> "}); err != nil {
		t.Fatal(err)
	}
	if _, err := sink.New("stdout", map[string]string{"ulr": "x"}); err == nil {
		t.Fatal("typo in config accepted")
	}
	_, err := sink.New("kafka", nil)
	if err == nil || !strings.Contains(err.Error(), "(have [stdout])") {
		t.Fatalf("err = %v", err)
	}
}

func TestDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate name accepted")
		}
	}()
	sink.Register("stdout", nil)
}
