package navidrome

import "testing"

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"kitten", "sitting", 3},
		{"creep", "creep", 0},
		{"Creep", "creep", 1}, // case-sensitive
	}
	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSimilarity(t *testing.T) {
	tests := []struct {
		a, b    string
		wantMin float64
		wantMax float64
	}{
		{"creep", "creep", 1.0, 1.0},
		{"", "", 1.0, 1.0},
		{"abc", "xyz", 0.0, 0.01},
		{"do i wanna know?", "do i wanna know", 0.9, 1.0},
		{"radiohead", "radioheed", 0.8, 0.9},
	}
	for _, tt := range tests {
		got := similarity(tt.a, tt.b)
		if got < tt.wantMin || got > tt.wantMax {
			t.Errorf("similarity(%q, %q) = %.3f, want [%.2f, %.2f]", tt.a, tt.b, got, tt.wantMin, tt.wantMax)
		}
	}
}

func TestMatchSong_Substring(t *testing.T) {
	tests := []struct {
		songArtist, songTitle   string
		queryArtist, queryTitle string
		want                    bool
	}{
		{"Arctic Monkeys", "Do I Wanna Know? (Official)", "Arctic Monkeys", "Do I Wanna Know?", true},
		{"Arctic Monkeys", "Do I Wanna Know?", "arctic monkeys", "do i wanna know?", true},
		{"Other Band", "Other Song", "Arctic Monkeys", "Do I Wanna Know?", false},
		{"Arctic Monkeys", "R U Mine?", "Arctic Monkeys", "Do I Wanna Know?", false},
	}
	for _, tt := range tests {
		got := matchSong(MatchSubstring, tt.songArtist, tt.songTitle, tt.queryArtist, tt.queryTitle)
		if got != tt.want {
			t.Errorf("matchSong(substring, %q/%q, %q/%q) = %v, want %v",
				tt.songArtist, tt.songTitle, tt.queryArtist, tt.queryTitle, got, tt.want)
		}
	}
}

func TestMatchSong_Exact(t *testing.T) {
	tests := []struct {
		songArtist, songTitle   string
		queryArtist, queryTitle string
		want                    bool
	}{
		{"Arctic Monkeys", "Do I Wanna Know?", "Arctic Monkeys", "Do I Wanna Know?", true},
		{"Arctic Monkeys", "Do I Wanna Know?", "arctic monkeys", "do i wanna know?", true},
		// Substring should NOT match in exact mode:
		{"Arctic Monkeys", "Do I Wanna Know? (Official)", "Arctic Monkeys", "Do I Wanna Know?", false},
	}
	for _, tt := range tests {
		got := matchSong(MatchExact, tt.songArtist, tt.songTitle, tt.queryArtist, tt.queryTitle)
		if got != tt.want {
			t.Errorf("matchSong(exact, %q/%q, %q/%q) = %v, want %v",
				tt.songArtist, tt.songTitle, tt.queryArtist, tt.queryTitle, got, tt.want)
		}
	}
}

func TestMatchSong_Fuzzy(t *testing.T) {
	tests := []struct {
		songArtist, songTitle   string
		queryArtist, queryTitle string
		want                    bool
	}{
		// Exact match → passes fuzzy
		{"Radiohead", "Creep", "Radiohead", "Creep", true},
		// Minor typo/variation → passes (similarity > 0.8)
		{"Radiohead", "Creep", "Radiohead", "Creeep", true},
		// Completely different → fails
		{"Other Band", "Other Song", "Radiohead", "Creep", false},
		// Short title with big difference → fails
		{"Radiohead", "Run", "Radiohead", "Running Up That Hill", false},
	}
	for _, tt := range tests {
		got := matchSong(MatchFuzzy, tt.songArtist, tt.songTitle, tt.queryArtist, tt.queryTitle)
		if got != tt.want {
			t.Errorf("matchSong(fuzzy, %q/%q, %q/%q) = %v, want %v",
				tt.songArtist, tt.songTitle, tt.queryArtist, tt.queryTitle, got, tt.want)
		}
	}
}
