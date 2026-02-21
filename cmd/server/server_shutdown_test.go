package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appkafka "example.com/cassandrafeed/internal/broker"
	"example.com/cassandrafeed/internal/store"
)

// TestServer_GracefulShutdown verifies that the HTTP server shuts down gracefully
// and that associated resources (mock store and Kafka) can be closed without errors.
func TestServer_GracefulShutdown(t *testing.T) {
	// Use mock store and Kafka to avoid real dependencies
	mockStore := store.NewMock()
	mockKafka := &appkafka.MockKafka{}

	s := &Server{
		store:       mockStore,
		kafkaWriter: mockKafka,
	}

	// Register HTTP handlers for testing
	mux := http.NewServeMux()
	mux.HandleFunc("/users", s.createUserHandler)
	mux.HandleFunc("/posts", s.createPostHandler)
	mux.HandleFunc("/feed", s.getFeedHandler)

	// Start an unstarted HTTP test server to control shutdown timing
	server := httptest.NewUnstartedServer(mux)
	server.Start()
	defer server.Close()

	// Create a context with a short timeout to simulate a shutdown signal
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})

	// Wait for the simulated shutdown signal
	// Gracefully close the server
	// Signal that shutdown is complete
	go func() {
		<-ctx.Done()
		server.Close()
		close(done)
	}()

	// Make a request before shutdown to ensure the server is running
	resp, err := http.Post(server.URL+"/users", "application/json",
		bytesReader(`{"username":"almaz"}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Wait for shutdown to complete or timeout
	select {
	case <-done:
		mockStore.Close()
		if err := mockKafka.Close(); err != nil {
			t.Fatalf("Kafka close error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("server did not shutdown gracefully within the expected time")
	}
}

// bytesReader creates an io.Reader from a string, used for HTTP request bodies.
func bytesReader(s string) *bytes.Buffer {
	return bytes.NewBuffer([]byte(s))
}
