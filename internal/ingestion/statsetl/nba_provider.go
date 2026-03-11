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
	defaultNBAStatsAPIBaseURL = "https://stats.nba.com"
	defaultNBAStatsAPITimeout = 10 * time.Second
	defaultNBAProviderSource  = "nba-stats-api"
	nbaLeagueID               = "00"
)

var nbaStatsHeaders = map[string]string{
	"Accept":          "application/json, text/plain, */*",
	"Accept-Language": "en-US,en;q=0.5",
	"Connection":      "keep-alive",
	"Host":            "stats.nba.com",
	"Origin":          "https://www.nba.com",
	"Referer":         "https://www.nba.com/",
	"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:61.0) Gecko/20100101 Firefox/61.0",
}

type NBAStatsAPIProvider struct {
	baseURL    string
	httpClient *http.Client
}

func NewNBAStatsAPIProvider(baseURL string, timeout time.Duration) *NBAStatsAPIProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultNBAStatsAPIBaseURL
	}
	if timeout <= 0 {
		timeout = defaultNBAStatsAPITimeout
	}
	return &NBAStatsAPIProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (p *NBAStatsAPIProvider) Fetch(ctx context.Context, req NBARequest) (NBASnapshot, error) {
	normalizedReq, err := NormalizeNBARequest(req)
	if err != nil {
		return NBASnapshot{}, err
	}

	endpoint, err := p.endpointURL("stats", "teamestimatedmetrics")
	if err != nil {
		return NBASnapshot{}, fmt.Errorf("build nba team stats endpoint: %w", err)
	}

	query := endpoint.Query()
	query.Set("LeagueID", nbaLeagueID)
	query.Set("Season", nbaSeasonString(normalizedReq.Season))
	query.Set("SeasonType", nbaSeasonTypeLabel(normalizedReq.SeasonType))
	endpoint.RawQuery = query.Encode()

	var payload nbaStatsResponse
	if err := p.getJSON(ctx, endpoint.String(), &payload); err != nil {
		return NBASnapshot{}, fmt.Errorf("fetch nba team estimated metrics: %w", err)
	}

	teams, err := mapNBATeamStats(payload)
	if err != nil {
		return NBASnapshot{}, err
	}

	return NBASnapshot{
		Source:     defaultNBAProviderSource,
		Season:     normalizedReq.Season,
		SeasonType: normalizedReq.SeasonType,
		StatDate:   normalizedReq.StatDate,
		Teams:      teams,
	}, nil
}

func (p *NBAStatsAPIProvider) endpointURL(parts ...string) (*url.URL, error) {
	endpoint, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, err
	}
	endpoint.Path = path.Join(append([]string{endpoint.Path}, parts...)...)
	return endpoint, nil
}

func (p *NBAStatsAPIProvider) getJSON(ctx context.Context, endpoint string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	for key, value := range nbaStatsHeaders {
		req.Header.Set(key, value)
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

	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func mapNBATeamStats(payload nbaStatsResponse) ([]NBATeamStat, error) {
	resultSet, ok := payload.primaryResultSet()
	if !ok {
		return nil, fmt.Errorf("nba provider response missing result set")
	}

	columnIndex := make(map[string]int, len(resultSet.Headers))
	for idx, header := range resultSet.Headers {
		columnIndex[header] = idx
	}

	required := []string{"TEAM_ID", "TEAM_NAME", "GP", "W", "L", "E_OFF_RATING", "E_DEF_RATING", "E_NET_RATING", "E_PACE"}
	for _, column := range required {
		if _, ok := columnIndex[column]; !ok {
			return nil, fmt.Errorf("nba provider response missing %s column", column)
		}
	}

	teams := make([]NBATeamStat, 0, len(resultSet.RowSet))
	for _, row := range resultSet.RowSet {
		teamID, err := nbaRowString(row, columnIndex["TEAM_ID"])
		if err != nil {
			return nil, fmt.Errorf("map TEAM_ID: %w", err)
		}
		teamName, err := nbaRowString(row, columnIndex["TEAM_NAME"])
		if err != nil {
			return nil, fmt.Errorf("map TEAM_NAME for %s: %w", teamID, err)
		}
		gamesPlayed, err := nbaRowInt32(row, columnIndex["GP"])
		if err != nil {
			return nil, fmt.Errorf("map GP for %s: %w", teamID, err)
		}
		wins, err := nbaRowInt32(row, columnIndex["W"])
		if err != nil {
			return nil, fmt.Errorf("map W for %s: %w", teamID, err)
		}
		losses, err := nbaRowInt32(row, columnIndex["L"])
		if err != nil {
			return nil, fmt.Errorf("map L for %s: %w", teamID, err)
		}

		teams = append(teams, NBATeamStat{
			ExternalID:      teamID,
			TeamName:        teamName,
			GamesPlayed:     gamesPlayed,
			Wins:            wins,
			Losses:          losses,
			OffensiveRating: nbaRowOptionalFloat64(row, columnIndex["E_OFF_RATING"]),
			DefensiveRating: nbaRowOptionalFloat64(row, columnIndex["E_DEF_RATING"]),
			NetRating:       nbaRowOptionalFloat64(row, columnIndex["E_NET_RATING"]),
			Pace:            nbaRowOptionalFloat64(row, columnIndex["E_PACE"]),
		})
	}

	return teams, nil
}

func nbaSeasonString(season int32) string {
	startYear := int(season) - 1
	endSuffix := int(season) % 100
	return fmt.Sprintf("%04d-%02d", startYear, endSuffix)
}

func nbaSeasonTypeLabel(seasonType string) string {
	switch normalizeSlug(seasonType) {
	case "", defaultNBASeasonType:
		return "Regular Season"
	case "playoffs", "postseason":
		return "Playoffs"
	case "preseason", "pre-season":
		return "Pre Season"
	default:
		return "Regular Season"
	}
}

func nbaRowString(row []any, idx int) (string, error) {
	if idx >= len(row) {
		return "", fmt.Errorf("column %d out of bounds", idx)
	}
	switch value := row[idx].(type) {
	case string:
		return value, nil
	case json.Number:
		return value.String(), nil
	case float64:
		return strconv.FormatInt(int64(value), 10), nil
	default:
		return "", fmt.Errorf("unexpected type %T", row[idx])
	}
}

func nbaRowInt32(row []any, idx int) (int32, error) {
	if idx >= len(row) {
		return 0, fmt.Errorf("column %d out of bounds", idx)
	}
	switch value := row[idx].(type) {
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			floatValue, floatErr := value.Float64()
			if floatErr != nil {
				return 0, err
			}
			return int32(floatValue), nil
		}
		return int32(parsed), nil
	case float64:
		return int32(value), nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
		if err != nil {
			return 0, err
		}
		return int32(parsed), nil
	default:
		return 0, fmt.Errorf("unexpected type %T", row[idx])
	}
}

func nbaRowOptionalFloat64(row []any, idx int) *float64 {
	if idx >= len(row) || row[idx] == nil {
		return nil
	}
	switch value := row[idx].(type) {
	case json.Number:
		parsed, err := value.Float64()
		if err != nil {
			return nil
		}
		return &parsed
	case float64:
		return &value
	case string:
		return parseOptionalFloat(value)
	default:
		return nil
	}
}

type nbaStatsResponse struct {
	ResultSet  nbaResultSet   `json:"resultSet"`
	ResultSets []nbaResultSet `json:"resultSets"`
}

type nbaResultSet struct {
	Name    string   `json:"name"`
	Headers []string `json:"headers"`
	RowSet  [][]any  `json:"rowSet"`
}

func (r nbaStatsResponse) primaryResultSet() (nbaResultSet, bool) {
	if len(r.ResultSet.Headers) > 0 {
		return r.ResultSet, true
	}
	for _, resultSet := range r.ResultSets {
		if len(resultSet.Headers) > 0 {
			return resultSet, true
		}
	}
	return nbaResultSet{}, false
}
