package statsetl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	defaultNHLAPIBaseURL = "https://api-web.nhle.com/v1"
	defaultNHLAPITimeout = 10 * time.Second
)

type NHLStatsAPIProvider struct {
	baseURL    string
	httpClient *http.Client
}

func NewNHLStatsAPIProvider(baseURL string, timeout time.Duration) *NHLStatsAPIProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultNHLAPIBaseURL
	}
	if timeout <= 0 {
		timeout = defaultNHLAPITimeout
	}
	return &NHLStatsAPIProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (p *NHLStatsAPIProvider) Fetch(ctx context.Context, req NHLRequest) (NHLSnapshot, error) {
	normalizedReq, err := NormalizeNHLRequest(req)
	if err != nil {
		return NHLSnapshot{}, err
	}
	if normalizedReq.SeasonType != defaultNHLSeasonType {
		return NHLSnapshot{}, fmt.Errorf("nhl provider only supports %s season type", defaultNHLSeasonType)
	}

	endpoint, err := p.endpointURL("standings", normalizedReq.StatDate.Format(time.DateOnly))
	if err != nil {
		return NHLSnapshot{}, fmt.Errorf("build nhl standings endpoint: %w", err)
	}

	var payload nhlStandingsResponse
	if err := p.getJSON(ctx, endpoint.String(), &payload); err != nil {
		return NHLSnapshot{}, fmt.Errorf("fetch nhl standings: %w", err)
	}

	teams := make([]NHLTeamStat, 0, len(payload.Standings))
	expectedSeasonID := nhlSeasonID(normalizedReq.Season)
	for _, standing := range payload.Standings {
		if standing.GameTypeID != 2 || standing.SeasonID != expectedSeasonID {
			continue
		}
		teams = append(teams, NHLTeamStat{
			ExternalID:          standing.TeamAbbrev.Default,
			TeamName:            standing.TeamName.Default,
			GamesPlayed:         standing.GamesPlayed,
			Wins:                standing.Wins,
			Losses:              standing.Losses,
			OTLosses:            standing.OTLosses,
			GoalsForPerGame:     nhlOptionalFloat(standing.GoalsForPctg),
			GoalsAgainstPerGame: nhlGoalsAgainstPerGame(standing.GoalAgainst, standing.GamesPlayed),
			ExpectedGoalsShare:  nil,
			SavePercentage:      nil,
		})
	}

	return NHLSnapshot{
		Source:     defaultNHLSource,
		Season:     normalizedReq.Season,
		SeasonType: normalizedReq.SeasonType,
		StatDate:   normalizedReq.StatDate,
		Teams:      teams,
	}, nil
}

func (p *NHLStatsAPIProvider) endpointURL(parts ...string) (*url.URL, error) {
	endpoint, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, err
	}
	endpoint.Path = path.Join(append([]string{endpoint.Path}, parts...)...)
	return endpoint, nil
}

func (p *NHLStatsAPIProvider) getJSON(ctx context.Context, endpoint string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func nhlSeasonID(season int32) int32 {
	startYear := season - 1
	return (startYear * 10000) + season
}

func nhlOptionalFloat(value float64) *float64 {
	return &value
}

func nhlGoalsAgainstPerGame(goalsAgainst int32, gamesPlayed int32) *float64 {
	if gamesPlayed <= 0 {
		return nil
	}
	value := float64(goalsAgainst) / float64(gamesPlayed)
	return &value
}

type nhlStandingsResponse struct {
	Standings []struct {
		GameTypeID   int32   `json:"gameTypeId"`
		SeasonID     int32   `json:"seasonId"`
		GamesPlayed  int32   `json:"gamesPlayed"`
		Wins         int32   `json:"wins"`
		Losses       int32   `json:"losses"`
		OTLosses     int32   `json:"otLosses"`
		GoalAgainst  int32   `json:"goalAgainst"`
		GoalsForPctg float64 `json:"goalsForPctg"`
		TeamName     struct {
			Default string `json:"default"`
		} `json:"teamName"`
		TeamAbbrev struct {
			Default string `json:"default"`
		} `json:"teamAbbrev"`
	} `json:"standings"`
}
