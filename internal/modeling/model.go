package modeling

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"betbot/internal/domain"
	"betbot/internal/modeling/features"
	"betbot/internal/store"
)

// PredictionStore is the sqlc-generated persistence surface required by PersistPrediction.
type PredictionStore interface {
	UpsertModelPrediction(ctx context.Context, arg store.UpsertModelPredictionParams) (store.ModelPrediction, error)
}

// PersistRequest captures the canonical model prediction payload persisted in model_predictions.
type PersistRequest struct {
	GameID               int64
	Source               string
	Sport                domain.Sport
	BookKey              string
	MarketKey            string
	ModelFamily          string
	ModelVersion         string
	FeatureVector        features.FeatureVector
	PredictedProbability float64
	MarketProbability    float64
	ClosingProbability   *float64
	EventTimeUTC         time.Time
}

// PersistPrediction writes a prediction with a manifest-backed, stably-indexed feature vector.
func PersistPrediction(ctx context.Context, storeSink PredictionStore, req PersistRequest) (store.ModelPrediction, error) {
	if storeSink == nil {
		return store.ModelPrediction{}, errors.New("prediction store is required")
	}
	if req.GameID <= 0 {
		return store.ModelPrediction{}, errors.New("game id must be > 0")
	}
	if strings.TrimSpace(req.Source) == "" {
		return store.ModelPrediction{}, errors.New("source is required")
	}
	if strings.TrimSpace(string(req.Sport)) == "" {
		return store.ModelPrediction{}, errors.New("sport is required")
	}
	if strings.TrimSpace(req.BookKey) == "" {
		return store.ModelPrediction{}, errors.New("book key is required")
	}
	if strings.TrimSpace(req.MarketKey) == "" {
		return store.ModelPrediction{}, errors.New("market key is required")
	}
	if strings.TrimSpace(req.ModelFamily) == "" {
		return store.ModelPrediction{}, errors.New("model family is required")
	}
	if strings.TrimSpace(req.ModelVersion) == "" {
		return store.ModelPrediction{}, errors.New("model version is required")
	}
	if req.EventTimeUTC.IsZero() {
		return store.ModelPrediction{}, errors.New("event time is required")
	}
	if err := validateProbability(req.PredictedProbability, "predicted probability"); err != nil {
		return store.ModelPrediction{}, err
	}
	if err := validateProbability(req.MarketProbability, "market probability"); err != nil {
		return store.ModelPrediction{}, err
	}
	if req.ClosingProbability != nil {
		if err := validateProbability(*req.ClosingProbability, "closing probability"); err != nil {
			return store.ModelPrediction{}, err
		}
	}

	manifest, err := features.ManifestFor(req.FeatureVector.Sport, req.FeatureVector.ModelFamily)
	if err != nil {
		return store.ModelPrediction{}, fmt.Errorf("resolve feature manifest: %w", err)
	}
	if err := features.ValidateVectorMatchesManifest(req.FeatureVector, manifest); err != nil {
		return store.ModelPrediction{}, fmt.Errorf("validate feature vector against manifest: %w", err)
	}

	encoded, err := features.EncodeVector(req.FeatureVector, manifest)
	if err != nil {
		return store.ModelPrediction{}, fmt.Errorf("encode feature vector: %w", err)
	}

	prediction, err := storeSink.UpsertModelPrediction(ctx, store.UpsertModelPredictionParams{
		GameID:               req.GameID,
		Source:               strings.TrimSpace(req.Source),
		Sport:                strings.TrimSpace(string(req.Sport)),
		BookKey:              strings.TrimSpace(req.BookKey),
		MarketKey:            strings.TrimSpace(req.MarketKey),
		ModelFamily:          strings.TrimSpace(req.ModelFamily),
		ModelVersion:         strings.TrimSpace(req.ModelVersion),
		ManifestVersion:      string(req.FeatureVector.ManifestVersion),
		FeatureVector:        encoded,
		PredictedProbability: req.PredictedProbability,
		MarketProbability:    req.MarketProbability,
		ClosingProbability:   req.ClosingProbability,
		EventTime:            store.Timestamptz(req.EventTimeUTC),
	})
	if err != nil {
		return store.ModelPrediction{}, fmt.Errorf("upsert model prediction: %w", err)
	}

	return prediction, nil
}

func validateProbability(value float64, field string) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return fmt.Errorf("%s must be finite in [0,1]", field)
	}
	return nil
}
