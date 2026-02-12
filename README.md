# ytToDeemix

Transfer YouTube playlists, songs, and channels to a self-hosted Deemix instance.

---

<p align="center">
  <img src="docs/demo.gif" alt="ytToDeemix dashboard showing playlist sync results" width="600">
</p>

## Quick Start

```bash
cp .env.example .env
# Edit .env with your DEEMIX_URL and DEEMIX_ARL

docker compose up -d
```

Open `http://localhost:8080`. Paste a YouTube URL, click **Analyze**, review the matches, click **Download**.

## Features

- **Queue multiple URLs** — playlists, songs, and entire channels in one go
- **Two-phase workflow** — analyze first, review matches, then download
- **Confidence scoring** — low-confidence matches flagged for manual review
- **Re-search tracks** — fix wrong matches with a custom search query
- **Pause / Resume / Cancel** — full control over operations
- **Navidrome integration** — skip tracks already in your library

## Install

**Prerequisites:** a running [Deemix](https://github.com/bambanah/deemix) instance and a [Deezer ARL token](https://www.google.com/search?q=deezer+arl+token).

### Docker Compose

```yaml
services:
  yttodeemix:
    image: ghcr.io/fabienpiette/yt-to-deemix:${YTTODEEMIX_VERSION:-latest}
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - DEEMIX_URL=http://deemix:6595
      - DEEMIX_ARL=your_arl_token_here
```

### Full stack (with Deemix and Navidrome)

```yaml
services:
  yttodeemix:
    image: ghcr.io/fabienpiette/yt-to-deemix:${YTTODEEMIX_VERSION:-latest}
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - DEEMIX_URL=http://deemix:6595
      - DEEMIX_ARL=${DEEMIX_ARL}
      - CONFIDENCE_THRESHOLD=70
      - NAVIDROME_URL=http://navidrome:4533
      - NAVIDROME_USER=${NAVIDROME_USER}
      - NAVIDROME_PASSWORD=${NAVIDROME_PASSWORD}
      - NAVIDROME_SKIP_DEFAULT=true
    depends_on:
      - deemix
      - navidrome

  deemix:
    image: ghcr.io/bambanah/deemix:${DEEMIX_VERSION:-latest}
    restart: unless-stopped
    ports:
      - "6595:6595"
    volumes:
      - ./downloads:/downloads
      - ./deemix-config:/config
    environment:
      - DEEMIX_SERVER_PORT=6595
      - DEEMIX_DATA_DIR=/config
      - DEEMIX_MUSIC_DIR=/downloads
      - DEEMIX_HOST=0.0.0.0
      - DEEMIX_SINGLE_USER=true
      - PUID=1000
      - PGID=1000
      - UMASK_SET=022

  navidrome:
    image: deluan/navidrome:${NAVIDROME_VERSION:-latest}
    restart: unless-stopped
    ports:
      - "4533:4533"
    volumes:
      - ./downloads:/music:ro
      - ./navidrome-data:/data
```

### From source

```bash
pip install yt-dlp  # dependency
make run
```

## Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DEEMIX_URL` | yes | `http://localhost:6595` | Deemix instance URL |
| `DEEMIX_ARL` | yes | — | Deezer ARL token |
| `PORT` | no | `8080` | Web server port |
| `CONFIDENCE_THRESHOLD` | no | `70` | Auto-selection threshold (0–100) |
| `NAVIDROME_URL` | no | — | Navidrome/Subsonic URL |
| `NAVIDROME_USER` | no | — | Navidrome username |
| `NAVIDROME_PASSWORD` | no | — | Navidrome password |
| `NAVIDROME_MATCH_MODE` | no | `substring` | `substring`, `exact`, or `fuzzy` |
| `NAVIDROME_SKIP_DEFAULT` | no | `false` | Enable "skip existing" by default |
| `DEV` | no | — | `1` to serve static files from disk |

## Usage

### Channel import

Paste a YouTube channel URL to import all its public playlists at once:

```
youtube.com/@username
youtube.com/channel/UCxxxxx
youtube.com/c/channelname
```

### Navidrome integration

When all three `NAVIDROME_*` connection variables are set, a "skip existing" toggle appears in the UI. Uses the Subsonic `search2` API, so it works with any Subsonic-compatible server.

| Match mode | Behaviour |
|------------|-----------|
| `substring` | Title/artist contained in the entry (case-insensitive). Catches "(Remastered)" variants. |
| `exact` | Exact match (case-insensitive). |
| `fuzzy` | Levenshtein similarity ≥ 80%. Tolerates minor typos. |

### Confidence scoring

Each Deezer match gets a score (0–100%) — 40% artist similarity, 60% title similarity. Tracks below the threshold are flagged for review instead of auto-selected. If no artist was parsed, confidence is capped at 60%.

## Development

```bash
make dev            # Live reload (serves static from disk)
make test           # Tests with race detector
make test-coverage  # Coverage report
make fmt            # Check formatting
make build-all      # Cross-compile for all platforms
```

Static assets are embedded in the binary and minified + gzipped at startup.

## Acknowledgments

Thanks to all [contributors](https://github.com/fabienpiette/quaycheck/graphs/contributors).

<p align="center">
<a href="https://buymeacoffee.com/fabienpiette" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me A Coffee" height="60"></a>
</p>

## License

[AGPL-3.0](LICENSE)