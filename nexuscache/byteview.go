package nexuscache

import "time"

// A ByteView holds an immutable view of bytes.
// ByteView represents a cache value and is the storage unit of NexusCache.
// It implements the lru.Value interface, allowing direct storage in the LRU cache.
type ByteView struct {
	b []byte
	e time.Time
}

func (v *ByteView) Len() int {
	return len(v.b)
}

// Returns the expire time associated with this view
func (v ByteView) Expire() time.Time {
	return v.e
}

func (v *ByteView) ByteSlice() []byte {
	return cloneBytes(v.b)
}

func (v *ByteView) String() string {
	return string(v.b)
}

func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

func NewByteView(b []byte, e time.Time) *ByteView {
	return &ByteView{b: b, e: e}
}
