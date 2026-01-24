package sync

import "github.com/gndm/ytToDeemix/internal/deemix"

// Session represents a single sync operation from a YouTube playlist.
type Session struct {
	ID             string   `json:"id"`
	URL            string   `json:"url"`
	Status         string   `json:"status"`
	Error          string   `json:"error,omitempty"`
	Tracks         []Track  `json:"tracks"`
	Progress       Progress `json:"progress"`
	Bitrate        int      `json:"bitrate"`
	CheckNavidrome bool     `json:"check_navidrome,omitempty"`
}

// Track represents a single video being processed through the pipeline.
type Track struct {
	YouTubeTitle string               `json:"youtube_title"`
	ParsedArtist string               `json:"parsed_artist"`
	ParsedSong   string               `json:"parsed_song"`
	DeezerMatch  *deemix.SearchResult `json:"deezer_match,omitempty"`
	Status       string               `json:"status"`
}

// Progress holds aggregate counts for the session.
type Progress struct {
	Total    int `json:"total"`
	Searched int `json:"searched"`
	Queued   int `json:"queued"`
	NotFound int `json:"not_found"`
	Skipped  int `json:"skipped"`
}

// Status constants for sessions.
const (
	StatusFetching  = "fetching"
	StatusParsing   = "parsing"
	StatusSearching = "searching"
	StatusChecking  = "checking"
	StatusQueuing   = "queuing"
	StatusDone      = "done"
	StatusError     = "error"
)

// Track status constants.
const (
	TrackPending   = "pending"
	TrackSearching = "searching"
	TrackFound     = "found"
	TrackNotFound  = "not_found"
	TrackSkipped   = "skipped"
	TrackQueued    = "queued"
	TrackError     = "error"
)
