// Package obs is Technique 88: four metrics, small cardinality.
package obs

import (
	"cmp"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	Requests *prometheus.CounterVec
	Duration *prometheus.HistogramVec
	InFlight prometheus.Gauge
}

func NewMetrics(reg prometheus.Registerer, poolInUse func() float64) *Metrics {
	f := promauto.With(reg)
	f.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "db_pool_acquired_conns",
		Help: "Pool connections in use, read on each scrape.",
	}, poolInUse)
	return &Metrics{
		Requests: f.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total", Help: "Requests by route and code.",
		}, []string{"route", "code"}),
		Duration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_request_duration_seconds", Help: "Latency by route.",
			// An edge near the 250 ms SLO, and room past the
			// worst normal case.
			Buckets: []float64{.025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"route"}),
		InFlight: f.NewGauge(prometheus.GaugeOpts{
			Name: "http_in_flight_requests", Help: "Requests in flight.",
		}),
	}
}

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

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(c int) {
	if w.status == 0 {
		w.status = c
	}
	w.ResponseWriter.WriteHeader(c)
}
