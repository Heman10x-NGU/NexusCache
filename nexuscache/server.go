package nexuscache

import (
	"NexusCache/connect"
	"NexusCache/consistenthash"
	pb "NexusCache/nexuscachepb"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// Server handles incoming requests from other nodes
var (
	ErrorServerHasStarted = errors.New("server has started, err:")
	ErrorTcpListen        = errors.New("tcp listen error")
	ErrorRegisterEtcd     = errors.New("register etcd error")
	ErrorGrpcServerStart  = errors.New("start grpc server error")
)

var (
	defaultListenAddr = "0.0.0.0:8888"
)

const (
	defaultReplicas = 50
)

type Server struct {
	pb.UnimplementedNexusCacheServer

	status  bool   // Indicates whether the server is running
	self    string // This node's IP address
	mu      sync.Mutex
	peers   *consistenthash.Map // Consistent hash ring
	etcd    *connect.Etcd
	name    string
	clients map[string]*connect.Client // Map of [node name] to client
}

// NewServer creates a gRPC server and binds it to etcd
func NewServer(serverName, selfAddr string, etcd *connect.Etcd) *Server {

	return &Server{
		self:    selfAddr,
		status:  false,
		peers:   consistenthash.New(defaultReplicas, nil),
		etcd:    etcd,
		clients: make(map[string]*connect.Client),
		name:    serverName,
	}
}

// Get implements the gRPC Get interface - returns cached value when remote node requests it
func (s *Server) Get(ctx context.Context, in *pb.GetRequest) (out *pb.GetResponse, err error) {
	groupName, key := in.GetGroup(), in.GetKey()
	group := GetGroup(groupName)
	bytes, err := group.Get(key)
	if err != nil {
		return nil, err
	}
	out = &pb.GetResponse{
		Value: bytes.ByteSlice(),
	}
	return out, nil
}

// Set implements the gRPC Set interface - sets cache when remote node requests it
func (s *Server) Set(ctx context.Context, in *pb.SetRequest) (out *pb.SetResponse, err error) {
	groupName, key, value, expire := in.GetGroup(), in.GetKey(), in.GetValue(), in.GetExpire()
	ishot := in.GetIshot()
	group := GetGroup(groupName)
	bytes := NewByteView(value, time.Unix(expire, 0))
	out = &pb.SetResponse{
		Ok: false,
	}
	err = group.Set(key, bytes, ishot)
	if err != nil {
		return out, err
	}
	return &pb.SetResponse{Ok: true}, nil
}

func (s *Server) Log(format string, v ...interface{}) {
	log.Printf("[Server %s] %s", s.self, fmt.Sprintf(format, v...))
}

// SetPeers discovers nodes in etcd, adds their IPs to the hash ring, and saves clients for later use
func (s *Server) SetPeers(names ...string) {

	for _, name := range names {
		//log.Printf("debug, In server.SetPeers, name:", name)
		ip, err := connect.GetAddrByName(s.etcd.EtcdCli, name)
		if err != nil {
			log.Printf("SetPeers err : %v", err)
			return
		}
		//log.Printf("debug, In server.SetPeers, ip:", ip)
		addr := strings.Split(ip, ":")[0]
		s.peers.AddNodes(addr)
		s.clients[addr] = &connect.Client{name, s.etcd}
	}
	//log.Println("SetPeers success, s.clients =", s.clients)
}

// PickPeer wraps the consistent hash Get() method to select a node based on the key
// and returns the corresponding RPC client.
func (s *Server) PickPeer(key string) (connect.PeerGetter, bool) {
	//s.mu.Lock()
	//defer s.mu.Unlock()
	if peer := s.peers.Get(key); peer != "" {
		ip := strings.Split(s.self, ":")[0]
		if peer == ip {
			log.Println("ops! peek my self! , i am :", peer)
			return nil, false
		}
		s.Log("Pick peer %s", peer)
		return s.clients[peer], true
	}
	return nil, false
}

// RemovePeerByKey finds and removes the node that stores the given key from the hash ring
func (s *Server) RemovePeerByKey(key string) {
	peer := s.peers.Get(key)
	s.peers.Remove(peer)
	log.Printf("RemovePeer %s", peer)
}

// StartServer 开启grpc服务，并在etcd上注册
func (s *Server) StartServer() error {
	// -----------------Start Server----------------------
	// 1. Set status to true indicating server is running
	// 2. Initialize TCP socket and start listening
	// 3. Register RPC service with gRPC so it can dispatch requests to server
	// ------------------------------------------------
	s.mu.Lock()
	if s.status {
		s.mu.Unlock()
		return ErrorServerHasStarted
	}
	// Start gRPC server
	lis, err := net.Listen("tcp", defaultListenAddr)
	if err != nil {
		log.Println("listen server error:", err)
		return ErrorTcpListen
	}
	grpcServer := grpc.NewServer()
	pb.RegisterNexusCacheServer(grpcServer, s)

	log.Println("start grpc server:", s.self)
	err = grpcServer.Serve(lis)
	if err != nil {
		log.Println(ErrorGrpcServerStart, "err： ", err)
		return ErrorGrpcServerStart
	}
	s.status = true
	s.mu.Unlock()
	return nil
}

var _ connect.PeerPicker = (*Server)(nil)
