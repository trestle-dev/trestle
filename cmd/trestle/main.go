package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/trestle-dev/trestle/internal/adminauth"
	"github.com/trestle-dev/trestle/internal/apidocs"
	"github.com/trestle-dev/trestle/internal/appauth"
	"github.com/trestle-dev/trestle/internal/audit"
	"github.com/trestle-dev/trestle/internal/backup"
	"github.com/trestle-dev/trestle/internal/buildinfo"
	"github.com/trestle-dev/trestle/internal/collections"
	"github.com/trestle-dev/trestle/internal/config"
	"github.com/trestle-dev/trestle/internal/databasesetup"
	"github.com/trestle-dev/trestle/internal/deployment"
	"github.com/trestle-dev/trestle/internal/events"
	filestore "github.com/trestle-dev/trestle/internal/files"
	functionapi "github.com/trestle-dev/trestle/internal/functions"
	"github.com/trestle-dev/trestle/internal/identities"
	"github.com/trestle-dev/trestle/internal/jobs"
	"github.com/trestle-dev/trestle/internal/records"
	"github.com/trestle-dev/trestle/internal/rules"
	"github.com/trestle-dev/trestle/internal/server"
	"github.com/trestle-dev/trestle/internal/store"
	"github.com/trestle-dev/trestle/internal/web"
	"github.com/trestle-dev/trestle/internal/webhooks"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "restore" {
		set := flag.NewFlagSet("restore", flag.ContinueOnError)
		archive := set.String("backup", "", "backup archive path")
		dataDir := set.String("data-dir", "", "new restore data directory (SQLite)")
		provider := set.String("provider", "sqlite", "restore destination provider: sqlite or postgres")
		databaseURL := set.String("database-url", "", "PostgreSQL destination URL")
		if err := set.Parse(os.Args[2:]); err != nil || *archive == "" {
			fmt.Fprintln(os.Stderr, "usage: trestle restore --backup ARCHIVE [--data-dir DIR] [--provider sqlite|postgres] [--database-url URL]")
			os.Exit(2)
		}
		restoreProvider, err := store.ParseProvider(*provider)
		if err != nil || (restoreProvider == store.SQLite && *dataDir == "") || (restoreProvider == store.Postgres && *databaseURL == "") {
			fmt.Fprintln(os.Stderr, "usage: trestle restore --backup ARCHIVE [--data-dir DIR] [--provider sqlite|postgres] [--database-url URL]")
			os.Exit(2)
		}
		if err := backup.Restore(context.Background(), *archive, *dataDir, backup.RestoreOptions{Provider: restoreProvider, URL: *databaseURL}); err != nil {
			fmt.Fprintln(os.Stderr, "restore failed:", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "restore complete:", *dataDir)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		set := flag.NewFlagSet("migrate", flag.ContinueOnError)
		fromProvider := set.String("from-provider", "", "source provider: sqlite or postgres")
		fromDir := set.String("from-dir", "", "source data directory (sqlite)")
		fromURL := set.String("from-url", "", "source PostgreSQL URL")
		toProvider := set.String("to-provider", "", "target provider: sqlite or postgres")
		toDir := set.String("to-dir", "", "target data directory (sqlite)")
		toURL := set.String("to-url", "", "target PostgreSQL URL")
		dryRun := set.Bool("dry-run", false, "export and validate without writing to the target")
		confirm := set.Bool("confirm-migration", false, "explicit confirmation for a real (non-dry-run) migration")
		if err := set.Parse(os.Args[2:]); err != nil {
			printMigrateUsage()
			os.Exit(2)
		}
		sourceProvider, err := store.ParseProvider(*fromProvider)
		if err != nil || *toProvider == "" {
			printMigrateUsage()
			os.Exit(2)
		}
		targetProvider, err := store.ParseProvider(*toProvider)
		if err != nil {
			printMigrateUsage()
			os.Exit(2)
		}
		report, err := backup.Migrate(context.Background(), backup.MigrateOptions{
			SourceProvider: sourceProvider, SourceDir: *fromDir, SourceURL: *fromURL,
			TargetProvider: targetProvider, TargetDir: *toDir, TargetURL: *toURL,
			DryRun: *dryRun, Confirm: *confirm,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "migration failed:", err)
			os.Exit(1)
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			os.Exit(1)
		}
		return
	}

	cfg, err := config.FromOS(os.Args[1:])
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(2)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{}))
	databaseContext, cancelDatabase := context.WithTimeout(context.Background(), cfg.DatabaseConnectTimeout)
	database, err := store.OpenWith(databaseContext, store.Options{DataDir: cfg.DataDir, Provider: store.Provider(cfg.DatabaseProvider), URL: cfg.DatabaseURL, MaxOpen: cfg.DatabaseMaxOpen, MaxIdle: cfg.DatabaseMaxIdle, ConnMaxLifetime: cfg.DatabaseConnMaxLifetime, ConnectTimeout: cfg.DatabaseConnectTimeout})
	cancelDatabase()
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	logger.Info("database ready", "provider", database.Provider(), "schema_version", store.CurrentVersion)
	dashboard, err := web.New(cfg.StaticDir)
	if err != nil {
		logger.Error("dashboard initialization failed", "error", err)
		os.Exit(1)
	}
	if cfg.StaticDir != "" {
		logger.Warn("using development static override", "directory", cfg.StaticDir)
	}
	admin := adminauth.New(database.DB(), string(database.Provider()))
	collectionAdmin := collections.New(database.DB(), admin)
	credentials := identities.New(database.DB(), admin)
	recordAPI := records.New(database.DB(), admin, credentials)
	applicationAuth := appauth.New(database.DB(), admin)
	accessRules := rules.New(database.DB(), admin)
	recordAPI.ConfigureAccess(applicationAuth, accessRules)
	eventAPI := events.New(database.DB(), admin, credentials)
	recordAPI.ConfigureEvents(eventAPI)
	auditAPI := audit.New(database.DB(), admin, string(database.Provider()))
	jobAPI := jobs.New(database.DB(), admin)
	webhookAPI, err := webhooks.New(database.DB(), admin, jobAPI, cfg.DataDir)
	if err != nil {
		logger.Error("webhook initialization failed", "error", err)
		os.Exit(1)
	}
	eventAPI.ConfigureDispatcher(webhookAPI)
	functionAPI := functionapi.New(database.DB(), admin, jobAPI, functionapi.Options{Region: cfg.AWSRegion, AccessKey: cfg.AWSAccessKey, SecretKey: cfg.AWSSecretKey})
	apiDocs := apidocs.New(database.DB(), admin)
	backupAPI, err := backup.New(database.DB(), admin, cfg.DataDir, cfg.StorageBackend)
	if err != nil {
		logger.Error("backup initialization failed", "error", err)
		os.Exit(1)
	}
	deploymentAPI := deployment.New(admin, deployment.Options{Listen: cfg.Listen, StorageBackend: cfg.StorageBackend, TrustedProxies: cfg.TrustedProxies, ReadHeaderTimeout: cfg.ReadHeaderTimeout, ReadTimeout: cfg.ReadTimeout, IdleTimeout: cfg.IdleTimeout, MaxHeaderBytes: cfg.MaxHeaderBytes})
	eventAPI.ConfigureDispatcher(functionAPI)
	recordAPI.ConfigureAudit(auditAPI)
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
	apiRoutes.Handle("/api/v1/openapi.json", apiDocs)
	apiRoutes.Handle("/api/v1/capabilities", apiDocs)
	adminRoutes := http.NewServeMux()
	databaseSetup := databasesetup.New(admin, databasesetup.Options{DataDir: cfg.DataDir, Current: database.Provider(), Explicit: cfg.DatabaseExplicit, MaxOpen: cfg.DatabaseMaxOpen, MaxIdle: cfg.DatabaseMaxIdle, ConnectTimeout: cfg.DatabaseConnectTimeout, ConnMaxLifetime: cfg.DatabaseConnMaxLifetime})
	adminRoutes.Handle("/admin/v1/database/setup", databaseSetup)
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
	adminRoutes.Handle("/admin/v1/audit", auditAPI)
	adminRoutes.Handle("/admin/v1/operations", auditAPI)
	adminRoutes.Handle("/admin/v1/jobs", jobAPI)
	adminRoutes.Handle("/admin/v1/jobs/", jobAPI)
	adminRoutes.Handle("/admin/v1/webhooks", webhookAPI)
	adminRoutes.Handle("/admin/v1/webhooks/", webhookAPI)
	adminRoutes.Handle("/admin/v1/functions", functionAPI)
	adminRoutes.Handle("/admin/v1/functions/", functionAPI)
	adminRoutes.Handle("/admin/v1/api/schema", apiDocs)
	adminRoutes.Handle("/admin/v1/backups", backupAPI)
	adminRoutes.Handle("/admin/v1/backups/", backupAPI)
	adminRoutes.Handle("/admin/v1/restores/preflight", backupAPI)
	adminRoutes.Handle("/admin/v1/export", backupAPI)
	adminRoutes.Handle("/admin/v1/imports/dry-run", backupAPI)
	adminRoutes.Handle("/admin/v1/deployment", deploymentAPI)
	adminRoutes.Handle("/admin/v1/support-bundle", deploymentAPI)
	adminRoutes.Handle("/", admin)
	app := server.NewWithOptions(logger, dashboard, apiRoutes, adminRoutes, server.Options{TrustedProxies: cfg.TrustedProxies})
	httpServer := &http.Server{Addr: cfg.Listen, Handler: app.Handler(), ReadHeaderTimeout: cfg.ReadHeaderTimeout, ReadTimeout: cfg.ReadTimeout, IdleTimeout: cfg.IdleTimeout, MaxHeaderBytes: cfg.MaxHeaderBytes}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	jobAPI.Start(ctx)
	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "listen", cfg.Listen, "data_dir", cfg.DataDir, "trusted_proxy_count", len(cfg.TrustedProxies), "read_header_timeout", cfg.ReadHeaderTimeout, "read_timeout", cfg.ReadTimeout, "idle_timeout", cfg.IdleTimeout, "max_header_bytes", cfg.MaxHeaderBytes)
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

func printMigrateUsage() {
	fmt.Fprintln(os.Stderr, "usage: trestle migrate --from-provider sqlite|postgres --from-dir DIR|--from-url URL --to-provider sqlite|postgres --to-dir DIR|--to-url URL [--dry-run] [--confirm-migration]")
}
