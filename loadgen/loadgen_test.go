package loadgen_test

import (
	"math/rand"
	"testing"

	"distributed-llm-inference-router/loadgen"
)

func TestRequestsAreReproducible(t *testing.T) {
	cfg := loadgen.Config{
		Seed:            42,
		TotalRequests:   10,
		OverlapFraction: 0.5,
		SharedPrefixLen: 8,
		PromptLenMin:    10,
		PromptLenMax:    20,
		MaxNewTokens:    5,
	}
	reqs1 := loadgen.GenerateRequests(cfg)
	reqs2 := loadgen.GenerateRequests(cfg)

	if len(reqs1) != 10 {
		t.Fatalf("got %d requests, want 10", len(reqs1))
	}
	for i := range reqs1 {
		if len(reqs1[i].PromptTokens) != len(reqs2[i].PromptTokens) {
			t.Fatalf("request %d: length mismatch", i)
		}
		for j, tok := range reqs1[i].PromptTokens {
			if tok != reqs2[i].PromptTokens[j] {
				t.Fatalf("request %d token %d: %d != %d", i, j, tok, reqs2[i].PromptTokens[j])
			}
		}
	}
	_ = rand.New // just ensuring import is used
}

func TestOverlapFractionProducesSharedPrefixes(t *testing.T) {
	cfg := loadgen.Config{
		Seed:            1,
		TotalRequests:   100,
		OverlapFraction: 1.0,
		SharedPrefixLen: 8,
		PromptLenMin:    10,
		PromptLenMax:    20,
		MaxNewTokens:    5,
	}
	reqs := loadgen.GenerateRequests(cfg)
	prefix := reqs[0].PromptTokens[:8]
	for i, r := range reqs {
		for j, tok := range prefix {
			if r.PromptTokens[j] != tok {
				t.Fatalf("request %d missing shared prefix at token %d", i, j)
			}
		}
	}
}

func TestMultiPrefixProducesDifferentPrefixes(t *testing.T) {
	cfg := loadgen.Config{
		Seed:            1,
		TotalRequests:   300,
		OverlapFraction: 1.0,
		SharedPrefixLen: 4,
		NumPrefixes:     3,
		PromptLenMin:    10,
		PromptLenMax:    10,
		MaxNewTokens:    1,
	}
	reqs := loadgen.GenerateRequests(cfg)
	if len(reqs) != 300 {
		t.Fatalf("got %d requests, want 300", len(reqs))
	}
	// With 3 distinct prefixes and OverlapFraction=1.0, not all requests
	// should start with the same 4 tokens.
	first := make([]int32, 4)
	copy(first, reqs[0].PromptTokens[:4])
	allSame := true
	for _, r := range reqs[1:] {
		diff := false
		for j, tok := range r.PromptTokens[:4] {
			if tok != first[j] {
				diff = true
				break
			}
		}
		if diff {
			allSame = false
			break
		}
	}
	if allSame {
		t.Fatal("all 300 requests have identical prefix — NumPrefixes=3 not working")
	}
}
