package integration_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSportStatTablesMigration(t *testing.T) {
	dbURL, cleanup := provisionTestDatabase(t)
	defer cleanup()

	ctx := context.Background()
	pool := openPool(t, dbURL)
	defer pool.Close()

	expectedColumns := map[string][]string{
		"mlb_team_stats":         {"source", "external_id", "season", "stat_date", "team_name", "created_at", "updated_at"},
		"mlb_pitcher_stats":      {"source", "external_id", "season", "stat_date", "player_name", "team_external_id", "team_name", "created_at", "updated_at"},
		"nba_team_stats":         {"source", "external_id", "season", "stat_date", "team_name", "created_at", "updated_at"},
		"nhl_team_stats":         {"source", "external_id", "season", "stat_date", "team_name", "created_at", "updated_at"},
		"nhl_goalie_stats":       {"source", "external_id", "season", "stat_date", "player_name", "team_external_id", "team_name", "created_at", "updated_at"},
		"nfl_team_stats":         {"source", "external_id", "season", "stat_date", "team_name", "created_at", "updated_at"},
		"nfl_qb_stats":           {"source", "external_id", "season", "stat_date", "player_name", "team_external_id", "team_name", "created_at", "updated_at"},
		"player_injury_reports":  {"source", "sport", "report_date", "external_id", "player_name", "team_external_id", "position", "injury", "status", "raw_json", "created_at", "updated_at"},
		"game_weather_snapshots": {"game_id", "source", "forecast_time", "venue_name", "venue_timezone", "roof_type", "raw_json", "created_at", "updated_at"},
	}

	for table, columns := range expectedColumns {
		table := table
		columns := columns
		t.Run(table+" columns", func(t *testing.T) {
			actual := columnNamesForTable(ctx, t, pool, table)
			for _, column := range columns {
				if _, ok := actual[column]; !ok {
					t.Fatalf("expected column %q on table %s, got columns %v", column, table, sortedKeys(actual))
				}
			}
		})
	}

	var weatherGameID int64
	if err := pool.QueryRow(ctx, `
        INSERT INTO games (source, external_id, sport, home_team, away_team, commence_time)
        VALUES ('the-odds-api', 'weather-phase2-game', 'MLB', 'Boston Red Sox', 'New York Yankees', $1)
        RETURNING id
    `, time.Date(2026, time.March, 12, 18, 30, 0, 0, time.UTC)).Scan(&weatherGameID); err != nil {
		t.Fatalf("insert weather game seed: %v", err)
	}

	inserts := []struct {
		name string
		sql  string
		args []any
	}{
		{
			name: "mlb_team_stats",
			sql:  "INSERT INTO mlb_team_stats (source, external_id, season, stat_date, team_name) VALUES ($1, $2, $3, $4, $5)",
			args: []any{"statcast", "bos", 2026, time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC), "Boston Red Sox"},
		},
		{
			name: "mlb_pitcher_stats",
			sql:  "INSERT INTO mlb_pitcher_stats (source, external_id, season, stat_date, player_name, team_external_id, team_name) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			args: []any{"statcast", "sale", 2026, time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC), "Chris Sale", "atl", "Atlanta Braves"},
		},
		{
			name: "nba_team_stats",
			sql:  "INSERT INTO nba_team_stats (source, external_id, season, stat_date, team_name) VALUES ($1, $2, $3, $4, $5)",
			args: []any{"nba-stats", "bos", 2026, time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC), "Boston Celtics"},
		},
		{
			name: "nhl_team_stats",
			sql:  "INSERT INTO nhl_team_stats (source, external_id, season, stat_date, team_name) VALUES ($1, $2, $3, $4, $5)",
			args: []any{"moneypuck", "bos", 2026, time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC), "Boston Bruins"},
		},
		{
			name: "nhl_goalie_stats",
			sql:  "INSERT INTO nhl_goalie_stats (source, external_id, season, stat_date, player_name, team_external_id, team_name) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			args: []any{"moneypuck", "swayman", 2026, time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC), "Jeremy Swayman", "bos", "Boston Bruins"},
		},
		{
			name: "nfl_team_stats",
			sql:  "INSERT INTO nfl_team_stats (source, external_id, season, stat_date, team_name) VALUES ($1, $2, $3, $4, $5)",
			args: []any{"nflverse", "buf", 2026, time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC), "Buffalo Bills"},
		},
		{
			name: "nfl_qb_stats",
			sql:  "INSERT INTO nfl_qb_stats (source, external_id, season, stat_date, player_name, team_external_id, team_name) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			args: []any{"nflverse", "allen", 2026, time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC), "Josh Allen", "buf", "Buffalo Bills"},
		},
		{
			name: "player_injury_reports",
			sql:  "INSERT INTO player_injury_reports (source, sport, report_date, external_id, player_name, team_external_id, position, injury, status, player_url, raw_json) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
			args: []any{"rotowire", "nfl", time.Date(2026, time.March, 11, 0, 0, 0, 0, time.UTC), "12483", "Josh Allen", "buf", "QB", "Foot", "Questionable", "https://www.rotowire.com/football/player/josh-allen-12483", []byte(`{"ID":"12483"}`)},
		},
		{
			name: "game_weather_snapshots",
			sql:  "INSERT INTO game_weather_snapshots (game_id, source, forecast_time, venue_name, venue_timezone, latitude, longitude, roof_type, temperature_f, raw_json) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)",
			args: []any{weatherGameID, "open-meteo", time.Date(2026, time.March, 12, 19, 0, 0, 0, time.UTC), "Fenway Park", "America/New_York", 42.3467, -71.0972, "outdoor", 68.5, []byte(`{"forecast_time":"2026-03-12T19:00:00Z"}`)},
		},
	}

	for _, tc := range inserts {
		tc := tc
		t.Run(tc.name+" unique key", func(t *testing.T) {
			if _, err := pool.Exec(ctx, tc.sql, tc.args...); err != nil {
				t.Fatalf("insert first row into %s: %v", tc.name, err)
			}
			if _, err := pool.Exec(ctx, tc.sql, tc.args...); err == nil {
				t.Fatalf("expected unique violation on duplicate insert into %s", tc.name)
			} else {
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
					t.Fatalf("expected unique violation on duplicate insert into %s, got %v", tc.name, err)
				}
			}
		})
	}
}

func columnNamesForTable(ctx context.Context, t *testing.T, pool *pgxpool.Pool, table string) map[string]struct{} {
	t.Helper()

	rows, err := pool.Query(ctx, `
        SELECT column_name
        FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = $1
        ORDER BY ordinal_position ASC
    `, table)
	if err != nil {
		t.Fatalf("list columns for %s: %v", table, err)
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column name for %s: %v", table, err)
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate columns for %s: %v", table, err)
	}
	if len(columns) == 0 {
		t.Fatalf("expected table %s to exist with columns", table)
	}
	return columns
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
