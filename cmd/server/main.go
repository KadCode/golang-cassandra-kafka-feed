package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	appkafka "example.com/cassandrafeed/internal/broker"
	"example.com/cassandrafeed/internal/models"
	"example.com/cassandrafeed/internal/store"
	"github.com/segmentio/kafka-go"
)

// Server represents the application server with dependencies on Cassandra store and Kafka writer.
type Server struct {
	store       store.StoreInterface
	kafkaWriter appkafka.KafkaWriter
}

// main initializes the store, Kafka writer, HTTP server, and handles graceful shutdown.
func main() {
	// Initialize Cassandra store.
	st, err := store.New()
	if err != nil {
		log.Fatal(err)
	}

	// Load Kafka configuration from environment variables.
	topic := getEnv("KAFKA_TOPIC", "feed-topic")
	brokers := []string{getEnv("KAFKA_BROKER", "localhost:9092")}
	writeTimeout := getEnvDuration("KAFKA_WRITE_TIMEOUT", 10*time.Second)

	cfg := appkafka.KafkaConfig{
		Brokers:      brokers,
		Topic:        topic,
		Partition:    0,
		WriteTimeout: writeTimeout,
	}

	// Initialize Kafka writer connection.
	writer, err := appkafka.NewKafkaWriter(cfg)
	if err != nil {
		log.Fatal(err)
	}

	server := &Server{
		store:       st,
		kafkaWriter: writer,
	}

	// Register HTTP handlers.
	mux := http.NewServeMux()
	mux.HandleFunc("/users", server.createUserHandler)
	mux.HandleFunc("/follow", server.followHandler)
	mux.HandleFunc("/posts", server.createPostHandler)
	mux.HandleFunc("/feed", server.getFeedHandler)

	httpServer := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Channel to listen for OS signals to gracefully shutdown the server.
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server in a separate goroutine.
	go func() {
		log.Println("Server running on :8080")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal.
	<-shutdownCh
	log.Println("Shutdown signal received, closing server...")

	// Create context with timeout for graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown HTTP server gracefully.
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Error during HTTP shutdown: %v", err)
	}

	// Close Kafka writer and Cassandra session.
	server.Close()
	log.Println("Server gracefully stopped.")
}

// Close gracefully closes Kafka writer and Cassandra session.
func (s *Server) Close() {
	log.Println("Closing Kafka writer...")
	if err := s.kafkaWriter.Close(); err != nil {
		log.Printf("Error closing Kafka: %v", err)
	}

	log.Println("Closing Cassandra session...")
	s.store.Close()
}

// --- Environment helpers ---

// getEnv retrieves an environment variable or returns a default value.
func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// getEnvDuration retrieves an environment variable as a duration or returns a default value.
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultValue
}

// --- HTTP Handlers ---

// createUserHandler handles POST requests to create a new user.
// Expects JSON body: {"username": "example"}
// Returns JSON response: {"user_id": <id>}
func (s *Server) createUserHandler(w http.ResponseWriter, r *http.Request) {
	type req struct{ Username string }
	var body req

	// Decode JSON request body.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Create user in Cassandra.
	id, err := s.store.CreateUser(body.Username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return created user ID as JSON.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"user_id": id})
}

// followHandler creates a "follow" relationship between users.
// Expects JSON body: {"user_id": 1, "followee_id": 2}
func (s *Server) followHandler(w http.ResponseWriter, r *http.Request) {
	type req struct {
		UserID     uint64 `json:"user_id"`
		FolloweeID uint64 `json:"followee_id"`
	}
	var body req

	// Decode JSON request body.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Create follow relationship in Cassandra.
	if err := s.store.CreateFollow(body.UserID, body.FolloweeID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// createPostHandler handles post creation, stores it in Cassandra, and publishes an event to Kafka.
// Expects JSON body: {"author_id": 1, "body": "post content"}
// Returns JSON response with created post data.
func (s *Server) createPostHandler(w http.ResponseWriter, r *http.Request) {
	type req struct {
		AuthorID uint64 `json:"author_id"`
		Body     string `json:"body"`
	}
	var body req

	// Decode JSON request body.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Build the post model.
	post := models.Post{
		ID:       uint64(time.Now().UnixNano()),
		AuthorID: body.AuthorID,
		Body:     body.Body,
		Created:  time.Now(),
	}

	// Serialize the post for Kafka.
	data, err := json.Marshal(post)
	if err != nil {
		http.Error(w, "failed to marshal post", http.StatusInternalServerError)
		return
	}

	// Create Kafka message for post creation event.
	msg := kafka.Message{
		Key:   []byte("post_created"),
		Value: data,
	}

	// Publish message to Kafka.
	if err := s.kafkaWriter.WriteMessages(msg); err != nil {
		http.Error(w, "failed to write Kafka message: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Store post in Cassandra.
	if err := s.store.AddPost(post); err != nil {
		http.Error(w, "failed to save post: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return created post as JSON.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

// getFeedHandler retrieves a user's feed based on their user ID.
// Query parameters: ?user_id=1&limit=50
func (s *Server) getFeedHandler(w http.ResponseWriter, r *http.Request) {
	userIdStr := r.URL.Query().Get("user_id")
	limitStr := r.URL.Query().Get("limit")

	// Parse user_id parameter.
	userId, err := strconv.ParseUint(userIdStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid user_id", http.StatusBadRequest)
		return
	}

	// Parse limit parameter with default.
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Retrieve feed from Cassandra.
	feed, err := s.store.GetFeed(userId, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return feed as JSON.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(feed)
}
