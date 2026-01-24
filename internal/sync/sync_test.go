package sync

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gndm/ytToDeemix/internal/deemix"
	"github.com/gndm/ytToDeemix/internal/ytdlp"
)

// mockYTClient implements ytdlp.Client for testing.
type mockYTClient struct {
	entries []ytdlp.PlaylistEntry
	err     error
}

func (m *mockYTClient) GetPlaylist(_ context.Context, _ string) ([]ytdlp.PlaylistEntry, error) {
	return m.entries, m.err
}

// mockDeemixClient implements deemix.Client for testing.
type mockDeemixClient struct {
	searchResults map[string][]deemix.SearchResult
	queuedURLs    []string
	queueErr      error
}

func (m *mockDeemixClient) Login(_ context.Context) error { return nil }

func (m *mockDeemixClient) Search(_ context.Context, query string) ([]deemix.SearchResult, error) {
	if results, ok := m.searchResults[query]; ok {
		return results, nil
	}
	return nil, nil
}

func (m *mockDeemixClient) AddToQueue(_ context.Context, url string, _ int) error {
	if m.queueErr != nil {
		return m.queueErr
	}
	m.queuedURLs = append(m.queuedURLs, url)
	return nil
}

func TestPipelineFullRun(t *testing.T) {
	yt := &mockYTClient{
		entries: []ytdlp.PlaylistEntry{
			{Title: "Arctic Monkeys - Do I Wanna Know?", VideoID: "abc"},
			{Title: "Unknown Song Title", VideoID: "def"},
			{Title: "Radiohead - Creep", VideoID: "ghi"},
		},
	}

	dx := &mockDeemixClient{
		searchResults: map[string][]deemix.SearchResult{
			"Arctic Monkeys Do I Wanna Know?": {
				{ID: 1, Title: "Do I Wanna Know?", Artist: "Arctic Monkeys", Link: "https://www.deezer.com/track/1"},
			},
			"Radiohead Creep": {
				{ID: 2, Title: "Creep", Artist: "Radiohead", Link: "https://www.deezer.com/track/2"},
			},
		},
	}

	pipeline := NewPipeline(yt, dx)
	pipeline.searchDelay = 0
	pipeline.queueDelay = 0

	id := pipeline.Start(context.Background(), "https://youtube.com/playlist?list=test", deemix.Bitrate320)

	// Wait for completion.
	var session *Session
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		s, ok := pipeline.GetSession(id)
		if ok && (s.Status == StatusDone || s.Status == StatusError) {
			session = s
			break
		}
	}

	if session == nil {
		t.Fatal("session never completed")
	}
	if session.Status != StatusDone {
		t.Fatalf("expected status 'done', got %q (error: %s)", session.Status, session.Error)
	}

	// Check progress.
	if session.Progress.Total != 3 {
		t.Errorf("total = %d, want 3", session.Progress.Total)
	}
	if session.Progress.Searched != 3 {
		t.Errorf("searched = %d, want 3", session.Progress.Searched)
	}
	if session.Progress.Queued != 2 {
		t.Errorf("queued = %d, want 2", session.Progress.Queued)
	}
	if session.Progress.NotFound != 1 {
		t.Errorf("not_found = %d, want 1", session.Progress.NotFound)
	}

	// Check track states.
	if session.Tracks[0].Status != TrackQueued {
		t.Errorf("track[0] status = %q, want 'queued'", session.Tracks[0].Status)
	}
	if session.Tracks[1].Status != TrackNotFound {
		t.Errorf("track[1] status = %q, want 'not_found'", session.Tracks[1].Status)
	}
	if session.Tracks[2].Status != TrackQueued {
		t.Errorf("track[2] status = %q, want 'queued'", session.Tracks[2].Status)
	}

	// Check queued URLs.
	if len(dx.queuedURLs) != 2 {
		t.Fatalf("expected 2 queued URLs, got %d", len(dx.queuedURLs))
	}
}

func TestPipelineYTError(t *testing.T) {
	yt := &mockYTClient{err: fmt.Errorf("network error")}
	dx := &mockDeemixClient{}

	pipeline := NewPipeline(yt, dx)
	id := pipeline.Start(context.Background(), "bad-url", deemix.Bitrate320)

	var session *Session
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		s, _ := pipeline.GetSession(id)
		if s.Status == StatusError {
			session = s
			break
		}
	}

	if session == nil {
		t.Fatal("session never errored")
	}
	if session.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestPipelineContextCanceled(t *testing.T) {
	yt := &mockYTClient{
		entries: []ytdlp.PlaylistEntry{
			{Title: "Artist - Song 1"},
			{Title: "Artist - Song 2"},
			{Title: "Artist - Song 3"},
		},
	}
	dx := &mockDeemixClient{
		searchResults: map[string][]deemix.SearchResult{
			"Artist Song 1": {{ID: 1, Link: "https://www.deezer.com/track/1"}},
			"Artist Song 2": {{ID: 2, Link: "https://www.deezer.com/track/2"}},
			"Artist Song 3": {{ID: 3, Link: "https://www.deezer.com/track/3"}},
		},
	}

	pipeline := NewPipeline(yt, dx)
	pipeline.searchDelay = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	pipeline.Start(ctx, "url", deemix.Bitrate320)

	// Cancel partway through.
	time.Sleep(30 * time.Millisecond)
	cancel()

	// Give time for the cancellation to propagate.
	time.Sleep(100 * time.Millisecond)
}

func TestBuildQuery(t *testing.T) {
	tests := []struct {
		artist, song, want string
	}{
		{"Arctic Monkeys", "Do I Wanna Know?", "Arctic Monkeys Do I Wanna Know?"},
		{"", "Wonderwall", "Wonderwall"},
	}
	for _, tt := range tests {
		got := buildQuery(tt.artist, tt.song)
		if got != tt.want {
			t.Errorf("buildQuery(%q, %q) = %q, want %q", tt.artist, tt.song, got, tt.want)
		}
	}
}
