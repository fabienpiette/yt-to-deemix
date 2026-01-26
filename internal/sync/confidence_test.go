package sync

import "testing"

func TestCalculateConfidence(t *testing.T) {
	tests := []struct {
		name         string
		parsedArtist string
		parsedSong   string
		resultArtist string
		resultTitle  string
		minConf      int
		maxConf      int
	}{
		{
			name:         "exact match",
			parsedArtist: "Anthrax",
			parsedSong:   "I'm Alive",
			resultArtist: "Anthrax",
			resultTitle:  "I'm Alive",
			minConf:      95,
			maxConf:      100,
		},
		{
			name:         "case insensitive match",
			parsedArtist: "ANTHRAX",
			parsedSong:   "i'm alive",
			resultArtist: "Anthrax",
			resultTitle:  "I'm Alive",
			minConf:      95,
			maxConf:      100,
		},
		{
			name:         "wrong artist",
			parsedArtist: "Anthrax",
			parsedSong:   "I'm Alive",
			resultArtist: "Céline Dion",
			resultTitle:  "I'm Alive",
			minConf:      50,
			maxConf:      70,
		},
		{
			name:         "no artist parsed - title match",
			parsedArtist: "",
			parsedSong:   "I'm Alive",
			resultArtist: "Céline Dion",
			resultTitle:  "I'm Alive",
			minConf:      55,
			maxConf:      60,
		},
		{
			name:         "no artist parsed - title mismatch",
			parsedArtist: "",
			parsedSong:   "I'm Alive",
			resultArtist: "Céline Dion",
			resultTitle:  "My Heart Will Go On",
			minConf:      0,
			maxConf:      30,
		},
		{
			name:         "partial title match",
			parsedArtist: "Metallica",
			parsedSong:   "Enter Sandman",
			resultArtist: "Metallica",
			resultTitle:  "Enter Sandman (Remastered)",
			minConf:      70,
			maxConf:      95,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conf := calculateConfidence(tc.parsedArtist, tc.parsedSong, tc.resultArtist, tc.resultTitle)
			if conf < tc.minConf || conf > tc.maxConf {
				t.Errorf("confidence = %d, want between %d and %d", conf, tc.minConf, tc.maxConf)
			}
		})
	}
}

func TestSimilarity(t *testing.T) {
	tests := []struct {
		a, b string
		min  float64
		max  float64
	}{
		{"hello", "hello", 1.0, 1.0},
		{"", "", 1.0, 1.0},
		{"hello", "hallo", 0.75, 0.85},
		{"abc", "xyz", 0.0, 0.1},
	}

	for _, tc := range tests {
		sim := similarity(tc.a, tc.b)
		if sim < tc.min || sim > tc.max {
			t.Errorf("similarity(%q, %q) = %f, want between %f and %f", tc.a, tc.b, sim, tc.min, tc.max)
		}
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		dist int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"hello", "hello", 0},
		{"hello", "hallo", 1},
		{"kitten", "sitting", 3},
	}

	for _, tc := range tests {
		dist := levenshtein(tc.a, tc.b)
		if dist != tc.dist {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, dist, tc.dist)
		}
	}
}
