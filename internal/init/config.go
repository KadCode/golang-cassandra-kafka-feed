package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	// App mode & server
	Mode       string
	ServerAddr string

	// Kafka
	KafkaBroker    string
	KafkaTopic     string
	KafkaGroupID   string
	KafkaPartition int
	KafkaReadTO    time.Duration
	KafkaWriteTO   time.Duration

	// Cassandra
	CassandraHost     string
	CassandraKeyspace string
	CassandraUsername string
	CassandraPassword string
	CassandraTimeout  time.Duration
	CassandraDC       string
}

var cfg *Config

// Init loads the config using Viper and returns it
func Init() *Config {
	viper.SetDefault("MODE", "server")
	viper.SetDefault("SERVER_ADDR", ":8080")

	viper.SetDefault("KAFKA_BROKER", "localhost:29092")
	viper.SetDefault("KAFKA_TOPIC", "feed-topic")
	viper.SetDefault("KAFKA_GROUP_ID", "worker-group")
	viper.SetDefault("KAFKA_PARTITION", 0)
	viper.SetDefault("KAFKA_READ_TIMEOUT", "10s")
	viper.SetDefault("KAFKA_WRITE_TIMEOUT", "10s")

	viper.SetDefault("CASSANDRA_HOST", "localhost")
	viper.SetDefault("CASSANDRA_KEYSPACE", "feedapp")
	viper.SetDefault("CASSANDRA_TIMEOUT", "10s")
	// Optional: Cassandra username/password/DC can be empty

	// Load env variables
	viper.AutomaticEnv()

	// Optional config file support
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	_ = viper.ReadInConfig() // ignore error if no file

	cfg = &Config{
		Mode:              viper.GetString("MODE"),
		ServerAddr:        viper.GetString("SERVER_ADDR"),
		KafkaBroker:       viper.GetString("KAFKA_BROKER"),
		KafkaTopic:        viper.GetString("KAFKA_TOPIC"),
		KafkaGroupID:      viper.GetString("KAFKA_GROUP_ID"),
		KafkaPartition:    viper.GetInt("KAFKA_PARTITION"),
		KafkaReadTO:       parseDuration(viper.GetString("KAFKA_READ_TIMEOUT"), 10*time.Second),
		KafkaWriteTO:      parseDuration(viper.GetString("KAFKA_WRITE_TIMEOUT"), 10*time.Second),
		CassandraHost:     viper.GetString("CASSANDRA_HOST"),
		CassandraKeyspace: viper.GetString("CASSANDRA_KEYSPACE"),
		CassandraUsername: viper.GetString("CASSANDRA_USERNAME"),
		CassandraPassword: viper.GetString("CASSANDRA_PASSWORD"),
		CassandraTimeout:  parseDuration(viper.GetString("CASSANDRA_TIMEOUT"), 10*time.Second),
		CassandraDC:       viper.GetString("CASSANDRA_DC"),
	}

	return cfg
}

func parseDuration(s string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}

// Get returns the loaded config instance
func Get() *Config {
	return cfg
}
