package features

import (
	"reflect"
	"strings"
	"testing"

	"betbot/internal/domain"
)

func TestSportBuildersRejectInvalidConfig(t *testing.T) {
	invalid := DefaultBuilderConfig()
	invalid.SpreadScale = 0

	constructors := []struct {
		name string
		fn   func(BuilderConfig) error
	}{
		{name: "mlb", fn: func(cfg BuilderConfig) error { _, err := NewMLBBuilder(cfg); return err }},
		{name: "nba", fn: func(cfg BuilderConfig) error { _, err := NewNBABuilder(cfg); return err }},
		{name: "nhl", fn: func(cfg BuilderConfig) error { _, err := NewNHLBuilder(cfg); return err }},
		{name: "nfl", fn: func(cfg BuilderConfig) error { _, err := NewNFLBuilder(cfg); return err }},
	}

	for _, tc := range constructors {
		err := tc.fn(invalid)
		if err == nil {
			t.Fatalf("%s builder expected invalid config error, got nil", tc.name)
		}
	}
}

func TestRegistryBuildRejectsMissingSportContext(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}

	cases := []struct {
		sport domain.Sport
		name  string
	}{
		{sport: domain.SportMLB, name: "mlb"},
		{sport: domain.SportNBA, name: "nba"},
		{sport: domain.SportNHL, name: "nhl"},
		{sport: domain.SportNFL, name: "nfl"},
	}

	for _, tc := range cases {
		req := validBaseRequest()
		req.Sport = tc.sport

		_, err := registry.Build(req)
		if err == nil {
			t.Fatalf("%s expected missing context validation error, got nil", tc.name)
		}
	}
}

func TestRegistryBuildRejectsOutOfRangeSportContext(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}

	tests := []struct {
		name  string
		req   BuildRequest
		check string
	}{
		{
			name: "mlb invalid starter era",
			req: func() BuildRequest {
				r := validRequestForSport(domain.SportMLB)
				r.MLB.HomeStarterERA = 20
				return r
			}(),
			check: "mlb",
		},
		{
			name: "nba invalid projected pace",
			req: func() BuildRequest {
				r := validRequestForSport(domain.SportNBA)
				r.NBA.ProjectedPace = 130
				return r
			}(),
			check: "nba",
		},
		{
			name: "nhl invalid pdo",
			req: func() BuildRequest {
				r := validRequestForSport(domain.SportNHL)
				r.NHL.HomePDO = 1.3
				return r
			}(),
			check: "nhl",
		},
		{
			name: "nfl invalid key number",
			req: func() BuildRequest {
				r := validRequestForSport(domain.SportNFL)
				r.NFL.PrimaryKeyNumber = 0
				return r
			}(),
			check: "nfl",
		},
	}

	for _, tc := range tests {
		_, err := registry.Build(tc.req)
		if err == nil {
			t.Fatalf("%s expected validation error, got nil", tc.name)
		}
		if !strings.Contains(strings.ToLower(err.Error()), tc.check) {
			t.Fatalf("%s error %q should mention %q", tc.name, err.Error(), tc.check)
		}
	}
}

func TestRegistryBuildIsDeterministicAcrossSports(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}

	sports := []domain.Sport{domain.SportMLB, domain.SportNBA, domain.SportNHL, domain.SportNFL}
	for _, sport := range sports {
		req := validRequestForSport(sport)
		first, err := registry.Build(req)
		if err != nil {
			t.Fatalf("first build for %s error = %v", sport, err)
		}
		second, err := registry.Build(req)
		if err != nil {
			t.Fatalf("second build for %s error = %v", sport, err)
		}

		if !reflect.DeepEqual(first, second) {
			t.Fatalf("expected deterministic output for %s", sport)
		}
	}
}

func TestRegistryBuildCrossSportFeatureDifferentiation(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}

	expectedSportSpecific := map[domain.Sport]string{
		domain.SportMLB: "mlb_starter_era_edge_norm",
		domain.SportNBA: "nba_lineup_net_rating_edge_norm",
		domain.SportNHL: "nhl_goalie_gsax_edge_norm",
		domain.SportNFL: "nfl_qb_epa_edge_norm",
	}

	vectors := make(map[domain.Sport]FeatureVector, len(expectedSportSpecific))
	for sport := range expectedSportSpecific {
		vector, err := registry.Build(validRequestForSport(sport))
		if err != nil {
			t.Fatalf("build for %s error = %v", sport, err)
		}
		vectors[sport] = vector
	}

	for sport, featureName := range expectedSportSpecific {
		if _, ok := vectors[sport].Value(featureName); !ok {
			t.Fatalf("%s missing expected feature %q", sport, featureName)
		}
	}

	if reflect.DeepEqual(vectors[domain.SportMLB].Features, vectors[domain.SportNBA].Features) {
		t.Fatal("mlb and nba feature vectors should not match")
	}
	if reflect.DeepEqual(vectors[domain.SportNHL].Features, vectors[domain.SportNFL].Features) {
		t.Fatal("nhl and nfl feature vectors should not match")
	}
}

func TestRegistryBuildNormalizedFeatureBounds(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}

	zeroToOneFeatures := map[string]struct{}{
		"market_home_implied_prob":    {},
		"market_away_implied_prob":    {},
		"market_key_number_proximity": {},
		"weather_severity_norm":       {},
		"injury_home_availability":    {},
		"injury_away_availability":    {},
		"injury_home_absence":         {},
		"injury_away_absence":         {},
		"nfl_key_number_proximity":    {},
	}

	for _, sport := range []domain.Sport{domain.SportMLB, domain.SportNBA, domain.SportNHL, domain.SportNFL} {
		vector, err := registry.Build(validRequestForSport(sport))
		if err != nil {
			t.Fatalf("build for %s error = %v", sport, err)
		}

		for _, feature := range vector.Features {
			if strings.HasSuffix(feature.Name, "_norm") {
				if feature.Value < -1 || feature.Value > 1 {
					t.Fatalf("%s feature %s = %.4f expected in [-1,1]", sport, feature.Name, feature.Value)
				}
			}
			if _, ok := zeroToOneFeatures[feature.Name]; ok {
				if feature.Value < 0 || feature.Value > 1 {
					t.Fatalf("%s feature %s = %.4f expected in [0,1]", sport, feature.Name, feature.Value)
				}
			}
		}
	}
}

func TestRegistryDispatchAllSports(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}

	for _, sport := range []domain.Sport{domain.SportMLB, domain.SportNBA, domain.SportNHL, domain.SportNFL} {
		vector, err := registry.Build(validRequestForSport(sport))
		if err != nil {
			t.Fatalf("dispatch build for %s error = %v", sport, err)
		}
		if vector.Sport != sport {
			t.Fatalf("dispatch build returned sport %s, want %s", vector.Sport, sport)
		}
		if len(vector.Features) == 0 {
			t.Fatalf("dispatch build for %s returned empty features", sport)
		}
	}

	invalid := validBaseRequest()
	invalid.Sport = domain.Sport("NCAAB")
	_, err = registry.Build(invalid)
	if err == nil {
		t.Fatal("expected unsupported sport error, got nil")
	}
}

func validBaseRequest() BuildRequest {
	return BuildRequest{
		Market: MarketInputs{
			HomeMoneylineProbability: 0.56,
			AwayMoneylineProbability: 0.44,
			HomeSpread:               -3.5,
			TotalPoints:              44.5,
		},
		TeamQuality: TeamQualityInputs{
			HomePowerRating:   93.0,
			AwayPowerRating:   90.0,
			HomeOffenseRating: 116.0,
			AwayOffenseRating: 111.0,
			HomeDefenseRating: 107.0,
			AwayDefenseRating: 109.0,
		},
		Situational: SituationalInputs{
			HomeRestDays:    2,
			AwayRestDays:    1,
			HomeTravelMiles: 150,
			AwayTravelMiles: 925,
			HomeGamesLast7:  3,
			AwayGamesLast7:  4,
		},
		Injuries: InjuryInputs{
			HomeAvailability: 0.92,
			AwayAvailability: 0.86,
		},
		Weather: WeatherInputs{
			TemperatureF:    55,
			WindMPH:         12,
			PrecipitationMM: 1.5,
			IsDome:          false,
		},
	}
}

func validRequestForSport(sport domain.Sport) BuildRequest {
	req := validBaseRequest()
	req.Sport = sport

	switch sport {
	case domain.SportMLB:
		req.Market.TotalPoints = 8.5
		req.MLB = &MLBContext{
			HomeStarterERA: 3.25,
			AwayStarterERA: 4.10,
			HomeBullpenERA: 3.55,
			AwayBullpenERA: 4.02,
			ParkFactor:     1.03,
		}
	case domain.SportNBA:
		req.Market.TotalPoints = 228.5
		req.Market.HomeSpread = -4.0
		req.NBA = &NBAContext{
			HomeLineupNetRating: 6.8,
			AwayLineupNetRating: 2.1,
			ProjectedPace:       100.3,
			HomeBackToBack:      false,
			AwayBackToBack:      true,
		}
	case domain.SportNHL:
		req.Market.TotalPoints = 6.0
		req.Market.HomeSpread = -1.5
		req.NHL = &NHLContext{
			HomeGoalieGSAx: 8.2,
			AwayGoalieGSAx: -1.4,
			HomeXGShare:    0.54,
			AwayXGShare:    0.49,
			HomePDO:        0.992,
			AwayPDO:        1.021,
		}
	case domain.SportNFL:
		req.Market.TotalPoints = 46.5
		req.Market.HomeSpread = -2.5
		req.NFL = &NFLContext{
			HomeQBEPA:        0.18,
			AwayQBEPA:        0.05,
			HomeDVOA:         0.19,
			AwayDVOA:         0.07,
			PrimaryKeyNumber: 3.0,
		}
	}

	return req
}
