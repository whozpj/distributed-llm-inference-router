# Distributed LLM Inference Router

A multi-replica LLM serving system in Go that studies the tradeoff between KV-cache locality and load balance. Implements continuous batching per replica and benchmarks three routing policies — round-robin, least-outstanding-requests, and prefix-cache-aware — under identical traffic to produce trustworthy latency and throughput comparisons.

---

## Motivation

Two facts drive modern LLM serving performance:

1. **Batching is where throughput comes from.** A GPU decoding one sequence at a time wastes most of its compute. Continuous batching — refilling the running batch every decode step rather than waiting for a static batch to drain — is the dominant throughput lever.

2. **KV-cache reuse is where latency comes from.** When two requests share a prompt prefix (a shared system prompt, a RAG context), routing them to the same replica lets the second request skip recomputing the first's prefill. That reuse is only possible if the router is aware of what each replica has cached.

These two objectives pull against each other. Routing for cache locality concentrates traffic on whichever replica is hot for a prefix, creating hotspots. Routing for load balance spreads traffic and discards cache reuse. This project measures that tradeoff rigorously, including the regimes where prefix-aware routing does not help.

---

## Architecture

```
                    ┌──────────────────┐
   clients ────────▶│     Router       │
   (load gen)       │                  │
                    │  admission queue │
                    │  routing policy  │
                    │  prefix-cache map│
                    │  health checks   │
                    └────────┬─────────┘
                             │ forwards requests
             ┌───────────────┼───────────────┐
             ▼               ▼               ▼
       ┌──────────┐    ┌──────────┐    ┌──────────┐
       │Replica 1 │    │Replica 2 │    │Replica 3 │
       │          │    │          │    │          │
       │ batcher  │    │ batcher  │    │ batcher  │
       │ KV cache │    │ KV cache │    │ KV cache │
       │  model   │    │  model   │    │  model   │
       └──────────┘    └──────────┘    └──────────┘
             │               │               │
             └───────────────┴───────────────┘
                             ▼
                    ┌──────────────────┐
                    │     Metrics      │
                    │ latency histograms│
                    │ cache hit rates  │
                    └──────────────────┘
```

### Components

| Component | Responsibility |
|-----------|---------------|
| **Router** | Receives requests, selects a replica via the active policy, forwards the request, streams the response back. Holds requests in a queue and applies backpressure when all replicas are saturated. |
| **Replica** | Runs a batcher loop and a mock model. Maintains a simulated KV cache keyed by prompt prefix. Exposes queue depth and cache state to the router. |
| **Batcher** | A single-goroutine loop that admits waiting requests into a running batch, advances every sequence one token per decode step, and evicts finished sequences. New arrivals join a batch already decoding on the next tick. |
| **Mock model** | `latency = base + per_token × tokens`. Prefill cost scales with uncached prompt tokens, so a cache hit produces a measurably cheaper request. |
| **Load generator** | Separate process producing bursty traffic with configurable prompt length distributions and prefix-overlap fraction. Overlap is a first-class experimental knob. |
| **Metrics** | Prometheus-style counters and latency histograms. Source of all reported numbers. |

---

## Routing Policies

All three policies implement a single interface, so they are swapped under identical traffic:

```go
type Policy interface {
    Pick(req *Request, replicas []*ReplicaClient) *ReplicaClient
}
```

| Policy | Strategy | Notes |
|--------|----------|-------|
| **Round-robin** | Cycles through replicas in order | Baseline; ignores load and cache state |
| **Least-outstanding-requests** | Picks the replica with fewest in-flight requests | Good load balance; cache-blind |
| **Prefix-cache-aware** | Scores replicas by load discounted by `localityWeight`; falls back to LOR when no hot replica exists or the best hot replica exceeds `hotspotThreshold` | Experimental surface: sweep the two knobs and observe the p99 / cache-hit-rate curve |

The `localityWeight` and `hotspotThreshold` knobs are the central experimental surface. The deliverable is the curve of p99 latency and cache hit rate as they vary, not a single tuned configuration.

---

## Prefix Tracking

A prefix hash (or radix tree over seen prefixes) maps `prefix → replicas currently hot for it`.

Two correctness requirements that are easy to omit but critical to include:

- **Eviction** — the prefix map is LRU-bounded. Without this it grows without limit.
- **Decay** — a replica stops being "hot" for a prefix after idle time or cache pressure. Stale hotness silently degrades the cache hit rate, so decay is part of the design.

---

## Continuous Batching

Each replica's batcher runs one goroutine that owns all batch state. Every decode tick:

1. **Admit** — pull from the waiting queue into the running batch up to `maxBatch` and a `tokenBudget`. Charge prefill cost only for the uncached portion of the prompt.
2. **Step** — advance every running sequence by one token; stream it to the caller.
3. **Evict** — remove finished sequences, free their working memory, and keep the prefix KV hot for future reuse.

Admit runs every tick, so a request arriving mid-decode joins on the next tick instead of waiting for the batch to drain. That refill is the entire throughput win and is the headline result of Phase 1.

Memory is capped on total tokens in flight (a proxy for KV memory) rather than request count, which is the honest model of GPU memory pressure when no real GPU is present.

---

## Concurrency Model

- One goroutine owns each batcher's state. No locks are needed on the hot path.
- The router uses atomics for per-replica outstanding counts and an `RWMutex` for the prefix map.
- Sharding a batch across goroutines with shared locks would trade clean latency numbers for race debugging. Clean measurement is a goal; the single-owner loop wins.

---

## Benchmark Methodology

- **Identical traffic across policies.** The same generated request stream is replayed per policy; otherwise comparisons are meaningless.
- **Conditions reported with every number.** Request rate, prefix-overlap fraction, prompt-length distribution, replica count.
- **Variance, not point estimates.** Each configuration is run multiple times; results report spread or confirm stability. A single run of a concurrent system is not evidence.
- **Reproducible.** `make bench` regenerates all numbers from a fixed seed.

### Traffic profiles

| Profile | Prefix overlap | Expected winner |
|---------|---------------|----------------|
| Cache-heavy | High | Prefix-cache-aware |
| Cache-cold | Near zero | Ties least-outstanding |
| Bursty | Varied | Tests backpressure and autoscaling |

### Metrics collected

- p50 / p90 / p99 latency
- Throughput (req/s and tokens/s)
- Cache hit rate
- Per-replica load distribution (to expose hotspots)

---

## Limitations

Stated up front because hiding them costs more credibility than naming them.

| Limitation | What it means |
|------------|--------------|
| Simulated inference | No real model or GPU. Decode and prefill costs are modeled functions of token counts. The distributed-systems behavior under study — routing, batching, backpressure, scaling — is independent of whether the matrix multiplications are real. |
| KV memory as token budget | A proxy for GPU memory pressure, not the real thing. |
| Coarse prefix matching | Hash or radix tree over leading tokens, not a production prefix-cache implementation. |
| Single machine | Router-to-replica fan-out over cheap local transport; no cross-host failures or partitions. |

Each is a simplification of scope, not a flaw in the results. All policies run against the same model, so the measured tradeoffs hold.

---

## Getting Started

```bash
# Build everything
make build

# Run the router and replicas
make run

# Run all benchmarks (fixed seed, reproducible)
make bench

# Run tests
make test
```

> **Requirements:** Go 1.22+

---

## Project Status

`Draft` — actively in development. Phases 0–2 are the primary deliverable.
