package main

import (
	"NexusCache/connect"
	"NexusCache/metrics"
	"NexusCache/nexuscache"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var store = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

// startAPIServer starts the HTTP API server for cache operations
func startAPIServer(apiAddr string, group *nexuscache.Group, svr *nexuscache.Server) {
	getHandle := func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		view, err := group.Get(key)
		if err != nil {
			if err == context.DeadlineExceeded {
				// If timeout, the remote node is unavailable
				// Remove the node from hash ring and request from database
				svr.RemovePeerByKey(key)
				view, err = group.Load(key)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				value := fmt.Sprintf("value=%v\n", string(view.ByteSlice()))
				w.Write([]byte(value))
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		value := fmt.Sprintf("value=%v\n", string(view.ByteSlice()))
		w.Write([]byte(value))
	}

	setPeerHandle := func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		peer := r.FormValue("peer")
		if peer == "" {
			http.Error(w, "peer is not allow empty!", http.StatusInternalServerError)
			return
		}
		svr.SetPeers(peer)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte(fmt.Sprintf("set peer %v successful\n", peer)))
	}

	setHandle := func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Error ParseForm", http.StatusInternalServerError)
			return
		}
		key := r.FormValue("key")
		value := r.FormValue("value")
		expire := r.FormValue("expire")
		hot := r.FormValue("hot")
		expireTime, err := strconv.Atoi(expire)
		if err != nil {
			w.Write([]byte("Please set expire time correctly, unit: minutes"))
			http.Error(w, "", http.StatusBadRequest)
			return
		}
		if expireTime < 0 || expireTime > 4321 {
			w.Write([]byte("Expire time error, unit is minutes, max 4320 minutes (3 days)"))
			http.Error(w, "", http.StatusBadRequest)
			return
		}
		ishot := false
		if hot == "true" {
			ishot = true
		}
		if hot != "true" && hot != "false" && hot != "" {
			w.Write([]byte("Invalid Param \"hot\" "))
			http.Error(w, "", http.StatusBadRequest)
			return
		}
		exp := time.Duration(expireTime) * time.Minute
		exptime := time.Now().Add(exp)
		byteView := nexuscache.NewByteView([]byte(value), exptime)
		if err := group.Set(key, byteView, ishot); err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte("done\n"))
	}

	http.HandleFunc("/api/get", getHandle)
	http.HandleFunc("/setpeer", setPeerHandle)
	http.HandleFunc("/api/set", setHandle)
	log.Println("frontend server is running at", apiAddr[7:])
	log.Fatal(http.ListenAndServe(apiAddr[7:], nil))
}

func main() {
	var (
		addr           = os.Getenv("IP_ADDRESS")
		svrName        = flag.String("name", "", "server name")
		port           = flag.String("port", "8888", "server port")
		peers          = flag.String("peer", "", "peers name")
		etcdAddr       = flag.String("etcd", "127.0.0.1:2379", "etcd address")
		defaultApiAddr = "http://0.0.0.0:9999"
	)
	flag.Parse()

	if *svrName == "" {
		log.Fatal("--name is required")
	}
	if *peers == "" {
		log.Fatal("--peer is required")
	}
	if !strings.Contains(*peers, *svrName) {
		log.Fatal("--peers must contain " + *svrName)
	}
	if addr == "" {
		log.Fatal("please set env IP_ADDRESS")
	}

	// Start Prometheus metrics server
	metrics.ServeMetrics(":9100")
	log.Println("Metrics server started on :9100/metrics")

	// Create cache group
	group := nexuscache.NewGroup("scores", 2<<10, 2<<7, nexuscache.GetterFunc(
		func(key string) ([]byte, error) {
			log.Printf("Searching \"%v\" from database", key)
			if v, ok := store[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))

	// Create etcd client
	etcd, err := connect.NewEtcd([]string{*etcdAddr})
	if err != nil {
		log.Println("etcd connect err:", err)
		panic(err)
	}

	log.Println("server name:", *svrName)
	address := fmt.Sprintf("%s:%s", addr, *port)
	err = etcd.RegisterServer(*svrName, address)
	if err != nil {
		log.Fatal("register server error:", err)
	}
	log.Println("register server is Done")

	log.Println("grpc server address:", address)
	// Create gRPC Server
	svr := nexuscache.NewServer(*svrName, address, etcd)

	// Add nodes to hash ring
	// Check if other nodes are registered in etcd, wait if not
	peer := strings.Split(*peers, ",")
	if len(peer) != 1 {
		timer := 0
		log.Println("waiting for other servers to register")
		done := make(chan bool, 1)
		go func() {
			for {
				if IfAllRegistered(etcd, peer) {
					break
				}
				time.Sleep(2 * time.Second)
				timer++
				if timer > 30 {
					log.Fatal("other services didn't register, please check and try again later")
				}
			}
			done <- true
		}()
		<-done
	}
	log.Println("other servers are registered")
	svr.SetPeers(peer...)
	// Bind service with group
	group.RegisterPeers(svr)
	// Start API server
	go startAPIServer(defaultApiAddr, group, svr)

	// Start gRPC server
	err = svr.StartServer()
	if err != nil {
		log.Println("grpc server start err:", err)
		panic(err)
	}
}

func IfAllRegistered(etcd *connect.Etcd, peer []string) bool {
	for _, v := range peer {
		resp, err := etcd.EtcdCli.Get(context.Background(), v)
		if err != nil || len(resp.Kvs) == 0 {
			return false
		}
	}
	return true
}
