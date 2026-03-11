package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"betbot/internal/config"
	"betbot/internal/logging"
	"betbot/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app, err := worker.New(ctx, cfg, logging.New(cfg.Env))
	if err != nil {
		log.Fatalf("new worker: %v", err)
	}

	if err := app.Run(ctx); err != nil {
		log.Fatalf("run worker: %v", err)
	}
}
