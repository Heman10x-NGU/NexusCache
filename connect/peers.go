package connect

import "time"

// Package connect provides RPC communication functionality between nodes

// PeerPicker defines the ability to pick a distributed node (implemented by Server)
type PeerPicker interface {
	PickPeer(key string) (peer PeerGetter, ok bool)
}

// PeerGetter defines the ability to fetch cache from a remote node (implemented by Client)
// In the connect.client package, the Client struct has Get and Set methods below,
// satisfying this interface, so it can be used as a PeerGetter
type PeerGetter interface {
	Get(group string, key string) ([]byte, error)
	Set(group string, key string, value []byte, expire time.Time, ishot bool) error
}
