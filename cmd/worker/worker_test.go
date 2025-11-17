package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appkafka "example.com/cassandrafeed/internal/broker"
	"example.com/cassandrafeed/internal/models"
	"example.com/cassandrafeed/internal/store"
	"github.com/segmentio/kafka-go"
)

// runWorkerOnce processes a single Kafka message for testing.
func runWorkerOnce(ctx context.Context, st store.StoreInterface, kafkaReader appkafka.KafkaReader) error {
	msg, err := kafkaReader.ReadMessage(ctx)
	if err != nil {
		return err
	}
	if len(msg.Value) == 0 {
		return nil
	}

	var post models.Post
	if err := json.Unmarshal(msg.Value, &post); err != nil {
		return err
	}

	followers, err := st.GetFollowers(post.AuthorID)
	if err != nil {
		return err
	}

	for _, uid := range followers {
		if err := st.AddToFeed(uid, post); err != nil {
			return err
		}
	}

	return nil
}

// ---------- Positive test ----------

func TestWorker_DistributePost(t *testing.T) {
	mockStore := store.NewMock()

	authorID, _ := mockStore.CreateUser("author")
	followerID, _ := mockStore.CreateUser("follower")
	mockStore.CreateFollow(followerID, authorID)

	post := models.Post{
		ID:       "100",
		AuthorID: authorID,
		Body:     "Hello followers!",
		Created:  time.Now(),
	}
	data, _ := json.Marshal(post)

	mockKafka := &appkafka.MockKafka{
		ReadMessages: []kafka.Message{{Value: data}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := runWorkerOnce(ctx, mockStore, mockKafka)

	if err != nil {
		t.Fatalf("worker failed: %v", err)
	}

	feed, _ := mockStore.GetFeed(followerID, 10)
	if len(feed) != 1 || feed[0].Body != post.Body {
		t.Fatalf("feed not updated correctly, got: %+v", feed)
	}
}

// ---------- Negative tests ----------

// Simulate Kafka read error
func TestWorker_KafkaReadError(t *testing.T) {
	mockStore := store.NewMock()
	mockKafka := &appkafka.MockKafkaFail{}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := runWorkerOnce(ctx, mockStore, mockKafka)
	if err == nil {
		t.Fatalf("expected error from Kafka read")
	}
}

// Simulate invalid post JSON
func TestWorker_InvalidPostJSON(t *testing.T) {
	mockStore := store.NewMock()

	mockKafka := &appkafka.MockKafka{
		ReadMessages: []kafka.Message{
			{Value: []byte("{invalid-json}")},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := runWorkerOnce(ctx, mockStore, mockKafka)
	if err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

// Simulate store failure when adding post to feed
func TestWorker_StoreAddToFeedFail(t *testing.T) {
	mockStore := &store.MockStoreFail{}

	// Post by a dummy author (ID doesn't matter; store always fails)
	post := models.Post{
		ID:       "",
		AuthorID: "",
		Body:     "test",
		Created:  time.Now(),
	}
	data, _ := json.Marshal(post)

	mockKafka := &appkafka.MockKafka{
		ReadMessages: []kafka.Message{{Value: data}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := runWorkerOnce(ctx, mockStore, mockKafka)
	if err == nil {
		t.Fatalf("expected error from store AddToFeed")
	}
}

func TestWorker_EmptyKafkaMessage(t *testing.T) {
	mockStore := store.NewMock()
	mockKafka := &appkafka.MockKafka{
		ReadMessages: []kafka.Message{{Value: nil}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := runWorkerOnce(ctx, mockStore, mockKafka)
	if err != nil {
		t.Fatalf("expected no error for empty Kafka message, got: %v", err)
	}
}

func TestWorker_StoreGetFollowersFail(t *testing.T) {
	mockStore := &store.MockStoreFail{}

	post := models.Post{
		ID:       "200",
		AuthorID: "author123",
		Body:     "Post that triggers GetFollowers error",
		Created:  time.Now(),
	}
	data, _ := json.Marshal(post)

	mockKafka := &appkafka.MockKafka{
		ReadMessages: []kafka.Message{{Value: data}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := runWorkerOnce(ctx, mockStore, mockKafka)
	if err == nil {
		t.Fatalf("expected error from store GetFollowers, got nil")
	}
}
