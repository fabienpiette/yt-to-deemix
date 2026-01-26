package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"runtime"
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

	pipeline := sync.NewPipeline(ytClient, dxClient, navClient)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sync", handleSync(pipeline))
	mux.HandleFunc("GET /api/sync/{id}", handleGetSession(pipeline))
	mux.HandleFunc("GET /api/channel/playlists", handleChannelPlaylists(ytClient))
	mux.HandleFunc("GET /api/url/info", handleURLInfo(ytClient))
	mux.HandleFunc("GET /api/stats", handleStats)
	mux.HandleFunc("GET /api/navidrome/status", handleNavidromeStatus(navidromeConfigured))
	mux.Handle("GET /", http.FileServer(http.Dir("static")))

	log.Printf("Starting server on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

type syncRequest struct {
	URL            string `json:"url"`
	Bitrate        int    `json:"bitrate"`
	CheckNavidrome bool   `json:"check_navidrome"`
}

type syncResponse struct {
	SessionID string `json:"session_id"`
}

func handleSync(pipeline *sync.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req syncRequest
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

		id := pipeline.Start(context.Background(), req.URL, req.Bitrate, req.CheckNavidrome)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(syncResponse{SessionID: id})
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
	Configured bool `json:"configured"`
}

func handleNavidromeStatus(configured bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(navidromeStatusResponse{Configured: configured})
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
