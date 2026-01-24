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

	pipeline := sync.NewPipeline(ytClient, dxClient)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sync", handleSync(pipeline))
	mux.HandleFunc("GET /api/sync/{id}", handleGetSession(pipeline))
	mux.HandleFunc("GET /api/stats", handleStats)
	mux.Handle("GET /", http.FileServer(http.Dir("static")))

	log.Printf("Starting server on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

type syncRequest struct {
	URL     string `json:"url"`
	Bitrate int    `json:"bitrate"`
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

		id := pipeline.Start(context.Background(), req.URL, req.Bitrate)

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

func isValidYouTubeURL(url string) bool {
	return strings.Contains(url, "youtube.com") || strings.Contains(url, "youtu.be")
}
