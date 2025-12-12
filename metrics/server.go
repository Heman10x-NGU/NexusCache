// Package metrics provides Prometheus metrics HTTP endpoint
package metrics

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ServeMetrics starts an HTTP server to expose Prometheus metrics
// on the specified address (e.g., ":9100")
func ServeMetrics(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	
	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("Metrics server starting on %s/metrics", addr)
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()
}
