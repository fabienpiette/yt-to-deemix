package navidrome

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearch_MatchFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"subsonic-response": {
				"status": "ok",
				"searchResult2": {
					"song": [
						{"id": "42", "title": "Do I Wanna Know?", "artist": "Arctic Monkeys", "album": "AM", "duration": 272},
						{"id": "99", "title": "Other Song", "artist": "Other Artist", "album": "X", "duration": 180}
					]
				}
			}
		}`))
	}))
	defer srv.Close()

	client := &HTTPClient{
		BaseURL:  srv.URL,
		User:     "testuser",
		Password: "testpass",
		Client:   srv.Client(),
	}

	results, err := client.Search(context.Background(), "Arctic Monkeys", "Do I Wanna Know?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "42" {
		t.Errorf("ID = %q, want %q", results[0].ID, "42")
	}
	if results[0].Title != "Do I Wanna Know?" {
		t.Errorf("Title = %q, want %q", results[0].Title, "Do I Wanna Know?")
	}
	if results[0].Artist != "Arctic Monkeys" {
		t.Errorf("Artist = %q, want %q", results[0].Artist, "Arctic Monkeys")
	}
}

func TestSearch_NoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"subsonic-response": {
				"status": "ok",
				"searchResult2": {
					"song": [
						{"id": "99", "title": "Completely Different", "artist": "Other Band", "album": "X", "duration": 200}
					]
				}
			}
		}`))
	}))
	defer srv.Close()

	client := &HTTPClient{
		BaseURL:  srv.URL,
		User:     "testuser",
		Password: "testpass",
		Client:   srv.Client(),
	}

	results, err := client.Search(context.Background(), "Arctic Monkeys", "Do I Wanna Know?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"subsonic-response": {
				"status": "failed",
				"error": {"message": "Wrong username or password"}
			}
		}`))
	}))
	defer srv.Close()

	client := &HTTPClient{
		BaseURL:  srv.URL,
		User:     "baduser",
		Password: "badpass",
		Client:   srv.Client(),
	}

	_, err := client.Search(context.Background(), "Artist", "Title")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "navidrome: API error: Wrong username or password" {
		t.Errorf("error = %q, want API error message", got)
	}
}

func TestSearch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &HTTPClient{
		BaseURL:  srv.URL,
		User:     "user",
		Password: "pass",
		Client:   srv.Client(),
	}

	_, err := client.Search(context.Background(), "Artist", "Title")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSearch_EmptySongList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"subsonic-response": {
				"status": "ok",
				"searchResult2": {}
			}
		}`))
	}))
	defer srv.Close()

	client := &HTTPClient{
		BaseURL:  srv.URL,
		User:     "user",
		Password: "pass",
		Client:   srv.Client(),
	}

	results, err := client.Search(context.Background(), "Artist", "Title")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_CaseInsensitiveMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"subsonic-response": {
				"status": "ok",
				"searchResult2": {
					"song": [
						{"id": "1", "title": "CREEP", "artist": "RADIOHEAD", "album": "Pablo Honey", "duration": 236}
					]
				}
			}
		}`))
	}))
	defer srv.Close()

	client := &HTTPClient{
		BaseURL:  srv.URL,
		User:     "user",
		Password: "pass",
		Client:   srv.Client(),
	}

	results, err := client.Search(context.Background(), "radiohead", "creep")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestSearch_ExactMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"subsonic-response": {
				"status": "ok",
				"searchResult2": {
					"song": [
						{"id": "1", "title": "Creep (Acoustic)", "artist": "Radiohead", "album": "B-Sides", "duration": 240},
						{"id": "2", "title": "Creep", "artist": "Radiohead", "album": "Pablo Honey", "duration": 236}
					]
				}
			}
		}`))
	}))
	defer srv.Close()

	client := &HTTPClient{
		BaseURL:   srv.URL,
		User:      "user",
		Password:  "pass",
		MatchMode: MatchExact,
		Client:    srv.Client(),
	}

	results, err := client.Search(context.Background(), "Radiohead", "Creep")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only exact match, not "Creep (Acoustic)"
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "2" {
		t.Errorf("ID = %q, want %q", results[0].ID, "2")
	}
}

func TestSearch_FuzzyMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"subsonic-response": {
				"status": "ok",
				"searchResult2": {
					"song": [
						{"id": "1", "title": "Do I Wanna Know", "artist": "Arctic Monkeys", "album": "AM", "duration": 272},
						{"id": "2", "title": "Totally Different Song", "artist": "Other Band", "album": "X", "duration": 180}
					]
				}
			}
		}`))
	}))
	defer srv.Close()

	client := &HTTPClient{
		BaseURL:   srv.URL,
		User:      "user",
		Password:  "pass",
		MatchMode: MatchFuzzy,
		Client:    srv.Client(),
	}

	// Query with trailing "?" — fuzzy should still match "Do I Wanna Know" (no "?")
	results, err := client.Search(context.Background(), "Arctic Monkeys", "Do I Wanna Know?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("ID = %q, want %q", results[0].ID, "1")
	}
}
