package features

import (
	"errors"
	"fmt"
	"math"

	"betbot/internal/domain"
)

type NFLBuilder struct {
	cfg       BuilderConfig
	sportMeta domain.SportConfig
}

func NewNFLBuilder(cfg BuilderConfig) (NFLBuilder, error) {
	if err := validateBuilderConfig(cfg); err != nil {
		return NFLBuilder{}, fmt.Errorf("validate builder config: %w", err)
	}
	sportMeta, err := sportConfig(domain.SportNFL)
	if err != nil {
		return NFLBuilder{}, err
	}
	return NFLBuilder{cfg: cfg, sportMeta: sportMeta}, nil
}

func (b NFLBuilder) Sport() domain.Sport {
	return domain.SportNFL
}

func (b NFLBuilder) Build(req BuildRequest) (FeatureVector, error) {
	if req.Sport != domain.SportNFL {
		return FeatureVector{}, fmt.Errorf("expected sport %s, got %s", domain.SportNFL, req.Sport)
	}
	if req.NFL == nil {
		return FeatureVector{}, errors.New("nfl context is required")
	}
	if err := validateNFLContext(*req.NFL); err != nil {
		return FeatureVector{}, fmt.Errorf("validate nfl context: %w", err)
	}

	features := make([]Feature, 0, 32)
	features = append(features, marketPriorFeatures(req.Market, b.cfg, b.sportMeta.MarketAnchors)...)
	features = append(features, teamQualityFeatures(req.TeamQuality, b.cfg)...)
	features = append(features, situationalFeatures(req.Situational, b.cfg)...)
	features = append(features, injuryFeatures(req.Injuries)...)
	features = append(features, weatherFeatures(req.Weather, b.cfg)...)
	features = append(features, b.nflSpecificFeatures(req)...)

	return FeatureVector{
		Sport:       domain.SportNFL,
		ModelFamily: b.sportMeta.DefaultModelFamily,
		Features:    sortedFeatures(features),
	}, nil
}

func (b NFLBuilder) nflSpecificFeatures(req BuildRequest) []Feature {
	qbEPAEdge := clamp((req.NFL.HomeQBEPA-req.NFL.AwayQBEPA)/b.cfg.QBEPAScale, -1, 1)
	dvoaEdge := clamp((req.NFL.HomeDVOA-req.NFL.AwayDVOA)/b.cfg.DVOAScale, -1, 1)
	keyGap := math.Abs(math.Abs(req.Market.HomeSpread) - req.NFL.PrimaryKeyNumber)
	keyProximity := 1 - clamp(keyGap/b.cfg.KeyNumberScale, 0, 1)

	weather := weatherFeatures(req.Weather, b.cfg)
	windPenalty := featureValue(weather, "weather_wind_penalty_norm")

	return []Feature{
		{Name: "nfl_qb_epa_edge_norm", Value: qbEPAEdge},
		{Name: "nfl_dvoa_edge_norm", Value: dvoaEdge},
		{Name: "nfl_key_number_proximity", Value: keyProximity},
		{Name: "nfl_wind_penalty_norm", Value: windPenalty},
	}
}

func validateNFLContext(in NFLContext) error {
	if !isFinite(in.HomeQBEPA) || in.HomeQBEPA < -1 || in.HomeQBEPA > 1 {
		return errors.New("home qb epa must be finite and in [-1,1]")
	}
	if !isFinite(in.AwayQBEPA) || in.AwayQBEPA < -1 || in.AwayQBEPA > 1 {
		return errors.New("away qb epa must be finite and in [-1,1]")
	}
	if !isFinite(in.HomeDVOA) || in.HomeDVOA < -1 || in.HomeDVOA > 1 {
		return errors.New("home dvoa must be finite and in [-1,1]")
	}
	if !isFinite(in.AwayDVOA) || in.AwayDVOA < -1 || in.AwayDVOA > 1 {
		return errors.New("away dvoa must be finite and in [-1,1]")
	}
	if !isFinite(in.PrimaryKeyNumber) || in.PrimaryKeyNumber <= 0 || in.PrimaryKeyNumber > 21 {
		return errors.New("primary key number must be finite and in (0,21]")
	}
	return nil
}
