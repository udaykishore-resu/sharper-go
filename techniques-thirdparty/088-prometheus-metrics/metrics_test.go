package obs

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// The Try it: a real route, a 404 path and a 405 method give two
// route values, and /metrics serves them from a private registry.
func TestTwoRoutes(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	m := NewMetrics(reg, func() float64 { return 3 })
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/accounts/{id}", func(http.ResponseWriter, *http.Request) {})
	h := m.Wrap(mux)
	for _, c := range [][2]string{{"GET", "/v1/accounts/a1"}, {"GET", "/wp-admin"}, {"POST", "/v1/accounts/a1"}} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(c[0], c[1], nil))
	}
	metrics := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer metrics.Close()
	resp, err := http.Get(metrics.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	routes := map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "http_requests_total{") {
			i := strings.Index(line, `route="`)
			routes[line[i+7:strings.Index(line[i+7:], `"`)+i+7]] = true
		}
	}
	if len(routes) != 2 || !routes["unmatched"] || !routes["GET /v1/accounts/{id}"] {
		t.Fatalf("routes = %v", routes)
	}
	if !strings.Contains(string(body), "db_pool_acquired_conns 3") {
		t.Fatal("pool gauge missing")
	}
}
