package router

type LeastOutstanding struct{}

func NewLeastOutstanding() *LeastOutstanding { return &LeastOutstanding{} }

func (lo *LeastOutstanding) Pick(_ *Request, replicas []*ReplicaClient) *ReplicaClient {
	healthy := healthyReplicas(replicas)
	if len(healthy) == 0 {
		return nil
	}
	best := healthy[0]
	for _, r := range healthy[1:] {
		if r.Outstanding() < best.Outstanding() {
			best = r
		}
	}
	return best
}
