package cache_test

import (
	"testing"
	"time"

	"distributed-llm-inference-router/replica/cache"
)

func TestMissOnEmpty(t *testing.T) {
	c := cache.New(10, time.Minute)
	_, ok := c.Lookup(12345)
	if ok {
		t.Fatal("expected miss on empty cache")
	}
}

func TestStoreAndLookup(t *testing.T) {
	c := cache.New(10, time.Minute)
	c.Store(42, 16)
	n, ok := c.Lookup(42)
	if !ok {
		t.Fatal("expected hit after store")
	}
	if n != 16 {
		t.Fatalf("got prefixLen %d, want 16", n)
	}
}

func TestLRUEviction(t *testing.T) {
	c := cache.New(2, time.Minute)
	c.Store(1, 10)
	c.Store(2, 20)
	c.Store(3, 30) // evicts key 1 (LRU)

	if _, ok := c.Lookup(1); ok {
		t.Fatal("key 1 should have been evicted")
	}
	if _, ok := c.Lookup(2); !ok {
		t.Fatal("key 2 should still be present")
	}
	if _, ok := c.Lookup(3); !ok {
		t.Fatal("key 3 should be present")
	}
}

func TestTTLExpiry(t *testing.T) {
	c := cache.New(10, 10*time.Millisecond)
	c.Store(99, 8)
	time.Sleep(20 * time.Millisecond)
	_, ok := c.Lookup(99)
	if ok {
		t.Fatal("entry should have expired")
	}
}
