package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"betbot/internal/config"
	"betbot/internal/ingestion/oddspoller"
	"betbot/internal/logging"
	"betbot/internal/server"
	"betbot/internal/store"
	"betbot/internal/worker"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOddsSnapshotInsertDedupIntegration(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	queries := store.New(pool)
	game, err := queries.UpsertGame(ctx, store.UpsertGameParams{
		Source:       "the-odds-api",
		ExternalID:   "game-1",
		Sport:        "NBA",
		HomeTeam:     "Boston Celtics",
		AwayTeam:     "Chicago Bulls",
		CommenceTime: store.Timestamptz(time.Date(2026, time.March, 11, 23, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("upsert game: %v", err)
	}

	lookup := store.GetLatestSnapshotHashParams{
		GameID:      game.ID,
		Source:      "the-odds-api",
		BookKey:     "draftkings",
		MarketKey:   "h2h",
		OutcomeName: "Boston Celtics",
		OutcomeSide: "home",
	}
	if _, err := queries.GetLatestSnapshotHash(ctx, lookup); !store.IsNoRows(err) {
		t.Fatalf("expected no rows before first insert, got %v", err)
	}

	snapshot := oddspoller.CanonicalOddsSnapshot{
		Source:             "the-odds-api",
		GameExternalID:     "game-1",
		BookKey:            "draftkings",
		BookName:           "DraftKings",
		MarketKey:          "h2h",
		MarketName:         "Moneyline",
		OutcomeName:        "Boston Celtics",
		OutcomeSide:        "home",
		PriceAmerican:      -110,
		ImpliedProbability: 0.5238,
		SnapshotHash:       "hash-1",
		CapturedAt:         time.Date(2026, time.March, 11, 18, 30, 0, 0, time.UTC),
		RawJSON:            json.RawMessage(`{"game_id":"game-1","price":-110}`),
	}

	inserted, err := insertSnapshotIfChanged(ctx, queries, game.ID, lookup, snapshot)
	if err != nil {
		t.Fatalf("insert first snapshot: %v", err)
	}
	if !inserted {
		t.Fatal("expected first snapshot to be inserted")
	}

	count, err := queries.CountOddsHistoryRows(ctx)
	if err != nil {
		t.Fatalf("count odds history after first insert: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 odds row after first insert, got %d", count)
	}

	inserted, err = insertSnapshotIfChanged(ctx, queries, game.ID, lookup, snapshot)
	if err != nil {
		t.Fatalf("insert duplicate snapshot: %v", err)
	}
	if inserted {
		t.Fatal("expected duplicate snapshot to be skipped")
	}

	count, err = queries.CountOddsHistoryRows(ctx)
	if err != nil {
		t.Fatalf("count odds history after duplicate: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected dedup to keep odds_history at 1 row, got %d", count)
	}

	snapshot.PriceAmerican = -125
	snapshot.ImpliedProbability = 0.5556
	snapshot.SnapshotHash = "hash-2"
	snapshot.CapturedAt = snapshot.CapturedAt.Add(2 * time.Minute)
	snapshot.RawJSON = json.RawMessage(`{"game_id":"game-1","price":-125}`)

	inserted, err = insertSnapshotIfChanged(ctx, queries, game.ID, lookup, snapshot)
	if err != nil {
		t.Fatalf("insert changed snapshot: %v", err)
	}
	if !inserted {
		t.Fatal("expected changed snapshot to be inserted")
	}

	count, err = queries.CountOddsHistoryRows(ctx)
	if err != nil {
		t.Fatalf("count odds history after second insert: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 odds rows after changed snapshot, got %d", count)
	}

	latestHash, err := queries.GetLatestSnapshotHash(ctx, lookup)
	if err != nil {
		t.Fatalf("lookup latest snapshot hash: %v", err)
	}
	if latestHash != "hash-2" {
		t.Fatalf("expected latest hash hash-2, got %q", latestHash)
	}

	rows, err := queries.ListLatestOdds(ctx, store.ListLatestOddsParams{RowLimit: 10})
	if err != nil {
		t.Fatalf("list latest odds: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 latest odds row, got %d", len(rows))
	}
	if rows[0].PriceAmerican != -125 {
		t.Fatalf("expected latest price -125, got %d", rows[0].PriceAmerican)
	}
	if !rows[0].CapturedAt.Valid || !rows[0].CapturedAt.Time.UTC().Equal(snapshot.CapturedAt) {
		t.Fatalf("expected latest captured_at %s, got %+v", snapshot.CapturedAt.Format(time.RFC3339), rows[0].CapturedAt)
	}

	nba := "NBA"
	nbaRows, err := queries.ListLatestOdds(ctx, store.ListLatestOddsParams{Sport: &nba, RowLimit: 10})
	if err != nil {
		t.Fatalf("list latest odds by sport: %v", err)
	}
	if len(nbaRows) != 1 {
		t.Fatalf("expected 1 NBA odds row, got %d", len(nbaRows))
	}

	nhl := "NHL"
	nhlRows, err := queries.ListLatestOdds(ctx, store.ListLatestOddsParams{Sport: &nhl, RowLimit: 10})
	if err != nil {
		t.Fatalf("list latest odds by non-matching sport: %v", err)
	}
	if len(nhlRows) != 0 {
		t.Fatalf("expected 0 NHL odds rows, got %d", len(nhlRows))
	}
}

func TestPostgres17BootSmoke(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	var version int
	if err := pool.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&version); err != nil {
		t.Fatalf("query postgres version: %v", err)
	}
	if version < 170000 || version >= 180000 {
		t.Fatalf("expected PostgreSQL 17.x for smoke test, got version_num=%d", version)
	}

	cfg := testConfig(dbURL)
	logger := logging.New(cfg.Env)

	srv, err := server.New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("server boot: %v", err)
	}
	defer srv.Close()

	wrk, err := worker.New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("worker boot: %v", err)
	}
	defer wrk.Close()

	summary, err := store.New(pool).GetDashboardSummary(ctx)
	if err != nil {
		t.Fatalf("dashboard summary query: %v", err)
	}
	if summary.GamesCount != 0 || summary.SnapshotsCount != 0 || summary.LastSnapshotAt.Valid {
		t.Fatalf("expected empty dashboard summary on fresh database, got %+v", summary)
	}
}

func insertSnapshotIfChanged(ctx context.Context, queries *store.Queries, gameID int64, lookup store.GetLatestSnapshotHashParams, snapshot oddspoller.CanonicalOddsSnapshot) (bool, error) {
	lookup.GameID = gameID
	previousHash, err := queries.GetLatestSnapshotHash(ctx, lookup)
	if err != nil && !store.IsNoRows(err) {
		return false, err
	}
	if err == nil && !oddspoller.ShouldInsertSnapshot(previousHash, snapshot) {
		return false, nil
	}

	_, err = queries.InsertOddsSnapshot(ctx, store.InsertOddsSnapshotParams{
		GameID:             gameID,
		Source:             snapshot.Source,
		BookKey:            snapshot.BookKey,
		BookName:           snapshot.BookName,
		MarketKey:          snapshot.MarketKey,
		MarketName:         snapshot.MarketName,
		OutcomeName:        snapshot.OutcomeName,
		OutcomeSide:        snapshot.OutcomeSide,
		PriceAmerican:      int32(snapshot.PriceAmerican),
		Point:              snapshot.Point,
		ImpliedProbability: snapshot.ImpliedProbability,
		SnapshotHash:       snapshot.SnapshotHash,
		RawJson:            snapshot.RawJSON,
		CapturedAt:         store.Timestamptz(snapshot.CapturedAt),
	})
	return err == nil, err
}

func provisionTestDatabase(t *testing.T) (string, func()) {
	t.Helper()

	baseURL := strings.TrimSpace(os.Getenv("BETBOT_TEST_DATABASE_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("BETBOT_DATABASE_URL"))
	}
	if baseURL == "" {
		t.Skip("set BETBOT_TEST_DATABASE_URL to run Postgres-backed integration tests")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}

	adminURL := *parsed
	adminURL.Path = "/postgres"
	ctx := context.Background()
	adminConn, err := pgx.Connect(ctx, adminURL.String())
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}

	dbName := fmt.Sprintf("betbot_test_%d", time.Now().UnixNano())
	if _, err := adminConn.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName)); err != nil {
		_ = adminConn.Close(ctx)
		t.Fatalf("create test database: %v", err)
	}

	testURL := *parsed
	testURL.Path = "/" + dbName
	applyMigrations(t, testURL.String())

	cleanup := func() {
		ctx := context.Background()
		_, _ = adminConn.Exec(ctx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()", dbName)
		_, _ = adminConn.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		_ = adminConn.Close(ctx)
	}

	return testURL.String(), cleanup
}

func applyMigrations(t *testing.T, databaseURL string) {
	t.Helper()

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer func() {
		_ = conn.Close(ctx)
	}()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		files = append(files, filepath.Join(migrationsDir, entry.Name()))
	}
	sort.Strings(files)

	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", filepath.Base(path), err)
		}
		if strings.TrimSpace(string(body)) == "" {
			continue
		}
		if _, err := conn.Exec(ctx, string(body)); err != nil {
			t.Fatalf("apply migration %s: %v", filepath.Base(path), err)
		}
	}
}

func openPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()

	pool, err := store.NewPool(context.Background(), testConfig(databaseURL))
	if err != nil {
		t.Fatalf("open pgx pool: %v", err)
	}
	return pool
}

func testConfig(databaseURL string) config.Config {
	return config.Config{
		Env:                 "test",
		HTTPAddr:            "127.0.0.1:0",
		DatabaseURL:         databaseURL,
		DBConnectTimeout:    5 * time.Second,
		DBMaxConns:          4,
		DBMinConns:          1,
		DBMaxConnLifetime:   5 * time.Minute,
		DBMaxConnIdleTime:   1 * time.Minute,
		DBHealthCheckPeriod: 30 * time.Second,
		OddsAPIBaseURL:      "https://api.the-odds-api.com/v4",
		OddsAPISports:       []string{"basketball_nba"},
		OddsAPIRegions:      "us",
		OddsAPIMarkets:      []string{"h2h"},
		OddsAPIOddsFormat:   "american",
		OddsAPIDateFormat:   "iso",
		OddsAPITimeout:      2 * time.Second,
		OddsAPIPollInterval: 5 * time.Minute,
		OddsAPIRateLimit:    100 * time.Millisecond,
		OddsAPISource:       "the-odds-api",
		RiverSchema:         "public",
		RecentPollWindow:    20 * time.Minute,
	}
}
