package obs

import (
	"cmp"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics are the four signals of Technique 88. Labels come from
// a small fixed set: the mux pattern and the status code.
type Metrics struct {
	Requests *prometheus.CounterVec
	Duration *prometheus.HistogramVec
	InFlight prometheus.Gauge
	reg      *prometheus.Registry
}

// Panics counts recovered panics (Technique 90). NewMetrics
// registers it; alert when it moves at all.
var Panics = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "ledgerd_http_panics_total",
	Help: "Handler panics turned into 500s.",
})

// NewMetrics registers the request metrics plus the Go and
// process collectors on a private registry.
func NewMetrics(reg *prometheus.Registry) *Metrics {
	f := promauto.With(reg)
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		Panics,
	)
	return &Metrics{
		reg: reg,
		Requests: f.NewCounterVec(prometheus.CounterOpts{
			Name: "ledgerd_http_requests_total",
			Help: "Requests by route pattern and status code.",
		}, []string{"route", "code"}),
		Duration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name: "ledgerd_http_request_duration_seconds",
			Help: "Request latency by route pattern.",
			// Edges around the 250 ms SLO, plus room past the
			// worst normal case so slow requests are not all +Inf.
			Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"route"}),
		InFlight: f.NewGauge(prometheus.GaugeOpts{
			Name: "ledgerd_http_in_flight_requests",
			Help: "Requests being served right now.",
		}),
	}
}

// WatchPool adds the fourth signal: pool connections in use,
// read on each scrape so it never drifts.
func (m *Metrics) WatchPool(inUse func() int32) {
	m.reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "ledgerd_db_pool_acquired_conns",
		Help: "Postgres pool connections in use.",
	}, func() float64 { return float64(inUse()) }))
}

// Wrap records rate, errors and latency per route. It passes r
// through untouched, so after the mux runs it reads r.Pattern
// from the same request (Technique 47).
func (m *Metrics) Wrap(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		m.InFlight.Inc()
		defer m.InFlight.Dec()
		sw := &statusWriter{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(sw, r)
		route := cmp.Or(r.Pattern, "unmatched")
		code := cmp.Or(sw.status, http.StatusOK)
		m.Duration.WithLabelValues(route).
			Observe(time.Since(start).Seconds())
		m.Requests.WithLabelValues(route,
			strconv.Itoa(code)).Inc()
	}
	return http.HandlerFunc(fn)
}

// statusWriter records the first status code. httpapi has the
// same small type; each package keeps its own copy so obs does
// not import the HTTP layer.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
