package features

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"betbot/internal/domain"
)

const calibrationEpsilon = 1e-9

type CalibrationSample struct {
	EventTime time.Time     `json:"event_time"`
	Vector    FeatureVector `json:"vector"`
	Outcome   float64       `json:"outcome"`
}

type WalkForwardConfig struct {
	TrainWindow      int `json:"train_window"`
	ValidationWindow int `json:"validation_window"`
	Step             int `json:"step"`
}

type CalibrationRequest struct {
	Sport       domain.Sport        `json:"sport"`
	ModelFamily string              `json:"model_family"`
	Samples     []CalibrationSample `json:"samples"`
	Window      WalkForwardConfig   `json:"window"`
	BaseConfig  BuilderConfig       `json:"base_config"`
}

type WalkForwardSplit struct {
	TrainStart      int `json:"train_start"`
	TrainEnd        int `json:"train_end"`
	ValidationStart int `json:"validation_start"`
	ValidationEnd   int `json:"validation_end"`
}

type FitDiagnostics struct {
	Samples          int     `json:"samples"`
	LogLoss          float64 `json:"log_loss"`
	BrierScore       float64 `json:"brier_score"`
	CalibrationError float64 `json:"calibration_error"`
}

type ScaleRecommendation struct {
	Parameter         string         `json:"parameter"`
	FeatureNames      []string       `json:"feature_names"`
	BaseScale         float64        `json:"base_scale"`
	RecommendedScale  float64        `json:"recommended_scale"`
	RecommendedWeight float64        `json:"recommended_weight"`
	Diagnostics       FitDiagnostics `json:"diagnostics"`
}

type WalkForwardSplitBounds struct {
	TrainStartUTC      time.Time `json:"train_start_utc"`
	TrainEndUTC        time.Time `json:"train_end_utc"`
	ValidationStartUTC time.Time `json:"validation_start_utc"`
	ValidationEndUTC   time.Time `json:"validation_end_utc"`
	TrainCount         int       `json:"train_count"`
	ValidationCount    int       `json:"validation_count"`
}

type CalibrationWindow struct {
	StartUTC         time.Time                `json:"start_utc"`
	EndUTC           time.Time                `json:"end_utc"`
	TrainWindow      int                      `json:"train_window"`
	ValidationWindow int                      `json:"validation_window"`
	Step             int                      `json:"step"`
	Splits           []WalkForwardSplitBounds `json:"splits"`
}

type CalibrationArtifact struct {
	GeneratedAtUTC  time.Time             `json:"generated_at_utc"`
	Sport           domain.Sport          `json:"sport"`
	ModelFamily     string                `json:"model_family"`
	ManifestVersion ManifestVersion       `json:"manifest_version"`
	InputWindow     CalibrationWindow     `json:"input_window"`
	Recommendations []ScaleRecommendation `json:"recommendations"`
	Diagnostics     FitDiagnostics        `json:"diagnostics"`
}

type calibrationParameterSpec struct {
	name      string
	baseScale float64
	features  []string
}

func BuildWalkForwardSplits(sampleCount int, cfg WalkForwardConfig) ([]WalkForwardSplit, error) {
	if cfg.TrainWindow <= 0 {
		return nil, errors.New("train window must be > 0")
	}
	if cfg.ValidationWindow <= 0 {
		return nil, errors.New("validation window must be > 0")
	}
	if cfg.Step <= 0 {
		return nil, errors.New("step must be > 0")
	}
	min := cfg.TrainWindow + cfg.ValidationWindow
	if sampleCount < min {
		return nil, fmt.Errorf("insufficient samples for walk-forward split: got %d, need at least %d", sampleCount, min)
	}

	splits := make([]WalkForwardSplit, 0)
	for start := 0; start+min <= sampleCount; start += cfg.Step {
		trainEnd := start + cfg.TrainWindow
		validationEnd := trainEnd + cfg.ValidationWindow
		splits = append(splits, WalkForwardSplit{
			TrainStart:      start,
			TrainEnd:        trainEnd,
			ValidationStart: trainEnd,
			ValidationEnd:   validationEnd,
		})
	}
	if len(splits) == 0 {
		return nil, errors.New("no walk-forward splits produced for the provided window configuration")
	}
	return splits, nil
}

func CalibrateNormalizationScales(req CalibrationRequest) (CalibrationArtifact, error) {
	if strings.TrimSpace(string(req.Sport)) == "" {
		return CalibrationArtifact{}, errors.New("sport is required")
	}

	cfg := req.BaseConfig
	if isZeroBuilderConfig(cfg) {
		cfg = DefaultBuilderConfig()
	}
	if err := validateBuilderConfig(cfg); err != nil {
		return CalibrationArtifact{}, fmt.Errorf("validate base config: %w", err)
	}

	modelFamily := strings.TrimSpace(req.ModelFamily)
	if modelFamily == "" {
		sportCfg, ok := domain.DefaultSportRegistry().Get(req.Sport)
		if !ok {
			return CalibrationArtifact{}, fmt.Errorf("unsupported sport %s", req.Sport)
		}
		modelFamily = sportCfg.DefaultModelFamily
	}

	manifest, err := ManifestFor(req.Sport, modelFamily)
	if err != nil {
		return CalibrationArtifact{}, fmt.Errorf("resolve feature manifest: %w", err)
	}

	samples, err := validateAndSortSamples(req.Samples, req.Sport, modelFamily, manifest)
	if err != nil {
		return CalibrationArtifact{}, fmt.Errorf("validate samples: %w", err)
	}

	splits, err := BuildWalkForwardSplits(len(samples), req.Window)
	if err != nil {
		return CalibrationArtifact{}, fmt.Errorf("build walk-forward splits: %w", err)
	}

	specs, err := calibrationSpecsForSport(req.Sport, cfg)
	if err != nil {
		return CalibrationArtifact{}, err
	}

	splitBounds := make([]WalkForwardSplitBounds, 0, len(splits))
	for _, split := range splits {
		splitBounds = append(splitBounds, WalkForwardSplitBounds{
			TrainStartUTC:      samples[split.TrainStart].EventTime.UTC(),
			TrainEndUTC:        samples[split.TrainEnd-1].EventTime.UTC(),
			ValidationStartUTC: samples[split.ValidationStart].EventTime.UTC(),
			ValidationEndUTC:   samples[split.ValidationEnd-1].EventTime.UTC(),
			TrainCount:         split.TrainEnd - split.TrainStart,
			ValidationCount:    split.ValidationEnd - split.ValidationStart,
		})
	}

	recommendations := make([]ScaleRecommendation, 0, len(specs))
	overallPredictions := make([]float64, 0, len(specs)*len(splits)*req.Window.ValidationWindow)
	overallOutcomes := make([]float64, 0, len(specs)*len(splits)*req.Window.ValidationWindow)

	for _, spec := range specs {
		totalWeight := 0.0
		weightedCoefficient := 0.0
		parameterPredictions := make([]float64, 0, len(splits)*req.Window.ValidationWindow)
		parameterOutcomes := make([]float64, 0, len(splits)*req.Window.ValidationWindow)

		for _, split := range splits {
			coefficient, err := fitCoefficient(samples[split.TrainStart:split.TrainEnd], spec)
			if err != nil {
				return CalibrationArtifact{}, fmt.Errorf("fit coefficient for %s: %w", spec.name, err)
			}

			for i := split.ValidationStart; i < split.ValidationEnd; i++ {
				signal, err := parameterSignal(samples[i].Vector, spec.features)
				if err != nil {
					return CalibrationArtifact{}, fmt.Errorf("compute validation signal for %s: %w", spec.name, err)
				}
				baseProb, ok := samples[i].Vector.Value("market_home_implied_prob")
				if !ok {
					return CalibrationArtifact{}, errors.New("missing market_home_implied_prob feature")
				}
				prediction := calibratedProbability(baseProb, signal, coefficient)
				parameterPredictions = append(parameterPredictions, prediction)
				parameterOutcomes = append(parameterOutcomes, samples[i].Outcome)
				overallPredictions = append(overallPredictions, prediction)
				overallOutcomes = append(overallOutcomes, samples[i].Outcome)
			}

			validationCount := float64(split.ValidationEnd - split.ValidationStart)
			weightedCoefficient += coefficient * validationCount
			totalWeight += validationCount
		}

		recommendedWeight := weightedCoefficient / totalWeight
		recommendedScale := spec.baseScale
		if recommendedWeight > calibrationEpsilon {
			recommendedScale = spec.baseScale / recommendedWeight
		}

		recommendations = append(recommendations, ScaleRecommendation{
			Parameter:         spec.name,
			FeatureNames:      append([]string(nil), spec.features...),
			BaseScale:         spec.baseScale,
			RecommendedScale:  recommendedScale,
			RecommendedWeight: recommendedWeight,
			Diagnostics:       computeDiagnostics(parameterPredictions, parameterOutcomes),
		})
	}

	generatedAt := samples[len(samples)-1].EventTime.UTC()
	if len(splitBounds) > 0 {
		generatedAt = splitBounds[len(splitBounds)-1].ValidationEndUTC
	}

	return CalibrationArtifact{
		GeneratedAtUTC:  generatedAt,
		Sport:           req.Sport,
		ModelFamily:     modelFamily,
		ManifestVersion: manifest.Version,
		InputWindow: CalibrationWindow{
			StartUTC:         samples[0].EventTime.UTC(),
			EndUTC:           samples[len(samples)-1].EventTime.UTC(),
			TrainWindow:      req.Window.TrainWindow,
			ValidationWindow: req.Window.ValidationWindow,
			Step:             req.Window.Step,
			Splits:           splitBounds,
		},
		Recommendations: recommendations,
		Diagnostics:     computeDiagnostics(overallPredictions, overallOutcomes),
	}, nil
}

func validateAndSortSamples(samples []CalibrationSample, sport domain.Sport, modelFamily string, manifest Manifest) ([]CalibrationSample, error) {
	if len(samples) == 0 {
		return nil, errors.New("samples cannot be empty")
	}

	sorted := append([]CalibrationSample(nil), samples...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].EventTime.Before(sorted[j].EventTime)
	})

	for i, sample := range sorted {
		if sample.EventTime.IsZero() {
			return nil, fmt.Errorf("sample %d event time is required", i)
		}
		if math.IsNaN(sample.Outcome) || math.IsInf(sample.Outcome, 0) || sample.Outcome < 0 || sample.Outcome > 1 {
			return nil, fmt.Errorf("sample %d outcome must be finite in [0,1]", i)
		}
		if err := validateFeatureVector(sample.Vector); err != nil {
			return nil, fmt.Errorf("sample %d vector validation: %w", i, err)
		}
		if sample.Vector.Sport != sport {
			return nil, fmt.Errorf("sample %d sport mismatch: got %s want %s", i, sample.Vector.Sport, sport)
		}
		if strings.TrimSpace(sample.Vector.ModelFamily) != modelFamily {
			return nil, fmt.Errorf("sample %d model family mismatch: got %s want %s", i, sample.Vector.ModelFamily, modelFamily)
		}
		if _, err := EncodeVector(sample.Vector, manifest); err != nil {
			return nil, fmt.Errorf("sample %d feature compatibility: %w", i, err)
		}
		if _, ok := sample.Vector.Value("market_home_implied_prob"); !ok {
			return nil, fmt.Errorf("sample %d missing market_home_implied_prob", i)
		}
	}

	return sorted, nil
}

func fitCoefficient(train []CalibrationSample, spec calibrationParameterSpec) (float64, error) {
	if len(train) == 0 {
		return 0, errors.New("training window cannot be empty")
	}

	candidates := []float64{0.25, 0.5, 0.75, 1.0, 1.25, 1.5, 2.0, 2.5, 3.0}
	best := candidates[0]
	bestLoss := math.MaxFloat64

	for _, candidate := range candidates {
		predictions := make([]float64, 0, len(train))
		outcomes := make([]float64, 0, len(train))
		for _, sample := range train {
			signal, err := parameterSignal(sample.Vector, spec.features)
			if err != nil {
				return 0, err
			}
			baseProb, ok := sample.Vector.Value("market_home_implied_prob")
			if !ok {
				return 0, errors.New("missing market_home_implied_prob feature")
			}
			predictions = append(predictions, calibratedProbability(baseProb, signal, candidate))
			outcomes = append(outcomes, sample.Outcome)
		}

		loss := computeDiagnostics(predictions, outcomes).LogLoss
		if loss < bestLoss-calibrationEpsilon || (math.Abs(loss-bestLoss) <= calibrationEpsilon && candidate < best) {
			bestLoss = loss
			best = candidate
		}
	}

	return best, nil
}

func parameterSignal(vector FeatureVector, names []string) (float64, error) {
	if len(names) == 0 {
		return 0, errors.New("parameter feature list cannot be empty")
	}
	total := 0.0
	for _, name := range names {
		value, ok := vector.Value(name)
		if !ok {
			return 0, fmt.Errorf("missing feature %s", name)
		}
		total += value
	}
	return total / float64(len(names)), nil
}

func calibratedProbability(baseProb, signal, coefficient float64) float64 {
	base := clamp(baseProb, calibrationEpsilon, 1-calibrationEpsilon)
	logit := math.Log(base / (1 - base))
	raw := 1 / (1 + math.Exp(-(logit + coefficient*signal)))
	return clamp(raw, calibrationEpsilon, 1-calibrationEpsilon)
}

func computeDiagnostics(predictions, outcomes []float64) FitDiagnostics {
	if len(predictions) == 0 || len(predictions) != len(outcomes) {
		return FitDiagnostics{}
	}

	logLoss := 0.0
	brier := 0.0
	for i := range predictions {
		p := clamp(predictions[i], calibrationEpsilon, 1-calibrationEpsilon)
		y := outcomes[i]
		logLoss += -(y*math.Log(p) + (1-y)*math.Log(1-p))
		delta := p - y
		brier += delta * delta
	}

	n := float64(len(predictions))
	return FitDiagnostics{
		Samples:          len(predictions),
		LogLoss:          logLoss / n,
		BrierScore:       brier / n,
		CalibrationError: expectedCalibrationError(predictions, outcomes, 10),
	}
}

func expectedCalibrationError(predictions, outcomes []float64, bins int) float64 {
	if len(predictions) == 0 || len(predictions) != len(outcomes) || bins <= 0 {
		return 0
	}

	type binStats struct {
		count int
		sumP  float64
		sumY  float64
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
		stats[index].sumY += outcomes[i]
	}

	total := float64(len(predictions))
	ece := 0.0
	for _, bin := range stats {
		if bin.count == 0 {
			continue
		}
		avgP := bin.sumP / float64(bin.count)
		avgY := bin.sumY / float64(bin.count)
		ece += (float64(bin.count) / total) * math.Abs(avgP-avgY)
	}

	return ece
}

func calibrationSpecsForSport(sport domain.Sport, cfg BuilderConfig) ([]calibrationParameterSpec, error) {
	specs := []calibrationParameterSpec{
		{name: "spread_scale", baseScale: cfg.SpreadScale, features: []string{"market_home_spread_norm"}},
		{name: "total_scale", baseScale: cfg.TotalScale, features: []string{"market_total_points_norm"}},
		{name: "travel_scale", baseScale: cfg.TravelScale, features: []string{"situational_travel_edge_norm"}},
	}

	ratingFeatures := []string{
		"team_quality_power_rating_diff_norm",
		"team_quality_offense_diff_norm",
		"team_quality_defense_diff_norm",
		"team_quality_net_rating_diff_norm",
	}
	if sport == domain.SportNBA {
		ratingFeatures = append(ratingFeatures, "nba_lineup_net_rating_edge_norm")
	}
	specs = append(specs, calibrationParameterSpec{name: "rating_scale", baseScale: cfg.RatingScale, features: ratingFeatures})

	switch sport {
	case domain.SportMLB:
		specs = append(specs,
			calibrationParameterSpec{name: "starter_era_scale", baseScale: cfg.StarterERAScale, features: []string{"mlb_starter_era_edge_norm", "mlb_bullpen_era_edge_norm"}},
			calibrationParameterSpec{name: "park_factor_scale", baseScale: cfg.ParkFactorScale, features: []string{"mlb_park_run_factor_norm"}},
		)
	case domain.SportNBA:
		specs = append(specs,
			calibrationParameterSpec{name: "pace_scale", baseScale: cfg.PaceScale, features: []string{"nba_projected_pace_norm"}},
		)
	case domain.SportNHL:
		specs = append(specs,
			calibrationParameterSpec{name: "goalie_gsax_scale", baseScale: cfg.GoalieGSAxScale, features: []string{"nhl_goalie_gsax_edge_norm"}},
			calibrationParameterSpec{name: "pdo_scale", baseScale: cfg.PDOScale, features: []string{"nhl_home_pdo_regression_pressure_norm", "nhl_away_pdo_regression_pressure_norm", "nhl_pdo_regression_edge_norm"}},
		)
	case domain.SportNFL:
		specs = append(specs,
			calibrationParameterSpec{name: "qb_epa_scale", baseScale: cfg.QBEPAScale, features: []string{"nfl_qb_epa_edge_norm"}},
			calibrationParameterSpec{name: "dvoa_scale", baseScale: cfg.DVOAScale, features: []string{"nfl_dvoa_edge_norm"}},
			calibrationParameterSpec{name: "key_number_scale", baseScale: cfg.KeyNumberScale, features: []string{"nfl_key_number_proximity", "market_key_number_proximity"}},
		)
	default:
		return nil, fmt.Errorf("unsupported sport %s", sport)
	}

	return specs, nil
}

func isZeroBuilderConfig(cfg BuilderConfig) bool {
	return cfg == (BuilderConfig{})
}
