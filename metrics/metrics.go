package metrics

import (
	"sort"
	"sync"
	"time"
)

type sample struct {
	latency  time.Duration
	cacheHit bool
	policy   string
}

type Recorder struct {
	mu        sync.Mutex
	samples   []sample
	startTime time.Time
}

type Summary struct {
	Count                int
	P50, P90, P99        time.Duration
	RPS                  float64
	CacheHitRate         float64
}

func New() *Recorder { return &Recorder{startTime: time.Now()} }

func (r *Recorder) Record(policy string, latency time.Duration, cacheHit bool) {
	r.mu.Lock()
	r.samples = append(r.samples, sample{latency: latency, cacheHit: cacheHit, policy: policy})
	r.mu.Unlock()
}

func (r *Recorder) Summary() Summary {
	r.mu.Lock()
	cp := make([]sample, len(r.samples))
	copy(cp, r.samples)
	elapsed := time.Since(r.startTime)
	r.mu.Unlock()

	if len(cp) == 0 {
		return Summary{}
	}

	latencies := make([]time.Duration, len(cp))
	hits := 0
	for i, s := range cp {
		latencies[i] = s.latency
		if s.cacheHit {
			hits++
		}
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	pct := func(p float64) time.Duration {
		idx := int(float64(len(latencies)-1) * p)
		return latencies[idx]
	}

	rps := 0.0
	if elapsed > 0 {
		rps = float64(len(cp)) / elapsed.Seconds()
	}

	return Summary{
		Count:        len(cp),
		P50:          pct(0.50),
		P90:          pct(0.90),
		P99:          pct(0.99),
		RPS:          rps,
		CacheHitRate: float64(hits) / float64(len(cp)),
	}
}
