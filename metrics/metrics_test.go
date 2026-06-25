package metrics_test

import (
	"testing"
	"time"

	"distributed-llm-inference-router/metrics"
)

func TestPercentilesOnKnownData(t *testing.T) {
	r := metrics.New()
	for i := 1; i <= 100; i++ {
		r.Record("rr", time.Duration(i)*time.Millisecond, i%2 == 0)
	}
	s := r.Summary()
	if s.Count != 100 {
		t.Fatalf("count: got %d, want 100", s.Count)
	}
	if s.P50 < 49*time.Millisecond || s.P50 > 52*time.Millisecond {
		t.Fatalf("p50 out of range: %v", s.P50)
	}
	if s.P90 < 89*time.Millisecond || s.P90 > 92*time.Millisecond {
		t.Fatalf("p90 out of range: %v", s.P90)
	}
	if s.P99 < 98*time.Millisecond || s.P99 > 100*time.Millisecond {
		t.Fatalf("p99 out of range: %v", s.P99)
	}
	if s.CacheHitRate < 0.49 || s.CacheHitRate > 0.51 {
		t.Fatalf("cache hit rate: got %.2f, want ~0.50", s.CacheHitRate)
	}
}

func TestEmptySummaryIsSafe(t *testing.T) {
	r := metrics.New()
	s := r.Summary()
	if s.Count != 0 || s.P50 != 0 || s.RPS != 0 {
		t.Fatalf("non-zero summary on empty recorder: %+v", s)
	}
}
