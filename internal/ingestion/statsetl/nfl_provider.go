package statsetl

import (
	"context"
	"encoding/csv"
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	htmlnode "golang.org/x/net/html"
)

const (
	defaultNFLStatsBaseURL     = "https://github.com/nflverse/nflverse-data"
	defaultNFLStandingsBaseURL = "https://www.nfl.com"
	defaultNFLProviderTimeout  = 15 * time.Second
)

type NFLverseProvider struct {
	statsBaseURL     string
	standingsBaseURL string
	httpClient       *http.Client
}

func NewNFLverseProvider(statsBaseURL string, standingsBaseURL string, timeout time.Duration) *NFLverseProvider {
	if strings.TrimSpace(statsBaseURL) == "" {
		statsBaseURL = defaultNFLStatsBaseURL
	}
	if strings.TrimSpace(standingsBaseURL) == "" {
		standingsBaseURL = defaultNFLStandingsBaseURL
	}
	if timeout <= 0 {
		timeout = defaultNFLProviderTimeout
	}
	return &NFLverseProvider{
		statsBaseURL:     strings.TrimRight(statsBaseURL, "/"),
		standingsBaseURL: strings.TrimRight(standingsBaseURL, "/"),
		httpClient:       &http.Client{Timeout: timeout},
	}
}

func (p *NFLverseProvider) Fetch(ctx context.Context, req NFLRequest) (NFLSnapshot, error) {
	normalizedReq, err := NormalizeNFLRequest(req)
	if err != nil {
		return NFLSnapshot{}, err
	}
	if normalizedReq.SeasonType != defaultNFLSeasonType {
		return NFLSnapshot{}, fmt.Errorf("nfl provider only supports %s season type", defaultNFLSeasonType)
	}

	statsURL, err := p.statsURL(normalizedReq.Season)
	if err != nil {
		return NFLSnapshot{}, fmt.Errorf("build nflverse stats url: %w", err)
	}
	standingsURL, err := p.standingsURL(normalizedReq.Season)
	if err != nil {
		return NFLSnapshot{}, fmt.Errorf("build nfl standings url: %w", err)
	}

	statsBody, err := p.getBody(ctx, statsURL.String())
	if err != nil {
		return NFLSnapshot{}, fmt.Errorf("fetch nflverse team stats: %w", err)
	}
	standingsBody, err := p.getBody(ctx, standingsURL.String())
	if err != nil {
		return NFLSnapshot{}, fmt.Errorf("fetch nfl standings: %w", err)
	}

	statsRows, err := parseNFLverseTeamStatsCSV(statsBody)
	if err != nil {
		return NFLSnapshot{}, fmt.Errorf("parse nflverse team stats: %w", err)
	}
	standingsRows, err := parseNFLStandingsHTML(standingsBody)
	if err != nil {
		return NFLSnapshot{}, fmt.Errorf("parse nfl standings: %w", err)
	}

	teams := make([]NFLTeamStat, 0, len(statsRows))
	for _, statsRow := range statsRows {
		standingsRow, ok := standingsRows[statsRow.ExternalID]
		if !ok {
			return NFLSnapshot{}, fmt.Errorf("missing nfl standings row for team %s", statsRow.ExternalID)
		}
		teams = append(teams, NFLTeamStat{
			ExternalID:           statsRow.ExternalID,
			TeamName:             standingsRow.TeamName,
			GamesPlayed:          statsRow.GamesPlayed,
			Wins:                 standingsRow.Wins,
			Losses:               standingsRow.Losses,
			Ties:                 standingsRow.Ties,
			PointsFor:            standingsRow.PointsFor,
			PointsAgainst:        standingsRow.PointsAgainst,
			OffensiveEPAPerPlay:  statsRow.OffensiveEPAPerPlay,
			DefensiveEPAPerPlay:  nil,
			OffensiveSuccessRate: nil,
			DefensiveSuccessRate: nil,
		})
	}

	return NFLSnapshot{
		Source:     defaultNFLSource,
		Season:     normalizedReq.Season,
		SeasonType: normalizedReq.SeasonType,
		StatDate:   normalizedReq.StatDate,
		Teams:      teams,
	}, nil
}

func (p *NFLverseProvider) statsURL(season int32) (*url.URL, error) {
	endpoint, err := url.Parse(p.statsBaseURL)
	if err != nil {
		return nil, err
	}
	endpoint.Path = path.Join(endpoint.Path, "releases", "download", "stats_team", fmt.Sprintf("stats_team_reg_%d.csv", season))
	return endpoint, nil
}

func (p *NFLverseProvider) standingsURL(season int32) (*url.URL, error) {
	endpoint, err := url.Parse(p.standingsBaseURL)
	if err != nil {
		return nil, err
	}
	endpoint.Path = path.Join(endpoint.Path, "standings", "division", strconv.Itoa(int(season)), "REG")
	return endpoint, nil
}

func (p *NFLverseProvider) getBody(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

type nflverseTeamStatsRow struct {
	ExternalID          string
	GamesPlayed         int32
	OffensiveEPAPerPlay *float64
}

type nflStandingsRow struct {
	TeamName      string
	Wins          int32
	Losses        int32
	Ties          int32
	PointsFor     int32
	PointsAgainst int32
}

func parseNFLverseTeamStatsCSV(body []byte) ([]nflverseTeamStatsRow, error) {
	reader := csv.NewReader(strings.NewReader(string(body)))
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("expected header and data rows")
	}

	headers := make(map[string]int, len(records[0]))
	for idx, header := range records[0] {
		headers[strings.TrimSpace(header)] = idx
	}

	required := []string{"team", "season_type", "games", "attempts", "sacks_suffered", "carries", "passing_epa", "rushing_epa"}
	for _, header := range required {
		if _, ok := headers[header]; !ok {
			return nil, fmt.Errorf("missing csv header %q", header)
		}
	}

	rows := make([]nflverseTeamStatsRow, 0, len(records)-1)
	for _, record := range records[1:] {
		if len(record) == 0 {
			continue
		}
		seasonType := strings.ToUpper(strings.TrimSpace(csvField(record, headers, "season_type")))
		if seasonType != "REG" {
			continue
		}

		externalID := normalizeSlug(csvField(record, headers, "team"))
		if externalID == "" {
			return nil, fmt.Errorf("team code is required")
		}
		gamesPlayed, err := parseCSVInt32(csvField(record, headers, "games"))
		if err != nil {
			return nil, fmt.Errorf("parse games for %s: %w", externalID, err)
		}
		attempts, err := parseCSVFloat64(csvField(record, headers, "attempts"))
		if err != nil {
			return nil, fmt.Errorf("parse attempts for %s: %w", externalID, err)
		}
		sacksSuffered, err := parseCSVFloat64(csvField(record, headers, "sacks_suffered"))
		if err != nil {
			return nil, fmt.Errorf("parse sacks suffered for %s: %w", externalID, err)
		}
		carries, err := parseCSVFloat64(csvField(record, headers, "carries"))
		if err != nil {
			return nil, fmt.Errorf("parse carries for %s: %w", externalID, err)
		}
		passingEPA, err := parseCSVFloat64(csvField(record, headers, "passing_epa"))
		if err != nil {
			return nil, fmt.Errorf("parse passing epa for %s: %w", externalID, err)
		}
		rushingEPA, err := parseCSVFloat64(csvField(record, headers, "rushing_epa"))
		if err != nil {
			return nil, fmt.Errorf("parse rushing epa for %s: %w", externalID, err)
		}

		var offensiveEPAPerPlay *float64
		if plays := attempts + sacksSuffered + carries; plays > 0 {
			value := (passingEPA + rushingEPA) / plays
			offensiveEPAPerPlay = &value
		}

		rows = append(rows, nflverseTeamStatsRow{
			ExternalID:          externalID,
			GamesPlayed:         gamesPlayed,
			OffensiveEPAPerPlay: offensiveEPAPerPlay,
		})
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("no regular-season team rows found")
	}
	return rows, nil
}

func parseNFLStandingsHTML(body []byte) (map[string]nflStandingsRow, error) {
	doc, err := htmlnode.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	rows := make(map[string]nflStandingsRow)
	for _, tr := range findElements(doc, "tr") {
		tds := childElements(tr, "td")
		if len(tds) < 7 {
			continue
		}

		teamName := nodeTextByClass(tds[0], "d3-o-club-fullname")
		externalID := logoCodeFromNode(tds[0])
		if teamName == "" || externalID == "" {
			continue
		}

		wins, err := parseHTMLInt(nodeText(tds[1]))
		if err != nil {
			return nil, fmt.Errorf("parse wins for %s: %w", externalID, err)
		}
		losses, err := parseHTMLInt(nodeText(tds[2]))
		if err != nil {
			return nil, fmt.Errorf("parse losses for %s: %w", externalID, err)
		}
		ties, err := parseHTMLInt(nodeText(tds[3]))
		if err != nil {
			return nil, fmt.Errorf("parse ties for %s: %w", externalID, err)
		}
		pointsFor, err := parseHTMLInt(nodeText(tds[5]))
		if err != nil {
			return nil, fmt.Errorf("parse points for %s: %w", externalID, err)
		}
		pointsAgainst, err := parseHTMLInt(nodeText(tds[6]))
		if err != nil {
			return nil, fmt.Errorf("parse points against for %s: %w", externalID, err)
		}

		rows[normalizeSlug(externalID)] = nflStandingsRow{
			TeamName:      teamName,
			Wins:          wins,
			Losses:        losses,
			Ties:          ties,
			PointsFor:     pointsFor,
			PointsAgainst: pointsAgainst,
		}
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("no standings rows found")
	}
	return rows, nil
}

func csvField(record []string, headers map[string]int, name string) string {
	idx, ok := headers[name]
	if !ok || idx >= len(record) {
		return ""
	}
	return record[idx]
}

func parseCSVInt32(value string) (int32, error) {
	parsed, err := parseCSVFloat64(value)
	if err != nil {
		return 0, err
	}
	return int32(parsed), nil
}

func parseCSVFloat64(value string) (float64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("value is empty")
	}
	return strconv.ParseFloat(trimmed, 64)
}

func parseHTMLInt(value string) (int32, error) {
	trimmed := strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, err
	}
	return int32(parsed), nil
}

func findElements(root *htmlnode.Node, tag string) []*htmlnode.Node {
	var nodes []*htmlnode.Node
	var walk func(*htmlnode.Node)
	walk = func(node *htmlnode.Node) {
		if node.Type == htmlnode.ElementNode && node.Data == tag {
			nodes = append(nodes, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return nodes
}

func childElements(node *htmlnode.Node, tag string) []*htmlnode.Node {
	children := make([]*htmlnode.Node, 0)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == htmlnode.ElementNode && child.Data == tag {
			children = append(children, child)
		}
	}
	return children
}

func nodeText(node *htmlnode.Node) string {
	var parts []string
	var walk func(*htmlnode.Node)
	walk = func(current *htmlnode.Node) {
		if current.Type == htmlnode.TextNode {
			parts = append(parts, current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return normalizeLabel(stdhtml.UnescapeString(strings.Join(parts, " ")))
}

func nodeTextByClass(node *htmlnode.Node, className string) string {
	var match *htmlnode.Node
	var walk func(*htmlnode.Node)
	walk = func(current *htmlnode.Node) {
		if match != nil {
			return
		}
		if current.Type == htmlnode.ElementNode && hasClass(current, className) {
			match = current
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	if match == nil {
		return ""
	}
	return nodeText(match)
}

func logoCodeFromNode(node *htmlnode.Node) string {
	var code string
	var walk func(*htmlnode.Node)
	walk = func(current *htmlnode.Node) {
		if code != "" {
			return
		}
		if current.Type == htmlnode.ElementNode && current.Data == "img" {
			for _, attr := range current.Attr {
				if attr.Key == "src" {
					trimmed := strings.TrimRight(attr.Val, "/")
					if idx := strings.LastIndex(trimmed, "/"); idx >= 0 && idx < len(trimmed)-1 {
						code = trimmed[idx+1:]
						return
					}
				}
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return code
}

func hasClass(node *htmlnode.Node, className string) bool {
	for _, attr := range node.Attr {
		if attr.Key != "class" {
			continue
		}
		for _, value := range strings.Fields(attr.Val) {
			if value == className {
				return true
			}
		}
	}
	return false
}
