package decision

import (
	"fmt"
	"math"
	"time"

	"betbot/internal/domain"
)

const (
	defaultCorrelationMaxPicksPerGame           = 1
	defaultCorrelationMaxStakeFractionPerGame   = 0.03
	defaultCorrelationMaxPicksPerSportDay       = 0
	maxCorrelationMaxPicksPerGame               = 25
	maxCorrelationMaxPicksPerSportDay           = 500
	correlationReasonRetainedWithinLimits       = "retained_within_limits"
	correlationReasonRetainedZeroStake          = "retained_zero_stake"
	correlationReasonDroppedInvalidStake        = "dropped_invalid_stake_fraction"
	correlationReasonDroppedMaxPicksPerGame     = "dropped_max_picks_per_game"
	correlationReasonDroppedMaxStakePerGame     = "dropped_max_stake_fraction_per_game"
	correlationReasonDroppedMaxPicksPerSportDay = "dropped_max_picks_per_sport_day"
)

const correlationStakeEpsilon = 1e-12

type CorrelationPolicy struct {
	MaxPicksPerGame         int     `json:"max_picks_per_game"`
	MaxStakeFractionPerGame float64 `json:"max_stake_fraction_per_game"`
	MaxPicksPerSportDay     int     `json:"max_picks_per_sport_day"`
}

type CorrelationGuardResult struct {
	Retained []Recommendation `json:"retained"`
	Dropped  []Recommendation `json:"dropped"`
}

func DefaultCorrelationPolicy() CorrelationPolicy {
	return CorrelationPolicy{
		MaxPicksPerGame:         defaultCorrelationMaxPicksPerGame,
		MaxStakeFractionPerGame: defaultCorrelationMaxStakeFractionPerGame,
		MaxPicksPerSportDay:     defaultCorrelationMaxPicksPerSportDay,
	}
}

func ResolveCorrelationPolicy(maxPicksPerGame int, maxStakeFractionPerGame float64, maxPicksPerSportDay int) (CorrelationPolicy, error) {
	if math.IsNaN(maxStakeFractionPerGame) || math.IsInf(maxStakeFractionPerGame, 0) {
		return CorrelationPolicy{}, fmt.Errorf("max stake fraction per game must be finite")
	}
	policy := DefaultCorrelationPolicy()
	if maxPicksPerGame > 0 {
		policy.MaxPicksPerGame = maxPicksPerGame
	}
	if maxStakeFractionPerGame > 0 {
		policy.MaxStakeFractionPerGame = maxStakeFractionPerGame
	}
	if maxPicksPerSportDay > 0 {
		policy.MaxPicksPerSportDay = maxPicksPerSportDay
	}

	if maxPicksPerGame < 0 {
		return CorrelationPolicy{}, fmt.Errorf("max picks per game must be >= 0")
	}
	if maxPicksPerSportDay < 0 {
		return CorrelationPolicy{}, fmt.Errorf("max picks per sport/day must be >= 0")
	}
	if maxStakeFractionPerGame < 0 {
		return CorrelationPolicy{}, fmt.Errorf("max stake fraction per game must be >= 0")
	}

	if policy.MaxPicksPerGame < 1 || policy.MaxPicksPerGame > maxCorrelationMaxPicksPerGame {
		return CorrelationPolicy{}, fmt.Errorf("max picks per game must be in [1,%d]", maxCorrelationMaxPicksPerGame)
	}
	if math.IsNaN(policy.MaxStakeFractionPerGame) || math.IsInf(policy.MaxStakeFractionPerGame, 0) || policy.MaxStakeFractionPerGame <= 0 || policy.MaxStakeFractionPerGame > 1 {
		return CorrelationPolicy{}, fmt.Errorf("max stake fraction per game must be finite in (0,1]")
	}
	if policy.MaxPicksPerSportDay > maxCorrelationMaxPicksPerSportDay {
		return CorrelationPolicy{}, fmt.Errorf("max picks per sport/day must be in [0,%d]", maxCorrelationMaxPicksPerSportDay)
	}
	return policy, nil
}

func ApplyCorrelationGuard(recommendations []Recommendation, policy CorrelationPolicy) (CorrelationGuardResult, error) {
	if err := validateCorrelationPolicy(policy); err != nil {
		return CorrelationGuardResult{}, err
	}

	retained := make([]Recommendation, 0, len(recommendations))
	dropped := make([]Recommendation, 0, len(recommendations))

	type gameExposure struct {
		picks         int
		stakeFraction float64
	}
	gameExposureByKey := make(map[string]gameExposure)
	sportDayPicks := make(map[string]int)

	for i := range recommendations {
		rec := recommendations[i]
		rec.CorrelationGroupKey = correlationGroupKey(rec)
		rec.CorrelationCheckPass = true
		rec.CorrelationCheckReason = correlationReasonRetainedWithinLimits

		if rec.SuggestedStakeCents <= 0 {
			rec.CorrelationCheckReason = correlationReasonRetainedZeroStake
			retained = append(retained, rec)
			continue
		}

		if !isFinite(rec.SuggestedStakeFraction) || rec.SuggestedStakeFraction < 0 {
			rec.CorrelationCheckPass = false
			rec.CorrelationCheckReason = correlationReasonDroppedInvalidStake
			dropped = append(dropped, rec)
			continue
		}

		exposure := gameExposureByKey[rec.CorrelationGroupKey]
		if exposure.picks+1 > policy.MaxPicksPerGame {
			rec.CorrelationCheckPass = false
			rec.CorrelationCheckReason = correlationReasonDroppedMaxPicksPerGame
			dropped = append(dropped, rec)
			continue
		}

		projectedStake := exposure.stakeFraction + rec.SuggestedStakeFraction
		if projectedStake-policy.MaxStakeFractionPerGame > correlationStakeEpsilon {
			rec.CorrelationCheckPass = false
			rec.CorrelationCheckReason = correlationReasonDroppedMaxStakePerGame
			dropped = append(dropped, rec)
			continue
		}

		sportDayKey := correlationSportDayKey(rec.EventTime, rec.Sport)
		if policy.MaxPicksPerSportDay > 0 && sportDayPicks[sportDayKey]+1 > policy.MaxPicksPerSportDay {
			rec.CorrelationCheckPass = false
			rec.CorrelationCheckReason = correlationReasonDroppedMaxPicksPerSportDay
			dropped = append(dropped, rec)
			continue
		}

		exposure.picks++
		exposure.stakeFraction = projectedStake
		gameExposureByKey[rec.CorrelationGroupKey] = exposure
		sportDayPicks[sportDayKey]++

		retained = append(retained, rec)
	}

	return CorrelationGuardResult{
		Retained: retained,
		Dropped:  dropped,
	}, nil
}

func validateCorrelationPolicy(policy CorrelationPolicy) error {
	if policy.MaxPicksPerGame < 1 || policy.MaxPicksPerGame > maxCorrelationMaxPicksPerGame {
		return fmt.Errorf("max picks per game must be in [1,%d]", maxCorrelationMaxPicksPerGame)
	}
	if math.IsNaN(policy.MaxStakeFractionPerGame) || math.IsInf(policy.MaxStakeFractionPerGame, 0) || policy.MaxStakeFractionPerGame <= 0 || policy.MaxStakeFractionPerGame > 1 {
		return fmt.Errorf("max stake fraction per game must be finite in (0,1]")
	}
	if policy.MaxPicksPerSportDay < 0 || policy.MaxPicksPerSportDay > maxCorrelationMaxPicksPerSportDay {
		return fmt.Errorf("max picks per sport/day must be in [0,%d]", maxCorrelationMaxPicksPerSportDay)
	}
	return nil
}

func correlationGroupKey(rec Recommendation) string {
	return fmt.Sprintf("%s|%d", rec.Sport, rec.GameID)
}

func correlationSportDayKey(eventTime time.Time, sport domain.Sport) string {
	eventUTC := eventTime.UTC()
	return fmt.Sprintf("%v|%04d-%02d-%02d", sport, eventUTC.Year(), int(eventUTC.Month()), eventUTC.Day())
}
