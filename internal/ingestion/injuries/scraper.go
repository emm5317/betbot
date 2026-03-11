package injuries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultInjurySource = "rotowire"
	defaultInjurySport  = "nfl"
)

var ErrProviderUnconfigured = errors.New("injury provider is not configured")

type Provider interface {
	Fetch(ctx context.Context, req Request) (Snapshot, error)
}

type Store interface {
	UpsertPlayerInjuryReport(ctx context.Context, arg store.UpsertPlayerInjuryReportParams) error
}

type Request struct {
	RequestedAt time.Time
	ReportDate  time.Time
	Sport       string
	Source      string
}

type Snapshot struct {
	Source     string
	Sport      string
	ReportDate time.Time
	Records    []Record
}

type Record struct {
	ExternalID      string
	PlayerName      string
	TeamExternalID  string
	Position        string
	Injury          string
	Status          string
	EstimatedReturn *string
	PlayerURL       string
	RawJSON         json.RawMessage
}

type RunMetrics struct {
	RecordRows int
}

type Scraper struct {
	provider Provider
	logger   *slog.Logger
}

type UnconfiguredProvider struct{}

func (UnconfiguredProvider) Fetch(context.Context, Request) (Snapshot, error) {
	return Snapshot{}, ErrProviderUnconfigured
}

func NewScraper(provider Provider, logger *slog.Logger) *Scraper {
	if provider == nil {
		provider = UnconfiguredProvider{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Scraper{provider: provider, logger: logger}
}

func (s *Scraper) Run(ctx context.Context, queries Store, req Request) (RunMetrics, error) {
	if queries == nil {
		return RunMetrics{}, errors.New("injury scraper store is nil")
	}

	normalizedReq, err := NormalizeRequest(req)
	if err != nil {
		return RunMetrics{}, err
	}

	snapshot, err := s.provider.Fetch(ctx, normalizedReq)
	if err != nil {
		return RunMetrics{}, fmt.Errorf("fetch injuries: %w", err)
	}

	normalizedSnapshot, err := normalizeSnapshot(snapshot, normalizedReq)
	if err != nil {
		return RunMetrics{}, err
	}

	metrics := RunMetrics{RecordRows: len(normalizedSnapshot.Records)}
	for _, record := range normalizedSnapshot.Records {
		if err := queries.UpsertPlayerInjuryReport(ctx, store.UpsertPlayerInjuryReportParams{
			Source:          normalizedSnapshot.Source,
			Sport:           normalizedSnapshot.Sport,
			ReportDate:      pgDate(normalizedSnapshot.ReportDate),
			ExternalID:      record.ExternalID,
			PlayerName:      record.PlayerName,
			TeamExternalID:  record.TeamExternalID,
			Position:        record.Position,
			Injury:          record.Injury,
			Status:          record.Status,
			EstimatedReturn: record.EstimatedReturn,
			PlayerUrl:       record.PlayerURL,
			RawJson:         record.RawJSON,
		}); err != nil {
			return RunMetrics{}, fmt.Errorf("upsert injury report %s: %w", record.ExternalID, err)
		}
	}

	s.logger.InfoContext(ctx, "injury ingestion completed",
		slog.Int("record_rows", metrics.RecordRows),
		slog.String("sport", normalizedSnapshot.Sport),
		slog.String("report_date", normalizedSnapshot.ReportDate.Format(time.DateOnly)),
		slog.String("source", normalizedSnapshot.Source),
	)

	return metrics, nil
}

func NormalizeRequest(req Request) (Request, error) {
	reportDate := req.ReportDate
	if reportDate.IsZero() {
		reportDate = req.RequestedAt
	}
	if reportDate.IsZero() {
		return Request{}, errors.New("injury report date is required")
	}

	normalized := Request{
		RequestedAt: req.RequestedAt.UTC(),
		ReportDate:  normalizeDate(reportDate),
		Sport:       normalizeSlug(req.Sport),
		Source:      normalizeSlug(req.Source),
	}
	if normalized.Sport == "" {
		normalized.Sport = defaultInjurySport
	}
	if normalized.Source == "" {
		normalized.Source = defaultInjurySource
	}
	if normalized.RequestedAt.IsZero() {
		normalized.RequestedAt = normalized.ReportDate
	}
	return normalized, nil
}

func normalizeSnapshot(snapshot Snapshot, req Request) (Snapshot, error) {
	normalized := Snapshot{
		Source:     normalizeSlug(snapshot.Source),
		Sport:      normalizeSlug(snapshot.Sport),
		ReportDate: snapshot.ReportDate,
		Records:    make([]Record, 0, len(snapshot.Records)),
	}
	if normalized.Source == "" {
		normalized.Source = req.Source
	}
	if normalized.Sport == "" {
		normalized.Sport = req.Sport
	}
	if normalized.ReportDate.IsZero() {
		normalized.ReportDate = req.ReportDate
	} else {
		normalized.ReportDate = normalizeDate(normalized.ReportDate)
	}
	if normalized.Source == "" {
		normalized.Source = defaultInjurySource
	}
	if normalized.Sport == "" {
		normalized.Sport = defaultInjurySport
	}
	if normalized.ReportDate.IsZero() {
		return Snapshot{}, errors.New("injury report date is required")
	}

	for _, record := range snapshot.Records {
		normalizedRecord, err := normalizeRecord(record)
		if err != nil {
			return Snapshot{}, err
		}
		normalized.Records = append(normalized.Records, normalizedRecord)
	}

	return normalized, nil
}

func normalizeRecord(record Record) (Record, error) {
	normalized := Record{
		ExternalID:      normalizeSlug(record.ExternalID),
		PlayerName:      normalizeLabel(record.PlayerName),
		TeamExternalID:  normalizeSlug(record.TeamExternalID),
		Position:        normalizeLabel(record.Position),
		Injury:          normalizeLabel(record.Injury),
		Status:          normalizeLabel(record.Status),
		EstimatedReturn: normalizeOptionalLabel(record.EstimatedReturn),
		PlayerURL:       strings.TrimSpace(record.PlayerURL),
		RawJSON:         record.RawJSON,
	}
	if normalized.ExternalID == "" {
		return Record{}, errors.New("injury external id is required")
	}
	if normalized.PlayerName == "" {
		return Record{}, fmt.Errorf("injury player name is required for %s", normalized.ExternalID)
	}
	if normalized.TeamExternalID == "" {
		return Record{}, fmt.Errorf("injury team external id is required for %s", normalized.ExternalID)
	}
	if normalized.Position == "" {
		return Record{}, fmt.Errorf("injury position is required for %s", normalized.ExternalID)
	}
	if normalized.Injury == "" {
		return Record{}, fmt.Errorf("injury description is required for %s", normalized.ExternalID)
	}
	if normalized.Status == "" {
		return Record{}, fmt.Errorf("injury status is required for %s", normalized.ExternalID)
	}
	if normalized.PlayerURL == "" {
		return Record{}, fmt.Errorf("injury player url is required for %s", normalized.ExternalID)
	}
	if len(normalized.RawJSON) == 0 {
		return Record{}, fmt.Errorf("injury raw json is required for %s", normalized.ExternalID)
	}
	return normalized, nil
}

func pgDate(value time.Time) pgtype.Date {
	return pgtype.Date{Time: normalizeDate(value), Valid: true}
}

func normalizeOptionalLabel(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := normalizeLabel(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func normalizeDate(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func normalizeSlug(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeLabel(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
