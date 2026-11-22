package catalog

import (
	"container/list"
	"encoding/json"
	"sync"
)

const maxReferenceCacheBytes = 16 << 20

type referenceEntry struct {
	key     string
	payload []byte
}
type referenceCache struct {
	mu                                 sync.Mutex
	items                              map[string]*list.Element
	lru                                *list.List
	bytes                              int
	revision                           int64
	enabled                            bool
	hits, misses, evictions, fallbacks uint64
}
type CacheStats struct {
	Bytes     int    `json:"bytes"`
	Entries   int    `json:"entries"`
	Hits      uint64 `json:"hits"`
	Misses    uint64 `json:"misses"`
	Evictions uint64 `json:"evictions"`
	Fallbacks uint64 `json:"fallbacks"`
}

func newReferenceCache() *referenceCache {
	return &referenceCache{items: map[string]*list.Element{}, lru: list.New(), enabled: true, revision: -1}
}
func (c *referenceCache) get(key string, destination any) bool {
	c.mu.Lock()
	if !c.enabled {
		c.fallbacks++
		c.mu.Unlock()
		return false
	}
	element, ok := c.items[key]
	if ok {
		c.lru.MoveToFront(element)
		payload := append([]byte(nil), element.Value.(*referenceEntry).payload...)
		c.hits++
		c.mu.Unlock()
		return json.Unmarshal(payload, destination) == nil
	}
	c.misses++
	c.mu.Unlock()
	return false
}
func (c *referenceCache) put(key string, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	if len(key)+len(payload) > maxReferenceCacheBytes {
		c.mu.Lock()
		c.fallbacks++
		c.mu.Unlock()
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.enabled {
		c.fallbacks++
		return
	}
	if element, ok := c.items[key]; ok {
		old := element.Value.(*referenceEntry)
		c.bytes -= len(old.key) + len(old.payload)
		c.lru.Remove(element)
		delete(c.items, key)
	}
	entry := &referenceEntry{key: key, payload: payload}
	element := c.lru.PushFront(entry)
	c.items[key] = element
	c.bytes += len(key) + len(payload)
	for c.bytes > maxReferenceCacheBytes {
		last := c.lru.Back()
		victim := last.Value.(*referenceEntry)
		c.bytes -= len(victim.key) + len(victim.payload)
		delete(c.items, victim.key)
		c.lru.Remove(last)
		c.evictions++
	}
}
func (c *referenceCache) setEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = enabled
	if !enabled {
		c.items = map[string]*list.Element{}
		c.lru.Init()
		c.bytes = 0
	}
}
func (c *referenceCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = map[string]*list.Element{}
	c.lru.Init()
	c.bytes = 0
	c.revision = -1
}
func (c *referenceCache) setRevision(revision int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.revision == revision {
		return
	}
	c.items = map[string]*list.Element{}
	c.lru.Init()
	c.bytes = 0
	c.revision = revision
}
func (c *referenceCache) stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{Bytes: c.bytes, Entries: len(c.items), Hits: c.hits, Misses: c.misses, Evictions: c.evictions, Fallbacks: c.fallbacks}
}
