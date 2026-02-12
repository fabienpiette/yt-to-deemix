# Architecture

This document describes the high-level architecture of ytToDeemix.
If you want to familiarize yourself with the codebase, you are in the
right place.

## Bird's Eye View

ytToDeemix is a web application that transfers music from YouTube to a
self-hosted Deemix instance. A user pastes a YouTube URL (playlist, song,
or channel), the server fetches metadata via yt-dlp, parses video titles
to extract artist/song, searches Deezer for matches, and queues selected
tracks for download through Deemix.

The runtime has two phases per session. **Analysis** fetches the playlist,
parses titles, searches Deezer, optionally checks Navidrome for duplicates,
and presents results. **Download** sends selected tracks to Deemix's queue.
Both phases support pause, resume, and cancel.

```
                 ┌──────────────────────────┐
                 │   Browser (SPA)          │
                 └────────┬─────────────────┘
                          │ HTTP/JSON
                 ┌────────▼─────────────────┐
                 │   main.go (HTTP server)  │
                 └────────┬─────────────────┘
                          │
                 ┌────────▼─────────────────┐
                 │   sync.Pipeline          │
                 │   (session orchestration) │
                 └──┬────┬────┬────┬────────┘
                    │    │    │    │
               ┌────▼┐ ┌▼────▼┐ ┌▼────────┐
               │ytdlp│ │deemix│ │navidrome │
               └──┬──┘ └──────┘ └──────────┘
                  │
               ┌──▼───┐
               │parser│
               └──────┘
```

## Code Map

### `main.go`

Entry point. Loads configuration from environment variables, creates
clients, registers HTTP routes, and starts the server. All route handlers
live here — they decode JSON, call Pipeline methods, and encode responses.

Key types: the route handlers (anonymous functions registered on
`http.ServeMux`).

### `static.go`

Embeds static assets into the binary at compile time via `//go:embed`.
At startup, minifies CSS/JS/SVG with `tdewolff/minify`, gzip-compresses
the result, and caches everything in memory. Serves pre-compressed content
with content negotiation on `Accept-Encoding`.

**Architecture Invariant:** in production mode, static assets are never
read from disk after startup. The `DEV=1` flag switches to disk serving
for development.

### `internal/sync/`

Core orchestration. `Pipeline` manages concurrent sessions, each running
in its own goroutine. Owns the session lifecycle: analyze, download,
pause, resume, cancel.

Key files: `sync.go` (Pipeline, session lifecycle), `types.go` (Session,
Track, Progress, status constants), `confidence.go` (match scoring).

**Architecture Invariant:** all session state is accessed through
`Pipeline.mu` (RWMutex). Handlers never hold a direct reference to
mutable session data.

### `internal/ytdlp/`

Adapter for the yt-dlp CLI. Executes yt-dlp as a subprocess, parses
JSON output into `PlaylistEntry` structs. Handles YouTube Music playlists
specially (hybrid flat + full metadata fetch).

Key files: `ytdlp.go` (Client interface, CommandClient implementation).

### `internal/deemix/`

Adapter for the Deemix HTTP API. Authenticates with a Deezer ARL token
via cookie jar. Searches tracks and queues downloads.

Key files: `deemix.go` (Client interface, HTTPClient implementation).

### `internal/navidrome/`

Adapter for the Subsonic REST API. Checks whether a track already exists
in the user's library. Supports three match modes: substring, exact,
and fuzzy (Levenshtein ≥ 80%).

Key files: `navidrome.go` (Client interface, HTTPClient implementation).

### `internal/parser/`

Stateless title parser. Extracts artist and song from YouTube video
titles by trying delimiter patterns, quoted patterns, and "by" patterns.
Strips common noise markers (`[Official Video]`, `(Lyrics)`, etc.).

Key files: `parser.go`.

### `static/`

Single-page application. One HTML file, one JS file, one CSS file.
No build step, no framework. The JS polls `/api/session/{id}` to
update the UI during analysis and download.

Key files: `app.js` (all frontend logic), `style.css`, `index.html`.

## Invariants

**Dependency direction is strictly layered.** `internal/ytdlp`,
`internal/deemix`, `internal/navidrome`, and `internal/parser` have zero
internal imports. `internal/sync` imports all four. `main.go` imports
everything. No lateral imports between adapter packages.

**External services are behind interfaces.** `ytdlp.Client`,
`deemix.Client`, and `navidrome.Client` are interfaces consumed by
`sync.Pipeline`. Tests use mock implementations.

**Session state is mutex-protected.** The `Pipeline.mu` RWMutex guards
all reads and writes to sessions and controls maps. Goroutines spawned
by Analyze/Download lock before mutating track state.

**Pause/resume uses channels, not polling.** Each session has a
`sessionControl` with buffered pause/resume channels. The `checkpoint()`
function is called at each loop iteration and blocks on resume if paused.

## Cross-Cutting Concerns

**Error handling.** Adapter errors bubble up to the Pipeline, which sets
`session.Status = "error"` and `session.Error`. HTTP handlers translate
Pipeline errors to JSON error responses. Partial failures (e.g. some
tracks not found) don't fail the session.

**Configuration.** All config comes from environment variables, read once
in `main.go`. No config files. Navidrome integration is entirely optional
— absent env vars disable it.

**Testing.** Each adapter package has unit tests with mock HTTP servers.
`sync` tests use mock client implementations. Run with `make test`
(includes race detector).

**Concurrency.** One goroutine per active session phase. No global worker
pool. Sequential processing within a session with configurable delays
between API calls (200ms search, 100ms queue, 100ms check).

## A Typical Change

**Adding a new external service check** (e.g. checking Spotify before
downloading):

1. Create `internal/spotify/` with a `Client` interface and
   `HTTPClient` implementation — follow `internal/navidrome/` as a
   template
2. Add the client as an optional field on `sync.Pipeline`
3. Insert the check step in `sync.go` `Analyze()`, between the Deezer
   search loop and the Navidrome check loop
4. Wire the client in `main.go` based on new environment variables
5. Add a test in `internal/sync/sync_test.go` using a mock client
