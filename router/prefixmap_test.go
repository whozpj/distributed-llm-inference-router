package router_test

import (
	"testing"
	"time"

	"distributed-llm-inference-router/router"
)

func TestPrefixMapMissOnEmpty(t *testing.T) {
	pm := router.NewPrefixMap(10, time.Minute)
	_, ok := pm.Lookup(999)
	if ok {
		t.Fatal("expected miss on empty map")
	}
}

func TestPrefixMapRecordAndLookup(t *testing.T) {
	pm := router.NewPrefixMap(10, time.Minute)
	pm.Record(42, "replica-0")
	pm.Record(42, "replica-1")

	ids, ok := pm.Lookup(42)
	if !ok {
		t.Fatal("expected hit after record")
	}
	found := map[string]bool{}
	for _, id := range ids {
		found[id] = true
	}
	if !found["replica-0"] || !found["replica-1"] {
		t.Fatalf("missing replica IDs: %v", ids)
	}
}

func TestPrefixMapLRUEviction(t *testing.T) {
	pm := router.NewPrefixMap(2, time.Minute)
	pm.Record(1, "r0")
	pm.Record(2, "r0")
	pm.Record(3, "r0") // evicts hash 1

	if _, ok := pm.Lookup(1); ok {
		t.Fatal("hash 1 should have been evicted")
	}
	if _, ok := pm.Lookup(3); !ok {
		t.Fatal("hash 3 should be present")
	}
}

func TestPrefixMapTTLDecay(t *testing.T) {
	pm := router.NewPrefixMap(10, 10*time.Millisecond)
	pm.Record(77, "r0")
	time.Sleep(20 * time.Millisecond)
	_, ok := pm.Lookup(77)
	if ok {
		t.Fatal("entry should have expired")
	}
}
