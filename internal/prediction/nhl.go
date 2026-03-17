package prediction

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"betbot/internal/backtest"
	modelnhl "betbot/internal/modeling/nhl"
	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultRollingWindow = 40
	modelFamily          = "xg-goalie-quality"
	modelVersion         = "v1"
	manifestVersion      = "live-bridge-v1"
	predictionSource     = "live"
	staleThreshold       = 15 * time.Minute
)

// NHLPredictionService runs the NHL model against live games and persists predictions.
type NHLPredictionService struct {
	pool          *pgxpool.Pool
	nhlModel      modelnhl.XGGoalieModel
	rollingWindow int
	logger        *slog.Logger
}

// NewNHLPredictionService creates a new NHL prediction service.
func NewNHLPredictionService(pool *pgxpool.Pool, logger *slog.Logger) *NHLPredictionService {
	if logger == nil {
		logger = slog.Default()
	}
	return &NHLPredictionService{
		pool:          pool,
		nhlModel:      modelnhl.NewDefaultXGGoalieModel(),
		rollingWindow: defaultRollingWindow,
		logger:        logger,
	}
}

// NHLPredictionResult holds both h2h and totals predictions from a single model run.
type NHLPredictionResult struct {
	HomeWinProbability float64
	ExpectedTotalGoals float64
	OverProbability    float64 // P(total > line)
	UnderProbability   float64 // P(total <= line)
	TotalsLine         float64 // the line used (e.g. 5.5)
}

// PredictGame runs the NHL model for a single game and persists both h2h and totals predictions.
func (s *NHLPredictionService) PredictGame(
	ctx context.Context,
	gameID int64,
	homeTeam, awayTeam string,
	gameDate time.Time,
	season int32,
	marketHomeProb float64,
) (float64, error) {
	result, err := s.PredictGameFull(ctx, gameID, homeTeam, awayTeam, gameDate, season, marketHomeProb, 0)
	if err != nil {
		return 0, err
	}
	return result.HomeWinProbability, nil
}

// PredictGameFull runs the NHL model and persists both h2h and totals predictions.
// If totalsLine is 0, totals odds are looked up from odds_history.
func (s *NHLPredictionService) PredictGameFull(
	ctx context.Context,
	gameID int64,
	homeTeam, awayTeam string,
	gameDate time.Time,
	season int32,
	marketHomeProb float64,
	totalsLine float64,
) (NHLPredictionResult, error) {
	q := store.New(s.pool)

	nhlResult, err := backtest.BuildNHLFeatures(
		ctx, q, homeTeam, awayTeam, gameDate, season, marketHomeProb, s.rollingWindow,
	)
	if err != nil {
		return NHLPredictionResult{}, fmt.Errorf("build NHL features for game %d: %w", gameID, err)
	}

	input := modelnhl.MatchupInput{
		HomeTeam: modelnhl.TeamProfile{
			Name:                homeTeam,
			ExpectedGoalsShare:  nhlResult.Request.NHL.HomeXGShare,
			GoalsForPerGame:     clamp(nhlResult.Request.TeamQuality.HomeOffenseRating/32.0, 1.8, 4.8),
			GoalsAgainstPerGame: clamp(nhlResult.Request.TeamQuality.HomeDefenseRating/34.0, 1.8, 4.8),
			GoalieGSAx:          nhlResult.Request.NHL.HomeGoalieGSAx,
			PDO:                 nhlResult.Request.NHL.HomePDO,
			CorsiShare:          nhlResult.Request.NHL.HomeCorsi,
		},
		AwayTeam: modelnhl.TeamProfile{
			Name:                awayTeam,
			ExpectedGoalsShare:  nhlResult.Request.NHL.AwayXGShare,
			GoalsForPerGame:     clamp(nhlResult.Request.TeamQuality.AwayOffenseRating/32.0, 1.8, 4.8),
			GoalsAgainstPerGame: clamp(nhlResult.Request.TeamQuality.AwayDefenseRating/34.0, 1.8, 4.8),
			GoalieGSAx:          nhlResult.Request.NHL.AwayGoalieGSAx,
			PDO:                 nhlResult.Request.NHL.AwayPDO,
			CorsiShare:          nhlResult.Request.NHL.AwayCorsi,
		},
	}

	pred, err := s.nhlModel.Predict(input)
	if err != nil {
		return NHLPredictionResult{}, fmt.Errorf("predict game %d: %w", gameID, err)
	}

	featureVector := []float64{
		nhlResult.Request.NHL.HomeXGShare,
		nhlResult.Request.NHL.AwayXGShare,
		nhlResult.Request.NHL.HomeGoalieGSAx,
		nhlResult.Request.NHL.AwayGoalieGSAx,
		nhlResult.Request.NHL.HomePDO,
		nhlResult.Request.NHL.AwayPDO,
		nhlResult.Request.NHL.HomeCorsi,
		nhlResult.Request.NHL.AwayCorsi,
		nhlResult.Request.TeamQuality.HomeOffenseRating,
		nhlResult.Request.TeamQuality.AwayOffenseRating,
		nhlResult.Request.TeamQuality.HomeDefenseRating,
		nhlResult.Request.TeamQuality.AwayDefenseRating,
	}

	// Persist h2h prediction
	_, err = q.UpsertModelPrediction(ctx, store.UpsertModelPredictionParams{
		GameID:               gameID,
		Source:               predictionSource,
		Sport:                "NHL",
		BookKey:              "consensus",
		MarketKey:            "h2h",
		ModelFamily:          modelFamily,
		ModelVersion:         modelVersion,
		ManifestVersion:      manifestVersion,
		FeatureVector:        featureVector,
		PredictedProbability: pred.HomeWinProbability,
		MarketProbability:    marketHomeProb,
		EventTime:            store.Timestamptz(gameDate),
	})
	if err != nil {
		return NHLPredictionResult{}, fmt.Errorf("upsert h2h prediction for game %d: %w", gameID, err)
	}

	// Determine totals line
	result := NHLPredictionResult{
		HomeWinProbability: pred.HomeWinProbability,
		ExpectedTotalGoals: pred.ExpectedTotalGoals,
	}

	if totalsLine <= 0 {
		// Look up from odds_history
		linePtr, lineErr := q.GetLatestTotalsLineForGame(ctx, gameID)
		if lineErr != nil || linePtr == nil {
			// No totals line available — still return h2h result
			s.logger.InfoContext(ctx, "NHL prediction generated (h2h only, no totals line)",
				slog.Int64("game_id", gameID),
				slog.String("home_team", homeTeam),
				slog.Float64("predicted_home_prob", pred.HomeWinProbability),
				slog.Float64("expected_total", pred.ExpectedTotalGoals),
			)
			return result, nil
		}
		totalsLine = *linePtr
	}

	overProb, underProb := pred.OverUnderProbability(totalsLine)
	result.OverProbability = overProb
	result.UnderProbability = underProb
	result.TotalsLine = totalsLine

	// Use over implied probability from the market as market_probability for the totals prediction.
	// Convention: predicted_probability = over probability, market_probability = market over implied.
	marketOverProb := 0.50 // default if no market data
	marketOver, moErr := q.GetLatestTotalsOverProbForGame(ctx, gameID)
	if moErr == nil {
		marketOverProb = marketOver
	}

	_, err = q.UpsertModelPrediction(ctx, store.UpsertModelPredictionParams{
		GameID:               gameID,
		Source:               predictionSource,
		Sport:                "NHL",
		BookKey:              "consensus",
		MarketKey:            "totals",
		ModelFamily:          modelFamily,
		ModelVersion:         modelVersion,
		ManifestVersion:      manifestVersion,
		FeatureVector:        append(featureVector, pred.ExpectedTotalGoals, totalsLine),
		PredictedProbability: overProb,
		MarketProbability:    marketOverProb,
		EventTime:            store.Timestamptz(gameDate),
	})
	if err != nil {
		return NHLPredictionResult{}, fmt.Errorf("upsert totals prediction for game %d: %w", gameID, err)
	}

	s.logger.InfoContext(ctx, "NHL prediction generated (h2h + totals)",
		slog.Int64("game_id", gameID),
		slog.String("home_team", homeTeam),
		slog.String("away_team", awayTeam),
		slog.Float64("predicted_home_prob", pred.HomeWinProbability),
		slog.Float64("market_home_prob", marketHomeProb),
		slog.Float64("expected_total", pred.ExpectedTotalGoals),
		slog.Float64("totals_line", totalsLine),
		slog.Float64("over_prob", overProb),
		slog.Float64("under_prob", underProb),
		slog.Bool("has_real_features", nhlResult.HasReal),
		slog.String("home_goalie", nhlResult.HomeGoalie),
		slog.String("away_goalie", nhlResult.AwayGoalie),
	)

	return result, nil
}

// PredictUpcomingGames scans upcoming NHL games and runs predictions for all that need them.
func (s *NHLPredictionService) PredictUpcomingGames(ctx context.Context) (int, error) {
	q := store.New(s.pool)

	games, err := q.ListUpcomingGamesForSport(ctx, "NHL")
	if err != nil {
		return 0, fmt.Errorf("list upcoming NHL games: %w", err)
	}

	if len(games) == 0 {
		s.logger.InfoContext(ctx, "no upcoming NHL games found")
		return 0, nil
	}

	// Pre-load existing live predictions to avoid per-game queries
	existing, err := q.ListModelPredictionsForSportSeason(ctx, store.ListModelPredictionsForSportSeasonParams{
		Sport: "NHL",
	})
	if err != nil {
		return 0, fmt.Errorf("check existing predictions: %w", err)
	}
	recentByGameID := make(map[int64]bool, len(existing))
	for _, p := range existing {
		if p.Source == predictionSource && p.CreatedAt.Valid && time.Since(p.CreatedAt.Time) < staleThreshold {
			recentByGameID[p.GameID] = true
		}
	}

	predicted := 0
	for _, game := range games {
		if !game.CommenceTime.Valid {
			continue
		}

		if recentByGameID[game.ID] {
			continue
		}

		// Get latest market implied probability
		marketProb, err := q.GetLatestMarketProbabilityForGame(ctx, game.ID)
		if err != nil {
			if store.IsNoRows(err) {
				s.logger.WarnContext(ctx, "no market odds for game, skipping",
					slog.Int64("game_id", game.ID),
					slog.String("home_team", game.HomeTeam),
				)
				continue
			}
			return predicted, fmt.Errorf("get market probability for game %d: %w", game.ID, err)
		}

		gameDate := game.CommenceTime.Time
		season := nhlSeasonFromDate(gameDate)

		_, err = s.PredictGame(ctx, game.ID, game.HomeTeam, game.AwayTeam, gameDate, season, marketProb)
		if err != nil {
			s.logger.ErrorContext(ctx, "prediction failed for game",
				slog.Int64("game_id", game.ID),
				slog.String("error", err.Error()),
			)
			continue
		}

		predicted++
	}

	s.logger.InfoContext(ctx, "NHL prediction run complete",
		slog.Int("games_found", len(games)),
		slog.Int("predictions_made", predicted),
	)

	return predicted, nil
}

// nhlSeasonFromDate converts a game date to the MoneyPuck season year.
func nhlSeasonFromDate(d time.Time) int32 {
	if d.Month() >= time.October {
		return int32(d.Year())
	}
	return int32(d.Year() - 1)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
