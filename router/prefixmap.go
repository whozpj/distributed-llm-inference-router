package router

import (
	"container/list"
	"sync"
	"time"
)

type prefixEntry struct {
	hash       uint64
	replicaIDs []string
	lastSeen   time.Time
}

type PrefixMap struct {
	mu       sync.Mutex
	items    map[uint64]*list.Element
	list     *list.List
	maxItems int
	ttl      time.Duration
}

func NewPrefixMap(maxItems int, ttl time.Duration) *PrefixMap {
	return &PrefixMap{
		items:    make(map[uint64]*list.Element),
		list:     list.New(),
		maxItems: maxItems,
		ttl:      ttl,
	}
}

func (pm *PrefixMap) Lookup(hash uint64) ([]string, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	el, ok := pm.items[hash]
	if !ok {
		return nil, false
	}
	e := el.Value.(*prefixEntry)
	if time.Since(e.lastSeen) > pm.ttl {
		pm.list.Remove(el)
		delete(pm.items, hash)
		return nil, false
	}
	pm.list.MoveToFront(el)
	out := make([]string, len(e.replicaIDs))
	copy(out, e.replicaIDs)
	return out, true
}

func (pm *PrefixMap) Record(hash uint64, replicaID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if el, ok := pm.items[hash]; ok {
		e := el.Value.(*prefixEntry)
		for _, id := range e.replicaIDs {
			if id == replicaID {
				e.lastSeen = time.Now()
				pm.list.MoveToFront(el)
				return
			}
		}
		e.replicaIDs = append(e.replicaIDs, replicaID)
		e.lastSeen = time.Now()
		pm.list.MoveToFront(el)
		return
	}
	if pm.list.Len() >= pm.maxItems {
		back := pm.list.Back()
		if back != nil {
			pm.list.Remove(back)
			delete(pm.items, back.Value.(*prefixEntry).hash)
		}
	}
	e := &prefixEntry{hash: hash, replicaIDs: []string{replicaID}, lastSeen: time.Now()}
	el := pm.list.PushFront(e)
	pm.items[hash] = el
}
