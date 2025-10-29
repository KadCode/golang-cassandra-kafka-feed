package store

import (
	"time"

	"example.com/cassandrafeed/internal/models"
)

func (s *Store) CreateUser(username string) (uint64, error) {
	id := uint64(time.Now().UnixNano())
	return id, s.Session.Query(
		`INSERT INTO users (user_id, username) VALUES (?, ?)`,
		id, username,
	).Exec()
}

// Create a follow relationship and mirror the record in both tables
func (s *Store) CreateFollow(userId, followeeId uint64) error {
	batch := s.Session.NewBatch(0) // logged batch
	batch.Query(`INSERT INTO follows (user_id, followee_id) VALUES (?, ?)`, userId, followeeId)
	batch.Query(`INSERT INTO followers_by_followee (followee_id, user_id) VALUES (?, ?)`, followeeId, userId)
	return s.Session.ExecuteBatch(batch)
}

// Use the correct table without ALLOW FILTERING
func (s *Store) GetFollowers(userId uint64) ([]uint64, error) {
	iter := s.Session.Query(
		`SELECT user_id FROM followers_by_followee WHERE followee_id = ?`,
		userId,
	).Iter()

	var id uint64
	var res []uint64
	for iter.Scan(&id) {
		res = append(res, id)
	}
	return res, iter.Close()
}

// Add a post to the posts table
func (s *Store) AddPost(post models.Post) error {
	return s.Session.Query(`
		INSERT INTO posts (post_id, author_id, body, created_at)
		VALUES (?, ?, ?, ?)`,
		post.ID, post.AuthorID, post.Body, post.Created,
	).Exec()
}

// Add a post to a user's feed
func (s *Store) AddToFeed(userId uint64, post models.Post) error {
	return s.Session.Query(`
		INSERT INTO feed_by_user (user_id, post_id, author_id, body, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		userId, post.ID, post.AuthorID, post.Body, post.Created,
	).Exec()
}

// Get a user's feed with a limit on the number of posts
func (s *Store) GetFeed(userId uint64, limit int) ([]models.Post, error) {
	iter := s.Session.Query(`
		SELECT post_id, author_id, body, created_at
		FROM feed_by_user WHERE user_id = ? LIMIT ?`,
		userId, limit).Iter()

	var res []models.Post
	var pid, aid int64
	var body string
	var created time.Time

	for iter.Scan(&pid, &aid, &body, &created) {
		res = append(res, models.Post{
			ID:       uint64(pid),
			AuthorID: uint64(aid),
			Body:     body,
			Created:  created,
		})
	}
	return res, iter.Close()
}
