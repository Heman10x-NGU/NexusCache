package consistenthash

import (
	"crypto/md5"
	"fmt"
	"github.com/segmentio/fasthash/fnv1"
	"sort"
	"strconv"
	"sync"
)

type Hash func(data []byte) uint64

type Map struct {
	sync.Mutex
	hash     Hash           // Hash function to use
	replicas int            // Number of virtual nodes per real node
	keys     []int          // Sorted array of hash values (the hash ring)
	hashMap  map[int]string // Map from virtual node hash to real node
}

func New(replicas int, fn Hash) *Map {
	m := &Map{
		replicas: replicas,
		hash:     fn,
		hashMap:  make(map[int]string),
	}
	// If no hash function provided, use default fnv1 algorithm
	if m.hash == nil {
		m.hash = fnv1.HashBytes64
		// m.hash = crc32.ChecksumIEEE
	}
	return m
}

// AddNodes add some nodes to hash
func (m *Map) AddNodes(keys ...string) {
	m.Lock()
	defer m.Unlock()
	//log.Println("In consistenthash.AddNodes, keys =", keys)
	for _, key := range keys {
		// Create virtual nodes for each real node to solve data skew in consistent hashing
		for i := 0; i < m.replicas; i++ {
			// Calculate hash value for virtual node
			hash := int(m.hash([]byte(fmt.Sprintf("%x", md5.Sum([]byte(strconv.Itoa(i)+key))))))
			m.keys = append(m.keys, hash)
			m.hashMap[hash] = key // Record mapping: hashMap[virtual node hash] = real node key
		}
	}
	sort.Ints(m.keys) // Sort the hash ring
	//fmt.Printf("In consistenthash.AddNodes, m.keys = %v, m.hashMap = %v \n", m.keys, m.hashMap)
}

// Get finds the real node that should store the given key and returns its IP address
func (m *Map) Get(key string) string {

	if len(m.keys) == 0 {
		return ""
	}
	hash := int(m.hash([]byte(key)))
	//idx := sort.Search(len(m.keys), func(i int) bool {
	//	return m.keys[i] >= hash
	//})

	// Binary search to find the node
	// Note: j is len(m.keys) not len(m.keys)-1, so when hash > max value on ring,
	// the result will be len(m.keys)
	i, j := 0, len(m.keys)
	for i < j {
		mid := uint((i + j) >> 1)
		if m.keys[mid] >= hash {
			j = int(mid)
		} else if m.keys[mid] < hash {
			i = int(mid) + 1
		}
	}
	idx := i
	// Special case: when hash value exceeds the max node hash on the ring,
	// we need to wrap around to the first node.
	// When this happens with 9 nodes, idx would equal 9,
	// so we use modulo to map idx=9 back to 0 (first node)
	return m.hashMap[m.keys[idx%len(m.keys)]]
}

func (m *Map) Remove(key string) {
	m.Lock()
	defer m.Unlock()
	for i := 0; i < m.replicas; i++ {
		hash := int(m.hash([]byte(fmt.Sprintf("%x", md5.Sum([]byte(strconv.Itoa(i)+key))))))
		idx := sort.SearchInts(m.keys, hash)
		m.keys = append(m.keys[:idx], m.keys[idx+1:]...)
	}
}
