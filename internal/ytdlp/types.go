package ytdlp

// PlaylistEntry represents a single video from a YouTube playlist.
type PlaylistEntry struct {
	Title   string `json:"title"`
	VideoID string `json:"id"`
	URL     string `json:"url"`
}

// ChannelPlaylist represents a playlist found on a YouTube channel.
type ChannelPlaylist struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}
