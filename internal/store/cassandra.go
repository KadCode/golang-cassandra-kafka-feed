package store

import (
	"fmt"
	"path/filepath"

	config "example.com/cassandrafeed/internal/init"
	"example.com/cassandrafeed/internal/logger"
	"example.com/cassandrafeed/internal/models"
	"github.com/gocql/gocql"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/cassandra"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

var logg = logger.New()

// --- Interfaces ---

type SessionInterface interface {
	Query(stmt string, values ...interface{}) *gocql.Query
	NewBatch(batchType gocql.BatchType) *gocql.Batch
	ExecuteBatch(batch *gocql.Batch) error
	Close()
}

type StoreInterface interface {
	CreateUser(username string) (string, error)
	CreateFollow(userId, followeeId string) error
	GetFollowers(userId string) ([]string, error)
	GetUserIDByUsername(username string) (string, error)
	AddPost(post models.Post) error
	AddToFeed(userId string, post models.Post) error
	GetFeed(userId string, limit int) ([]models.Post, error)
	Close()
}

// --- Store Implementation ---

type Store struct {
	Session SessionInterface
}

// New initializes Cassandra connection using config package.
func New() (StoreInterface, error) {
	cfg := config.Get()

	if err := ensureKeyspace(cfg); err != nil {
		return nil, fmt.Errorf("failed to ensure keyspace: %w", err)
	}

	if err := runMigrations(cfg); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	cluster := gocql.NewCluster(cfg.CassandraHost)
	cluster.Keyspace = cfg.CassandraKeyspace
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = cfg.CassandraTimeout
	cluster.ConnectTimeout = cfg.CassandraTimeout

	if cfg.CassandraUsername != "" && cfg.CassandraPassword != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: cfg.CassandraUsername,
			Password: cfg.CassandraPassword,
		}
	}

	if cfg.CassandraDC != "" {
		cluster.HostFilter = gocql.DataCentreHostFilter(cfg.CassandraDC)
	}

	sess, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create Cassandra session: %w", err)
	}

	logg.Info("store", "Connected to Cassandra keyspace (host anonymized)")
	return &Store{Session: sess}, nil
}

// --- Ensure keyspace exists before migrations ---

func ensureKeyspace(cfg *config.Config) error {
	cluster := gocql.NewCluster(cfg.CassandraHost)
	cluster.Keyspace = "system"
	sess, err := cluster.CreateSession()
	if err != nil {
		return fmt.Errorf("failed to connect to Cassandra system keyspace: %w", err)
	}
	defer sess.Close()

	query := fmt.Sprintf(`
        CREATE KEYSPACE IF NOT EXISTS %s
        WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};
    `, cfg.CassandraKeyspace)

	if err := sess.Query(query).Exec(); err != nil {
		return fmt.Errorf("failed to create keyspace: %w", err)
	}

	logg.Info("store", "Ensured Cassandra keyspace exists (keyspace name anonymized)")
	return nil
}

// --- Migration runner ---

func runMigrations(cfg *config.Config) error {
	migrationsPath := filepath.Join("./migrations/cassandra")
	sourceURL := fmt.Sprintf("file://%s", migrationsPath)
	dbURL := fmt.Sprintf(
		"cassandra://%s/%s?x-migrations-table=schema_migrations&x-multi-statement=true",
		cfg.CassandraHost, cfg.CassandraKeyspace,
	)

	m, err := migrate.New(sourceURL, dbURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration up failed: %w", err)
	}

	if err == migrate.ErrNoChange {
		logg.Info("store", "No new migrations to apply")
	} else {
		logg.Info("store", "Migrations applied successfully")
	}
	return nil
}

// Close gracefully closes Cassandra session.
func (s *Store) Close() {
	if s.Session != nil {
		s.Session.Close()
		logg.Info("store", "Cassandra session closed")
	}
}
