package store

import (
	"time"

	"example.com/cassandrafeed/internal/models"
	"github.com/gocql/gocql"
)

// --- Interfaces ---
type SessionInterface interface {
	Query(stmt string, values ...interface{}) *gocql.Query
	NewBatch(batchType gocql.BatchType) *gocql.Batch
	ExecuteBatch(batch *gocql.Batch) error
	Close()
}

type StoreInterface interface {
	CreateUser(username string) (uint64, error)
	CreateFollow(userId, followeeId uint64) error
	GetFollowers(userId uint64) ([]uint64, error)
	AddPost(post models.Post) error
	AddToFeed(userId uint64, post models.Post) error
	GetFeed(userId uint64, limit int) ([]models.Post, error)
	Close()
}

// --- Store Implementation ---
type Store struct {
	Session SessionInterface
}

func New() (StoreInterface, error) {
	cluster := gocql.NewCluster("localhost")
	cluster.Keyspace = "feedapp"
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 120 * time.Second

	sess, err := cluster.CreateSession()
	if err != nil {
		return nil, err
	}

	return &Store{Session: sess}, nil
}

func (s *Store) Close() {
	if s.Session != nil {
		s.Session.Close()
	}
}
