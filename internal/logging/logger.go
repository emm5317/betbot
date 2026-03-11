package logging

import (
	"log/slog"
	"os"
)

func New(env string) *slog.Logger {
	opts := &slog.HandlerOptions{}
	if env == "development" {
		opts.Level = slog.LevelDebug
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}

	opts.Level = slog.LevelInfo
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
