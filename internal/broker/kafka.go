package appkafka

import (
	"context"
	"errors"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaWriter defines an interface for writing messages to Kafka.
type KafkaWriter interface {
	WriteMessages(messages ...kafka.Message) error
	Close() error
}

// KafkaReader defines an interface for reading messages from Kafka.
type KafkaReader interface {
	ReadMessage(ctx context.Context) (kafka.Message, error)
	Close() error
}

// KafkaConfig holds configuration parameters for Kafka.
type KafkaConfig struct {
	Brokers      []string      // list of Kafka brokers
	Topic        string        // topic name
	Partition    int           // partition number (used for low-level writes)
	WriteTimeout time.Duration // write timeout duration
	ReadTimeout  time.Duration // read timeout duration (used for consumer group)
	GroupID      string        // consumer group ID
}

// RealKafkaWriter implements KafkaWriter using kafka.Conn (low-level writes).
type RealKafkaWriter struct {
	conn   *kafka.Conn
	config KafkaConfig
}

// NewKafkaWriter creates a new Kafka writer connection.
func NewKafkaWriter(cfg KafkaConfig) (*RealKafkaWriter, error) {
	if len(cfg.Brokers) == 0 {
		cfg.Brokers = []string{"localhost:9092"}
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 10 * time.Second
	}

	conn, err := kafka.DialLeader(context.Background(), "tcp", cfg.Brokers[0], cfg.Topic, cfg.Partition)
	if err != nil {
		return nil, err
	}

	return &RealKafkaWriter{
		conn:   conn,
		config: cfg,
	}, nil
}

func (w *RealKafkaWriter) WriteMessages(messages ...kafka.Message) error {
	if w.conn == nil {
		return errors.New("kafka connection is nil")
	}
	w.conn.SetWriteDeadline(time.Now().Add(w.config.WriteTimeout))
	_, err := w.conn.WriteMessages(messages...)
	return err
}

func (w *RealKafkaWriter) Close() error {
	if w.conn != nil {
		return w.conn.Close()
	}
	return nil
}

// RealKafkaReader implements KafkaReader using kafka.Reader (consumer group).
type RealKafkaReader struct {
	reader *kafka.Reader
}

// NewKafkaReader creates a new Kafka consumer group reader.
func NewKafkaReader(cfg KafkaConfig) KafkaReader {
	if len(cfg.Brokers) == 0 {
		cfg.Brokers = []string{"localhost:9092"}
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.Topic,
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
	})
	return &RealKafkaReader{reader: r}
}

func (r *RealKafkaReader) ReadMessage(ctx context.Context) (kafka.Message, error) {
	return r.reader.ReadMessage(ctx)
}

func (r *RealKafkaReader) Close() error {
	return r.reader.Close()
}
