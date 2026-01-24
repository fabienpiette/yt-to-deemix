package navidrome

import "strings"

// MatchMode determines how Navidrome results are compared to the search query.
const (
	MatchSubstring = "substring" // default: case-insensitive substring
	MatchExact     = "exact"     // case-insensitive exact match
	MatchFuzzy     = "fuzzy"     // Levenshtein similarity >= 0.8
)

const fuzzySimilarityThreshold = 0.8

// matchSong returns true if the song matches the given artist/title according to mode.
func matchSong(mode, songArtist, songTitle, queryArtist, queryTitle string) bool {
	switch mode {
	case MatchExact:
		return strings.EqualFold(songTitle, queryTitle) &&
			strings.EqualFold(songArtist, queryArtist)
	case MatchFuzzy:
		return similarity(strings.ToLower(songTitle), strings.ToLower(queryTitle)) >= fuzzySimilarityThreshold &&
			similarity(strings.ToLower(songArtist), strings.ToLower(queryArtist)) >= fuzzySimilarityThreshold
	default: // substring
		return strings.Contains(strings.ToLower(songTitle), strings.ToLower(queryTitle)) &&
			strings.Contains(strings.ToLower(songArtist), strings.ToLower(queryArtist))
	}
}

// similarity returns a normalized similarity score [0.0, 1.0] using Levenshtein distance.
func similarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	if maxLen == 0 {
		return 1.0
	}
	dist := levenshtein(a, b)
	return 1.0 - float64(dist)/float64(maxLen)
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
