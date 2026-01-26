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

// Pipeline manages sync sessions.
type Pipeline struct {
	ytClient            ytdlp.Client
	deemixClient        deemix.Client
	navidromeClient     navidrome.Client
	sessions            map[string]*Session
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

// Start begins a new sync session for the given playlist URL and bitrate.
// Returns the session ID immediately; processing runs in a goroutine.
func (p *Pipeline) Start(ctx context.Context, playlistURL string, bitrate int, checkNavidrome bool) string {
	id := generateID()
	session := &Session{
		ID:             id,
		URL:            playlistURL,
		Status:         StatusFetching,
		Bitrate:        bitrate,
		CheckNavidrome: checkNavidrome,
	}

	p.mu.Lock()
	p.sessions[id] = session
	p.mu.Unlock()

	log.Printf("[sync] session %s started: %s", id, playlistURL)
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
		artist, song := parser.Parse(entry.Title)
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
		if ctx.Err() != nil {
			p.setError(session, "canceled")
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
			if ctx.Err() != nil {
				p.setError(session, "canceled")
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
				session.Tracks[i].Status = TrackSkipped
				session.Progress.Skipped++
				p.mu.Unlock()
			}

			if i < len(session.Tracks)-1 {
				time.Sleep(p.checkDelay)
			}
		}
	}

	// Phase 4: Queue all found tracks.
	p.mu.Lock()
	session.Status = StatusQueuing
	p.mu.Unlock()

	for i := range session.Tracks {
		if ctx.Err() != nil {
			p.setError(session, "canceled")
			return
		}

		p.mu.RLock()
		track := session.Tracks[i]
		p.mu.RUnlock()

		if track.DeezerMatch == nil || track.Status == TrackSkipped || track.Status == TrackNeedsReview {
			continue
		}

		err := p.deemixClient.AddToQueue(ctx, track.DeezerMatch.Link, session.Bitrate)

		p.mu.Lock()
		if err != nil {
			session.Tracks[i].Status = TrackError
		} else {
			session.Tracks[i].Status = TrackQueued
			session.Progress.Queued++
		}
		p.mu.Unlock()

		time.Sleep(p.queueDelay)
	}

	p.mu.Lock()
	session.Status = StatusDone
	log.Printf("[sync] session %s done: %d queued, %d skipped, %d needs review, %d not found",
		session.ID, session.Progress.Queued, session.Progress.Skipped, session.Progress.NeedsReview, session.Progress.NotFound)
	p.mu.Unlock()
}

func (p *Pipeline) setError(session *Session, msg string) {
	p.mu.Lock()
	session.Status = StatusError
	session.Error = msg
	log.Printf("[sync] session %s error: %s", session.ID, msg)
	p.mu.Unlock()
}

func buildQuery(artist, song string) string {
	if artist == "" {
		return song
	}
	return artist + " " + song
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ApproveTrack queues a track that was marked as needs_review.
// Returns an error if the track doesn't exist or isn't in needs_review status.
func (p *Pipeline) ApproveTrack(ctx context.Context, sessionID string, trackIndex int) error {
	p.mu.Lock()
	session, ok := p.sessions[sessionID]
	if !ok {
		p.mu.Unlock()
		return ErrSessionNotFound
	}
	if trackIndex < 0 || trackIndex >= len(session.Tracks) {
		p.mu.Unlock()
		return ErrTrackNotFound
	}
	track := session.Tracks[trackIndex]
	if track.Status != TrackNeedsReview {
		p.mu.Unlock()
		return ErrTrackNotReviewable
	}
	if track.DeezerMatch == nil {
		p.mu.Unlock()
		return ErrNoMatch
	}
	p.mu.Unlock()

	// Queue the track.
	err := p.deemixClient.AddToQueue(ctx, track.DeezerMatch.Link, session.Bitrate)

	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		session.Tracks[trackIndex].Status = TrackError
		return err
	}
	session.Tracks[trackIndex].Status = TrackQueued
	session.Progress.Queued++
	session.Progress.NeedsReview--
	log.Printf("[sync] session %s: track %d approved and queued", sessionID, trackIndex)
	return nil
}

// RejectTrack marks a needs_review track as rejected (not_found).
func (p *Pipeline) RejectTrack(sessionID string, trackIndex int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	session, ok := p.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}
	if trackIndex < 0 || trackIndex >= len(session.Tracks) {
		return ErrTrackNotFound
	}
	if session.Tracks[trackIndex].Status != TrackNeedsReview {
		return ErrTrackNotReviewable
	}

	session.Tracks[trackIndex].Status = TrackNotFound
	session.Progress.NeedsReview--
	session.Progress.NotFound++
	log.Printf("[sync] session %s: track %d rejected", sessionID, trackIndex)
	return nil
}
