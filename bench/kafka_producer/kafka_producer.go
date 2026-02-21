package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gocql/gocql"
	"github.com/segmentio/kafka-go"
)

// Post represents the structure of a post message sent to Kafka
type Post struct {
	ID       string    `json:"id"`
	AuthorID string    `json:"author_id"`
	Body     string    `json:"body"`
	Created  time.Time `json:"created"`
}

func main() {
	const (
		total       = 100000 // total number of messages to send
		batchSize   = 100    // batch size for sending messages
		numWorkers  = 4      // number of parallel goroutines
		kafkaBroker = "localhost:9092"
		topic       = "feed-topic"
	)

	// Kafka writer with asynchronous sending enabled
	w := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{kafkaBroker},
		Topic:   topic,
		Async:   true,
	})
	defer w.Close()

	// Generate a unique author ID for this benchmark
	authorID := gocql.TimeUUID().String()
	start := time.Now()

	var successCount uint64
	var failCount uint64

	// Channel for feeding message indexes to worker goroutines
	jobs := make(chan int, total)
	var wg sync.WaitGroup

	// --- Start worker goroutines ---
	for wID := 0; wID < numWorkers; wID++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			batch := make([]kafka.Message, 0, batchSize)

			for i := range jobs {
				// Create a new post
				p := Post{
					ID:       strconv.FormatInt(time.Now().UnixNano(), 10),
					AuthorID: authorID,
					Body:     fmt.Sprintf("kafka bench %d", i),
					Created:  time.Now(),
				}

				// Marshal post to JSON
				v, err := json.Marshal(p)
				if err != nil {
					atomic.AddUint64(&failCount, 1)
					fmt.Printf("marshal error: %v\n", err)
					continue
				}

				// Add message to batch
				batch = append(batch, kafka.Message{
					Key:   []byte("post_created"),
					Value: v,
				})

				// Send batch if batch size reached
				if len(batch) >= batchSize {
					if err := w.WriteMessages(context.Background(), batch...); err != nil {
						atomic.AddUint64(&failCount, uint64(len(batch)))
						fmt.Printf("write error: %v\n", err)
					} else {
						atomic.AddUint64(&successCount, uint64(len(batch)))
					}
					batch = batch[:0] // clear the batch
				}
			}

			// Send any remaining messages after finishing loop
			if len(batch) > 0 {
				if err := w.WriteMessages(context.Background(), batch...); err != nil {
					atomic.AddUint64(&failCount, uint64(len(batch)))
					fmt.Printf("write error: %v\n", err)
				} else {
					atomic.AddUint64(&successCount, uint64(len(batch)))
				}
			}
		}()
	}

	// Feed jobs channel with indexes
	for i := 0; i < total; i++ {
		jobs <- i
	}
	close(jobs)

	// Wait for all worker goroutines to finish
	wg.Wait()

	// --- Benchmark results ---
	elapsed := time.Since(start)
	fmt.Printf("Total messages: %d\n", total)
	fmt.Printf("Successful: %d, Failed: %d\n", successCount, failCount)
	fmt.Printf("Elapsed time: %s\n", elapsed)
	fmt.Printf("Throughput: %.2f msg/s\n", float64(successCount)/elapsed.Seconds())
}
