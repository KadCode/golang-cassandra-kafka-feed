package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	appkafka "example.com/cassandrafeed/internal/broker"
	"example.com/cassandrafeed/internal/models"
	"example.com/cassandrafeed/internal/store"
)

// Worker consumes Kafka messages and updates user feeds in Cassandra.
type Worker struct {
	store  store.StoreInterface
	reader appkafka.KafkaReader
}

// Run starts the worker loop to consume Kafka messages until the context is canceled.
func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := w.reader.ReadMessage(ctx)
			if err != nil {
				log.Printf("Kafka read error: %v", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}

			if len(msg.Value) == 0 {
				time.Sleep(500 * time.Millisecond)
				continue
			}

			var post models.Post
			if err := json.Unmarshal(msg.Value, &post); err != nil {
				log.Printf("Invalid message JSON: %v", err)
				continue
			}

			followers, err := w.store.GetFollowers(post.AuthorID)
			if err != nil {
				log.Printf("GetFollowers error: %v", err)
				continue
			}

			for _, uid := range followers {
				if err := w.store.AddToFeed(uid, post); err != nil {
					log.Printf("AddToFeed error for user %d: %v", uid, err)
				}
			}

			log.Printf("Post %d distributed to %d followers", post.ID, len(followers))
		}
	}
}

// Close gracefully closes Kafka reader and Cassandra connection.
func (w *Worker) Close() error {
	log.Println("Closing Kafka reader...")
	if err := w.reader.Close(); err != nil {
		log.Printf("Error closing Kafka reader: %v", err)
		return err
	}

	log.Println("Closing Cassandra session...")
	w.store.Close()
	return nil
}

// --- Entry point ---

func main() {
	// Initialize Cassandra store
	st, err := store.New()
	if err != nil {
		log.Fatalf("failed to connect to Cassandra: %v", err)
	}

	// Load Kafka configuration
	cfg := appkafka.KafkaConfig{
		Brokers:     []string{getEnv("KAFKA_BROKER", "localhost:9092")},
		Topic:       getEnv("KAFKA_TOPIC", "feed-topic"),
		GroupID:     getEnv("KAFKA_GROUP_ID", "worker-group"),
		ReadTimeout: getEnvDuration("KAFKA_READ_TIMEOUT", 10*time.Second),
	}

	reader := appkafka.NewKafkaReader(cfg)
	worker := &Worker{store: st, reader: reader}

	// Graceful shutdown setup
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Println("Worker started. Listening for Kafka messages...")

	// Run the worker
	worker.Run(ctx)

	// Close resources
	worker.Close()
	log.Println("Worker stopped gracefully.")
}

// --- Helper functions for environment variables ---

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultValue
}
