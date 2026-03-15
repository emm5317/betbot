package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"betbot/internal/ingestion/moneypuck"
	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	importType := flag.String("type", "", "Import type: teams, goalies, odds, asplayed")
	filePath := flag.String("file", "", "Path to CSV file")
	batchSize := flag.Int("batch-size", 500, "Rows per database batch")
	dryRun := flag.Bool("dry-run", false, "Validate and count without writing to database")
	seasonFilter := flag.Int("season-filter", 0, "Import only this season (0 = all)")
	flag.Parse()

	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()

	if *importType == "" || *filePath == "" {
		log.Fatal().Msg("--type and --file are required")
	}

	switch *importType {
	case "teams", "goalies", "odds", "asplayed":
	default:
		log.Fatal().Str("type", *importType).Msg("invalid import type (must be teams, goalies, odds, or asplayed)")
	}

	f, err := os.Open(*filePath)
	if err != nil {
		log.Fatal().Err(err).Str("file", *filePath).Msg("open file")
	}
	defer f.Close()

	ctx := context.Background()
	tm := moneypuck.NewTeamMap()

	if *dryRun {
		log.Info().Str("type", *importType).Str("file", *filePath).Msg("dry run — validating only")
		count, err := dryRunImport(*importType, f, tm, seasonFilter)
		if err != nil {
			log.Fatal().Err(err).Msg("dry run failed")
		}
		log.Info().Int("rows_validated", count).Msg("dry run complete")
		return
	}

	dbURL := os.Getenv("BETBOT_DATABASE_URL")
	if dbURL == "" {
		log.Fatal().Msg("BETBOT_DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal().Err(err).Msg("connect to database")
	}
	defer pool.Close()

	queries := store.New(pool)
	start := time.Now()

	switch *importType {
	case "teams":
		count, err := importTeams(ctx, f, queries, tm, *batchSize, seasonFilter)
		if err != nil {
			log.Fatal().Err(err).Msg("import teams failed")
		}
		log.Info().Int("rows_imported", count).Dur("duration", time.Since(start)).Msg("teams import complete")

	case "goalies":
		count, err := importGoalies(ctx, f, queries, tm, *batchSize, seasonFilter)
		if err != nil {
			log.Fatal().Err(err).Msg("import goalies failed")
		}
		log.Info().Int("rows_imported", count).Dur("duration", time.Since(start)).Msg("goalies import complete")

	case "odds":
		count, err := importOdds(ctx, f, queries, tm, *batchSize)
		if err != nil {
			log.Fatal().Err(err).Msg("import odds failed")
		}
		log.Info().Int("rows_imported", count).Dur("duration", time.Since(start)).Msg("odds import complete")

	case "asplayed":
		count, err := importAsPlayed(ctx, f, queries, tm, *batchSize)
		if err != nil {
			log.Fatal().Err(err).Msg("import as-played failed")
		}
		log.Info().Int("rows_imported", count).Dur("duration", time.Since(start)).Msg("as-played import complete")
	}
}

func dryRunImport(importType string, r io.ReadSeeker, tm moneypuck.TeamMap, seasonFilter *int) (int, error) {
	var sf *int32
	if seasonFilter != nil && *seasonFilter > 0 {
		v := int32(*seasonFilter)
		sf = &v
	}

	count := 0
	switch importType {
	case "teams":
		reader, err := moneypuck.NewTeamCSVReader(r, tm, sf)
		if err != nil {
			return 0, err
		}
		for {
			_, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return count, fmt.Errorf("row %d: %w", count+1, err)
			}
			count++
			if count%5000 == 0 {
				log.Info().Int("validated", count).Msg("progress")
			}
		}
	case "goalies":
		reader, err := moneypuck.NewGoalieCSVReader(r, tm, sf)
		if err != nil {
			return 0, err
		}
		for {
			_, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return count, fmt.Errorf("row %d: %w", count+1, err)
			}
			count++
			if count%5000 == 0 {
				log.Info().Int("validated", count).Msg("progress")
			}
		}
	case "odds":
		reader, err := moneypuck.NewOddsCSVReader(r, tm)
		if err != nil {
			return 0, err
		}
		for {
			_, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return count, fmt.Errorf("row %d: %w", count+1, err)
			}
			count++
		}
	case "asplayed":
		reader, err := moneypuck.NewAsPlayedCSVReader(r, tm)
		if err != nil {
			return 0, err
		}
		for {
			_, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return count, fmt.Errorf("row %d: %w", count+1, err)
			}
			count++
		}
	}
	return count, nil
}

func importTeams(ctx context.Context, r io.Reader, q *store.Queries, tm moneypuck.TeamMap, batchSize int, seasonFilter *int) (int, error) {
	var sf *int32
	if seasonFilter != nil && *seasonFilter > 0 {
		v := int32(*seasonFilter)
		sf = &v
	}

	reader, err := moneypuck.NewTeamCSVReader(r, tm, sf)
	if err != nil {
		return 0, err
	}

	count := 0
	for {
		row, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("row %d: %w", count+1, err)
		}

		if err := q.UpsertMoneypuckTeamGame(ctx, store.UpsertMoneypuckTeamGameParams{
			Season:                          row.Season,
			GameID:                          row.GameID,
			Team:                            row.Team,
			Opponent:                        row.Opponent,
			HomeOrAway:                      row.HomeOrAway,
			GameDate:                        row.GameDate,
			Situation:                       row.Situation,
			IsPlayoff:                       row.IsPlayoff,
			XgoalsPercentage:                row.XgoalsPercentage,
			XgoalsFor:                       row.XgoalsFor,
			XgoalsAgainst:                   row.XgoalsAgainst,
			ScoreVenueAdjustedXgoalsFor:     row.ScoreVenueAdjustedXgoalsFor,
			ScoreVenueAdjustedXgoalsAgainst: row.ScoreVenueAdjustedXgoalsAgainst,
			CorsiPercentage:                 row.CorsiPercentage,
			FenwickPercentage:               row.FenwickPercentage,
			ShotsOnGoalFor:                  row.ShotsOnGoalFor,
			ShotsOnGoalAgainst:              row.ShotsOnGoalAgainst,
			ShotAttemptsFor:                 row.ShotAttemptsFor,
			ShotAttemptsAgainst:             row.ShotAttemptsAgainst,
			HighDangerShotsFor:              row.HighDangerShotsFor,
			HighDangerShotsAgainst:          row.HighDangerShotsAgainst,
			HighDangerXgoalsFor:             row.HighDangerXgoalsFor,
			HighDangerXgoalsAgainst:         row.HighDangerXgoalsAgainst,
			GoalsFor:                        row.GoalsFor,
			GoalsAgainst:                    row.GoalsAgainst,
		}); err != nil {
			return count, fmt.Errorf("upsert team game row %d: %w", count+1, err)
		}

		count++
		if count%5000 == 0 {
			log.Info().Int("imported", count).Msg("progress")
		}
	}

	return count, nil
}

func importGoalies(ctx context.Context, r io.Reader, q *store.Queries, tm moneypuck.TeamMap, batchSize int, seasonFilter *int) (int, error) {
	var sf *int32
	if seasonFilter != nil && *seasonFilter > 0 {
		v := int32(*seasonFilter)
		sf = &v
	}

	reader, err := moneypuck.NewGoalieCSVReader(r, tm, sf)
	if err != nil {
		return 0, err
	}

	count := 0
	for {
		row, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("row %d: %w", count+1, err)
		}

		if err := q.UpsertMoneypuckGoalieGame(ctx, store.UpsertMoneypuckGoalieGameParams{
			Season:           row.Season,
			GameID:           row.GameID,
			PlayerID:         row.PlayerID,
			Name:             row.Name,
			Team:             row.Team,
			Opponent:         row.Opponent,
			HomeOrAway:       row.HomeOrAway,
			GameDate:         row.GameDate,
			Situation:        row.Situation,
			Icetime:          row.Icetime,
			Xgoals:           row.Xgoals,
			Goals:            row.Goals,
			Gsax:             row.Gsax,
			ShotsAgainst:     row.ShotsAgainst,
			HighDangerXgoals: row.HighDangerXgoals,
			HighDangerGoals:  row.HighDangerGoals,
		}); err != nil {
			return count, fmt.Errorf("upsert goalie game row %d: %w", count+1, err)
		}

		count++
		if count%5000 == 0 {
			log.Info().Int("imported", count).Msg("progress")
		}
	}

	return count, nil
}

func importOdds(ctx context.Context, r io.Reader, q *store.Queries, tm moneypuck.TeamMap, batchSize int) (int, error) {
	reader, err := moneypuck.NewOddsCSVReader(r, tm)
	if err != nil {
		return 0, err
	}

	count := 0
	for {
		row, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("row %d: %w", count+1, err)
		}

		homeOddsAPI, err := tm.ToOddsAPIName(row.HomeTeam)
		if err != nil {
			return count, fmt.Errorf("row %d home team: %w", count+1, err)
		}
		awayOddsAPI, err := tm.ToOddsAPIName(row.AwayTeam)
		if err != nil {
			return count, fmt.Errorf("row %d away team: %w", count+1, err)
		}

		externalID := fmt.Sprintf("odds-csv-%s-%s-%s",
			row.GameDate.Time.Format("20060102"), row.HomeTeam, row.AwayTeam)

		game, err := q.UpsertGame(ctx, store.UpsertGameParams{
			Source:     "odds-csv",
			ExternalID: externalID,
			Sport:      "NHL",
			HomeTeam:   homeOddsAPI,
			AwayTeam:   awayOddsAPI,
			CommenceTime: pgtype.Timestamptz{
				Time:  row.CommenceTime,
				Valid: true,
			},
		})
		if err != nil {
			return count, fmt.Errorf("upsert game row %d: %w", count+1, err)
		}

		capturedAt := pgtype.Timestamptz{
			Time:  row.CommenceTime.Add(-1 * time.Hour),
			Valid: true,
		}

		snapshots := buildOddsSnapshots(game.ID, row, capturedAt)
		for _, snap := range snapshots {
			if _, err := q.InsertOddsSnapshot(ctx, snap); err != nil {
				return count, fmt.Errorf("insert odds snapshot row %d: %w", count+1, err)
			}
		}

		count++
		if count%200 == 0 {
			log.Info().Int("imported", count).Msg("progress")
		}
	}

	return count, nil
}

func buildOddsSnapshots(gameID int64, row *moneypuck.OddsRow, capturedAt pgtype.Timestamptz) []store.InsertOddsSnapshotParams {
	source := "odds-csv"
	bookKey := "consensus"
	bookName := "Consensus Line"

	homeOddsAPI, _ := moneypuck.NewTeamMap().ToOddsAPIName(row.HomeTeam)
	awayOddsAPI, _ := moneypuck.NewTeamMap().ToOddsAPIName(row.AwayTeam)

	var snapshots []store.InsertOddsSnapshotParams

	// H2H home
	snapshots = append(snapshots, store.InsertOddsSnapshotParams{
		GameID:             gameID,
		Source:             source,
		BookKey:            bookKey,
		BookName:           bookName,
		MarketKey:          "h2h",
		MarketName:         "Moneyline",
		OutcomeName:        homeOddsAPI,
		OutcomeSide:        "home",
		PriceAmerican:      int32(row.HomeMoneyLine),
		ImpliedProbability: row.HomeImpliedProbability(),
		SnapshotHash:       snapshotHash(gameID, "h2h", "home", row.HomeMoneyLine),
		RawJson:            json.RawMessage(`{}`),
		CapturedAt:         capturedAt,
	})

	// H2H away
	snapshots = append(snapshots, store.InsertOddsSnapshotParams{
		GameID:             gameID,
		Source:             source,
		BookKey:            bookKey,
		BookName:           bookName,
		MarketKey:          "h2h",
		MarketName:         "Moneyline",
		OutcomeName:        awayOddsAPI,
		OutcomeSide:        "away",
		PriceAmerican:      int32(row.AwayMoneyLine),
		ImpliedProbability: row.AwayImpliedProbability(),
		SnapshotHash:       snapshotHash(gameID, "h2h", "away", row.AwayMoneyLine),
		RawJson:            json.RawMessage(`{}`),
		CapturedAt:         capturedAt,
	})

	// Spread home
	homeSpread := row.HomeSpread
	snapshots = append(snapshots, store.InsertOddsSnapshotParams{
		GameID:             gameID,
		Source:             source,
		BookKey:            bookKey,
		BookName:           bookName,
		MarketKey:          "spreads",
		MarketName:         "Puck Line",
		OutcomeName:        homeOddsAPI,
		OutcomeSide:        "home",
		PriceAmerican:      int32(row.HomeSpreadLine),
		Point:              &homeSpread,
		ImpliedProbability: moneypuck.AmericanToImpliedProbability(row.HomeSpreadLine),
		SnapshotHash:       snapshotHash(gameID, "spreads", "home", row.HomeSpreadLine),
		RawJson:            json.RawMessage(`{}`),
		CapturedAt:         capturedAt,
	})

	// Spread away
	awaySpread := row.AwaySpread
	snapshots = append(snapshots, store.InsertOddsSnapshotParams{
		GameID:             gameID,
		Source:             source,
		BookKey:            bookKey,
		BookName:           bookName,
		MarketKey:          "spreads",
		MarketName:         "Puck Line",
		OutcomeName:        awayOddsAPI,
		OutcomeSide:        "away",
		PriceAmerican:      int32(row.AwaySpreadLine),
		Point:              &awaySpread,
		ImpliedProbability: moneypuck.AmericanToImpliedProbability(row.AwaySpreadLine),
		SnapshotHash:       snapshotHash(gameID, "spreads", "away", row.AwaySpreadLine),
		RawJson:            json.RawMessage(`{}`),
		CapturedAt:         capturedAt,
	})

	// Totals over
	ou := row.OverUnder
	snapshots = append(snapshots, store.InsertOddsSnapshotParams{
		GameID:             gameID,
		Source:             source,
		BookKey:            bookKey,
		BookName:           bookName,
		MarketKey:          "totals",
		MarketName:         "Over/Under",
		OutcomeName:        "Over",
		OutcomeSide:        "over",
		PriceAmerican:      int32(row.OverLine),
		Point:              &ou,
		ImpliedProbability: moneypuck.AmericanToImpliedProbability(row.OverLine),
		SnapshotHash:       snapshotHash(gameID, "totals", "over", row.OverLine),
		RawJson:            json.RawMessage(`{}`),
		CapturedAt:         capturedAt,
	})

	// Totals under
	snapshots = append(snapshots, store.InsertOddsSnapshotParams{
		GameID:             gameID,
		Source:             source,
		BookKey:            bookKey,
		BookName:           bookName,
		MarketKey:          "totals",
		MarketName:         "Over/Under",
		OutcomeName:        "Under",
		OutcomeSide:        "under",
		PriceAmerican:      int32(row.UnderLine),
		Point:              &ou,
		ImpliedProbability: moneypuck.AmericanToImpliedProbability(row.UnderLine),
		SnapshotHash:       snapshotHash(gameID, "totals", "under", row.UnderLine),
		RawJson:            json.RawMessage(`{}`),
		CapturedAt:         capturedAt,
	})

	return snapshots
}

func importAsPlayed(ctx context.Context, r io.Reader, q *store.Queries, tm moneypuck.TeamMap, batchSize int) (int, error) {
	reader, err := moneypuck.NewAsPlayedCSVReader(r, tm)
	if err != nil {
		return 0, err
	}

	count := 0
	for {
		row, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("row %d: %w", count+1, err)
		}

		homeOddsAPI, err := tm.ToOddsAPIName(row.HomeTeam)
		if err != nil {
			return count, fmt.Errorf("row %d home team: %w", count+1, err)
		}
		awayOddsAPI, err := tm.ToOddsAPIName(row.AwayTeam)
		if err != nil {
			return count, fmt.Errorf("row %d away team: %w", count+1, err)
		}

		externalID := fmt.Sprintf("asplayed-%s-%s-%s",
			row.GameDate.Time.Format("20060102"), row.HomeTeam, row.AwayTeam)

		game, err := q.UpsertGame(ctx, store.UpsertGameParams{
			Source:     "asplayed-csv",
			ExternalID: externalID,
			Sport:      "NHL",
			HomeTeam:   homeOddsAPI,
			AwayTeam:   awayOddsAPI,
			CommenceTime: pgtype.Timestamptz{
				Time:  row.CommenceTime,
				Valid: true,
			},
		})
		if err != nil {
			return count, fmt.Errorf("upsert game row %d: %w", count+1, err)
		}

		if row.HasOdds {
			capturedAt := pgtype.Timestamptz{
				Time:  row.CommenceTime.Add(-1 * time.Hour),
				Valid: true,
			}
			snapshots := buildAsPlayedOddsSnapshots(game.ID, row, capturedAt, tm)
			for _, snap := range snapshots {
				if _, err := q.InsertOddsSnapshot(ctx, snap); err != nil {
					return count, fmt.Errorf("insert odds snapshot row %d: %w", count+1, err)
				}
			}
		}

		count++
		if count%200 == 0 {
			log.Info().Int("imported", count).Msg("progress")
		}
	}

	return count, nil
}

func buildAsPlayedOddsSnapshots(gameID int64, row *moneypuck.AsPlayedRow, capturedAt pgtype.Timestamptz, tm moneypuck.TeamMap) []store.InsertOddsSnapshotParams {
	source := "asplayed-csv"
	bookKey := "consensus"
	bookName := "Consensus Line"

	homeOddsAPI, _ := tm.ToOddsAPIName(row.HomeTeam)
	awayOddsAPI, _ := tm.ToOddsAPIName(row.AwayTeam)

	var snapshots []store.InsertOddsSnapshotParams

	// H2H
	snapshots = append(snapshots, store.InsertOddsSnapshotParams{
		GameID: gameID, Source: source, BookKey: bookKey, BookName: bookName,
		MarketKey: "h2h", MarketName: "Moneyline",
		OutcomeName: homeOddsAPI, OutcomeSide: "home",
		PriceAmerican: int32(row.HomeMoneyLine), ImpliedProbability: row.HomeImpliedProbability(),
		SnapshotHash: snapshotHash(gameID, "h2h", "home", row.HomeMoneyLine),
		RawJson: json.RawMessage(`{}`), CapturedAt: capturedAt,
	})
	snapshots = append(snapshots, store.InsertOddsSnapshotParams{
		GameID: gameID, Source: source, BookKey: bookKey, BookName: bookName,
		MarketKey: "h2h", MarketName: "Moneyline",
		OutcomeName: awayOddsAPI, OutcomeSide: "away",
		PriceAmerican: int32(row.AwayMoneyLine), ImpliedProbability: row.AwayImpliedProbability(),
		SnapshotHash: snapshotHash(gameID, "h2h", "away", row.AwayMoneyLine),
		RawJson: json.RawMessage(`{}`), CapturedAt: capturedAt,
	})

	// Spreads
	homeSpread := row.HomeSpread
	awaySpread := -row.HomeSpread
	snapshots = append(snapshots, store.InsertOddsSnapshotParams{
		GameID: gameID, Source: source, BookKey: bookKey, BookName: bookName,
		MarketKey: "spreads", MarketName: "Puck Line",
		OutcomeName: homeOddsAPI, OutcomeSide: "home",
		PriceAmerican: int32(row.HomeSpreadLine), Point: &homeSpread,
		ImpliedProbability: moneypuck.AmericanToImpliedProbability(row.HomeSpreadLine),
		SnapshotHash: snapshotHash(gameID, "spreads", "home", row.HomeSpreadLine),
		RawJson: json.RawMessage(`{}`), CapturedAt: capturedAt,
	})
	snapshots = append(snapshots, store.InsertOddsSnapshotParams{
		GameID: gameID, Source: source, BookKey: bookKey, BookName: bookName,
		MarketKey: "spreads", MarketName: "Puck Line",
		OutcomeName: awayOddsAPI, OutcomeSide: "away",
		PriceAmerican: int32(row.AwaySpreadLine), Point: &awaySpread,
		ImpliedProbability: moneypuck.AmericanToImpliedProbability(row.AwaySpreadLine),
		SnapshotHash: snapshotHash(gameID, "spreads", "away", row.AwaySpreadLine),
		RawJson: json.RawMessage(`{}`), CapturedAt: capturedAt,
	})

	// Totals
	ou := row.OverUnder
	snapshots = append(snapshots, store.InsertOddsSnapshotParams{
		GameID: gameID, Source: source, BookKey: bookKey, BookName: bookName,
		MarketKey: "totals", MarketName: "Over/Under",
		OutcomeName: "Over", OutcomeSide: "over",
		PriceAmerican: int32(row.OverLine), Point: &ou,
		ImpliedProbability: moneypuck.AmericanToImpliedProbability(row.OverLine),
		SnapshotHash: snapshotHash(gameID, "totals", "over", row.OverLine),
		RawJson: json.RawMessage(`{}`), CapturedAt: capturedAt,
	})
	snapshots = append(snapshots, store.InsertOddsSnapshotParams{
		GameID: gameID, Source: source, BookKey: bookKey, BookName: bookName,
		MarketKey: "totals", MarketName: "Over/Under",
		OutcomeName: "Under", OutcomeSide: "under",
		PriceAmerican: int32(row.UnderLine), Point: &ou,
		ImpliedProbability: moneypuck.AmericanToImpliedProbability(row.UnderLine),
		SnapshotHash: snapshotHash(gameID, "totals", "under", row.UnderLine),
		RawJson: json.RawMessage(`{}`), CapturedAt: capturedAt,
	})

	return snapshots
}

func snapshotHash(gameID int64, market, side string, price int) string {
	data := fmt.Sprintf("%d|%s|%s|%d", gameID, market, side, price)
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h[:8])
}
