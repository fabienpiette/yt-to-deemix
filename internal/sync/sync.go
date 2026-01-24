package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/gndm/ytToDeemix/internal/deemix"
	"github.com/gndm/ytToDeemix/internal/parser"
	"github.com/gndm/ytToDeemix/internal/ytdlp"
)

// Pipeline manages sync sessions.
type Pipeline struct {
	ytClient     ytdlp.Client
	deemixClient deemix.Client
	sessions     map[string]*Session
	mu           sync.RWMutex
	searchDelay  time.Duration
	queueDelay   time.Duration
}

// NewPipeline creates a new sync pipeline with the given clients.
func NewPipeline(yt ytdlp.Client, dx deemix.Client) *Pipeline {
	return &Pipeline{
		ytClient:     yt,
		deemixClient: dx,
		sessions:     make(map[string]*Session),
		searchDelay:  200 * time.Millisecond,
		queueDelay:   100 * time.Millisecond,
	}
}

// Start begins a new sync session for the given playlist URL and bitrate.
// Returns the session ID immediately; processing runs in a goroutine.
func (p *Pipeline) Start(ctx context.Context, playlistURL string, bitrate int) string {
	id := generateID()
	session := &Session{
		ID:      id,
		URL:     playlistURL,
		Status:  StatusFetching,
		Bitrate: bitrate,
	}

	p.mu.Lock()
	p.sessions[id] = session
	p.mu.Unlock()

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
			session.Tracks[i].Status = TrackFound
		}
		session.Progress.Searched++
		p.mu.Unlock()

		if i < len(session.Tracks)-1 {
			time.Sleep(p.searchDelay)
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

		if track.DeezerMatch == nil {
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
	p.mu.Unlock()
}

func (p *Pipeline) setError(session *Session, msg string) {
	p.mu.Lock()
	session.Status = StatusError
	session.Error = msg
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
