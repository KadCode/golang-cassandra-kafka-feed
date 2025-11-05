package store

import (
	"errors"
	"fmt"

	"example.com/cassandrafeed/internal/models"
)

var mockUserCounter int

// MockStore simulates Cassandra operations for testing.
type MockStore struct {
	Users      map[string]string
	Followers  map[string][]string
	Feed       map[string][]models.Post
	Posts      map[string]models.Post
	ShouldFail bool // flag to simulate failures
}

// NewMock initializes a new mock store
func NewMock() *MockStore {
	return &MockStore{
		Users:     make(map[string]string),
		Followers: make(map[string][]string),
		Feed:      make(map[string][]models.Post),
		Posts:     make(map[string]models.Post),
	}
}

func (m *MockStore) Close() {}

// CreateUser simulates creating a new user
func (m *MockStore) CreateUser(username string) (string, error) {
	if m.ShouldFail {
		return "", errors.New("mock: create user failed")
	}
	mockUserCounter++
	id := fmt.Sprintf("user_%d", mockUserCounter)
	m.Users[id] = username
	return id, nil
}

// CreateFollow simulates creating a follow relationship
func (m *MockStore) CreateFollow(followerID, followeeID string) error {
	if m.ShouldFail {
		return errors.New("mock: follow failed")
	}
	// Key is followeeID so that GetFollowers(followeeID) returns the followerID
	m.Followers[followeeID] = append(m.Followers[followeeID], followerID)
	return nil
}

// GetFollowers returns all followers of a given user
func (m *MockStore) GetFollowers(userID string) ([]string, error) {
	if m.ShouldFail {
		return nil, errors.New("mock: get followers failed")
	}
	return m.Followers[userID], nil
}

// AddPost simulates adding a post
func (m *MockStore) AddPost(post models.Post) error {
	if m.ShouldFail {
		return errors.New("mock: add post failed")
	}
	m.Posts[post.ID] = post
	return nil
}

// AddToFeed simulates adding a post to a user's feed
func (m *MockStore) AddToFeed(userID string, post models.Post) error {
	if m.ShouldFail {
		return errors.New("mock: add to feed failed")
	}
	m.Feed[userID] = append(m.Feed[userID], post)
	return nil
}

// GetFeed retrieves a user's feed with an optional limit
func (m *MockStore) GetFeed(userID string, limit int) ([]models.Post, error) {
	if m.ShouldFail {
		return nil, errors.New("mock: get feed failed")
	}
	posts := m.Feed[userID]
	if len(posts) > limit {
		return posts[:limit], nil
	}
	return posts, nil
}

// GetUserIDByUsername returns the user ID for a given username
func (m *MockStore) GetUserIDByUsername(username string) (string, error) {
	for id, u := range m.Users {
		if u == username {
			return id, nil
		}
	}
	return "", nil
}

// ---------------------------------------------
// MockStoreFail always returns errors for negative tests
type MockStoreFail struct{}

func (m *MockStoreFail) Close() {}

func (m *MockStoreFail) CreateUser(username string) (string, error) {
	return "", errors.New("mock store create user failed")
}

func (m *MockStoreFail) CreateFollow(followerID, followeeID string) error {
	return errors.New("mock store create follow failed")
}

func (m *MockStoreFail) GetUserIDByUsername(username string) (string, error) {
	return "", errors.New("mock store get user by username failed")
}

func (m *MockStoreFail) GetFollowers(userID string) ([]string, error) {
	return nil, errors.New("mock store get followers failed")
}

func (m *MockStoreFail) AddPost(post models.Post) error {
	return errors.New("mock store add post failed")
}

func (m *MockStoreFail) AddToFeed(userID string, post models.Post) error {
	return errors.New("mock store add to feed failed")
}

func (m *MockStoreFail) GetFeed(userID string, limit int) ([]models.Post, error) {
	return nil, errors.New("mock store get feed failed")
}
