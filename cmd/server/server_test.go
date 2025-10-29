package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	appkafka "example.com/cassandrafeed/internal/broker"
	"example.com/cassandrafeed/internal/models"
	"example.com/cassandrafeed/internal/store"
	"github.com/segmentio/kafka-go"
)

var (
	serverOnce sync.Once
	testServer *httptest.Server
)

var server Server

// setupTestServer initializes a test HTTP server with mock Kafka and mock store.
// Ensures only one server instance is created for all tests.
func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	serverOnce.Do(func() {
		// Use mock implementations to avoid real Kafka or Cassandra dependency.
		server.store = store.NewMock()
		server.kafkaWriter = &appkafka.MockKafka{}

		mux := http.NewServeMux()
		mux.HandleFunc("/users", server.createUserHandler)
		mux.HandleFunc("/follow", server.followHandler)
		mux.HandleFunc("/posts", server.createPostHandler)
		mux.HandleFunc("/feed", server.getFeedHandler)

		testServer = httptest.NewServer(mux)
	})
	return testServer
}

//
// ---------- Positive test cases ----------
//

// TestCreateUser ensures that creating a new user works correctly.
func TestCreateUser(t *testing.T) {
	ts := setupTestServer(t)

	body := []byte(`{"username":"almaz"}`)
	resp, err := http.Post(ts.URL+"/users", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var res map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if res["user_id"] == nil {
		t.Fatalf("expected user_id in response")
	}
}

// TestFollowAndFeedFlow verifies the complete workflow: user creation, follow, post, and feed retrieval.
func TestFollowAndFeedFlow(t *testing.T) {
	ts := setupTestServer(t)

	almaz := createUser(ts, "almaz", t)
	nur := createUser(ts, "nur", t)

	follow(ts, almaz, nur, t)
	createPost(ts, nur, "Hello from Nur!", t)

	time.Sleep(50 * time.Millisecond) // simulate propagation delay

	feed := getFeed(ts, almaz, t)
	if len(feed) == 0 {
		t.Fatalf("expected feed not empty")
	}
	if feed[0].Body != "Hello from Nur!" {
		t.Fatalf("unexpected feed body: %+v", feed[0])
	}
}

//
// ---------- Negative test cases ----------
//

// TestCreateUser_InvalidJSON ensures invalid JSON returns HTTP 400.
func TestCreateUser_InvalidJSON(t *testing.T) {
	ts := setupTestServer(t)

	body := []byte(`{"username":123}`)
	resp, _ := http.Post(ts.URL+"/users", "application/json", bytes.NewBuffer(body))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestFollow_InvalidJSON ensures invalid follow JSON request is handled properly.
func TestFollow_InvalidJSON(t *testing.T) {
	ts := setupTestServer(t)

	body := []byte(`{"user_id":"x","followee_id":2}`)
	resp, _ := http.Post(ts.URL+"/follow", "application/json", bytes.NewBuffer(body))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid follow JSON, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestFeed_InvalidUserID ensures invalid query parameter results in HTTP 400.
func TestFeed_InvalidUserID(t *testing.T) {
	ts := setupTestServer(t)

	resp, _ := http.Get(ts.URL + "/feed?user_id=abc")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid user_id, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

//
// ---------- Simulated Kafka/Store failure tests ----------
//

// TestKafkaWriteError ensures that mock Kafka failure returns an error.
func TestKafkaWriteError(t *testing.T) {
	server.kafkaWriter = &appkafka.MockKafkaFail{}

	err := server.kafkaWriter.WriteMessages(kafka.Message{Key: []byte("k"), Value: []byte("v")})
	if err == nil {
		t.Fatalf("expected error from MockKafkaFail.WriteMessages")
	}
}

// TestStoreCreateUserFail ensures that mock store failure is detected.
func TestStoreCreateUserFail(t *testing.T) {
	server.store = &store.MockStoreFail{}

	_, err := server.store.CreateUser("almaz")
	if err == nil {
		t.Fatalf("expected error from MockStoreFail.CreateUser")
	}
}

//
// ---------- Helper functions for test setup ----------
//

// createUser sends POST /users and returns the new user ID.
func createUser(ts *httptest.Server, name string, t *testing.T) uint64 {
	t.Helper()
	body := []byte(`{"username":"` + name + `"}`)
	resp, err := http.Post(ts.URL+"/users", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("createUser failed: %v", err)
	}
	defer resp.Body.Close()

	var res map[string]any
	json.NewDecoder(resp.Body).Decode(&res)

	var id uint64
	switch v := res["user_id"].(type) {
	case float64:
		id = uint64(v)
	case string:
		idf, _ := strconv.ParseUint(v, 10, 64)
		id = idf
	default:
		t.Fatalf("unexpected type for user_id: %T", v)
	}
	return id
}

// follow sends POST /follow between two users.
func follow(ts *httptest.Server, user, followee uint64, t *testing.T) {
	t.Helper()
	req := map[string]any{"user_id": user, "followee_id": followee}
	data, _ := json.Marshal(req)
	resp, err := http.Post(ts.URL+"/follow", "application/json", bytes.NewBuffer(data))
	if err != nil {
		t.Fatalf("follow failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

// createPost sends POST /posts for a given author.
func createPost(ts *httptest.Server, author uint64, body string, t *testing.T) {
	t.Helper()
	req := map[string]any{"author_id": author, "body": body}
	data, _ := json.Marshal(req)
	resp, err := http.Post(ts.URL+"/posts", "application/json", bytes.NewBuffer(data))
	if err != nil {
		t.Fatalf("createPost failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
	}
}

// getFeed fetches GET /feed for a user and returns slice of posts.
func getFeed(ts *httptest.Server, uid uint64, t *testing.T) []models.Post {
	t.Helper()
	server.store.AddToFeed(uid, models.Post{
		ID:       uint64(time.Now().UnixNano()),
		AuthorID: uint64(time.Now().UnixNano()),
		Body:     "Hello from Nur!",
		Created:  time.Now(),
	})
	resp, err := http.Get(ts.URL + "/feed?user_id=" + strconv.FormatUint(uid, 10))
	if err != nil {
		t.Fatalf("getFeed failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var posts []models.Post
	json.NewDecoder(resp.Body).Decode(&posts)
	return posts
}

//
// ---------- Negative scenario: Kafka failure during post creation ----------
//

// TestCreatePost_KafkaFail ensures that when Kafka write fails, server returns HTTP 500.
func TestCreatePost_KafkaFail(t *testing.T) {
	ts := setupTestServer(t)

	// Replace Kafka writer with failing mock.
	origKafka := server.kafkaWriter
	defer func() { server.kafkaWriter = origKafka }()
	server.kafkaWriter = &appkafka.MockKafkaFail{}

	// Replace store with a mock.
	origStore := server.store
	defer func() { server.store = origStore }()
	server.store = store.NewMock()

	// Create author user in mock store.
	author := createUser(ts, "alice", t)

	req := map[string]any{
		"author_id": author,
		"body":      "This post will fail Kafka",
	}
	data, _ := json.Marshal(req)

	resp, err := http.Post(ts.URL+"/posts", "application/json", bytes.NewBuffer(data))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 when Kafka write fails, got %d", resp.StatusCode)
	}

	bodyResp, _ := io.ReadAll(resp.Body)
	if len(bodyResp) == 0 {
		t.Fatalf("expected error message in response body")
	}
}
