package backtest

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"time"

	"betbot/internal/domain"
	"betbot/internal/ingestion/moneypuck"
	"betbot/internal/modeling"
	"betbot/internal/modeling/features"
	modelmlb "betbot/internal/modeling/mlb"
	modelnba "betbot/internal/modeling/nba"
	modelnfl "betbot/internal/modeling/nfl"
	modelnhl "betbot/internal/modeling/nhl"
	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultRowLimit              = 5000
	defaultMarketKey             = "h2h"
	defaultModelVersion          = "baseline-v1"
	defaultWalkForwardTrain      = 64
	defaultWalkForwardValidation = 16
	defaultWalkForwardStep       = 16
	defaultStartingBankroll      = 10000.0
	defaultMinimumStakeDollars   = 1.0
)

// ReplayStore captures the sqlc-generated storage APIs required for backtest replay.
type ReplayStore interface {
	ListBacktestReplayRows(ctx context.Context, arg store.ListBacktestReplayRowsParams) ([]store.ListBacktestReplayRowsRow, error)
	UpsertModelPrediction(ctx context.Context, arg store.UpsertModelPredictionParams) (store.ModelPrediction, error)
}

type RunConfig struct {
	Sport                 *domain.Sport
	Season                *int
	MarketKey             string
	RowLimit              int
	ModelVersion          string
	RollingWindow         int
	WalkForwardTrain      int
	WalkForwardValidation int
	WalkForwardStep       int
	StartingBankroll      float64
	MinimumStakeDollars   float64
	KellyFraction         float64
	MaxStakeFraction      float64
}

// OutcomeRunConfig configures a multi-season outcome-based backtest that iterates
// MoneyPuck data directly without requiring odds data.
type OutcomeRunConfig struct {
	SeasonStart           *int
	SeasonEnd             *int
	RollingWindow         int
	ModelVersion          string
	WalkForwardTrain      int
	WalkForwardValidation int
	WalkForwardStep       int
}

type Outcome struct {
	GameID                     int64        `json:"game_id"`
	Source                     string       `json:"source"`
	ExternalID                 string       `json:"external_id"`
	Sport                      domain.Sport `json:"sport"`
	HomeTeam                   string       `json:"home_team"`
	AwayTeam                   string       `json:"away_team"`
	BookKey                    string       `json:"book_key"`
	MarketKey                  string       `json:"market_key"`
	CommenceTimeUTC            time.Time    `json:"commence_time_utc"`
	OpeningCapturedAtUTC       time.Time    `json:"opening_captured_at_utc"`
	ClosingCapturedAtUTC       time.Time    `json:"closing_captured_at_utc"`
	OpeningHomeProbability     float64      `json:"opening_home_probability"`
	ClosingHomeProbability     float64      `json:"closing_home_probability"`
	PredictedHomeProbability   float64      `json:"predicted_home_probability"`
	RecommendedSide            string       `json:"recommended_side"`
	OpeningSideProbability     float64      `json:"opening_side_probability"`
	ClosingSideProbability     float64      `json:"closing_side_probability"`
	ModelEdge                  float64      `json:"model_edge"`
	CLVDelta                   float64      `json:"clv_delta"`
	CalibrationError           float64      `json:"calibration_error"`
	KellyFraction              float64      `json:"kelly_fraction"`
	MaxStakeFraction           float64      `json:"max_stake_fraction"`
	RecommendedStakeFraction   float64      `json:"recommended_stake_fraction"`
	RecommendedStakeDollars    float64      `json:"recommended_stake_dollars"`
	VirtualBankrollBalancePost float64      `json:"virtual_bankroll_balance_post"`
	HasRealFeatures            bool         `json:"has_real_features"`
	ActualHomeGoals            *float64     `json:"actual_home_goals,omitempty"`
	ActualAwayGoals            *float64     `json:"actual_away_goals,omitempty"`
	ActualHomeWin              *bool        `json:"actual_home_win,omitempty"`
	OutcomeCalibrationError    *float64     `json:"outcome_calibration_error,omitempty"`
}

type WalkForwardFold struct {
	TrainStartUTC           time.Time `json:"train_start_utc"`
	TrainEndUTC             time.Time `json:"train_end_utc"`
	ValidationStartUTC      time.Time `json:"validation_start_utc"`
	ValidationEndUTC        time.Time `json:"validation_end_utc"`
	ValidationSamples       int       `json:"validation_samples"`
	MeanCLV                 float64   `json:"mean_clv"`
	PositiveCLVRate         float64   `json:"positive_clv_rate"`
	MeanCalibrationAbsError float64   `json:"mean_calibration_abs_error"`
	BrierCalibrationError   float64   `json:"brier_calibration_error"`
}

type CLVReport struct {
	Samples             int     `json:"samples"`
	MeanCLV             float64 `json:"mean_clv"`
	MedianCLV           float64 `json:"median_clv"`
	PositiveCLVRate     float64 `json:"positive_clv_rate"`
	AverageAbsoluteEdge float64 `json:"average_absolute_model_edge"`
}

type CalibrationReport struct {
	Samples                int     `json:"samples"`
	MeanAbsoluteError      float64 `json:"mean_absolute_error"`
	BrierScore             float64 `json:"brier_score"`
	ExpectedCalibrationErr float64 `json:"expected_calibration_error"`
}

type SportCalibrationArtifact struct {
	Sport    domain.Sport                  `json:"sport"`
	Artifact *features.CalibrationArtifact `json:"artifact,omitempty"`
	Error    string                        `json:"error,omitempty"`
}

// SeasonCalibration holds per-season calibration metrics for outcome-based backtests.
type SeasonCalibration struct {
	Season                int     `json:"season"`
	Samples               int     `json:"samples"`
	RealFeatureRate       float64 `json:"real_feature_rate"`
	MeanAbsoluteError     float64 `json:"mean_absolute_error"`
	BrierScore            float64 `json:"brier_score"`
	HomeWinRate           float64 `json:"home_win_rate"`
	PredictedHomeWinRate  float64 `json:"predicted_home_win_rate"`
}

type PipelineArtifact struct {
	GeneratedAtUTC          time.Time                  `json:"generated_at_utc"`
	BacktestMode            string                     `json:"backtest_mode,omitempty"`
	CLVMode                 string                     `json:"clv_mode,omitempty"`
	RollingWindow           int                        `json:"rolling_window,omitempty"`
	SportFilter             *domain.Sport              `json:"sport_filter,omitempty"`
	SeasonFilter            *int                       `json:"season_filter,omitempty"`
	SeasonStartFilter       *int                       `json:"season_start_filter,omitempty"`
	SeasonEndFilter         *int                       `json:"season_end_filter,omitempty"`
	MarketKey               string                     `json:"market_key"`
	ModelVersion            string                     `json:"model_version"`
	PersistedPredictionRows int                        `json:"persisted_prediction_rows"`
	Outcomes                []Outcome                  `json:"outcomes"`
	WalkForward             []WalkForwardFold          `json:"walk_forward"`
	CLV                     CLVReport                  `json:"clv"`
	Calibration             CalibrationReport          `json:"calibration"`
	SportCalibrations       []SportCalibrationArtifact `json:"sport_calibrations"`
	SeasonCalibrations      []SeasonCalibration        `json:"season_calibrations,omitempty"`
}

type Engine struct {
	store    ReplayStore
	mpStore  MoneyPuckStore // optional: enables real NHL features
	registry features.Registry

	mlbModel modelmlb.PitcherMatchupModel
	nbaModel modelnba.NetRatingModel
	nhlModel modelnhl.XGGoalieModel
	nflModel modelnfl.EPADVOAModel
}

func NewEngine(storeSink ReplayStore, opts ...EngineOption) (Engine, error) {
	if storeSink == nil {
		return Engine{}, errors.New("replay store is required")
	}
	registry, err := features.NewDefaultRegistry()
	if err != nil {
		return Engine{}, fmt.Errorf("create feature registry: %w", err)
	}

	e := Engine{
		store:    storeSink,
		registry: registry,
		mlbModel: modelmlb.NewDefaultPitcherMatchupModel(),
		nbaModel: modelnba.NewDefaultNetRatingModel(),
		nhlModel: modelnhl.NewDefaultXGGoalieModel(),
		nflModel: modelnfl.NewDefaultEPADVOAModel(),
	}
	for _, opt := range opts {
		opt(&e)
	}
	return e, nil
}

// EngineOption configures optional Engine behavior.
type EngineOption func(*Engine)

// WithMoneyPuckStore enables real NHL features from MoneyPuck data.
func WithMoneyPuckStore(s MoneyPuckStore) EngineOption {
	return func(e *Engine) {
		e.mpStore = s
	}
}

func (e Engine) Run(ctx context.Context, cfg RunConfig) (PipelineArtifact, error) {
	resolved, err := resolveRunConfig(cfg)
	if err != nil {
		return PipelineArtifact{}, err
	}

	var sportFilter *string
	if resolved.Sport != nil {
		s := string(*resolved.Sport)
		sportFilter = &s
	}
	var seasonFilter *int32
	if resolved.Season != nil {
		s := int32(*resolved.Season)
		seasonFilter = &s
	}
	market := resolved.MarketKey

	rows, err := e.store.ListBacktestReplayRows(ctx, store.ListBacktestReplayRowsParams{
		RowLimit:  int32(resolved.RowLimit),
		Sport:     sportFilter,
		Season:    seasonFilter,
		MarketKey: &market,
	})
	if err != nil {
		return PipelineArtifact{}, fmt.Errorf("list replay rows: %w", err)
	}
	if len(rows) == 0 {
		return PipelineArtifact{}, errors.New("no replay rows found for the selected filters")
	}

	outcomes := make([]Outcome, 0, len(rows))
	samplesBySport := make(map[domain.Sport][]features.CalibrationSample)
	persistedCount := 0

	bankrollCfg := BankrollConfig{
		StartingBankroll:    resolved.StartingBankroll,
		KellyFraction:       resolved.KellyFraction,
		MaxStakeFraction:    resolved.MaxStakeFraction,
		MinimumStakeDollars: resolved.MinimumStakeDollars,
	}
	if resolved.Sport != nil {
		bankrollCfg.Sport = *resolved.Sport
	}
	virtualBankroll, err := NewVirtualBankroll(bankrollCfg)
	if err != nil {
		return PipelineArtifact{}, fmt.Errorf("build virtual bankroll: %w", err)
	}

	for _, row := range rows {
		sport := domain.Sport(strings.TrimSpace(row.Sport))
		if _, ok := domain.DefaultSportRegistry().Get(sport); !ok {
			return PipelineArtifact{}, fmt.Errorf("unsupported sport in replay row: %s", row.Sport)
		}

		commenceAt, err := requireTimestamp(row.CommenceTime, "commence_time")
		if err != nil {
			return PipelineArtifact{}, err
		}
		openAt, err := requireTimestamp(row.OpeningCapturedAt, "opening_captured_at")
		if err != nil {
			return PipelineArtifact{}, err
		}
		closeAt, err := requireTimestamp(row.ClosingCapturedAt, "closing_captured_at")
		if err != nil {
			return PipelineArtifact{}, err
		}

		if row.OpeningHomeImpliedProbability < 0 || row.OpeningHomeImpliedProbability > 1 {
			return PipelineArtifact{}, fmt.Errorf("opening home probability out of range for game %d", row.GameID)
		}
		if row.ClosingHomeImpliedProbability < 0 || row.ClosingHomeImpliedProbability > 1 {
			return PipelineArtifact{}, fmt.Errorf("closing home probability out of range for game %d", row.GameID)
		}

		var req features.BuildRequest
		hasRealFeatures := false
		if sport == domain.SportNHL && e.mpStore != nil {
			nhlResult, nhlErr := BuildNHLFeatures(ctx, e.mpStore,
				row.HomeTeam, row.AwayTeam, commenceAt, nhlSeasonFromDate(commenceAt), row.OpeningHomeImpliedProbability, resolved.RollingWindow)
			if nhlErr == nil {
				req = nhlResult.Request
				hasRealFeatures = nhlResult.HasReal
			} else {
				req = deterministicBuildRequest(row, sport)
			}
		} else {
			req = deterministicBuildRequest(row, sport)
		}
		vector, err := e.registry.Build(req)
		if err != nil {
			return PipelineArtifact{}, fmt.Errorf("build features for game %d: %w", row.GameID, err)
		}

		predictedHome, err := e.predictHomeProbability(req)
		if err != nil {
			return PipelineArtifact{}, fmt.Errorf("predict home probability for game %d: %w", row.GameID, err)
		}

		closing := row.ClosingHomeImpliedProbability
		if _, err := modeling.PersistPrediction(ctx, e.store, modeling.PersistRequest{
			GameID:               row.GameID,
			Source:               row.Source,
			Sport:                sport,
			BookKey:              row.BookKey,
			MarketKey:            row.MarketKey,
			ModelFamily:          vector.ModelFamily,
			ModelVersion:         resolved.ModelVersion,
			FeatureVector:        vector,
			PredictedProbability: predictedHome,
			MarketProbability:    row.OpeningHomeImpliedProbability,
			ClosingProbability:   &closing,
			EventTimeUTC:         openAt,
		}); err != nil {
			return PipelineArtifact{}, fmt.Errorf("persist prediction for game %d: %w", row.GameID, err)
		}
		persistedCount++

		recommendedSide := "home"
		openingSideProb := row.OpeningHomeImpliedProbability
		closingSideProb := row.ClosingHomeImpliedProbability
		modelEdge := predictedHome - row.OpeningHomeImpliedProbability
		if predictedHome < row.OpeningHomeImpliedProbability {
			recommendedSide = "away"
			openingSideProb = 1 - row.OpeningHomeImpliedProbability
			closingSideProb = 1 - row.ClosingHomeImpliedProbability
			modelEdge = (1 - predictedHome) - (1 - row.OpeningHomeImpliedProbability)
		}

		clvDelta := closingSideProb - openingSideProb
		sizing, err := virtualBankroll.RecommendStakeForSport(sport, modelEdge)
		if err != nil {
			return PipelineArtifact{}, fmt.Errorf("recommend stake for game %d: %w", row.GameID, err)
		}
		virtualBankroll.ApplyCLV(sizing.StakeDollars, clvDelta)

		outcome := Outcome{
			GameID:                     row.GameID,
			Source:                     row.Source,
			ExternalID:                 row.ExternalID,
			Sport:                      sport,
			HomeTeam:                   row.HomeTeam,
			AwayTeam:                   row.AwayTeam,
			BookKey:                    row.BookKey,
			MarketKey:                  row.MarketKey,
			CommenceTimeUTC:            commenceAt,
			OpeningCapturedAtUTC:       openAt,
			ClosingCapturedAtUTC:       closeAt,
			OpeningHomeProbability:     row.OpeningHomeImpliedProbability,
			ClosingHomeProbability:     row.ClosingHomeImpliedProbability,
			PredictedHomeProbability:   predictedHome,
			RecommendedSide:            recommendedSide,
			OpeningSideProbability:     openingSideProb,
			ClosingSideProbability:     closingSideProb,
			ModelEdge:                  modelEdge,
			CLVDelta:                   clvDelta,
			CalibrationError:           predictedHome - row.ClosingHomeImpliedProbability,
			HasRealFeatures:            hasRealFeatures,
			KellyFraction:              sizing.KellyFraction,
			MaxStakeFraction:           sizing.MaxBetFraction,
			RecommendedStakeFraction:   sizing.StakeFraction,
			RecommendedStakeDollars:    sizing.StakeDollars,
			VirtualBankrollBalancePost: virtualBankroll.Balance(),
		}

		// Outcome-based calibration: look up actual game result if MoneyPuck data available
		if e.mpStore != nil && sport == domain.SportNHL {
			tm := moneypuck.NewTeamMap()
			homeAbbr, _ := tm.FromOddsAPIName(row.HomeTeam)
			mpGID, _ := e.mpStore.FindMoneypuckGameID(ctx, store.FindMoneypuckGameIDParams{
				Team:     homeAbbr,
				GameDate: pgtype.Date{Time: commenceAt, Valid: true},
			})
			gameResult, resErr := LookupGameOutcome(ctx, e.mpStore, mpGID)
			if resErr == nil && gameResult.Available {
				outcome.ActualHomeGoals = &gameResult.HomeGoals
				outcome.ActualAwayGoals = &gameResult.AwayGoals
				outcome.ActualHomeWin = &gameResult.HomeWin
				actualBinary := 0.0
				if gameResult.HomeWin {
					actualBinary = 1.0
				}
				outcomeCalErr := predictedHome - actualBinary
				outcome.OutcomeCalibrationError = &outcomeCalErr
			}
		}

		outcomes = append(outcomes, outcome)

		binaryOutcome := 0.0
		if row.ClosingHomeImpliedProbability >= 0.5 {
			binaryOutcome = 1.0
		}
		samplesBySport[sport] = append(samplesBySport[sport], features.CalibrationSample{
			EventTime: closeAt,
			Vector:    vector,
			Outcome:   binaryOutcome,
		})
	}

	sort.SliceStable(outcomes, func(i, j int) bool {
		if outcomes[i].ClosingCapturedAtUTC.Equal(outcomes[j].ClosingCapturedAtUTC) {
			if outcomes[i].GameID == outcomes[j].GameID {
				return outcomes[i].BookKey < outcomes[j].BookKey
			}
			return outcomes[i].GameID < outcomes[j].GameID
		}
		return outcomes[i].ClosingCapturedAtUTC.Before(outcomes[j].ClosingCapturedAtUTC)
	})

	calibration := computeCalibrationReport(outcomes)
	clv := computeCLVReport(outcomes)
	walkForward, err := computeWalkForward(outcomes, resolved)
	if err != nil {
		return PipelineArtifact{}, err
	}
	sportCalibrations := computeSportCalibrations(samplesBySport, resolved)

	// Detect CLV mode: if all CLV deltas are zero, it's single-snapshot odds data
	clvMode := "real"
	allZeroCLV := true
	for _, o := range outcomes {
		if o.CLVDelta != 0.0 {
			allZeroCLV = false
			break
		}
	}
	if allZeroCLV {
		clvMode = "single-snapshot"
	}

	generatedAt := outcomes[len(outcomes)-1].ClosingCapturedAtUTC
	return PipelineArtifact{
		GeneratedAtUTC:          generatedAt,
		BacktestMode:            "odds",
		CLVMode:                 clvMode,
		RollingWindow:           resolved.RollingWindow,
		SportFilter:             resolved.Sport,
		SeasonFilter:            resolved.Season,
		MarketKey:               resolved.MarketKey,
		ModelVersion:            resolved.ModelVersion,
		PersistedPredictionRows: persistedCount,
		Outcomes:                outcomes,
		WalkForward:             walkForward,
		CLV:                     clv,
		Calibration:             calibration,
		SportCalibrations:       sportCalibrations,
	}, nil
}

// RunOutcomeBacktest runs a multi-season outcome-based backtest that iterates MoneyPuck
// data directly without requiring odds data. It evaluates model predictions against actual
// game outcomes.
func (e Engine) RunOutcomeBacktest(ctx context.Context, cfg OutcomeRunConfig) (PipelineArtifact, error) {
	if e.mpStore == nil {
		return PipelineArtifact{}, errors.New("MoneyPuck store is required for outcome backtesting")
	}

	rollingWindow := cfg.RollingWindow
	if rollingWindow <= 0 {
		rollingWindow = defaultRollingWindow
	}
	modelVersion := strings.TrimSpace(cfg.ModelVersion)
	if modelVersion == "" {
		modelVersion = defaultModelVersion
	}
	wfTrain := cfg.WalkForwardTrain
	if wfTrain <= 0 {
		wfTrain = defaultWalkForwardTrain
	}
	wfValidation := cfg.WalkForwardValidation
	if wfValidation <= 0 {
		wfValidation = defaultWalkForwardValidation
	}
	wfStep := cfg.WalkForwardStep
	if wfStep <= 0 {
		wfStep = defaultWalkForwardStep
	}

	var seasonStart, seasonEnd *int32
	if cfg.SeasonStart != nil {
		v := int32(*cfg.SeasonStart)
		seasonStart = &v
	}
	if cfg.SeasonEnd != nil {
		v := int32(*cfg.SeasonEnd)
		seasonEnd = &v
	}

	rows, err := e.mpStore.ListOutcomeBacktestGames(ctx, store.ListOutcomeBacktestGamesParams{
		SeasonStart: seasonStart,
		SeasonEnd:   seasonEnd,
	})
	if err != nil {
		return PipelineArtifact{}, fmt.Errorf("list outcome backtest games: %w", err)
	}
	if len(rows) == 0 {
		return PipelineArtifact{}, errors.New("no outcome backtest games found for the selected season range")
	}

	outcomes := make([]Outcome, 0, len(rows))
	seasonStats := make(map[int]*seasonAccumulator)

	for _, row := range rows {
		gameDate := row.GameDate.Time
		season := row.Season

		nhlResult, nhlErr := BuildNHLFeaturesFromAbbrev(ctx, e.mpStore,
			row.HomeTeam, row.AwayTeam, gameDate, season, rollingWindow)

		var predictedHome float64
		hasRealFeatures := false
		if nhlErr == nil {
			hasRealFeatures = nhlResult.HasReal
			pred, predErr := e.nhlModel.Predict(modelnhl.MatchupInput{
				HomeTeam: modelnhl.TeamProfile{
					Name:                "home",
					ExpectedGoalsShare:  nhlResult.Request.NHL.HomeXGShare,
					GoalsForPerGame:     clamp(nhlResult.Request.TeamQuality.HomeOffenseRating/32.0, 1.8, 4.8),
					GoalsAgainstPerGame: clamp(nhlResult.Request.TeamQuality.HomeDefenseRating/34.0, 1.8, 4.8),
					GoalieGSAx:          nhlResult.Request.NHL.HomeGoalieGSAx,
					PDO:                 nhlResult.Request.NHL.HomePDO,
					CorsiShare:          nhlResult.Request.NHL.HomeCorsi,
				},
				AwayTeam: modelnhl.TeamProfile{
					Name:                "away",
					ExpectedGoalsShare:  nhlResult.Request.NHL.AwayXGShare,
					GoalsForPerGame:     clamp(nhlResult.Request.TeamQuality.AwayOffenseRating/32.0, 1.8, 4.8),
					GoalsAgainstPerGame: clamp(nhlResult.Request.TeamQuality.AwayDefenseRating/34.0, 1.8, 4.8),
					GoalieGSAx:          nhlResult.Request.NHL.AwayGoalieGSAx,
					PDO:                 nhlResult.Request.NHL.AwayPDO,
					CorsiShare:          nhlResult.Request.NHL.AwayCorsi,
				},
			})
			if predErr != nil {
				return PipelineArtifact{}, fmt.Errorf("predict home probability for game %s: %w", row.GameID, predErr)
			}
			predictedHome = pred.HomeWinProbability
		} else {
			predictedHome = 0.50
		}

		homeGoals := deref(row.HomeGoals, 0)
		awayGoals := deref(row.AwayGoals, 0)
		homeWin := homeGoals > awayGoals
		actualBinary := 0.0
		if homeWin {
			actualBinary = 1.0
		}
		outcomeCalErr := predictedHome - actualBinary

		outcome := Outcome{
			GameID:                   0,
			Source:                   "moneypuck",
			ExternalID:               row.GameID,
			Sport:                    domain.SportNHL,
			HomeTeam:                 row.HomeTeam,
			AwayTeam:                 row.AwayTeam,
			BookKey:                  "none",
			MarketKey:                "outcome",
			CommenceTimeUTC:          gameDate,
			OpeningCapturedAtUTC:     gameDate,
			ClosingCapturedAtUTC:     gameDate,
			PredictedHomeProbability: predictedHome,
			HasRealFeatures:         hasRealFeatures,
			CalibrationError:        outcomeCalErr,
			ActualHomeGoals:          &homeGoals,
			ActualAwayGoals:          &awayGoals,
			ActualHomeWin:            &homeWin,
			OutcomeCalibrationError:  &outcomeCalErr,
		}
		outcomes = append(outcomes, outcome)

		// Per-season accumulation
		acc, ok := seasonStats[int(season)]
		if !ok {
			acc = &seasonAccumulator{}
			seasonStats[int(season)] = acc
		}
		acc.samples++
		if hasRealFeatures {
			acc.realFeatures++
		}
		acc.sumAbsErr += math.Abs(outcomeCalErr)
		acc.sumBrier += outcomeCalErr * outcomeCalErr
		if homeWin {
			acc.homeWins++
		}
		acc.sumPredicted += predictedHome
	}

	// Build per-season calibrations
	seasonKeys := make([]int, 0, len(seasonStats))
	for k := range seasonStats {
		seasonKeys = append(seasonKeys, k)
	}
	sort.Ints(seasonKeys)

	seasonCalibrations := make([]SeasonCalibration, 0, len(seasonKeys))
	for _, s := range seasonKeys {
		acc := seasonStats[s]
		n := float64(acc.samples)
		seasonCalibrations = append(seasonCalibrations, SeasonCalibration{
			Season:               s,
			Samples:              acc.samples,
			RealFeatureRate:      float64(acc.realFeatures) / n,
			MeanAbsoluteError:    acc.sumAbsErr / n,
			BrierScore:           acc.sumBrier / n,
			HomeWinRate:          float64(acc.homeWins) / n,
			PredictedHomeWinRate: acc.sumPredicted / n,
		})
	}

	// Overall calibration from outcome calibration errors
	calibration := computeOutcomeCalibrationReport(outcomes)

	// Walk-forward on outcomes using game dates
	wfResolved := resolvedRunConfig{
		WalkForwardTrain:      wfTrain,
		WalkForwardValidation: wfValidation,
		WalkForwardStep:       wfStep,
	}
	walkForward, err := computeWalkForward(outcomes, wfResolved)
	if err != nil {
		return PipelineArtifact{}, err
	}

	generatedAt := time.Now().UTC()
	if len(outcomes) > 0 {
		generatedAt = outcomes[len(outcomes)-1].CommenceTimeUTC
	}

	return PipelineArtifact{
		GeneratedAtUTC:     generatedAt,
		BacktestMode:       "outcome",
		CLVMode:            "unavailable",
		RollingWindow:      rollingWindow,
		SeasonStartFilter:  cfg.SeasonStart,
		SeasonEndFilter:    cfg.SeasonEnd,
		MarketKey:          "outcome",
		ModelVersion:       modelVersion,
		Outcomes:           outcomes,
		WalkForward:        walkForward,
		Calibration:        calibration,
		SeasonCalibrations: seasonCalibrations,
	}, nil
}

type seasonAccumulator struct {
	samples      int
	realFeatures int
	sumAbsErr    float64
	sumBrier     float64
	homeWins     int
	sumPredicted float64
}

// computeOutcomeCalibrationReport computes calibration using actual outcomes (binary) instead of closing lines.
func computeOutcomeCalibrationReport(outcomes []Outcome) CalibrationReport {
	if len(outcomes) == 0 {
		return CalibrationReport{}
	}

	predictions := make([]float64, 0, len(outcomes))
	targets := make([]float64, 0, len(outcomes))
	absErr := 0.0
	brier := 0.0
	count := 0

	for _, outcome := range outcomes {
		if outcome.OutcomeCalibrationError == nil {
			continue
		}
		predicted := outcome.PredictedHomeProbability
		actual := 0.0
		if outcome.ActualHomeWin != nil && *outcome.ActualHomeWin {
			actual = 1.0
		}
		predictions = append(predictions, predicted)
		targets = append(targets, actual)
		delta := predicted - actual
		absErr += math.Abs(delta)
		brier += delta * delta
		count++
	}

	if count == 0 {
		return CalibrationReport{}
	}

	n := float64(count)
	return CalibrationReport{
		Samples:                count,
		MeanAbsoluteError:      absErr / n,
		BrierScore:             brier / n,
		ExpectedCalibrationErr: expectedCalibrationErrorContinuous(predictions, targets, 10),
	}
}

func (e Engine) predictHomeProbability(req features.BuildRequest) (float64, error) {
	switch req.Sport {
	case domain.SportMLB:
		homeRuns := clamp(req.TeamQuality.HomeOffenseRating/25.0, 2.5, 7.5)
		awayRuns := clamp(req.TeamQuality.AwayOffenseRating/25.0, 2.5, 7.5)
		homeERA := clamp((220-req.TeamQuality.HomeDefenseRating)/25.0, 2.8, 6.5)
		awayERA := clamp((220-req.TeamQuality.AwayDefenseRating)/25.0, 2.8, 6.5)
		prediction, err := e.mlbModel.Predict(modelmlb.MatchupInput{
			HomeTeam: modelmlb.TeamProfile{
				Name:        "home",
				RunsPerGame: homeRuns,
				BattingOPS:  floatPtr(clamp(0.650+req.TeamQuality.HomeOffenseRating/250.0, 0.600, 1.000)),
				TeamERA:     homeERA,
			},
			AwayTeam: modelmlb.TeamProfile{
				Name:        "away",
				RunsPerGame: awayRuns,
				BattingOPS:  floatPtr(clamp(0.650+req.TeamQuality.AwayOffenseRating/250.0, 0.600, 1.000)),
				TeamERA:     awayERA,
			},
			HomeStarter: modelmlb.PitcherProfile{
				Name:          "home-starter",
				ERA:           floatPtr(req.MLB.HomeStarterERA),
				FIP:           floatPtr(req.MLB.HomeStarterERA + 0.2),
				WHIP:          floatPtr(clamp(1.1+req.MLB.HomeStarterERA/20.0, 0.9, 1.8)),
				StrikeoutRate: floatPtr(0.24),
				WalkRate:      floatPtr(0.08),
			},
			AwayStarter: modelmlb.PitcherProfile{
				Name:          "away-starter",
				ERA:           floatPtr(req.MLB.AwayStarterERA),
				FIP:           floatPtr(req.MLB.AwayStarterERA + 0.2),
				WHIP:          floatPtr(clamp(1.1+req.MLB.AwayStarterERA/20.0, 0.9, 1.8)),
				StrikeoutRate: floatPtr(0.23),
				WalkRate:      floatPtr(0.08),
			},
		})
		if err != nil {
			return 0, err
		}
		return prediction.HomeMoneylineProbability, nil
	case domain.SportNBA:
		prediction, err := e.nbaModel.Predict(modelnba.MatchupInput{
			HomeTeam: modelnba.TeamProfile{
				Name:            "home",
				OffensiveRating: req.TeamQuality.HomeOffenseRating,
				DefensiveRating: req.TeamQuality.HomeDefenseRating,
				Pace:            req.NBA.ProjectedPace,
				Lineup: []modelnba.PlayerAvailability{{
					Name:            "home-core",
					Availability:    req.Injuries.HomeAvailability,
					OffensiveImpact: clamp(req.NBA.HomeLineupNetRating/3.0, -10, 10),
					DefensiveImpact: clamp(req.NBA.HomeLineupNetRating/4.0, -10, 10),
				}},
			},
			AwayTeam: modelnba.TeamProfile{
				Name:            "away",
				OffensiveRating: req.TeamQuality.AwayOffenseRating,
				DefensiveRating: req.TeamQuality.AwayDefenseRating,
				Pace:            req.NBA.ProjectedPace,
				Lineup: []modelnba.PlayerAvailability{{
					Name:            "away-core",
					Availability:    req.Injuries.AwayAvailability,
					OffensiveImpact: clamp(req.NBA.AwayLineupNetRating/3.0, -10, 10),
					DefensiveImpact: clamp(req.NBA.AwayLineupNetRating/4.0, -10, 10),
				}},
			},
			HomeSpreadLine: req.Market.HomeSpread,
			HomeRestDays:   req.Situational.HomeRestDays,
			AwayRestDays:   req.Situational.AwayRestDays,
		})
		if err != nil {
			return 0, err
		}
		return prediction.HomeWinProbability, nil
	case domain.SportNHL:
		prediction, err := e.nhlModel.Predict(modelnhl.MatchupInput{
			HomeTeam: modelnhl.TeamProfile{
				Name:                "home",
				ExpectedGoalsShare:  req.NHL.HomeXGShare,
				GoalsForPerGame:     clamp(req.TeamQuality.HomeOffenseRating/32.0, 1.8, 4.8),
				GoalsAgainstPerGame: clamp(req.TeamQuality.HomeDefenseRating/34.0, 1.8, 4.8),
				GoalieGSAx:          req.NHL.HomeGoalieGSAx,
				PDO:                 req.NHL.HomePDO,
				CorsiShare:          req.NHL.HomeCorsi,
			},
			AwayTeam: modelnhl.TeamProfile{
				Name:                "away",
				ExpectedGoalsShare:  req.NHL.AwayXGShare,
				GoalsForPerGame:     clamp(req.TeamQuality.AwayOffenseRating/32.0, 1.8, 4.8),
				GoalsAgainstPerGame: clamp(req.TeamQuality.AwayDefenseRating/34.0, 1.8, 4.8),
				GoalieGSAx:          req.NHL.AwayGoalieGSAx,
				PDO:                 req.NHL.AwayPDO,
				CorsiShare:          req.NHL.AwayCorsi,
			},
		})
		if err != nil {
			return 0, err
		}
		return prediction.HomeWinProbability, nil
	case domain.SportNFL:
		prediction, err := e.nflModel.Predict(modelnfl.MatchupInput{
			HomeTeam: modelnfl.TeamProfile{
				Name:       "home",
				QBEPA:      req.NFL.HomeQBEPA,
				DVOA:       req.NFL.HomeDVOA,
				OffenseEPA: req.NFL.HomeQBEPA,
				DefenseEPA: -req.NFL.HomeDVOA,
			},
			AwayTeam: modelnfl.TeamProfile{
				Name:       "away",
				QBEPA:      req.NFL.AwayQBEPA,
				DVOA:       req.NFL.AwayDVOA,
				OffenseEPA: req.NFL.AwayQBEPA,
				DefenseEPA: -req.NFL.AwayDVOA,
			},
			HomeSpreadLine:   req.Market.HomeSpread,
			TotalPointsLine:  req.Market.TotalPoints,
			WindMPH:          req.Weather.WindMPH,
			PrimaryKeyNumber: req.NFL.PrimaryKeyNumber,
			HomeRestDays:     req.Situational.HomeRestDays,
			AwayRestDays:     req.Situational.AwayRestDays,
		})
		if err != nil {
			return 0, err
		}
		return prediction.HomeWinProbability, nil
	default:
		return 0, fmt.Errorf("unsupported sport %s", req.Sport)
	}
}

func deterministicBuildRequest(row store.ListBacktestReplayRowsRow, sport domain.Sport) features.BuildRequest {
	seed := strings.Join([]string{row.ExternalID, row.HomeTeam, row.AwayTeam, row.BookKey, row.MarketKey}, "|")
	sportBaselines := map[domain.Sport]struct {
		offense float64
		defense float64
		total   float64
	}{
		domain.SportMLB: {offense: 108, defense: 108, total: 8.5},
		domain.SportNBA: {offense: 114, defense: 111, total: 226},
		domain.SportNHL: {offense: 97, defense: 99, total: 6.0},
		domain.SportNFL: {offense: 104, defense: 104, total: 45},
	}
	base := sportBaselines[sport]

	homeOffense := clamp(base.offense+spreadUnit(seed+":hoff", -12, 12), 70, 140)
	awayOffense := clamp(base.offense+spreadUnit(seed+":aoff", -12, 12), 70, 140)
	homeDefense := clamp(base.defense+spreadUnit(seed+":hdef", -12, 12), 70, 140)
	awayDefense := clamp(base.defense+spreadUnit(seed+":adef", -12, 12), 70, 140)

	homeSpread := clamp((0.5-row.OpeningHomeImpliedProbability)*20.0, -14, 14)
	total := clamp(base.total+spreadUnit(seed+":total", -6, 6), 4, 260)

	req := features.BuildRequest{
		Sport: sport,
		Market: features.MarketInputs{
			HomeMoneylineProbability: row.OpeningHomeImpliedProbability,
			AwayMoneylineProbability: 1 - row.OpeningHomeImpliedProbability,
			HomeSpread:               homeSpread,
			TotalPoints:              total,
		},
		TeamQuality: features.TeamQualityInputs{
			HomePowerRating:   clamp(90+spreadUnit(seed+":hpwr", -8, 8), 60, 130),
			AwayPowerRating:   clamp(90+spreadUnit(seed+":apwr", -8, 8), 60, 130),
			HomeOffenseRating: homeOffense,
			AwayOffenseRating: awayOffense,
			HomeDefenseRating: homeDefense,
			AwayDefenseRating: awayDefense,
		},
		Situational: features.SituationalInputs{
			HomeRestDays:    int(math.Round(spreadUnit(seed+":hrest", 0, 3))),
			AwayRestDays:    int(math.Round(spreadUnit(seed+":arest", 0, 3))),
			HomeTravelMiles: clamp(spreadUnit(seed+":htravel", 0, 600), 0, 6000),
			AwayTravelMiles: clamp(spreadUnit(seed+":atravel", 200, 1800), 0, 6000),
			HomeGamesLast7:  int(math.Round(spreadUnit(seed+":hgames", 2, 5))),
			AwayGamesLast7:  int(math.Round(spreadUnit(seed+":agames", 2, 5))),
		},
		Injuries: features.InjuryInputs{
			HomeAvailability: clamp(spreadUnit(seed+":havail", 0.82, 0.99), 0.5, 1),
			AwayAvailability: clamp(spreadUnit(seed+":aavail", 0.80, 0.99), 0.5, 1),
		},
		Weather: weatherInputsForSport(seed, sport),
	}

	switch sport {
	case domain.SportMLB:
		req.MLB = &features.MLBContext{
			HomeStarterERA: clamp(spreadUnit(seed+":hera", 2.8, 5.4), 2.0, 8.0),
			AwayStarterERA: clamp(spreadUnit(seed+":aera", 2.8, 5.4), 2.0, 8.0),
			HomeBullpenERA: clamp(spreadUnit(seed+":hbp", 3.2, 5.0), 2.0, 8.0),
			AwayBullpenERA: clamp(spreadUnit(seed+":abp", 3.2, 5.0), 2.0, 8.0),
			ParkFactor:     clamp(spreadUnit(seed+":park", 0.9, 1.1), 0.75, 1.25),
		}
	case domain.SportNBA:
		req.NBA = &features.NBAContext{
			HomeLineupNetRating: clamp(spreadUnit(seed+":hnet", -8, 12), -25, 25),
			AwayLineupNetRating: clamp(spreadUnit(seed+":anet", -8, 12), -25, 25),
			ProjectedPace:       clamp(spreadUnit(seed+":pace", 94, 102), 85, 115),
			HomeBackToBack:      deterministicUnit(seed+":hb2b") > 0.7,
			AwayBackToBack:      deterministicUnit(seed+":ab2b") > 0.7,
		}
	case domain.SportNHL:
		req.NHL = &features.NHLContext{
			HomeGoalieGSAx: clamp(spreadUnit(seed+":hgsax", -6, 16), -40, 40),
			AwayGoalieGSAx: clamp(spreadUnit(seed+":agsax", -6, 16), -40, 40),
			HomeXGShare:    clamp(spreadUnit(seed+":hxg", 0.46, 0.58), 0.35, 0.65),
			AwayXGShare:    clamp(spreadUnit(seed+":axg", 0.44, 0.56), 0.35, 0.65),
			HomePDO:        clamp(spreadUnit(seed+":hpdo", 0.975, 1.03), 0.90, 1.10),
			AwayPDO:        clamp(spreadUnit(seed+":apdo", 0.975, 1.03), 0.90, 1.10),
			HomeCorsi:      clamp(spreadUnit(seed+":hcorsi", 0.45, 0.57), 0.35, 0.65),
			AwayCorsi:      clamp(spreadUnit(seed+":acorsi", 0.43, 0.55), 0.35, 0.65),
		}
	case domain.SportNFL:
		keyNumber := 3.0
		if math.Abs(homeSpread) >= 5.5 {
			keyNumber = 7.0
		}
		req.NFL = &features.NFLContext{
			HomeQBEPA:        clamp(spreadUnit(seed+":hqb", -0.08, 0.24), -0.6, 0.6),
			AwayQBEPA:        clamp(spreadUnit(seed+":aqb", -0.10, 0.22), -0.6, 0.6),
			HomeDVOA:         clamp(spreadUnit(seed+":hdvoa", -0.12, 0.25), -0.8, 0.8),
			AwayDVOA:         clamp(spreadUnit(seed+":advoa", -0.12, 0.22), -0.8, 0.8),
			PrimaryKeyNumber: keyNumber,
		}
	}

	return req
}

func weatherInputsForSport(seed string, sport domain.Sport) features.WeatherInputs {
	switch sport {
	case domain.SportNBA, domain.SportNHL:
		return features.WeatherInputs{TemperatureF: 70, WindMPH: 0, PrecipitationMM: 0, IsDome: true}
	case domain.SportMLB:
		return features.WeatherInputs{
			TemperatureF:    clamp(spreadUnit(seed+":temp", 52, 86), -10, 120),
			WindMPH:         clamp(spreadUnit(seed+":wind", 2, 18), 0, 60),
			PrecipitationMM: clamp(spreadUnit(seed+":precip", 0, 4), 0, 40),
			IsDome:          deterministicUnit(seed+":dome") > 0.85,
		}
	default:
		return features.WeatherInputs{
			TemperatureF:    clamp(spreadUnit(seed+":temp", 38, 78), -10, 120),
			WindMPH:         clamp(spreadUnit(seed+":wind", 4, 22), 0, 60),
			PrecipitationMM: clamp(spreadUnit(seed+":precip", 0, 6), 0, 40),
			IsDome:          deterministicUnit(seed+":dome") > 0.55,
		}
	}
}

func computeWalkForward(outcomes []Outcome, cfg resolvedRunConfig) ([]WalkForwardFold, error) {
	if len(outcomes) < cfg.WalkForwardTrain+cfg.WalkForwardValidation {
		return nil, nil
	}

	splits, err := features.BuildWalkForwardSplits(len(outcomes), features.WalkForwardConfig{
		TrainWindow:      cfg.WalkForwardTrain,
		ValidationWindow: cfg.WalkForwardValidation,
		Step:             cfg.WalkForwardStep,
	})
	if err != nil {
		return nil, fmt.Errorf("build walk-forward splits: %w", err)
	}

	folds := make([]WalkForwardFold, 0, len(splits))
	for _, split := range splits {
		validation := outcomes[split.ValidationStart:split.ValidationEnd]
		metrics := computeCalibrationReport(validation)
		clv := computeCLVReport(validation)

		folds = append(folds, WalkForwardFold{
			TrainStartUTC:           outcomes[split.TrainStart].ClosingCapturedAtUTC,
			TrainEndUTC:             outcomes[split.TrainEnd-1].ClosingCapturedAtUTC,
			ValidationStartUTC:      outcomes[split.ValidationStart].ClosingCapturedAtUTC,
			ValidationEndUTC:        outcomes[split.ValidationEnd-1].ClosingCapturedAtUTC,
			ValidationSamples:       len(validation),
			MeanCLV:                 clv.MeanCLV,
			PositiveCLVRate:         clv.PositiveCLVRate,
			MeanCalibrationAbsError: metrics.MeanAbsoluteError,
			BrierCalibrationError:   metrics.BrierScore,
		})
	}

	return folds, nil
}

func computeSportCalibrations(samplesBySport map[domain.Sport][]features.CalibrationSample, cfg resolvedRunConfig) []SportCalibrationArtifact {
	orderedSports := []domain.Sport{domain.SportMLB, domain.SportNBA, domain.SportNHL, domain.SportNFL}
	artifacts := make([]SportCalibrationArtifact, 0, len(orderedSports))

	for _, sport := range orderedSports {
		samples := samplesBySport[sport]
		if len(samples) == 0 {
			continue
		}

		artifact, err := features.CalibrateNormalizationScales(features.CalibrationRequest{
			Sport:      sport,
			Samples:    samples,
			Window:     features.WalkForwardConfig{TrainWindow: cfg.WalkForwardTrain, ValidationWindow: cfg.WalkForwardValidation, Step: cfg.WalkForwardStep},
			BaseConfig: features.DefaultBuilderConfig(),
		})
		if err != nil {
			artifacts = append(artifacts, SportCalibrationArtifact{Sport: sport, Error: err.Error()})
			continue
		}
		artifacts = append(artifacts, SportCalibrationArtifact{Sport: sport, Artifact: &artifact})
	}

	return artifacts
}

func computeCLVReport(outcomes []Outcome) CLVReport {
	if len(outcomes) == 0 {
		return CLVReport{}
	}

	clvs := make([]float64, len(outcomes))
	edges := make([]float64, len(outcomes))
	sumCLV := 0.0
	positive := 0
	sumAbsEdge := 0.0
	for i, outcome := range outcomes {
		clvs[i] = outcome.CLVDelta
		edges[i] = math.Abs(outcome.ModelEdge)
		sumCLV += outcome.CLVDelta
		sumAbsEdge += edges[i]
		if outcome.CLVDelta > 0 {
			positive++
		}
	}
	sort.Float64s(clvs)

	median := clvs[len(clvs)/2]
	if len(clvs)%2 == 0 {
		median = (clvs[len(clvs)/2-1] + clvs[len(clvs)/2]) / 2
	}

	n := float64(len(outcomes))
	return CLVReport{
		Samples:             len(outcomes),
		MeanCLV:             sumCLV / n,
		MedianCLV:           median,
		PositiveCLVRate:     float64(positive) / n,
		AverageAbsoluteEdge: sumAbsEdge / n,
	}
}

func computeCalibrationReport(outcomes []Outcome) CalibrationReport {
	if len(outcomes) == 0 {
		return CalibrationReport{}
	}

	predictions := make([]float64, len(outcomes))
	targets := make([]float64, len(outcomes))
	absErr := 0.0
	brier := 0.0

	for i, outcome := range outcomes {
		predictions[i] = outcome.PredictedHomeProbability
		targets[i] = outcome.ClosingHomeProbability
		delta := outcome.PredictedHomeProbability - outcome.ClosingHomeProbability
		absErr += math.Abs(delta)
		brier += delta * delta
	}

	n := float64(len(outcomes))
	return CalibrationReport{
		Samples:                len(outcomes),
		MeanAbsoluteError:      absErr / n,
		BrierScore:             brier / n,
		ExpectedCalibrationErr: expectedCalibrationErrorContinuous(predictions, targets, 10),
	}
}

func expectedCalibrationErrorContinuous(predictions, targets []float64, bins int) float64 {
	if len(predictions) == 0 || len(predictions) != len(targets) || bins <= 0 {
		return 0
	}
	type binStats struct {
		count int
		sumP  float64
		sumT  float64
	}
	stats := make([]binStats, bins)
	for i := range predictions {
		p := clamp(predictions[i], 0, 1)
		index := int(math.Floor(p * float64(bins)))
		if index >= bins {
			index = bins - 1
		}
		stats[index].count++
		stats[index].sumP += p
		stats[index].sumT += clamp(targets[i], 0, 1)
	}

	total := float64(len(predictions))
	ece := 0.0
	for _, bin := range stats {
		if bin.count == 0 {
			continue
		}
		avgP := bin.sumP / float64(bin.count)
		avgT := bin.sumT / float64(bin.count)
		ece += (float64(bin.count) / total) * math.Abs(avgP-avgT)
	}
	return ece
}

func requireTimestamp(value pgtype.Timestamptz, field string) (time.Time, error) {
	if !value.Valid {
		return time.Time{}, fmt.Errorf("%s is required", field)
	}
	return value.Time.UTC(), nil
}

type resolvedRunConfig struct {
	Sport                 *domain.Sport
	Season                *int
	MarketKey             string
	RowLimit              int
	ModelVersion          string
	RollingWindow         int
	WalkForwardTrain      int
	WalkForwardValidation int
	WalkForwardStep       int
	StartingBankroll      float64
	MinimumStakeDollars   float64
	KellyFraction         float64
	MaxStakeFraction      float64
}

func resolveRunConfig(cfg RunConfig) (resolvedRunConfig, error) {
	resolved := resolvedRunConfig{
		Sport:                 cfg.Sport,
		Season:                cfg.Season,
		MarketKey:             strings.TrimSpace(cfg.MarketKey),
		RowLimit:              cfg.RowLimit,
		ModelVersion:          strings.TrimSpace(cfg.ModelVersion),
		RollingWindow:         cfg.RollingWindow,
		WalkForwardTrain:      cfg.WalkForwardTrain,
		WalkForwardValidation: cfg.WalkForwardValidation,
		WalkForwardStep:       cfg.WalkForwardStep,
		StartingBankroll:      cfg.StartingBankroll,
		MinimumStakeDollars:   cfg.MinimumStakeDollars,
		KellyFraction:         cfg.KellyFraction,
		MaxStakeFraction:      cfg.MaxStakeFraction,
	}
	if resolved.RollingWindow <= 0 {
		resolved.RollingWindow = defaultRollingWindow
	}
	if resolved.MarketKey == "" {
		resolved.MarketKey = defaultMarketKey
	}
	if resolved.RowLimit <= 0 {
		resolved.RowLimit = defaultRowLimit
	}
	if resolved.ModelVersion == "" {
		resolved.ModelVersion = defaultModelVersion
	}
	if resolved.WalkForwardTrain <= 0 {
		resolved.WalkForwardTrain = defaultWalkForwardTrain
	}
	if resolved.WalkForwardValidation <= 0 {
		resolved.WalkForwardValidation = defaultWalkForwardValidation
	}
	if resolved.WalkForwardStep <= 0 {
		resolved.WalkForwardStep = defaultWalkForwardStep
	}
	if resolved.StartingBankroll <= 0 {
		resolved.StartingBankroll = defaultStartingBankroll
	}
	if resolved.MinimumStakeDollars < 0 {
		return resolvedRunConfig{}, errors.New("minimum stake dollars must be >= 0")
	}
	if resolved.MinimumStakeDollars == 0 {
		resolved.MinimumStakeDollars = defaultMinimumStakeDollars
	}
	if resolved.KellyFraction < 0 || resolved.KellyFraction > 1 {
		return resolvedRunConfig{}, errors.New("kelly fraction must be in [0,1]")
	}
	if resolved.MaxStakeFraction < 0 || resolved.MaxStakeFraction > 1 {
		return resolvedRunConfig{}, errors.New("max stake fraction must be in [0,1]")
	}
	if resolved.Season != nil && *resolved.Season < 2000 {
		return resolvedRunConfig{}, errors.New("season must be >= 2000")
	}
	if resolved.Sport != nil {
		if _, ok := domain.DefaultSportRegistry().Get(*resolved.Sport); !ok {
			return resolvedRunConfig{}, fmt.Errorf("unsupported sport filter %s", *resolved.Sport)
		}
	}
	return resolved, nil
}

func deterministicUnit(seed string) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	return float64(h.Sum64()%1000000) / 999999.0
}

func spreadUnit(seed string, min, max float64) float64 {
	return min + (max-min)*deterministicUnit(seed)
}

func floatPtr(v float64) *float64 {
	return &v
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
