package features

import (
	"errors"
	"fmt"

	"betbot/internal/domain"
)

type NHLBuilder struct {
	cfg       BuilderConfig
	sportMeta domain.SportConfig
}

func NewNHLBuilder(cfg BuilderConfig) (NHLBuilder, error) {
	if err := validateBuilderConfig(cfg); err != nil {
		return NHLBuilder{}, fmt.Errorf("validate builder config: %w", err)
	}
	sportMeta, err := sportConfig(domain.SportNHL)
	if err != nil {
		return NHLBuilder{}, err
	}
	return NHLBuilder{cfg: cfg, sportMeta: sportMeta}, nil
}

func (b NHLBuilder) Sport() domain.Sport {
	return domain.SportNHL
}

func (b NHLBuilder) Build(req BuildRequest) (FeatureVector, error) {
	if req.Sport != domain.SportNHL {
		return FeatureVector{}, fmt.Errorf("expected sport %s, got %s", domain.SportNHL, req.Sport)
	}
	if req.NHL == nil {
		return FeatureVector{}, errors.New("nhl context is required")
	}
	if err := validateNHLContext(*req.NHL); err != nil {
		return FeatureVector{}, fmt.Errorf("validate nhl context: %w", err)
	}

	features := make([]Feature, 0, 32)
	features = append(features, marketPriorFeatures(req.Market, b.cfg, b.sportMeta.MarketAnchors)...)
	features = append(features, teamQualityFeatures(req.TeamQuality, b.cfg)...)
	features = append(features, situationalFeatures(req.Situational, b.cfg)...)
	features = append(features, injuryFeatures(req.Injuries)...)
	features = append(features, weatherFeatures(req.Weather, b.cfg)...)
	features = append(features, b.nhlSpecificFeatures(req)...)

	return FeatureVector{
		Sport:           domain.SportNHL,
		ModelFamily:     b.sportMeta.DefaultModelFamily,
		ManifestVersion: ManifestVersionV1,
		Features:        sortedFeatures(features),
	}, nil
}

func (b NHLBuilder) nhlSpecificFeatures(req BuildRequest) []Feature {
	goalieEdge := clamp((req.NHL.HomeGoalieGSAx-req.NHL.AwayGoalieGSAx)/b.cfg.GoalieGSAxScale, -1, 1)
	xgShareEdge := clamp((req.NHL.HomeXGShare-req.NHL.AwayXGShare)*2.0, -1, 1)
	homeRegressionPressure := clamp((b.cfg.PDOBaseline-req.NHL.HomePDO)/b.cfg.PDOScale, -1, 1)
	awayRegressionPressure := clamp((b.cfg.PDOBaseline-req.NHL.AwayPDO)/b.cfg.PDOScale, -1, 1)

	return []Feature{
		{Name: "nhl_goalie_gsax_edge_norm", Value: goalieEdge},
		{Name: "nhl_xg_share_edge_norm", Value: xgShareEdge},
		{Name: "nhl_home_pdo_regression_pressure_norm", Value: homeRegressionPressure},
		{Name: "nhl_away_pdo_regression_pressure_norm", Value: awayRegressionPressure},
		{Name: "nhl_pdo_regression_edge_norm", Value: clamp(homeRegressionPressure-awayRegressionPressure, -1, 1)},
	}
}

func validateNHLContext(in NHLContext) error {
	if !isFinite(in.HomeGoalieGSAx) || in.HomeGoalieGSAx < -50 || in.HomeGoalieGSAx > 50 {
		return errors.New("home goalie gsax must be finite and in [-50,50]")
	}
	if !isFinite(in.AwayGoalieGSAx) || in.AwayGoalieGSAx < -50 || in.AwayGoalieGSAx > 50 {
		return errors.New("away goalie gsax must be finite and in [-50,50]")
	}
	if !isFinite(in.HomeXGShare) || in.HomeXGShare < 0 || in.HomeXGShare > 1 {
		return errors.New("home xg share must be in [0,1]")
	}
	if !isFinite(in.AwayXGShare) || in.AwayXGShare < 0 || in.AwayXGShare > 1 {
		return errors.New("away xg share must be in [0,1]")
	}
	if !isFinite(in.HomePDO) || in.HomePDO < 0.85 || in.HomePDO > 1.15 {
		return errors.New("home pdo must be finite and in [0.85,1.15]")
	}
	if !isFinite(in.AwayPDO) || in.AwayPDO < 0.85 || in.AwayPDO > 1.15 {
		return errors.New("away pdo must be finite and in [0.85,1.15]")
	}
	return nil
}
