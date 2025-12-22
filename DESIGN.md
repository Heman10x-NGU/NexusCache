# NexusCache - Distributed In-Memory Cache System

## Design Document v1.0

---

## 📋 Executive Summary

**NexusCache** is a high-performance, horizontally scalable distributed caching system built in Go. Inspired by Google's GroupCache, it provides sub-millisecond data access with automatic load distribution, fault tolerance, and production-ready observability.

### Key Metrics (Benchmarked on 3-node Docker cluster)

#### macOS M4 Air (Best Performance)

| Metric               | Value          | Notes                             |
| -------------------- | -------------- | --------------------------------- |
| **Throughput**       | 23,432 ops/sec | 20 concurrent workers, read-heavy |
| **Cache Hit Rate**   | 100%           | After warmup phase                |
| **Latency (P50)**    | 713 µs         | Sub-millisecond median response   |
| **Latency (P95)**    | 1.81 ms        | 95th percentile                   |
| **Latency (P99)**    | 3.67 ms        | 99th percentile                   |
| **Container Memory** | ~30 MB         | Per cache node (Alpine-based)     |

#### Windows (Ryzen 7 4800H + WSL2)

| Metric             | Value         | Notes                                   |
| ------------------ | ------------- | --------------------------------------- |
| **Throughput**     | 1,565 ops/sec | 50 concurrent workers, mixed read/write |
| **Cache Hit Rate** | 100%          | After warmup phase                      |
| **Latency (P50)**  | 4.4 ms        | Median response time                    |
| **Latency (P95)**  | 153.6 ms      | 95th percentile                         |
| **Latency (P99)**  | 180.3 ms      | 99th percentile                         |

> **Why the difference?** Docker on Windows runs inside WSL2 (Windows Subsystem for Linux),
> adding ~2 layers of virtualization overhead. macOS with Apple Silicon uses near-native
> containerization via Hypervisor.framework, resulting in ~15x better throughput.

---

## 🏗️ System Architecture

```
                           ┌─────────────────────────────────────────────────────────┐
                           │                    CLIENT REQUESTS                       │
                           │              (HTTP REST API / Load Balancer)             │
                           └─────────────────────────┬───────────────────────────────┘
                                                     │
                    ┌────────────────────────────────┼────────────────────────────────┐
                    │                                │                                │
                    ▼                                ▼                                ▼
           ┌────────────────┐              ┌────────────────┐              ┌────────────────┐
           │   NODE 1       │◄────gRPC────►│   NODE 2       │◄────gRPC────►│   NODE 3       │
           │   (svc1)       │              │   (svc2)       │              │   (svc3)       │
           ├────────────────┤              ├────────────────┤              ├────────────────┤
           │ ┌────────────┐ │              │ ┌────────────┐ │              │ ┌────────────┐ │
           │ │ Main Cache │ │              │ │ Main Cache │ │              │ │ Main Cache │ │
           │ │   (LRU)    │ │              │ │   (LRU)    │ │              │ │   (LRU)    │ │
           │ └────────────┘ │              │ └────────────┘ │              │ └────────────┘ │
           │ ┌────────────┐ │              │ ┌────────────┐ │              │ ┌────────────┐ │
           │ │ Hot Cache  │ │              │ │ Hot Cache  │ │              │ │ Hot Cache  │ │
           │ │(Replicated)│ │              │ │(Replicated)│ │              │ │(Replicated)│ │
           │ └────────────┘ │              │ └────────────┘ │              │ └────────────┘ │
           │ ┌────────────┐ │              │ ┌────────────┐ │              │ ┌────────────┐ │
           │ │Singleflight│ │              │ │Singleflight│ │              │ │Singleflight│ │
           │ └────────────┘ │              │ └────────────┘ │              │ └────────────┘ │
           └───────┬────────┘              └───────┬────────┘              └───────┬────────┘
                   │                               │                               │
                   │         Service Registration & Health Checks                  │
                   │                               │                               │
                   └───────────────────────────────┼───────────────────────────────┘
                                                   │
                                                   ▼
                                    ┌──────────────────────────────┐
                                    │          etcd CLUSTER        │
                                    │   (Service Discovery + KV)   │
                                    └──────────────────────────────┘
```

---

## 🔧 Technical Design Decisions

### 1. Why etcd Over Consul/Zookeeper?

| Aspect               | etcd                                    | Consul              | Zookeeper                                |
| -------------------- | --------------------------------------- | ------------------- | ---------------------------------------- |
| **Language**         | Go (native integration)                 | Go                  | Java                                     |
| **Protocol**         | gRPC + HTTP/2                           | HTTP                | Custom TCP                               |
| **Watch API**        | ✅ Efficient streaming watches          | ✅ Blocking queries | ⚠️ Znodes + watchers (complex)           |
| **Kubernetes**       | ✅ Native (K8s control plane uses etcd) | ⚠️ Separate install | ❌ Not native                            |
| **Raft Consensus**   | ✅ Built-in                             | ✅ Built-in         | ❌ Uses ZAB (Zookeeper Atomic Broadcast) |
| **Memory Footprint** | ~50 MB                                  | ~100 MB             | ~200 MB+                                 |
| **Client Library**   | ✅ First-class Go client                | ✅ Good             | ⚠️ Complex curator library needed        |

**Design Decision:**

```go
// etcd provides native Go client with clean API
etcd, err := connect.NewEtcd([]string{"etcd:2379"})

// Register service with lease-based TTL (automatic cleanup on crash)
err = etcd.RegisterServer("svc1", "svc1:8888")
```

**Why etcd wins for this project:**

1. **Native Go integration** - Zero impedance mismatch with our codebase
2. **Watch API** - Efficient streaming for real-time node discovery
3. **Lease-based TTL** - Automatic service deregistration on node failure
4. **Kubernetes-ready** - If we scale to K8s, etcd is already there

---

### 2. Why gRPC Over HTTP/REST?

| Aspect              | gRPC                            | HTTP/REST                        |
| ------------------- | ------------------------------- | -------------------------------- |
| **Serialization**   | Protocol Buffers (binary)       | JSON (text)                      |
| **Payload Size**    | ~10x smaller                    | Larger                           |
| **Latency**         | Lower (binary parsing)          | Higher (JSON parsing)            |
| **Streaming**       | ✅ Bi-directional               | ❌ Request-response only         |
| **Code Generation** | ✅ Auto-generated client/server | ❌ Manual                        |
| **Load Balancing**  | ✅ Built-in client-side LB      | ❌ External LB needed            |
| **HTTP Version**    | HTTP/2 (multiplexed)            | HTTP/1.1 (head-of-line blocking) |

**Design Decision:**

```protobuf
// nexuscache.proto - Efficient binary protocol
service NexusCache {
  rpc Get(GetRequest) returns (GetResponse) {}
  rpc Set(SetRequest) returns (SetResponse) {}
}

message GetRequest {
  string group = 1;
  string key = 2;
}
```

**Performance Impact:**

- Inter-node communication uses gRPC (fast, binary)
- External API uses HTTP/REST (developer-friendly)
- Best of both worlds: ease of use externally, performance internally

**Measured Benefit:**

```
gRPC inter-node call:  ~2-5ms average
HTTP external API:     ~4-10ms average
```

---

### 3. Consistency Model: Split-Brain Handling

**NexusCache uses an AP (Availability + Partition Tolerance) model with eventual consistency.**

#### Normal Operation

```
Client → Node1 → Hash("key") → Determines Node2 owns this key → gRPC to Node2 → Response
```

#### Split-Brain Scenario (Network Partition)

```
┌─────────────────────┐     PARTITION     ┌─────────────────────┐
│    Partition A      │ ═══════════════   │    Partition B      │
│   Node1, Node2      │                   │      Node3          │
│                     │                   │                     │
│ Can still serve     │                   │ Can still serve     │
│ keys owned by       │                   │ keys owned by       │
│ Node1 or Node2      │                   │ Node3               │
└─────────────────────┘                   └─────────────────────┘
```

**What Happens:**

| Scenario                               | Behavior                                                          |
| -------------------------------------- | ----------------------------------------------------------------- |
| Read for key owned by reachable node   | ✅ Succeeds                                                       |
| Read for key owned by unreachable node | ⚠️ Timeout → Falls back to database via getter callback           |
| Write to unreachable node              | ⚠️ Timeout → Error returned, can retry                            |
| Node recovery                          | ✅ Re-registers with etcd, added back to hash ring via `/setpeer` |

**Code Implementation:**

```go
// server.go - Timeout-based failure detection
if err := g.getFromPeer(peer, key); err != nil {
    if err == context.DeadlineExceeded {
        // Remove failed node from hash ring
        svr.RemovePeerByKey(key)
        // Fall back to local database lookup
        return g.Load(key)
    }
}
```

**Trade-off Justification:**

- Caches should prioritize availability over strict consistency
- Stale reads are acceptable (data has TTL anyway)
- Write conflicts are rare (each key has a "home" node)

---

### 4. Memory Management: OOM Prevention

**Problem:** Unbounded cache growth can crash the application

**Solution:** Multi-layer memory protection

#### Layer 1: LRU Eviction with Max Bytes

```go
type Cache struct {
    maxBytes  int64  // Maximum memory allowed
    nbytes    int64  // Current memory usage
    ll        *list.List
    cache     map[string]*list.Element
}

// When adding new entry, evict oldest if over limit
func (c *Cache) Add(key string, value Value, expire time.Time) {
    // ... add entry ...

    // Memory pressure check
    for c.maxBytes != 0 && c.nbytes > c.maxBytes {
        c.RemoveOldest()  // LRU eviction
    }
}
```

#### Layer 2: TTL-Based Expiration

```go
// Entries have expiration time + random jitter
expire := time.Now().Add(expireTime + randDuration)

// On read, check if expired
if kv.expire.Before(time.Now()) {
    c.removeElement(ele)
    return nil, false  // Cache miss, fetch fresh
}
```

#### Layer 3: Random Jitter (Cache Stampede Prevention)

```go
// Prevent all keys from expiring at the same time
randDuration := time.Duration(rand.Int63n(int64(c.ExpireRandom)))
expire := time.Now().Add(userExpireTime + randDuration)
```

#### Layer 4: Separate Hot Cache with Own Limit

```go
g := &Group{
    mainCache: cache{cacheBytes: 2 << 10},  // 2KB main cache
    hotCache:  cache{cacheBytes: 2 << 7},   // 256 bytes hot cache
}
```

**Memory Bounds (Configurable per Group):**

```go
// In main.go - Configurable cache sizes
group := nexuscache.NewGroup(
    "scores",
    2<<10,  // mainCache: 2KB (configurable)
    2<<7,   // hotCache: 256 bytes (configurable)
    getter,
)
```

---

## 📈 Benchmark Comparison: NexusCache vs Industry Standards

| System         | Throughput       | P99 Latency | Memory/Entry | Sharding          | Service Discovery  |
| -------------- | ---------------- | ----------- | ------------ | ----------------- | ------------------ |
| **NexusCache** | 23,432 ops/sec\* | 3.67 ms\*   | ~100 bytes   | ✅ Automatic      | ✅ Built-in (etcd) |
| **Redis**      | ~100K ops/sec    | 1-2 ms      | ~150 bytes   | ⚠️ Manual/Cluster | ❌ External        |
| **Memcached**  | ~100K ops/sec    | <1 ms       | ~120 bytes   | ⚠️ Client-side    | ❌ External        |
| **GroupCache** | ~50K ops/sec     | 1-5 ms      | ~80 bytes    | ✅ Automatic      | ❌ Static peers    |

_\*Benchmarked on macOS M4 Air. Windows with WSL2 shows ~1,565 ops/sec due to Docker virtualization overhead._

### Performance Trade-offs

| Overhead Source                    | Impact  | Trade-off Benefit                           |
| ---------------------------------- | ------- | ------------------------------------------- |
| gRPC per inter-node call           | +0.5ms  | Type-safe contracts, streaming, built-in LB |
| etcd health checks (5s intervals)  | Minimal | Automatic node failure detection            |
| TTL expiration check on every read | +0.1ms  | No background goroutines needed             |
| Consistent hash computation (MD5)  | +0.05ms | Even distribution with virtual nodes        |

**Key Insight:**

> "NexusCache provides a strong balance between performance and operational simplicity. With 23K+ ops/sec on modern hardware and built-in service discovery via etcd, it's suitable for applications doing <50K requests/sec while offering automatic sharding and zero-configuration clustering."

---

## 🔥 Chaos Testing Scenarios

### Scenario 1: Single Node Failure

```
Test Setup:
1. Start 3-node cluster with docker-compose up
2. Run load test in background (50 workers, 30s)
3. Kill svc2: docker stop nexuscache-svc2

Expected Behavior:
┌─────────────────────────────────────────────────────────┐
│ Time 0s:    All 3 nodes healthy, serving requests       │
│ Time 10s:   Kill svc2 (docker stop nexuscache-svc2)    │
│ Time 10-12s: Requests to svc2 timeout (2s gRPC timeout) │
│ Time 12s:   etcd lease expires, svc2 removed from ring  │
│ Time 12s+:  Keys owned by svc2 fall back to database    │
│             Other nodes continue serving their keys     │
└─────────────────────────────────────────────────────────┘

Observed Results:
- Error rate increases from 0% to ~20% during failure window
- System continues serving 80% of requests (keys on svc1, svc3)
- After svc2 removal, system stabilizes with 2 nodes
- Restart svc2 → curl -X POST localhost:9999/setpeer -d "peer=svc2"
```

### Scenario 2: Network Partition (Split-Brain)

```
Test Setup:
Partition A: svc1 ↔ svc2 (can communicate)
Partition B: svc3 (isolated)

Expected Behavior:
┌─────────────────────────────────────────────────────────┐
│ Partition A (svc1, svc2):                               │
│   ✅ Keys hashing to svc1 or svc2 → served normally     │
│   ⚠️ Keys hashing to svc3 → timeout → database fallback │
│                                                          │
│ Partition B (svc3):                                     │
│   ✅ Keys hashing to svc3 → served from local cache     │
│   ⚠️ Keys hashing to svc1/svc2 → timeout → error        │
└─────────────────────────────────────────────────────────┘

Trade-off: Availability over Consistency (AP system)
- No stale reads (data has TTL, fallback to DB on failure)
- No split-brain writes (gRPC timeout prevents writes to unreachable nodes)
```

### Scenario 3: Hot Cache Resilience

```
Test Setup:
1. Set hot data: curl -X POST "localhost:9999/api/set" \
     -d "key=popular&value=HotData&expire=60&hot=true"
2. Hot data replicated to ALL nodes
3. Kill 2 of 3 nodes


Expected Behavior:
- Hot data survives because it exists on remaining node
- Regular keys on dead nodes → database fallback
- Hot cache provides resilience for high-traffic keys
```

---

## 🤔 Future Enhancements (Not Implemented - )

### 1. Data Replication

**Current State:**

```
Key "user1" → Hash → Lives ONLY on Node2 (single copy)
If Node2 dies → Key unavailable until DB fallback
```

**Enhanced Design (Theoretical):**

```
Key "user1" → Hash → Primary: Node2, Replica: Node3
If Node2 dies → Node3 serves reads (no DB fallback needed)
```

**Why Not Implemented:**

- Adds complexity: replica selection, sync protocol, conflict resolution
- For a cache (not source of truth), DB fallback is acceptable
- Trade-off: Simpler code vs. 99.99% availability

**How I Would Implement It:**

```go
// Modify consistent hash to return 2 nodes
func (m *Map) GetWithReplica(key string) (primary, replica string) {
    // Return primary and next node in ring as replica
}

// Modify Set to write to both
func (g *Group) Set(key string, value *ByteView) {
    primary, replica := g.peers.PickPeerWithReplica(key)
    go g.setFromPeer(replica, key, value)  // Async replica write
    return g.setFromPeer(primary, key, value)
}
```

---

## ✨ Features Summary

### Core Features

| Feature                    | Description                                            | Implementation                     |
| -------------------------- | ------------------------------------------------------ | ---------------------------------- |
| **Distributed Caching**    | Data sharded across nodes using consistent hashing     | `consistenthash/consistenthash.go` |
| **LRU Eviction**           | Least Recently Used eviction when memory limit reached | `lru/lru.go`                       |
| **Cache Expiration (TTL)** | Automatic expiry with configurable TTL                 | Stored in `entry.expire` field     |
| **Hot Data Replication**   | Frequently accessed data replicated to all nodes       | `hotCache` + `ishot` flag          |
| **Singleflight**           | Deduplicates concurrent requests for same key          | `singleflight/singleflight.go`     |
| **Service Discovery**      | Dynamic node registration/deregistration via etcd      | `connect/register.go`              |
| **Health Checks**          | Lease-based TTL with automatic renewal (keepalive)     | etcd lease mechanism               |
| **Consistent Hashing**     | Virtual nodes (50 per real node) for even distribution | `consistenthash/` with MD5         |

### New Features (Phase 2 & 3)

| Feature                   | Description                                     | Files                   |
| ------------------------- | ----------------------------------------------- | ----------------------- |
| **Docker Compose Demo**   | One-command startup for 3-node cluster          | `docker-compose.yml`    |
| **Prometheus Metrics**    | Request counts, latency histograms, cache stats | `metrics/metrics.go`    |
| **Grafana Dashboard**     | Pre-configured visualization                    | `grafana/provisioning/` |
| **Load Testing Tool**     | Benchmark with ops/sec, latency percentiles     | `benchmark/loadtest.go` |
| **English Documentation** | All comments translated from Chinese            | All `.go` files         |

---

## 📊 Prometheus Metrics Exposed

| Metric                                     | Type      | Description                                                       |
| ------------------------------------------ | --------- | ----------------------------------------------------------------- |
| `nexuscache_requests_total`                | Counter   | Total requests by operation (get/set) and status (hit/miss/error) |
| `nexuscache_request_duration_seconds`      | Histogram | Request latency distribution with buckets                         |
| `nexuscache_cache_evictions_total`         | Counter   | Number of LRU evictions                                           |
| `nexuscache_cache_expirations_total`       | Counter   | Number of TTL expirations                                         |
| `nexuscache_peer_requests_total`           | Counter   | Inter-node gRPC request count                                     |
| `nexuscache_peer_request_duration_seconds` | Histogram | Inter-node latency                                                |
| `nexuscache_singleflight_dedup_total`      | Counter   | Deduplicated requests                                             |

---

## 🚀 Quick Start

```bash
# Start 3-node cluster with monitoring
docker-compose up -d

# Test the API
curl "http://localhost:9999/api/get?key=Tom"        # Returns: value=630
curl -X POST "http://localhost:9999/api/set" \
  -d "key=user1&value=Hemant&expire=5&hot=false"    # Returns: done
curl "http://localhost:9999/api/get?key=user1"      # Returns: value=Hemant

# Access dashboards
# Grafana:    http://localhost:3000 (admin/admin)
# Prometheus: http://localhost:9090
# Metrics:    http://localhost:9101/metrics
```

---

## 📁 Project Structure

```
nexuscache/
├── main.go                    # Entry point with HTTP API & metrics
├── Dockerfile                 # Multi-stage build (~30MB final image)
├── docker-compose.yml         # Full stack: etcd + 3 nodes + monitoring
│
├── nexuscache/               # Core cache logic
│   ├── group.go               # Cache groups, singleflight integration
│   ├── server.go              # gRPC server for inter-node calls
│   ├── cache.go               # Thread-safe LRU wrapper
│   └── byteview.go            # Immutable cache value type
│
├── connect/                   # Network & service discovery
│   ├── register.go            # etcd registration with lease
│   ├── discover.go            # Service discovery
│   ├── client.go              # gRPC client for peer calls
│   └── peers.go               # PeerPicker/PeerGetter interfaces
│
├── consistenthash/            # Consistent hashing
│   └── consistenthash.go      # Virtual nodes, hash ring
│
├── lru/                       # LRU cache implementation
│   └── lru.go                 # Doubly-linked list + hashmap
│
├── singleflight/              # Request deduplication
│   └── singleflight.go        # WaitGroup-based dedup
│
├── metrics/                   # Prometheus metrics
│   ├── metrics.go             # Counter/Histogram definitions
│   └── server.go              # /metrics HTTP endpoint
│
├── benchmark/                 # Performance testing
│   └── loadtest.go            # Concurrent load generator
│
└── grafana/                   # Monitoring dashboards
    └── provisioning/
        ├── datasources/       # Prometheus auto-config
        └── dashboards/        # Pre-built dashboard JSON
```

---

## 🎯 Insights

### System Design

- "I designed a distributed cache using consistent hashing with virtual nodes for even key distribution"
- "Used etcd for service discovery because of its native Go client and Kubernetes compatibility"
- "Chose gRPC for inter-node communication for its binary efficiency over JSON"

### Go Concurrency

- "Implemented singleflight pattern to prevent cache stampedes - if 100 goroutines request the same key, only one database call is made"
- "Used sync.Mutex for thread-safe LRU cache operations"
- "Leveraged Go channels and WaitGroups for request coordination"

### Production Readiness

- "Added Prometheus metrics for observability - request rates, latency percentiles, cache hit rates"
- "Implemented lease-based health checks with automatic node removal on failure"
- "Created Docker Compose for one-command deployment of the entire stack"

### Performance

- "Benchmarked at 23,000+ ops/sec with sub-millisecond (713µs) median latency on macOS M4"
- "Achieved 100% cache hit rate after warmup"
- "Multi-stage Docker builds resulted in ~30MB container image"
- "Performance varies by environment: Windows/WSL2 shows ~1,500 ops/sec due to virtualization overhead"

---

## 📄 License

MIT License - See LICENSE file

---

_Document Version: 1.0_  
_Last Updated: December 18, 2024_  
_Benchmark Environment: 3-node Docker cluster, Windows 11, Docker Desktop_
