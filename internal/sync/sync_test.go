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

// slowDeemixClient adds artificial delay to simulate slow API calls for pause/resume testing.
type slowDeemixClient struct {
	searchResults map[string][]deemix.SearchResult
	queuedURLs    []string
	delay         time.Duration
}

func (m *slowDeemixClient) Login(_ context.Context) error { return nil }

func (m *slowDeemixClient) Search(ctx context.Context, query string) ([]deemix.SearchResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(m.delay):
	}
	if results, ok := m.searchResults[query]; ok {
		return results, nil
	}
	return nil, nil
}

func (m *slowDeemixClient) AddToQueue(ctx context.Context, url string, _ int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(m.delay):
	}
	m.queuedURLs = append(m.queuedURLs, url)
	return nil
}

func TestPauseResumeAnalyze(t *testing.T) {
	yt := &mockYTClient{
		entries: []ytdlp.PlaylistEntry{
			{Title: "Artist - Song 1"},
			{Title: "Artist - Song 2"},
			{Title: "Artist - Song 3"},
		},
	}
	dx := &slowDeemixClient{
		searchResults: map[string][]deemix.SearchResult{
			"Artist Song 1": {{ID: 1, Title: "Song 1", Artist: "Artist", Link: "https://www.deezer.com/track/1"}},
			"Artist Song 2": {{ID: 2, Title: "Song 2", Artist: "Artist", Link: "https://www.deezer.com/track/2"}},
			"Artist Song 3": {{ID: 3, Title: "Song 3", Artist: "Artist", Link: "https://www.deezer.com/track/3"}},
		},
		delay: 50 * time.Millisecond,
	}

	pipeline := NewPipeline(yt, dx, nil)
	pipeline.searchDelay = 10 * time.Millisecond

	id := pipeline.Analyze(context.Background(), "url", deemix.Bitrate320, false)

	// Wait for searching status.
	time.Sleep(30 * time.Millisecond)

	// Pause the session.
	err := pipeline.PauseSession(id)
	if err != nil {
		t.Fatalf("PauseSession failed: %v", err)
	}

	// Wait for pause to take effect.
	time.Sleep(100 * time.Millisecond)

	session, _ := pipeline.GetSession(id)
	if session.Status != StatusPaused {
		t.Errorf("expected status 'paused', got %q", session.Status)
	}

	// Resume the session.
	err = pipeline.ResumeSession(id)
	if err != nil {
		t.Fatalf("ResumeSession failed: %v", err)
	}

	// Wait for completion.
	for i := 0; i < 100; i++ {
		time.Sleep(20 * time.Millisecond)
		session, _ = pipeline.GetSession(id)
		if session.Status == StatusReady || session.Status == StatusError {
			break
		}
	}

	if session.Status != StatusReady {
		t.Fatalf("expected status 'ready' after resume, got %q (error: %s)", session.Status, session.Error)
	}
	if session.Progress.Searched != 3 {
		t.Errorf("expected 3 tracks searched, got %d", session.Progress.Searched)
	}
}

func TestCancelAnalyze(t *testing.T) {
	yt := &mockYTClient{
		entries: []ytdlp.PlaylistEntry{
			{Title: "Artist - Song 1"},
			{Title: "Artist - Song 2"},
			{Title: "Artist - Song 3"},
		},
	}
	dx := &slowDeemixClient{
		searchResults: map[string][]deemix.SearchResult{
			"Artist Song 1": {{ID: 1, Title: "Song 1", Artist: "Artist", Link: "https://www.deezer.com/track/1"}},
			"Artist Song 2": {{ID: 2, Title: "Song 2", Artist: "Artist", Link: "https://www.deezer.com/track/2"}},
			"Artist Song 3": {{ID: 3, Title: "Song 3", Artist: "Artist", Link: "https://www.deezer.com/track/3"}},
		},
		delay: 50 * time.Millisecond,
	}

	pipeline := NewPipeline(yt, dx, nil)
	pipeline.searchDelay = 10 * time.Millisecond

	id := pipeline.Analyze(context.Background(), "url", deemix.Bitrate320, false)

	// Wait for searching to start.
	time.Sleep(30 * time.Millisecond)

	// Cancel the session.
	err := pipeline.CancelSession(id)
	if err != nil {
		t.Fatalf("CancelSession failed: %v", err)
	}

	// Wait for cancellation to complete.
	time.Sleep(100 * time.Millisecond)

	session, _ := pipeline.GetSession(id)
	if session.Status != StatusCanceled {
		t.Errorf("expected status 'canceled', got %q", session.Status)
	}
}

func TestPauseResumeDownload(t *testing.T) {
	yt := &mockYTClient{
		entries: []ytdlp.PlaylistEntry{
			{Title: "Artist - Song 1"},
			{Title: "Artist - Song 2"},
			{Title: "Artist - Song 3"},
		},
	}
	dx := &slowDeemixClient{
		searchResults: map[string][]deemix.SearchResult{
			"Artist Song 1": {{ID: 1, Title: "Song 1", Artist: "Artist", Link: "https://www.deezer.com/track/1"}},
			"Artist Song 2": {{ID: 2, Title: "Song 2", Artist: "Artist", Link: "https://www.deezer.com/track/2"}},
			"Artist Song 3": {{ID: 3, Title: "Song 3", Artist: "Artist", Link: "https://www.deezer.com/track/3"}},
		},
		delay: 5 * time.Millisecond,
	}

	pipeline := NewPipeline(yt, dx, nil)
	pipeline.searchDelay = 0
	pipeline.queueDelay = 0

	id := pipeline.Analyze(context.Background(), "url", deemix.Bitrate320, false)

	// Wait for ready.
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		s, _ := pipeline.GetSession(id)
		if s.Status == StatusReady {
			break
		}
	}

	// Now slow down the queue for download test.
	dx.delay = 50 * time.Millisecond

	// Start download in goroutine.
	go pipeline.Download(context.Background(), id)

	// Wait for downloading to start.
	time.Sleep(30 * time.Millisecond)

	// Pause the download.
	err := pipeline.PauseSession(id)
	if err != nil {
		t.Fatalf("PauseSession during download failed: %v", err)
	}

	// Wait for pause to take effect.
	time.Sleep(100 * time.Millisecond)

	session, _ := pipeline.GetSession(id)
	if session.Status != StatusPaused {
		t.Errorf("expected status 'paused' during download, got %q", session.Status)
	}

	// Resume the download.
	err = pipeline.ResumeSession(id)
	if err != nil {
		t.Fatalf("ResumeSession during download failed: %v", err)
	}

	// Wait for completion.
	for i := 0; i < 100; i++ {
		time.Sleep(20 * time.Millisecond)
		session, _ = pipeline.GetSession(id)
		if session.Status == StatusDone || session.Status == StatusError {
			break
		}
	}

	if session.Status != StatusDone {
		t.Fatalf("expected status 'done' after download resume, got %q", session.Status)
	}
	if session.Progress.Queued != 3 {
		t.Errorf("expected 3 tracks queued, got %d", session.Progress.Queued)
	}
}

func TestPauseErrors(t *testing.T) {
	yt := &mockYTClient{entries: []ytdlp.PlaylistEntry{{Title: "Artist - Song"}}}
	dx := &mockDeemixClient{
		searchResults: map[string][]deemix.SearchResult{
			"Artist Song": {{ID: 1, Title: "Song", Artist: "Artist", Link: "https://www.deezer.com/track/1"}},
		},
	}

	pipeline := NewPipeline(yt, dx, nil)
	pipeline.searchDelay = 0

	// Test pausing non-existent session.
	err := pipeline.PauseSession("nonexistent")
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}

	// Test resuming non-existent session.
	err = pipeline.ResumeSession("nonexistent")
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}

	// Start a session and wait for ready.
	id := pipeline.Analyze(context.Background(), "url", deemix.Bitrate320, false)
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		s, _ := pipeline.GetSession(id)
		if s.Status == StatusReady {
			break
		}
	}

	// Test pausing a ready (non-active) session.
	err = pipeline.PauseSession(id)
	if err != ErrSessionNotReady {
		t.Errorf("expected ErrSessionNotReady for ready session, got %v", err)
	}

	// Test resuming a non-paused session.
	err = pipeline.ResumeSession(id)
	if err != ErrSessionNotPaused {
		t.Errorf("expected ErrSessionNotPaused for ready session, got %v", err)
	}
}
