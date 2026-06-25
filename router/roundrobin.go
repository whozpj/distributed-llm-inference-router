package router

import "sync/atomic"

type RoundRobin struct{ counter atomic.Uint64 }

func NewRoundRobin() *RoundRobin { return &RoundRobin{} }

func (rr *RoundRobin) Pick(_ *Request, replicas []*ReplicaClient) *ReplicaClient {
	healthy := healthyReplicas(replicas)
	if len(healthy) == 0 {
		return nil
	}
	idx := rr.counter.Add(1) - 1
	return healthy[idx%uint64(len(healthy))]
}

func healthyReplicas(replicas []*ReplicaClient) []*ReplicaClient {
	out := make([]*ReplicaClient, 0, len(replicas))
	for _, r := range replicas {
		if r.IsHealthy() {
			out = append(out, r)
		}
	}
	return out
}
