package config

import (
	"errors"
	"os"
	"time"
)

const (
	defaultEnv              = "development"
	defaultHTTPAddr         = ":8080"
	defaultDatabaseURL      = "postgres://betbot:betbot-dev-password@localhost:5432/betbot?sslmode=disable"
	defaultDBConnectTimeout = 5 * time.Second
)

type Config struct {
	Env              string
	HTTPAddr         string
	DatabaseURL      string
	DBConnectTimeout time.Duration
}

func Load() (Config, error) {
	timeout := defaultDBConnectTimeout
	if value := os.Getenv("BETBOT_DB_CONNECT_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, err
		}
		timeout = parsed
	}

	cfg := Config{
		Env:              getenv("BETBOT_ENV", defaultEnv),
		HTTPAddr:         getenv("BETBOT_HTTP_ADDR", defaultHTTPAddr),
		DatabaseURL:      getenv("BETBOT_DATABASE_URL", defaultDatabaseURL),
		DBConnectTimeout: timeout,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("BETBOT_DATABASE_URL must not be empty")
	}

	return cfg, nil
}

func getenv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
