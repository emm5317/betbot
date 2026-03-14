package features

import (
	"errors"
	"fmt"

	"betbot/internal/domain"
)

type NBABuilder struct {
	cfg       BuilderConfig
	sportMeta domain.SportConfig
}

func NewNBABuilder(cfg BuilderConfig) (NBABuilder, error) {
	if err := validateBuilderConfig(cfg); err != nil {
		return NBABuilder{}, fmt.Errorf("validate builder config: %w", err)
	}
	sportMeta, err := sportConfig(domain.SportNBA)
	if err != nil {
		return NBABuilder{}, err
	}
	return NBABuilder{cfg: cfg, sportMeta: sportMeta}, nil
}

func (b NBABuilder) Sport() domain.Sport {
	return domain.SportNBA
}

func (b NBABuilder) Build(req BuildRequest) (FeatureVector, error) {
	if req.Sport != domain.SportNBA {
		return FeatureVector{}, fmt.Errorf("expected sport %s, got %s", domain.SportNBA, req.Sport)
	}
	if req.NBA == nil {
		return FeatureVector{}, errors.New("nba context is required")
	}
	if err := validateNBAContext(*req.NBA); err != nil {
		return FeatureVector{}, fmt.Errorf("validate nba context: %w", err)
	}

	features := make([]Feature, 0, 32)
	features = append(features, marketPriorFeatures(req.Market, b.cfg, b.sportMeta.MarketAnchors)...)
	features = append(features, teamQualityFeatures(req.TeamQuality, b.cfg)...)
	features = append(features, situationalFeatures(req.Situational, b.cfg)...)
	features = append(features, injuryFeatures(req.Injuries)...)
	features = append(features, weatherFeatures(req.Weather, b.cfg)...)
	features = append(features, b.nbaSpecificFeatures(req)...)

	return FeatureVector{
		Sport:           domain.SportNBA,
		ModelFamily:     b.sportMeta.DefaultModelFamily,
		ManifestVersion: ManifestVersionV1,
		Features:        sortedFeatures(features),
	}, nil
}

func (b NBABuilder) nbaSpecificFeatures(req BuildRequest) []Feature {
	lineupEdge := clamp((req.NBA.HomeLineupNetRating-req.NBA.AwayLineupNetRating)/b.cfg.RatingScale, -1, 1)
	pace := clamp((req.NBA.ProjectedPace-b.cfg.NBAPaceBaseline)/b.cfg.PaceScale, -1, 1)
	backToBackEdge := signedBool(req.NBA.AwayBackToBack) - signedBool(req.NBA.HomeBackToBack)

	return []Feature{
		{Name: "nba_lineup_net_rating_edge_norm", Value: lineupEdge},
		{Name: "nba_projected_pace_norm", Value: pace},
		{Name: "nba_back_to_back_edge_norm", Value: backToBackEdge},
		{Name: "nba_rest_days_diff_norm", Value: clamp(float64(req.Situational.HomeRestDays-req.Situational.AwayRestDays)/3.0, -1, 1)},
	}
}

func validateNBAContext(in NBAContext) error {
	if !isFinite(in.HomeLineupNetRating) || in.HomeLineupNetRating < -40 || in.HomeLineupNetRating > 40 {
		return errors.New("home lineup net rating must be finite and in [-40,40]")
	}
	if !isFinite(in.AwayLineupNetRating) || in.AwayLineupNetRating < -40 || in.AwayLineupNetRating > 40 {
		return errors.New("away lineup net rating must be finite and in [-40,40]")
	}
	if !isFinite(in.ProjectedPace) || in.ProjectedPace < 80 || in.ProjectedPace > 120 {
		return errors.New("projected pace must be finite and in [80,120]")
	}
	return nil
}
