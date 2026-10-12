package golden

import (
	"bytes"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite")

func golden(t *testing.T, got []byte) {
	t.Helper()
	path := filepath.Join("testdata",
		t.Name()+".golden")
	if *update {
		os.MkdirAll(filepath.Dir(path), 0o755)
		err := os.WriteFile(path, got, 0o644)
		if err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update?)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s:\n got:\n%s\nwant:\n%s",
			path, got, want)
	}
}

func TestGetAccount(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/accounts/{id}", GetAccount)
	for _, id := range []string{"acc_1", "missing"} {
		t.Run(id, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/v1/accounts/"+id, nil)
			mux.ServeHTTP(rec, req)
			golden(t, rec.Body.Bytes())
		})
	}
}
