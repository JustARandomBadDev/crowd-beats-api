package cache

import (
	"sync"
	"time"

	"crowdbeats/internal/domain/spotify"
)

type SearchCache struct {
	ttl   time.Duration
	mu    sync.Mutex
	items map[string]cacheEntry
}

type cacheEntry struct {
	ExpiresAt time.Time
	Items     []spotify.Track
}

func NewSearchCache(ttl time.Duration) *SearchCache {
	return &SearchCache{ttl: ttl, items: make(map[string]cacheEntry)}
}

func (c *SearchCache) Get(key string) ([]spotify.Track, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if !ok || time.Now().UTC().After(entry.ExpiresAt) {
		if ok {
			delete(c.items, key)
		}
		return nil, false
	}
	return entry.Items, true
}

func (c *SearchCache) Set(key string, items []spotify.Track) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = cacheEntry{
		ExpiresAt: time.Now().UTC().Add(c.ttl),
		Items:     items,
	}
}
