package modeling

import (
	"context"
	"strings"
	"testing"
	"time"

	"betbot/internal/domain"
	"betbot/internal/modeling/features"
	"betbot/internal/store"
)

type fakePredictionStore struct {
	arg       store.UpsertModelPredictionParams
	called    bool
	result    store.ModelPrediction
	returnErr error
}

func (f *fakePredictionStore) UpsertModelPrediction(_ context.Context, arg store.UpsertModelPredictionParams) (store.ModelPrediction, error) {
	f.called = true
	f.arg = arg
	if f.returnErr != nil {
		return store.ModelPrediction{}, f.returnErr
	}
	if f.result.ID == 0 {
		f.result = store.ModelPrediction{
			ID:                   99,
			GameID:               arg.GameID,
			Source:               arg.Source,
			Sport:                arg.Sport,
			BookKey:              arg.BookKey,
			MarketKey:            arg.MarketKey,
			ModelFamily:          arg.ModelFamily,
			ModelVersion:         arg.ModelVersion,
			ManifestVersion:      arg.ManifestVersion,
			FeatureVector:        append([]float64(nil), arg.FeatureVector...),
			PredictedProbability: arg.PredictedProbability,
			MarketProbability:    arg.MarketProbability,
			ClosingProbability:   arg.ClosingProbability,
			EventTime:            arg.EventTime,
		}
	}
	return f.result, nil
}

func TestPersistPredictionEncodesManifestStableOrdering(t *testing.T) {
	registry, err := features.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}

	vector, err := registry.Build(features.BuildRequest{
		Sport: domain.SportNFL,
		Market: features.MarketInputs{
			HomeMoneylineProbability: 0.57,
			AwayMoneylineProbability: 0.43,
			HomeSpread:               -2.5,
			TotalPoints:              46.0,
		},
		TeamQuality: features.TeamQualityInputs{
			HomePowerRating:   92,
			AwayPowerRating:   88,
			HomeOffenseRating: 111,
			AwayOffenseRating: 106,
			HomeDefenseRating: 103,
			AwayDefenseRating: 108,
		},
		Situational: features.SituationalInputs{
			HomeRestDays:    2,
			AwayRestDays:    1,
			HomeTravelMiles: 100,
			AwayTravelMiles: 900,
			HomeGamesLast7:  1,
			AwayGamesLast7:  1,
		},
		Injuries: features.InjuryInputs{HomeAvailability: 0.91, AwayAvailability: 0.82},
		Weather:  features.WeatherInputs{TemperatureF: 45, WindMPH: 13, PrecipitationMM: 0.5},
		NFL: &features.NFLContext{
			HomeQBEPA:        0.14,
			AwayQBEPA:        0.04,
			HomeDVOA:         0.12,
			AwayDVOA:         0.03,
			PrimaryKeyNumber: 3,
		},
	})
	if err != nil {
		t.Fatalf("registry.Build() error = %v", err)
	}

	fake := &fakePredictionStore{}
	closing := 0.61
	stored, err := PersistPrediction(context.Background(), fake, PersistRequest{
		GameID:               17,
		Source:               "the-odds-api",
		Sport:                domain.SportNFL,
		BookKey:              "draftkings",
		MarketKey:            "h2h",
		ModelFamily:          vector.ModelFamily,
		ModelVersion:         "v1",
		FeatureVector:        vector,
		PredictedProbability: 0.59,
		MarketProbability:    0.57,
		ClosingProbability:   &closing,
		EventTimeUTC:         time.Date(2026, time.January, 4, 18, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("PersistPrediction() error = %v", err)
	}
	if !fake.called {
		t.Fatal("expected store upsert call")
	}

	manifest, err := features.ManifestFor(domain.SportNFL, vector.ModelFamily)
	if err != nil {
		t.Fatalf("ManifestFor() error = %v", err)
	}
	if len(fake.arg.FeatureVector) != len(manifest.Features) {
		t.Fatalf("encoded feature length = %d, want %d", len(fake.arg.FeatureVector), len(manifest.Features))
	}

	decoded, err := features.DecodeVector(fake.arg.FeatureVector, manifest)
	if err != nil {
		t.Fatalf("DecodeVector() error = %v", err)
	}
	for i := range decoded {
		if decoded[i].Name != manifest.Features[i] {
			t.Fatalf("decoded[%d] name = %s, want %s", i, decoded[i].Name, manifest.Features[i])
		}
	}

	if stored.ID == 0 {
		t.Fatal("stored prediction id should be populated")
	}
	if stored.ManifestVersion != string(features.ManifestVersionV1) {
		t.Fatalf("manifest version = %s, want %s", stored.ManifestVersion, features.ManifestVersionV1)
	}
}

func TestPersistPredictionValidationFailures(t *testing.T) {
	fake := &fakePredictionStore{}
	_, err := PersistPrediction(context.Background(), fake, PersistRequest{})
	if err == nil || !strings.Contains(err.Error(), "game id") {
		t.Fatalf("expected game id validation error, got %v", err)
	}

	registry, err := features.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}
	vector, err := registry.Build(features.BuildRequest{
		Sport: domain.SportMLB,
		Market: features.MarketInputs{
			HomeMoneylineProbability: 0.54,
			AwayMoneylineProbability: 0.46,
			HomeSpread:               -1.5,
			TotalPoints:              8.5,
		},
		TeamQuality: features.TeamQualityInputs{HomePowerRating: 92, AwayPowerRating: 90, HomeOffenseRating: 106, AwayOffenseRating: 102, HomeDefenseRating: 98, AwayDefenseRating: 101},
		Situational: features.SituationalInputs{HomeRestDays: 1, AwayRestDays: 1, HomeTravelMiles: 100, AwayTravelMiles: 650, HomeGamesLast7: 5, AwayGamesLast7: 5},
		Injuries:    features.InjuryInputs{HomeAvailability: 0.95, AwayAvailability: 0.93},
		Weather:     features.WeatherInputs{TemperatureF: 72, WindMPH: 8, PrecipitationMM: 0},
		MLB:         &features.MLBContext{HomeStarterERA: 3.4, AwayStarterERA: 4.1, HomeBullpenERA: 3.7, AwayBullpenERA: 4.2, ParkFactor: 1.0},
	})
	if err != nil {
		t.Fatalf("registry.Build() error = %v", err)
	}

	badProbReq := PersistRequest{
		GameID:               1,
		Source:               "src",
		Sport:                domain.SportMLB,
		BookKey:              "book",
		MarketKey:            "h2h",
		ModelFamily:          vector.ModelFamily,
		ModelVersion:         "v1",
		FeatureVector:        vector,
		PredictedProbability: 1.2,
		MarketProbability:    0.5,
		EventTimeUTC:         time.Now().UTC(),
	}
	_, err = PersistPrediction(context.Background(), fake, badProbReq)
	if err == nil || !strings.Contains(err.Error(), "predicted probability") {
		t.Fatalf("expected predicted probability error, got %v", err)
	}
}
