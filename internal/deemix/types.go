package deemix

// SearchResult represents a track found on Deezer via Deemix.
type SearchResult struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int    `json:"duration"`
	Link     string `json:"link"`
}

// Bitrate constants for Deemix queue requests.
const (
	BitrateFLAC = 9 // FLAC quality
	Bitrate320  = 3 // MP3 320kbps
	Bitrate128  = 1 // MP3 128kbps
)
