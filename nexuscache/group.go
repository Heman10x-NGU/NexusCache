package nexuscache

import (
	"NexusCache/connect"
	"NexusCache/metrics"
	"NexusCache/singleflight"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pkg/errors"
)

var DefaultExpireTime = 30 * time.Second // Short expiration time for testing

// Group is the core data structure of NexusCache, responsible for user interaction
// and controlling the cache storage and retrieval process.
type Group struct {
	name      string
	getter    Getter // Interface for fetching source data
	mainCache cache  // Local storage for key-value pairs based on consistent hashing
	hotCache  cache  // Storage for hot/frequently accessed data
	peers     connect.PeerPicker
	// use singleflight.Group to make sure that
	// each key is only fetched once
	loader *singleflight.Group // Controls concurrent request deduplication
}

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group) // Global variable that records all created groups
)

func NewGroup(name string, cacheBytes int64, hotcacheBytes int64, getter Getter) *Group {
	if getter == nil {
		panic("nexuscache: getter is nil")
	}
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: cache{cacheBytes: cacheBytes},
		hotCache:  cache{cacheBytes: hotcacheBytes},
		loader:    &singleflight.Group{},
	}
	groups[name] = g
	return g
}

func (g *Group) RegisterPeers(peers connect.PeerPicker) {
	if g.peers != nil {
		panic("nexuscache: peer already registered")
	}
	g.peers = peers
}

func GetGroup(name string) *Group {
	mu.RLock()
	g := groups[name]
	mu.RUnlock()
	return g
}

func (g *Group) Get(key string) (*ByteView, error) {
	start := time.Now()
	defer func() {
		metrics.RecordRequestDuration("get", time.Since(start).Seconds())
	}()

	if key == "" {
		metrics.RecordCacheError("get")
		return &ByteView{}, fmt.Errorf("nexuscache: key is empty")
	}
	if v, ok := g.lookupCache(key); ok {
		log.Println("NexusCache hit")
		metrics.RecordCacheHit("get")
		return v, nil
	}
	log.Println("NexusCache miss, try to add it")
	metrics.RecordCacheMiss("get")
	return g.Load(key)
}

// Load fetches the key from a remote peer or local database if cache miss.
// Load loads key either by invoking the getter locally or by sending it to another machine.
func (g *Group) Load(key string) (value *ByteView, err error) {
	// Wrap the actual load operation with DoOnce to ensure concurrent safety
	view, err := g.loader.DoOnce(key, func() (interface{}, error) {
		if g.peers != nil {
			log.Println("try to search from peers")
			if peer, ok := g.peers.PickPeer(key); ok {
				if value, err = g.getFromPeer(peer, key); err != nil {
					log.Println("nexuscache: get from peer error:", err)
					return nil, err
				}
				return value, nil
			}
		}
		return g.getLocally(key)
	})
	if err == nil {
		return view.(*ByteView), nil
	}
	return
}

func (g *Group) getFromPeer(peer connect.PeerGetter, key string) (*ByteView, error) {
	bytes, err := peer.Get(g.name, key)
	if err != nil {
		return nil, err
	}
	return &ByteView{b: bytes}, nil
}

// getLocally fetches data from the database and adds it to the cache
func (g *Group) getLocally(key string) (*ByteView, error) {
	// Call the getter function stored when creating the Group
	bytes, err := g.getter.Get(key)
	if err != nil {
		return &ByteView{}, err
	}
	value := &ByteView{b: cloneBytes(bytes), e: time.Now().Add(DefaultExpireTime)}
	g.populateCache(key, value)
	return value, nil
}

// populateCache adds the source data to the mainCache
func (g *Group) populateCache(key string, value *ByteView) {
	g.mainCache.add(key, value)
}

func (g *Group) lookupCache(key string) (value *ByteView, ok bool) {
	value, ok = g.mainCache.get(key)
	if ok {
		return
	}
	value, ok = g.hotCache.get(key)
	return
}

func (g *Group) Set(key string, value *ByteView, ishot bool) error {
	start := time.Now()
	defer func() {
		metrics.RecordRequestDuration("set", time.Since(start).Seconds())
	}()

	if key == "" {
		metrics.RecordCacheError("set")
		return errors.New("key is empty")
	}
	if ishot {
		return g.setHotCache(key, value)
	}
	_, err := g.loader.DoOnce(key, func() (interface{}, error) {
		if peer, ok := g.peers.PickPeer(key); ok {
			err := g.setFromPeer(peer, key, value, ishot)
			if err != nil {
				log.Println("nexuscache: set from peer error:", err)
				return nil, err
			}
			return value, nil
		}
		// If !ok, it means the current node is selected
		g.mainCache.add(key, value)
		return value, nil
	})
	return err
}

func (g *Group) setFromPeer(peer connect.PeerGetter, key string, value *ByteView, ishot bool) error {
	return peer.Set(g.name, key, value.ByteSlice(), value.Expire(), ishot)
}

// setHotCache sets a hot/frequently accessed cache entry
func (g *Group) setHotCache(key string, value *ByteView) error {
	if key == "" {
		return errors.New("key is empty")
	}
	g.loader.DoOnce(key, func() (interface{}, error) {
		g.hotCache.add(key, value)
		log.Printf("NexusCache set hot cache %v \n", value.ByteSlice())
		return nil, nil
	})
	return nil
}
