package cache

import (
	"container/list"
	"sync"
	"time"
)

type entry struct {
	hash      uint64
	prefixLen int
	storedAt  time.Time
}

type KVCache struct {
	mu       sync.Mutex
	items    map[uint64]*list.Element
	list     *list.List
	maxItems int
	ttl      time.Duration
}

func New(maxItems int, ttl time.Duration) *KVCache {
	return &KVCache{
		items:    make(map[uint64]*list.Element),
		list:     list.New(),
		maxItems: maxItems,
		ttl:      ttl,
	}
}

func (c *KVCache) Lookup(hash uint64) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[hash]
	if !ok {
		return 0, false
	}
	e := el.Value.(*entry)
	if time.Since(e.storedAt) > c.ttl {
		c.list.Remove(el)
		delete(c.items, hash)
		return 0, false
	}
	c.list.MoveToFront(el)
	return e.prefixLen, true
}

func (c *KVCache) Store(hash uint64, prefixLen int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[hash]; ok {
		c.list.MoveToFront(el)
		el.Value.(*entry).storedAt = time.Now()
		el.Value.(*entry).prefixLen = prefixLen
		return
	}
	if c.list.Len() >= c.maxItems {
		back := c.list.Back()
		if back != nil {
			c.list.Remove(back)
			delete(c.items, back.Value.(*entry).hash)
		}
	}
	e := &entry{hash: hash, prefixLen: prefixLen, storedAt: time.Now()}
	el := c.list.PushFront(e)
	c.items[hash] = el
}
