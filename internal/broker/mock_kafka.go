package appkafka

import (
	"context"
	"encoding/json"
	"errors"

	"example.com/cassandrafeed/internal/models"
	"example.com/cassandrafeed/internal/store"
	"github.com/segmentio/kafka-go"
)

// MockKafka immediately applies posts to the store for followers.
type MockKafka struct {
	Store           *store.MockStore
	WrittenMessages []kafka.Message // stores messages written via WriteMessages
	ReadMessages    []kafka.Message // queue of messages to be read via ReadMessage
	ShouldFail      bool            // flag to simulate failures during write or read operations
}

// WriteMessages simulates writing a post to Kafka, immediately adding it to followers' feeds.
func (m *MockKafka) WriteMessages(messages ...kafka.Message) error {
	if m.Store == nil {
		return errors.New("store is nil")
	}

	for _, msg := range messages {
		var post models.Post
		if err := json.Unmarshal(msg.Value, &post); err != nil {
			return err
		}

		// Add post to author's own feed
		_ = m.Store.AddToFeed(post.AuthorID, post)

		// Add post to followers' feeds
		followers, _ := m.Store.GetFollowers(post.AuthorID)
		for _, followerID := range followers {
			_ = m.Store.AddToFeed(followerID, post)
		}

		// Store posts in Posts map
		_ = m.Store.AddPost(post)
	}

	return nil
}

// ReadMessage is a no-op in tests.
func (m *MockKafka) ReadMessage(ctx context.Context) (kafka.Message, error) {
	if m.ShouldFail {
		return kafka.Message{}, errors.New("mock kafka read failed")
	}
	if len(m.ReadMessages) == 0 {
		return kafka.Message{}, errors.New("no messages")
	}
	// Take the first message from the queue and remove it
	msg := m.ReadMessages[0]
	m.ReadMessages = m.ReadMessages[1:]
	return msg, nil
}

// Close is a no-op.
func (m *MockKafka) Close() error { return nil }

// MockKafkaFail always fails.
type MockKafkaFail struct{}

func (m *MockKafkaFail) WriteMessages(messages ...kafka.Message) error {
	return errors.New("mock kafka write failed")
}

func (m *MockKafkaFail) ReadMessage(ctx context.Context) (kafka.Message, error) {
	return kafka.Message{}, errors.New("mock kafka read failed")
}

func (m *MockKafkaFail) Close() error { return nil }
