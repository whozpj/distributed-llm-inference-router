package router_test

import (
	"testing"
	"time"

	"distributed-llm-inference-router/router"
)

func TestPrefixAwareRoutesToHotReplica(t *testing.T) {
	pm := router.NewPrefixMap(100, time.Minute)
	cfg := router.PrefixAwareConfig{PrefixLen: 4, LocalityWeight: 1.0, HotspotThreshold: 100}
	pa := router.NewPrefixAware(cfg, pm, router.NewLeastOutstanding())

	replicas := makeReplicas(3)
	req := &router.Request{ID: "r", PromptTokens: []int32{1, 2, 3, 4, 5}, MaxNewTokens: 5}

	hash := router.HashPrefix(req.PromptTokens, cfg.PrefixLen)
	pm.Record(hash, replicas[2].ID())

	picked := pa.Pick(req, replicas)
	if picked.Index() != 2 {
		t.Fatalf("expected hot replica 2, got %d", picked.Index())
	}
}

func TestPrefixAwareFallsBackWhenHotReplicaOverloaded(t *testing.T) {
	pm := router.NewPrefixMap(100, time.Minute)
	cfg := router.PrefixAwareConfig{PrefixLen: 4, LocalityWeight: 1.0, HotspotThreshold: 2}
	pa := router.NewPrefixAware(cfg, pm, router.NewLeastOutstanding())

	replicas := makeReplicas(3)
	replicas[0].AddOutstanding(10)
	replicas[1].AddOutstanding(1)
	replicas[2].AddOutstanding(5)

	req := &router.Request{ID: "r", PromptTokens: []int32{1, 2, 3, 4, 5}, MaxNewTokens: 5}
	hash := router.HashPrefix(req.PromptTokens, cfg.PrefixLen)
	pm.Record(hash, replicas[0].ID())

	picked := pa.Pick(req, replicas)
	if picked.Index() == 0 {
		t.Fatal("should have fallen back from overloaded hot replica")
	}

	// Verify the fallback choice was recorded so the next identical request goes to the same replica.
	third := pa.Pick(req, replicas)
	if third.Index() != picked.Index() {
		t.Fatalf("fallback from overloaded hot replica not recorded: third pick went to %d, want %d", third.Index(), picked.Index())
	}
}

func TestPrefixAwareFallsBackOnColdPrefix(t *testing.T) {
	pm := router.NewPrefixMap(100, time.Minute)
	cfg := router.PrefixAwareConfig{PrefixLen: 4, LocalityWeight: 1.0, HotspotThreshold: 100}
	pa := router.NewPrefixAware(cfg, pm, router.NewLeastOutstanding())

	replicas := makeReplicas(3)
	replicas[0].AddOutstanding(5)
	replicas[1].AddOutstanding(1)
	replicas[2].AddOutstanding(3)

	req := &router.Request{ID: "r", PromptTokens: []int32{99, 98, 97, 96}, MaxNewTokens: 2}
	picked := pa.Pick(req, replicas)
	if picked.Index() != 1 {
		t.Fatalf("cold prefix: expected LOR to pick replica 1 (outstanding=1), got %d", picked.Index())
	}
}

func TestPrefixAwareRecordsOnFallback(t *testing.T) {
	pm := router.NewPrefixMap(100, time.Minute)
	cfg := router.PrefixAwareConfig{PrefixLen: 4, LocalityWeight: 1.0, HotspotThreshold: 100}
	pa := router.NewPrefixAware(cfg, pm, router.NewRoundRobin())

	replicas := makeReplicas(3)
	req := &router.Request{ID: "r", PromptTokens: []int32{7, 8, 9, 10, 11}, MaxNewTokens: 2}

	// First pick: PrefixMap is empty, falls back to round-robin, picks replica 0.
	first := pa.Pick(req, replicas)

	// Second pick: PrefixMap should now have an entry → must return same replica.
	second := pa.Pick(req, replicas)
	if first.Index() != second.Index() {
		t.Fatalf("second request routed to replica %d, want %d (same as first)",
			second.Index(), first.Index())
	}
}
