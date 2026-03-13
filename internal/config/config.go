package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultEnv              = "development"
	defaultHTTPAddr         = ":8080"
	defaultDatabaseURL      = "postgres://betbot:betbot-dev-password@localhost:5432/betbot?sslmode=disable"
	defaultDBConnectTimeout = 5 * time.Second
	defaultDBMaxConns       = int32(8)
	defaultDBMinConns       = int32(1)
	defaultDBMaxConnLife    = 30 * time.Minute
	defaultDBMaxConnIdle    = 10 * time.Minute
	defaultDBHealthPeriod   = 30 * time.Second
	defaultOddsAPIBaseURL   = "https://api.the-odds-api.com/v4"
	defaultOddsAPIRegions   = "us"
	defaultOddsAPIOddsFmt   = "american"
	defaultOddsAPIDateFmt   = "iso"
	defaultOddsAPITimeout   = 10 * time.Second
	defaultOddsPollInterval = 5 * time.Minute
	defaultOddsPollingOn    = true
	defaultOddsRateLimit    = 750 * time.Millisecond
	defaultOddsSource       = "the-odds-api"
	defaultRiverSchema      = "public"
	defaultRecentPollWindow = 20 * time.Minute
	oddsAPIKeyPlaceholder   = "TODO_SET_BETBOT_ODDS_API_KEY"
)

type Config struct {
	Env                 string
	HTTPAddr            string
	DatabaseURL         string
	DBConnectTimeout    time.Duration
	DBMaxConns          int32
	DBMinConns          int32
	DBMaxConnLifetime   time.Duration
	DBMaxConnIdleTime   time.Duration
	DBHealthCheckPeriod time.Duration
	OddsAPIKey          string
	OddsAPIBaseURL      string
	OddsAPISports       []string
	OddsAPIRegions      string
	OddsAPIMarkets      []string
	OddsAPIOddsFormat   string
	OddsAPIDateFormat   string
	OddsAPITimeout      time.Duration
	OddsPollingEnabled  bool
	OddsAPIPollInterval time.Duration
	OddsAPIRateLimit    time.Duration
	OddsAPISource       string
	RiverSchema         string
	RecentPollWindow    time.Duration
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
	maxConns, err := getInt32("BETBOT_DB_MAX_CONNS", defaultDBMaxConns)
	if err != nil {
		return Config{}, err
	}
	minConns, err := getInt32("BETBOT_DB_MIN_CONNS", defaultDBMinConns)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Env:                 getenv("BETBOT_ENV", defaultEnv),
		HTTPAddr:            getenv("BETBOT_HTTP_ADDR", defaultHTTPAddr),
		DatabaseURL:         getenv("BETBOT_DATABASE_URL", defaultDatabaseURL),
		DBConnectTimeout:    timeout,
		DBMaxConns:          maxConns,
		DBMinConns:          minConns,
		DBMaxConnLifetime:   maxConnLifetime,
		DBMaxConnIdleTime:   maxConnIdle,
		DBHealthCheckPeriod: healthPeriod,
		OddsAPIKey:          strings.TrimSpace(os.Getenv("BETBOT_ODDS_API_KEY")),
		OddsAPIBaseURL:      getenv("BETBOT_ODDS_API_BASE_URL", defaultOddsAPIBaseURL),
		OddsAPISports:       getCSV("BETBOT_ODDS_API_SPORTS", []string{"baseball_mlb", "basketball_nba", "icehockey_nhl", "americanfootball_nfl"}),
		OddsAPIRegions:      getenv("BETBOT_ODDS_API_REGIONS", defaultOddsAPIRegions),
		OddsAPIMarkets:      getCSV("BETBOT_ODDS_API_MARKETS", []string{"h2h", "spreads", "totals"}),
		OddsAPIOddsFormat:   getenv("BETBOT_ODDS_API_ODDS_FORMAT", defaultOddsAPIOddsFmt),
		OddsAPIDateFormat:   getenv("BETBOT_ODDS_API_DATE_FORMAT", defaultOddsAPIDateFmt),
		OddsAPITimeout:      oddsTimeout,
		OddsPollingEnabled:  oddsPollingEnabled,
		OddsAPIPollInterval: pollInterval,
		OddsAPIRateLimit:    rateLimit,
		OddsAPISource:       getenv("BETBOT_ODDS_API_SOURCE", defaultOddsSource),
		RiverSchema:         getenv("BETBOT_RIVER_SCHEMA", defaultRiverSchema),
		RecentPollWindow:    recentPollWindow,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("BETBOT_DATABASE_URL must not be empty")
	}
	if cfg.DBMinConns < 0 || cfg.DBMaxConns < 1 || cfg.DBMinConns > cfg.DBMaxConns {
		return Config{}, errors.New("invalid pgxpool bounds")
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

func IsUnresolvedOddsAPIKey(apiKey string) bool {
	return strings.EqualFold(strings.TrimSpace(apiKey), oddsAPIKeyPlaceholder)
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
