# ytToDeemix

Transfer music from YouTube to a self-hosted Deemix instance. Works with playlists, single songs, and entire channels.

You have a YouTube playlist or a song link. You want those tracks on your server. This bridges the gap: it reads the URL, figures out what each video is, finds the matching track on Deezer, and queues it for download through your existing Deemix setup.

[![License: AGPL-3.0](https://img.shields.io/github/license/fabienpiette/yt-to-deemix)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/fabienpiette/yt-to-deemix)](go.mod)
[![Tag](https://img.shields.io/github/v/tag/fabienpiette/yt-to-deemix)](https://github.com/fabienpiette/yt-to-deemix/tags)

![ytToDeemix dashboard showing playlist sync results](docs/screenshot-light.png)

<p align="center">
<a href="https://buymeacoffee.com/fabienpiette" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me A Coffee" height="60"></a>
</p>

## Features

- **Queue multiple URLs** - Add playlists and songs to a queue before processing
- **Channel import** - Paste a YouTube channel URL to import all its playlists at once
- **Two-phase workflow** - Analyze first, review matches, then download selected tracks
- **Manual track selection** - Select/deselect individual tracks before downloading
- **Confidence scoring** - Low-confidence matches are flagged for manual review
- **Re-search tracks** - Search again with a custom query if the match is wrong
- **Pause/Resume/Cancel** - Full control over analyze and download operations
- **Navidrome integration** - Skip tracks already in your library
- **Dark/Light theme** - Toggle in the UI header
- **Optimized delivery** - Static assets are embedded, minified, and gzip-compressed

## How it works

1. Add YouTube URLs to the queue (playlists, songs, or channels)
2. Click **Analyze** - yt-dlp fetches metadata, titles are parsed, Deezer matches are found
3. Review the results - high-confidence matches are pre-selected, low-confidence flagged
4. Adjust selections, re-search mismatches if needed
5. Click **Download** - selected tracks are queued in Deemix at your chosen bitrate

## Requirements

- A running Deemix instance
- A Deezer ARL token (grab from your browser cookies on deezer.com)
- Docker (recommended) or Go 1.24+ and yt-dlp installed locally

## Quick start

```bash
cp .env.example .env
# Edit .env with your DEEMIX_URL and DEEMIX_ARL

docker compose up -d
```

Open `http://localhost:8080` in your browser.

## Deploy with Docker Compose

```yaml
services:
  yttodeemix:
    image: ghcr.io/fabienpiette/yttodeemix:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - DEEMIX_URL=http://deemix:6595
      - DEEMIX_ARL=your_arl_token_here
      # Optional: Navidrome integration
      # - NAVIDROME_URL=http://navidrome:4533
      # - NAVIDROME_USER=admin
      # - NAVIDROME_PASSWORD=secret
      # - NAVIDROME_MATCH_MODE=substring
```

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DEEMIX_URL` | yes | `http://localhost:6595` | Your Deemix instance URL |
| `DEEMIX_ARL` | yes | - | Deezer ARL authentication token |
| `PORT` | no | `8080` | Web server port |
| `CONFIDENCE_THRESHOLD` | no | `70` | Minimum confidence score (0-100) for auto-selection |
| `NAVIDROME_URL` | no | - | Navidrome/Subsonic instance URL |
| `NAVIDROME_USER` | no | - | Navidrome username |
| `NAVIDROME_PASSWORD` | no | - | Navidrome password |
| `NAVIDROME_MATCH_MODE` | no | `substring` | How to match tracks: `substring`, `exact`, or `fuzzy` |
| `NAVIDROME_SKIP_DEFAULT` | no | `false` | Enable "skip existing" toggle by default in UI |
| `DEV` | no | - | Set to `1` to serve static files from disk (development) |

## Running locally

```bash
# Install yt-dlp if you don't have it
pip install yt-dlp

# Build and run
make run
```

## Development

```bash
make dev            # Run with live reload (serves static files from disk)
make test           # Run tests with race detector
make test-coverage  # Run tests with coverage report
make fmt            # Check formatting
make build-all      # Cross-compile for all platforms
```

Static assets (CSS, JS, SVG) are embedded in the binary and minified + gzipped at startup (~85% size reduction). Use `make dev` during development to serve files from disk without rebuilding.

## API

### Session workflow

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/analyze` | Start analysis. Body: `{"url": "...", "bitrate": 1, "check_navidrome": false}` |
| GET | `/api/session/{id}` | Get session state (tracks, progress, status) |
| POST | `/api/session/{id}/download` | Start downloading selected tracks |
| POST | `/api/session/{id}/pause` | Pause the current operation |
| POST | `/api/session/{id}/resume` | Resume a paused session |
| POST | `/api/session/{id}/cancel` | Cancel the session |

### Track operations

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/session/{id}/track/{index}/select` | Select/deselect track. Body: `{"selected": true}` |
| POST | `/api/session/{id}/track/{index}/search` | Re-search track. Body: `{"query": "artist song"}` |

### Utilities

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/channel/playlists?url=...` | Get all playlists from a YouTube channel |
| GET | `/api/url/info?url=...` | Get title for a YouTube URL |
| GET | `/api/stats` | Server stats (memory, goroutines, uptime) |
| GET | `/api/navidrome/status` | Navidrome integration status |

**Bitrate values:** `9` (FLAC), `3` (320 kbps), `1` (128 kbps)

## Session statuses

| Status | Description |
|--------|-------------|
| `fetching` | Fetching playlist metadata from YouTube |
| `parsing` | Parsing video titles to extract artist/song |
| `searching` | Searching tracks on Deezer |
| `checking` | Checking Navidrome for existing tracks |
| `ready` | Analysis complete, waiting for user action |
| `downloading` | Queuing tracks to Deemix |
| `done` | All selected tracks queued |
| `paused` | Operation paused by user |
| `canceled` | Session canceled by user |
| `error` | An error occurred |

## Track statuses

| Status | Description |
|--------|-------------|
| `pending` | Waiting to be processed |
| `searching` | Currently searching on Deezer |
| `found` | Match found with high confidence (auto-selected) |
| `needs_review` | Match found but low confidence (not auto-selected) |
| `not_found` | No match found on Deezer |
| `skipped` | Already exists in Navidrome |
| `queued` | Successfully queued in Deemix |
| `error` | Failed to queue |

## Navidrome integration

When configured, ytToDeemix can check your Navidrome library before downloading, skipping tracks you already have. It uses the Subsonic API (`search2` endpoint), so it works with any Subsonic-compatible server.

All three `NAVIDROME_*` connection variables must be set to enable this feature. When enabled, a "skip existing" toggle appears in the UI.

**Match modes:**

| Mode | Behaviour |
|------|-----------|
| `substring` | Title and artist are contained within the Navidrome entry (case-insensitive). Catches variants like "(Remastered)". |
| `exact` | Title and artist must match exactly (case-insensitive). Strictest mode. |
| `fuzzy` | Levenshtein similarity >= 80%. Tolerates minor typos or punctuation differences. |

## Confidence scoring

Each Deezer match is assigned a confidence score (0-100%) based on how well the artist and title match the parsed YouTube title.

- **High confidence** (≥70% by default): Auto-selected for download
- **Low confidence** (<70%): Flagged for review, not auto-selected

This prevents incorrect downloads when the parser extracts the wrong artist. For example, "I'm Alive" without artist info might match Céline Dion instead of Anthrax - the confidence score catches this.

**Scoring formula:**
- 40% weight on artist similarity
- 60% weight on title similarity
- If no artist was parsed, max confidence capped at 60%

## Title parsing

The parser handles common YouTube music title formats:

- `Artist - Song`
- `Artist - Song (Official Video)`
- `Artist - Song [Lyrics]`
- `Artist "Song Title"`
- `Song by Artist`
- Various noise: `[HD]`, `(Audio)`, `[MV]`, `(Visualizer)`, `(Official Lyric Video)`, etc.

When parsing fails, the full cleaned title is used as the search query.

## Channel import

Paste a YouTube channel URL to fetch all its public playlists:

- `youtube.com/@username`
- `youtube.com/channel/UCxxxxx`
- `youtube.com/c/channelname`
- `youtube.com/user/username`

The playlists appear in the queue with clickable links. Select which ones to include before analyzing.

## License

AGPL-3.0
