package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("BETBOT_ENV", "")
	t.Setenv("BETBOT_HTTP_ADDR", "")
	t.Setenv("BETBOT_DATABASE_URL", "")
	t.Setenv("BETBOT_DB_CONNECT_TIMEOUT", "")
	t.Setenv("BETBOT_ODDS_POLLING_ENABLED", "")

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
