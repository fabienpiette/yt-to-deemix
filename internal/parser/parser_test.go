package parser

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		wantArtist string
		wantSong   string
	}{
		// Standard delimiter formats
		{
			name:       "basic hyphen",
			title:      "Arctic Monkeys - Do I Wanna Know?",
			wantArtist: "Arctic Monkeys",
			wantSong:   "Do I Wanna Know?",
		},
		{
			name:       "en-dash delimiter",
			title:      "Radiohead – Creep",
			wantArtist: "Radiohead",
			wantSong:   "Creep",
		},
		{
			name:       "em-dash delimiter",
			title:      "Nirvana — Smells Like Teen Spirit",
			wantArtist: "Nirvana",
			wantSong:   "Smells Like Teen Spirit",
		},
		{
			name:       "pipe delimiter",
			title:      "Daft Punk | Get Lucky",
			wantArtist: "Daft Punk",
			wantSong:   "Get Lucky",
		},
		{
			name:       "tilde delimiter",
			title:      "Gorillaz ~ Feel Good Inc",
			wantArtist: "Gorillaz",
			wantSong:   "Feel Good Inc",
		},

		// Noise removal
		{
			name:       "official video suffix",
			title:      "The Weeknd - Blinding Lights (Official Video)",
			wantArtist: "The Weeknd",
			wantSong:   "Blinding Lights",
		},
		{
			name:       "official music video",
			title:      "Billie Eilish - bad guy (Official Music Video)",
			wantArtist: "Billie Eilish",
			wantSong:   "bad guy",
		},
		{
			name:       "lyrics in brackets",
			title:      "Imagine Dragons - Believer [Lyrics]",
			wantArtist: "Imagine Dragons",
			wantSong:   "Believer",
		},
		{
			name:       "audio tag",
			title:      "Adele - Hello (Audio)",
			wantArtist: "Adele",
			wantSong:   "Hello",
		},
		{
			name:       "HD tag",
			title:      "Queen - Bohemian Rhapsody [HD]",
			wantArtist: "Queen",
			wantSong:   "Bohemian Rhapsody",
		},
		{
			name:       "official audio",
			title:      "Taylor Swift - Anti-Hero (Official Audio)",
			wantArtist: "Taylor Swift",
			wantSong:   "Anti-Hero",
		},
		{
			name:       "MV tag",
			title:      "BTS - Dynamite [MV]",
			wantArtist: "BTS",
			wantSong:   "Dynamite",
		},
		{
			name:       "visualizer tag",
			title:      "The Strokes - Bad Decisions (Visualizer)",
			wantArtist: "The Strokes",
			wantSong:   "Bad Decisions",
		},
		{
			name:       "trailing official video no brackets",
			title:      "Tame Impala - The Less I Know The Better - Official Video",
			wantArtist: "Tame Impala",
			wantSong:   "The Less I Know The Better",
		},
		{
			name:       "4K tag",
			title:      "Pink Floyd - Comfortably Numb [4K]",
			wantArtist: "Pink Floyd",
			wantSong:   "Comfortably Numb",
		},

		// Featured artists
		{
			name:       "feat in parens",
			title:      "Calvin Harris - This Is What You Came For (feat. Rihanna)",
			wantArtist: "Calvin Harris",
			wantSong:   "This Is What You Came For",
		},
		{
			name:       "ft in song",
			title:      "Post Malone - Sunflower ft. Swae Lee",
			wantArtist: "Post Malone",
			wantSong:   "Sunflower feat. Swae Lee",
		},

		// "by" pattern
		{
			name:       "song by artist",
			title:      "Lovely by Billie Eilish",
			wantArtist: "Billie Eilish",
			wantSong:   "Lovely",
		},

		// Fallback
		{
			name:       "no delimiter single word",
			title:      "Wonderwall",
			wantArtist: "",
			wantSong:   "Wonderwall",
		},
		{
			name:       "no delimiter multi word",
			title:      "Bohemian Rhapsody Live",
			wantArtist: "",
			wantSong:   "Bohemian Rhapsody Live",
		},
		{
			name:       "noise removed then fallback",
			title:      "Stairway to Heaven (Official Audio)",
			wantArtist: "",
			wantSong:   "Stairway to Heaven",
		},

		// Quoted title format
		{
			name:       "quoted title with suffix",
			title:      `Snoop Lion "Here Comes the King" (Official Lyric Video)`,
			wantArtist: "Snoop Lion",
			wantSong:   "Here Comes the King",
		},
		{
			name:       "quoted title clean",
			title:      `Eminem "Lose Yourself"`,
			wantArtist: "Eminem",
			wantSong:   "Lose Yourself",
		},

		// Topic channel suffix
		{
			name:       "topic suffix removal",
			title:      "Dua Lipa - Levitating - Topic",
			wantArtist: "Dua Lipa",
			wantSong:   "Levitating",
		},

		// Multiple noise tags
		{
			name:       "multiple tags",
			title:      "Kendrick Lamar - HUMBLE. (Official Music Video) [HD]",
			wantArtist: "Kendrick Lamar",
			wantSong:   "HUMBLE.",
		},

		// Edge cases
		{
			name:       "extra whitespace",
			title:      "  Oasis   -   Wonderwall  ",
			wantArtist: "Oasis",
			wantSong:   "Wonderwall",
		},
		{
			name:       "video oficial spanish",
			title:      "Bad Bunny - Titi Me Pregunto (Video Oficial)",
			wantArtist: "Bad Bunny",
			wantSong:   "Titi Me Pregunto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotArtist, gotSong := Parse(tt.title)
			if gotArtist != tt.wantArtist {
				t.Errorf("Parse(%q) artist = %q, want %q", tt.title, gotArtist, tt.wantArtist)
			}
			if gotSong != tt.wantSong {
				t.Errorf("Parse(%q) song = %q, want %q", tt.title, gotSong, tt.wantSong)
			}
		})
	}
}
