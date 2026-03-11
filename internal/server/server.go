package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	"betbot/internal/config"
)

type DBChecker func(ctx context.Context) error

type App struct {
	server *http.Server
}

func New(cfg config.Config) *App {
	handler := NewHandler(cfg, NewPostgresReachabilityChecker(cfg.DatabaseURL, cfg.DBConnectTimeout))

	return &App{
		server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		log.Printf("betbot server listening on %s", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}

		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func NewHandler(cfg config.Config, checkDB DBChecker) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		payload := map[string]string{
			"service": "betbot",
			"status":  "running",
			"env":     cfg.Env,
		}
		writeJSON(w, http.StatusOK, payload)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK
		payload := map[string]string{
			"service":  "betbot",
			"status":   "ok",
			"database": "ok",
			"env":      cfg.Env,
		}

		if err := checkDB(r.Context()); err != nil {
			status = http.StatusServiceUnavailable
			payload["status"] = "degraded"
			payload["database"] = err.Error()
		}

		writeJSON(w, status, payload)
	})

	return mux
}

func NewPostgresReachabilityChecker(databaseURL string, timeout time.Duration) DBChecker {
	return func(ctx context.Context) error {
		parsed, err := url.Parse(databaseURL)
		if err != nil {
			return fmt.Errorf("invalid database url: %w", err)
		}

		host := parsed.Hostname()
		if host == "" {
			return errors.New("database host is missing")
		}

		port := parsed.Port()
		if port == "" {
			port = "5432"
		}

		dialer := net.Dialer{Timeout: timeout}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
		if err != nil {
			return fmt.Errorf("postgres unreachable: %w", err)
		}

		_ = conn.Close()
		return nil
	}
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
