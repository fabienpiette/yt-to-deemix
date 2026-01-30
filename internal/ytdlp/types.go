package ytdlp

// PlaylistEntry represents a single video from a YouTube playlist.
type PlaylistEntry struct {
	Title   string `json:"title"`
	VideoID string `json:"id"`
	URL     string `json:"url"`
	// Artist and Track are provided by yt-dlp for YouTube Music content.
	Artist  string `json:"artist,omitempty"`
	Track   string `json:"track,omitempty"`
	Channel string `json:"channel,omitempty"`
}

// ChannelPlaylist represents a playlist found on a YouTube channel.
type ChannelPlaylist struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}
