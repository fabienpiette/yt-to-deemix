package parser

import (
	"regexp"
	"strings"
)

// suffixPatterns matches common noise suffixes in YouTube music titles.
var suffixPatterns = regexp.MustCompile(`(?i)\s*[\(\[](official\s*(music\s*|lyric\s*)?video|official\s*audio|lyrics?\s*(video)?|audio|hd|hq|4k|music\s*video|lyric\s*video|mv|visuali[sz]er|live|remix|feat\.?[^\)\]]*|ft\.?[^\)\]]*|prod\.?[^\)\]]*|video\s*oficial)[\)\]]`)

// trailingNoise matches trailing markers not in brackets.
var trailingNoise = regexp.MustCompile(`(?i)\s*[-–—|]\s*(official\s*(music\s*)?video|official\s*audio|lyrics?\s*(video)?|audio|hd|hq|4k|music\s*video|mv|visuali[sz]er)\s*$`)

// topicSuffix matches " - Topic" channel name artifacts.
var topicSuffix = regexp.MustCompile(`(?i)\s*-\s*topic\s*$`)

// featPattern normalizes featured artist notation within the song title.
var featPattern = regexp.MustCompile(`(?i)\s*\b(feat\.?|ft\.?)\s+`)

// delimiters in priority order.
var delimiters = []string{" - ", " – ", " — ", " | ", " ~ "}

// quotedPattern matches Artist "Song Title" format (straight or curly quotes).
var quotedPattern = regexp.MustCompile("^(.+?)\\s+[\"\u201c](.+?)[\"\u201d]$")

// byPattern matches "Song by Artist" format.
var byPattern = regexp.MustCompile(`(?i)^(.+?)\s+by\s+(.+)$`)

// extraWhitespace collapses multiple spaces.
var extraWhitespace = regexp.MustCompile(`\s{2,}`)

// Parse extracts artist and song from a YouTube video title.
// Returns (artist, song). If parsing fails, artist is empty and song
// is the cleaned title (still usable as a search query).
func Parse(title string) (artist, song string) {
	cleaned := clean(title)

	// Try delimiter-based splitting.
	for _, delim := range delimiters {
		if idx := strings.Index(cleaned, delim); idx > 0 {
			a := strings.TrimSpace(cleaned[:idx])
			s := strings.TrimSpace(cleaned[idx+len(delim):])
			if a != "" && s != "" {
				a = topicSuffix.ReplaceAllString(a, "")
				return normalizeFeat(strings.TrimSpace(a)), normalizeFeat(s)
			}
		}
	}

	// Try quoted title: Artist "Song Title".
	if matches := quotedPattern.FindStringSubmatch(cleaned); matches != nil {
		a := strings.TrimSpace(matches[1])
		s := strings.TrimSpace(matches[2])
		if a != "" && s != "" {
			return normalizeFeat(a), normalizeFeat(s)
		}
	}

	// Try "Song by Artist" pattern.
	if matches := byPattern.FindStringSubmatch(cleaned); matches != nil {
		s := strings.TrimSpace(matches[1])
		a := strings.TrimSpace(matches[2])
		if a != "" && s != "" {
			return normalizeFeat(a), normalizeFeat(s)
		}
	}

	// Fallback: return cleaned title as song, no artist.
	return "", normalizeFeat(cleaned)
}

// clean removes noise from a title.
func clean(title string) string {
	s := title
	s = suffixPatterns.ReplaceAllString(s, "")
	s = trailingNoise.ReplaceAllString(s, "")
	s = topicSuffix.ReplaceAllString(s, "")
	s = extraWhitespace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return s
}

// normalizeFeat standardizes "feat." and "ft." to "feat.".
func normalizeFeat(s string) string {
	return strings.TrimSpace(featPattern.ReplaceAllString(s, " feat. "))
}
