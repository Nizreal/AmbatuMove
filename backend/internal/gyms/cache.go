package gyms

import (
	"container/list"
	"encoding/json"
	"sync"
	"time"
)

const (
	gymCacheMaxBytes   = 8 << 20
	gymCacheMaxEntries = 128
	gymCacheTTL        = 120 * time.Second
)

type gymCacheEntry struct {
	key     string
	payload []byte
	expires time.Time
}
type gymReadCache struct {
	mu                                                sync.Mutex
	entries                                           map[string]*list.Element
	lru                                               *list.List
	size                                              int
	revision                                          int64
	enabled                                           bool
	hits, misses, evictions, invalidations, fallbacks uint64
}
type CacheStats struct {
	Entries       int    `json:"entries"`
	Bytes         int    `json:"bytes"`
	Hits          uint64 `json:"hits"`
	Misses        uint64 `json:"misses"`
	Evictions     uint64 `json:"evictions"`
	Invalidations uint64 `json:"invalidations"`
	Fallbacks     uint64 `json:"fallbacks"`
}

func newGymReadCache() *gymReadCache {
	return &gymReadCache{entries: map[string]*list.Element{}, lru: list.New(), enabled: true, revision: -1}
}
func (c *gymReadCache) get(key string, dest any) bool {
	c.mu.Lock()
	if !c.enabled {
		c.fallbacks++
		c.mu.Unlock()
		return false
	}
	el, ok := c.entries[key]
	if !ok {
		c.misses++
		c.mu.Unlock()
		return false
	}
	entry := el.Value.(*gymCacheEntry)
	if time.Now().After(entry.expires) {
		c.remove(el)
		c.misses++
		c.mu.Unlock()
		return false
	}
	c.lru.MoveToFront(el)
	payload := append([]byte(nil), entry.payload...)
	c.hits++
	c.mu.Unlock()
	return json.Unmarshal(payload, dest) == nil
}
func (c *gymReadCache) put(key string, value any) {
	if len(key) > 2048 {
		return
	}
	payload, err := json.Marshal(value)
	if err != nil || len(key)+len(payload) > gymCacheMaxBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.enabled {
		c.fallbacks++
		return
	}
	if old, ok := c.entries[key]; ok {
		c.remove(old)
	}
	el := c.lru.PushFront(&gymCacheEntry{key: key, payload: payload, expires: time.Now().Add(gymCacheTTL)})
	c.entries[key] = el
	c.size += len(key) + len(payload)
	for c.size > gymCacheMaxBytes || len(c.entries) > gymCacheMaxEntries {
		c.remove(c.lru.Back())
		c.evictions++
	}
}
func (c *gymReadCache) remove(el *list.Element) {
	if el == nil {
		return
	}
	entry := el.Value.(*gymCacheEntry)
	delete(c.entries, entry.key)
	c.size -= len(entry.key) + len(entry.payload)
	c.lru.Remove(el)
}
func (c *gymReadCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) > 0 {
		c.invalidations++
	}
	c.entries = map[string]*list.Element{}
	c.lru.Init()
	c.size = 0
	c.revision = -1
}
func (c *gymReadCache) setRevision(revision int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.revision == revision {
		return
	}
	c.entries = map[string]*list.Element{}
	c.lru.Init()
	c.size = 0
	c.revision = revision
}
func (c *gymReadCache) stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{Entries: len(c.entries), Bytes: c.size, Hits: c.hits, Misses: c.misses, Evictions: c.evictions, Invalidations: c.invalidations, Fallbacks: c.fallbacks}
}
func (c *gymReadCache) setEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = enabled
	if !enabled {
		c.entries = map[string]*list.Element{}
		c.lru.Init()
		c.size = 0
	}
}
