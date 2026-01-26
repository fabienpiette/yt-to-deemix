package sync

import "strings"

// calculateConfidence returns a confidence score (0-100) for a Deezer match.
// Higher score = more confident the match is correct.
func calculateConfidence(parsedArtist, parsedSong, resultArtist, resultTitle string) int {
	// Normalize strings for comparison.
	parsedArtist = strings.ToLower(strings.TrimSpace(parsedArtist))
	parsedSong = strings.ToLower(strings.TrimSpace(parsedSong))
	resultArtist = strings.ToLower(strings.TrimSpace(resultArtist))
	resultTitle = strings.ToLower(strings.TrimSpace(resultTitle))

	// Title similarity is always calculated.
	titleSim := similarity(parsedSong, resultTitle)

	// If no artist was parsed, confidence is based only on title match (capped lower).
	if parsedArtist == "" {
		// Max 60% confidence when we don't have artist info.
		return int(titleSim * 60)
	}

	// Artist similarity.
	artistSim := similarity(parsedArtist, resultArtist)

	// Combined score: 40% artist + 60% title.
	combined := (artistSim * 0.4) + (titleSim * 0.6)
	return int(combined * 100)
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
