package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("BETBOT_ENV", "")
	t.Setenv("BETBOT_HTTP_ADDR", "")
	t.Setenv("BETBOT_DATABASE_URL", "")
	t.Setenv("BETBOT_DB_CONNECT_TIMEOUT", "")

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

	if len(cfg.OddsAPISports) != 4 {
		t.Fatalf("OddsAPISports len = %d, want 4", len(cfg.OddsAPISports))
	}
}
