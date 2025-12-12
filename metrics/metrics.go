// Package metrics provides Prometheus metrics for NexusCache observability
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestsTotal counts total cache requests by type and status
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "nexuscache",
			Name:      "requests_total",
			Help:      "Total number of cache requests",
		},
		[]string{"operation", "status"},
	)

	// RequestDuration measures request latency in seconds
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "nexuscache",
			Name:      "request_duration_seconds",
			Help:      "Request duration in seconds",
			Buckets:   []float64{.0001, .0005, .001, .005, .01, .025, .05, .1, .25, .5, 1},
		},
		[]string{"operation"},
	)

	// CacheSize tracks the current cache size in bytes
	CacheSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "nexuscache",
			Name:      "cache_size_bytes",
			Help:      "Current cache size in bytes",
		},
		[]string{"cache_type"},
	)

	// CacheItems tracks the number of items in cache
	CacheItems = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "nexuscache",
			Name:      "cache_items",
			Help:      "Number of items in the cache",
		},
		[]string{"cache_type"},
	)

	// PeerRequestsTotal counts requests to peer nodes
	PeerRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "nexuscache",
			Name:      "peer_requests_total",
			Help:      "Total number of requests to peer nodes",
		},
		[]string{"peer", "status"},
	)

	// PeerRequestDuration measures latency for peer requests
	PeerRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "nexuscache",
			Name:      "peer_request_duration_seconds",
			Help:      "Duration of peer requests in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5},
		},
		[]string{"peer"},
	)

	// CacheEvictionsTotal counts cache evictions
	CacheEvictionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "nexuscache",
			Name:      "cache_evictions_total",
			Help:      "Total number of cache evictions",
		},
	)

	// CacheExpirations counts cache expirations (TTL)
	CacheExpirationsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "nexuscache",
			Name:      "cache_expirations_total",
			Help:      "Total number of cache expirations due to TTL",
		},
	)

	// SingleflightDedup counts deduplicated requests
	SingleflightDedupTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "nexuscache",
			Name:      "singleflight_dedup_total",
			Help:      "Total number of deduplicated requests via singleflight",
		},
	)
)

// RecordCacheHit records a cache hit metric
func RecordCacheHit(operation string) {
	RequestsTotal.WithLabelValues(operation, "hit").Inc()
}

// RecordCacheMiss records a cache miss metric
func RecordCacheMiss(operation string) {
	RequestsTotal.WithLabelValues(operation, "miss").Inc()
}

// RecordCacheError records a cache error metric
func RecordCacheError(operation string) {
	RequestsTotal.WithLabelValues(operation, "error").Inc()
}

// RecordRequestDuration records request latency
func RecordRequestDuration(operation string, seconds float64) {
	RequestDuration.WithLabelValues(operation).Observe(seconds)
}

// RecordPeerRequest records a request to a peer node
func RecordPeerRequest(peer, status string, durationSeconds float64) {
	PeerRequestsTotal.WithLabelValues(peer, status).Inc()
	PeerRequestDuration.WithLabelValues(peer).Observe(durationSeconds)
}

// UpdateCacheStats updates cache size and item count metrics
func UpdateCacheStats(cacheType string, sizeBytes float64, itemCount float64) {
	CacheSize.WithLabelValues(cacheType).Set(sizeBytes)
	CacheItems.WithLabelValues(cacheType).Set(itemCount)
}
