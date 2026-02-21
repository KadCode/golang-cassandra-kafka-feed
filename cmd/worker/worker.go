package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	appkafka "example.com/cassandrafeed/internal/broker"
	"example.com/cassandrafeed/internal/logger"
	"example.com/cassandrafeed/internal/models"
	"example.com/cassandrafeed/internal/store"
)

var logg = logger.New()

// Worker consumes Kafka messages and updates user feeds in Cassandra concurrently.
type Worker struct {
	store        store.StoreInterface
	reader       appkafka.KafkaReader
	workerCount  int
	jobQueueSize int
}

// New creates a new concurrent Worker using pre-initialized dependencies.
func New(store store.StoreInterface, reader appkafka.KafkaReader, workerCount, jobQueueSize int) *Worker {
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	if jobQueueSize <= 0 {
		jobQueueSize = workerCount * 10
	}
	return &Worker{
		store:        store,
		reader:       reader,
		workerCount:  workerCount,
		jobQueueSize: jobQueueSize,
	}
}

// Run starts message reading and concurrent processing.
func (w *Worker) Run(ctx context.Context) {
	if w.workerCount <= 0 {
		w.workerCount = 1
	}
	if w.jobQueueSize <= 0 {
		w.jobQueueSize = 10
	}

	logg.Info("worker", "Starting "+fmt.Sprint(w.workerCount)+" workers with queue size "+fmt.Sprint(w.jobQueueSize))

	jobs := make(chan []byte, w.jobQueueSize)
	var wg sync.WaitGroup

	for i := 0; i < w.workerCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			w.processLoop(ctx, jobs)
		}(i)
	}

	w.readLoop(ctx, jobs)

	close(jobs)
	wg.Wait()
	logg.Info("worker", "All workers stopped gracefully")
}

// readLoop reads Kafka messages and pushes them into a job queue.
func (w *Worker) readLoop(ctx context.Context, jobs chan<- []byte) {
	var retry int
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := w.reader.ReadMessage(ctx)
			if err != nil {
				backoff := time.Duration(math.Min(1000, math.Pow(2, float64(retry)))) * time.Millisecond
				logg.Error("worker", "Kafka read error, backing off", err)
				if !waitWithContext(ctx, backoff) {
					return
				}
				retry++
				continue
			}
			retry = 0

			if len(msg.Value) == 0 {
				if !waitWithContext(ctx, 50*time.Millisecond) {
					return
				}
				continue
			}

			select {
			case jobs <- msg.Value:
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				logg.Info("worker", "Queue full, waiting to enqueue Kafka message")
			}
		}
	}
}

// processLoop handles JSON decoding and feed updates concurrently.
func (w *Worker) processLoop(ctx context.Context, jobs <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-jobs:
			if !ok {
				return
			}

			var post models.Post
			if err := json.Unmarshal(data, &post); err != nil {
				logg.Error("worker", "Invalid JSON in Kafka message", err)
				continue
			}

			followers, err := w.store.GetFollowers(post.AuthorID)
			if err != nil {
				logg.Error("worker", "Error fetching followers for post author", err)
				continue
			}

			const fanoutLimit = 20
			var fanoutWG sync.WaitGroup
			semaphore := make(chan struct{}, fanoutLimit)

			for _, uid := range followers {
				select {
				case <-ctx.Done():
					return
				default:
					fanoutWG.Add(1)
					semaphore <- struct{}{}

					go func(u string) {
						defer fanoutWG.Done()
						defer func() { <-semaphore }()
						if err := w.store.AddToFeed(u, post); err != nil {
							logg.Error("worker", "Failed to add post to user feed", err)
						}
					}(uid)
				}
			}

			fanoutWG.Wait()
			logg.Info("worker", "Post delivered to followers (post ID anonymized)")
		}
	}
}

// waitWithContext waits for duration or context cancellation.
func waitWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Close shuts down Kafka reader and Cassandra session.
func (w *Worker) Close() error {
	logg.Info("worker", "Closing Kafka reader")
	if err := w.reader.Close(); err != nil {
		logg.Error("worker", "Error closing Kafka reader", err)
		return err
	}

	logg.Info("worker", "Closing Cassandra session")
	w.store.Close()
	return nil
}
