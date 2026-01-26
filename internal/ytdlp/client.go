package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	bin := c.BinaryPath
	if bin == "" {
		bin = "yt-dlp"
	}

	args := []string{"--dump-json", "--no-warnings"}
	if isPlaylistURL(playlistURL) {
		args = append(args, "--flat-playlist")
	} else {
		args = append(args, "--no-playlist")
	}
	args = append(args, playlistURL)

	cmd := exec.CommandContext(ctx, bin, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("yt-dlp failed: %w: %s", err, stderr.String())
	}

	var entries []PlaylistEntry
	dec := json.NewDecoder(&stdout)
	for {
		var entry PlaylistEntry
		if err := dec.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("failed to parse yt-dlp output: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func isPlaylistURL(url string) bool {
	return strings.Contains(url, "list=")
}

// GetChannelPlaylists fetches all playlist URLs from a YouTube channel.
func (c *CommandClient) GetChannelPlaylists(ctx context.Context, channelURL string) ([]ChannelPlaylist, error) {
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

	return playlists, nil
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
