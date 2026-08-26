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

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/appauth"
	"github.com/trestle-dev/trestle/internal/buildinfo"
	"github.com/trestle-dev/trestle/internal/collections"
	"github.com/trestle-dev/trestle/internal/config"
	"github.com/trestle-dev/trestle/internal/events"
	filestore "github.com/trestle-dev/trestle/internal/files"
	"github.com/trestle-dev/trestle/internal/identities"
	"github.com/trestle-dev/trestle/internal/records"
	"github.com/trestle-dev/trestle/internal/rules"
	"github.com/trestle-dev/trestle/internal/server"
	"github.com/trestle-dev/trestle/internal/store"
	"github.com/trestle-dev/trestle/internal/web"
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
	database, err := store.Open(context.Background(), cfg.DataDir)
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	logger.Info("database ready", "path", database.Path(), "schema_version", store.CurrentVersion)
	dashboard, err := web.New(cfg.StaticDir)
	if err != nil {
		logger.Error("dashboard initialization failed", "error", err)
		os.Exit(1)
	}
	if cfg.StaticDir != "" {
		logger.Warn("using development static override", "directory", cfg.StaticDir)
	}
	admin := adminauth.New(database.DB())
	collectionAdmin := collections.New(database.DB(), admin)
	credentials := identities.New(database.DB(), admin)
	recordAPI := records.New(database.DB(), admin, credentials)
	applicationAuth := appauth.New(database.DB(), admin)
	accessRules := rules.New(database.DB(), admin)
	recordAPI.ConfigureAccess(applicationAuth, accessRules)
	eventAPI := events.New(database.DB(), admin, credentials)
	recordAPI.ConfigureEvents(eventAPI)
	fileAPI, err := filestore.New(database.DB(), admin, credentials, cfg.DataDir, filestore.Options{Backend: cfg.StorageBackend, S3Endpoint: cfg.S3Endpoint, S3Region: cfg.S3Region, S3Bucket: cfg.S3Bucket, S3AccessKey: cfg.S3AccessKey, S3SecretKey: cfg.S3SecretKey})
	if err != nil {
		logger.Error("file storage initialization failed", "error", err)
		os.Exit(1)
	}
	apiRoutes := http.NewServeMux()
	apiRoutes.Handle("/api/v1/auth/", applicationAuth)
	apiRoutes.Handle("/api/v1/collections/", recordAPI)
	apiRoutes.Handle("/api/v1/files", fileAPI)
	apiRoutes.Handle("/api/v1/files/", fileAPI)
	apiRoutes.Handle("/api/v1/realtime", eventAPI)
	adminRoutes := http.NewServeMux()
	adminRoutes.Handle("/admin/v1/collections", collectionAdmin)
	adminRoutes.Handle("/admin/v1/collections/", collectionAdmin)
	adminRoutes.Handle("/admin/v1/data/", recordAPI)
	adminRoutes.Handle("/admin/v1/app-users", applicationAuth)
	adminRoutes.Handle("/admin/v1/app-users/", applicationAuth)
	adminRoutes.Handle("/admin/v1/credentials", credentials)
	adminRoutes.Handle("/admin/v1/credentials/", credentials)
	adminRoutes.Handle("/admin/v1/collection-rules/", accessRules)
	adminRoutes.Handle("/admin/v1/files", fileAPI)
	adminRoutes.Handle("/admin/v1/files/", fileAPI)
	adminRoutes.Handle("/admin/v1/storage/status", fileAPI)
	adminRoutes.Handle("/admin/v1/events", eventAPI)
	adminRoutes.Handle("/", admin)
	app := server.NewWithHandlers(logger, dashboard, apiRoutes, adminRoutes)
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
