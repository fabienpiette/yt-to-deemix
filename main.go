package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gndm/ytToDeemix/internal/deemix"
	"github.com/gndm/ytToDeemix/internal/navidrome"
	"github.com/gndm/ytToDeemix/internal/sync"
	"github.com/gndm/ytToDeemix/internal/ytdlp"
)

var startTime = time.Now()

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	deemixURL := os.Getenv("DEEMIX_URL")
	if deemixURL == "" {
		deemixURL = "http://localhost:6595"
	}
	arl := os.Getenv("DEEMIX_ARL")
	if arl == "" {
		log.Fatal("DEEMIX_ARL environment variable is required")
	}

	// Initialize clients.
	ytClient := ytdlp.NewClient()
	dxClient := deemix.NewClient(deemixURL, arl)

	// Login to Deemix.
	ctx := context.Background()
	if err := dxClient.Login(ctx); err != nil {
		log.Printf("WARNING: Deemix login failed: %v", err)
	} else {
		log.Printf("Logged in to Deemix at %s", deemixURL)
	}

	// Optional Navidrome integration.
	var navClient navidrome.Client
	navURL := os.Getenv("NAVIDROME_URL")
	navUser := os.Getenv("NAVIDROME_USER")
	navPass := os.Getenv("NAVIDROME_PASSWORD")
	navMatchMode := os.Getenv("NAVIDROME_MATCH_MODE")
	if navURL != "" && navUser != "" && navPass != "" {
		navClient = &navidrome.HTTPClient{
			BaseURL:   navURL,
			User:      navUser,
			Password:  navPass,
			MatchMode: navMatchMode,
		}
		log.Printf("Navidrome integration enabled at %s (match: %s)", navURL, effectiveMatchMode(navMatchMode))
	}

	navidromeConfigured := navClient != nil
	navidromeSkipDefault := os.Getenv("NAVIDROME_SKIP_DEFAULT") == "true"

	pipeline := sync.NewPipeline(ytClient, dxClient, navClient)

	// Optional confidence threshold.
	if thresholdStr := os.Getenv("CONFIDENCE_THRESHOLD"); thresholdStr != "" {
		if threshold, err := strconv.Atoi(thresholdStr); err == nil {
			pipeline.SetConfidenceThreshold(threshold)
			log.Printf("Confidence threshold set to %d%%", threshold)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/analyze", handleAnalyze(pipeline))
	mux.HandleFunc("GET /api/session/{id}", handleGetSession(pipeline))
	mux.HandleFunc("POST /api/session/{id}/download", handleDownload(pipeline))
	mux.HandleFunc("POST /api/session/{id}/pause", handlePause(pipeline))
	mux.HandleFunc("POST /api/session/{id}/resume", handleResume(pipeline))
	mux.HandleFunc("POST /api/session/{id}/cancel", handleCancel(pipeline))
	mux.HandleFunc("POST /api/session/{id}/track/{index}/select", handleSelectTrack(pipeline))
	mux.HandleFunc("POST /api/session/{id}/track/{index}/search", handleSearchTrack(pipeline))
	mux.HandleFunc("GET /api/channel/playlists", handleChannelPlaylists(ytClient))
	mux.HandleFunc("GET /api/url/info", handleURLInfo(ytClient))
	mux.HandleFunc("GET /api/stats", handleStats)
	mux.HandleFunc("GET /api/navidrome/status", handleNavidromeStatus(navidromeConfigured, navidromeSkipDefault))
	mux.Handle("GET /", staticHandler())

	log.Printf("Starting server on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

type analyzeRequest struct {
	URL            string `json:"url"`
	Bitrate        int    `json:"bitrate"`
	CheckNavidrome bool   `json:"check_navidrome"`
}

type analyzeResponse struct {
	SessionID string `json:"session_id"`
}

func handleAnalyze(pipeline *sync.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req analyzeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.URL == "" {
			http.Error(w, `{"error":"url is required"}`, http.StatusBadRequest)
			return
		}
		if !isValidYouTubeURL(req.URL) {
			http.Error(w, `{"error":"invalid YouTube URL"}`, http.StatusBadRequest)
			return
		}
		if req.Bitrate == 0 {
			req.Bitrate = deemix.Bitrate128
		}

		id := pipeline.Analyze(context.Background(), req.URL, req.Bitrate, req.CheckNavidrome)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(analyzeResponse{SessionID: id})
	}
}

func handleGetSession(pipeline *sync.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		session, ok := pipeline.GetSession(id)
		if !ok {
			http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(session)
	}
}

type statsResponse struct {
	MemoryMB   float64 `json:"memory_mb"`
	Goroutines int     `json:"goroutines"`
	UptimeSec  float64 `json:"uptime_sec"`
}

func handleStats(w http.ResponseWriter, _ *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := statsResponse{
		MemoryMB:   float64(m.Alloc) / 1024 / 1024,
		Goroutines: runtime.NumGoroutine(),
		UptimeSec:  time.Since(startTime).Seconds(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

type navidromeStatusResponse struct {
	Configured  bool `json:"configured"`
	SkipDefault bool `json:"skip_default"`
}

func handleNavidromeStatus(configured, skipDefault bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(navidromeStatusResponse{Configured: configured, SkipDefault: skipDefault})
	}
}

func effectiveMatchMode(mode string) string {
	if mode == "" {
		return navidrome.MatchSubstring
	}
	return mode
}

func isValidYouTubeURL(url string) bool {
	return strings.Contains(url, "youtube.com") || strings.Contains(url, "youtu.be")
}

type channelPlaylistsResponse struct {
	Playlists []playlistInfo `json:"playlists"`
}

type playlistInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

func handleChannelPlaylists(ytClient *ytdlp.CommandClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		url := r.URL.Query().Get("url")
		if url == "" {
			http.Error(w, `{"error":"url query parameter is required"}`, http.StatusBadRequest)
			return
		}
		if !isChannelURL(url) {
			http.Error(w, `{"error":"invalid YouTube channel URL"}`, http.StatusBadRequest)
			return
		}

		playlists, err := ytClient.GetChannelPlaylists(r.Context(), url)
		if err != nil {
			log.Printf("[channel] failed to fetch playlists from %s: %v", url, err)
			http.Error(w, `{"error":"failed to fetch channel playlists"}`, http.StatusInternalServerError)
			return
		}

		log.Printf("[channel] fetched %d playlists from %s", len(playlists), url)
		resp := channelPlaylistsResponse{Playlists: make([]playlistInfo, len(playlists))}
		for i, p := range playlists {
			resp.Playlists[i] = playlistInfo{ID: p.ID, Title: p.Title, URL: p.URL}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

type urlInfoResponse struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

func handleURLInfo(ytClient *ytdlp.CommandClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		url := r.URL.Query().Get("url")
		if url == "" {
			http.Error(w, `{"error":"url query parameter is required"}`, http.StatusBadRequest)
			return
		}
		if !isValidYouTubeURL(url) {
			http.Error(w, `{"error":"invalid YouTube URL"}`, http.StatusBadRequest)
			return
		}

		title, err := ytClient.GetURLInfo(r.Context(), url)
		if err != nil {
			log.Printf("[url-info] failed to fetch info for %s: %v", url, err)
			// Return URL as title fallback instead of error
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(urlInfoResponse{URL: url, Title: ""})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(urlInfoResponse{URL: url, Title: title})
	}
}

func isChannelURL(url string) bool {
	return strings.Contains(url, "youtube.com/@") ||
		strings.Contains(url, "youtube.com/channel/") ||
		strings.Contains(url, "youtube.com/c/") ||
		strings.Contains(url, "youtube.com/user/") ||
		strings.Contains(url, "youtube.com/browse/")
}

func handleDownload(pipeline *sync.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")

		go func() {
			if err := pipeline.Download(context.Background(), sessionID); err != nil {
				// Error is logged in pipeline; client polls for status
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"downloading"}`))
	}
}

type selectRequest struct {
	Selected bool `json:"selected"`
}

type selectResponse struct {
	Selected bool `json:"selected"`
}

func handleSelectTrack(pipeline *sync.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")
		indexStr := r.PathValue("index")
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			http.Error(w, `{"error":"invalid track index"}`, http.StatusBadRequest)
			return
		}

		var req selectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		if err := pipeline.SetTrackSelected(sessionID, index, req.Selected); err != nil {
			switch err {
			case sync.ErrSessionNotFound:
				http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
			case sync.ErrSessionNotReady:
				http.Error(w, `{"error":"session is not ready for modifications"}`, http.StatusBadRequest)
			case sync.ErrTrackNotFound:
				http.Error(w, `{"error":"track not found"}`, http.StatusNotFound)
			default:
				http.Error(w, `{"error":"failed to update track selection"}`, http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(selectResponse{Selected: req.Selected})
	}
}

type searchRequest struct {
	Query string `json:"query"`
}

type searchResponse struct {
	DeezerMatch *struct {
		ID     int64  `json:"id"`
		Title  string `json:"title"`
		Artist string `json:"artist"`
		Link   string `json:"link"`
	} `json:"deezer_match,omitempty"`
	Confidence int `json:"confidence"`
}

func handleSearchTrack(pipeline *sync.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")
		indexStr := r.PathValue("index")
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			http.Error(w, `{"error":"invalid track index"}`, http.StatusBadRequest)
			return
		}

		var req searchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		if req.Query == "" {
			http.Error(w, `{"error":"query is required"}`, http.StatusBadRequest)
			return
		}

		if err := pipeline.SearchTrack(r.Context(), sessionID, index, req.Query); err != nil {
			switch err {
			case sync.ErrSessionNotFound:
				http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
			case sync.ErrSessionNotReady:
				http.Error(w, `{"error":"session is not ready for modifications"}`, http.StatusBadRequest)
			case sync.ErrTrackNotFound:
				http.Error(w, `{"error":"track not found"}`, http.StatusNotFound)
			default:
				http.Error(w, `{"error":"search failed"}`, http.StatusInternalServerError)
			}
			return
		}

		// Get updated session to return the new match
		session, _ := pipeline.GetSession(sessionID)
		track := session.Tracks[index]

		resp := searchResponse{Confidence: track.Confidence}
		if track.DeezerMatch != nil {
			resp.DeezerMatch = &struct {
				ID     int64  `json:"id"`
				Title  string `json:"title"`
				Artist string `json:"artist"`
				Link   string `json:"link"`
			}{
				ID:     track.DeezerMatch.ID,
				Title:  track.DeezerMatch.Title,
				Artist: track.DeezerMatch.Artist,
				Link:   track.DeezerMatch.Link,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func handlePause(pipeline *sync.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")

		if err := pipeline.PauseSession(sessionID); err != nil {
			switch err {
			case sync.ErrSessionNotFound:
				http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
			case sync.ErrSessionPaused:
				http.Error(w, `{"error":"session is already paused"}`, http.StatusBadRequest)
			case sync.ErrSessionNotReady:
				http.Error(w, `{"error":"session is not in an active state"}`, http.StatusBadRequest)
			default:
				http.Error(w, `{"error":"failed to pause session"}`, http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"paused"}`))
	}
}

func handleResume(pipeline *sync.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")

		if err := pipeline.ResumeSession(sessionID); err != nil {
			switch err {
			case sync.ErrSessionNotFound:
				http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
			case sync.ErrSessionNotPaused:
				http.Error(w, `{"error":"session is not paused"}`, http.StatusBadRequest)
			default:
				http.Error(w, `{"error":"failed to resume session"}`, http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"resumed"}`))
	}
}

func handleCancel(pipeline *sync.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")

		if err := pipeline.CancelSession(sessionID); err != nil {
			switch err {
			case sync.ErrSessionNotFound:
				http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
			case sync.ErrSessionCanceled:
				http.Error(w, `{"error":"session is already in terminal state"}`, http.StatusBadRequest)
			default:
				http.Error(w, `{"error":"failed to cancel session"}`, http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"canceled"}`))
	}
}
