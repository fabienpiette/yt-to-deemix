package main

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/js"
	"github.com/tdewolff/minify/v2/svg"
)

//go:embed static/*
var staticFS embed.FS

// asset holds a minified and gzipped version of a static file.
type asset struct {
	content     []byte // minified content
	gzipped     []byte // gzipped minified content
	contentType string
}

// assetCache stores processed assets, keyed by path.
var assetCache = struct {
	sync.RWMutex
	m map[string]*asset
}{m: make(map[string]*asset)}

// initAssets processes all embedded static files at startup.
func initAssets() {
	m := minify.New()
	m.AddFunc("text/css", css.Minify)
	m.AddFunc("text/javascript", js.Minify)
	m.AddFunc("application/javascript", js.Minify)
	m.AddFunc("image/svg+xml", svg.Minify)

	err := fs.WalkDir(staticFS, "static", func(filePath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		data, err := staticFS.ReadFile(filePath)
		if err != nil {
			return err
		}

		// Determine content type from extension.
		ext := filepath.Ext(filePath)
		contentType := mime.TypeByExtension(ext)
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		// Strip "static/" prefix for serving path.
		servePath := strings.TrimPrefix(filePath, "static/")

		// Minify if applicable.
		minified := data
		mediaType := strings.Split(contentType, ";")[0]
		if _, _, fn := m.Match(mediaType); fn != nil {
			var buf bytes.Buffer
			if err := m.Minify(mediaType, &buf, bytes.NewReader(data)); err != nil {
				log.Printf("[static] failed to minify %s: %v (using original)", servePath, err)
			} else {
				minified = buf.Bytes()
				reduction := 100 - (len(minified)*100)/len(data)
				log.Printf("[static] minified %s: %d -> %d bytes (%d%% reduction)",
					servePath, len(data), len(minified), reduction)
			}
		}

		// Gzip the content.
		var gzBuf bytes.Buffer
		gz, _ := gzip.NewWriterLevel(&gzBuf, gzip.BestCompression)
		gz.Write(minified)
		gz.Close()

		assetCache.Lock()
		assetCache.m[servePath] = &asset{
			content:     minified,
			gzipped:     gzBuf.Bytes(),
			contentType: contentType,
		}
		assetCache.Unlock()

		return nil
	})

	if err != nil {
		log.Printf("[static] warning: failed to process embedded assets: %v", err)
	}

	assetCache.RLock()
	total := len(assetCache.m)
	assetCache.RUnlock()
	log.Printf("[static] initialized %d embedded assets", total)
}

// staticHandler returns an http.Handler for serving static files.
// In development mode (DEV=1), serves files directly from disk.
// In production mode, serves embedded, minified, and gzipped assets.
func staticHandler() http.Handler {
	if os.Getenv("DEV") == "1" {
		log.Println("[static] development mode: serving from disk")
		return http.FileServer(http.Dir("static"))
	}

	// Production: serve from embedded cache.
	initAssets()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean and normalize the path.
		urlPath := path.Clean(r.URL.Path)
		if urlPath == "/" || urlPath == "" {
			urlPath = "index.html"
		} else {
			urlPath = strings.TrimPrefix(urlPath, "/")
		}

		assetCache.RLock()
		a, ok := assetCache.m[urlPath]
		assetCache.RUnlock()

		if !ok {
			// Try index.html for directory paths.
			assetCache.RLock()
			a, ok = assetCache.m["index.html"]
			assetCache.RUnlock()
			if !ok {
				http.NotFound(w, r)
				return
			}
		}

		// Set content type and cache headers.
		w.Header().Set("Content-Type", a.contentType)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Vary", "Accept-Encoding")

		// Check if client accepts gzip.
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && len(a.gzipped) > 0 {
			w.Header().Set("Content-Encoding", "gzip")
			w.Write(a.gzipped)
			return
		}

		w.Write(a.content)
	})
}
