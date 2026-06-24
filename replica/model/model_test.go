package model_test

import (
	"testing"
	"time"

	"distributed-llm-inference-router/replica/model"
)

func TestCostsScaleWithTokens(t *testing.T) {
	m := model.New(model.Config{
		Base:            10 * time.Millisecond,
		PrefillPerToken: 2 * time.Millisecond,
		DecodePerToken:  1 * time.Millisecond,
	})

	c := m.Costs(50, 20)

	if c.Prefill != 100*time.Millisecond {
		t.Fatalf("prefill: got %v, want 100ms", c.Prefill)
	}
	if c.Decode != 20*time.Millisecond {
		t.Fatalf("decode: got %v, want 20ms", c.Decode)
	}
	if c.Total != 130*time.Millisecond {
		t.Fatalf("total: got %v, want 130ms", c.Total)
	}
}

func TestZeroUncachedTokensMeansNoPrefillCost(t *testing.T) {
	m := model.New(model.Config{
		Base:            0,
		PrefillPerToken: 5 * time.Millisecond,
		DecodePerToken:  1 * time.Millisecond,
	})
	c := m.Costs(0, 10)
	if c.Prefill != 0 {
		t.Fatalf("expected zero prefill cost, got %v", c.Prefill)
	}
}
