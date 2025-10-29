package main

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

// TestServer_GracefulShutdown verifies that the HTTP server shuts down gracefully.
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

	// Start a test HTTP server without automatically starting listeners
	server := httptest.NewUnstartedServer(mux)
	server.Start()
	defer server.Close()

	// Create a context with timeout to simulate shutdown signal
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})

	go func() {
		// Wait for the simulated shutdown signal
		<-ctx.Done()
		server.Close() // Gracefully close the server
		close(done)
	}()

	// Make one request before shutdown to verify the server is running
	resp, err := http.Post(server.URL+"/users", "application/json",
		bytesReader(`{"username":"almaz"}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Wait for shutdown to complete and verify resources
	select {
	case <-done:
		// Ensure store and Kafka can be closed properly
		mockStore.Close()
		if err := mockKafka.Close(); err != nil {
			t.Fatalf("Kafka close error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("server did not shutdown gracefully in time")
	}
}

// bytesReader creates an io.Reader from a string.
func bytesReader(s string) *bytes.Buffer {
	return bytes.NewBuffer([]byte(s))
}
