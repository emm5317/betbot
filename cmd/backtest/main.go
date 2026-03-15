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

	artifact, err := engine.Run(ctx, flags.RunConfig)
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
	fmt.Printf("rows: %d\n", len(artifact.Outcomes))
	fmt.Printf("persisted_predictions: %d\n", artifact.PersistedPredictionRows)
	fmt.Printf("mean_clv: %.6f\n", artifact.CLV.MeanCLV)
	fmt.Printf("positive_clv_rate: %.4f\n", artifact.CLV.PositiveCLVRate)
	fmt.Printf("calibration_mae: %.6f\n", artifact.Calibration.MeanAbsoluteError)
	fmt.Printf("walk_forward_splits: %d\n", len(artifact.WalkForward))
	fmt.Printf("guardrails_pass: %t\n", guardrails.Pass)
	if len(guardrails.Findings) > 0 {
		fmt.Printf("guardrail_findings: %s\n", strings.Join(guardrails.Findings, ","))
	}
	fmt.Printf("pipeline_report: %s\n", paths.PipelineJSON)
	fmt.Printf("outcomes_csv: %s\n", paths.OutcomesCSV)
}

type parsedFlags struct {
	RunConfig backtest.RunConfig
	OutputDir string
}

func parseFlags() (parsedFlags, error) {
	var sportFlag string
	var seasonFlag int
	var marketKey string
	var rowLimit int
	var modelVersion string
	var outputDir string
	var trainWindow int
	var validationWindow int
	var step int

	flag.StringVar(&sportFlag, "sport", "", "sport filter: MLB|NBA|NHL|NFL (default all)")
	flag.IntVar(&seasonFlag, "season", 0, "season year filter (default all)")
	flag.StringVar(&marketKey, "market", "h2h", "market key filter")
	flag.IntVar(&rowLimit, "row-limit", 5000, "max replay rows")
	flag.StringVar(&modelVersion, "model-version", "baseline-v1", "model version written to model_predictions")
	flag.StringVar(&outputDir, "output-dir", "artifacts/backtest", "artifact output directory")
	flag.IntVar(&trainWindow, "train-window", 64, "walk-forward train window length")
	flag.IntVar(&validationWindow, "validation-window", 16, "walk-forward validation window length")
	flag.IntVar(&step, "step", 16, "walk-forward step size")
	flag.Parse()

	runCfg := backtest.RunConfig{
		MarketKey:             strings.TrimSpace(marketKey),
		RowLimit:              rowLimit,
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

	if strings.TrimSpace(outputDir) == "" {
		return parsedFlags{}, errors.New("output-dir cannot be empty")
	}

	return parsedFlags{RunConfig: runCfg, OutputDir: strings.TrimSpace(outputDir)}, nil
}

func parseSport(raw string) (domain.Sport, error) {
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	sport := domain.Sport(normalized)
	if _, ok := domain.DefaultSportRegistry().Get(sport); !ok {
		return "", fmt.Errorf("unsupported sport %q", raw)
	}
	return sport, nil
}
