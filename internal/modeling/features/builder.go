package features

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"betbot/internal/domain"
)

// Feature is a deterministic numeric value keyed by name.
type Feature struct {
	Name  string
	Value float64
}

// FeatureVector is the typed output produced by a sport-specific feature builder.
type FeatureVector struct {
	Sport       domain.Sport
	ModelFamily string
	Features    []Feature
}

func (v FeatureVector) Value(name string) (float64, bool) {
	for _, feature := range v.Features {
		if feature.Name == name {
			return feature.Value, true
		}
	}
	return 0, false
}

// Builder defines the shared contract for sport-specific feature generation.
type Builder interface {
	Sport() domain.Sport
	Build(req BuildRequest) (FeatureVector, error)
}

// Registry dispatches feature builds to the correct sport-specific builder.
type Registry struct {
	builders map[domain.Sport]Builder
}

func NewRegistry(builders ...Builder) (Registry, error) {
	if len(builders) == 0 {
		return Registry{}, errors.New("at least one builder is required")
	}

	mapped := make(map[domain.Sport]Builder, len(builders))
	for _, builder := range builders {
		if builder == nil {
			return Registry{}, errors.New("builder cannot be nil")
		}
		sport := builder.Sport()
		if strings.TrimSpace(string(sport)) == "" {
			return Registry{}, errors.New("builder sport cannot be empty")
		}
		if _, exists := mapped[sport]; exists {
			return Registry{}, fmt.Errorf("duplicate builder for sport %s", sport)
		}
		mapped[sport] = builder
	}

	return Registry{builders: mapped}, nil
}

func NewDefaultRegistry() (Registry, error) {
	cfg := DefaultBuilderConfig()

	mlbBuilder, err := NewMLBBuilder(cfg)
	if err != nil {
		return Registry{}, fmt.Errorf("create mlb builder: %w", err)
	}
	nbaBuilder, err := NewNBABuilder(cfg)
	if err != nil {
		return Registry{}, fmt.Errorf("create nba builder: %w", err)
	}
	nhlBuilder, err := NewNHLBuilder(cfg)
	if err != nil {
		return Registry{}, fmt.Errorf("create nhl builder: %w", err)
	}
	nflBuilder, err := NewNFLBuilder(cfg)
	if err != nil {
		return Registry{}, fmt.Errorf("create nfl builder: %w", err)
	}

	return NewRegistry(mlbBuilder, nbaBuilder, nhlBuilder, nflBuilder)
}

func (r Registry) Build(req BuildRequest) (FeatureVector, error) {
	if err := validateCommonRequest(req); err != nil {
		return FeatureVector{}, fmt.Errorf("validate request: %w", err)
	}

	builder, ok := r.builders[req.Sport]
	if !ok {
		return FeatureVector{}, fmt.Errorf("no feature builder registered for sport %s", req.Sport)
	}

	vector, err := builder.Build(req)
	if err != nil {
		return FeatureVector{}, fmt.Errorf("build features for sport %s: %w", req.Sport, err)
	}

	if err := validateFeatureVector(vector); err != nil {
		return FeatureVector{}, fmt.Errorf("validate feature vector: %w", err)
	}

	return vector, nil
}

type BuildRequest struct {
	Sport       domain.Sport
	Market      MarketInputs
	TeamQuality TeamQualityInputs
	Situational SituationalInputs
	Injuries    InjuryInputs
	Weather     WeatherInputs

	MLB *MLBContext
	NBA *NBAContext
	NHL *NHLContext
	NFL *NFLContext
}

type MarketInputs struct {
	HomeMoneylineProbability float64
	AwayMoneylineProbability float64
	HomeSpread               float64
	TotalPoints              float64
}

type TeamQualityInputs struct {
	HomePowerRating   float64
	AwayPowerRating   float64
	HomeOffenseRating float64
	AwayOffenseRating float64
	HomeDefenseRating float64
	AwayDefenseRating float64
}

type SituationalInputs struct {
	HomeRestDays    int
	AwayRestDays    int
	HomeTravelMiles float64
	AwayTravelMiles float64
	HomeGamesLast7  int
	AwayGamesLast7  int
	NeutralSiteGame bool
}

type InjuryInputs struct {
	HomeAvailability float64
	AwayAvailability float64
}

type WeatherInputs struct {
	TemperatureF    float64
	WindMPH         float64
	PrecipitationMM float64
	IsDome          bool
}

type MLBContext struct {
	HomeStarterERA float64
	AwayStarterERA float64
	HomeBullpenERA float64
	AwayBullpenERA float64
	ParkFactor     float64
}

type NBAContext struct {
	HomeLineupNetRating float64
	AwayLineupNetRating float64
	ProjectedPace       float64
	HomeBackToBack      bool
	AwayBackToBack      bool
}

type NHLContext struct {
	HomeGoalieGSAx float64
	AwayGoalieGSAx float64
	HomeXGShare    float64
	AwayXGShare    float64
	HomePDO        float64
	AwayPDO        float64
}

type NFLContext struct {
	HomeQBEPA        float64
	AwayQBEPA        float64
	HomeDVOA         float64
	AwayDVOA         float64
	PrimaryKeyNumber float64
}

type BuilderConfig struct {
	SpreadScale        float64
	TotalScale         float64
	TotalBaseline      float64
	RatingScale        float64
	TravelScale        float64
	StarterERAScale    float64
	ParkFactorScale    float64
	NBAPaceBaseline    float64
	PaceScale          float64
	GoalieGSAxScale    float64
	PDOBaseline        float64
	PDOScale           float64
	QBEPAScale         float64
	DVOAScale          float64
	KeyNumberScale     float64
	TemperatureScale   float64
	WindScale          float64
	PrecipitationScale float64
}

func DefaultBuilderConfig() BuilderConfig {
	return BuilderConfig{
		SpreadScale:        14.0,
		TotalScale:         35.0,
		TotalBaseline:      45.0,
		RatingScale:        20.0,
		TravelScale:        2000.0,
		StarterERAScale:    2.0,
		ParkFactorScale:    0.25,
		NBAPaceBaseline:    99.0,
		PaceScale:          10.0,
		GoalieGSAxScale:    15.0,
		PDOBaseline:        1.0,
		PDOScale:           0.08,
		QBEPAScale:         0.35,
		DVOAScale:          0.35,
		KeyNumberScale:     4.0,
		TemperatureScale:   35.0,
		WindScale:          35.0,
		PrecipitationScale: 25.0,
	}
}

func validateBuilderConfig(cfg BuilderConfig) error {
	checks := map[string]float64{
		"spread scale":        cfg.SpreadScale,
		"total scale":         cfg.TotalScale,
		"rating scale":        cfg.RatingScale,
		"travel scale":        cfg.TravelScale,
		"starter era scale":   cfg.StarterERAScale,
		"park factor scale":   cfg.ParkFactorScale,
		"nba pace baseline":   cfg.NBAPaceBaseline,
		"pace scale":          cfg.PaceScale,
		"goalie gsax scale":   cfg.GoalieGSAxScale,
		"pdo baseline":        cfg.PDOBaseline,
		"pdo scale":           cfg.PDOScale,
		"qb epa scale":        cfg.QBEPAScale,
		"dvoa scale":          cfg.DVOAScale,
		"key number scale":    cfg.KeyNumberScale,
		"temperature scale":   cfg.TemperatureScale,
		"wind scale":          cfg.WindScale,
		"precipitation scale": cfg.PrecipitationScale,
	}
	for name, value := range checks {
		if !isFinite(value) || value <= 0 {
			return fmt.Errorf("%s must be finite and > 0", name)
		}
	}

	if !isFinite(cfg.TotalBaseline) || cfg.TotalBaseline <= 0 {
		return errors.New("total baseline must be finite and > 0")
	}

	return nil
}

func validateCommonRequest(req BuildRequest) error {
	if strings.TrimSpace(string(req.Sport)) == "" {
		return errors.New("sport is required")
	}

	if err := validateMarketInputs(req.Market); err != nil {
		return fmt.Errorf("market inputs: %w", err)
	}
	if err := validateTeamQualityInputs(req.TeamQuality); err != nil {
		return fmt.Errorf("team quality inputs: %w", err)
	}
	if err := validateSituationalInputs(req.Situational); err != nil {
		return fmt.Errorf("situational inputs: %w", err)
	}
	if err := validateInjuryInputs(req.Injuries); err != nil {
		return fmt.Errorf("injury inputs: %w", err)
	}
	if err := validateWeatherInputs(req.Weather); err != nil {
		return fmt.Errorf("weather inputs: %w", err)
	}

	return nil
}

func validateMarketInputs(in MarketInputs) error {
	if !isFinite(in.HomeMoneylineProbability) || in.HomeMoneylineProbability < 0 || in.HomeMoneylineProbability > 1 {
		return errors.New("home moneyline probability must be in [0,1]")
	}
	if !isFinite(in.AwayMoneylineProbability) || in.AwayMoneylineProbability < 0 || in.AwayMoneylineProbability > 1 {
		return errors.New("away moneyline probability must be in [0,1]")
	}
	if math.Abs((in.HomeMoneylineProbability+in.AwayMoneylineProbability)-1.0) > 1e-6 {
		return errors.New("home and away moneyline probabilities must sum to 1")
	}
	if !isFinite(in.HomeSpread) || math.Abs(in.HomeSpread) > 40 {
		return errors.New("home spread must be finite and within [-40,40]")
	}
	if !isFinite(in.TotalPoints) || in.TotalPoints <= 0 || in.TotalPoints > 400 {
		return errors.New("total points must be finite and in (0,400]")
	}
	return nil
}

func validateTeamQualityInputs(in TeamQualityInputs) error {
	values := map[string]float64{
		"home power rating":   in.HomePowerRating,
		"away power rating":   in.AwayPowerRating,
		"home offense rating": in.HomeOffenseRating,
		"away offense rating": in.AwayOffenseRating,
		"home defense rating": in.HomeDefenseRating,
		"away defense rating": in.AwayDefenseRating,
	}
	for name, value := range values {
		if !isFinite(value) || value < 0 || value > 200 {
			return fmt.Errorf("%s must be finite and in [0,200]", name)
		}
	}
	return nil
}

func validateSituationalInputs(in SituationalInputs) error {
	if in.HomeRestDays < 0 || in.HomeRestDays > 14 {
		return errors.New("home rest days must be in [0,14]")
	}
	if in.AwayRestDays < 0 || in.AwayRestDays > 14 {
		return errors.New("away rest days must be in [0,14]")
	}
	if !isFinite(in.HomeTravelMiles) || in.HomeTravelMiles < 0 || in.HomeTravelMiles > 10000 {
		return errors.New("home travel miles must be finite and in [0,10000]")
	}
	if !isFinite(in.AwayTravelMiles) || in.AwayTravelMiles < 0 || in.AwayTravelMiles > 10000 {
		return errors.New("away travel miles must be finite and in [0,10000]")
	}
	if in.HomeGamesLast7 < 0 || in.HomeGamesLast7 > 7 {
		return errors.New("home games last 7 must be in [0,7]")
	}
	if in.AwayGamesLast7 < 0 || in.AwayGamesLast7 > 7 {
		return errors.New("away games last 7 must be in [0,7]")
	}
	return nil
}

func validateInjuryInputs(in InjuryInputs) error {
	if !isFinite(in.HomeAvailability) || in.HomeAvailability < 0 || in.HomeAvailability > 1 {
		return errors.New("home availability must be in [0,1]")
	}
	if !isFinite(in.AwayAvailability) || in.AwayAvailability < 0 || in.AwayAvailability > 1 {
		return errors.New("away availability must be in [0,1]")
	}
	return nil
}

func validateWeatherInputs(in WeatherInputs) error {
	if !isFinite(in.TemperatureF) || in.TemperatureF < -60 || in.TemperatureF > 140 {
		return errors.New("temperature must be finite and in [-60,140]")
	}
	if !isFinite(in.WindMPH) || in.WindMPH < 0 || in.WindMPH > 150 {
		return errors.New("wind mph must be finite and in [0,150]")
	}
	if !isFinite(in.PrecipitationMM) || in.PrecipitationMM < 0 || in.PrecipitationMM > 500 {
		return errors.New("precipitation mm must be finite and in [0,500]")
	}
	return nil
}

func validateFeatureVector(v FeatureVector) error {
	if strings.TrimSpace(string(v.Sport)) == "" {
		return errors.New("feature vector sport is required")
	}
	if strings.TrimSpace(v.ModelFamily) == "" {
		return errors.New("feature vector model family is required")
	}
	if len(v.Features) == 0 {
		return errors.New("feature vector cannot be empty")
	}

	names := make(map[string]struct{}, len(v.Features))
	for _, feature := range v.Features {
		if strings.TrimSpace(feature.Name) == "" {
			return errors.New("feature name cannot be empty")
		}
		if _, exists := names[feature.Name]; exists {
			return fmt.Errorf("duplicate feature name %q", feature.Name)
		}
		if !isFinite(feature.Value) {
			return fmt.Errorf("feature %q has non-finite value", feature.Name)
		}
		names[feature.Name] = struct{}{}
	}

	return nil
}

func sortedFeatures(features []Feature) []Feature {
	out := append([]Feature(nil), features...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func sportConfig(sport domain.Sport) (domain.SportConfig, error) {
	cfg, ok := domain.DefaultSportRegistry().Get(sport)
	if !ok {
		return domain.SportConfig{}, fmt.Errorf("unsupported sport %s", sport)
	}
	return cfg, nil
}

func signedBool(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
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
