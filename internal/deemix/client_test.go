package deemix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/loginArl" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["arl"] != "valid-token" {
			json.NewEncoder(w).Encode(map[string]int{"status": 0})
			return
		}
		json.NewEncoder(w).Encode(map[string]int{"status": 1})
	}))
	defer server.Close()

	t.Run("valid token", func(t *testing.T) {
		client := NewClient(server.URL, "valid-token")
		if err := client.Login(context.Background()); err != nil {
			t.Fatalf("Login() error = %v", err)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		client := NewClient(server.URL, "bad-token")
		err := client.Login(context.Background())
		if err == nil {
			t.Fatal("expected error for invalid token")
		}
	})
}

func TestSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		term := r.URL.Query().Get("term")
		if term == "" {
			t.Error("missing term parameter")
		}

		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id":       1234,
					"title":    "Do I Wanna Know?",
					"artist":   map[string]string{"name": "Arctic Monkeys"},
					"album":    map[string]string{"title": "AM"},
					"duration": 272,
					"link":     "https://www.deezer.com/track/1234",
				},
				{
					"id":       5678,
					"title":    "R U Mine?",
					"artist":   map[string]string{"name": "Arctic Monkeys"},
					"album":    map[string]string{"title": "AM"},
					"duration": 202,
					"link":     "",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	results, err := client.Search(context.Background(), "Arctic Monkeys Do I Wanna Know")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Title != "Do I Wanna Know?" {
		t.Errorf("results[0].Title = %q, want %q", results[0].Title, "Do I Wanna Know?")
	}
	if results[0].Artist != "Arctic Monkeys" {
		t.Errorf("results[0].Artist = %q, want %q", results[0].Artist, "Arctic Monkeys")
	}
	if results[0].Link != "https://www.deezer.com/track/1234" {
		t.Errorf("results[0].Link = %q", results[0].Link)
	}
	// Test fallback link generation when API returns empty link.
	if results[1].Link != "https://www.deezer.com/track/5678" {
		t.Errorf("results[1].Link = %q, want generated link", results[1].Link)
	}
}

func TestSearchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	_, err := client.Search(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestAddToQueue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/addToQueue" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["url"] != "https://www.deezer.com/track/1234" {
			t.Errorf("unexpected url: %v", body["url"])
		}
		if int(body["bitrate"].(float64)) != Bitrate320 {
			t.Errorf("unexpected bitrate: %v", body["bitrate"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	err := client.AddToQueue(context.Background(), "https://www.deezer.com/track/1234", Bitrate320)
	if err != nil {
		t.Fatalf("AddToQueue() error = %v", err)
	}
}

func TestAddToQueueError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid request"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	err := client.AddToQueue(context.Background(), "bad-url", Bitrate320)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}
