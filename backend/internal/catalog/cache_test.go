package catalog

import (
	"strings"
	"sync"
	"testing"
)

func TestReferenceCacheBoundAndConcurrentRead(t *testing.T) {
	c := newReferenceCache()
	c.put("small", map[string]string{"value": "ok"})
	var got map[string]string
	if !c.get("small", &got) || got["value"] != "ok" {
		t.Fatalf("cache hit got=%v", got)
	}
	c.put("large-a", strings.Repeat("x", maxReferenceCacheBytes/2))
	c.put("large-b", strings.Repeat("y", maxReferenceCacheBytes/2))
	c.put("oversized", strings.Repeat("z", maxReferenceCacheBytes))
	stats := c.stats()
	if stats.Bytes > maxReferenceCacheBytes || stats.Evictions == 0 || stats.Fallbacks == 0 {
		t.Fatalf("cache not bounded: %+v", stats)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); var result map[string]string; _ = c.get("large-b", &result) }()
	}
	wg.Wait()
}
