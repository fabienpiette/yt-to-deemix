package ytdlp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGetPlaylist(t *testing.T) {
	// Create a fake yt-dlp script that outputs known JSON.
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "yt-dlp")

	script := `#!/bin/sh
echo '{"title":"Arctic Monkeys - Do I Wanna Know?","id":"bpOSxM0rNPM","url":"https://www.youtube.com/watch?v=bpOSxM0rNPM"}'
echo '{"title":"Tame Impala - The Less I Know The Better","id":"sBzrzS1Ag_g","url":"https://www.youtube.com/watch?v=sBzrzS1Ag_g"}'
echo '{"title":"Radiohead - Creep","id":"XFkzRNyygfk","url":"https://www.youtube.com/watch?v=XFkzRNyygfk"}'
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	client := &CommandClient{BinaryPath: fakeBin}
	entries, err := client.GetPlaylist(context.Background(), "https://www.youtube.com/playlist?list=test")
	if err != nil {
		t.Fatalf("GetPlaylist() error = %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	want := []PlaylistEntry{
		{Title: "Arctic Monkeys - Do I Wanna Know?", VideoID: "bpOSxM0rNPM", URL: "https://www.youtube.com/watch?v=bpOSxM0rNPM"},
		{Title: "Tame Impala - The Less I Know The Better", VideoID: "sBzrzS1Ag_g", URL: "https://www.youtube.com/watch?v=sBzrzS1Ag_g"},
		{Title: "Radiohead - Creep", VideoID: "XFkzRNyygfk", URL: "https://www.youtube.com/watch?v=XFkzRNyygfk"},
	}

	for i, entry := range entries {
		if entry != want[i] {
			t.Errorf("entry[%d] = %+v, want %+v", i, entry, want[i])
		}
	}
}

func TestGetPlaylistError(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "yt-dlp")

	script := `#!/bin/sh
echo "ERROR: Invalid URL" >&2
exit 1
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	client := &CommandClient{BinaryPath: fakeBin}
	_, err := client.GetPlaylist(context.Background(), "not-a-url")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetPlaylistEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "yt-dlp")

	script := `#!/bin/sh
# Empty playlist, no output
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	client := &CommandClient{BinaryPath: fakeBin}
	entries, err := client.GetPlaylist(context.Background(), "https://www.youtube.com/playlist?list=empty")
	if err != nil {
		t.Fatalf("GetPlaylist() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestGetPlaylistContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	client := NewClient()
	_, err := client.GetPlaylist(ctx, "https://www.youtube.com/playlist?list=test")
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}
