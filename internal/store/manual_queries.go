package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Queries struct {
	db DBTX
}

func New(db DBTX) *Queries {
	return &Queries{db: db}
}

type Game struct {
	ID           int64     `json:"id"`
	Source       string    `json:"source"`
	ExternalID   string    `json:"external_id"`
	Sport        string    `json:"sport"`
	HomeTeam     string    `json:"home_team"`
	AwayTeam     string    `json:"away_team"`
	CommenceTime time.Time `json:"commence_time"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type OddsSnapshot struct {
	ID                 int64           `json:"id"`
	GameID             int64           `json:"game_id"`
	Source             string          `json:"source"`
	BookKey            string          `json:"book_key"`
	BookName           string          `json:"book_name"`
	MarketKey          string          `json:"market_key"`
	MarketName         string          `json:"market_name"`
	OutcomeName        string          `json:"outcome_name"`
	OutcomeSide        string          `json:"outcome_side"`
	PriceAmerican      int32           `json:"price_american"`
	Point              *float64        `json:"point"`
	ImpliedProbability float64         `json:"implied_probability"`
	SnapshotHash       string          `json:"snapshot_hash"`
	RawJSON            json.RawMessage `json:"raw_json"`
	CapturedAt         time.Time       `json:"captured_at"`
	CreatedAt          time.Time       `json:"created_at"`
}

type PollRun struct {
	ID            int64      `json:"id"`
	Source        string     `json:"source"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	Status        string     `json:"status"`
	GamesSeen     int32      `json:"games_seen"`
	SnapshotsSeen int32      `json:"snapshots_seen"`
	InsertsCount  int32      `json:"inserts_count"`
	DedupSkips    int32      `json:"dedup_skips"`
	ErrorText     string     `json:"error_text"`
}

type DashboardSummary struct {
	GamesCount     int64      `json:"games_count"`
	SnapshotsCount int64      `json:"snapshots_count"`
	LastSnapshotAt *time.Time `json:"last_snapshot_at"`
}

type ListLatestOddsRow struct {
	GameID             int64     `json:"game_id"`
	Sport              string    `json:"sport"`
	HomeTeam           string    `json:"home_team"`
	AwayTeam           string    `json:"away_team"`
	CommenceTime       time.Time `json:"commence_time"`
	BookKey            string    `json:"book_key"`
	BookName           string    `json:"book_name"`
	MarketKey          string    `json:"market_key"`
	MarketName         string    `json:"market_name"`
	OutcomeName        string    `json:"outcome_name"`
	OutcomeSide        string    `json:"outcome_side"`
	PriceAmerican      int32     `json:"price_american"`
	Point              *float64  `json:"point"`
	ImpliedProbability float64   `json:"implied_probability"`
	CapturedAt         time.Time `json:"captured_at"`
}

type UpsertGameParams struct {
	Source       string
	ExternalID   string
	Sport        string
	HomeTeam     string
	AwayTeam     string
	CommenceTime time.Time
}

type GetLatestSnapshotHashParams struct {
	GameID      int64
	Source      string
	BookKey     string
	MarketKey   string
	OutcomeName string
	OutcomeSide string
	Point       *float64
}

type InsertOddsSnapshotParams struct {
	GameID             int64
	Source             string
	BookKey            string
	BookName           string
	MarketKey          string
	MarketName         string
	OutcomeName        string
	OutcomeSide        string
	PriceAmerican      int32
	Point              *float64
	ImpliedProbability float64
	SnapshotHash       string
	RawJSON            json.RawMessage
	CapturedAt         time.Time
}

type InsertPollRunParams struct {
	Source    string
	StartedAt time.Time
}

type CompletePollRunParams struct {
	ID            int64
	FinishedAt    *time.Time
	Status        string
	GamesSeen     int32
	SnapshotsSeen int32
	InsertsCount  int32
	DedupSkips    int32
	ErrorText     string
}

type InsertBankrollEntryParams struct {
	EntryType     string
	AmountCents   int64
	Currency      string
	ReferenceType string
	ReferenceID   string
	Metadata      json.RawMessage
}

func (q *Queries) UpsertGame(ctx context.Context, arg UpsertGameParams) (Game, error) {
	const query = `
INSERT INTO games (
    source,
    external_id,
    sport,
    home_team,
    away_team,
    commence_time
) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (source, external_id) DO UPDATE
SET
    sport = EXCLUDED.sport,
    home_team = EXCLUDED.home_team,
    away_team = EXCLUDED.away_team,
    commence_time = EXCLUDED.commence_time,
    updated_at = NOW()
RETURNING id, source, external_id, sport, home_team, away_team, commence_time, created_at, updated_at`
	var row Game
	err := q.db.QueryRow(ctx, query, arg.Source, arg.ExternalID, arg.Sport, arg.HomeTeam, arg.AwayTeam, arg.CommenceTime).Scan(
		&row.ID,
		&row.Source,
		&row.ExternalID,
		&row.Sport,
		&row.HomeTeam,
		&row.AwayTeam,
		&row.CommenceTime,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	return row, err
}

func (q *Queries) GetLatestSnapshotHash(ctx context.Context, arg GetLatestSnapshotHashParams) (string, error) {
	const query = `
SELECT snapshot_hash
FROM odds_history
WHERE game_id = $1
  AND source = $2
  AND book_key = $3
  AND market_key = $4
  AND outcome_name = $5
  AND outcome_side = $6
  AND point IS NOT DISTINCT FROM $7
ORDER BY captured_at DESC
LIMIT 1`
	var hash string
	err := q.db.QueryRow(ctx, query, arg.GameID, arg.Source, arg.BookKey, arg.MarketKey, arg.OutcomeName, arg.OutcomeSide, arg.Point).Scan(&hash)
	return hash, err
}

func (q *Queries) InsertOddsSnapshot(ctx context.Context, arg InsertOddsSnapshotParams) (OddsSnapshot, error) {
	const query = `
INSERT INTO odds_history (
    game_id,
    source,
    book_key,
    book_name,
    market_key,
    market_name,
    outcome_name,
    outcome_side,
    price_american,
    point,
    implied_probability,
    snapshot_hash,
    raw_json,
    captured_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
RETURNING id, game_id, source, book_key, book_name, market_key, market_name, outcome_name, outcome_side, price_american, point, implied_probability, snapshot_hash, raw_json, captured_at, created_at`
	var (
		row   OddsSnapshot
		point pgtype.Float8
	)
	err := q.db.QueryRow(ctx, query,
		arg.GameID,
		arg.Source,
		arg.BookKey,
		arg.BookName,
		arg.MarketKey,
		arg.MarketName,
		arg.OutcomeName,
		arg.OutcomeSide,
		arg.PriceAmerican,
		arg.Point,
		arg.ImpliedProbability,
		arg.SnapshotHash,
		arg.RawJSON,
		arg.CapturedAt,
	).Scan(
		&row.ID,
		&row.GameID,
		&row.Source,
		&row.BookKey,
		&row.BookName,
		&row.MarketKey,
		&row.MarketName,
		&row.OutcomeName,
		&row.OutcomeSide,
		&row.PriceAmerican,
		&point,
		&row.ImpliedProbability,
		&row.SnapshotHash,
		&row.RawJSON,
		&row.CapturedAt,
		&row.CreatedAt,
	)
	if err != nil {
		return OddsSnapshot{}, err
	}
	row.Point = nullableFloat(point)
	return row, nil
}

func (q *Queries) ListLatestOdds(ctx context.Context, limit int32) ([]ListLatestOddsRow, error) {
	const query = `
WITH latest AS (
    SELECT DISTINCT ON (
        oh.game_id,
        oh.book_key,
        oh.market_key,
        oh.outcome_name,
        oh.outcome_side,
        oh.point
    )
        oh.game_id,
        oh.book_key,
        oh.book_name,
        oh.market_key,
        oh.market_name,
        oh.outcome_name,
        oh.outcome_side,
        oh.price_american,
        oh.point,
        oh.implied_probability,
        oh.captured_at
    FROM odds_history AS oh
    ORDER BY
        oh.game_id,
        oh.book_key,
        oh.market_key,
        oh.outcome_name,
        oh.outcome_side,
        oh.point,
        oh.captured_at DESC
)
SELECT
    latest.game_id,
    g.sport,
    g.home_team,
    g.away_team,
    g.commence_time,
    latest.book_key,
    latest.book_name,
    latest.market_key,
    latest.market_name,
    latest.outcome_name,
    latest.outcome_side,
    latest.price_american,
    latest.point,
    latest.implied_probability,
    latest.captured_at
FROM latest
JOIN games AS g ON g.id = latest.game_id
ORDER BY
    g.commence_time ASC,
    latest.book_key ASC,
    latest.market_key ASC,
    latest.outcome_name ASC
LIMIT $1`
	rows, err := q.db.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]ListLatestOddsRow, 0)
	for rows.Next() {
		var (
			row   ListLatestOddsRow
			point pgtype.Float8
		)
		if err := rows.Scan(
			&row.GameID,
			&row.Sport,
			&row.HomeTeam,
			&row.AwayTeam,
			&row.CommenceTime,
			&row.BookKey,
			&row.BookName,
			&row.MarketKey,
			&row.MarketName,
			&row.OutcomeName,
			&row.OutcomeSide,
			&row.PriceAmerican,
			&point,
			&row.ImpliedProbability,
			&row.CapturedAt,
		); err != nil {
			return nil, err
		}
		row.Point = nullableFloat(point)
		result = append(result, row)
	}
	return result, rows.Err()
}

func (q *Queries) CountOddsHistoryRows(ctx context.Context) (int64, error) {
	const query = `SELECT COUNT(*)::BIGINT FROM odds_history`
	var count int64
	err := q.db.QueryRow(ctx, query).Scan(&count)
	return count, err
}

func (q *Queries) InsertPollRun(ctx context.Context, arg InsertPollRunParams) (PollRun, error) {
	const query = `
INSERT INTO poll_runs (source, started_at)
VALUES ($1, $2)
RETURNING id, source, started_at, finished_at, status, games_seen, snapshots_seen, inserts_count, dedup_skips, error_text`
	return q.scanPollRun(q.db.QueryRow(ctx, query, arg.Source, arg.StartedAt))
}

func (q *Queries) CompletePollRun(ctx context.Context, arg CompletePollRunParams) error {
	const query = `
UPDATE poll_runs
SET
    finished_at = $2,
    status = $3,
    games_seen = $4,
    snapshots_seen = $5,
    inserts_count = $6,
    dedup_skips = $7,
    error_text = $8
WHERE id = $1`
	_, err := q.db.Exec(ctx, query, arg.ID, arg.FinishedAt, arg.Status, arg.GamesSeen, arg.SnapshotsSeen, arg.InsertsCount, arg.DedupSkips, arg.ErrorText)
	return err
}

func (q *Queries) GetLatestPollRun(ctx context.Context) (PollRun, error) {
	const query = `
SELECT id, source, started_at, finished_at, status, games_seen, snapshots_seen, inserts_count, dedup_skips, error_text
FROM poll_runs
ORDER BY started_at DESC
LIMIT 1`
	return q.scanPollRun(q.db.QueryRow(ctx, query))
}

func (q *Queries) GetDashboardSummary(ctx context.Context) (DashboardSummary, error) {
	const query = `
SELECT
    (SELECT COUNT(*)::BIGINT FROM games WHERE commence_time >= NOW() - INTERVAL '7 days') AS games_count,
    (SELECT COUNT(*)::BIGINT FROM odds_history) AS snapshots_count,
    (SELECT MAX(captured_at) FROM odds_history) AS last_snapshot_at`
	var (
		summary DashboardSummary
		last    pgtype.Timestamptz
	)
	err := q.db.QueryRow(ctx, query).Scan(&summary.GamesCount, &summary.SnapshotsCount, &last)
	if err != nil {
		return DashboardSummary{}, err
	}
	summary.LastSnapshotAt = nullableTime(last)
	return summary, nil
}

func (q *Queries) InsertBankrollEntry(ctx context.Context, arg InsertBankrollEntryParams) error {
	const query = `
INSERT INTO bankroll_ledger (
    entry_type,
    amount_cents,
    currency,
    reference_type,
    reference_id,
    metadata
) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := q.db.Exec(ctx, query, arg.EntryType, arg.AmountCents, arg.Currency, arg.ReferenceType, arg.ReferenceID, arg.Metadata)
	return err
}

func (q *Queries) GetBankrollBalanceCents(ctx context.Context) (int64, error) {
	const query = `SELECT COALESCE(SUM(amount_cents), 0)::BIGINT FROM bankroll_ledger`
	var balance int64
	err := q.db.QueryRow(ctx, query).Scan(&balance)
	return balance, err
}

func (q *Queries) scanPollRun(row pgx.Row) (PollRun, error) {
	var (
		item     PollRun
		finished pgtype.Timestamptz
	)
	err := row.Scan(
		&item.ID,
		&item.Source,
		&item.StartedAt,
		&finished,
		&item.Status,
		&item.GamesSeen,
		&item.SnapshotsSeen,
		&item.InsertsCount,
		&item.DedupSkips,
		&item.ErrorText,
	)
	if err != nil {
		return PollRun{}, err
	}
	item.FinishedAt = nullableTime(finished)
	return item, nil
}

func nullableFloat(value pgtype.Float8) *float64 {
	if !value.Valid {
		return nil
	}
	out := value.Float64
	return &out
}

func nullableTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	out := value.Time.UTC()
	return &out
}

var ErrNoRows = pgx.ErrNoRows

func IsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
