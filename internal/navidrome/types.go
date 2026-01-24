package navidrome

// SearchResult represents a song found in Navidrome via the Subsonic API.
type SearchResult struct {
	ID       string
	Title    string
	Artist   string
	Album    string
	Duration int
}
