package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"sync"
	"time"

	"github.com/gndm/ytToDeemix/internal/deemix"
	"github.com/gndm/ytToDeemix/internal/navidrome"
	"github.com/gndm/ytToDeemix/internal/parser"
	"github.com/gndm/ytToDeemix/internal/ytdlp"
)

// Default confidence threshold (0-100).
const DefaultConfidenceThreshold = 70

// sessionControl holds cancellation and pause/resume channels for a session.
type sessionControl struct {
	cancel   context.CancelFunc
	pauseCh  chan struct{}
	resumeCh chan struct{}
}

// Pipeline manages sync sessions.
type Pipeline struct {
	ytClient            ytdlp.Client
	deemixClient        deemix.Client
	navidromeClient     navidrome.Client
	sessions            map[string]*Session
	controls            map[string]*sessionControl
	mu                  sync.RWMutex
	searchDelay         time.Duration
	queueDelay          time.Duration
	checkDelay          time.Duration
	confidenceThreshold int
}

// NewPipeline creates a new sync pipeline with the given clients.
// nav can be nil to disable Navidrome checking.
func NewPipeline(yt ytdlp.Client, dx deemix.Client, nav navidrome.Client) *Pipeline {
	return &Pipeline{
		ytClient:            yt,
		deemixClient:        dx,
		navidromeClient:     nav,
		sessions:            make(map[string]*Session),
		controls:            make(map[string]*sessionControl),
		searchDelay:         200 * time.Millisecond,
		queueDelay:          100 * time.Millisecond,
		checkDelay:          100 * time.Millisecond,
		confidenceThreshold: DefaultConfidenceThreshold,
	}
}

// SetConfidenceThreshold sets the minimum confidence score (0-100) for auto-queuing.
// Tracks below this threshold will be marked as needs_review.
func (p *Pipeline) SetConfidenceThreshold(threshold int) {
	if threshold < 0 {
		threshold = 0
	}
	if threshold > 100 {
		threshold = 100
	}
	p.confidenceThreshold = threshold
}

// Analyze begins a new analysis session for the given playlist URL and bitrate.
// Returns the session ID immediately; processing runs in a goroutine.
// Analysis fetches, parses, searches Deezer, and checks Navidrome, then stops at StatusReady.
func (p *Pipeline) Analyze(ctx context.Context, playlistURL string, bitrate int, checkNavidrome bool) string {
	id := generateID()
	session := &Session{
		ID:             id,
		URL:            playlistURL,
		Status:         StatusFetching,
		Bitrate:        bitrate,
		CheckNavidrome: checkNavidrome,
	}

	// Create cancellable context and control channels.
	ctx, cancel := context.WithCancel(ctx)
	ctrl := &sessionControl{
		cancel:   cancel,
		pauseCh:  make(chan struct{}, 1),
		resumeCh: make(chan struct{}, 1),
	}

	p.mu.Lock()
	p.sessions[id] = session
	p.controls[id] = ctrl
	p.mu.Unlock()

	log.Printf("[sync] session %s analyzing: %s", id, playlistURL)
	go p.run(ctx, session)
	return id
}

// GetSession returns a copy of the session state.
func (p *Pipeline) GetSession(id string) (*Session, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	s, ok := p.sessions[id]
	if !ok {
		return nil, false
	}
	// Return a copy to avoid races.
	cp := *s
	cp.Tracks = make([]Track, len(s.Tracks))
	copy(cp.Tracks, s.Tracks)
	return &cp, true
}

func (p *Pipeline) run(ctx context.Context, session *Session) {
	// Phase 1: Fetch playlist.
	entries, err := p.ytClient.GetPlaylist(ctx, session.URL)
	if err != nil {
		p.setError(session, "failed to fetch playlist: "+err.Error())
		return
	}

	// Phase 2: Parse titles.
	p.mu.Lock()
	session.Status = StatusParsing
	session.Tracks = make([]Track, len(entries))
	session.Progress.Total = len(entries)
	for i, entry := range entries {
		var artist, song string

		// Priority 1: yt-dlp artist/track fields (YouTube Music metadata).
		if entry.Artist != "" {
			artist = entry.Artist
			if entry.Track != "" {
				song = entry.Track
			} else {
				song = entry.Title
			}
		} else {
			// Priority 2: Parse from title (handles "Artist - Song" format).
			artist, song = parser.Parse(entry.Title)
			// Note: We don't use channel as fallback because it's often
			// unreliable (could be uploader, label, band member, etc.).
			// Better to search with just title than wrong artist.
		}

		session.Tracks[i] = Track{
			YouTubeTitle: entry.Title,
			ParsedArtist: artist,
			ParsedSong:   song,
			Status:       TrackPending,
		}
	}
	session.Status = StatusSearching
	p.mu.Unlock()

	// Phase 3: Search Deemix for each track.
	for i := range session.Tracks {
		if err := p.checkpoint(ctx, session, StatusSearching); err != nil {
			if session.Status != StatusCanceled {
				p.setError(session, "canceled")
			}
			return
		}

		p.mu.Lock()
		session.Tracks[i].Status = TrackSearching
		p.mu.Unlock()

		query := buildQuery(session.Tracks[i].ParsedArtist, session.Tracks[i].ParsedSong)
		results, err := p.deemixClient.Search(ctx, query)

		p.mu.Lock()
		if err != nil || len(results) == 0 {
			session.Tracks[i].Status = TrackNotFound
			session.Progress.NotFound++
		} else {
			match := results[0]
			session.Tracks[i].DeezerMatch = &match

			// Calculate confidence score.
			confidence := calculateConfidence(
				session.Tracks[i].ParsedArtist,
				session.Tracks[i].ParsedSong,
				match.Artist,
				match.Title,
			)
			session.Tracks[i].Confidence = confidence

			if confidence >= p.confidenceThreshold {
				session.Tracks[i].Status = TrackFound
				session.Tracks[i].Selected = true
				session.Progress.Selected++
			} else {
				session.Tracks[i].Status = TrackNeedsReview
				session.Progress.NeedsReview++
			}
		}
		session.Progress.Searched++
		p.mu.Unlock()

		if i < len(session.Tracks)-1 {
			time.Sleep(p.searchDelay)
		}
	}

	// Phase 3.5: Check Navidrome for existing tracks.
	if p.navidromeClient != nil && session.CheckNavidrome {
		p.mu.Lock()
		session.Status = StatusChecking
		p.mu.Unlock()

		for i := range session.Tracks {
			if err := p.checkpoint(ctx, session, StatusChecking); err != nil {
				if session.Status != StatusCanceled {
					p.setError(session, "canceled")
				}
				return
			}

			p.mu.RLock()
			track := session.Tracks[i]
			p.mu.RUnlock()

			if track.DeezerMatch == nil {
				continue
			}

			results, err := p.navidromeClient.Search(ctx, track.ParsedArtist, track.ParsedSong)
			if err == nil && len(results) > 0 {
				p.mu.Lock()
				// Deselect if it was selected before marking as skipped.
				if session.Tracks[i].Selected {
					session.Tracks[i].Selected = false
					session.Progress.Selected--
				}
				session.Tracks[i].Status = TrackSkipped
				session.Progress.Skipped++
				p.mu.Unlock()
			}

			if i < len(session.Tracks)-1 {
				time.Sleep(p.checkDelay)
			}
		}
	}

	// Analysis complete - wait for user to trigger download.
	p.mu.Lock()
	session.Status = StatusReady
	log.Printf("[sync] session %s ready: %d selected, %d skipped, %d needs review, %d not found",
		session.ID, session.Progress.Selected, session.Progress.Skipped, session.Progress.NeedsReview, session.Progress.NotFound)
	p.mu.Unlock()
}

func (p *Pipeline) setError(session *Session, msg string) {
	p.mu.Lock()
	session.Status = StatusError
	session.Error = msg
	log.Printf("[sync] session %s error: %s", session.ID, msg)
	p.mu.Unlock()
}

// checkpoint checks for cancellation or pause signals.
// Returns an error if the context is canceled, or blocks if paused until resumed.
func (p *Pipeline) checkpoint(ctx context.Context, session *Session, previousStatus string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	p.mu.RLock()
	ctrl, ok := p.controls[session.ID]
	p.mu.RUnlock()
	if !ok {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ctrl.pauseCh:
		p.mu.Lock()
		session.Status = StatusPaused
		log.Printf("[sync] session %s paused", session.ID)
		p.mu.Unlock()

		// Wait for resume or cancel.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ctrl.resumeCh:
			p.mu.Lock()
			session.Status = previousStatus
			log.Printf("[sync] session %s resumed", session.ID)
			p.mu.Unlock()
		}
	default:
		// Not paused, continue.
	}
	return nil
}

// PauseSession pauses the session, allowing it to be resumed later.
func (p *Pipeline) PauseSession(sessionID string) error {
	p.mu.RLock()
	session, ok := p.sessions[sessionID]
	ctrl, ctrlOk := p.controls[sessionID]
	p.mu.RUnlock()

	if !ok || !ctrlOk {
		return ErrSessionNotFound
	}

	// Only allow pausing active operations.
	switch session.Status {
	case StatusFetching, StatusParsing, StatusSearching, StatusChecking, StatusDownloading:
		// Valid states for pausing.
	case StatusPaused:
		return ErrSessionPaused
	default:
		return ErrSessionNotReady
	}

	// Signal pause (non-blocking).
	select {
	case ctrl.pauseCh <- struct{}{}:
	default:
		// Already signaled.
	}

	log.Printf("[sync] session %s pause requested", sessionID)
	return nil
}

// ResumeSession resumes a paused session.
func (p *Pipeline) ResumeSession(sessionID string) error {
	p.mu.RLock()
	session, ok := p.sessions[sessionID]
	ctrl, ctrlOk := p.controls[sessionID]
	p.mu.RUnlock()

	if !ok || !ctrlOk {
		return ErrSessionNotFound
	}

	if session.Status != StatusPaused {
		return ErrSessionNotPaused
	}

	// Signal resume (non-blocking).
	select {
	case ctrl.resumeCh <- struct{}{}:
	default:
		// Already signaled.
	}

	log.Printf("[sync] session %s resume requested", sessionID)
	return nil
}

// CancelSession cancels a session, stopping it permanently.
func (p *Pipeline) CancelSession(sessionID string) error {
	p.mu.Lock()
	session, ok := p.sessions[sessionID]
	ctrl, ctrlOk := p.controls[sessionID]
	p.mu.Unlock()

	if !ok {
		return ErrSessionNotFound
	}

	// Check if already in terminal state.
	switch session.Status {
	case StatusDone, StatusError, StatusCanceled:
		return ErrSessionCanceled
	}

	// Cancel the context if we have control.
	if ctrlOk {
		ctrl.cancel()
		// Also signal resume in case it's paused, so it can exit.
		select {
		case ctrl.resumeCh <- struct{}{}:
		default:
		}
	}

	p.mu.Lock()
	session.Status = StatusCanceled
	p.mu.Unlock()

	log.Printf("[sync] session %s canceled", sessionID)
	return nil
}

func buildQuery(artist, song string) string {
	if artist == "" {
		return song
	}
	return artist + " " + song
}

// cleanChannel removes common noise from YouTube channel names.
func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Download queues all selected tracks for download.
// Only works when session is in StatusReady state.
func (p *Pipeline) Download(ctx context.Context, sessionID string) error {
	p.mu.Lock()
	session, ok := p.sessions[sessionID]
	if !ok {
		p.mu.Unlock()
		return ErrSessionNotFound
	}
	if session.Status != StatusReady {
		p.mu.Unlock()
		return ErrSessionNotReady
	}
	session.Status = StatusDownloading

	// Create cancellable context and control channels for download.
	ctx, cancel := context.WithCancel(ctx)
	ctrl := &sessionControl{
		cancel:   cancel,
		pauseCh:  make(chan struct{}, 1),
		resumeCh: make(chan struct{}, 1),
	}
	p.controls[sessionID] = ctrl
	p.mu.Unlock()

	log.Printf("[sync] session %s: starting download of %d selected tracks", sessionID, session.Progress.Selected)

	for i := range session.Tracks {
		if err := p.checkpoint(ctx, session, StatusDownloading); err != nil {
			if session.Status != StatusCanceled {
				p.setError(session, "canceled")
			}
			return err
		}

		p.mu.RLock()
		track := session.Tracks[i]
		p.mu.RUnlock()

		if !track.Selected || track.DeezerMatch == nil {
			continue
		}

		err := p.deemixClient.AddToQueue(ctx, track.DeezerMatch.Link, session.Bitrate)

		p.mu.Lock()
		if err != nil {
			session.Tracks[i].Status = TrackError
		} else {
			session.Tracks[i].Status = TrackDownloaded
			session.Progress.Queued++
		}
		p.mu.Unlock()

		time.Sleep(p.queueDelay)
	}

	p.mu.Lock()
	session.Status = StatusDone
	log.Printf("[sync] session %s done: %d queued", sessionID, session.Progress.Queued)
	p.mu.Unlock()

	return nil
}

// SetTrackSelected toggles the selection state of a track.
// Only works when session is in StatusReady state.
func (p *Pipeline) SetTrackSelected(sessionID string, trackIndex int, selected bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	session, ok := p.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}
	if session.Status != StatusReady {
		return ErrSessionNotReady
	}
	if trackIndex < 0 || trackIndex >= len(session.Tracks) {
		return ErrTrackNotFound
	}

	track := &session.Tracks[trackIndex]
	if track.Selected == selected {
		return nil // No change needed
	}

	track.Selected = selected
	if selected {
		session.Progress.Selected++
	} else {
		session.Progress.Selected--
	}

	log.Printf("[sync] session %s: track %d selected=%v", sessionID, trackIndex, selected)
	return nil
}

// SearchTrack performs a manual Deezer search for a track and updates its match.
// Only works when session is in StatusReady state.
func (p *Pipeline) SearchTrack(ctx context.Context, sessionID string, trackIndex int, query string) error {
	p.mu.Lock()
	session, ok := p.sessions[sessionID]
	if !ok {
		p.mu.Unlock()
		return ErrSessionNotFound
	}
	if session.Status != StatusReady {
		p.mu.Unlock()
		return ErrSessionNotReady
	}
	if trackIndex < 0 || trackIndex >= len(session.Tracks) {
		p.mu.Unlock()
		return ErrTrackNotFound
	}
	checkNavidrome := session.CheckNavidrome
	parsedArtist := session.Tracks[trackIndex].ParsedArtist
	p.mu.Unlock()

	// Combine parsed artist with user query for better Deezer results.
	searchQuery := buildQuery(parsedArtist, query)
	results, err := p.deemixClient.Search(ctx, searchQuery)
	if err != nil {
		return err
	}

	// Check Navidrome for the new match (outside lock).
	var existsInNavidrome bool
	if len(results) > 0 && p.navidromeClient != nil && checkNavidrome {
		match := results[0]
		navResults, err := p.navidromeClient.Search(ctx, match.Artist, match.Title)
		if err == nil && len(navResults) > 0 {
			existsInNavidrome = true
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	track := &session.Tracks[trackIndex]
	prevStatus := track.Status

	if len(results) == 0 {
		track.DeezerMatch = nil
		track.Confidence = 0
		if prevStatus != TrackNotFound {
			track.Status = TrackNotFound
			p.updateProgressForStatusChange(session, prevStatus, TrackNotFound, track.Selected)
			track.Selected = false
		}
		return nil
	}

	match := results[0]
	track.DeezerMatch = &match
	track.Confidence = calculateConfidence(track.ParsedArtist, track.ParsedSong, match.Artist, match.Title)

	var newStatus string
	if existsInNavidrome {
		newStatus = TrackSkipped
		track.Selected = false
	} else if track.Confidence >= p.confidenceThreshold {
		newStatus = TrackFound
		track.Selected = true
	} else {
		newStatus = TrackNeedsReview
		track.Selected = false
	}

	track.Status = newStatus
	p.updateProgressForStatusChange(session, prevStatus, newStatus, false)

	log.Printf("[sync] session %s: track %d manual search found: %s - %s (status: %s)", sessionID, trackIndex, match.Artist, match.Title, newStatus)
	return nil
}

// updateProgressForStatusChange adjusts session progress counters when a track status changes.
// Must be called with p.mu held.
func (p *Pipeline) updateProgressForStatusChange(session *Session, oldStatus, newStatus string, wasSelected bool) {
	if oldStatus == newStatus {
		return
	}

	// Decrement old status counter.
	switch oldStatus {
	case TrackNotFound:
		session.Progress.NotFound--
	case TrackNeedsReview:
		session.Progress.NeedsReview--
	case TrackSkipped:
		session.Progress.Skipped--
	}
	if wasSelected {
		session.Progress.Selected--
	}

	// Increment new status counter.
	switch newStatus {
	case TrackNotFound:
		session.Progress.NotFound++
	case TrackNeedsReview:
		session.Progress.NeedsReview++
	case TrackSkipped:
		session.Progress.Skipped++
	}
}
