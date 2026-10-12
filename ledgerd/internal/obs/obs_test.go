package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	stdlog "log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/trace"
)

// Technique 86: trace IDs survive logger.With.
func TestTraceIDAfterWith(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "test").With("route", "x")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{1},
	})
	ctx := trace.ContextWithSpanContext(
		context.Background(), sc)
	log.InfoContext(ctx, "posted")
	if !strings.Contains(buf.String(), `"trace_id"`) {
		t.Fatalf("trace_id lost after With: %s", &buf)
	}
}

func TestRedact(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "test")
	log.Info("auth", "Authorization", "Bearer x", "dsn", Secret("postgres://u:p@h"))
	if strings.Contains(buf.String(), "Bearer x") ||
		strings.Contains(buf.String(), "u:p@h") {
		t.Fatalf("secret leaked: %s", &buf)
	}
}

// Technique 88: a real route, a 404 and a 405 give two route values.
func TestMetricsRouteLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/accounts/{id}", func(http.ResponseWriter, *http.Request) {})
	h := m.Wrap(mux)
	for _, c := range []struct{ method, path string }{
		{"GET", "/v1/accounts/a1"},
		{"GET", "/wp-admin"},
		{"DELETE", "/v1/accounts/a1"},
	} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(c.method, c.path, nil))
	}
	if n := testutil.CollectAndCount(m.Duration); n != 2 {
		t.Fatalf("route series = %d, want 2", n)
	}
	want := `
		# HELP ledgerd_http_requests_total Requests by route pattern and status code.
		# TYPE ledgerd_http_requests_total counter
		ledgerd_http_requests_total{code="200",route="GET /v1/accounts/{id}"} 1
		ledgerd_http_requests_total{code="404",route="unmatched"} 1
		ledgerd_http_requests_total{code="405",route="unmatched"} 1
	`
	if err := testutil.CollectAndCompare(m.Requests, strings.NewReader(want)); err != nil {
		t.Fatal(err)
	}
}

func TestBuildInfo(t *testing.T) {
	b := BuildInfo()
	if b.Version == "" || !strings.HasPrefix(b.Go, "go1.") {
		t.Fatalf("BuildInfo() = %+v", b)
	}
	_ = slog.Default()
}

type withToken struct{ id string }

// LogValue returns a group whose leaf key is on the key list.
func (w withToken) LogValue() slog.Value {
	return slog.GroupValue(slog.String("id", w.id),
		slog.String("token", "tok-in-logvalue"))
}

// Technique 86 review: what the key list and the Secret type
// catch, and the shapes they don't.
func TestRedactNested(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "test")
	log.Info("top", "AUTHORIZATION", "Bearer top")
	log.Info("group", slog.Group("req",
		slog.String("password", "pw-in-group"),
		slog.Group("inner", slog.String("Token", "tok-deep"))))
	log.WithGroup("req").Info("withgroup", "token", "tok-withgroup")
	log.With("dsn", "postgres://u:with@h").Info("with")
	log.Info("logvaluer", "acct", withToken{"a1"})
	log.Info("secret in group", slog.Group("req",
		slog.Any("auth", Secret("Bearer secret-in-group"))))
	out := buf.String()
	for _, leak := range []string{"Bearer top", "pw-in-group",
		"tok-deep", "tok-withgroup", "u:with@h",
		"tok-in-logvalue", "secret-in-group"} {
		if strings.Contains(out, leak) {
			t.Errorf("leaked %q: %s", leak, out)
		}
	}

	// The limits the book states: the key list matches leaf keys
	// only, and nothing looks inside maps, structs or error text.
	buf.Reset()
	log.Info("map", "h", map[string]string{"Authorization": "map-leak"})
	log.Info("struct", "c", struct{ Password string }{"struct-leak"})
	log.Info("group name", slog.Group("token", slog.String("v", "groupname-leak")))
	log.Info("err", "err", errors.New("dial: password=err-leak"))
	for _, leak := range []string{"map-leak", "struct-leak", "groupname-leak", "err-leak"} {
		if !strings.Contains(buf.String(), leak) {
			t.Errorf("expected %q to pass through (documented limit)", leak)
		}
	}
}

// Technique 85 review: levels, the old log package, and errors.
func TestLoggerLevelsDefaultAndErrors(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(&buf, "v9")
	prev := slog.Default()
	slog.SetDefault(l)
	t.Cleanup(func() { slog.SetDefault(prev) })

	stdlog.Printf("from a dependency") // routed through l's handler
	l.Debug("hidden at info")
	Level.Set(slog.LevelDebug)
	l.Debug("shown at debug")
	Level.Set(slog.LevelInfo)
	l.Error("transfer failed", "err", errors.New("insufficient funds"))

	var lines []map[string]any
	for _, s := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatalf("not JSON: %q", s)
		}
		lines = append(lines, m)
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines: %s", len(lines), &buf)
	}
	if lines[0]["msg"] != "from a dependency" || lines[0]["service"] != "ledgerd" {
		t.Fatalf("log.Printf not routed: %v", lines[0])
	}
	if lines[1]["msg"] != "shown at debug" {
		t.Fatalf("LevelVar: %v", lines[1])
	}
	if lines[2]["err"] != "insufficient funds" || lines[2]["level"] != "ERROR" {
		t.Fatalf("error field: %v", lines[2])
	}
}

// Technique 88 review: every metric on the registry is labelled
// from a fixed set, so no ID can become a label.
func TestNoIDLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	m.WatchPool(func() int32 { return 1 })
	h := m.Wrap(http.NotFoundHandler())
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/accounts/acct_118", nil))
	Panics.Add(0)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"route": true, "code": true}
	for _, mf := range mfs {
		if !strings.HasPrefix(mf.GetName(), "ledgerd_") {
			continue // Go and process collectors carry no labels of ours
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if !allowed[lp.GetName()] || strings.Contains(lp.GetValue(), "acct_") {
					t.Errorf("%s has label %s=%q", mf.GetName(), lp.GetName(), lp.GetValue())
				}
			}
		}
	}
}

// Technique 89 review: expvar is on the debug mux only. The
// public side never serves http.DefaultServeMux, where importing
// expvar registers /debug/vars.
func TestVarsOnDebugMuxOnly(t *testing.T) {
	ConfigVersion.Set("v41")
	rec := httptest.NewRecorder()
	DebugMux().ServeHTTP(rec, httptest.NewRequest("GET", "/debug/vars", nil))
	var vars map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &vars); err != nil || vars["config_version"] != "v41" {
		t.Fatalf("debug mux: %d %v", rec.Code, err)
	}
	if _, ok := vars["build"]; !ok {
		t.Fatal("build var missing")
	}
	rec = httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(rec, httptest.NewRequest("GET", "/debug/vars", nil))
	if rec.Code != 200 {
		t.Fatal("expected the import side effect on DefaultServeMux")
	}
}
