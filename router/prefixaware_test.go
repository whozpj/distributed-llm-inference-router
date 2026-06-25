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
