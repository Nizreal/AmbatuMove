package gyms

import (
	"fmt"
	"testing"
)

func TestGymCacheBoundsEntriesAndInvalidates(t *testing.T) {
	c := newGymReadCache()
	for i := 0; i < gymCacheMaxEntries+10; i++ {
		c.put(fmt.Sprintf("gym:%d", i), map[string]string{"id": fmt.Sprint(i)})
	}
	stats := c.stats()
	if stats.Entries > gymCacheMaxEntries || stats.Evictions == 0 {
		t.Fatalf("entry bound not enforced: %+v", stats)
	}
	c.invalidate()
	stats = c.stats()
	if stats.Entries != 0 || stats.Invalidations != 1 {
		t.Fatalf("cache invalidation failed: %+v", stats)
	}
}
