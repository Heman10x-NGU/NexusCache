// Package main provides a load testing tool for NexusCache benchmark
package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Result stores the result of a single request
type Result struct {
	Duration time.Duration
	Error    error
	IsHit    bool
}

// Stats holds aggregated statistics
type Stats struct {
	TotalRequests int64
	SuccessCount  int64
	ErrorCount    int64
	CacheHits     int64
	CacheMisses   int64
	TotalDuration time.Duration
	Latencies     []time.Duration
	StartTime     time.Time
	EndTime       time.Time
}

func main() {
	// Command line flags
	baseURL := flag.String("url", "http://localhost:9999", "Base URL of the cache server")
	duration := flag.Duration("duration", 30*time.Second, "Duration of the test")
	concurrency := flag.Int("concurrency", 50, "Number of concurrent workers")
	keyCount := flag.Int("keys", 100, "Number of unique keys to use")
	readRatio := flag.Float64("read-ratio", 0.8, "Ratio of read operations (0.0-1.0)")
	warmup := flag.Duration("warmup", 5*time.Second, "Warmup duration before measurements")

	flag.Parse()

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║           NexusCache Load Test & Benchmark                   ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("Configuration:\n")
	fmt.Printf("  Target URL:    %s\n", *baseURL)
	fmt.Printf("  Duration:      %v\n", *duration)
	fmt.Printf("  Concurrency:   %d workers\n", *concurrency)
	fmt.Printf("  Key Space:     %d keys\n", *keyCount)
	fmt.Printf("  Read Ratio:    %.0f%% reads, %.0f%% writes\n", *readRatio*100, (1-*readRatio)*100)
	fmt.Println()

	// Warmup phase - populate cache with initial data
	fmt.Printf("Warmup phase (%v): Populating cache...\n", *warmup)
	populateCache(*baseURL, *keyCount)
	time.Sleep(*warmup)

	// Run the benchmark
	fmt.Printf("Running benchmark for %v with %d workers...\n\n", *duration, *concurrency)

	stats := runBenchmark(*baseURL, *duration, *concurrency, *keyCount, *readRatio)

	// Print results
	printResults(stats)
}

func populateCache(baseURL string, keyCount int) {
	client := &http.Client{Timeout: 5 * time.Second}

	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		value := fmt.Sprintf("benchmark_value_%d_%s", i, randomString(50))

		data := url.Values{}
		data.Set("key", key)
		data.Set("value", value)
		data.Set("expire", "60") // 60 minutes
		data.Set("hot", "false")

		resp, err := client.Post(
			baseURL+"/api/set",
			"application/x-www-form-urlencoded",
			strings.NewReader(data.Encode()),
		)
		if err != nil {
			continue
		}
		resp.Body.Close()
	}
}

func runBenchmark(baseURL string, duration time.Duration, concurrency, keyCount int, readRatio float64) *Stats {
	stats := &Stats{
		Latencies: make([]time.Duration, 0, 100000),
		StartTime: time.Now(),
	}

	var (
		totalReqs    int64
		successCount int64
		errorCount   int64
		cacheHits    int64
		cacheMisses  int64
		latencyMu    sync.Mutex
	)

	// Create worker pool
	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        concurrency * 2,
			MaxIdleConnsPerHost: concurrency * 2,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			for {
				select {
				case <-stopChan:
					return
				default:
				}

				// Decide read or write
				isRead := rng.Float64() < readRatio
				key := fmt.Sprintf("bench_key_%d", rng.Intn(keyCount))

				start := time.Now()
				var err error
				var isHit bool

				if isRead {
					isHit, err = doGet(client, baseURL, key)
				} else {
					err = doSet(client, baseURL, key, randomString(100))
				}

				elapsed := time.Since(start)

				atomic.AddInt64(&totalReqs, 1)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
					if isRead {
						if isHit {
							atomic.AddInt64(&cacheHits, 1)
						} else {
							atomic.AddInt64(&cacheMisses, 1)
						}
					}
				}

				// Record latency (with sampling to avoid memory issues)
				if rng.Float32() < 0.1 { // Sample 10% of requests
					latencyMu.Lock()
					stats.Latencies = append(stats.Latencies, elapsed)
					latencyMu.Unlock()
				}
			}
		}(i)
	}

	// Wait for duration
	time.Sleep(duration)
	close(stopChan)
	wg.Wait()

	stats.EndTime = time.Now()
	stats.TotalRequests = totalReqs
	stats.SuccessCount = successCount
	stats.ErrorCount = errorCount
	stats.CacheHits = cacheHits
	stats.CacheMisses = cacheMisses
	stats.TotalDuration = stats.EndTime.Sub(stats.StartTime)

	return stats
}

func doGet(client *http.Client, baseURL, key string) (bool, error) {
	resp, err := client.Get(fmt.Sprintf("%s/api/get?key=%s", baseURL, key))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("status %d", resp.StatusCode)
	}

	// Check if it was a cache hit (contains "value=")
	isHit := strings.Contains(string(body), "value=")
	return isHit, nil
}

func doSet(client *http.Client, baseURL, key, value string) error {
	data := url.Values{}
	data.Set("key", key)
	data.Set("value", value)
	data.Set("expire", "60")
	data.Set("hot", "false")

	resp, err := client.Post(
		baseURL+"/api/set",
		"application/x-www-form-urlencoded",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func printResults(stats *Stats) {
	// Sort latencies for percentile calculation
	sort.Slice(stats.Latencies, func(i, j int) bool {
		return stats.Latencies[i] < stats.Latencies[j]
	})

	// Calculate percentiles
	p50 := percentile(stats.Latencies, 50)
	p95 := percentile(stats.Latencies, 95)
	p99 := percentile(stats.Latencies, 99)

	// Calculate ops/sec
	opsPerSec := float64(stats.TotalRequests) / stats.TotalDuration.Seconds()

	// Calculate cache hit rate
	totalCacheOps := stats.CacheHits + stats.CacheMisses
	hitRate := float64(0)
	if totalCacheOps > 0 {
		hitRate = float64(stats.CacheHits) / float64(totalCacheOps) * 100
	}

	// Calculate average latency
	var avgLatency time.Duration
	if len(stats.Latencies) > 0 {
		var total time.Duration
		for _, l := range stats.Latencies {
			total += l
		}
		avgLatency = total / time.Duration(len(stats.Latencies))
	}

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    BENCHMARK RESULTS                         ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Duration:           %-40v ║\n", stats.TotalDuration.Round(time.Millisecond))
	fmt.Printf("║  Total Requests:     %-40d ║\n", stats.TotalRequests)
	fmt.Printf("║  Successful:         %-40d ║\n", stats.SuccessCount)
	fmt.Printf("║  Errors:             %-40d ║\n", stats.ErrorCount)
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Throughput:         %-40s ║\n", fmt.Sprintf("%.2f ops/sec", opsPerSec))
	fmt.Printf("║  Cache Hit Rate:     %-40s ║\n", fmt.Sprintf("%.2f%%", hitRate))
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  LATENCY PERCENTILES                                         ║")
	fmt.Printf("║    Average:          %-40v ║\n", avgLatency.Round(time.Microsecond))
	fmt.Printf("║    P50 (median):     %-40v ║\n", p50.Round(time.Microsecond))
	fmt.Printf("║    P95:              %-40v ║\n", p95.Round(time.Microsecond))
	fmt.Printf("║    P99:              %-40v ║\n", p99.Round(time.Microsecond))
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")

	// Print markdown table for README
	fmt.Println()
	fmt.Println("## Markdown Table (copy to README):")
	fmt.Println()
	fmt.Println("| Metric | Value |")
	fmt.Println("|--------|-------|")
	fmt.Printf("| Throughput | %.2f ops/sec |\n", opsPerSec)
	fmt.Printf("| Cache Hit Rate | %.2f%% |\n", hitRate)
	fmt.Printf("| Latency (avg) | %v |\n", avgLatency.Round(time.Microsecond))
	fmt.Printf("| Latency (p50) | %v |\n", p50.Round(time.Microsecond))
	fmt.Printf("| Latency (p95) | %v |\n", p95.Round(time.Microsecond))
	fmt.Printf("| Latency (p99) | %v |\n", p99.Round(time.Microsecond))
	fmt.Printf("| Total Requests | %d |\n", stats.TotalRequests)
	fmt.Printf("| Error Rate | %.2f%% |\n", float64(stats.ErrorCount)/float64(stats.TotalRequests)*100)
}

func percentile(latencies []time.Duration, p float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	idx := int(float64(len(latencies)) * p / 100)
	if idx >= len(latencies) {
		idx = len(latencies) - 1
	}
	return latencies[idx]
}
