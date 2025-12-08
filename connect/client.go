package connect

import (
	pb "NexusCache/nexuscachepb"
	"context"
	"fmt"
	"log"
	"time"
)

// Package connect provides gRPC client functionality for calling remote nodes' Get and Set methods

type Client struct {
	Name string
	Etcd *Etcd
}

func newClient(name string, etcd *Etcd) *Client {
	return &Client{name, etcd}
}

func (c *Client) Get(group string, key string) ([]byte, error) {

	// Use etcd for service discovery to get grpc connection
	conn, err := DialPeer(c.Etcd.EtcdCli, c.Name)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Create gRPC client and call remote peer's Get method
	grpcClient := pb.NewNexusCacheClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := grpcClient.Get(ctx, &pb.GetRequest{
		Group: group,
		Key:   key,
	})
	if err != nil {
		return nil, fmt.Errorf("could not get %s/%s from peer %s", group, key, c.Name)
	}
	log.Println("In client.Get, grpcClient.Get Done, resp :", resp)
	return resp.GetValue(), nil
}

func (c *Client) Set(group string, key string, value []byte, expire time.Time, ishot bool) error {

	// Use etcd for service discovery to get grpc connection
	conn, err := DialPeer(c.Etcd.EtcdCli, c.Name)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Create gRPC client and call remote peer's Set method
	grpcClient := pb.NewNexusCacheClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := grpcClient.Set(ctx, &pb.SetRequest{
		Group:  group,
		Key:    key,
		Value:  value,
		Expire: expire.Unix(),
		Ishot:  ishot,
	})
	if err != nil {
		log.Println("grpcClient.Set Error:", err)
		return err
	}
	if !resp.GetOk() {
		return fmt.Errorf("grpcClient.Set Failed !")
	}
	return nil
}

// Verify that Client implements the PeerGetter interface
var _ PeerGetter = (*Client)(nil)
