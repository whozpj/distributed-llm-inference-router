package batcher_test

import (
	"testing"
	"time"

	"distributed-llm-inference-router/replica/batcher"
	"distributed-llm-inference-router/replica/cache"
	"distributed-llm-inference-router/replica/model"
)

func newBatcher(maxBatch int) *batcher.Batcher {
	m := model.New(model.Config{
		Base:            0,
		PrefillPerToken: time.Millisecond,
		DecodePerToken:  time.Millisecond,
	})
	kv := cache.New(100, time.Minute)
	return batcher.New(batcher.Config{
		MaxBatch:     maxBatch,
		TokenBudget:  10_000,
		TickInterval: time.Millisecond,
		PrefixLen:    8,
	}, m, kv)
}

func TestRequestCompletes(t *testing.T) {
	b := newBatcher(4)
	b.Start()
	defer b.Stop()

	ch := make(chan batcher.Result, 1)
	b.Submit(&batcher.Request{
		ID:           "r1",
		PromptTokens: []int32{1, 2, 3, 4},
		MaxNewTokens: 5,
		ResultCh:     ch,
	})

	select {
	case res := <-ch:
		if res.Err != nil {
			t.Fatalf("unexpected error: %v", res.Err)
		}
		if len(res.OutputTokens) != 5 {
			t.Fatalf("got %d output tokens, want 5", len(res.OutputTokens))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request did not complete within 2s")
	}
}

func TestCacheHitOnSecondRequest(t *testing.T) {
	b := newBatcher(4)
	b.Start()
	defer b.Stop()

	tokens := []int32{10, 20, 30, 40, 50, 60, 70, 80}
	send := func() batcher.Result {
		ch := make(chan batcher.Result, 1)
		b.Submit(&batcher.Request{
			ID:           "r",
			PromptTokens: tokens,
			MaxNewTokens: 2,
			ResultCh:     ch,
		})
		return <-ch
	}

	first := send()
	if first.CacheHit {
		t.Fatal("first request should be a cache miss")
	}
	second := send()
	if !second.CacheHit {
		t.Fatal("second identical request should be a cache hit")
	}
	if second.PrefillTokens != 0 {
		t.Fatalf("cache hit should have 0 uncached tokens, got %d", second.PrefillTokens)
	}
}

func TestContinuousAdmission(t *testing.T) {
	// Two requests sent while the batch is running — both should complete.
	b := newBatcher(2)
	b.Start()
	defer b.Stop()

	results := make(chan batcher.Result, 4)
	for i := 0; i < 4; i++ {
		b.Submit(&batcher.Request{
			ID:           "r",
			PromptTokens: []int32{1, 2, 3},
			MaxNewTokens: 3,
			ResultCh:     results,
		})
	}

	for i := 0; i < 4; i++ {
		select {
		case res := <-results:
			if res.Err != nil {
				t.Fatalf("request %d failed: %v", i, res.Err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of 4 requests completed", i)
		}
	}
}
