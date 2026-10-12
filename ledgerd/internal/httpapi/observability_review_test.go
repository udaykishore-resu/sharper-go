package httpapi

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"example.com/ledgerd/internal/obs"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// prodStack is serve.go's chain, in the order that
// cmd/ledgerd's TestServeMiddlewareOrderAndListeners pins, over
// the real routes plus two that panic.
func prodStack(t *testing.T) (http.Handler, *obs.Metrics, *prometheus.Registry, *syncBuf) {
	t.Helper()
	logs := &syncBuf{}
	log := slog.New(slog.NewJSONHandler(logs, nil))
	routes := New(newFake(), log).Routes()
	routes.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) {
		nilMap()["x"] = 1
	})
	routes.HandleFunc("GET /late", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("partial"))
		http.NewResponseController(w).Flush()
		panic("late")
	})
	reg := prometheus.NewRegistry()
	m := obs.NewMetrics(reg)
	api := Chain(routes,
		RequestID,
		Auth(map[string]string{token: "acme"}),
		Deadline(time.Second),
		otelhttp.NewMiddleware("ledgerd"),
		AccessLog(log),
		m.Wrap,
		Recover(log),
		LimitBody(1<<20),
	)
	h := NewHealth(nil)
	return h.Mux(api), m, reg, logs
}

type labelPair struct{ route, code string }

// labelPairs reads every request-counter series from the
// registry, the same data /metrics serves.
func labelPairs(t *testing.T, reg *prometheus.Registry) []labelPair {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var out []labelPair
	for _, mf := range mfs {
		if mf.GetName() != "ledgerd_http_requests_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			var p labelPair
			for _, lp := range m.GetLabel() {
				switch lp.GetName() {
				case "route":
					p.route = lp.GetValue()
				case "code":
					p.code = lp.GetValue()
				}
			}
			out = append(out, p)
		}
	}
	return out
}

// Technique 88 review: through the production chain, 404s,
// 405s, scanner paths, unclean paths and CONNECT all land on a
// registered pattern or "unmatched": the label set is bounded by
// the route table, whatever paths clients send.
func TestRouteLabelsThroughProdChain(t *testing.T) {
	h, m, reg, _ := prodStack(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	send := func(method, path string) int {
		req, _ := http.NewRequest(method, srv.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := send("GET", "/v1/accounts/a1"); c != 200 {
		t.Fatalf("real route: %d", c)
	}
	if send("GET", "/wp-admin") != 404 || send("DELETE", "/v1/accounts/a1") != 405 {
		t.Fatal("want 404 and 405")
	}
	for i := range 50 {
		send("GET", fmt.Sprintf("/scan/%d/x", i))
		send("GET", fmt.Sprintf("/v1/accounts/a%d/", i))
		send("GET", fmt.Sprintf("/v1//accounts/a%d", i)) // redirected by the outer mux
		// The client sends CONNECT in authority form, so drive
		// the handler directly to put a path on it.
		req := httptest.NewRequest("CONNECT",
			fmt.Sprintf("/v1/accounts/c%d", i), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	allowed := map[string]bool{
		"unmatched":                     true,
		"GET /v1/accounts/{id}":         true,
		"GET /v1/accounts/{id}/entries": true,
	}
	routes := map[string]bool{}
	for _, l := range labelPairs(t, reg) {
		routes[l.route] = true
		if !allowed[l.route] {
			t.Errorf("unexpected route label %q (code %s)", l.route, l.code)
		}
	}
	if !routes["unmatched"] || !routes["GET /v1/accounts/{id}"] {
		t.Fatalf("routes = %v", routes)
	}
	if n := testutil.CollectAndCount(m.Duration); n > len(allowed) {
		t.Fatalf("duration series = %d", n)
	}
}

// Technique 90 review: through the production chain, a panic
// before the first write is a 500 that the access log and the
// request counter both record; a panic after the first write
// aborts the connection, so only the panic counter sees it.
func TestPanicsThroughProdChain(t *testing.T) {
	h, m, reg, logs := prodStack(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	get := func(path string) (*http.Response, error) {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		return srv.Client().Do(req)
	}
	before := testutil.ToFloat64(obs.Panics)

	resp, err := get("/boom")
	if err != nil || resp.StatusCode != 500 {
		t.Fatalf("early panic: %v %v", resp, err)
	}
	resp.Body.Close()
	if v := testutil.ToFloat64(m.Requests.WithLabelValues("GET /boom", "500")); v != 1 {
		t.Fatalf("early panic counted %v times", v)
	}

	resp, err = get("/late")
	if err == nil {
		_, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("late panic: half body read as success")
	}
	if got := testutil.ToFloat64(obs.Panics) - before; got != 2 {
		t.Fatalf("panic counter moved by %v, want 2", got)
	}
	for _, l := range labelPairs(t, reg) {
		if l.route == "GET /late" {
			t.Fatalf("aborted request counted as %s", l.code)
		}
	}
	out := logs.String()
	if !strings.Contains(out, `"route":"GET /boom","status":500`) {
		t.Fatalf("access log lacks the 500: %s", out)
	}
	if strings.Contains(out, `"msg":"request","route":"GET /late"`) {
		t.Fatalf("aborted request reached the access log: %s", out)
	}
}

// Auth runs outside metrics: a 401 never reaches the request
// counter, so Auth logs its own rejections (Technique 47).
func TestAuthRejectionsSkipMetrics(t *testing.T) {
	h, m, _, _ := prodStack(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/accounts/a1", nil))
	if rec.Code != 401 || testutil.CollectAndCount(m.Requests) != 0 {
		t.Fatalf("code %d, series %d", rec.Code, testutil.CollectAndCount(m.Requests))
	}
}
