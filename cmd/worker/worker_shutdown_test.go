package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"example.com/cassandrafeed/internal/models"
	"example.com/cassandrafeed/internal/store"
	"github.com/segmentio/kafka-go"
)

// TestWorker_GracefulShutdown ensures the worker processes messages
// and shuts down gracefully.
func TestWorker_GracefulShutdown(t *testing.T) {
	// Initialize mock store
	mockStore := store.NewMock()

	authorID := uint64(1)
	followerID := uint64(2)

	// Create author and follower in mock store
	mockStore.CreateUser("author")   // returns ID 1
	mockStore.CreateUser("follower") // returns ID 2

	// Follower follows author
	mockStore.CreateFollow(followerID, authorID)

	// Prepare post message
	post := models.Post{
		ID:       100,
		AuthorID: authorID,
		Body:     "Shutdown test post",
		Created:  time.Now(),
	}
	data, _ := json.Marshal(post)

	// Mock Kafka reader with one message
	mockKafka := &MockKafkaReader{
		Messages: []kafka.Message{{Value: data}},
	}

	// Create a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})

	// Initialize worker
	worker := &Worker{
		store:  mockStore,
		reader: mockKafka,
	}

	// Run worker in a goroutine
	go func() {
		worker.Run(ctx) // Run should process messages until ctx.Done()
		close(done)
	}()

	// Wait for worker to finish
	select {
	case <-done:
		// Check that the follower's feed received the post
		feed, _ := mockStore.GetFeed(followerID, 10)
		if len(feed) != 1 || feed[0].Body != post.Body {
			t.Fatalf("feed not updated correctly: %+v", feed)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker did not shutdown gracefully in time")
	}

	// Close worker manually and ensure Kafka reader is closed
	if err := worker.Close(); err != nil {
		t.Fatalf("worker Close() error: %v", err)
	}

	if !mockKafka.Closed {
		t.Fatal("expected Kafka reader to be closed")
	}
}

// MockKafkaReader simulates a Kafka reader for testing
type MockKafkaReader struct {
	Messages   []kafka.Message
	ShouldFail bool
	Closed     bool
}

// ReadMessage simulates reading a message from Kafka
func (m *MockKafkaReader) ReadMessage(ctx context.Context) (kafka.Message, error) {
	if m.ShouldFail {
		return kafka.Message{}, ctx.Err()
	}
	select {
	case <-ctx.Done():
		return kafka.Message{}, ctx.Err()
	default:
	}

	if len(m.Messages) == 0 {
		time.Sleep(5 * time.Millisecond)
		return kafka.Message{}, nil
	}

	msg := m.Messages[0]
	m.Messages = m.Messages[1:]
	return msg, nil
}

// Close simulates closing the Kafka reader
func (m *MockKafkaReader) Close() error {
	m.Closed = true
	return nil
}
