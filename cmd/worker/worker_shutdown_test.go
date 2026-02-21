package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"example.com/cassandrafeed/internal/models"
	"example.com/cassandrafeed/internal/store"
	"github.com/segmentio/kafka-go"
)

// TestWorker_GracefulShutdown ensures that the worker:
// 1. Processes messages from Kafka.
// 2. Updates followers' feeds correctly.
// 3. Shuts down gracefully when the context is canceled.
func TestWorker_GracefulShutdown(t *testing.T) {
	mockStore := store.NewMock()

	authorID := "1"
	followerID := "2"

	mockStore.CreateUser("author")
	mockStore.CreateUser("follower")

	mockStore.CreateFollow(followerID, authorID)

	post := models.Post{
		ID:       "100",
		AuthorID: authorID,
		Body:     "Shutdown test post",
		Created:  time.Now(),
	}
	data, _ := json.Marshal(post)

	// Mock Kafka reader with a single message
	mockKafka := &MockKafkaReader{
		Messages: []kafka.Message{{Value: data}},
	}

	// Context with timeout to simulate graceful shutdown signal
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})

	// Initialize worker with mock store and Kafka reader
	worker := &Worker{
		store:  mockStore,
		reader: mockKafka,
	}

	// Run the worker in a separate goroutine
	go func() {
		worker.Run(ctx) // Worker processes messages until ctx.Done()
		close(done)
	}()

	// Wait for worker to finish or timeout
	select {
	case <-done:
		// Verify that follower's feed contains the post
		feed, _ := mockStore.GetFeed(followerID, 10)
		if len(feed) != 1 || feed[0].Body != post.Body {
			t.Fatalf("feed not updated correctly: %+v", feed)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker did not shutdown gracefully in time")
	}

	if err := worker.Close(); err != nil {
		t.Fatalf("worker Close() error: %v", err)
	}

	if !mockKafka.Closed {
		t.Fatal("expected Kafka reader to be closed")
	}
}

// MockKafkaReader simulates a Kafka reader for testing purposes
type MockKafkaReader struct {
	Messages   []kafka.Message // Queue of messages to return
	ShouldFail bool            // If true, ReadMessage will fail
	Closed     bool            // Tracks whether Close() has been called
}

// ReadMessage returns the next message in the queue or simulates a failure/context cancel
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
		time.Sleep(5 * time.Millisecond) // simulate idle wait
		return kafka.Message{}, nil
	}

	msg := m.Messages[0]
	m.Messages = m.Messages[1:]
	return msg, nil
}

// Close marks the mock Kafka reader as closed
func (m *MockKafkaReader) Close() error {
	m.Closed = true
	return nil
}
