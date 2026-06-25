# Design: Simulation Realism + Prometheus Metrics

**Date:** 2026-06-24  
**Status:** Approved

---

## Goal

Make the benchmark tell a true, interesting story about routing policy tradeoffs, and make the results observable via Prometheus. Currently all three policies show identical latency and cache-hit rates because the simulation is too forgiving. This design fixes that and adds live metrics export.

---

## Problem: Why policies don't diverge today

With 3 replicas and one shared prefix, every replica warms its KV cache after the first request. After that, round-robin, LOR, and prefix-aware all get the same cache hit rate — the routing decision is irrelevant.

For routing to matter, replicas must **not** all be able to cache everything. This requires:
1. More distinct prefixes than any single replica can hold
2. A large enough latency penalty for a cache miss that the routing decision dominates

---

## Change 1: Simulation Realism

### Multiple distinct prefixes

`loadgen.Config` gets a new field `NumPrefixes int`. The generator builds `NumPrefixes` distinct shared prefixes from the seed, then assigns each request one of them (weighted by `OverlapFraction`). Requests not assigned a prefix get a fully random prompt.

Default for benchmark: `NumPrefixes = 8`.

### Constrained replica cache capacity

Replicas are configured with `cache.New(maxItems=4, ...)` — half of `NumPrefixes`. Each replica can only hold 4 of the 8 prefixes at once, forcing LRU eviction as new prefixes arrive.

This means: a request routed to a replica that evicted its prefix will miss, paying full prefill cost.

### Dramatic latency gap

| Parameter | Before | After |
|-----------|--------|-------|
| PrefillPerToken | 2ms | 10ms |
| DecodePerToken | 1ms | 1ms |
| TickInterval | 5ms | 10ms |

A 32-token cold prefill now costs ~320ms. A full cache hit (0 uncached tokens) costs ~0ms prefill. The routing decision becomes the dominant latency factor.

### Expected benchmark behavior

| Scenario | Round-Robin | LOR | Prefix-Aware |
|----------|-------------|-----|--------------|
| cache-heavy | high p99 (cold replicas) | similar to RR | low p99 (warm replica) |
| cache-cold | similar across all | similar | similar (no prefix to exploit) |
| bursty | high variance | better than RR | best |

Prefix-aware should be 3–5x lower p99 than round-robin in cache-heavy.

---

## Change 2: Prometheus Metrics on the Router

### New file: `router/observer.go`

Defines and registers three Prometheus metrics:

```
router_request_duration_seconds  histogram  labels: policy
router_requests_total            counter    labels: policy
router_cache_hits_total          counter    labels: policy, hit (true|false)
```

Exposes a single `observe(policy, duration, cacheHit)` function called from the router's `worker()` after each replica response.

### Router config changes

`router.Config` gets two new fields:

```go
PolicyName  string        // e.g. "roundrobin", "lor", "prefixaware"
MetricsAddr string        // e.g. ":9100" — empty disables metrics
```

### HTTP metrics endpoint

`cmd/router/main.go` starts an `http.Server` on `MetricsAddr` serving `promhttp.Handler()` at `/metrics`. The server runs in a goroutine alongside the gRPC server.

### Integration with existing metrics.Recorder

`metrics.Recorder` is unchanged — it still drives the printed benchmark table. Prometheus is purely additive. The loadgen prints `Metrics: http://localhost:9100/metrics` at the start of each policy run so results can be curled or scraped.

---

## Change 3: README + Architecture Diagram

`README.md` gets:

1. **Architecture diagram** (ASCII) showing: `loadgen → router → [replica-0 ... replica-N]`, with the router's policy and metrics port labeled.
2. **How to run** section: `make bench` command and expected output.
3. **Results table** with the post-fix benchmark numbers showing policy divergence.
4. **Metrics** section: how to curl `/metrics` during a run.

---

## Files changed

| File | Action |
|------|--------|
| `loadgen/loadgen.go` | Add `NumPrefixes` field, multi-prefix generation |
| `loadgen/loadgen_test.go` | Add test for multi-prefix distribution |
| `router/observer.go` | New — Prometheus metric definitions + `observe()` |
| `router/router.go` | Call `observe()` in `worker()` after replica response |
| `router/router.go` | Add `PolicyName`, `MetricsAddr` to `Config` |
| `cmd/router/main.go` | Start HTTP metrics server if `MetricsAddr` set |
| `cmd/loadgen/main.go` | Use new loadgen config, print metrics URL, updated sim params |
| `go.mod` / `go.sum` | Add `prometheus/client_golang` |
| `README.md` | Architecture diagram, how-to-run, results table |

---

## Success criteria

1. `make bench` produces a results table where prefix-aware p99 is at least 2x lower than round-robin in the `cache-heavy` scenario.
2. `curl http://localhost:9100/metrics` during a benchmark run returns valid Prometheus text format with all three metric families.
3. All existing tests still pass with `go test ./... -race`.
4. README renders correctly on GitHub with diagram and results table.
