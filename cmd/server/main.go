package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"betbot/internal/config"
	"betbot/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app := server.New(cfg)
	if err := app.Run(ctx); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
