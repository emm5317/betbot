package features

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"betbot/internal/domain"
)

func TestBuildWalkForwardSplitsValidationFailures(t *testing.T) {
	_, err := BuildWalkForwardSplits(20, WalkForwardConfig{TrainWindow: 0, ValidationWindow: 5, Step: 1})
	if err == nil || !strings.Contains(err.Error(), "train window") {
		t.Fatalf("train window error = %v", err)
	}

	_, err = BuildWalkForwardSplits(20, WalkForwardConfig{TrainWindow: 8, ValidationWindow: 0, Step: 1})
	if err == nil || !strings.Contains(err.Error(), "validation window") {
		t.Fatalf("validation window error = %v", err)
	}

	_, err = BuildWalkForwardSplits(20, WalkForwardConfig{TrainWindow: 8, ValidationWindow: 5, Step: 0})
	if err == nil || !strings.Contains(err.Error(), "step") {
		t.Fatalf("step error = %v", err)
	}

	_, err = BuildWalkForwardSplits(10, WalkForwardConfig{TrainWindow: 8, ValidationWindow: 5, Step: 1})
	if err == nil || !strings.Contains(err.Error(), "insufficient samples") {
		t.Fatalf("insufficient sample error = %v", err)
	}
}

func TestBuildWalkForwardSplitsNoLookAheadLeakage(t *testing.T) {
	splits, err := BuildWalkForwardSplits(20, WalkForwardConfig{TrainWindow: 8, ValidationWindow: 4, Step: 3})
	if err != nil {
		t.Fatalf("BuildWalkForwardSplits() error = %v", err)
	}

	if len(splits) != 3 {
		t.Fatalf("split count = %d, want 3", len(splits))
	}

	for i, split := range splits {
		if split.TrainStart >= split.TrainEnd {
			t.Fatalf("split %d invalid train bounds: %+v", i, split)
		}
		if split.ValidationStart != split.TrainEnd {
			t.Fatalf("split %d leaks or gaps: trainEnd=%d validationStart=%d", i, split.TrainEnd, split.ValidationStart)
		}
		if split.ValidationStart >= split.ValidationEnd {
			t.Fatalf("split %d invalid validation bounds: %+v", i, split)
		}
	}
}

func TestCalibrateNormalizationScalesValidationFailures(t *testing.T) {
	_, err := CalibrateNormalizationScales(CalibrationRequest{
		Sport:       domain.SportNFL,
		ModelFamily: "epa-dvoa-situational",
		Window:      WalkForwardConfig{TrainWindow: 8, ValidationWindow: 4, Step: 2},
	})
	if err == nil || !strings.Contains(err.Error(), "samples cannot be empty") {
		t.Fatalf("empty sample error = %v", err)
	}

	samples := makeCalibrationSamples(t, domain.SportNFL, 20)

	invalidWindowReq := CalibrationRequest{
		Sport:       domain.SportNFL,
		ModelFamily: "epa-dvoa-situational",
		Samples:     samples,
		Window:      WalkForwardConfig{TrainWindow: 0, ValidationWindow: 4, Step: 2},
	}
	_, err = CalibrateNormalizationScales(invalidWindowReq)
	if err == nil || !strings.Contains(err.Error(), "train window") {
		t.Fatalf("invalid window error = %v", err)
	}

	mismatch := append([]CalibrationSample(nil), samples...)
	mismatch[0].Vector.Sport = domain.SportNBA
	mismatchReq := CalibrationRequest{
		Sport:       domain.SportNFL,
		ModelFamily: "epa-dvoa-situational",
		Samples:     mismatch,
		Window:      WalkForwardConfig{TrainWindow: 8, ValidationWindow: 4, Step: 2},
	}
	_, err = CalibrateNormalizationScales(mismatchReq)
	if err == nil || !strings.Contains(err.Error(), "sport mismatch") {
		t.Fatalf("sport mismatch error = %v", err)
	}
}

func TestCalibrateNormalizationScalesDeterministic(t *testing.T) {
	samples := makeCalibrationSamples(t, domain.SportNFL, 24)
	req := CalibrationRequest{
		Sport:       domain.SportNFL,
		ModelFamily: "epa-dvoa-situational",
		Samples:     samples,
		Window:      WalkForwardConfig{TrainWindow: 10, ValidationWindow: 4, Step: 3},
		BaseConfig:  DefaultBuilderConfig(),
	}

	first, err := CalibrateNormalizationScales(req)
	if err != nil {
		t.Fatalf("first calibration error = %v", err)
	}
	second, err := CalibrateNormalizationScales(req)
	if err != nil {
		t.Fatalf("second calibration error = %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatal("calibration output is not deterministic")
	}

	if first.Sport != domain.SportNFL {
		t.Fatalf("artifact sport = %s, want %s", first.Sport, domain.SportNFL)
	}
	if first.ManifestVersion != ManifestVersionV1 {
		t.Fatalf("manifest version = %s, want %s", first.ManifestVersion, ManifestVersionV1)
	}
	if len(first.InputWindow.Splits) == 0 {
		t.Fatal("expected non-empty split bounds in artifact")
	}
	if first.Diagnostics.Samples == 0 {
		t.Fatal("expected non-zero diagnostics sample count")
	}

	required := map[string]struct{}{
		"spread_scale":     {},
		"total_scale":      {},
		"rating_scale":     {},
		"travel_scale":     {},
		"qb_epa_scale":     {},
		"dvoa_scale":       {},
		"key_number_scale": {},
	}
	for _, rec := range first.Recommendations {
		delete(required, rec.Parameter)
		if rec.RecommendedScale <= 0 {
			t.Fatalf("parameter %s produced non-positive scale %.4f", rec.Parameter, rec.RecommendedScale)
		}
	}
	if len(required) != 0 {
		t.Fatalf("missing expected recommendations: %v", required)
	}
}

func makeCalibrationSamples(t *testing.T, sport domain.Sport, n int) []CalibrationSample {
	t.Helper()

	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}

	samples := make([]CalibrationSample, 0, n)
	start := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	baseTotal := map[domain.Sport]float64{
		domain.SportMLB: 8.5,
		domain.SportNBA: 224.0,
		domain.SportNHL: 6.0,
		domain.SportNFL: 45.0,
	}

	for i := 0; i < n; i++ {
		req := validRequestForSport(sport)
		homeProb := 0.45 + float64(i%11)/100.0
		req.Market.HomeMoneylineProbability = homeProb
		req.Market.AwayMoneylineProbability = 1 - homeProb
		req.Market.HomeSpread = -7 + float64(i%15)
		req.Market.TotalPoints = baseTotal[sport] + float64((i%7)-3)

		req.TeamQuality.HomePowerRating += float64((i % 6) - 3)
		req.TeamQuality.AwayPowerRating += float64(((i + 2) % 6) - 3)
		req.TeamQuality.HomeOffenseRating += float64((i % 5) - 2)
		req.TeamQuality.AwayOffenseRating += float64(((i + 1) % 5) - 2)
		req.TeamQuality.HomeDefenseRating += float64((i % 5) - 2)
		req.TeamQuality.AwayDefenseRating += float64(((i + 3) % 5) - 2)

		req.Situational.HomeRestDays = i % 5
		req.Situational.AwayRestDays = (i + 2) % 5
		req.Situational.HomeTravelMiles = 100 + float64((i%10)*45)
		req.Situational.AwayTravelMiles = 300 + float64((i%10)*90)
		req.Situational.HomeGamesLast7 = i % 7
		req.Situational.AwayGamesLast7 = (i + 3) % 7

		switch sport {
		case domain.SportMLB:
			req.MLB.HomeStarterERA += float64(i%4) * 0.1
			req.MLB.AwayStarterERA += float64((i+1)%4) * 0.1
			req.MLB.HomeBullpenERA += float64(i%3) * 0.05
			req.MLB.AwayBullpenERA += float64((i+2)%3) * 0.05
		case domain.SportNBA:
			req.NBA.HomeLineupNetRating += float64((i%7)-3) * 0.3
			req.NBA.AwayLineupNetRating += float64(((i+2)%7)-3) * 0.3
			req.NBA.ProjectedPace += float64((i%5)-2) * 0.4
		case domain.SportNHL:
			req.NHL.HomeGoalieGSAx += float64((i%6)-3) * 0.4
			req.NHL.AwayGoalieGSAx += float64(((i+1)%6)-3) * 0.4
			req.NHL.HomePDO += float64((i%5)-2) * 0.002
			req.NHL.AwayPDO += float64(((i+2)%5)-2) * 0.002
		case domain.SportNFL:
			req.NFL.HomeQBEPA += float64((i%7)-3) * 0.01
			req.NFL.AwayQBEPA += float64(((i+1)%7)-3) * 0.01
			req.NFL.HomeDVOA += float64((i%6)-3) * 0.01
			req.NFL.AwayDVOA += float64(((i+2)%6)-3) * 0.01
		}

		vector, err := registry.Build(req)
		if err != nil {
			t.Fatalf("registry.Build(%s) error = %v", sport, err)
		}

		signal := req.Market.HomeMoneylineProbability - 0.5
		if req.Market.HomeSpread < 0 {
			signal += 0.06
		}
		if i%4 == 0 {
			signal += 0.03
		}
		outcome := 0.0
		if signal >= 0 {
			outcome = 1.0
		}

		samples = append(samples, CalibrationSample{
			EventTime: start.AddDate(0, 0, i),
			Vector:    vector,
			Outcome:   outcome,
		})
	}

	return samples
}
