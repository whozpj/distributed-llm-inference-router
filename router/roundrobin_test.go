package router_test

import (
	"testing"

	"distributed-llm-inference-router/router"
)

func makeReplicas(n int) []*router.ReplicaClient {
	replicas := make([]*router.ReplicaClient, n)
	for i := range replicas {
		replicas[i] = router.NewTestReplicaClient(i)
	}
	return replicas
}

func TestRoundRobinCycles(t *testing.T) {
	rr := router.NewRoundRobin()
	replicas := makeReplicas(3)

	seen := make([]int, 3)
	for i := 0; i < 9; i++ {
		r := rr.Pick(&router.Request{ID: "r"}, replicas)
		seen[r.Index()]++
	}
	for i, count := range seen {
		if count != 3 {
			t.Fatalf("replica %d picked %d times, want 3", i, count)
		}
	}
}

func TestRoundRobinSkipsUnhealthy(t *testing.T) {
	rr := router.NewRoundRobin()
	replicas := makeReplicas(3)
	replicas[1].SetHealthy(false)

	for i := 0; i < 6; i++ {
		r := rr.Pick(&router.Request{ID: "r"}, replicas)
		if r.Index() == 1 {
			t.Fatal("picked unhealthy replica")
		}
	}
}
