package scores

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultDaysFrom = "3"
)

type Client struct {
	apiKey      string
	baseURL     string
	httpClient  *http.Client
	minInterval time.Duration
	mu          sync.Mutex
	lastCallAt  time.Time
}

type GameScore struct {
	ExternalID string
	HomeTeam   string
	AwayTeam   string
	HomeScore  int
	AwayScore  int
	Completed  bool
}

type apiScoreEvent struct {
	ID        string          `json:"id"`
	SportKey  string          `json:"sport_key"`
	HomeTeam  string          `json:"home_team"`
	AwayTeam  string          `json:"away_team"`
	Completed bool            `json:"completed"`
	Scores    []apiTeamScore  `json:"scores"`
	Raw       json.RawMessage `json:"-"`
}

type apiTeamScore struct {
	Name  string `json:"name"`
	Score string `json:"score"`
}

func NewClient(apiKey string, baseURL string, timeout time.Duration, minInterval time.Duration) *Client {
	return &Client{
		apiKey:      strings.TrimSpace(apiKey),
		baseURL:     strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient:  &http.Client{Timeout: timeout},
		minInterval: minInterval,
	}
}

func (c *Client) FetchSport(ctx context.Context, sportKey string) ([]GameScore, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, fmt.Errorf("BETBOT_ODDS_API_KEY is required")
	}
	if strings.TrimSpace(sportKey) == "" {
		return nil, fmt.Errorf("sport key is required")
	}
	if err := c.waitRateLimit(ctx); err != nil {
		return nil, err
	}

	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse odds api base url: %w", err)
	}
	endpoint.Path = path.Join(endpoint.Path, "sports", sportKey, "scores")

	query := endpoint.Query()
	query.Set("apiKey", c.apiKey)
	query.Set("daysFrom", defaultDaysFrom)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build odds api scores request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch odds api scores response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read odds api scores response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("odds api scores status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var events []apiScoreEvent
	if err := json.Unmarshal(body, &events); err != nil {
		return nil, fmt.Errorf("decode odds api scores response: %w", err)
	}

	results := make([]GameScore, 0, len(events))
	for _, event := range events {
		parsed, ok := toGameScore(event)
		if !ok {
			continue
		}
		results = append(results, parsed)
	}

	return results, nil
}

func toGameScore(event apiScoreEvent) (GameScore, bool) {
	if !event.Completed || len(event.Scores) == 0 {
		return GameScore{}, false
	}
	homeTeam := strings.TrimSpace(event.HomeTeam)
	awayTeam := strings.TrimSpace(event.AwayTeam)
	if homeTeam == "" || awayTeam == "" {
		return GameScore{}, false
	}

	scoreByTeam := make(map[string]int, len(event.Scores))
	for _, row := range event.Scores {
		teamName := strings.TrimSpace(strings.ToLower(row.Name))
		if teamName == "" {
			continue
		}
		score, err := strconv.Atoi(strings.TrimSpace(row.Score))
		if err != nil {
			continue
		}
		scoreByTeam[teamName] = score
	}

	homeScore, okHome := scoreByTeam[strings.ToLower(homeTeam)]
	awayScore, okAway := scoreByTeam[strings.ToLower(awayTeam)]
	if !okHome || !okAway {
		return GameScore{}, false
	}

	return GameScore{
		ExternalID: strings.TrimSpace(event.ID),
		HomeTeam:   homeTeam,
		AwayTeam:   awayTeam,
		HomeScore:  homeScore,
		AwayScore:  awayScore,
		Completed:  true,
	}, true
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
