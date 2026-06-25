package loadgen

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	pb "distributed-llm-inference-router/gen"
	"distributed-llm-inference-router/metrics"
)

type Config struct {
	Seed            int64
	TargetRPS       float64
	TotalRequests   int
	OverlapFraction float64
	SharedPrefixLen int
	PromptLenMin    int
	PromptLenMax    int
	MaxNewTokens    int
}

type GeneratedRequest struct {
	ID           string
	PromptTokens []int32
	MaxNewTokens int32
}

func GenerateRequests(cfg Config) []GeneratedRequest {
	rng := rand.New(rand.NewSource(cfg.Seed))

	sharedPrefix := make([]int32, cfg.SharedPrefixLen)
	for i := range sharedPrefix {
		sharedPrefix[i] = int32(rng.Intn(1000) + 1)
	}

	reqs := make([]GeneratedRequest, cfg.TotalRequests)
	for i := range reqs {
		length := cfg.PromptLenMin + rng.Intn(cfg.PromptLenMax-cfg.PromptLenMin+1)
		tokens := make([]int32, length)

		useShared := rng.Float64() < cfg.OverlapFraction
		start := 0
		if useShared && cfg.SharedPrefixLen <= length {
			copy(tokens, sharedPrefix)
			start = cfg.SharedPrefixLen
		}
		for j := start; j < length; j++ {
			tokens[j] = int32(rng.Intn(50000) + 1)
		}
		reqs[i] = GeneratedRequest{
			ID:           fmt.Sprintf("req-%d", i),
			PromptTokens: tokens,
			MaxNewTokens: int32(cfg.MaxNewTokens),
		}
	}
	return reqs
}

type LoadGen struct {
	cfg     Config
	client  pb.InferenceServiceClient
	metrics *metrics.Recorder
}

func New(cfg Config, client pb.InferenceServiceClient, rec *metrics.Recorder) *LoadGen {
	return &LoadGen{cfg: cfg, client: client, metrics: rec}
}

func (lg *LoadGen) Run(ctx context.Context) {
	reqs := GenerateRequests(lg.cfg)

	interval := time.Duration(0)
	if lg.cfg.TargetRPS > 0 {
		interval = time.Duration(float64(time.Second) / lg.cfg.TargetRPS)
	}

	var wg sync.WaitGroup

	for _, req := range reqs {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		r := req
		go func() {
			defer wg.Done()
			start := time.Now()
			resp, err := lg.client.Infer(ctx, &pb.InferRequest{
				RequestId:    r.ID,
				PromptTokens: r.PromptTokens,
				MaxNewTokens: r.MaxNewTokens,
			})
			elapsed := time.Since(start)
			if err == nil {
				lg.metrics.Record("", elapsed, resp.CacheHit)
			}
		}()
		if interval > 0 {
			time.Sleep(interval)
		}
	}
	wg.Wait()
}
