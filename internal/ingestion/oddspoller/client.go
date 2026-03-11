package oddspoller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

type Client struct {
	apiKey      string
	baseURL     string
	regions     string
	markets     []string
	oddsFormat  string
	dateFormat  string
	httpClient  *http.Client
	minInterval time.Duration
	mu          sync.Mutex
	lastCallAt  time.Time
}

func NewClient(apiKey string, baseURL string, regions string, markets []string, oddsFormat string, dateFormat string, timeout time.Duration, minInterval time.Duration) *Client {
	return &Client{
		apiKey:      apiKey,
		baseURL:     strings.TrimRight(baseURL, "/"),
		regions:     regions,
		markets:     append([]string(nil), markets...),
		oddsFormat:  oddsFormat,
		dateFormat:  dateFormat,
		httpClient:  &http.Client{Timeout: timeout},
		minInterval: minInterval,
	}
}

func (c *Client) FetchSport(ctx context.Context, sport string) ([]APIGame, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, fmt.Errorf("BETBOT_ODDS_API_KEY is required")
	}
	if err := c.waitRateLimit(ctx); err != nil {
		return nil, err
	}

	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse odds api base url: %w", err)
	}
	endpoint.Path = path.Join(endpoint.Path, "sports", sport, "odds")

	query := endpoint.Query()
	query.Set("apiKey", c.apiKey)
	query.Set("regions", c.regions)
	query.Set("markets", strings.Join(c.markets, ","))
	query.Set("oddsFormat", c.oddsFormat)
	query.Set("dateFormat", c.dateFormat)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build odds api request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch odds api response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read odds api response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("odds api status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var games []APIGame
	if err := json.Unmarshal(body, &games); err != nil {
		return nil, fmt.Errorf("decode odds api response: %w", err)
	}
	for i := range games {
		raw, err := json.Marshal(games[i])
		if err != nil {
			return nil, fmt.Errorf("marshal raw event: %w", err)
		}
		games[i].Raw = raw
	}

	return games, nil
}

func (c *Client) waitRateLimit(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.minInterval <= 0 || c.lastCallAt.IsZero() {
		c.lastCallAt = time.Now()
		return nil
	}

	wait := c.minInterval - time.Since(c.lastCallAt)
	if wait <= 0 {
		c.lastCallAt = time.Now()
		return nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		c.lastCallAt = time.Now()
		return nil
	}
}
