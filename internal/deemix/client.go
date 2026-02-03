package deemix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
)

// Client defines the interface for interacting with Deemix.
type Client interface {
	Login(ctx context.Context) error
	Search(ctx context.Context, query string) ([]SearchResult, error)
	AddToQueue(ctx context.Context, deezerURL string, bitrate int) error
}

// HTTPClient implements Client using Deemix's HTTP API.
type HTTPClient struct {
	BaseURL    string
	ARL        string
	HTTPClient *http.Client
}

// NewClient creates a new Deemix HTTPClient.
func NewClient(baseURL, arl string) *HTTPClient {
	jar, _ := cookiejar.New(nil)
	return &HTTPClient{
		BaseURL:    baseURL,
		ARL:        arl,
		HTTPClient: &http.Client{Jar: jar},
	}
}

// Login authenticates with the Deemix instance using the ARL token.
func (c *HTTPClient) Login(ctx context.Context) error {
	log.Printf("[deemix] logging in to %s", c.BaseURL)
	body, _ := json.Marshal(map[string]string{"arl": c.ARL})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/loginArl", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		log.Printf("[deemix] login request failed: %v", err)
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("[deemix] login failed (status %d): %s", resp.StatusCode, string(respBody))
		return fmt.Errorf("login failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Status int `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding login response: %w", err)
	}
	if result.Status == 0 {
		log.Printf("[deemix] login failed: invalid ARL token")
		return fmt.Errorf("login failed: invalid ARL token")
	}

	log.Printf("[deemix] login successful")
	return nil
}

// Search queries Deemix for tracks matching the given query string.
func (c *HTTPClient) Search(ctx context.Context, query string) ([]SearchResult, error) {
	log.Printf("[deemix] searching: %s", query)
	endpoint := c.BaseURL + "/api/search?" + url.Values{
		"term": {query},
		"type": {"track"},
		"nb":   {"5"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating search request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		log.Printf("[deemix] search request failed: %v", err)
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[deemix] search failed with status %d for query: %s", resp.StatusCode, query)
		return nil, fmt.Errorf("search failed (status %d)", resp.StatusCode)
	}

	var apiResp struct {
		Data []struct {
			ID     int64  `json:"id"`
			Title  string `json:"title"`
			Artist struct {
				Name string `json:"name"`
			} `json:"artist"`
			Album struct {
				Title string `json:"title"`
			} `json:"album"`
			Duration int    `json:"duration"`
			Link     string `json:"link"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decoding search response: %w", err)
	}

	results := make([]SearchResult, len(apiResp.Data))
	for i, d := range apiResp.Data {
		link := d.Link
		if link == "" {
			link = "https://www.deezer.com/track/" + strconv.FormatInt(d.ID, 10)
		}
		results[i] = SearchResult{
			ID:       d.ID,
			Title:    d.Title,
			Artist:   d.Artist.Name,
			Album:    d.Album.Title,
			Duration: d.Duration,
			Link:     link,
		}
	}

	log.Printf("[deemix] search found %d results for: %s", len(results), query)
	return results, nil
}

// AddToQueue adds a track to the Deemix download queue.
func (c *HTTPClient) AddToQueue(ctx context.Context, deezerURL string, bitrate int) error {
	log.Printf("[deemix] adding to queue: %s (bitrate: %d)", deezerURL, bitrate)
	body, _ := json.Marshal(map[string]interface{}{
		"url":     deezerURL,
		"bitrate": bitrate,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/addToQueue", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating queue request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		log.Printf("[deemix] queue request failed: %v", err)
		return fmt.Errorf("queue request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("[deemix] queue failed (status %d) for %s: %s", resp.StatusCode, deezerURL, string(respBody))
		return fmt.Errorf("queue failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	log.Printf("[deemix] queued successfully: %s", deezerURL)
	return nil
}
