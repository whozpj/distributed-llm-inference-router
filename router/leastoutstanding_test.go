package router_test

import (
	"testing"

	"distributed-llm-inference-router/router"
)

func TestLORPicksLeastLoaded(t *testing.T) {
	lor := router.NewLeastOutstanding()
	replicas := makeReplicas(3)
	replicas[0].AddOutstanding(5)
	replicas[1].AddOutstanding(1)
	replicas[2].AddOutstanding(3)

	picked := lor.Pick(&router.Request{ID: "r"}, replicas)
	if picked.Index() != 1 {
		t.Fatalf("expected replica 1 (outstanding=1), got %d", picked.Index())
	}
}

func TestLORSkipsUnhealthy(t *testing.T) {
	lor := router.NewLeastOutstanding()
	replicas := makeReplicas(3)
	replicas[0].SetHealthy(false)
	replicas[1].SetHealthy(false)

	picked := lor.Pick(&router.Request{ID: "r"}, replicas)
	if picked.Index() != 2 {
		t.Fatalf("expected replica 2, got %d", picked.Index())
	}
}
