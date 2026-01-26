package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gndm/ytToDeemix/internal/deemix"
	"github.com/gndm/ytToDeemix/internal/sync"
	"github.com/gndm/ytToDeemix/internal/ytdlp"
)

type mockYT struct {
	entries []ytdlp.PlaylistEntry
}

func (m *mockYT) GetPlaylist(_ context.Context, _ string) ([]ytdlp.PlaylistEntry, error) {
	return m.entries, nil
}

type mockDX struct{}

func (m *mockDX) Login(_ context.Context) error { return nil }
func (m *mockDX) Search(_ context.Context, _ string) ([]deemix.SearchResult, error) {
	return []deemix.SearchResult{{ID: 1, Title: "Track", Artist: "Artist", Link: "https://www.deezer.com/track/1"}}, nil
}
func (m *mockDX) AddToQueue(_ context.Context, _ string, _ int) error { return nil }

func testPipeline() *sync.Pipeline {
	yt := &mockYT{
		entries: []ytdlp.PlaylistEntry{
			{Title: "Artist - Song", VideoID: "abc"},
		},
	}
	return sync.NewPipeline(yt, &mockDX{}, nil)
}

func TestHandleSyncValid(t *testing.T) {
	pipeline := testPipeline()
	handler := handleSync(pipeline)

	body := `{"url":"https://youtube.com/playlist?list=test","bitrate":3}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp syncResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.SessionID == "" {
		t.Error("expected non-empty session_id")
	}
}

func TestHandleSyncMissingURL(t *testing.T) {
	pipeline := testPipeline()
	handler := handleSync(pipeline)

	body := `{"bitrate":3}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleSyncInvalidURL(t *testing.T) {
	pipeline := testPipeline()
	handler := handleSync(pipeline)

	body := `{"url":"https://example.com/not-youtube"}`
	req := httptest.NewRequest(http.MethodPost, "/api/sync", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleSyncInvalidBody(t *testing.T) {
	pipeline := testPipeline()
	handler := handleSync(pipeline)

	req := httptest.NewRequest(http.MethodPost, "/api/sync", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleGetSession(t *testing.T) {
	pipeline := testPipeline()

	// Start a session.
	id := pipeline.Start(context.Background(), "https://youtube.com/playlist?list=test", deemix.Bitrate320, false)

	// Wait for completion.
	time.Sleep(100 * time.Millisecond)

	handler := handleGetSession(pipeline)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/"+id, nil)
	req.SetPathValue("id", id)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var session sync.Session
	if err := json.NewDecoder(w.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.ID != id {
		t.Errorf("session.ID = %q, want %q", session.ID, id)
	}
}

func TestHandleGetSessionNotFound(t *testing.T) {
	pipeline := testPipeline()
	handler := handleGetSession(pipeline)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestHandleStats(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()

	handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var stats statsResponse
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if stats.Goroutines == 0 {
		t.Error("expected non-zero goroutines")
	}
}

func TestHandleNavidromeStatus(t *testing.T) {
	tests := []struct {
		name       string
		configured bool
		want       bool
	}{
		{"configured", true, true},
		{"not configured", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := handleNavidromeStatus(tt.configured)
			req := httptest.NewRequest(http.MethodGet, "/api/navidrome/status", nil)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}

			var resp navidromeStatusResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatal(err)
			}
			if resp.Configured != tt.want {
				t.Errorf("configured = %v, want %v", resp.Configured, tt.want)
			}
		})
	}
}

func TestIsValidPlaylistURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.youtube.com/playlist?list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", true},
		{"https://youtube.com/playlist?list=test", true},
		{"https://youtu.be/abc123", true},
		{"https://example.com/playlist", false},
		{"not a url", false},
	}
	for _, tt := range tests {
		if got := isValidYouTubeURL(tt.url); got != tt.want {
			t.Errorf("isValidYouTubeURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestIsChannelURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.youtube.com/@username", true},
		{"https://youtube.com/@username/videos", true},
		{"https://www.youtube.com/channel/UCxxxx", true},
		{"https://www.youtube.com/c/ChannelName", true},
		{"https://www.youtube.com/user/username", true},
		{"https://www.youtube.com/playlist?list=PLxxxx", false},
		{"https://youtu.be/abc123", false},
		{"https://example.com/@user", false},
	}
	for _, tt := range tests {
		if got := isChannelURL(tt.url); got != tt.want {
			t.Errorf("isChannelURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}
