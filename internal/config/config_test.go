package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("BETBOT_ENV", "")
	t.Setenv("BETBOT_HTTP_ADDR", "")
	t.Setenv("BETBOT_DATABASE_URL", "")
	t.Setenv("BETBOT_DB_CONNECT_TIMEOUT", "")
	t.Setenv("BETBOT_ODDS_POLLING_ENABLED", "")
	t.Setenv("BETBOT_EV_THRESHOLD", "")
	t.Setenv("BETBOT_KELLY_FRACTION", "")
	t.Setenv("BETBOT_MAX_BET_FRACTION", "")
	t.Setenv("BETBOT_CORRELATION_MAX_PICKS_PER_GAME", "")
	t.Setenv("BETBOT_CORRELATION_MAX_STAKE_FRACTION_PER_GAME", "")
	t.Setenv("BETBOT_CORRELATION_MAX_PICKS_PER_SPORT_DAY", "")
	t.Setenv("BETBOT_DAILY_LOSS_STOP", "")
	t.Setenv("BETBOT_WEEKLY_LOSS_STOP", "")
	t.Setenv("BETBOT_DRAWDOWN_BREAKER", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Env != defaultEnv {
		t.Fatalf("Env = %q, want %q", cfg.Env, defaultEnv)
	}

	if cfg.HTTPAddr != defaultHTTPAddr {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, defaultHTTPAddr)
	}

	if cfg.DatabaseURL != defaultDatabaseURL {
		t.Fatalf("DatabaseURL = %q, want %q", cfg.DatabaseURL, defaultDatabaseURL)
	}

	if cfg.DBConnectTimeout != defaultDBConnectTimeout {
		t.Fatalf("DBConnectTimeout = %s, want %s", cfg.DBConnectTimeout, defaultDBConnectTimeout)
	}

	if cfg.DBMaxConns != defaultDBMaxConns {
		t.Fatalf("DBMaxConns = %d, want %d", cfg.DBMaxConns, defaultDBMaxConns)
	}

	if cfg.DBMinConns != defaultDBMinConns {
		t.Fatalf("DBMinConns = %d, want %d", cfg.DBMinConns, defaultDBMinConns)
	}

	if cfg.OddsAPIPollInterval != defaultOddsPollInterval {
		t.Fatalf("OddsAPIPollInterval = %s, want %s", cfg.OddsAPIPollInterval, defaultOddsPollInterval)
	}

	if !cfg.OddsPollingEnabled {
		t.Fatalf("OddsPollingEnabled = %t, want true", cfg.OddsPollingEnabled)
	}

	if cfg.EVThreshold != defaultEVThreshold {
		t.Fatalf("EVThreshold = %.3f, want %.3f", cfg.EVThreshold, defaultEVThreshold)
	}
	if cfg.KellyFraction != defaultKellyFraction {
		t.Fatalf("KellyFraction = %.3f, want %.3f", cfg.KellyFraction, defaultKellyFraction)
	}
	if cfg.MaxBetFraction != defaultMaxBetFraction {
		t.Fatalf("MaxBetFraction = %.3f, want %.3f", cfg.MaxBetFraction, defaultMaxBetFraction)
	}
	if cfg.CorrelationMaxPicksPerGame != defaultCorrelationMaxPicksPerGame {
		t.Fatalf("CorrelationMaxPicksPerGame = %d, want %d", cfg.CorrelationMaxPicksPerGame, defaultCorrelationMaxPicksPerGame)
	}
	if cfg.CorrelationMaxStakeFractionPerGame != defaultCorrelationMaxStakeFractionPerGame {
		t.Fatalf("CorrelationMaxStakeFractionPerGame = %.3f, want %.3f", cfg.CorrelationMaxStakeFractionPerGame, defaultCorrelationMaxStakeFractionPerGame)
	}
	if cfg.CorrelationMaxPicksPerSportDay != defaultCorrelationMaxPicksPerSportDay {
		t.Fatalf("CorrelationMaxPicksPerSportDay = %d, want %d", cfg.CorrelationMaxPicksPerSportDay, defaultCorrelationMaxPicksPerSportDay)
	}
	if cfg.DailyLossStop != defaultDailyLossStop {
		t.Fatalf("DailyLossStop = %.3f, want %.3f", cfg.DailyLossStop, defaultDailyLossStop)
	}
	if cfg.WeeklyLossStop != defaultWeeklyLossStop {
		t.Fatalf("WeeklyLossStop = %.3f, want %.3f", cfg.WeeklyLossStop, defaultWeeklyLossStop)
	}
	if cfg.DrawdownBreaker != defaultDrawdownBreaker {
		t.Fatalf("DrawdownBreaker = %.3f, want %.3f", cfg.DrawdownBreaker, defaultDrawdownBreaker)
	}

	if len(cfg.OddsAPISports) != 4 {
		t.Fatalf("OddsAPISports len = %d, want 4", len(cfg.OddsAPISports))
	}
}

func TestLoadOddsPollingEnabled(t *testing.T) {
	t.Setenv("BETBOT_ODDS_POLLING_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.OddsPollingEnabled {
		t.Fatalf("OddsPollingEnabled = %t, want false", cfg.OddsPollingEnabled)
	}
}

func TestLoadOddsPollingEnabledInvalid(t *testing.T) {
	t.Setenv("BETBOT_ODDS_POLLING_ENABLED", "not-a-bool")

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid BETBOT_ODDS_POLLING_ENABLED")
	}
}

func TestLoadEVThresholdOverride(t *testing.T) {
	t.Setenv("BETBOT_EV_THRESHOLD", "0.035")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.EVThreshold != 0.035 {
		t.Fatalf("EVThreshold = %.3f, want 0.035", cfg.EVThreshold)
	}
}

func TestLoadEVThresholdInvalid(t *testing.T) {
	t.Setenv("BETBOT_EV_THRESHOLD", "0")

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid BETBOT_EV_THRESHOLD")
	}
}

func TestLoadKellyAndMaxBetOverrides(t *testing.T) {
	t.Setenv("BETBOT_KELLY_FRACTION", "0.22")
	t.Setenv("BETBOT_MAX_BET_FRACTION", "0.018")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.KellyFraction != 0.22 {
		t.Fatalf("KellyFraction = %.3f, want 0.22", cfg.KellyFraction)
	}
	if cfg.MaxBetFraction != 0.018 {
		t.Fatalf("MaxBetFraction = %.3f, want 0.018", cfg.MaxBetFraction)
	}
}

func TestLoadKellyAndMaxBetInvalid(t *testing.T) {
	t.Setenv("BETBOT_KELLY_FRACTION", "1.5")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid BETBOT_KELLY_FRACTION")
	}

	t.Setenv("BETBOT_KELLY_FRACTION", "")
	t.Setenv("BETBOT_MAX_BET_FRACTION", "-0.1")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid BETBOT_MAX_BET_FRACTION")
	}
}

func TestLoadCorrelationOverrides(t *testing.T) {
	t.Setenv("BETBOT_CORRELATION_MAX_PICKS_PER_GAME", "2")
	t.Setenv("BETBOT_CORRELATION_MAX_STAKE_FRACTION_PER_GAME", "0.06")
	t.Setenv("BETBOT_CORRELATION_MAX_PICKS_PER_SPORT_DAY", "8")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.CorrelationMaxPicksPerGame != 2 {
		t.Fatalf("CorrelationMaxPicksPerGame = %d, want 2", cfg.CorrelationMaxPicksPerGame)
	}
	if cfg.CorrelationMaxStakeFractionPerGame != 0.06 {
		t.Fatalf("CorrelationMaxStakeFractionPerGame = %.3f, want 0.06", cfg.CorrelationMaxStakeFractionPerGame)
	}
	if cfg.CorrelationMaxPicksPerSportDay != 8 {
		t.Fatalf("CorrelationMaxPicksPerSportDay = %d, want 8", cfg.CorrelationMaxPicksPerSportDay)
	}
}

func TestLoadCorrelationInvalid(t *testing.T) {
	t.Setenv("BETBOT_CORRELATION_MAX_PICKS_PER_GAME", "0")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid BETBOT_CORRELATION_MAX_PICKS_PER_GAME")
	}

	t.Setenv("BETBOT_CORRELATION_MAX_PICKS_PER_GAME", "")
	t.Setenv("BETBOT_CORRELATION_MAX_STAKE_FRACTION_PER_GAME", "NaN")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid BETBOT_CORRELATION_MAX_STAKE_FRACTION_PER_GAME")
	}

	t.Setenv("BETBOT_CORRELATION_MAX_STAKE_FRACTION_PER_GAME", "")
	t.Setenv("BETBOT_CORRELATION_MAX_PICKS_PER_SPORT_DAY", "-1")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid BETBOT_CORRELATION_MAX_PICKS_PER_SPORT_DAY")
	}
}

func TestLoadCircuitBreakerOverrides(t *testing.T) {
	t.Setenv("BETBOT_DAILY_LOSS_STOP", "0.08")
	t.Setenv("BETBOT_WEEKLY_LOSS_STOP", "0.16")
	t.Setenv("BETBOT_DRAWDOWN_BREAKER", "0.22")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DailyLossStop != 0.08 {
		t.Fatalf("DailyLossStop = %.3f, want 0.08", cfg.DailyLossStop)
	}
	if cfg.WeeklyLossStop != 0.16 {
		t.Fatalf("WeeklyLossStop = %.3f, want 0.16", cfg.WeeklyLossStop)
	}
	if cfg.DrawdownBreaker != 0.22 {
		t.Fatalf("DrawdownBreaker = %.3f, want 0.22", cfg.DrawdownBreaker)
	}
}

func TestLoadCircuitBreakerInvalid(t *testing.T) {
	t.Setenv("BETBOT_DAILY_LOSS_STOP", "-0.01")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid BETBOT_DAILY_LOSS_STOP")
	}

	t.Setenv("BETBOT_DAILY_LOSS_STOP", "")
	t.Setenv("BETBOT_WEEKLY_LOSS_STOP", "Infinity")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid BETBOT_WEEKLY_LOSS_STOP")
	}

	t.Setenv("BETBOT_WEEKLY_LOSS_STOP", "")
	t.Setenv("BETBOT_DRAWDOWN_BREAKER", "1.1")
	if _, err := Load(); err == nil {
		t.Fatal("Load() expected error for invalid BETBOT_DRAWDOWN_BREAKER")
	}
}

func TestOddsPollingRuntime(t *testing.T) {
	testCases := []struct {
		name    string
		cfg     Config
		enabled bool
		reason  string
	}{
		{
			name: "enabled with real key",
			cfg: Config{
				OddsPollingEnabled: true,
				OddsAPIKey:         "real-key",
			},
			enabled: true,
			reason:  "",
		},
		{
			name: "disabled by config",
			cfg: Config{
				OddsPollingEnabled: false,
				OddsAPIKey:         "real-key",
			},
			enabled: false,
			reason:  "disabled-by-config",
		},
		{
			name: "disabled by placeholder",
			cfg: Config{
				OddsPollingEnabled: true,
				OddsAPIKey:         "TODO_SET_BETBOT_ODDS_API_KEY",
			},
			enabled: false,
			reason:  "unresolved-placeholder-api-key",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			enabled, reason := tc.cfg.OddsPollingRuntime()
			if enabled != tc.enabled {
				t.Fatalf("OddsPollingRuntime() enabled = %t, want %t", enabled, tc.enabled)
			}
			if reason != tc.reason {
				t.Fatalf("OddsPollingRuntime() reason = %q, want %q", reason, tc.reason)
			}
		})
	}
}

func TestIsUnresolvedOddsAPIKey(t *testing.T) {
	if !IsUnresolvedOddsAPIKey("TODO_SET_BETBOT_ODDS_API_KEY") {
		t.Fatal("IsUnresolvedOddsAPIKey() = false, want true for placeholder")
	}
	if IsUnresolvedOddsAPIKey("real-key") {
		t.Fatal("IsUnresolvedOddsAPIKey() = true, want false for non-placeholder")
	}
}
