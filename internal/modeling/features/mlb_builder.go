package features

import (
	"errors"
	"fmt"

	"betbot/internal/domain"
)

type MLBBuilder struct {
	cfg       BuilderConfig
	sportMeta domain.SportConfig
}

func NewMLBBuilder(cfg BuilderConfig) (MLBBuilder, error) {
	if err := validateBuilderConfig(cfg); err != nil {
		return MLBBuilder{}, fmt.Errorf("validate builder config: %w", err)
	}
	sportMeta, err := sportConfig(domain.SportMLB)
	if err != nil {
		return MLBBuilder{}, err
	}
	return MLBBuilder{cfg: cfg, sportMeta: sportMeta}, nil
}

func (b MLBBuilder) Sport() domain.Sport {
	return domain.SportMLB
}

func (b MLBBuilder) Build(req BuildRequest) (FeatureVector, error) {
	if req.Sport != domain.SportMLB {
		return FeatureVector{}, fmt.Errorf("expected sport %s, got %s", domain.SportMLB, req.Sport)
	}
	if req.MLB == nil {
		return FeatureVector{}, errors.New("mlb context is required")
	}
	if err := validateMLBContext(*req.MLB); err != nil {
		return FeatureVector{}, fmt.Errorf("validate mlb context: %w", err)
	}

	features := make([]Feature, 0, 32)
	features = append(features, marketPriorFeatures(req.Market, b.cfg, b.sportMeta.MarketAnchors)...)
	features = append(features, teamQualityFeatures(req.TeamQuality, b.cfg)...)
	features = append(features, situationalFeatures(req.Situational, b.cfg)...)
	features = append(features, injuryFeatures(req.Injuries)...)
	features = append(features, weatherFeatures(req.Weather, b.cfg)...)
	features = append(features, b.mlbSpecificFeatures(req)...)

	return FeatureVector{
		Sport:           domain.SportMLB,
		ModelFamily:     b.sportMeta.DefaultModelFamily,
		ManifestVersion: ManifestVersionV1,
		Features:        sortedFeatures(features),
	}, nil
}

func (b MLBBuilder) mlbSpecificFeatures(req BuildRequest) []Feature {
	homeStarterEdge := clamp((req.MLB.AwayStarterERA-req.MLB.HomeStarterERA)/b.cfg.StarterERAScale, -1, 1)
	homeBullpenEdge := clamp((req.MLB.AwayBullpenERA-req.MLB.HomeBullpenERA)/b.cfg.StarterERAScale, -1, 1)
	parkRunFactor := clamp((req.MLB.ParkFactor-1.0)/b.cfg.ParkFactorScale, -1, 1)
	weatherRunDrag := -featureValue(weatherFeatures(req.Weather, b.cfg), "weather_severity_norm")

	return []Feature{
		{Name: "mlb_starter_era_edge_norm", Value: homeStarterEdge},
		{Name: "mlb_bullpen_era_edge_norm", Value: homeBullpenEdge},
		{Name: "mlb_park_run_factor_norm", Value: parkRunFactor},
		{Name: "mlb_weather_run_drag_norm", Value: weatherRunDrag},
	}
}

func validateMLBContext(in MLBContext) error {
	if !isFinite(in.HomeStarterERA) || in.HomeStarterERA <= 0 || in.HomeStarterERA > 15 {
		return errors.New("home starter era must be finite and in (0,15]")
	}
	if !isFinite(in.AwayStarterERA) || in.AwayStarterERA <= 0 || in.AwayStarterERA > 15 {
		return errors.New("away starter era must be finite and in (0,15]")
	}
	if !isFinite(in.HomeBullpenERA) || in.HomeBullpenERA <= 0 || in.HomeBullpenERA > 15 {
		return errors.New("home bullpen era must be finite and in (0,15]")
	}
	if !isFinite(in.AwayBullpenERA) || in.AwayBullpenERA <= 0 || in.AwayBullpenERA > 15 {
		return errors.New("away bullpen era must be finite and in (0,15]")
	}
	if !isFinite(in.ParkFactor) || in.ParkFactor < 0.5 || in.ParkFactor > 1.5 {
		return errors.New("park factor must be finite and in [0.5,1.5]")
	}
	return nil
}

func featureValue(features []Feature, name string) float64 {
	for _, feature := range features {
		if feature.Name == name {
			return feature.Value
		}
	}
	return 0
}
