// Package lru implements an LRU (Least Recently Used) cache algorithm
package lru

import (
	"container/list"
	"math/rand"
	"time"
)

var DefaultMaxBytes int64 = 10
var DefaultExpireRandom time.Duration = 3 * time.Minute

type NowFunc func() time.Time

var nowFunc NowFunc = time.Now

type Cache struct {
	maxBytes  int64                         // Maximum memory allowed
	nbytes    int64                         // Current memory usage
	ll        *list.List                    // Doubly-linked list for LRU ordering
	cache     map[string]*list.Element      // Map storing actual key-value pairs
	OnEvicted func(key string, value Value) // Optional callback when an entry is evicted

	// Now is the Now() function the cache will use to determine
	// the current time which is used to calculate expired values
	// Defaults to time.Now()
	Now NowFunc
	//
	ExpireRandom time.Duration
}

type entry struct {
	key     string
	value   Value
	expire  time.Time // Expiration time
	addTime time.Time // Time when entry was added
}

type Value interface {
	Len() int
}

func New(maxBytes int64, onEvicted func(string, Value)) *Cache {
	return &Cache{
		maxBytes:     maxBytes,
		ll:           list.New(),
		cache:        make(map[string]*list.Element),
		OnEvicted:    onEvicted,
		Now:          nowFunc,
		ExpireRandom: DefaultExpireRandom,
	}
}

func (c *Cache) Len() int {
	return c.ll.Len()
}

// Get retrieves a value from the cache and moves it to the front (most recently used)
func (c *Cache) Get(key string) (value Value, ok bool) {
	// If found in cache, move it to front
	if ele, ok := c.cache[key]; ok {
		// ll.Value is interface{} type that can store any data type
		// Since we stored *entry type, we can assert it back to *entry
		kv := ele.Value.(*entry)
		// If entry has expired, remove it from cache
		if kv.expire.Before(time.Now()) {
			c.removeElement(ele)
			return nil, false
		}
		// If not expired, refresh the expiration time
		expireTime := kv.expire.Sub(kv.addTime)
		kv.expire = time.Now().Add(expireTime)
		kv.addTime = time.Now()
		// With doubly-linked list as queue, front/back is relative - here we define front as most recent
		c.ll.MoveToFront(ele)
		return kv.value, true
	}
	return nil, false
}

func (c *Cache) RemoveOldest() {
	ele := c.ll.Back()
	if ele != nil {
		// Remove this node from the LRU list
		c.ll.Remove(ele)
		kv := ele.Value.(*entry)
		// Delete from the cache map
		delete(c.cache, kv.key)
		c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
		if c.OnEvicted != nil {
			c.OnEvicted(kv.key, kv.value)
		}
	}
}

func (c *Cache) Remove(key string) {
	if ele, ok := c.cache[key]; ok {
		c.removeElement(ele)
	}
}

func (c *Cache) removeElement(ele *list.Element) {
	kv := ele.Value.(*entry)
	delete(c.cache, kv.key)
	c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
	if c.OnEvicted != nil {
		c.OnEvicted(kv.key, kv.value)
	}
}

func (c *Cache) Add(key string, value Value, expire time.Time) {
	// randDuration adds randomness to expiration time to prevent cache stampede
	randDuration := time.Duration(rand.Int63n(int64(c.ExpireRandom)))

	if ele, ok := c.cache[key]; ok {
		// If key already exists, update the value
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		c.nbytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
		kv.expire = expire.Add(randDuration)
	} else {
		newEle := c.ll.PushFront(&entry{key, value, expire.Add(randDuration), time.Now()})
		c.cache[key] = newEle
		c.nbytes += int64(len(key)) + int64(value.Len())
	}
	for c.maxBytes != 0 && c.maxBytes < c.nbytes {
		c.RemoveOldest()
	}
}
