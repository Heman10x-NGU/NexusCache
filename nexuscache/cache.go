package nexuscache

import (
	"NexusCache/lru"
	"sync"
)

// Concurrent-safe cache wrapper
type cache struct {
	mu         sync.Mutex
	lru        *lru.Cache
	cacheBytes int64
}

// add uses a lock to ensure data consistency, calls the underlying LRU Add method
func (c *cache) add(key string, value *ByteView) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		if lru.DefaultMaxBytes > c.cacheBytes {
			c.lru = lru.New(lru.DefaultMaxBytes, nil)
		} else {
			c.lru = lru.New(c.cacheBytes, nil)
		}
	}
	c.lru.Add(key, value, value.Expire())
}

// get acquires lock and calls the underlying Get
func (c *cache) get(key string) (value *ByteView, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		return
	}
	if v, ok := c.lru.Get(key); ok {
		return v.(*ByteView), ok
	}
	return
}
