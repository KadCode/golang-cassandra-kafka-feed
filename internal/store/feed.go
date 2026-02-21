package store

import (
	"time"

	"example.com/cassandrafeed/internal/models"
	"github.com/gocql/gocql"
)

// --- User operations ---

// GetUserIDByUsername returns the existing user_id by username.
// If the user does not exist, it returns empty string without an error.
func (s *Store) GetUserIDByUsername(username string) (string, error) {
	var id string
	err := s.Session.Query(
		`SELECT user_id FROM users_by_username WHERE username = ?`,
		username,
	).Scan(&id)
	if err != nil {
		if err == gocql.ErrNotFound {
			return "", nil
		}
		logg.Error("store", "Failed to query user by username", err)
		return "", err
	}
	return id, nil
}

// CreateUser creates a new user if the username does not exist.
// Returns the existing user_id if username already exists.
func (s *Store) CreateUser(username string) (string, error) {
	existingID, err := s.GetUserIDByUsername(username)
	if err != nil {
		return "", err
	}
	if existingID != "" {
		return existingID, nil
	}

	// Generate a new UUID for user_id
	id := gocql.TimeUUID().String()

	// Insert into users_by_username table using CAS
	result := make(map[string]interface{})
	applied, err := s.Session.Query(`
		INSERT INTO users_by_username (username, user_id)
		VALUES (?, ?) IF NOT EXISTS`,
		username, id,
	).MapScanCAS(result)
	if err != nil {
		logg.Error("store", "Failed to create username entry", err)
		return "", err
	}

	if !applied {
		// Another process already created this user
		return s.GetUserIDByUsername(username)
	}

	// Insert into main users table
	err = s.Session.Query(`
		INSERT INTO users (user_id, username)
		VALUES (?, ?)`,
		id, username,
	).Exec()
	if err != nil {
		logg.Error("store", "Failed to create user in main table", err)
		return "", err
	}

	logg.Info("store", "User created successfully (username anonymized)")
	return id, nil
}

// --- Follow operations ---

func (s *Store) CreateFollow(userID, followeeID string) error {
	batch := s.Session.NewBatch(gocql.LoggedBatch)
	batch.Query(`INSERT INTO follows (user_id, followee_id) VALUES (?, ?)`, userID, followeeID)
	batch.Query(`INSERT INTO followers_by_followee (followee_id, user_id) VALUES (?, ?)`, followeeID, userID)

	if err := s.Session.ExecuteBatch(batch); err != nil {
		logg.Error("store", "Failed to create follow relationship", err)
		return err
	}

	logg.Info("store", "Follow relationship created (user IDs anonymized)")
	return nil
}

func (s *Store) GetFollowers(userID string) ([]string, error) {
	iter := s.Session.Query(
		`SELECT user_id FROM followers_by_followee WHERE followee_id = ?`,
		userID,
	).Iter()

	var id string
	var res []string
	for iter.Scan(&id) {
		res = append(res, id)
	}

	if err := iter.Close(); err != nil {
		logg.Error("store", "Failed to get followers", err)
		return nil, err
	}

	logg.Info("store", "Retrieved followers (user IDs anonymized)")
	return res, nil
}

// --- Post operations ---

func (s *Store) AddPost(post models.Post) error {
	if err := s.Session.Query(`
		INSERT INTO posts (post_id, author_id, body, created_at)
		VALUES (?, ?, ?, ?)`,
		post.ID, post.AuthorID, post.Body, post.Created,
	).Exec(); err != nil {
		logg.Error("store", "Failed to add post", err)
		return err
	}

	logg.Info("store", "Post added to posts table (post content anonymized)")
	return nil
}

func (s *Store) AddToFeed(userID string, post models.Post) error {
	if err := s.Session.Query(`
		INSERT INTO feed_by_user (user_id, post_id, author_id, body, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		userID, post.ID, post.AuthorID, post.Body, post.Created,
	).Exec(); err != nil {
		logg.Error("store", "Failed to add post to feed", err)
		return err
	}

	logg.Info("store", "Post added to user's feed (IDs and content anonymized)")
	return nil
}

func (s *Store) GetFeed(userID string, limit int) ([]models.Post, error) {
	iter := s.Session.Query(`
		SELECT post_id, author_id, body, created_at
		FROM feed_by_user WHERE user_id = ? LIMIT ?`,
		userID, limit,
	).Iter()

	var res []models.Post
	var pid, aid string
	var body string
	var created time.Time

	for iter.Scan(&pid, &aid, &body, &created) {
		res = append(res, models.Post{
			ID:       pid,
			AuthorID: aid,
			Body:     body,
			Created:  created,
		})
	}

	if err := iter.Close(); err != nil {
		logg.Error("store", "Failed to retrieve user feed", err)
		return nil, err
	}

	logg.Info("store", "User feed retrieved successfully (IDs and content anonymized)")
	return res, nil
}
