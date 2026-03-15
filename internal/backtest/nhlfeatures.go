package backtest

import (
	"context"
	"fmt"
	"time"

	"betbot/internal/domain"
	"betbot/internal/ingestion/moneypuck"
	"betbot/internal/modeling/features"
	"betbot/internal/store"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultRollingWindow = 20
	minRollingGames      = 10
)

// MoneyPuckStore captures the sqlc queries needed for real NHL features.
type MoneyPuckStore interface {
	GetTeamRolling5on5Stats(ctx context.Context, arg store.GetTeamRolling5on5StatsParams) ([]store.GetTeamRolling5on5StatsRow, error)
	GetStartingGoalie(ctx context.Context, arg store.GetStartingGoalieParams) (store.GetStartingGoalieRow, error)
	GetGoalieSeasonGSAx(ctx context.Context, arg store.GetGoalieSeasonGSAxParams) (store.GetGoalieSeasonGSAxRow, error)
	GetGameResult(ctx context.Context, gameID string) ([]store.GetGameResultRow, error)
	FindMoneypuckGameID(ctx context.Context, arg store.FindMoneypuckGameIDParams) (string, error)
}

// NHLFeatureResult holds the computed features and metadata for an NHL game.
type NHLFeatureResult struct {
	Request    features.BuildRequest
	HasReal    bool   // true if real MoneyPuck data was used
	HomeGoalie string // starting goalie name (empty if unavailable)
	AwayGoalie string
}

// GameOutcome holds the actual result of a game.
type GameOutcome struct {
	HomeGoals float64
	AwayGoals float64
	HomeWin   bool
	Available bool
}

// BuildNHLFeatures computes a BuildRequest using real MoneyPuck data for an NHL game.
// homeTeamAPI and awayTeamAPI are Odds API full names (e.g. "Tampa Bay Lightning").
// Falls back to deterministic defaults if insufficient data exists.
func BuildNHLFeatures(
	ctx context.Context,
	mpStore MoneyPuckStore,
	homeTeamAPI, awayTeamAPI string,
	gameDate time.Time,
	season int32,
	openingHomeProb float64,
) (NHLFeatureResult, error) {

	tm := moneypuck.NewTeamMap()
	homeAbbrev, err := tm.FromOddsAPIName(homeTeamAPI)
	if err != nil {
		return buildDefaultNHLFeatures(openingHomeProb), nil
	}
	awayAbbrev, err := tm.FromOddsAPIName(awayTeamAPI)
	if err != nil {
		return buildDefaultNHLFeatures(openingHomeProb), nil
	}

	pgDate := pgtype.Date{Time: gameDate, Valid: true}

	// Find the MoneyPuck game_id for goalie lookups
	mpGameID, _ := mpStore.FindMoneypuckGameID(ctx, store.FindMoneypuckGameIDParams{
		Team:     homeAbbrev,
		GameDate: pgDate,
	})

	homeStats, err := mpStore.GetTeamRolling5on5Stats(ctx, store.GetTeamRolling5on5StatsParams{
		Team:     homeAbbrev,
		GameDate: pgDate,
		Season:   season,
		Limit:    int32(defaultRollingWindow),
	})
	if err != nil {
		return NHLFeatureResult{}, fmt.Errorf("get home rolling stats: %w", err)
	}

	awayStats, err := mpStore.GetTeamRolling5on5Stats(ctx, store.GetTeamRolling5on5StatsParams{
		Team:     awayAbbrev,
		GameDate: pgDate,
		Season:   season,
		Limit:    int32(defaultRollingWindow),
	})
	if err != nil {
		return NHLFeatureResult{}, fmt.Errorf("get away rolling stats: %w", err)
	}

	if len(homeStats) < minRollingGames || len(awayStats) < minRollingGames {
		return buildDefaultNHLFeatures(openingHomeProb), nil
	}

	homeRolling := computeRollingAverages(homeStats)
	awayRolling := computeRollingAverages(awayStats)

	result := NHLFeatureResult{HasReal: true}

	// Look up starting goalies and their cumulative GSAx
	homeGSAx := 0.0
	awayGSAx := 0.0

	homeGoalie, hgErr := mpStore.GetStartingGoalie(ctx, store.GetStartingGoalieParams{
		GameID: mpGameID,
		Team:   homeAbbrev,
	})
	if hgErr == nil {
		result.HomeGoalie = homeGoalie.Name
		gsaxRow, gErr := mpStore.GetGoalieSeasonGSAx(ctx, store.GetGoalieSeasonGSAxParams{
			PlayerID: homeGoalie.PlayerID,
			Season:   season,
			GameDate: pgDate,
		})
		if gErr == nil && gsaxRow.GamesPlayed > 0 {
			homeGSAx = gsaxRow.CumulativeGsax
		}
	}

	awayGoalie, agErr := mpStore.GetStartingGoalie(ctx, store.GetStartingGoalieParams{
		GameID: mpGameID,
		Team:   awayAbbrev,
	})
	if agErr == nil {
		result.AwayGoalie = awayGoalie.Name
		gsaxRow, gErr := mpStore.GetGoalieSeasonGSAx(ctx, store.GetGoalieSeasonGSAxParams{
			PlayerID: awayGoalie.PlayerID,
			Season:   season,
			GameDate: pgDate,
		})
		if gErr == nil && gsaxRow.GamesPlayed > 0 {
			awayGSAx = gsaxRow.CumulativeGsax
		}
	}

	homeSpread := clamp((0.5-openingHomeProb)*20.0, -14, 14)

	result.Request = features.BuildRequest{
		Sport: domain.SportNHL,
		Market: features.MarketInputs{
			HomeMoneylineProbability: openingHomeProb,
			AwayMoneylineProbability: 1 - openingHomeProb,
			HomeSpread:               homeSpread,
			TotalPoints:              homeRolling.goalsFor + homeRolling.goalsAgainst + awayRolling.goalsFor + awayRolling.goalsAgainst,
		},
		TeamQuality: features.TeamQualityInputs{
			HomePowerRating:   clamp(homeRolling.xgPct*200, 60, 130),
			AwayPowerRating:   clamp(awayRolling.xgPct*200, 60, 130),
			HomeOffenseRating: clamp(homeRolling.goalsFor*32, 70, 140),
			AwayOffenseRating: clamp(awayRolling.goalsFor*32, 70, 140),
			HomeDefenseRating: clamp(homeRolling.goalsAgainst*34, 70, 140),
			AwayDefenseRating: clamp(awayRolling.goalsAgainst*34, 70, 140),
		},
		Situational: features.SituationalInputs{
			HomeRestDays:   1,
			AwayRestDays:   1,
			HomeGamesLast7: clampInt(len(homeStats)/3, 2, 5),
			AwayGamesLast7: clampInt(len(awayStats)/3, 2, 5),
		},
		Injuries: features.InjuryInputs{
			HomeAvailability: 0.95,
			AwayAvailability: 0.95,
		},
		Weather: features.WeatherInputs{
			TemperatureF: 70,
			WindMPH:      0,
			IsDome:       true,
		},
		NHL: &features.NHLContext{
			HomeGoalieGSAx: clamp(homeGSAx, -40, 40),
			AwayGoalieGSAx: clamp(awayGSAx, -40, 40),
			HomeXGShare:    clamp(homeRolling.xgPct, 0.35, 0.65),
			AwayXGShare:    clamp(awayRolling.xgPct, 0.35, 0.65),
			HomePDO:        clamp(homeRolling.pdo, 0.90, 1.10),
			AwayPDO:        clamp(awayRolling.pdo, 0.90, 1.10),
		},
	}

	return result, nil
}

// LookupGameOutcome queries MoneyPuck for the actual game result.
func LookupGameOutcome(ctx context.Context, mpStore MoneyPuckStore, gameID string) (GameOutcome, error) {
	rows, err := mpStore.GetGameResult(ctx, gameID)
	if err != nil {
		return GameOutcome{}, fmt.Errorf("get game result: %w", err)
	}
	if len(rows) < 2 {
		return GameOutcome{Available: false}, nil
	}

	var homeGoals, awayGoals float64
	for _, row := range rows {
		gf := 0.0
		if row.GoalsFor != nil {
			gf = *row.GoalsFor
		}
		if row.HomeOrAway == "HOME" {
			homeGoals = gf
		} else {
			awayGoals = gf
		}
	}

	return GameOutcome{
		HomeGoals: homeGoals,
		AwayGoals: awayGoals,
		HomeWin:   homeGoals > awayGoals,
		Available: true,
	}, nil
}

type rollingAverages struct {
	xgPct       float64
	goalsFor    float64
	goalsAgainst float64
	pdo         float64
	corsi       float64
}

func computeRollingAverages(rows []store.GetTeamRolling5on5StatsRow) rollingAverages {
	n := float64(len(rows))
	if n == 0 {
		return rollingAverages{xgPct: 0.50, goalsFor: 3.0, goalsAgainst: 3.0, pdo: 1.0, corsi: 0.50}
	}

	var sumXGPct, sumGF, sumGA, sumSOGF, sumSOGA, sumCorsi float64
	for _, r := range rows {
		sumXGPct += deref(r.XgoalsPercentage, 0.50)
		sumGF += deref(r.GoalsFor, 3.0)
		sumGA += deref(r.GoalsAgainst, 3.0)
		sumSOGF += deref(r.ShotsOnGoalFor, 30.0)
		sumSOGA += deref(r.ShotsOnGoalAgainst, 30.0)
		sumCorsi += deref(r.CorsiPercentage, 0.50)
	}

	avgGF := sumGF / n
	avgGA := sumGA / n
	avgSOGF := sumSOGF / n
	avgSOGA := sumSOGA / n

	shootingPct := 0.0
	if avgSOGF > 0 {
		shootingPct = avgGF / avgSOGF
	}
	savePct := 0.0
	if avgSOGA > 0 {
		savePct = 1.0 - (avgGA / avgSOGA)
	}

	return rollingAverages{
		xgPct:        sumXGPct / n,
		goalsFor:     avgGF,
		goalsAgainst: avgGA,
		pdo:          shootingPct + savePct,
		corsi:        sumCorsi / n,
	}
}

// nhlSeasonFromDate converts a game date to the MoneyPuck season year.
// NHL seasons span Oct–Jun: Oct 2023 → season 2023, Jan 2024 → season 2023.
func nhlSeasonFromDate(d time.Time) int32 {
	if d.Month() >= time.October {
		return int32(d.Year())
	}
	return int32(d.Year() - 1)
}

func buildDefaultNHLFeatures(openingHomeProb float64) NHLFeatureResult {
	homeSpread := clamp((0.5-openingHomeProb)*20.0, -14, 14)
	return NHLFeatureResult{
		HasReal: false,
		Request: features.BuildRequest{
			Sport: domain.SportNHL,
			Market: features.MarketInputs{
				HomeMoneylineProbability: openingHomeProb,
				AwayMoneylineProbability: 1 - openingHomeProb,
				HomeSpread:               homeSpread,
				TotalPoints:              6.0,
			},
			TeamQuality: features.TeamQualityInputs{
				HomePowerRating: 100, AwayPowerRating: 100,
				HomeOffenseRating: 97, AwayOffenseRating: 97,
				HomeDefenseRating: 99, AwayDefenseRating: 99,
			},
			Situational: features.SituationalInputs{
				HomeRestDays: 1, AwayRestDays: 1,
				HomeGamesLast7: 3, AwayGamesLast7: 3,
			},
			Injuries: features.InjuryInputs{HomeAvailability: 0.95, AwayAvailability: 0.95},
			Weather:  features.WeatherInputs{TemperatureF: 70, IsDome: true},
			NHL: &features.NHLContext{
				HomeGoalieGSAx: 0, AwayGoalieGSAx: 0,
				HomeXGShare: 0.50, AwayXGShare: 0.50,
				HomePDO: 1.0, AwayPDO: 1.0,
			},
		},
	}
}

func deref(p *float64, fallback float64) float64 {
	if p == nil {
		return fallback
	}
	return *p
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
