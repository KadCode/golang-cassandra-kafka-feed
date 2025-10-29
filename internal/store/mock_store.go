package store

import (
	"errors"
	"time"

	"example.com/cassandrafeed/internal/models"
)

// MockStore simulates Cassandra operations for testing.
type MockStore struct {
	Users      map[uint64]string
	Followers  map[uint64][]uint64
	Feed       map[uint64][]models.Post
	Posts      map[uint64]models.Post
	ShouldFail bool // to simulate failures
}

type MockStoreFail struct{}

// Close implements StoreInterface.
func (m *MockStore) Close() {
}

// NewMock creates an empty mock store.
func NewMock() *MockStore {
	return &MockStore{
		Users:     make(map[uint64]string),
		Followers: make(map[uint64][]uint64),
		Feed:      make(map[uint64][]models.Post),
		Posts:     make(map[uint64]models.Post),
	}
}

// --- Methods ---

func (m *MockStore) CreateUser(username string) (uint64, error) {
	if m.ShouldFail {
		return 0, errors.New("mock: create user failed")
	}
	id := uint64(time.Now().UnixNano())
	m.Users[id] = username
	return id, nil
}

func (m *MockStore) CreateFollow(userId, followeeId uint64) error {
	if m.ShouldFail {
		return errors.New("mock: follow failed")
	}
	m.Followers[followeeId] = append(m.Followers[followeeId], userId)
	return nil
}

func (m *MockStore) GetFollowers(userId uint64) ([]uint64, error) {
	if m.ShouldFail {
		return nil, errors.New("mock: get followers failed")
	}
	return m.Followers[userId], nil
}

func (m *MockStore) AddPost(post models.Post) error {
	if m.ShouldFail {
		return errors.New("mock: add post failed")
	}
	m.Posts[post.ID] = post
	return nil
}

func (m *MockStore) AddToFeed(userId uint64, post models.Post) error {
	if m.ShouldFail {
		return errors.New("mock: add to feed failed")
	}
	m.Feed[userId] = append(m.Feed[userId], post)
	return nil
}

func (m *MockStore) GetFeed(userId uint64, limit int) ([]models.Post, error) {
	if m.ShouldFail {
		return nil, errors.New("mock: get feed failed")
	}
	posts := m.Feed[userId]
	if len(posts) > limit {
		return posts[:limit], nil
	}
	return posts, nil
}

// MockStoreFail simulates a store that always fails
func (m *MockStoreFail) CreateUser(username string) (uint64, error) {
	return 0, errors.New("mock store create user failed")
}
func (m *MockStoreFail) CreateFollow(userId, followeeId uint64) error {
	return errors.New("mock store create follow failed")
}
func (m *MockStoreFail) GetFollowers(userId uint64) ([]uint64, error) {
	return nil, errors.New("mock store get followers failed")
}
func (m *MockStoreFail) AddPost(post models.Post) error {
	return errors.New("mock store add post failed")
}
func (m *MockStoreFail) AddToFeed(userId uint64, post models.Post) error {
	return errors.New("mock store add to feed failed")
}
func (m *MockStoreFail) GetFeed(userId uint64, limit int) ([]models.Post, error) {
	return nil, errors.New("mock store get feed failed")
}
func (m *MockStoreFail) Close() {}
