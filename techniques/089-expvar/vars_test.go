package obs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVars(t *testing.T) {
	ConfigVersion.Set("v41")
	Rejects.Add("insufficient_funds", 1)
	fraudBreaker.open.Store(true)
	debug := http.NewServeMux()
	MountVars(debug)
	rec := httptest.NewRecorder()
	debug.ServeHTTP(rec, httptest.NewRequest("GET", "/debug/vars", nil))
	var vars map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &vars); err != nil {
		t.Fatal(err)
	}
	if vars["config_version"] != "v41" || vars["breaker_fraud"] != "open" {
		t.Fatalf("vars: %v %v", vars["config_version"], vars["breaker_fraud"])
	}
	if _, ok := vars["memstats"]; !ok {
		t.Fatal("memstats missing")
	}
	// The public server has its own mux, so the path is a 404 there.
	public := http.NewServeMux()
	rec = httptest.NewRecorder()
	public.ServeHTTP(rec, httptest.NewRequest("GET", "/debug/vars", nil))
	if rec.Code != 404 {
		t.Fatalf("public port serves vars: %d", rec.Code)
	}
}
