# Distributed LLM Inference Router

A simulated distributed LLM inference router benchmarking three routing policies under identical traffic. Built in Go with gRPC transport.

## Architecture

```
                        ┌─────────────────────────────────────────┐
                        │              Router                      │
  Load Generator ──────▶│  Policy: roundrobin | lor | prefixaware  │──▶ /metrics :9100
                        │  Queue depth: 512  Workers: 8           │
                        └────────────┬────────────────────────────┘
                                     │ gRPC (InferenceService)
                   ┌─────────────────┼─────────────────┐
                   ▼                 ▼                 ▼
            ┌────────────┐   ┌────────────┐   ┌────────────┐
            │  Replica 0 │   │  Replica 1 │   │  Replica 2 │
            │  Batcher   │   │  Batcher   │   │  Batcher   │
            │  KV Cache  │   │  KV Cache  │   │  KV Cache  │
            └────────────┘   └────────────┘   └────────────┘
```

Each replica runs a tick-based continuous batcher with an LRU KV cache (capacity: 2 prefixes). Routing policy determines which replica handles each request — and therefore whether the request hits a warm cache.

## Routing Policies

| Policy | Strategy |
|--------|----------|
| `roundrobin` | Cycles through replicas evenly |
| `lor` | Routes to the replica with fewest outstanding requests |
| `prefixaware` | Routes to the replica that previously cached this prompt prefix; falls back to LOR |

## Simulation Parameters

- **Prefill cost (cache miss):** 10ms per token — a 32-token prompt costs ~320ms prefill
- **Prefill cost (cache hit):** 0ms — KV cache serves it instantly
- **Decode cost:** 1ms per token
- **KV cache per replica:** 2 slots (LRU eviction)
- **Distinct prompt prefixes:** 6 (replicas can only cache 2, forcing eviction under round-robin)

## Results

`make bench` output (`--seed=42 --requests=200 --replicas=3`):

```
scenario        policy           count       p50       p90       p99    cache%
----------------------------------------------------------------------
cache-heavy     roundrobin         200     2.29s    3.958s     4.49s     25.5%
cache-heavy     lor                200    2.179s     3.85s    4.166s     27.0%
cache-heavy     prefixaware        200     868ms    1.327s    1.579s     61.0%
cache-cold      roundrobin         200     3.62s    6.359s     6.93s      0.0%
cache-cold      lor                200    3.617s    6.357s     6.93s      0.0%
cache-cold      prefixaware        200    3.619s    6.359s     6.93s      0.0%
bursty          roundrobin         200     4.47s    7.971s    8.759s      6.5%
bursty          lor                200    4.465s     8.15s    8.829s      6.0%
bursty          prefixaware        200    3.745s    6.762s    7.419s     22.5%
```

**Key insight:** `prefixaware` routes same-prefix requests to the same replica, keeping its cache warm. `roundrobin` distributes them across all replicas, which can only hold 2 of 6 prefixes — causing frequent eviction and cold prefill costs.

## How to Run

```bash
# Run the full benchmark
make bench

# Single scenario
go run ./cmd/loadgen --scenario=cache-heavy --requests=200 --replicas=3

# With Prometheus metrics (scrape during run)
go run ./cmd/loadgen --scenario=cache-heavy --metrics-addr=:9100

# Run tests
make test
```

## Prometheus Metrics

When running with `--metrics-addr=:9100`:

```bash
curl http://localhost:9100/metrics | grep router_
```

Exposes:
- `router_request_duration_seconds` — latency histogram, labeled by `policy`
- `router_requests_total` — request count, labeled by `policy`
- `router_cache_hits_total` — cache hit/miss counts, labeled by `policy` and `hit`

## Project Structure

```
cmd/
  loadgen/    # Benchmark runner (3 scenarios × 3 policies)
  replica/    # Standalone replica binary
  router/     # Standalone router binary
loadgen/      # Load generator with multi-prefix support
metrics/      # In-process p50/p90/p99 recorder
replica/
  batcher/    # Tick-based continuous batcher
  cache/      # LRU KV cache with TTL
  model/      # Mock model (prefill/decode cost simulation)
  server/     # gRPC server wrapping the batcher
router/
  client.go         # ReplicaClient with outstanding-request tracking
  observer.go       # Prometheus metrics
  policy.go         # Policy interface
  prefixaware.go    # Prefix-cache-aware policy
  prefixmap.go      # LRU prefix→replica map with TTL
  leastoutstanding.go
  roundrobin.go
  router.go         # Admission queue + dispatch workers
```
