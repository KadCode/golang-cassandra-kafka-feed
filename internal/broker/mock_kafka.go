package appkafka

import (
	"context"
	"errors"

	"github.com/segmentio/kafka-go"
)

// MockKafka provides a mock implementation of KafkaWriter and KafkaReader
// for testing without connecting to a real Kafka broker.
type MockKafka struct {
	WrittenMessages []kafka.Message // stores messages written via WriteMessages
	ReadMessages    []kafka.Message // queue of messages to be read via ReadMessage
	ShouldFail      bool            // flag to simulate failures during write or read operations
}

// MockKafkaFail is a simplified mock that always fails on both read and write operations.
type MockKafkaFail struct{}

// WriteMessages appends messages to the internal slice or simulates a failure.
func (m *MockKafka) WriteMessages(messages ...kafka.Message) error {
	if m.ShouldFail {
		return errors.New("mock kafka write error")
	}
	m.WrittenMessages = append(m.WrittenMessages, messages...)
	return nil
}

// ReadMessage returns the next message from the mock queue or an empty message if none left.
func (m *MockKafka) ReadMessage(ctx context.Context) (kafka.Message, error) {
	if m.ShouldFail {
		return kafka.Message{}, errors.New("mock kafka read error")
	}

	if len(m.ReadMessages) == 0 {
		return kafka.Message{}, nil
	}

	msg := m.ReadMessages[0]
	m.ReadMessages = m.ReadMessages[1:]
	return msg, nil
}

// Close is a no-op for MockKafka.
func (m *MockKafka) Close() error {
	return nil
}

// WriteMessages always returns an error — simulating a failed write operation.
func (m *MockKafkaFail) WriteMessages(messages ...kafka.Message) error {
	return errors.New("mock kafka write failed")
}

// ReadMessage always returns an error — simulating a failed read operation.
func (m *MockKafkaFail) ReadMessage(ctx context.Context) (kafka.Message, error) {
	return kafka.Message{}, errors.New("mock kafka read failed")
}

// Close is a no-op for MockKafkaFail.
func (m *MockKafkaFail) Close() error {
	return nil
}
