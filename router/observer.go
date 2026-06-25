package router

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Observer records per-request Prometheus metrics for the router.
type Observer struct {
	policy   string
	duration *prometheus.HistogramVec
	requests *prometheus.CounterVec
	hits     *prometheus.CounterVec
}

// NewObserver registers metrics with reg and returns an Observer labeled by policy.
func NewObserver(reg prometheus.Registerer, policy string) *Observer {
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "router_request_duration_seconds",
		Help:    "End-to-end request latency.",
		Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	}, []string{"policy"})
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "router_requests_total",
		Help: "Total requests dispatched.",
	}, []string{"policy"})
	hits := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "router_cache_hits_total",
		Help: "Cache hit/miss counts.",
	}, []string{"policy", "hit"})
	reg.MustRegister(duration, requests, hits)
	return &Observer{policy: policy, duration: duration, requests: requests, hits: hits}
}

// Observe records a single completed request.
func (o *Observer) Observe(latency time.Duration, cacheHit bool) {
	o.duration.WithLabelValues(o.policy).Observe(latency.Seconds())
	o.requests.WithLabelValues(o.policy).Inc()
	hit := "false"
	if cacheHit {
		hit = "true"
	}
	o.hits.WithLabelValues(o.policy, hit).Inc()
}
