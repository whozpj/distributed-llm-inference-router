package router

type PrefixAwareConfig struct {
	PrefixLen        int
	LocalityWeight   float64
	HotspotThreshold int64
}

type PrefixAware struct {
	cfg      PrefixAwareConfig
	pm       *PrefixMap
	fallback Policy
}

func NewPrefixAware(cfg PrefixAwareConfig, pm *PrefixMap, fallback Policy) *PrefixAware {
	return &PrefixAware{cfg: cfg, pm: pm, fallback: fallback}
}

func (pa *PrefixAware) Pick(req *Request, replicas []*ReplicaClient) *ReplicaClient {
	healthy := healthyReplicas(replicas)
	if len(healthy) == 0 {
		return nil
	}

	hash := HashPrefix(req.PromptTokens, pa.cfg.PrefixLen)
	hotIDs, ok := pa.pm.Lookup(hash)
	if !ok {
		return pa.fallback.Pick(req, replicas)
	}

	hotSet := make(map[string]bool, len(hotIDs))
	for _, id := range hotIDs {
		hotSet[id] = true
	}

	var best *ReplicaClient
	bestScore := float64(-1)
	for _, r := range healthy {
		if !hotSet[r.ID()] {
			continue
		}
		if r.Outstanding() > pa.cfg.HotspotThreshold {
			continue
		}
		score := float64(r.Outstanding()) / (1 + pa.cfg.LocalityWeight)
		if best == nil || score < bestScore {
			best = r
			bestScore = score
		}
	}
	if best == nil {
		return pa.fallback.Pick(req, replicas)
	}

	pa.pm.Record(hash, best.ID())
	return best
}
