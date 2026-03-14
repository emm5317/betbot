package features

import (
	"strings"
	"testing"

	"betbot/internal/domain"
)

func TestManifestForCompletenessPerSport(t *testing.T) {
	sports := []domain.Sport{domain.SportMLB, domain.SportNBA, domain.SportNHL, domain.SportNFL}
	for _, sport := range sports {
		cfg, ok := domain.DefaultSportRegistry().Get(sport)
		if !ok {
			t.Fatalf("missing sport config for %s", sport)
		}

		manifest, err := ManifestFor(sport, cfg.DefaultModelFamily)
		if err != nil {
			t.Fatalf("ManifestFor(%s,%s) error = %v", sport, cfg.DefaultModelFamily, err)
		}
		if manifest.Version != ManifestVersionV1 {
			t.Fatalf("manifest version = %s, want %s", manifest.Version, ManifestVersionV1)
		}
		if len(manifest.Features) == 0 {
			t.Fatalf("manifest for %s is empty", sport)
		}
	}
}

func TestEncodeDecodeVectorRoundTripFixedIndexMapping(t *testing.T) {
	manifest, err := ManifestFor(domain.SportNFL, "epa-dvoa-situational")
	if err != nil {
		t.Fatalf("ManifestFor() error = %v", err)
	}

	features := make([]Feature, 0, len(manifest.Features))
	for i := len(manifest.Features) - 1; i >= 0; i-- {
		features = append(features, Feature{Name: manifest.Features[i], Value: float64(i + 1)})
	}
	vector := FeatureVector{
		Sport:           domain.SportNFL,
		ModelFamily:     "epa-dvoa-situational",
		ManifestVersion: ManifestVersionV1,
		Features:        features,
	}

	encoded, err := EncodeVector(vector, manifest)
	if err != nil {
		t.Fatalf("EncodeVector() error = %v", err)
	}
	for i, value := range encoded {
		want := float64(i + 1)
		if value != want {
			t.Fatalf("encoded[%d] = %.2f, want %.2f", i, value, want)
		}
	}

	decoded, err := DecodeVector(encoded, manifest)
	if err != nil {
		t.Fatalf("DecodeVector() error = %v", err)
	}
	if len(decoded) != len(manifest.Features) {
		t.Fatalf("decoded length = %d, want %d", len(decoded), len(manifest.Features))
	}
	for i := range decoded {
		if decoded[i].Name != manifest.Features[i] {
			t.Fatalf("decoded[%d].Name = %s, want %s", i, decoded[i].Name, manifest.Features[i])
		}
		if decoded[i].Value != float64(i+1) {
			t.Fatalf("decoded[%d].Value = %.2f, want %.2f", i, decoded[i].Value, float64(i+1))
		}
	}
}

func TestManifestMismatchErrors(t *testing.T) {
	manifest, err := ManifestFor(domain.SportNBA, "lineup-adjusted-net-rating")
	if err != nil {
		t.Fatalf("ManifestFor() error = %v", err)
	}

	base := FeatureVector{
		Sport:           domain.SportNBA,
		ModelFamily:     "lineup-adjusted-net-rating",
		ManifestVersion: ManifestVersionV1,
		Features:        make([]Feature, 0, len(manifest.Features)),
	}
	for i, name := range manifest.Features {
		base.Features = append(base.Features, Feature{Name: name, Value: float64(i)})
	}

	missing := base
	missing.Features = missing.Features[1:]
	if _, err := EncodeVector(missing, manifest); err == nil || !strings.Contains(err.Error(), "missing=") {
		t.Fatalf("missing feature error = %v", err)
	}

	extra := base
	extra.Features = append(extra.Features, Feature{Name: "unknown_feature", Value: 1})
	if _, err := EncodeVector(extra, manifest); err == nil || !strings.Contains(err.Error(), "extra=") {
		t.Fatalf("extra feature error = %v", err)
	}

	wrongVersion := base
	wrongVersion.ManifestVersion = ManifestVersion("v0")
	if _, err := EncodeVector(wrongVersion, manifest); err == nil || !strings.Contains(err.Error(), "manifest version mismatch") {
		t.Fatalf("wrong version error = %v", err)
	}

	wrongOrder := base
	wrongOrder.Features[0], wrongOrder.Features[1] = wrongOrder.Features[1], wrongOrder.Features[0]
	if err := ValidateVectorMatchesManifest(wrongOrder, manifest); err == nil || !strings.Contains(err.Error(), "feature order mismatch") {
		t.Fatalf("wrong order error = %v", err)
	}
}

func TestBuilderOutputMatchesManifest(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}

	for _, sport := range []domain.Sport{domain.SportMLB, domain.SportNBA, domain.SportNHL, domain.SportNFL} {
		vector, err := registry.Build(validRequestForSport(sport))
		if err != nil {
			t.Fatalf("registry.Build(%s) error = %v", sport, err)
		}
		manifest, err := ManifestFor(vector.Sport, vector.ModelFamily)
		if err != nil {
			t.Fatalf("ManifestFor(%s,%s) error = %v", vector.Sport, vector.ModelFamily, err)
		}
		if err := ValidateVectorMatchesManifest(vector, manifest); err != nil {
			t.Fatalf("manifest mismatch for %s: %v", sport, err)
		}
		encoded, err := EncodeVector(vector, manifest)
		if err != nil {
			t.Fatalf("EncodeVector(%s) error = %v", sport, err)
		}
		if len(encoded) != len(manifest.Features) {
			t.Fatalf("encoded length for %s = %d, want %d", sport, len(encoded), len(manifest.Features))
		}
	}
}
