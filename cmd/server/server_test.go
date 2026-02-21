package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	appkafka "example.com/cassandrafeed/internal/broker"
	"example.com/cassandrafeed/internal/middleware"
	"example.com/cassandrafeed/internal/models"
	"example.com/cassandrafeed/internal/store"
	"github.com/golang-jwt/jwt/v5"
	"github.com/segmentio/kafka-go"
)

//
// --- Helpers ---
//

// generate JWT token for test user
func makeTestJWT(userID string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		panic(err)
	}
	return tokenStr
}

// create HTTP request with JWT token
func sendJSONRequest(t *testing.T, method, url string, body any, token string, expectedStatus int) *http.Response {
	t.Helper()

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != expectedStatus {
		b, _ := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		t.Fatalf("expected %d, got %d: %s", expectedStatus, resp.StatusCode, string(b))
	}
	return resp
}

//
// --- Setup test server ---
//

func setupTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	mockStore := store.NewMock()
	s := &Server{
		store:       mockStore,
		kafkaWriter: &appkafka.MockKafka{Store: mockStore},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/users", s.createUserHandler)
	mux.HandleFunc("/follow", func(w http.ResponseWriter, r *http.Request) {
		middleware.JWTAuth(http.HandlerFunc(s.followHandler)).ServeHTTP(w, r)
	})
	mux.HandleFunc("/posts", func(w http.ResponseWriter, r *http.Request) {
		middleware.JWTAuth(http.HandlerFunc(s.createPostHandler)).ServeHTTP(w, r)
	})
	mux.HandleFunc("/feed", func(w http.ResponseWriter, r *http.Request) {
		middleware.JWTAuth(http.HandlerFunc(s.getFeedHandler)).ServeHTTP(w, r)
	})

	return s, httptest.NewServer(mux)
}

//
// --- Tests ---
//

// create a new user
func TestCreateUser(t *testing.T) {
	_, ts := setupTestServer(t)
	defer ts.Close()

	id := createUserHelper(ts, "almaz", t)
	if id == "" {
		t.Fatalf("expected non-zero user ID")
	}
}

// full flow: follow -> post -> feed
func TestFollowAndFeedFlow(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret")

	s, ts := setupTestServer(t)
	defer ts.Close()

	almazID, _ := s.store.CreateUser("almaz")
	nurID, _ := s.store.CreateUser("nur")

	almazToken := makeTestJWT(almazID)
	nurToken := makeTestJWT(nurID)

	// Almaz -> follow Nur
	followReq := map[string]any{"followee_id": nurID}
	sendJSONRequest(t, http.MethodPost, ts.URL+"/follow", followReq, almazToken, http.StatusOK)

	// Nur -> create post
	postBody := "Hello from Nur!"
	postReq := map[string]any{"body": postBody}
	sendJSONRequest(t, http.MethodPost, ts.URL+"/posts", postReq, nurToken, http.StatusOK)

	// Almaz -> check feed (polling)
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		feed := getFeedHelper(t, ts, almazToken)
		for _, p := range feed {
			if p.Body == postBody {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("expected post in feed")
}

// invalid JSON for creating user
func TestCreateUser_InvalidJSON(t *testing.T) {
	_, ts := setupTestServer(t)
	defer ts.Close()

	body := []byte(`{"username":123}`)
	resp, err := http.Post(ts.URL+"/users", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("http.Post failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// invalid JSON for follow
func TestFollow_InvalidJSON(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret")
	_, ts := setupTestServer(t)
	defer ts.Close()

	token := makeTestJWT("1")
	body := []byte(`{"followee_id":1}`)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/follow", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// Kafka write error
func TestKafkaWriteError(t *testing.T) {
	s, _ := setupTestServer(t)
	s.kafkaWriter = &appkafka.MockKafkaFail{}

	if err := s.kafkaWriter.WriteMessages(kafka.Message{Key: []byte("k"), Value: []byte("v")}); err == nil {
		t.Fatalf("expected error from MockKafkaFail")
	}
}

// Store create user failure
func TestStoreCreateUserFail(t *testing.T) {
	s, _ := setupTestServer(t)
	s.store = &store.MockStoreFail{}

	if _, err := s.store.CreateUser("almaz"); err == nil {
		t.Fatalf("expected error from MockStoreFail")
	}
}

//
// --- Helpers for test logic ---
//

// helper: create a new user
func createUserHelper(ts *httptest.Server, name string, t *testing.T) string {
	t.Helper()
	body := []byte(`{"username":"` + name + `"}`)
	resp, err := http.Post(ts.URL+"/users", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("createUser failed: %v", err)
	}
	defer resp.Body.Close()

	var res map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	switch v := res["user_id"].(type) {
	case string:
		return v
	default:
		t.Fatalf("unexpected type for user_id: %T", v)
		return ""
	}
}

// helper: get user feed using JWT token
func getFeedHelper(t *testing.T, ts *httptest.Server, token string) []models.Post {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/feed", nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("getFeed failed: %v", err)
	}
	defer resp.Body.Close()

	var posts []models.Post
	_ = json.NewDecoder(resp.Body).Decode(&posts)
	return posts
}
