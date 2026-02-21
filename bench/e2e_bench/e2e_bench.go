package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

// UserResp represents the server's response when a user is created.
type UserResp struct {
	UserID string `json:"user_id"`
	Token  string `json:"token"`
}

// PostReq defines the request payload for creating a new post.
type PostReq struct {
	Body string `json:"body"`
}

// Post represents a post entity returned by the API.
type Post struct {
	ID       string    `json:"id"`
	AuthorID string    `json:"author_id"`
	Body     string    `json:"body"`
	Created  time.Time `json:"created"`
}

func main() {
	// CLI flags
	var serverAddr string
	var U, F, P, concurrency int
	var pollTimeout int

	flag.StringVar(&serverAddr, "server", "https://localhost:8080", "server base URL")
	flag.IntVar(&U, "users", 50, "number of users to create")
	flag.IntVar(&F, "follows", 10, "average follows per user")
	flag.IntVar(&P, "posts", 100, "number of posts to publish")
	flag.IntVar(&concurrency, "c", 20, "concurrency for posting")
	flag.IntVar(&pollTimeout, "timeout", 10, "seconds to wait for post delivery")
	flag.Parse()

	ctx := context.Background()

	// --- TLS setup for secure communication ---
	cert, err := tls.LoadX509KeyPair("../../certs/cert.pem", "../../certs/key.pem")
	if err != nil {
		panic(fmt.Sprintf("failed to load cert/key: %v", err))
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		},
		Timeout: 10 * time.Second,
	}

	// --- 1) Create users ---
	fmt.Printf("Creating %d users...\n", U)
	users := make([]UserResp, 0, U)
	for i := 0; i < U; i++ {
		// Generate unique username
		payload := map[string]string{"username": fmt.Sprintf("user-%d-%d", i, time.Now().UnixNano())}
		b, _ := json.Marshal(payload)

		// Send POST request to create user
		resp, err := client.Post(serverAddr+"/users", "application/json", bytes.NewReader(b))
		if err != nil {
			fmt.Printf("create user error: %v\n", err)
			os.Exit(1)
		}

		var ur UserResp
		if err := json.NewDecoder(resp.Body).Decode(&ur); err != nil {
			resp.Body.Close()
			fmt.Printf("decode user resp error: %v\n", err)
			os.Exit(1)
		}
		resp.Body.Close()
		users = append(users, ur)
	}
	fmt.Println("Users created successfully.")

	// --- 2) Build a token map for quick authorization lookup ---
	userTokens := make(map[string]string, len(users))
	for _, u := range users {
		userTokens[u.UserID] = u.Token
	}

	// --- 3) Create follow relationships between users ---
	fmt.Printf("Creating follows (~%d per user)...\n", F)
	followMap := make(map[string][]string)
	for _, u := range users {
		for j := 0; j < F; j++ {
			followee := users[rand.Intn(len(users))]
			if followee.UserID == u.UserID {
				continue
			}
			payload := map[string]string{"followee_id": followee.UserID}
			b, _ := json.Marshal(payload)
			req, _ := http.NewRequestWithContext(ctx, "POST", serverAddr+"/follow", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+u.Token)

			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("follow error: %v\n", err)
				os.Exit(1)
			}
			resp.Body.Close()
			followMap[followee.UserID] = append(followMap[followee.UserID], u.UserID)
		}
	}
	fmt.Println("Follow relationships established.")

	// --- 4) Publish posts concurrently ---
	fmt.Printf("Publishing %d posts with concurrency %d...\n", P, concurrency)
	type postRecord struct {
		PostID   string
		AuthorID string
		Created  time.Time
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency) // concurrency limiter
	postsCh := make(chan postRecord, P)

	for i := 0; i < P; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			author := users[rand.Intn(len(users))]
			body := fmt.Sprintf("post %d", rand.Int())
			reqBody := PostReq{Body: body}
			b, _ := json.Marshal(reqBody)

			req, _ := http.NewRequestWithContext(ctx, "POST", serverAddr+"/posts", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+author.Token)

			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("post error: %v\n", err)
				return
			}

			var p Post
			if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
				resp.Body.Close()
				fmt.Printf("decode post error: %v\n", err)
				return
			}
			resp.Body.Close()
			postsCh <- postRecord{PostID: p.ID, AuthorID: p.AuthorID, Created: p.Created}
		}()
	}

	wg.Wait()
	close(postsCh)

	// --- 5) Verify post delivery to followers' feeds ---
	fmt.Println("Checking feed delivery...")
	var latencies []float64
	var latMu sync.Mutex
	var failCount int64
	var checksWg sync.WaitGroup

	for pr := range postsCh {
		followers := followMap[pr.AuthorID]
		for _, fid := range followers {
			checksWg.Add(1)
			go func(pr postRecord, fid string) {
				defer checksWg.Done()
				deadline := time.Now().Add(time.Duration(pollTimeout) * time.Second)
				found := false
				token := userTokens[fid]

				// Poll the feed until post appears or timeout
				for time.Now().Before(deadline) {
					req, _ := http.NewRequestWithContext(ctx, "GET", serverAddr+"/feed", nil)
					req.Header.Set("Authorization", "Bearer "+token)
					resp, err := client.Do(req)
					if err != nil {
						time.Sleep(200 * time.Millisecond)
						continue
					}

					var posts []Post
					if err := json.NewDecoder(resp.Body).Decode(&posts); err != nil {
						resp.Body.Close()
						time.Sleep(200 * time.Millisecond)
						continue
					}
					resp.Body.Close()

					for _, pp := range posts {
						if pp.ID == pr.PostID {
							lat := time.Since(pr.Created).Seconds() * 1000
							latMu.Lock()
							latencies = append(latencies, lat)
							latMu.Unlock()
							found = true
							return
						}
					}
					time.Sleep(200 * time.Millisecond)
				}

				if !found {
					latMu.Lock()
					failCount++
					latMu.Unlock()
				}
			}(pr, fid)
		}
	}

	checksWg.Wait()

	// --- 6) Compute latency statistics and export to CSV ---
	if len(latencies) == 0 {
		fmt.Println("No successful deliveries recorded.")
	} else {
		trimPercent := 1.0
		meanVal := trimmedMean(latencies, trimPercent)
		p50 := trimmedPercentile(latencies, 50, trimPercent)
		p90 := trimmedPercentile(latencies, 90, trimPercent)
		p99 := trimmedPercentile(latencies, 99, trimPercent)
		fmt.Printf("Delivery stats (ms): count=%d mean=%.2f p50=%.2f p90=%.2f p99=%.2f fails=%d\n",
			len(latencies), meanVal, p50, p90, p99, failCount)

		// Export latencies to CSV
		f, _ := os.Create("e2e_latencies.csv")
		w := csv.NewWriter(f)
		w.Write([]string{"latency_ms"})
		for _, v := range latencies {
			w.Write([]string{fmt.Sprintf("%.3f", v)})
		}
		w.Flush()
		f.Close()
		fmt.Println("Saved e2e_latencies.csv")
	}
}

// trimmedMean calculates the mean of a dataset excluding extreme values.
func trimmedMean(data []float64, trimPercent float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sort.Float64s(data)
	trim := int(float64(len(data)) * trimPercent / 100.0)
	if trim*2 >= len(data) {
		trim = len(data) / 2
	}
	data = data[trim : len(data)-trim]
	var sum float64
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

// trimmedPercentile returns a percentile value after trimming extremes.
func trimmedPercentile(data []float64, p float64, trimPercent float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sort.Float64s(data)
	trim := int(float64(len(data)) * trimPercent / 100.0)
	if trim*2 >= len(data) {
		trim = len(data) / 2
	}
	data = data[trim : len(data)-trim]
	return percentile(data, p)
}

// percentile calculates the requested percentile using linear interpolation.
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
	d0 := data[f] * (float64(c) - k)
	d1 := data[c] * (k - float64(f))
	return d0 + d1
}
