package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
)

// Client defines the interface for fetching YouTube playlist data.
type Client interface {
	GetPlaylist(ctx context.Context, playlistURL string) ([]PlaylistEntry, error)
}

// CommandClient implements Client by calling the yt-dlp binary.
type CommandClient struct {
	// BinaryPath is the path to the yt-dlp executable. Defaults to "yt-dlp".
	BinaryPath string
}

// NewClient creates a new yt-dlp CommandClient.
func NewClient() *CommandClient {
	return &CommandClient{BinaryPath: "yt-dlp"}
}

// GetPlaylist fetches all video entries from a YouTube playlist URL.
func (c *CommandClient) GetPlaylist(ctx context.Context, playlistURL string) ([]PlaylistEntry, error) {
	log.Printf("[ytdlp] fetching playlist: %s", playlistURL)
	bin := c.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	// For YouTube Music, use hybrid approach: flat first, then full metadata.
	if isPlaylistURL(playlistURL) && isYouTubeMusicURL(playlistURL) {
		return c.getYouTubeMusicPlaylist(ctx, bin, playlistURL)
	}

	args := []string{"--dump-json", "--no-warnings", "--ignore-errors"}
	if isPlaylistURL(playlistURL) {
		args = append(args, "--flat-playlist")
	} else {
		args = append(args, "--no-playlist")
	}
	args = append(args, playlistURL)

	entries, err := c.runYtdlp(ctx, bin, args, playlistURL)
	if err != nil {
		return nil, err
	}
	log.Printf("[ytdlp] fetched %d entries from playlist", len(entries))
	return entries, nil
}

// getYouTubeMusicPlaylist fetches YouTube Music playlists with full metadata.
// First fetches flat playlist for complete list, then fetches full metadata.
// Falls back to flat data for videos that fail full fetch.
func (c *CommandClient) getYouTubeMusicPlaylist(ctx context.Context, bin, playlistURL string) ([]PlaylistEntry, error) {
	// Step 1: Get flat playlist (fast, complete list).
	flatArgs := []string{"--dump-json", "--no-warnings", "--flat-playlist", playlistURL}
	flatEntries, err := c.runYtdlp(ctx, bin, flatArgs, playlistURL)
	if err != nil {
		return nil, err
	}

	// Step 2: Get full metadata (slower, may fail for some videos).
	fullArgs := []string{"--dump-json", "--no-warnings", "--ignore-errors", playlistURL}
	fullEntries, _ := c.runYtdlp(ctx, bin, fullArgs, playlistURL)

	// Build map of full entries by video ID.
	fullMap := make(map[string]PlaylistEntry)
	for _, e := range fullEntries {
		if e.VideoID != "" {
			fullMap[e.VideoID] = e
		}
	}

	// Merge: prefer full metadata, fallback to flat.
	result := make([]PlaylistEntry, 0, len(flatEntries))
	for _, flat := range flatEntries {
		if full, ok := fullMap[flat.VideoID]; ok {
			result = append(result, full)
		} else {
			result = append(result, flat)
		}
	}

	log.Printf("[ytdlp] YouTube Music: %d entries (%d with full metadata)", len(result), len(fullEntries))
	return result, nil
}

// runYtdlp executes yt-dlp and parses the JSON output.
func (c *CommandClient) runYtdlp(ctx context.Context, bin string, args []string, url string) ([]PlaylistEntry, error) {
	cmd := exec.CommandContext(ctx, bin, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	// Parse output even if command failed (some videos may have succeeded).
	var entries []PlaylistEntry
	dec := json.NewDecoder(&stdout)
	for {
		var entry PlaylistEntry
		if err := dec.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			// Skip malformed entries, continue parsing.
			continue
		}
		entries = append(entries, entry)
	}

	// Only fail if command failed AND we got no entries.
	if runErr != nil && len(entries) == 0 {
		log.Printf("[ytdlp] command failed for %s: %v", url, runErr)
		return nil, fmt.Errorf("yt-dlp failed: %w: %s", runErr, stderr.String())
	}

	return entries, nil
}

func isPlaylistURL(url string) bool {
	return strings.Contains(url, "list=")
}

func isYouTubeMusicURL(url string) bool {
	return strings.Contains(url, "music.youtube.com")
}

// GetChannelPlaylists fetches all playlist URLs from a YouTube channel.
func (c *CommandClient) GetChannelPlaylists(ctx context.Context, channelURL string) ([]ChannelPlaylist, error) {
	log.Printf("[ytdlp] fetching channel playlists: %s", channelURL)
	bin := c.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	// Ensure URL points to playlists tab.
	url := normalizeChannelURL(channelURL)

	args := []string{"--flat-playlist", "--dump-json", "--no-warnings", url}
	cmd := exec.CommandContext(ctx, bin, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("[ytdlp] command failed for channel %s: %v", channelURL, err)
		return nil, fmt.Errorf("yt-dlp failed: %w: %s", err, stderr.String())
	}

	var playlists []ChannelPlaylist
	dec := json.NewDecoder(&stdout)
	for {
		var entry struct {
			Type  string `json:"_type"`
			ID    string `json:"id"`
			Title string `json:"title"`
			URL   string `json:"url"`
		}
		if err := dec.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("failed to parse yt-dlp output: %w", err)
		}
		// Only include playlist entries (not videos).
		if entry.URL != "" && strings.Contains(entry.URL, "list=") {
			playlists = append(playlists, ChannelPlaylist{
				ID:    entry.ID,
				Title: entry.Title,
				URL:   entry.URL,
			})
		}
	}

	log.Printf("[ytdlp] found %d playlists on channel", len(playlists))
	return playlists, nil
}

// GetURLInfo fetches the title for a YouTube URL (playlist or video).
func (c *CommandClient) GetURLInfo(ctx context.Context, url string) (string, error) {
	log.Printf("[ytdlp] fetching URL info: %s", url)
	bin := c.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{"--dump-single-json", "--flat-playlist", "--no-warnings", url}
	cmd := exec.CommandContext(ctx, bin, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("[ytdlp] command failed for URL info %s: %v", url, err)
		return "", fmt.Errorf("yt-dlp failed: %w: %s", err, stderr.String())
	}

	var info struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return "", fmt.Errorf("failed to parse yt-dlp output: %w", err)
	}

	log.Printf("[ytdlp] URL title: %s", info.Title)
	return info.Title, nil
}

// normalizeChannelURL ensures the URL points to the channel's playlists tab.
func normalizeChannelURL(url string) string {
	url = strings.TrimSuffix(url, "/")
	if strings.HasSuffix(url, "/playlists") {
		return url
	}
	if strings.HasSuffix(url, "/videos") || strings.HasSuffix(url, "/shorts") ||
		strings.HasSuffix(url, "/streams") || strings.HasSuffix(url, "/community") {
		url = url[:strings.LastIndex(url, "/")]
	}
	return url + "/playlists"
}
