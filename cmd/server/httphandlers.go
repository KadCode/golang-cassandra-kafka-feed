package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"example.com/cassandrafeed/internal/middleware"
	"example.com/cassandrafeed/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

// --- HTTP Handlers ---

// createUserHandler handles POST requests to create a new user.
// Expects JSON body: {"username": "example"}
// Returns JSON response: {"user_id": <id>}
func (s *Server) createUserHandler(w http.ResponseWriter, r *http.Request) {
	type req struct{ Username string }
	var body req

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		logg.Error("http/users", "Invalid request body", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if len(body.Username) == 0 || len(body.Username) > 50 {
		logg.Info("http/users", "Invalid username length")
		http.Error(w, "username must be 1-50 characters", http.StatusBadRequest)
		return
	}

	userID, err := s.store.GetUserIDByUsername(body.Username)
	if err != nil {
		logg.Error("http/users", "Failed to query existing username", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if userID == "" {
		userID, err = s.store.CreateUser(body.Username)
		if err != nil {
			logg.Error("http/users", "Failed to create user", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		logg.Info("http/users", "User created successfully with user_id="+userID)
	} else {
		logg.Info("http/users", "User already exists, returning existing user_id="+userID)
	}

	secret := []byte(os.Getenv("JWT_SECRET"))
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	})
	tokenStr, err := token.SignedString(secret)
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"user_id": userID,
		"token":   tokenStr,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// followHandler creates a "follow" relationship between users.
// Expects JSON body: {"followee_id": 2}
// Uses user_id from JWT token.
func (s *Server) followHandler(w http.ResponseWriter, r *http.Request) {
	type req struct {
		FolloweeID string `json:"followee_id"`
	}
	var body req

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		logg.Error("http/follow", "Invalid request body", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		logg.Info("http/follow", "Unauthorized follow attempt")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := s.store.CreateFollow(userID, body.FolloweeID); err != nil {
		logg.Error("http/follow", "Failed to create follow relationship", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logg.Info("http/follow", "User "+userID+" followed "+body.FolloweeID)
	w.WriteHeader(http.StatusOK)
}

// createPostHandler handles post creation, stores it in Cassandra, and publishes an event to Kafka.
// Expects JSON body: {"body": "post content"}
// Returns JSON response with created post data.
func (s *Server) createPostHandler(w http.ResponseWriter, r *http.Request) {
	type req struct {
		Body string `json:"body"`
	}
	var body req

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		logg.Error("http/posts", "Invalid request body", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		logg.Info("http/posts", "Unauthorized post creation attempt")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if len(body.Body) == 0 || len(body.Body) > 1000 {
		logg.Info("http/posts", "Post body length invalid for user_id="+userID)
		http.Error(w, "post body must be 1-1000 characters", http.StatusBadRequest)
		return
	}

	post := models.Post{
		ID:       uuid.NewString(),
		AuthorID: userID,
		Body:     body.Body,
		Created:  time.Now(),
	}

	data, err := json.Marshal(post)
	if err != nil {
		logg.Error("http/posts", "Failed to marshal post", err)
		http.Error(w, "failed to marshal post", http.StatusInternalServerError)
		return
	}

	// Create Kafka message for post creation event.
	msg := kafka.Message{
		Key:   []byte("post_created"),
		Value: data,
	}

	if err := s.kafkaWriter.WriteMessages(msg); err != nil {
		logg.Error("http/posts", "Failed to write Kafka message", err)
		http.Error(w, "failed to write Kafka message: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.store.AddPost(post); err != nil {
		logg.Error("http/posts", "Failed to save post to Cassandra", err)
		http.Error(w, "failed to save post: "+err.Error(), http.StatusInternalServerError)
		return
	}

	logg.Info("http/posts", "Post created successfully by user_id="+userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

// getFeedHandler retrieves a user's feed based on their user ID.
// Query parameters: ?limit=50
// Uses user_id from JWT token.
func (s *Server) getFeedHandler(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		logg.Info("http/feed", "Unauthorized feed access attempt")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	feed, err := s.store.GetFeed(userID, limit)
	if err != nil {
		logg.Error("http/feed", "Failed to get feed for user_id="+userID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logg.Info("http/feed", "Feed retrieved for user_id="+userID+" with limit="+strconv.Itoa(limit))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(feed)
}
