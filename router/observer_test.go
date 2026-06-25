package router_test

import (
	"testing"
	"time"

	"distributed-llm-inference-router/router"

	"github.com/prometheus/client_golang/prometheus"
)

func TestObserverRecordsMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	obs := router.NewObserver(reg, "testpolicy")

	obs.Observe(100*time.Millisecond, true)
	obs.Observe(200*time.Millisecond, false)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(mfs) != 3 {
		t.Fatalf("expected 3 metric families, got %d", len(mfs))
	}
	names := map[string]bool{}
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	for _, want := range []string{
		"router_request_duration_seconds",
		"router_requests_total",
		"router_cache_hits_total",
	} {
		if !names[want] {
			t.Fatalf("missing metric family %q", want)
		}
	}
}
