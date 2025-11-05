package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"example.com/cassandrafeed/cmd/server"
	"example.com/cassandrafeed/cmd/worker"
	appkafka "example.com/cassandrafeed/internal/broker"
	config "example.com/cassandrafeed/internal/init"
	"example.com/cassandrafeed/internal/store"
)

func main() {
	// Initialize application configuration
	cfg := config.Init()
	mode := cfg.Mode

	// Initialize Cassandra store connection
	st, err := store.New()
	if err != nil {
		log.Fatalf("Cassandra connection failed: %v", err)
	}
	defer st.Close()

	// Configure Kafka client parameters
	kafkaCfg := appkafka.KafkaConfig{
		Brokers:      []string{cfg.KafkaBroker},
		Topic:        cfg.KafkaTopic,
		Partition:    cfg.KafkaPartition,
		GroupID:      cfg.KafkaGroupID,
		WriteTimeout: cfg.KafkaWriteTO,
		ReadTimeout:  cfg.KafkaReadTO,
	}

	var kafkaWriter appkafka.KafkaWriter
	var kafkaReader appkafka.KafkaReader

	// Initialize Kafka writer for server mode
	if mode == "server" {
		kafkaWriter, err = appkafka.NewKafkaWriter(kafkaCfg)
		if err != nil {
			log.Fatalf("Kafka writer init failed: %v", err)
		}
		defer kafkaWriter.Close()
	} else {
		// Initialize Kafka reader for worker mode
		kafkaReader = appkafka.NewKafkaReader(kafkaCfg)
		defer kafkaReader.Close()
	}

	// Setup OS signal handling for graceful shutdown (SIGINT, SIGTERM)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run application depending on selected mode
	switch mode {
	case "server":
		// Start the server that writes posts to Kafka
		server.Run(ctx, st, kafkaWriter, cfg.ServerAddr)
	case "worker":
		// Start the worker that reads posts from Kafka and processes them
		w := worker.New(st, kafkaReader, 0, 0)
		w.Run(ctx)
	default:
		log.Fatalf("unknown mode: %s", mode)
	}

	log.Println("Shutdown completed")
}
