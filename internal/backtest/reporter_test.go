package backtest

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"betbot/internal/domain"
)

func TestWriteArtifactsWritesPipelineAndCSV(t *testing.T) {
	dir := t.TempDir()

	artifact := PipelineArtifact{
		GeneratedAtUTC: time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC),
		MarketKey:      "h2h",
		ModelVersion:   "v1",
		Outcomes: []Outcome{
			{
				GameID:                   1,
				Source:                   "the-odds-api",
				ExternalID:               "game-1",
				Sport:                    domain.SportNFL,
				HomeTeam:                 "Home",
				AwayTeam:                 "Away",
				BookKey:                  "draftkings",
				MarketKey:                "h2h",
				CommenceTimeUTC:          time.Date(2026, time.January, 4, 18, 0, 0, 0, time.UTC),
				OpeningCapturedAtUTC:     time.Date(2026, time.January, 4, 13, 0, 0, 0, time.UTC),
				ClosingCapturedAtUTC:     time.Date(2026, time.January, 4, 17, 30, 0, 0, time.UTC),
				OpeningHomeProbability:   0.52,
				ClosingHomeProbability:   0.55,
				PredictedHomeProbability: 0.58,
				RecommendedSide:          "home",
				OpeningSideProbability:   0.52,
				ClosingSideProbability:   0.55,
				ModelEdge:                0.06,
				CLVDelta:                 0.03,
				CalibrationError:         0.03,
			},
		},
	}

	paths, err := WriteArtifacts(dir, artifact)
	if err != nil {
		t.Fatalf("WriteArtifacts() error = %v", err)
	}

	if _, err := os.Stat(paths.PipelineJSON); err != nil {
		t.Fatalf("expected pipeline artifact file: %v", err)
	}
	if _, err := os.Stat(paths.OutcomesCSV); err != nil {
		t.Fatalf("expected outcomes csv file: %v", err)
	}

	body, err := os.ReadFile(paths.PipelineJSON)
	if err != nil {
		t.Fatalf("read pipeline json: %v", err)
	}
	var decoded PipelineArtifact
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal pipeline json: %v", err)
	}
	if len(decoded.Outcomes) != 1 {
		t.Fatalf("decoded outcomes count = %d, want 1", len(decoded.Outcomes))
	}

	f, err := os.Open(paths.OutcomesCSV)
	if err != nil {
		t.Fatalf("open outcomes csv: %v", err)
	}
	defer f.Close()
	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read outcomes csv: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("csv row count = %d, want 2", len(records))
	}
	if got := records[0][20]; got != "kelly_fraction" {
		t.Fatalf("csv header[20] = %q, want %q", got, "kelly_fraction")
	}
	if got := records[0][23]; got != "recommended_stake_dollars" {
		t.Fatalf("csv header[23] = %q, want %q", got, "recommended_stake_dollars")
	}
}

func TestWriteArtifactsRejectsEmptyOutputDir(t *testing.T) {
	_, err := WriteArtifacts("", PipelineArtifact{})
	if err == nil {
		t.Fatal("expected error for empty output dir")
	}
}

func TestWriteArtifactsUsesExpectedFilenames(t *testing.T) {
	dir := t.TempDir()
	paths, err := WriteArtifacts(dir, PipelineArtifact{})
	if err != nil {
		t.Fatalf("WriteArtifacts() error = %v", err)
	}
	if filepath.Base(paths.PipelineJSON) != "pipeline_report.json" {
		t.Fatalf("pipeline filename = %s", filepath.Base(paths.PipelineJSON))
	}
	if filepath.Base(paths.OutcomesCSV) != "outcomes.csv" {
		t.Fatalf("outcomes filename = %s", filepath.Base(paths.OutcomesCSV))
	}
}
