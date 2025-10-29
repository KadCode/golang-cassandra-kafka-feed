# 🧩 Golang Microservice — Cassandra + Kafka Feed System

This project is a microservice-based feed system built with **Go**, **Apache Kafka**, and **Cassandra**.
It consists of two main services: **Server** and **Worker**, using **Kafka** for message passing and **Cassandra** for data storage.

---

## ⚙️ Architecture

```
[Client]
   │
   ▼
[Server API] ─────► [Kafka Topic] ─────► [Worker Service]
   │                                     │
   │                                     ▼
   └──────────────► [Cassandra Database] ◄──────────────┘
```

### Components:

* **Server** — HTTP API for managing users, follows, and posts.
* **Worker** — consumes messages from Kafka and updates followers’ feeds.
* **Kafka** — message broker for asynchronous communication.
* **Cassandra** — database for storing users, posts, and feeds.

---

## 🏗️ Project Structure

```
cmd/
 ├── server/          # REST HTTP server
 └── worker/          # Kafka consumer service
internal/
 ├── broker/          # Kafka integration logic and mocks
 ├── models/          # Data structures (User, Post, Follow)
 └── store/           # Cassandra logic and mocks for testing
```

---

## 🚀 Running the Project

### 🔹 1. Run with Docker Compose

```bash
docker compose up --build
```

This will start:

* **Kafka** (port `9092`)
* **Cassandra** (port `9042`)
* **Server API** (port `8080`)
* **Worker** (listens to Kafka)

> Make sure your Docker Compose file creates a keyspace named `feedapp` in Cassandra.

---

### 🔹 2. Run Locally (without Docker)

1. Make sure the following are running locally:

   * **Kafka** (`localhost:9092`)
   * **Cassandra** (`localhost:9042`) with keyspace `feedapp`

2. Start the server:

   ```bash
   go run ./cmd/server
   ```

3. Start the worker in another terminal:

   ```bash
   go run ./cmd/worker
   ```

---

## 🌐 REST API

| Method | Path                           | Description                        |
| ------ | ------------------------------ | ---------------------------------- |
| `POST` | `/users`                       | Create a new user                  |
| `POST` | `/follow`                      | Follow another user                |
| `POST` | `/posts`                       | Create a post and send it to Kafka |
| `GET`  | `/feed?user_id={id}&limit={n}` | Get a user’s feed                  |

### Example Requests

**Create a User**

```bash
curl -X POST localhost:8080/users -d '{"username":"almaz"}' -H "Content-Type: application/json"
```

**Follow a User**

```bash
curl -X POST localhost:8080/follow -d '{"user_id":1, "followee_id":2}' -H "Content-Type: application/json"
```

**Create a Post**

```bash
curl -X POST localhost:8080/posts -d '{"author_id":2,"body":"Hello world!"}' -H "Content-Type: application/json"
```

**Get a Feed**

```bash
curl "localhost:8080/feed?user_id=1&limit=10"
```

---

## 🧪 Testing

The project includes unit and integration tests:

* Kafka mocks (`internal/broker/mock_kafka.go`)
* Store mocks (`internal/store/mock_store.go`)
* Tests for all services (`cmd/server/server_test.go`, `cmd/worker/worker_test.go`)

Run all tests:

```bash
go test ./... -v
```

---

## 🧰 Configuration

| Variable              | Description                                   | Default          |
| --------------------- | --------------------------------------------- | ---------------- |
| `KAFKA_BROKER`        | Kafka broker address                          | `localhost:9092` |
| `KAFKA_TOPIC`         | Kafka topic                                   | `feed-topic`     |
| `KAFKA_GROUP_ID`      | Kafka consumer group ID (used by Worker only) | `worker-group`   |
| `KAFKA_WRITE_TIMEOUT` | Write timeout for Kafka messages              | `10s`            |
| `KAFKA_READ_TIMEOUT`  | Read timeout for Kafka messages               | `10s`            |

> Note: The server writes to Kafka without using `KAFKA_GROUP_ID`. Only the worker uses the group ID.

---

## 🧩 Technologies Used

* **Golang 1.22+**
* **Apache Kafka** — message broker
* **Cassandra 4.1** — distributed database
* **gocql** — Cassandra driver
* **segmentio/kafka-go** — Kafka library
* **Docker Compose** — development environment

---

## 🧠 Core Idea

This project simulates a social media feed system:

1. A user creates a post — the server saves it in Cassandra and sends a message to Kafka.
2. The worker reads the Kafka message and adds the post to all followers’ feeds.
3. The client retrieves the feed via `/feed`.

---

## 👨‍💻 Author

**Almaz**
📧 [alkadriev@gmail.com](mailto:alkadriev@gmail.com)
📦 Repository: `golang-cassandra-kafka-feed-app`
