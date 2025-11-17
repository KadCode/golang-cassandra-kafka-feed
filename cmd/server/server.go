package server

import (
	"context"
	"net/http"
	"time"

	appkafka "example.com/cassandrafeed/internal/broker"
	"example.com/cassandrafeed/internal/logger"
	"example.com/cassandrafeed/internal/middleware"
	"example.com/cassandrafeed/internal/store"
)

type Server struct {
	store       store.StoreInterface
	kafkaWriter appkafka.KafkaWriter
}

var logg = logger.New()

// Run starts the HTTPS server with JWT-protected routes and graceful shutdown.
func Run(ctx context.Context, st store.StoreInterface, writer appkafka.KafkaWriter, addr string) {
	s := &Server{
		store:       st,
		kafkaWriter: writer,
	}

	// --- HTTP routes ---
	mux := http.NewServeMux()

	// Protected endpoints with JWT authentication middleware
	mux.Handle("/posts", middleware.JWTAuth(http.HandlerFunc(s.createPostHandler)))
	mux.Handle("/follow", middleware.JWTAuth(http.HandlerFunc(s.followHandler)))
	mux.Handle("/feed", middleware.JWTAuth(http.HandlerFunc(s.getFeedHandler)))

	// Public endpoint for user registration (no JWT required)
	mux.Handle("/users", http.HandlerFunc(s.createUserHandler))

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second, // prevent slowloris attacks
		WriteTimeout: 10 * time.Second,
	}

	// --- Start server in a goroutine ---
	go func() {
		logg.Info("server", "Starting HTTPS server on "+addr)
		// TLS: cert.pem and key.pem should be valid certificates in specified paths
		if err := srv.ListenAndServeTLS("/certs/cert.pem", "/certs/key.pem"); err != nil && err != http.ErrServerClosed {
			logg.Error("server", "Server stopped unexpectedly", err)
		}
	}()

	// --- Graceful shutdown ---
	<-ctx.Done()
	logg.Info("server", "Shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logg.Error("server", "Error during server shutdown", err)
	} else {
		logg.Info("server", "Server stopped gracefully")
	}
}
