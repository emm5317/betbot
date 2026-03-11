package injuries

import (
	"context"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	defaultRotowireBaseURL = "https://www.rotowire.com"
	defaultRotowireTimeout = 10 * time.Second
)

type RotowireProvider struct {
	baseURL    string
	httpClient *http.Client
}

func NewRotowireProvider(baseURL string, timeout time.Duration) *RotowireProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultRotowireBaseURL
	}
	if timeout <= 0 {
		timeout = defaultRotowireTimeout
	}
	return &RotowireProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (p *RotowireProvider) Fetch(ctx context.Context, req Request) (Snapshot, error) {
	normalizedReq, err := NormalizeRequest(req)
	if err != nil {
		return Snapshot{}, err
	}
	if normalizedReq.Sport != defaultInjurySport {
		return Snapshot{}, fmt.Errorf("rotowire provider only supports %s injuries", defaultInjurySport)
	}

	endpoint, err := p.endpointURL("football", "tables", "injury-report.php")
	if err != nil {
		return Snapshot{}, fmt.Errorf("build rotowire endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("team", "ALL")
	query.Set("pos", "ALL")
	endpoint.RawQuery = query.Encode()

	payload, err := p.getJSON(ctx, endpoint.String())
	if err != nil {
		return Snapshot{}, fmt.Errorf("fetch rotowire injuries: %w", err)
	}

	records := make([]Record, 0, len(payload))
	for _, row := range payload {
		raw, err := json.Marshal(row)
		if err != nil {
			return Snapshot{}, fmt.Errorf("marshal rotowire row %s: %w", row.ID, err)
		}
		playerName := normalizeLabel(row.Player)
		if playerName == "" {
			playerName = normalizeLabel(strings.TrimSpace(row.FirstName + " " + row.LastName))
		}
		records = append(records, Record{
			ExternalID:      row.ID,
			PlayerName:      playerName,
			TeamExternalID:  row.Team,
			Position:        row.Position,
			Injury:          row.Injury,
			Status:          row.Status,
			EstimatedReturn: normalizeRotowireReturnDate(row.ReturnDate),
			PlayerURL:       p.playerURL(row.URL),
			RawJSON:         raw,
		})
	}

	return Snapshot{
		Source:     defaultInjurySource,
		Sport:      normalizedReq.Sport,
		ReportDate: normalizedReq.ReportDate,
		Records:    records,
	}, nil
}

func (p *RotowireProvider) endpointURL(parts ...string) (*url.URL, error) {
	endpoint, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, err
	}
	endpoint.Path = path.Join(append([]string{endpoint.Path}, parts...)...)
	return endpoint, nil
}

func (p *RotowireProvider) playerURL(relative string) string {
	trimmed := strings.TrimSpace(relative)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	return strings.TrimRight(p.baseURL, "/") + "/" + strings.TrimLeft(trimmed, "/")
}

func (p *RotowireProvider) getJSON(ctx context.Context, endpoint string) ([]rotowireInjuryRow, error) {
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

	var payload []rotowireInjuryRow
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return payload, nil
}

func normalizeRotowireReturnDate(value string) *string {
	cleaned := normalizeLabel(stdhtml.UnescapeString(stripHTMLTags(value)))
	if cleaned == "" {
		return nil
	}
	if strings.EqualFold(cleaned, "Subscribers Only") {
		return nil
	}
	return &cleaned
}

func stripHTMLTags(value string) string {
	var builder strings.Builder
	inTag := false
	for _, r := range value {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				builder.WriteRune(r)
			}
		}
	}
	return builder.String()
}

type rotowireInjuryRow struct {
	ID         string `json:"ID"`
	FirstName  string `json:"firstname"`
	LastName   string `json:"lastname"`
	Player     string `json:"player"`
	URL        string `json:"URL"`
	Team       string `json:"team"`
	Position   string `json:"position"`
	Injury     string `json:"injury"`
	Status     string `json:"status"`
	ReturnDate string `json:"rDate"`
}
