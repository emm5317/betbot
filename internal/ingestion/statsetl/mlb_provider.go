package statsetl

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
	"time"
)

const (
	defaultMLBStatsAPIBaseURL  = "https://statsapi.mlb.com/api/v1"
	defaultMLBStatsAPITimeout  = 10 * time.Second
	defaultMLBProviderSource   = "mlb-stats-api"
	mlbStatsAPISportID         = "1"
	mlbPitcherPoolAll          = "ALL"
	mlbPitcherQueryResultLimit = "1000"
)

type MLBStatsAPIProvider struct {
	baseURL    string
	httpClient *http.Client
}

func NewMLBStatsAPIProvider(baseURL string, timeout time.Duration) *MLBStatsAPIProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultMLBStatsAPIBaseURL
	}
	if timeout <= 0 {
		timeout = defaultMLBStatsAPITimeout
	}
	return &MLBStatsAPIProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (p *MLBStatsAPIProvider) Fetch(ctx context.Context, req MLBRequest) (MLBSnapshot, error) {
	normalizedReq, err := NormalizeMLBRequest(req)
	if err != nil {
		return MLBSnapshot{}, err
	}

	teamStats, err := p.fetchTeamStats(ctx, normalizedReq)
	if err != nil {
		return MLBSnapshot{}, err
	}

	pitcherStats, err := p.fetchPitcherStats(ctx, normalizedReq)
	if err != nil {
		return MLBSnapshot{}, err
	}

	return MLBSnapshot{
		Source:     defaultMLBProviderSource,
		Season:     normalizedReq.Season,
		SeasonType: normalizedReq.SeasonType,
		StatDate:   normalizedReq.StatDate,
		Teams:      teamStats,
		Pitchers:   pitcherStats,
	}, nil
}

func (p *MLBStatsAPIProvider) fetchTeamStats(ctx context.Context, req MLBRequest) ([]MLBTeamStat, error) {
	endpoint, err := p.endpointURL("teams", "stats")
	if err != nil {
		return nil, fmt.Errorf("build mlb team stats endpoint: %w", err)
	}

	query := endpoint.Query()
	query.Set("stats", "season")
	query.Set("group", "hitting,pitching")
	query.Set("season", strconv.Itoa(int(req.Season)))
	query.Set("gameType", mlbSeasonTypeCode(req.SeasonType))
	query.Set("sportIds", mlbStatsAPISportID)
	endpoint.RawQuery = query.Encode()

	var payload mlbTeamStatsResponse
	if err := p.getJSON(ctx, endpoint.String(), &payload); err != nil {
		return nil, fmt.Errorf("fetch mlb team stats: %w", err)
	}

	teamsByID := make(map[int]*MLBTeamStat, 30)
	for _, statsGroup := range payload.Stats {
		group := normalizeSlug(statsGroup.Group.DisplayName)
		for _, split := range statsGroup.Splits {
			teamID := split.Team.ID
			if teamID == 0 {
				continue
			}

			team := teamsByID[teamID]
			if team == nil {
				team = &MLBTeamStat{
					ExternalID: strconv.Itoa(teamID),
					TeamName:   split.Team.Name,
				}
				teamsByID[teamID] = team
			}
			if strings.TrimSpace(team.TeamName) == "" {
				team.TeamName = split.Team.Name
			}

			switch group {
			case "hitting":
				team.GamesPlayed = split.Stat.GamesPlayed
				team.RunsScored = split.Stat.Runs
				team.BattingOPS = parseOptionalFloat(split.Stat.OPS)
			case "pitching":
				if team.GamesPlayed == 0 {
					team.GamesPlayed = split.Stat.GamesPlayed
				}
				team.Wins = split.Stat.Wins
				team.Losses = split.Stat.Losses
				team.RunsAllowed = split.Stat.Runs
				team.TeamERA = parseOptionalFloat(split.Stat.ERA)
			}
		}
	}

	teams := make([]MLBTeamStat, 0, len(teamsByID))
	for _, team := range teamsByID {
		teams = append(teams, *team)
	}
	return teams, nil
}

func (p *MLBStatsAPIProvider) fetchPitcherStats(ctx context.Context, req MLBRequest) ([]MLBPitcherStat, error) {
	endpoint, err := p.endpointURL("stats")
	if err != nil {
		return nil, fmt.Errorf("build mlb pitcher stats endpoint: %w", err)
	}

	query := endpoint.Query()
	query.Set("stats", "season")
	query.Set("group", "pitching")
	query.Set("playerPool", mlbPitcherPoolAll)
	query.Set("limit", mlbPitcherQueryResultLimit)
	query.Set("season", strconv.Itoa(int(req.Season)))
	query.Set("gameType", mlbSeasonTypeCode(req.SeasonType))
	query.Set("sportIds", mlbStatsAPISportID)
	endpoint.RawQuery = query.Encode()

	var payload mlbPitcherStatsResponse
	if err := p.getJSON(ctx, endpoint.String(), &payload); err != nil {
		return nil, fmt.Errorf("fetch mlb pitcher stats: %w", err)
	}

	pitchers := make([]MLBPitcherStat, 0)
	for _, statsGroup := range payload.Stats {
		for _, split := range statsGroup.Splits {
			if split.Player.ID == 0 || split.Team.ID == 0 || split.Stat.GamesStarted <= 0 {
				continue
			}

			pitchers = append(pitchers, MLBPitcherStat{
				ExternalID:     strconv.Itoa(split.Player.ID),
				PlayerName:     split.Player.FullName,
				TeamExternalID: strconv.Itoa(split.Team.ID),
				TeamName:       split.Team.Name,
				GamesStarted:   split.Stat.GamesStarted,
				InningsPitched: parseOptionalFloat(split.Stat.InningsPitched),
				Era:            parseOptionalFloat(split.Stat.ERA),
				Fip:            nil,
				Whip:           parseOptionalFloat(split.Stat.WHIP),
				StrikeoutRate:  ratioFloat(split.Stat.StrikeOuts, split.Stat.BattersFaced),
				WalkRate:       ratioFloat(split.Stat.BaseOnBalls, split.Stat.BattersFaced),
			})
		}
	}

	return pitchers, nil
}

func (p *MLBStatsAPIProvider) endpointURL(parts ...string) (*url.URL, error) {
	endpoint, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, err
	}
	endpoint.Path = path.Join(append([]string{endpoint.Path}, parts...)...)
	return endpoint, nil
}

func (p *MLBStatsAPIProvider) getJSON(ctx context.Context, endpoint string, dest any) error {
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

func mlbSeasonTypeCode(seasonType string) string {
	switch normalizeSlug(seasonType) {
	case "", defaultMLBSeasonType:
		return "R"
	case "spring", "spring-training":
		return "S"
	case "postseason", "playoffs":
		return "P"
	default:
		return "R"
	}
}

func parseOptionalFloat(value string) *float64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func ratioFloat(numerator int32, denominator int32) *float64 {
	if denominator <= 0 {
		return nil
	}
	value := float64(numerator) / float64(denominator)
	return &value
}

type mlbTeamStatsResponse struct {
	Stats []struct {
		Group struct {
			DisplayName string `json:"displayName"`
		} `json:"group"`
		Splits []struct {
			Team struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"team"`
			Stat struct {
				GamesPlayed int32  `json:"gamesPlayed"`
				Wins        int32  `json:"wins"`
				Losses      int32  `json:"losses"`
				Runs        int32  `json:"runs"`
				OPS         string `json:"ops"`
				ERA         string `json:"era"`
			} `json:"stat"`
		} `json:"splits"`
	} `json:"stats"`
}

type mlbPitcherStatsResponse struct {
	Stats []struct {
		Splits []struct {
			Team struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"team"`
			Player struct {
				ID       int    `json:"id"`
				FullName string `json:"fullName"`
			} `json:"player"`
			Stat struct {
				GamesStarted   int32  `json:"gamesStarted"`
				StrikeOuts     int32  `json:"strikeOuts"`
				BaseOnBalls    int32  `json:"baseOnBalls"`
				BattersFaced   int32  `json:"battersFaced"`
				InningsPitched string `json:"inningsPitched"`
				ERA            string `json:"era"`
				WHIP           string `json:"whip"`
			} `json:"stat"`
		} `json:"splits"`
	} `json:"stats"`
}
