package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/trestle-dev/trestle/internal/buildinfo"
	"github.com/trestle-dev/trestle/internal/config"
	"github.com/trestle-dev/trestle/internal/server"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
		return
	}

	cfg, err := config.FromOS(os.Args[1:])
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(2)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{}))
	app := server.New(logger)
	httpServer := &http.Server{Addr: cfg.Listen, Handler: app.Handler(), ReadHeaderTimeout: 5_000_000_000, IdleTimeout: 60_000_000_000}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "listen", cfg.Listen, "data_dir", cfg.DataDir)
		app.SetReady(true)
		errCh <- httpServer.ListenAndServe()
	}()
	select {
	case err = <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
		return
	case <-ctx.Done():
	}
	app.SetReady(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}
