package navidrome

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Client provides search capability against a Navidrome/Subsonic instance.
type Client interface {
	Search(ctx context.Context, artist, title string) ([]SearchResult, error)
}

// HTTPClient implements Client using the Subsonic REST API.
type HTTPClient struct {
	BaseURL   string
	User      string
	Password  string
	MatchMode string // "substring" (default), "exact", or "fuzzy"
	Client    *http.Client
}

// subsonicResponse represents the outer JSON envelope from the Subsonic API.
type subsonicResponse struct {
	SubsonicResponse struct {
		Status        string                    `json:"status"`
		Error         *struct{ Message string } `json:"error,omitempty"`
		SearchResult2 struct {
			Song []subsonicSong `json:"song"`
		} `json:"searchResult2"`
	} `json:"subsonic-response"`
}

type subsonicSong struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int    `json:"duration"`
}

func (c *HTTPClient) Search(ctx context.Context, artist, title string) ([]SearchResult, error) {
	query := artist + " " + title
	params := url.Values{
		"query":     {query},
		"songCount": {"5"},
		"f":         {"json"},
		"v":         {"1.16.1"},
		"c":         {"ytToDeemix"},
		"u":         {c.User},
		"p":         {c.Password},
	}

	reqURL := strings.TrimRight(c.BaseURL, "/") + "/rest/search2?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("navidrome: build request: %w", err)
	}

	httpClient := c.Client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[navidrome] request failed: %v", err)
		return nil, fmt.Errorf("navidrome: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[navidrome] unexpected status %d", resp.StatusCode)
		return nil, fmt.Errorf("navidrome: unexpected status %d", resp.StatusCode)
	}

	var sr subsonicResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("navidrome: decode response: %w", err)
	}

	if sr.SubsonicResponse.Status != "ok" {
		msg := "unknown error"
		if sr.SubsonicResponse.Error != nil {
			msg = sr.SubsonicResponse.Error.Message
		}
		log.Printf("[navidrome] API error: %s", msg)
		return nil, fmt.Errorf("navidrome: API error: %s", msg)
	}

	// Filter results using the configured match mode.
	mode := c.MatchMode
	if mode == "" {
		mode = MatchSubstring
	}

	var results []SearchResult
	for _, song := range sr.SubsonicResponse.SearchResult2.Song {
		if matchSong(mode, song.Artist, song.Title, artist, title) {
			results = append(results, SearchResult{
				ID:       song.ID,
				Title:    song.Title,
				Artist:   song.Artist,
				Album:    song.Album,
				Duration: song.Duration,
			})
		}
	}

	return results, nil
}
