package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"betbot/internal/backtest"
	"betbot/internal/config"
	"betbot/internal/domain"
	"betbot/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	flags, err := parseFlags()
	if err != nil {
		log.Fatalf("parse flags: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := store.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("open pool: %v", err)
	}
	defer pool.Close()

	queries := store.New(pool)
	engine, err := backtest.NewEngine(queries, backtest.WithMoneyPuckStore(queries))
	if err != nil {
		log.Fatalf("create backtest engine: %v", err)
	}

	var artifact backtest.PipelineArtifact
	if flags.Mode == "outcome" {
		artifact, err = engine.RunOutcomeBacktest(ctx, flags.OutcomeRunConfig)
	} else {
		artifact, err = engine.Run(ctx, flags.RunConfig)
	}
	if err != nil {
		log.Fatalf("run backtest: %v", err)
	}

	outputDir := flags.OutputDir
	if !filepath.IsAbs(outputDir) {
		cwd, _ := os.Getwd()
		outputDir = filepath.Join(cwd, outputDir)
	}
	paths, err := backtest.WriteArtifacts(outputDir, artifact)
	if err != nil {
		log.Fatalf("write artifacts: %v", err)
	}

	guardrails := backtest.EvaluateGuardrails(artifact, backtest.GuardrailConfig{
		MinimumSamples:     50,
		MinimumPositiveCLV: 0.50,
		MaximumCalibMAE:    0.07,
		MaximumCalibBrier:  0.01,
	})

	fmt.Printf("backtest complete\n")
	fmt.Printf("mode: %s\n", artifact.BacktestMode)
	fmt.Printf("clv_mode: %s\n", artifact.CLVMode)
	fmt.Printf("rolling_window: %d\n", artifact.RollingWindow)
	fmt.Printf("rows: %d\n", len(artifact.Outcomes))
	fmt.Printf("persisted_predictions: %d\n", artifact.PersistedPredictionRows)
	fmt.Printf("mean_clv: %.6f\n", artifact.CLV.MeanCLV)
	fmt.Printf("positive_clv_rate: %.4f\n", artifact.CLV.PositiveCLVRate)
	fmt.Printf("calibration_mae: %.6f\n", artifact.Calibration.MeanAbsoluteError)
	fmt.Printf("brier_score: %.6f\n", artifact.Calibration.BrierScore)
	fmt.Printf("walk_forward_splits: %d\n", len(artifact.WalkForward))
	fmt.Printf("guardrails_pass: %t\n", guardrails.Pass)
	if len(guardrails.Findings) > 0 {
		fmt.Printf("guardrail_findings: %s\n", strings.Join(guardrails.Findings, ","))
	}
	if len(artifact.SeasonCalibrations) > 0 {
		fmt.Printf("season_calibrations:\n")
		for _, sc := range artifact.SeasonCalibrations {
			fmt.Printf("  %d: samples=%d real_rate=%.3f mae=%.4f brier=%.4f home_win=%.3f predicted=%.3f\n",
				sc.Season, sc.Samples, sc.RealFeatureRate, sc.MeanAbsoluteError, sc.BrierScore,
				sc.HomeWinRate, sc.PredictedHomeWinRate)
		}
	}
	fmt.Printf("pipeline_report: %s\n", paths.PipelineJSON)
	fmt.Printf("outcomes_csv: %s\n", paths.OutcomesCSV)
}

type parsedFlags struct {
	Mode             string
	RunConfig        backtest.RunConfig
	OutcomeRunConfig backtest.OutcomeRunConfig
	OutputDir        string
}

func parseFlags() (parsedFlags, error) {
	var mode string
	var sportFlag string
	var seasonFlag int
	var seasonStart int
	var seasonEnd int
	var marketKey string
	var rowLimit int
	var rollingWindow int
	var modelVersion string
	var outputDir string
	var trainWindow int
	var validationWindow int
	var step int

	flag.StringVar(&mode, "mode", "odds", "backtest mode: odds|outcome")
	flag.StringVar(&sportFlag, "sport", "", "sport filter: MLB|NBA|NHL|NFL (default all)")
	flag.IntVar(&seasonFlag, "season", 0, "season year filter (default all)")
	flag.IntVar(&seasonStart, "season-start", 0, "outcome mode: start season (inclusive)")
	flag.IntVar(&seasonEnd, "season-end", 0, "outcome mode: end season (inclusive)")
	flag.StringVar(&marketKey, "market", "h2h", "market key filter")
	flag.IntVar(&rowLimit, "row-limit", 5000, "max replay rows")
	flag.IntVar(&rollingWindow, "rolling-window", 20, "NHL rolling window size for feature computation")
	flag.StringVar(&modelVersion, "model-version", "baseline-v1", "model version written to model_predictions")
	flag.StringVar(&outputDir, "output-dir", "artifacts/backtest", "artifact output directory")
	flag.IntVar(&trainWindow, "train-window", 64, "walk-forward train window length")
	flag.IntVar(&validationWindow, "validation-window", 16, "walk-forward validation window length")
	flag.IntVar(&step, "step", 16, "walk-forward step size")
	flag.Parse()

	if mode != "odds" && mode != "outcome" {
		return parsedFlags{}, fmt.Errorf("invalid mode %q (must be odds or outcome)", mode)
	}

	runCfg := backtest.RunConfig{
		MarketKey:             strings.TrimSpace(marketKey),
		RowLimit:              rowLimit,
		RollingWindow:         rollingWindow,
		ModelVersion:          strings.TrimSpace(modelVersion),
		WalkForwardTrain:      trainWindow,
		WalkForwardValidation: validationWindow,
		WalkForwardStep:       step,
	}

	if sportFlag != "" {
		sport, err := parseSport(sportFlag)
		if err != nil {
			return parsedFlags{}, err
		}
		runCfg.Sport = &sport
	}
	if seasonFlag > 0 {
		runCfg.Season = &seasonFlag
	}

	outcomeCfg := backtest.OutcomeRunConfig{
		RollingWindow:         rollingWindow,
		ModelVersion:          strings.TrimSpace(modelVersion),
		WalkForwardTrain:      trainWindow,
		WalkForwardValidation: validationWindow,
		WalkForwardStep:       step,
	}
	if seasonStart > 0 {
		outcomeCfg.SeasonStart = &seasonStart
	}
	if seasonEnd > 0 {
		outcomeCfg.SeasonEnd = &seasonEnd
	}

	if strings.TrimSpace(outputDir) == "" {
		return parsedFlags{}, errors.New("output-dir cannot be empty")
	}

	return parsedFlags{Mode: mode, RunConfig: runCfg, OutcomeRunConfig: outcomeCfg, OutputDir: strings.TrimSpace(outputDir)}, nil
}

func parseSport(raw string) (domain.Sport, error) {
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	sport := domain.Sport(normalized)
	if _, ok := domain.DefaultSportRegistry().Get(sport); !ok {
		return "", fmt.Errorf("unsupported sport %q", raw)
	}
	return sport, nil
}
