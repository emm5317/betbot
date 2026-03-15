package features

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"betbot/internal/domain"
)

type ManifestVersion string

const ManifestVersionV1 ManifestVersion = "v1"

type Manifest struct {
	Version     ManifestVersion `json:"version"`
	Sport       domain.Sport    `json:"sport"`
	ModelFamily string          `json:"model_family"`
	Features    []string        `json:"features"`
}

type manifestKey struct {
	version     ManifestVersion
	sport       domain.Sport
	modelFamily string
}

type compiledManifest struct {
	manifest Manifest
	index    map[string]int
}

var compiledManifests = buildCompiledManifests()

func ManifestFor(sport domain.Sport, modelFamily string) (Manifest, error) {
	return ManifestForVersion(ManifestVersionV1, sport, modelFamily)
}

func ManifestForVersion(version ManifestVersion, sport domain.Sport, modelFamily string) (Manifest, error) {
	if strings.TrimSpace(string(version)) == "" {
		return Manifest{}, errors.New("manifest version is required")
	}
	if version != ManifestVersionV1 {
		return Manifest{}, fmt.Errorf("unsupported manifest version %q", version)
	}
	if strings.TrimSpace(string(sport)) == "" {
		return Manifest{}, errors.New("sport is required")
	}
	modelFamily = strings.TrimSpace(modelFamily)
	if modelFamily == "" {
		cfg, ok := domain.DefaultSportRegistry().Get(sport)
		if !ok {
			return Manifest{}, fmt.Errorf("unsupported sport %s", sport)
		}
		modelFamily = cfg.DefaultModelFamily
	}

	key := manifestKey{version: version, sport: sport, modelFamily: modelFamily}
	compiled, ok := compiledManifests[key]
	if !ok {
		return Manifest{}, fmt.Errorf("no feature manifest for version=%s sport=%s modelFamily=%s", version, sport, modelFamily)
	}

	out := compiled.manifest
	out.Features = append([]string(nil), out.Features...)
	return out, nil
}

func EncodeVector(vector FeatureVector, manifest Manifest) ([]float64, error) {
	compiled, err := compileManifest(manifest)
	if err != nil {
		return nil, fmt.Errorf("compile manifest: %w", err)
	}
	if err := validateVectorManifestHeaders(vector, compiled.manifest); err != nil {
		return nil, err
	}

	values := make([]float64, len(compiled.manifest.Features))
	seen := make(map[string]struct{}, len(vector.Features))
	extra := make([]string, 0)
	duplicates := make([]string, 0)

	for _, feature := range vector.Features {
		idx, ok := compiled.index[feature.Name]
		if !ok {
			extra = append(extra, feature.Name)
			continue
		}
		if _, exists := seen[feature.Name]; exists {
			duplicates = append(duplicates, feature.Name)
			continue
		}
		values[idx] = feature.Value
		seen[feature.Name] = struct{}{}
	}

	missing := make([]string, 0)
	for _, name := range compiled.manifest.Features {
		if _, ok := seen[name]; !ok {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 || len(extra) > 0 || len(duplicates) > 0 {
		return nil, fmt.Errorf(
			"feature set mismatch for manifest version=%s sport=%s modelFamily=%s: missing=%v extra=%v duplicates=%v",
			compiled.manifest.Version,
			compiled.manifest.Sport,
			compiled.manifest.ModelFamily,
			uniqueSorted(missing),
			uniqueSorted(extra),
			uniqueSorted(duplicates),
		)
	}

	return values, nil
}

func DecodeVector(values []float64, manifest Manifest) ([]Feature, error) {
	compiled, err := compileManifest(manifest)
	if err != nil {
		return nil, fmt.Errorf("compile manifest: %w", err)
	}
	if len(values) != len(compiled.manifest.Features) {
		return nil, fmt.Errorf("vector length mismatch: got %d, want %d", len(values), len(compiled.manifest.Features))
	}

	out := make([]Feature, len(values))
	for i, value := range values {
		out[i] = Feature{Name: compiled.manifest.Features[i], Value: value}
	}

	return out, nil
}

func ValidateVectorMatchesManifest(vector FeatureVector, manifest Manifest) error {
	compiled, err := compileManifest(manifest)
	if err != nil {
		return fmt.Errorf("compile manifest: %w", err)
	}
	if err := validateVectorManifestHeaders(vector, compiled.manifest); err != nil {
		return err
	}
	if len(vector.Features) != len(compiled.manifest.Features) {
		return fmt.Errorf("feature length mismatch: got %d, want %d", len(vector.Features), len(compiled.manifest.Features))
	}

	for i := range compiled.manifest.Features {
		got := vector.Features[i].Name
		want := compiled.manifest.Features[i]
		if got != want {
			return fmt.Errorf("feature order mismatch at index %d: got %q, want %q", i, got, want)
		}
	}

	return nil
}

func validateVectorManifestHeaders(vector FeatureVector, manifest Manifest) error {
	if vector.ManifestVersion != manifest.Version {
		return fmt.Errorf("manifest version mismatch: vector=%s manifest=%s", vector.ManifestVersion, manifest.Version)
	}
	if vector.Sport != manifest.Sport {
		return fmt.Errorf("sport mismatch: vector=%s manifest=%s", vector.Sport, manifest.Sport)
	}
	if strings.TrimSpace(vector.ModelFamily) != manifest.ModelFamily {
		return fmt.Errorf("model family mismatch: vector=%s manifest=%s", vector.ModelFamily, manifest.ModelFamily)
	}
	return nil
}

func compileManifest(manifest Manifest) (compiledManifest, error) {
	if strings.TrimSpace(string(manifest.Version)) == "" {
		return compiledManifest{}, errors.New("manifest version is required")
	}
	if strings.TrimSpace(string(manifest.Sport)) == "" {
		return compiledManifest{}, errors.New("manifest sport is required")
	}
	manifest.ModelFamily = strings.TrimSpace(manifest.ModelFamily)
	if manifest.ModelFamily == "" {
		return compiledManifest{}, errors.New("manifest model family is required")
	}
	if len(manifest.Features) == 0 {
		return compiledManifest{}, errors.New("manifest features cannot be empty")
	}

	index := make(map[string]int, len(manifest.Features))
	for i, name := range manifest.Features {
		name = strings.TrimSpace(name)
		if name == "" {
			return compiledManifest{}, fmt.Errorf("manifest feature at index %d is empty", i)
		}
		if _, exists := index[name]; exists {
			return compiledManifest{}, fmt.Errorf("manifest contains duplicate feature %q", name)
		}
		manifest.Features[i] = name
		index[name] = i
	}

	return compiledManifest{manifest: manifest, index: index}, nil
}

func buildCompiledManifests() map[manifestKey]compiledManifest {
	raw := []Manifest{
		{
			Version:     ManifestVersionV1,
			Sport:       domain.SportMLB,
			ModelFamily: "starter-run-environment",
			Features: []string{
				"injury_availability_edge_norm",
				"injury_away_absence",
				"injury_away_availability",
				"injury_home_absence",
				"injury_home_availability",
				"market_away_implied_prob",
				"market_home_implied_prob",
				"market_home_spread",
				"market_home_spread_norm",
				"market_key_number_proximity",
				"market_total_points",
				"market_total_points_norm",
				"mlb_bullpen_era_edge_norm",
				"mlb_park_run_factor_norm",
				"mlb_starter_era_edge_norm",
				"mlb_weather_run_drag_norm",
				"situational_neutral_site",
				"situational_rest_days_diff_norm",
				"situational_schedule_density_edge_norm",
				"situational_travel_edge_norm",
				"team_quality_defense_diff_norm",
				"team_quality_net_rating_diff_norm",
				"team_quality_offense_diff_norm",
				"team_quality_power_rating_diff_norm",
				"weather_is_dome",
				"weather_precip_penalty_norm",
				"weather_severity_norm",
				"weather_temp_penalty_norm",
				"weather_wind_penalty_norm",
			},
		},
		{
			Version:     ManifestVersionV1,
			Sport:       domain.SportNBA,
			ModelFamily: "lineup-adjusted-net-rating",
			Features: []string{
				"injury_availability_edge_norm",
				"injury_away_absence",
				"injury_away_availability",
				"injury_home_absence",
				"injury_home_availability",
				"market_away_implied_prob",
				"market_home_implied_prob",
				"market_home_spread",
				"market_home_spread_norm",
				"market_key_number_proximity",
				"market_total_points",
				"market_total_points_norm",
				"nba_back_to_back_edge_norm",
				"nba_lineup_net_rating_edge_norm",
				"nba_projected_pace_norm",
				"nba_rest_days_diff_norm",
				"situational_neutral_site",
				"situational_rest_days_diff_norm",
				"situational_schedule_density_edge_norm",
				"situational_travel_edge_norm",
				"team_quality_defense_diff_norm",
				"team_quality_net_rating_diff_norm",
				"team_quality_offense_diff_norm",
				"team_quality_power_rating_diff_norm",
				"weather_is_dome",
				"weather_precip_penalty_norm",
				"weather_severity_norm",
				"weather_temp_penalty_norm",
				"weather_wind_penalty_norm",
			},
		},
		{
			Version:     ManifestVersionV1,
			Sport:       domain.SportNHL,
			ModelFamily: "xg-goalie-quality",
			Features: []string{
				"injury_availability_edge_norm",
				"injury_away_absence",
				"injury_away_availability",
				"injury_home_absence",
				"injury_home_availability",
				"market_away_implied_prob",
				"market_home_implied_prob",
				"market_home_spread",
				"market_home_spread_norm",
				"market_key_number_proximity",
				"market_total_points",
				"market_total_points_norm",
				"nhl_away_pdo_regression_pressure_norm",
				"nhl_corsi_edge_norm",
				"nhl_goalie_gsax_edge_norm",
				"nhl_home_pdo_regression_pressure_norm",
				"nhl_pdo_regression_edge_norm",
				"nhl_xg_share_edge_norm",
				"situational_neutral_site",
				"situational_rest_days_diff_norm",
				"situational_schedule_density_edge_norm",
				"situational_travel_edge_norm",
				"team_quality_defense_diff_norm",
				"team_quality_net_rating_diff_norm",
				"team_quality_offense_diff_norm",
				"team_quality_power_rating_diff_norm",
				"weather_is_dome",
				"weather_precip_penalty_norm",
				"weather_severity_norm",
				"weather_temp_penalty_norm",
				"weather_wind_penalty_norm",
			},
		},
		{
			Version:     ManifestVersionV1,
			Sport:       domain.SportNFL,
			ModelFamily: "epa-dvoa-situational",
			Features: []string{
				"injury_availability_edge_norm",
				"injury_away_absence",
				"injury_away_availability",
				"injury_home_absence",
				"injury_home_availability",
				"market_away_implied_prob",
				"market_home_implied_prob",
				"market_home_spread",
				"market_home_spread_norm",
				"market_key_number_proximity",
				"market_total_points",
				"market_total_points_norm",
				"nfl_dvoa_edge_norm",
				"nfl_key_number_proximity",
				"nfl_qb_epa_edge_norm",
				"nfl_wind_penalty_norm",
				"situational_neutral_site",
				"situational_rest_days_diff_norm",
				"situational_schedule_density_edge_norm",
				"situational_travel_edge_norm",
				"team_quality_defense_diff_norm",
				"team_quality_net_rating_diff_norm",
				"team_quality_offense_diff_norm",
				"team_quality_power_rating_diff_norm",
				"weather_is_dome",
				"weather_precip_penalty_norm",
				"weather_severity_norm",
				"weather_temp_penalty_norm",
				"weather_wind_penalty_norm",
			},
		},
	}

	compiled := make(map[manifestKey]compiledManifest, len(raw))
	for _, manifest := range raw {
		entry, err := compileManifest(manifest)
		if err != nil {
			panic(fmt.Sprintf("invalid built-in manifest for sport=%s model=%s: %v", manifest.Sport, manifest.ModelFamily, err))
		}
		key := manifestKey{
			version:     manifest.Version,
			sport:       manifest.Sport,
			modelFamily: manifest.ModelFamily,
		}
		compiled[key] = entry
	}

	return compiled
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	slices.Sort(out)
	return out
}
