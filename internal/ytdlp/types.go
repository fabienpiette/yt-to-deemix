package ytdlp

// PlaylistEntry represents a single video from a YouTube playlist.
type PlaylistEntry struct {
	Title   string `json:"title"`
	VideoID string `json:"id"`
	URL     string `json:"url"`
}
