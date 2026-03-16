package backtest

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type ArtifactPaths struct {
	PipelineJSON string
	OutcomesCSV  string
}

// WriteArtifacts writes a single pipeline JSON artifact and an outcomes CSV extracted from that artifact.
func WriteArtifacts(outputDir string, artifact PipelineArtifact) (ArtifactPaths, error) {
	if outputDir == "" {
		return ArtifactPaths{}, fmt.Errorf("output directory is required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return ArtifactPaths{}, fmt.Errorf("create output directory: %w", err)
	}

	pipelinePath := filepath.Join(outputDir, "pipeline_report.json")
	outcomesPath := filepath.Join(outputDir, "outcomes.csv")

	body, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return ArtifactPaths{}, fmt.Errorf("marshal pipeline artifact: %w", err)
	}
	if err := os.WriteFile(pipelinePath, body, 0o644); err != nil {
		return ArtifactPaths{}, fmt.Errorf("write pipeline artifact: %w", err)
	}

	file, err := os.Create(outcomesPath)
	if err != nil {
		return ArtifactPaths{}, fmt.Errorf("create outcomes csv: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	header := []string{
		"game_id",
		"source",
		"external_id",
		"sport",
		"home_team",
		"away_team",
		"book_key",
		"market_key",
		"commence_time_utc",
		"opening_captured_at_utc",
		"closing_captured_at_utc",
		"opening_home_probability",
		"closing_home_probability",
		"predicted_home_probability",
		"recommended_side",
		"opening_side_probability",
		"closing_side_probability",
		"model_edge",
		"clv_delta",
		"calibration_error",
		"kelly_fraction",
		"max_stake_fraction",
		"recommended_stake_fraction",
		"recommended_stake_dollars",
		"virtual_bankroll_balance_post",
		"actual_home_goals",
		"actual_away_goals",
		"actual_home_win",
		"outcome_calibration_error",
	}
	if err := writer.Write(header); err != nil {
		return ArtifactPaths{}, fmt.Errorf("write outcomes csv header: %w", err)
	}

	for _, outcome := range artifact.Outcomes {
		record := []string{
			strconv.FormatInt(outcome.GameID, 10),
			outcome.Source,
			outcome.ExternalID,
			string(outcome.Sport),
			outcome.HomeTeam,
			outcome.AwayTeam,
			outcome.BookKey,
			outcome.MarketKey,
			outcome.CommenceTimeUTC.Format("2006-01-02T15:04:05Z"),
			outcome.OpeningCapturedAtUTC.Format("2006-01-02T15:04:05Z"),
			outcome.ClosingCapturedAtUTC.Format("2006-01-02T15:04:05Z"),
			strconv.FormatFloat(outcome.OpeningHomeProbability, 'f', 6, 64),
			strconv.FormatFloat(outcome.ClosingHomeProbability, 'f', 6, 64),
			strconv.FormatFloat(outcome.PredictedHomeProbability, 'f', 6, 64),
			outcome.RecommendedSide,
			strconv.FormatFloat(outcome.OpeningSideProbability, 'f', 6, 64),
			strconv.FormatFloat(outcome.ClosingSideProbability, 'f', 6, 64),
			strconv.FormatFloat(outcome.ModelEdge, 'f', 6, 64),
			strconv.FormatFloat(outcome.CLVDelta, 'f', 6, 64),
			strconv.FormatFloat(outcome.CalibrationError, 'f', 6, 64),
			strconv.FormatFloat(outcome.KellyFraction, 'f', 6, 64),
			strconv.FormatFloat(outcome.MaxStakeFraction, 'f', 6, 64),
			strconv.FormatFloat(outcome.RecommendedStakeFraction, 'f', 6, 64),
			strconv.FormatFloat(outcome.RecommendedStakeDollars, 'f', 6, 64),
			strconv.FormatFloat(outcome.VirtualBankrollBalancePost, 'f', 6, 64),
			formatOptionalFloat(outcome.ActualHomeGoals),
			formatOptionalFloat(outcome.ActualAwayGoals),
			formatOptionalBool(outcome.ActualHomeWin),
			formatOptionalFloat(outcome.OutcomeCalibrationError),
		}
		if err := writer.Write(record); err != nil {
			return ArtifactPaths{}, fmt.Errorf("write outcomes csv row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return ArtifactPaths{}, fmt.Errorf("flush outcomes csv: %w", err)
	}

	return ArtifactPaths{PipelineJSON: pipelinePath, OutcomesCSV: outcomesPath}, nil
}

func formatOptionalFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', 6, 64)
}

func formatOptionalBool(value *bool) string {
	if value == nil {
		return ""
	}
	return strconv.FormatBool(*value)
}
