package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultEnv                                = "development"
	defaultHTTPAddr                           = ":8080"
	defaultDatabaseURL                        = "postgres://betbot:betbot-dev-password@localhost:5432/betbot?sslmode=disable"
	defaultDBConnectTimeout                   = 5 * time.Second
	defaultDBMaxConns                         = int32(8)
	defaultDBMinConns                         = int32(1)
	defaultDBMaxConnLife                      = 30 * time.Minute
	defaultDBMaxConnIdle                      = 10 * time.Minute
	defaultDBHealthPeriod                     = 30 * time.Second
	defaultOddsAPIBaseURL                     = "https://api.the-odds-api.com/v4"
	defaultOddsAPIRegions                     = "us"
	defaultOddsAPIOddsFmt                     = "american"
	defaultOddsAPIDateFmt                     = "iso"
	defaultOddsAPITimeout                     = 10 * time.Second
	defaultOddsPollInterval                   = 5 * time.Minute
	defaultOddsPollingOn                      = true
	defaultOddsRateLimit                      = 750 * time.Millisecond
	defaultOddsSource                         = "the-odds-api"
	defaultRiverSchema                        = "public"
	defaultRecentPollWindow                   = 20 * time.Minute
	defaultEVThreshold                        = 0.02
	defaultKellyFraction                      = 0.0
	defaultMaxBetFraction                     = 0.0
	defaultCorrelationMaxPicksPerGame         = 1
	defaultCorrelationMaxStakeFractionPerGame = 0.03
	defaultCorrelationMaxPicksPerSportDay     = 0
	maxCorrelationMaxPicksPerGame             = 25
	maxCorrelationMaxPicksPerSportDay         = 500
	defaultDailyLossStop                      = 0.05
	defaultWeeklyLossStop                     = 0.10
	defaultDrawdownBreaker                    = 0.15
	executionAdapterPaper                     = "paper"
	executionAdapterDraftKings                = "draftkings"
	executionAdapterFanDuel                   = "fanduel"
	executionAdapterBetMGM                    = "betmgm"
	executionAdapterPinnacle                  = "pinnacle"
	defaultExecutionAdapter                   = executionAdapterPaper
	oddsAPIKeyPlaceholder                     = "TODO_SET_BETBOT_ODDS_API_KEY"
)

type Config struct {
	Env                                string
	HTTPAddr                           string
	DatabaseURL                        string
	DBConnectTimeout                   time.Duration
	DBMaxConns                         int32
	DBMinConns                         int32
	DBMaxConnLifetime                  time.Duration
	DBMaxConnIdleTime                  time.Duration
	DBHealthCheckPeriod                time.Duration
	OddsAPIKey                         string
	OddsAPIBaseURL                     string
	OddsAPISports                      []string
	OddsAPIRegions                     string
	OddsAPIMarkets                     []string
	OddsAPIOddsFormat                  string
	OddsAPIDateFormat                  string
	OddsAPITimeout                     time.Duration
	OddsPollingEnabled                 bool
	OddsAPIPollInterval                time.Duration
	OddsAPIRateLimit                   time.Duration
	OddsAPISource                      string
	RiverSchema                        string
	RecentPollWindow                   time.Duration
	EVThreshold                        float64
	KellyFraction                      float64
	MaxBetFraction                     float64
	CorrelationMaxPicksPerGame         int
	CorrelationMaxStakeFractionPerGame float64
	CorrelationMaxPicksPerSportDay     int
	DailyLossStop                      float64
	WeeklyLossStop                     float64
	DrawdownBreaker                    float64
	PaperMode                          bool
	ExecutionAdapter                   string
	AutoPlacementEnabled               bool
}

func Load() (Config, error) {
	timeout, err := getDuration("BETBOT_DB_CONNECT_TIMEOUT", defaultDBConnectTimeout)
	if err != nil {
		return Config{}, err
	}
	maxConnLifetime, err := getDuration("BETBOT_DB_MAX_CONN_LIFETIME", defaultDBMaxConnLife)
	if err != nil {
		return Config{}, err
	}
	maxConnIdle, err := getDuration("BETBOT_DB_MAX_CONN_IDLE_TIME", defaultDBMaxConnIdle)
	if err != nil {
		return Config{}, err
	}
	healthPeriod, err := getDuration("BETBOT_DB_HEALTH_CHECK_PERIOD", defaultDBHealthPeriod)
	if err != nil {
		return Config{}, err
	}
	oddsTimeout, err := getDuration("BETBOT_ODDS_API_TIMEOUT", defaultOddsAPITimeout)
	if err != nil {
		return Config{}, err
	}
	pollInterval, err := getDuration("BETBOT_ODDS_API_POLL_INTERVAL", defaultOddsPollInterval)
	if err != nil {
		return Config{}, err
	}
	rateLimit, err := getDuration("BETBOT_ODDS_API_RATE_LIMIT_INTERVAL", defaultOddsRateLimit)
	if err != nil {
		return Config{}, err
	}
	oddsPollingEnabled, err := getBool("BETBOT_ODDS_POLLING_ENABLED", defaultOddsPollingOn)
	if err != nil {
		return Config{}, err
	}
	recentPollWindow, err := getDuration("BETBOT_RECENT_POLL_WINDOW", defaultRecentPollWindow)
	if err != nil {
		return Config{}, err
	}
	evThreshold, err := getFloat64("BETBOT_EV_THRESHOLD", defaultEVThreshold)
	if err != nil {
		return Config{}, err
	}
	kellyFraction, err := getFloat64("BETBOT_KELLY_FRACTION", defaultKellyFraction)
	if err != nil {
		return Config{}, err
	}
	maxBetFraction, err := getFloat64("BETBOT_MAX_BET_FRACTION", defaultMaxBetFraction)
	if err != nil {
		return Config{}, err
	}
	correlationMaxPicksPerGame, err := getInt("BETBOT_CORRELATION_MAX_PICKS_PER_GAME", defaultCorrelationMaxPicksPerGame)
	if err != nil {
		return Config{}, err
	}
	correlationMaxStakeFractionPerGame, err := getFloat64("BETBOT_CORRELATION_MAX_STAKE_FRACTION_PER_GAME", defaultCorrelationMaxStakeFractionPerGame)
	if err != nil {
		return Config{}, err
	}
	correlationMaxPicksPerSportDay, err := getInt("BETBOT_CORRELATION_MAX_PICKS_PER_SPORT_DAY", defaultCorrelationMaxPicksPerSportDay)
	if err != nil {
		return Config{}, err
	}
	dailyLossStop, err := getFloat64("BETBOT_DAILY_LOSS_STOP", defaultDailyLossStop)
	if err != nil {
		return Config{}, err
	}
	weeklyLossStop, err := getFloat64("BETBOT_WEEKLY_LOSS_STOP", defaultWeeklyLossStop)
	if err != nil {
		return Config{}, err
	}
	drawdownBreaker, err := getFloat64("BETBOT_DRAWDOWN_BREAKER", defaultDrawdownBreaker)
	if err != nil {
		return Config{}, err
	}
	paperMode, err := getBool("BETBOT_PAPER_MODE", true)
	if err != nil {
		return Config{}, err
	}
	executionAdapter, err := resolveExecutionAdapter(paperMode)
	if err != nil {
		return Config{}, err
	}
	autoPlacementEnabled, err := getBool("BETBOT_AUTO_PLACEMENT_ENABLED", paperMode)
	if err != nil {
		return Config{}, err
	}
	maxConns, err := getInt32("BETBOT_DB_MAX_CONNS", defaultDBMaxConns)
	if err != nil {
		return Config{}, err
	}
	minConns, err := getInt32("BETBOT_DB_MIN_CONNS", defaultDBMinConns)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Env:                                getenv("BETBOT_ENV", defaultEnv),
		HTTPAddr:                           getenv("BETBOT_HTTP_ADDR", defaultHTTPAddr),
		DatabaseURL:                        getenv("BETBOT_DATABASE_URL", defaultDatabaseURL),
		DBConnectTimeout:                   timeout,
		DBMaxConns:                         maxConns,
		DBMinConns:                         minConns,
		DBMaxConnLifetime:                  maxConnLifetime,
		DBMaxConnIdleTime:                  maxConnIdle,
		DBHealthCheckPeriod:                healthPeriod,
		OddsAPIKey:                         strings.TrimSpace(os.Getenv("BETBOT_ODDS_API_KEY")),
		OddsAPIBaseURL:                     getenv("BETBOT_ODDS_API_BASE_URL", defaultOddsAPIBaseURL),
		OddsAPISports:                      getCSV("BETBOT_ODDS_API_SPORTS", []string{"baseball_mlb", "basketball_nba", "icehockey_nhl", "americanfootball_nfl"}),
		OddsAPIRegions:                     getenv("BETBOT_ODDS_API_REGIONS", defaultOddsAPIRegions),
		OddsAPIMarkets:                     getCSV("BETBOT_ODDS_API_MARKETS", []string{"h2h", "spreads", "totals"}),
		OddsAPIOddsFormat:                  getenv("BETBOT_ODDS_API_ODDS_FORMAT", defaultOddsAPIOddsFmt),
		OddsAPIDateFormat:                  getenv("BETBOT_ODDS_API_DATE_FORMAT", defaultOddsAPIDateFmt),
		OddsAPITimeout:                     oddsTimeout,
		OddsPollingEnabled:                 oddsPollingEnabled,
		OddsAPIPollInterval:                pollInterval,
		OddsAPIRateLimit:                   rateLimit,
		OddsAPISource:                      getenv("BETBOT_ODDS_API_SOURCE", defaultOddsSource),
		RiverSchema:                        getenv("BETBOT_RIVER_SCHEMA", defaultRiverSchema),
		RecentPollWindow:                   recentPollWindow,
		EVThreshold:                        evThreshold,
		KellyFraction:                      kellyFraction,
		MaxBetFraction:                     maxBetFraction,
		CorrelationMaxPicksPerGame:         correlationMaxPicksPerGame,
		CorrelationMaxStakeFractionPerGame: correlationMaxStakeFractionPerGame,
		CorrelationMaxPicksPerSportDay:     correlationMaxPicksPerSportDay,
		DailyLossStop:                      dailyLossStop,
		WeeklyLossStop:                     weeklyLossStop,
		DrawdownBreaker:                    drawdownBreaker,
		PaperMode:                          paperMode,
		ExecutionAdapter:                   executionAdapter,
		AutoPlacementEnabled:               autoPlacementEnabled,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("BETBOT_DATABASE_URL must not be empty")
	}
	if cfg.DBMinConns < 0 || cfg.DBMaxConns < 1 || cfg.DBMinConns > cfg.DBMaxConns {
		return Config{}, errors.New("invalid pgxpool bounds")
	}
	if math.IsNaN(cfg.EVThreshold) || math.IsInf(cfg.EVThreshold, 0) || cfg.EVThreshold <= 0 || cfg.EVThreshold > 1 {
		return Config{}, errors.New("BETBOT_EV_THRESHOLD must be finite in (0,1]")
	}
	if math.IsNaN(cfg.KellyFraction) || math.IsInf(cfg.KellyFraction, 0) || cfg.KellyFraction < 0 || cfg.KellyFraction > 1 {
		return Config{}, errors.New("BETBOT_KELLY_FRACTION must be finite in [0,1]")
	}
	if math.IsNaN(cfg.MaxBetFraction) || math.IsInf(cfg.MaxBetFraction, 0) || cfg.MaxBetFraction < 0 || cfg.MaxBetFraction > 1 {
		return Config{}, errors.New("BETBOT_MAX_BET_FRACTION must be finite in [0,1]")
	}
	if cfg.CorrelationMaxPicksPerGame < 1 || cfg.CorrelationMaxPicksPerGame > maxCorrelationMaxPicksPerGame {
		return Config{}, fmt.Errorf("BETBOT_CORRELATION_MAX_PICKS_PER_GAME must be in [1,%d]", maxCorrelationMaxPicksPerGame)
	}
	if math.IsNaN(cfg.CorrelationMaxStakeFractionPerGame) || math.IsInf(cfg.CorrelationMaxStakeFractionPerGame, 0) || cfg.CorrelationMaxStakeFractionPerGame <= 0 || cfg.CorrelationMaxStakeFractionPerGame > 1 {
		return Config{}, errors.New("BETBOT_CORRELATION_MAX_STAKE_FRACTION_PER_GAME must be finite in (0,1]")
	}
	if cfg.CorrelationMaxPicksPerSportDay < 0 || cfg.CorrelationMaxPicksPerSportDay > maxCorrelationMaxPicksPerSportDay {
		return Config{}, fmt.Errorf("BETBOT_CORRELATION_MAX_PICKS_PER_SPORT_DAY must be in [0,%d]", maxCorrelationMaxPicksPerSportDay)
	}
	if math.IsNaN(cfg.DailyLossStop) || math.IsInf(cfg.DailyLossStop, 0) || cfg.DailyLossStop < 0 || cfg.DailyLossStop > 1 {
		return Config{}, errors.New("BETBOT_DAILY_LOSS_STOP must be finite in [0,1]")
	}
	if math.IsNaN(cfg.WeeklyLossStop) || math.IsInf(cfg.WeeklyLossStop, 0) || cfg.WeeklyLossStop < 0 || cfg.WeeklyLossStop > 1 {
		return Config{}, errors.New("BETBOT_WEEKLY_LOSS_STOP must be finite in [0,1]")
	}
	if math.IsNaN(cfg.DrawdownBreaker) || math.IsInf(cfg.DrawdownBreaker, 0) || cfg.DrawdownBreaker < 0 || cfg.DrawdownBreaker > 1 {
		return Config{}, errors.New("BETBOT_DRAWDOWN_BREAKER must be finite in [0,1]")
	}

	return cfg, nil
}

func (c Config) OddsPollingRuntime() (bool, string) {
	if !c.OddsPollingEnabled {
		return false, "disabled-by-config"
	}
	if IsUnresolvedOddsAPIKey(c.OddsAPIKey) {
		return false, "unresolved-placeholder-api-key"
	}
	return true, ""
}

func (c Config) AutoPlacementRuntime() (bool, string) {
	if c.AutoPlacementEnabled {
		return true, ""
	}
	if c.PaperMode {
		return false, "disabled-by-config"
	}
	return false, "disabled-in-live-mode"
}

func IsUnresolvedOddsAPIKey(apiKey string) bool {
	return strings.EqualFold(strings.TrimSpace(apiKey), oddsAPIKeyPlaceholder)
}

func resolveExecutionAdapter(paperMode bool) (string, error) {
	value := NormalizeExecutionAdapter(os.Getenv("BETBOT_EXECUTION_ADAPTER"))
	if value == "" {
		if paperMode {
			return defaultExecutionAdapter, nil
		}
		return "", errors.New("BETBOT_EXECUTION_ADAPTER must be set when BETBOT_PAPER_MODE=false")
	}
	if !isSupportedExecutionAdapter(value) {
		return "", fmt.Errorf("BETBOT_EXECUTION_ADAPTER must be one of paper, draftkings, fanduel, betmgm, pinnacle")
	}
	if paperMode && value != executionAdapterPaper {
		return "", errors.New("BETBOT_EXECUTION_ADAPTER must be \"paper\" when BETBOT_PAPER_MODE=true")
	}
	if !paperMode && value == executionAdapterPaper {
		return "", errors.New("BETBOT_EXECUTION_ADAPTER must be a live adapter when BETBOT_PAPER_MODE=false")
	}
	return value, nil
}

func NormalizeExecutionAdapter(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isSupportedExecutionAdapter(value string) bool {
	switch NormalizeExecutionAdapter(value) {
	case executionAdapterPaper, executionAdapterDraftKings, executionAdapterFanDuel, executionAdapterBetMGM, executionAdapterPinnacle:
		return true
	default:
		return false
	}
}

func getenv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	if value := os.Getenv(key); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", key, err)
		}
		return parsed, nil
	}
	return fallback, nil
}

func getInt32(key string, fallback int32) (int32, error) {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", key, err)
		}
		return int32(parsed), nil
	}
	return fallback, nil
}

func getInt(key string, fallback int) (int, error) {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, fmt.Errorf("%s: %w", key, err)
		}
		return parsed, nil
	}
	return fallback, nil
}

func getBool(key string, fallback bool) (bool, error) {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false, fmt.Errorf("%s: %w", key, err)
		}
		return parsed, nil
	}
	return fallback, nil
}

func getFloat64(key string, fallback float64) (float64, error) {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", key, err)
		}
		return parsed, nil
	}
	return fallback, nil
}

func getCSV(key string, fallback []string) []string {
	value := os.Getenv(key)
	if strings.TrimSpace(value) == "" {
		return append([]string(nil), fallback...)
	}

	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	if len(items) == 0 {
		return append([]string(nil), fallback...)
	}
	return items
}
