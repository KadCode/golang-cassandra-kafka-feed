package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// UserResp represents the response returned by the server after user creation
type UserResp struct {
	UserID string `json:"user_id"`
	Token  string `json:"token"`
}

// PostReq represents the JSON payload for creating a post
type PostReq struct {
	Body string `json:"body"`
}

func main() {
	// --- Command-line flags ---
	var server string
	var duration int
	var concurrency int
	var csvFile string
	var trimPercent float64

	flag.StringVar(&server, "server", "https://localhost:8080", "server base URL")
	flag.IntVar(&duration, "duration", 30, "duration in seconds")
	flag.IntVar(&concurrency, "c", 50, "number of concurrent goroutines / users")
	flag.StringVar(&csvFile, "csv", "latencies.csv", "CSV file to save latencies")
	flag.Float64Var(&trimPercent, "trim", 1.0, "percent of latency to trim from top and bottom for trimmed mean")
	flag.Parse()

	// --- Load client certificate for mTLS ---
	cert, err := tls.LoadX509KeyPair("../../certs/cert.pem", "../../certs/key.pem")
	if err != nil {
		panic(fmt.Sprintf("failed to load cert/key: %v", err))
	}

	// Configure HTTP client with TLS
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		},
		Timeout: 10 * time.Second,
	}

	// --- Create users for each goroutine ---
	fmt.Printf("Creating %d users...\n", concurrency)
	users := make([]UserResp, concurrency)
	for i := 0; i < concurrency; i++ {
		payload := map[string]string{"username": fmt.Sprintf("load-user-%d-%d", i, time.Now().UnixNano())}
		b, _ := json.Marshal(payload)

		resp, err := client.Post(server+"/users", "application/json", bytes.NewReader(b))
		if err != nil {
			panic(fmt.Sprintf("failed to create user: %v", err))
		}

		if err := json.NewDecoder(resp.Body).Decode(&users[i]); err != nil {
			resp.Body.Close()
			panic(fmt.Sprintf("failed to decode user response: %v", err))
		}
		resp.Body.Close()
	}
	fmt.Println("Users created.")

	// --- Prepare concurrency test ---
	stopTime := time.Now().Add(time.Duration(duration) * time.Second)
	var wg sync.WaitGroup

	// Atomic counters for thread-safe tracking
	var requests int64
	var successes int64
	var errors4xx int64
	var errors5xx int64

	latencySlices := make([][]float64, concurrency) // each goroutine records latencies

	// --- Start concurrent goroutines for load test ---
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			user := users[idx]
			var localLatencies []float64

			// Keep sending POST requests until the test duration ends
			for time.Now().Before(stopTime) {
				start := time.Now()
				body := PostReq{Body: fmt.Sprintf("load test post %d", time.Now().UnixNano())}
				b, _ := json.Marshal(body)

				req, _ := http.NewRequestWithContext(context.Background(), "POST", server+"/posts", bytes.NewReader(b))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+user.Token)

				resp, err := client.Do(req)
				lat := time.Since(start).Seconds() * 1000 // latency in ms
				localLatencies = append(localLatencies, lat)
				atomic.AddInt64(&requests, 1)

				if err != nil {
					fmt.Printf("Request error: %v\n", err)
					continue
				}

				// Count success/failure by status code
				if resp != nil {
					if resp.StatusCode >= 200 && resp.StatusCode < 300 {
						atomic.AddInt64(&successes, 1)
					} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
						atomic.AddInt64(&errors4xx, 1)
					} else if resp.StatusCode >= 500 {
						atomic.AddInt64(&errors5xx, 1)
					}

					bodyBytes, _ := io.ReadAll(resp.Body)
					if len(bodyBytes) > 0 {
						fmt.Printf("Status %d: %s\n", resp.StatusCode, string(bodyBytes))
					}
					resp.Body.Close()
				}
			}

			latencySlices[idx] = localLatencies
		}(i)
	}

	wg.Wait()

	// --- Merge all latencies ---
	var allLatencies []float64
	for _, slice := range latencySlices {
		allLatencies = append(allLatencies, slice...)
	}
	sort.Float64s(allLatencies)

	// --- Compute statistics ---
	trimmedMeanVal := trimmedMean(allLatencies, trimPercent)
	p50 := percentile(allLatencies, 50)
	p90 := percentile(allLatencies, 90)
	p99 := percentile(allLatencies, 99)

	fmt.Printf("Requests: %d  Successes: %d  4xx: %d  5xx: %d\n", requests, successes, errors4xx, errors5xx)
	fmt.Printf("Latency (ms): trimmed_mean=%.2f p50=%.2f p90=%.2f p99=%.2f\n", trimmedMeanVal, p50, p90, p99)

	// --- Save latencies to CSV ---
	f, err := os.Create(csvFile)
	if err != nil {
		fmt.Printf("Failed to create CSV file: %v\n", err)
		return
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"latency_ms"})
	for _, d := range allLatencies {
		w.Write([]string{fmt.Sprintf("%.3f", d)})
	}
	fmt.Printf("Saved latencies to %s\n", csvFile)
}

// trimmedMean calculates mean latency after trimming top/bottom trimPercent values
func trimmedMean(data []float64, trimPercent float64) float64 {
	if len(data) == 0 {
		return 0
	}
	trim := int(float64(len(data)) * trimPercent / 100.0)
	if trim*2 >= len(data) {
		trim = len(data) / 2
	}
	trimmed := data[trim : len(data)-trim]
	var sum float64
	for _, v := range trimmed {
		sum += v
	}
	return sum / float64(len(trimmed))
}

// percentile calculates the p-th percentile from sorted data
func percentile(data []float64, p float64) float64 {
	if len(data) == 0 {
		return 0
	}
	k := (p / 100.0) * float64(len(data)-1)
	f := int(k)
	c := f + 1
	if c >= len(data) {
		return data[len(data)-1]
	}
	d0 := data[f]*(float64(c)-k) + data[c]*(k-float64(f))
	return d0
}
