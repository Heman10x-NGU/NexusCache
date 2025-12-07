package connect

import (
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"golang.org/x/net/context"
	"log"
	"time"
)

var (
	//defaultEndpoints    = []string{"10.0.0.166:2379"}
	defaultTimeout      = 3 * time.Second
	defaultLeaseExpTime = 10
)

type Etcd struct {
	EtcdCli *clientv3.Client
	leaseId clientv3.LeaseID // 租约ID
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewEtcd(endpoints []string) (*Etcd, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: defaultTimeout,
	})
	if err != nil {
		log.Println("create etcd register err:", err)
		return nil, err
	}
	// defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	svr := &Etcd{
		EtcdCli: client,
		ctx:     ctx,
		cancel:  cancel,
	}
	return svr, nil
}

// CreateLease creates a lease for the node registered in etcd.
// Since the server cannot guarantee it's always available (may crash), the lease with etcd has a TTL.
// Once the lease expires, the server's address info stored in etcd will disappear.
// On the other hand, if the server is running normally, the address info must exist in etcd,
// so we send heartbeats to renew the lease when it's about to expire.
func (s *Etcd) CreateLease(expireTime int) error {

	res, err := s.EtcdCli.Grant(s.ctx, int64(expireTime))
	if err != nil {
		return err
	}
	s.leaseId = res.ID
	log.Println("create lease success:", s.leaseId)
	return nil

}

// BindLease binds the service to its corresponding lease
func (s *Etcd) BindLease(server string, addr string) error {

	_, err := s.EtcdCli.Put(s.ctx, server, addr, clientv3.WithLease(s.leaseId))
	if err != nil {
		return err
	}
	return nil
}

func (s *Etcd) KeepAlive() error {
	log.Println("keep alive start")
	log.Println("s.leaseId:", s.leaseId)
	KeepRespChan, err := s.EtcdCli.KeepAlive(context.Background(), s.leaseId)
	if err != nil {
		log.Println("keep alive err:", err)
		return err
	}
	go func() {
		for {
			for KeepResp := range KeepRespChan {
				if KeepResp == nil {
					fmt.Println("keep alive is stop")
					return
				} else {
					fmt.Println("keep alive is ok")
				}
			}
			time.Sleep(5 * time.Second)
		}
	}()
	return nil
}

//func (s *Etcd) KeepAlive() error {
//	log.Println("keep alive start")
//	log.Println("s.leaseId:", s.leaseId)
//	KeepRespChan, err := s.EtcdCli.KeepAlive(context.Background(), s.leaseId)
//	if err != nil {
//		log.Println("keep alive err:", err)
//		return err
//	}
//	go func() {
//		for {
//			select {
//			case KeepResp := <-KeepRespChan:
//				if KeepResp == nil {
//					fmt.Println("keep alive is stop")
//					return
//				} else {
//					fmt.Println("keep alive is ok")
//				}
//			}
//			time.Sleep(7 * time.Second)
//		}
//	}()
//	return nil
//}

// RegisterServer stores serviceName as key and addr as value in etcd
func (s *Etcd) RegisterServer(serviceName, addr string) error {
	// Create lease
	err := s.CreateLease(defaultLeaseExpTime)
	if err != nil {
		log.Println("create etcd register err:", err)
		return err
	}
	// Bind lease to service
	err = s.BindLease(serviceName, addr)
	if err != nil {
		log.Println("bind etcd register err:", err)
		return err
	}
	// Start heartbeat/keepalive
	err = s.KeepAlive()
	if err != nil {
		log.Println("keep alive register err:", err)
		return err
	}
	// Register service for service discovery
	em, err := endpoints.NewManager(s.EtcdCli, serviceName)
	if err != nil {
		log.Println("create etcd register err:", err)
		return err
	}
	return em.AddEndpoint(s.ctx, serviceName+"/"+addr, endpoints.Endpoint{Addr: addr}, clientv3.WithLease(s.leaseId))
}
