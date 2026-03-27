package livescores

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const defaultBaseURL = "https://api-web.nhle.com/v1"

// Client fetches live NHL scores from the official NHL API.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	minInterval time.Duration
	mu          sync.Mutex
	lastCallAt  time.Time
}

// NewClient creates a new NHL score API client.
func NewClient(baseURL string, timeout, minInterval time.Duration) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if minInterval <= 0 {
		minInterval = 5 * time.Second
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		minInterval: minInterval,
	}
}

// FetchScores calls GET /v1/score/now and returns the parsed response.
func (c *Client) FetchScores(ctx context.Context) (*APIScoreResponse, error) {
	if err := c.waitRateLimit(ctx); err != nil {
		return nil, err
	}

	url := c.baseURL + "/score/now"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch scores: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NHL API returned %d: %s", resp.StatusCode, truncate(body, 200))
	}

	var result APIScoreResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse scores JSON: %w", err)
	}

	return &result, nil
}

func (c *Client) waitRateLimit(ctx context.Context) error {
	c.mu.Lock()
	elapsed := time.Since(c.lastCallAt)
	wait := c.minInterval - elapsed
	c.lastCallAt = time.Now()
	if wait > 0 {
		c.lastCallAt = c.lastCallAt.Add(wait)
	}
	c.mu.Unlock()

	if wait <= 0 {
		return nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
