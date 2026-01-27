package sync

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gndm/ytToDeemix/internal/deemix"
	"github.com/gndm/ytToDeemix/internal/navidrome"
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

func TestPipelineAnalyzeAndDownload(t *testing.T) {
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

	pipeline := NewPipeline(yt, dx, nil)
	pipeline.searchDelay = 0
	pipeline.queueDelay = 0

	id := pipeline.Analyze(context.Background(), "https://youtube.com/playlist?list=test", deemix.Bitrate320, false)

	// Wait for ready status.
	var session *Session
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		s, ok := pipeline.GetSession(id)
		if ok && (s.Status == StatusReady || s.Status == StatusError) {
			session = s
			break
		}
	}

	if session == nil {
		t.Fatal("session never reached ready")
	}
	if session.Status != StatusReady {
		t.Fatalf("expected status 'ready', got %q (error: %s)", session.Status, session.Error)
	}

	// Check progress after analysis.
	if session.Progress.Total != 3 {
		t.Errorf("total = %d, want 3", session.Progress.Total)
	}
	if session.Progress.Searched != 3 {
		t.Errorf("searched = %d, want 3", session.Progress.Searched)
	}
	if session.Progress.Selected != 2 {
		t.Errorf("selected = %d, want 2", session.Progress.Selected)
	}
	if session.Progress.NotFound != 1 {
		t.Errorf("not_found = %d, want 1", session.Progress.NotFound)
	}
	if session.Progress.Queued != 0 {
		t.Errorf("queued = %d, want 0 (before download)", session.Progress.Queued)
	}

	// Check track states after analysis.
	if session.Tracks[0].Status != TrackFound {
		t.Errorf("track[0] status = %q, want 'found'", session.Tracks[0].Status)
	}
	if !session.Tracks[0].Selected {
		t.Error("track[0] should be selected")
	}
	if session.Tracks[1].Status != TrackNotFound {
		t.Errorf("track[1] status = %q, want 'not_found'", session.Tracks[1].Status)
	}
	if session.Tracks[2].Status != TrackFound {
		t.Errorf("track[2] status = %q, want 'found'", session.Tracks[2].Status)
	}

	// No tracks queued yet.
	if len(dx.queuedURLs) != 0 {
		t.Fatalf("expected 0 queued URLs before download, got %d", len(dx.queuedURLs))
	}

	// Now trigger download.
	err := pipeline.Download(context.Background(), id)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	// Get final session state.
	session, _ = pipeline.GetSession(id)
	if session.Status != StatusDone {
		t.Fatalf("expected status 'done' after download, got %q", session.Status)
	}
	if session.Progress.Queued != 2 {
		t.Errorf("queued = %d, want 2", session.Progress.Queued)
	}

	// Check track states after download.
	if session.Tracks[0].Status != TrackQueued {
		t.Errorf("track[0] status = %q, want 'queued'", session.Tracks[0].Status)
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

	pipeline := NewPipeline(yt, dx, nil)
	id := pipeline.Analyze(context.Background(), "bad-url", deemix.Bitrate320, false)

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

	pipeline := NewPipeline(yt, dx, nil)
	pipeline.searchDelay = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	pipeline.Analyze(ctx, "url", deemix.Bitrate320, false)

	// Cancel partway through.
	time.Sleep(30 * time.Millisecond)
	cancel()

	// Give time for the cancellation to propagate.
	time.Sleep(100 * time.Millisecond)
}

func TestSetTrackSelected(t *testing.T) {
	yt := &mockYTClient{
		entries: []ytdlp.PlaylistEntry{
			{Title: "Artist - Song", VideoID: "abc"},
		},
	}
	dx := &mockDeemixClient{
		searchResults: map[string][]deemix.SearchResult{
			"Artist Song": {{ID: 1, Title: "Song", Artist: "Artist", Link: "https://www.deezer.com/track/1"}},
		},
	}

	pipeline := NewPipeline(yt, dx, nil)
	pipeline.searchDelay = 0
	id := pipeline.Analyze(context.Background(), "url", deemix.Bitrate320, false)

	// Wait for ready.
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		s, _ := pipeline.GetSession(id)
		if s.Status == StatusReady {
			break
		}
	}

	session, _ := pipeline.GetSession(id)
	if !session.Tracks[0].Selected {
		t.Error("track should be selected initially")
	}
	if session.Progress.Selected != 1 {
		t.Errorf("selected = %d, want 1", session.Progress.Selected)
	}

	// Deselect the track.
	err := pipeline.SetTrackSelected(id, 0, false)
	if err != nil {
		t.Fatalf("SetTrackSelected failed: %v", err)
	}

	session, _ = pipeline.GetSession(id)
	if session.Tracks[0].Selected {
		t.Error("track should be deselected")
	}
	if session.Progress.Selected != 0 {
		t.Errorf("selected = %d, want 0", session.Progress.Selected)
	}

	// Select it again.
	err = pipeline.SetTrackSelected(id, 0, true)
	if err != nil {
		t.Fatalf("SetTrackSelected failed: %v", err)
	}

	session, _ = pipeline.GetSession(id)
	if !session.Tracks[0].Selected {
		t.Error("track should be selected again")
	}
}

func TestDownloadOnlySelected(t *testing.T) {
	yt := &mockYTClient{
		entries: []ytdlp.PlaylistEntry{
			{Title: "Artist - Song 1", VideoID: "abc"},
			{Title: "Artist - Song 2", VideoID: "def"},
		},
	}
	dx := &mockDeemixClient{
		searchResults: map[string][]deemix.SearchResult{
			"Artist Song 1": {{ID: 1, Title: "Song 1", Artist: "Artist", Link: "https://www.deezer.com/track/1"}},
			"Artist Song 2": {{ID: 2, Title: "Song 2", Artist: "Artist", Link: "https://www.deezer.com/track/2"}},
		},
	}

	pipeline := NewPipeline(yt, dx, nil)
	pipeline.searchDelay = 0
	pipeline.queueDelay = 0
	id := pipeline.Analyze(context.Background(), "url", deemix.Bitrate320, false)

	// Wait for ready.
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		s, _ := pipeline.GetSession(id)
		if s.Status == StatusReady {
			break
		}
	}

	// Deselect the first track.
	pipeline.SetTrackSelected(id, 0, false)

	// Download - should only queue track 2.
	err := pipeline.Download(context.Background(), id)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	session, _ := pipeline.GetSession(id)
	if session.Progress.Queued != 1 {
		t.Errorf("queued = %d, want 1", session.Progress.Queued)
	}
	if len(dx.queuedURLs) != 1 {
		t.Errorf("expected 1 queued URL, got %d", len(dx.queuedURLs))
	}
	if dx.queuedURLs[0] != "https://www.deezer.com/track/2" {
		t.Errorf("wrong track queued: %s", dx.queuedURLs[0])
	}
}

// mockNavidromeClient implements navidrome.Client for testing.
type mockNavidromeClient struct {
	existing map[string][]navidrome.SearchResult
}

func (m *mockNavidromeClient) Search(_ context.Context, artist, title string) ([]navidrome.SearchResult, error) {
	key := artist + "|" + title
	if results, ok := m.existing[key]; ok {
		return results, nil
	}
	return nil, nil
}

func TestPipelineNavidromeSkip(t *testing.T) {
	yt := &mockYTClient{
		entries: []ytdlp.PlaylistEntry{
			{Title: "Arctic Monkeys - Do I Wanna Know?", VideoID: "abc"},
			{Title: "Radiohead - Creep", VideoID: "def"},
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

	nav := &mockNavidromeClient{
		existing: map[string][]navidrome.SearchResult{
			"Arctic Monkeys|Do I Wanna Know?": {
				{ID: "42", Title: "Do I Wanna Know?", Artist: "Arctic Monkeys"},
			},
		},
	}

	pipeline := NewPipeline(yt, dx, nav)
	pipeline.searchDelay = 0
	pipeline.queueDelay = 0
	pipeline.checkDelay = 0

	id := pipeline.Analyze(context.Background(), "https://youtube.com/playlist?list=test", deemix.Bitrate320, true)

	// Wait for ready.
	var session *Session
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		s, ok := pipeline.GetSession(id)
		if ok && (s.Status == StatusReady || s.Status == StatusError) {
			session = s
			break
		}
	}

	if session == nil {
		t.Fatal("session never reached ready")
	}
	if session.Status != StatusReady {
		t.Fatalf("expected status 'ready', got %q (error: %s)", session.Status, session.Error)
	}

	// First track should be skipped (exists in Navidrome) and deselected.
	if session.Tracks[0].Status != TrackSkipped {
		t.Errorf("track[0] status = %q, want 'skipped'", session.Tracks[0].Status)
	}
	if session.Tracks[0].Selected {
		t.Error("track[0] should be deselected (skipped)")
	}
	// Second track should be found and selected.
	if session.Tracks[1].Status != TrackFound {
		t.Errorf("track[1] status = %q, want 'found'", session.Tracks[1].Status)
	}
	if !session.Tracks[1].Selected {
		t.Error("track[1] should be selected")
	}

	if session.Progress.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", session.Progress.Skipped)
	}
	if session.Progress.Selected != 1 {
		t.Errorf("selected = %d, want 1", session.Progress.Selected)
	}

	// Now download.
	err := pipeline.Download(context.Background(), id)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	session, _ = pipeline.GetSession(id)
	if session.Progress.Queued != 1 {
		t.Errorf("queued = %d, want 1", session.Progress.Queued)
	}

	// Only one track should have been queued in Deemix.
	if len(dx.queuedURLs) != 1 {
		t.Fatalf("expected 1 queued URL, got %d", len(dx.queuedURLs))
	}
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
