// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package main is the entrypoint for the Chef Migration Metrics application.
// It loads configuration, connects to the database, runs migrations, syncs
// organisations from config, and starts the HTTP server with graceful
// shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/collector"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/embedded"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/webapi"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	// -------------------------------------------------------------------
	// CLI flags
	// -------------------------------------------------------------------
	var (
		configPath     string
		migrationsDir  string
		showVersion    bool
		healthcheck    bool
		healthcheckURL string
	)

	flag.StringVar(&configPath, "config", "", "Path to configuration file (or set CHEF_MIGRATION_METRICS_CONFIG)")
	flag.StringVar(&migrationsDir, "migrations-dir", "", "Path to SQL migrations directory (default: ./migrations or /usr/share/chef-migration-metrics/migrations)")
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")
	flag.BoolVar(&healthcheck, "healthcheck", false, "Run health check against a running instance and exit")
	flag.StringVar(&healthcheckURL, "healthcheck-url", "", "URL for health check (default: http://localhost:<port>/api/v1/health)")
	flag.Parse()

	if showVersion {
		fmt.Println("chef-migration-metrics", version)
		return 0
	}

	if healthcheck {
		return runHealthcheck(healthcheckURL)
	}

	// -------------------------------------------------------------------
	// Bootstrap logger — stdout only until the database is available.
	// We start with INFO; this will be overridden once config is loaded.
	// -------------------------------------------------------------------
	stdoutWriter := logging.NewStdoutWriter()
	logger := logging.New(logging.Options{
		Level:   logging.INFO,
		Writers: []logging.Writer{stdoutWriter},
	})
	startup := logger.WithScope(logging.ScopeStartup)

	// -------------------------------------------------------------------
	// Configuration
	// -------------------------------------------------------------------
	cfg, warnings, err := config.Load(configPath)
	if err != nil {
		startup.Error(fmt.Sprintf("loading configuration: %v", err))
		return 1
	}
	if warnings != nil {
		for _, w := range warnings.Messages {
			startup.Warn(fmt.Sprintf("config: %s", w))
		}
	}

	// Re-create the logger with the configured log level now that config
	// is available. The DBWriter will be added once the DB is connected.
	configuredLevel := logging.INFO
	if cfg.Logging.Level != "" {
		parsed, parseErr := logging.ParseSeverity(cfg.Logging.Level)
		if parseErr != nil {
			startup.Warn(fmt.Sprintf("config: %v", parseErr))
		}
		configuredLevel = parsed
	}
	logger = logging.New(logging.Options{
		Level:   configuredLevel,
		Writers: []logging.Writer{stdoutWriter},
	})
	startup = logger.WithScope(logging.ScopeStartup)
	startup.Info("configuration loaded successfully")

	// -------------------------------------------------------------------
	// Database connection
	// -------------------------------------------------------------------
	dbURL := cfg.Datastore.URL
	if dbURL == "" {
		dbURL = os.Getenv("CMM_DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		startup.Error("no database URL configured (set datastore.url in config, CMM_DATABASE_URL, or DATABASE_URL)")
		return 1
	}

	db, err := datastore.Open(dbURL)
	if err != nil {
		startup.Error(fmt.Sprintf("connecting to database: %v", err))
		return 1
	}
	defer db.Close()
	startup.Info("database connection established")

	// -------------------------------------------------------------------
	// Attach DBWriter — from this point, log entries are also persisted
	// to the log_entries table for the web UI log viewer.
	// -------------------------------------------------------------------
	dbAdapter := logging.NewDatastoreAdapter(
		func(ctx context.Context, p logging.LogEntryParams) (string, error) {
			entry, dsErr := db.InsertLogEntry(ctx, datastore.InsertLogEntryParams{
				Timestamp:           p.Timestamp,
				Severity:            p.Severity,
				Scope:               p.Scope,
				Message:             p.Message,
				Organisation:        p.Organisation,
				CookbookName:        p.CookbookName,
				CookbookVersion:     p.CookbookVersion,
				CommitSHA:           p.CommitSHA,
				ChefClientVersion:   p.ChefClientVersion,
				ProcessOutput:       p.ProcessOutput,
				CollectionRunID:     p.CollectionRunID,
				NotificationChannel: p.NotificationChannel,
				ExportJobID:         p.ExportJobID,
				TLSDomain:           p.TLSDomain,
			})
			if dsErr != nil {
				return "", dsErr
			}
			return entry.ID, nil
		},
	)
	dbWriter := logging.NewDBWriter(dbAdapter,
		logging.WithContext(context.Background()),
		logging.WithOnError(func(entry logging.Entry, dbErr error) {
			// Fall back to stderr so DB-write failures are not silent.
			// We cannot use the logger here (infinite loop), so use
			// the stdlib log package for this one edge case.
			log.Printf("WARN: failed to persist log entry to database: %v", dbErr)
		}),
	)

	logger = logging.New(logging.Options{
		Level:   configuredLevel,
		Writers: []logging.Writer{stdoutWriter, dbWriter},
	})
	startup = logger.WithScope(logging.ScopeStartup)
	startup.Debug("database log writer attached")

	// -------------------------------------------------------------------
	// Migrations
	// -------------------------------------------------------------------
	migDir := resolveMigrationsDir(migrationsDir)
	if migDir == "" {
		startup.Error("migrations directory not found — pass -migrations-dir or place migrations in ./migrations")
		return 1
	}

	ctx := context.Background()
	applied, err := db.MigrateUp(ctx, migDir)
	if err != nil {
		startup.Error(fmt.Sprintf("running database migrations: %v", err))
		return 1
	}
	if applied > 0 {
		startup.Info(fmt.Sprintf("applied %d database migration(s)", applied))
	} else {
		startup.Info("database schema is up to date")
	}

	ver, err := db.MigrationVersion(ctx)
	if err != nil {
		startup.Warn(fmt.Sprintf("could not read migration version: %v", err))
	} else {
		startup.Info(fmt.Sprintf("database schema version: %d", ver))
	}

	// -------------------------------------------------------------------
	// Mark interrupted collection runs from previous process
	// -------------------------------------------------------------------
	staleRuns, err := db.GetRunningCollectionRuns(ctx)
	if err != nil {
		startup.Warn(fmt.Sprintf("could not check for interrupted collection runs: %v", err))
	} else if len(staleRuns) > 0 {
		for _, r := range staleRuns {
			if _, err := db.InterruptCollectionRun(ctx, r.ID); err != nil {
				startup.Warn(fmt.Sprintf("could not mark collection run %s as interrupted: %v", r.ID, err))
			} else {
				startup.Info(fmt.Sprintf("marked stale collection run %s (org %s) as interrupted", r.ID, r.OrganisationID))
			}
		}
	}

	// -------------------------------------------------------------------
	// Secrets: master key validation and credential store setup
	// -------------------------------------------------------------------
	secretsLog := logger.WithScope(logging.ScopeSecrets)

	// Determine the env var name for the master encryption key. The config
	// field allows operators to override the default variable name.
	masterKeyEnvName := cfg.CredentialEncryptionKeyEnv
	if masterKeyEnvName == "" {
		masterKeyEnvName = "CMM_CREDENTIAL_ENCRYPTION_KEY"
	}

	// Build the credential store and (optionally) the encryptor. The
	// encryptor is nil when no master key is configured — this is fine as
	// long as no credentials are stored in the database.
	var encryptor *secrets.Encryptor
	var credStore *secrets.DBCredentialStore

	masterKeyBase64 := os.Getenv(masterKeyEnvName)
	if masterKeyBase64 != "" {
		var mkErr error
		encryptor, mkErr = secrets.NewEncryptor(masterKeyBase64)
		if mkErr != nil {
			secretsLog.Error(fmt.Sprintf("master encryption key from %s is invalid: %v", masterKeyEnvName, mkErr))
			return 1
		}
		defer encryptor.Close()
		secretsLog.Info(fmt.Sprintf("master encryption key loaded from %s", masterKeyEnvName))
	}

	credStore = secrets.NewDBCredentialStore(db.Pool(), encryptor)

	// Check whether stored credentials exist. If they do but no master key
	// is configured, we cannot decrypt them — warn loudly.
	credCount, credCountErr := credStore.CredentialCount(ctx)
	if credCountErr != nil {
		secretsLog.Warn(fmt.Sprintf("could not count stored credentials: %v", credCountErr))
	} else if credCount > 0 && encryptor == nil {
		secretsLog.Error(fmt.Sprintf(
			"%d credential(s) are stored in the database but no master encryption key is configured (set %s)",
			credCount, masterKeyEnvName,
		))
		return 1
	} else if credCount > 0 {
		secretsLog.Info(fmt.Sprintf("%d stored credential(s) found; master key is configured", credCount))
	} else {
		secretsLog.Debug("no stored credentials — master key validation skipped")
	}

	// -------------------------------------------------------------------
	// Secrets: master key rotation (if previous key is provided)
	// -------------------------------------------------------------------
	if secrets.NeedsRotation(os.LookupEnv) {
		if encryptor == nil {
			secretsLog.Error("CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS is set but no current master key is configured — cannot rotate")
			return 1
		}

		prevKeyBase64, _ := os.LookupEnv("CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS")
		prevEncryptor, prevErr := secrets.NewEncryptor(prevKeyBase64)
		if prevErr != nil {
			secretsLog.Error(fmt.Sprintf("previous master encryption key is invalid: %v", prevErr))
			return 1
		}
		defer prevEncryptor.Close()

		secretsLog.Info("master key rotation requested — re-encrypting stored credentials")

		rotationRows, rrErr := credStore.ListRotationRows(ctx)
		if rrErr != nil {
			secretsLog.Error(fmt.Sprintf("failed to read credentials for rotation: %v", rrErr))
			return 1
		}

		rotationWriter := func(wCtx context.Context, row secrets.RotatedRow) error {
			return credStore.UpdateEncryptedValueRaw(wCtx, row.Name, row.NewEncryptedValue)
		}

		result, rotErr := secrets.RotateMasterKey(ctx, rotationRows, encryptor, prevEncryptor, rotationWriter)
		if rotErr != nil {
			secretsLog.Error(fmt.Sprintf("master key rotation failed: %v", rotErr))
			return 1
		}

		secretsLog.Info(fmt.Sprintf(
			"master key rotation complete in %s: %d total, %d re-encrypted, %d already rotated, %d failed",
			result.Duration.Round(time.Millisecond), result.TotalCredentials,
			result.ReEncrypted, result.AlreadyRotated, result.Failed,
		))

		for name, rotItemErr := range result.Errors {
			secretsLog.Error(fmt.Sprintf("credential %q could not be rotated: %v", name, rotItemErr))
		}

		if result.Failed > 0 {
			secretsLog.Warn(fmt.Sprintf(
				"%d credential(s) failed rotation — they may be undecryptable. "+
					"Remove CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS to skip rotation on next startup.",
				result.Failed,
			))
		}
	}

	// -------------------------------------------------------------------
	// Secrets: validate all stored credentials can be decrypted
	// -------------------------------------------------------------------
	if credCount > 0 && encryptor != nil {
		rotationRows, rrErr := credStore.ListRotationRows(ctx)
		if rrErr != nil {
			secretsLog.Warn(fmt.Sprintf("could not validate stored credentials: %v", rrErr))
		} else {
			decryptFailures := 0
			for _, row := range rotationRows {
				aad, aadErr := secrets.BuildAAD(row.CredentialType, row.Name)
				if aadErr != nil {
					secretsLog.Error(fmt.Sprintf("credential %q: failed to build AAD: %v", row.Name, aadErr))
					decryptFailures++
					continue
				}
				plaintext, decErr := encryptor.Decrypt(row.EncryptedValue, aad)
				if decErr != nil {
					secretsLog.Error(fmt.Sprintf("credential %q: decryption failed (wrong key or corrupted data)", row.Name))
					decryptFailures++
					continue
				}
				secrets.ZeroBytes(plaintext)
			}
			if decryptFailures > 0 {
				secretsLog.Warn(fmt.Sprintf("%d of %d credential(s) failed decryption validation", decryptFailures, len(rotationRows)))
			} else if len(rotationRows) > 0 {
				secretsLog.Info(fmt.Sprintf("all %d credential(s) passed decryption validation", len(rotationRows)))
			}
		}
	}

	// -------------------------------------------------------------------
	// Secrets: warn on overly permissive key file permissions
	// -------------------------------------------------------------------
	for _, org := range cfg.Organisations {
		if org.ClientKeyPath == "" {
			continue
		}
		info, statErr := os.Stat(org.ClientKeyPath)
		if statErr != nil {
			// File may not exist yet or may be resolved at collection
			// time — don't treat as fatal, just skip the permission check.
			continue
		}
		perm := info.Mode().Perm()
		if perm&0o077 != 0 {
			secretsLog.Warn(fmt.Sprintf(
				"key file %s for organisation %q has permissions %04o — should be 0600 or more restrictive",
				org.ClientKeyPath, org.Name, perm,
			))
		}
	}

	// Build the credential resolver for use by downstream components
	// (collector, notifications, etc.).
	credResolver := secrets.NewCredentialResolver(credStore)

	// -------------------------------------------------------------------
	// Sync organisations from configuration
	// -------------------------------------------------------------------
	orgParams := make([]datastore.UpsertOrganisationParams, 0, len(cfg.Organisations))
	for _, org := range cfg.Organisations {
		orgParams = append(orgParams, datastore.UpsertOrganisationParams{
			Name:          org.Name,
			ChefServerURL: org.ChefServerURL,
			OrgName:       org.OrgName,
			ClientName:    org.ClientName,
			// ClientKeyCredentialID is resolved later by the secrets package
		})
	}

	orgs, err := db.SyncOrganisationsFromConfig(ctx, orgParams)
	if err != nil {
		startup.Error(fmt.Sprintf("syncing organisations from config: %v", err))
		return 1
	}
	startup.Info(fmt.Sprintf("%d organisation(s) synced from configuration", len(orgs)))
	for _, org := range orgs {
		startup.Info(fmt.Sprintf("  - %s (%s/%s)", org.Name, org.ChefServerURL, org.OrgName))
	}

	// -------------------------------------------------------------------
	// Analysis pipeline: resolve external tools
	// -------------------------------------------------------------------
	toolResolver := embedded.NewResolver(cfg.AnalysisTools.EmbeddedBinDir)
	toolResult := toolResolver.ValidateAll(ctx)

	// Log tool availability — these are informational; only git is mandatory.
	if toolResult.Git.Available {
		startup.Info(fmt.Sprintf("git available: %s (version %s)", toolResult.Git.Path, toolResult.Git.Version))
	} else {
		startup.Warn(fmt.Sprintf("git not available: %s — git cookbook fetching will fail", toolResult.Git.Error))
	}
	if toolResult.Cookstyle.Available {
		startup.Info(fmt.Sprintf("cookstyle available: %s (version %s)", toolResult.Cookstyle.Path, toolResult.Cookstyle.Version))
	} else {
		startup.Info(fmt.Sprintf("cookstyle not available: %s — CookStyle scanning disabled", toolResult.Cookstyle.Error))
	}
	if toolResult.Kitchen.Available {
		startup.Info(fmt.Sprintf("kitchen available: %s (version %s)", toolResult.Kitchen.Path, toolResult.Kitchen.Version))
	} else {
		startup.Info(fmt.Sprintf("kitchen not available: %s — Test Kitchen testing disabled", toolResult.Kitchen.Error))
	}
	if toolResult.Docker.Available {
		startup.Info(fmt.Sprintf("docker available: %s (version %s)", toolResult.Docker.Path, toolResult.Docker.Version))
	} else {
		startup.Info(fmt.Sprintf("docker not available: %s — Test Kitchen testing disabled", toolResult.Docker.Error))
	}

	if !toolResult.CookstyleEnabled && !toolResult.KitchenEnabled {
		startup.Warn("neither CookStyle nor Test Kitchen available — no cookbook compatibility testing will be performed")
	}

	// Construct available analysis pipeline components.
	var collOpts []collector.Option

	// CookStyle scanner + autocorrect preview generator (both require cookstyle).
	if toolResult.CookstyleEnabled {
		csScanner := analysis.NewCookstyleScanner(
			db, logger, toolResult.Cookstyle.Path,
			cfg.Concurrency.CookstyleScan,
			cfg.AnalysisTools.CookstyleTimeoutMinutes,
		)
		collOpts = append(collOpts, collector.WithCookstyleScanner(csScanner))
		startup.Info("CookStyle scanner enabled")

		acGen := remediation.NewAutocorrectGenerator(
			db, logger, toolResult.Cookstyle.Path,
			cfg.AnalysisTools.CookstyleTimeoutMinutes,
		)
		collOpts = append(collOpts, collector.WithAutocorrectGenerator(acGen))
		startup.Info("autocorrect preview generator enabled")
	}

	// Test Kitchen scanner (requires both kitchen and docker).
	if toolResult.KitchenEnabled {
		tkScanner := analysis.NewKitchenScanner(
			db, logger, toolResult.Kitchen.Path,
			cfg.Concurrency.TestKitchenRun,
			cfg.AnalysisTools.TestKitchenTimeoutMinutes,
			cfg.AnalysisTools.TestKitchen,
		)
		collOpts = append(collOpts, collector.WithKitchenScanner(tkScanner))
		startup.Info("Test Kitchen scanner enabled")
	}

	// Complexity scorer — always available (reads from DB, no external tool).
	cxScorer := remediation.NewComplexityScorer(db, logger)
	collOpts = append(collOpts, collector.WithComplexityScorer(cxScorer))

	// Readiness evaluator — always available (reads from DB, no external tool).
	readinessEval := analysis.NewReadinessEvaluator(
		db, logger,
		cfg.Concurrency.ReadinessEvaluation,
		cfg.Readiness.MinFreeDiskMB,
	)
	collOpts = append(collOpts, collector.WithReadinessEvaluator(readinessEval))

	// Cookbook directory resolver. Chef server cookbooks are downloaded
	// to a temp directory keyed by org/name/version. Git cookbooks are
	// cloned under the git base directory. This function is used by
	// CookStyle scanning, Test Kitchen, and autocorrect preview generation.
	gitBaseDir := filepath.Join(os.TempDir(), "chef-migration-metrics", "git-cookbooks")
	cookbookCacheDir := filepath.Join(os.TempDir(), "chef-migration-metrics", "cookbook-cache")
	collOpts = append(collOpts, collector.WithCookbookCacheDir(cookbookCacheDir))
	collOpts = append(collOpts, collector.WithCookbookDirFn(func(cb datastore.Cookbook) string {
		if cb.IsGit() {
			return filepath.Join(gitBaseDir, cb.Name)
		}
		if cb.IsChefServer() && cb.IsDownloaded() {
			return filepath.Join(cookbookCacheDir, cb.OrganisationID, cb.Name, cb.Version)
		}
		return ""
	}))

	startup.Info("analysis pipeline configured: complexity scorer and readiness evaluator always enabled")

	// -------------------------------------------------------------------
	// Data collection scheduler
	// -------------------------------------------------------------------
	coll := collector.New(db, cfg, logger, credResolver, collOpts...)

	schedule, schedErr := collector.ParseSchedule(cfg.Collection.Schedule)
	if schedErr != nil {
		startup.Error(fmt.Sprintf("invalid collection schedule %q: %v", cfg.Collection.Schedule, schedErr))
		return 1
	}

	sched := collector.NewScheduler(coll, schedule, logger)
	if err := sched.Start(); err != nil {
		startup.Error(fmt.Sprintf("starting collection scheduler: %v", err))
		return 1
	}
	defer sched.Stop()
	startup.Info(fmt.Sprintf("collection scheduler started (schedule: %s)", cfg.Collection.Schedule))

	// -------------------------------------------------------------------
	// HTTP server — wire up the webapi.Router which owns all API routes,
	// WebSocket endpoint, health/version endpoints, and SPA fallback.
	// -------------------------------------------------------------------
	hub := webapi.NewEventHub()
	go hub.Run()

	apiRouter := webapi.NewRouter(db, cfg, hub,
		webapi.WithVersion(version),
		webapi.WithLogger(func(level, msg string) {
			switch level {
			case "DEBUG":
				logger.WithScope(logging.ScopeWebAPI).Debug(msg)
			case "WARN":
				logger.WithScope(logging.ScopeWebAPI).Warn(msg)
			case "ERROR":
				logger.WithScope(logging.ScopeWebAPI).Error(msg)
			default:
				logger.WithScope(logging.ScopeWebAPI).Info(msg)
			}
		}),
	)
	startup.Info("webapi router initialised with all API routes")

	listenAddr := cfg.Server.ListenAddress
	if listenAddr == "" {
		listenAddr = "0.0.0.0"
	}
	port := cfg.Server.Port
	if port == 0 {
		port = 8080
	}
	addr := fmt.Sprintf("%s:%d", listenAddr, port)

	shutdownTimeout := time.Duration(cfg.Server.GracefulShutdownSeconds) * time.Second
	if shutdownTimeout <= 0 {
		shutdownTimeout = 15 * time.Second
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      apiRouter,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	// -------------------------------------------------------------------
	// Start server + signal handling
	// -------------------------------------------------------------------
	errCh := make(chan error, 1)
	go func() {
		startup.Info(fmt.Sprintf("HTTP server listening on %s", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for interrupt signal or server error.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		startup.Info(fmt.Sprintf("received signal %s, shutting down gracefully...", sig))
	case err := <-errCh:
		if err != nil {
			startup.Error(fmt.Sprintf("HTTP server failed: %v", err))
			return 1
		}
	}

	// Graceful shutdown — stop the scheduler first so no new collection
	// runs start, then shut down the HTTP server.
	startup.Info("stopping collection scheduler...")
	sched.Stop()
	startup.Info("collection scheduler stopped")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		startup.Error(fmt.Sprintf("HTTP server shutdown: %v", err))
		return 1
	}

	startup.Info("server stopped cleanly")
	return 0
}

// resolveMigrationsDir finds the migrations directory. It checks, in order:
//  1. The explicit path passed via -migrations-dir flag
//  2. ./migrations (relative to working directory)
//  3. The directory containing the executable + /migrations
//  4. /usr/share/chef-migration-metrics/migrations (installed package path)
func resolveMigrationsDir(explicit string) string {
	candidates := []string{explicit}

	// Relative to working directory.
	candidates = append(candidates, "migrations")

	// Relative to the executable.
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "migrations"))
	}

	// Installed package location.
	candidates = append(candidates, "/usr/share/chef-migration-metrics/migrations")

	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}

// runHealthcheck performs an HTTP GET against the health endpoint and exits
// with 0 on success or 1 on failure. Used by container HEALTHCHECK.
func runHealthcheck(url string) int {
	if url == "" {
		url = "http://localhost:8080/api/v1/health"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck failed: HTTP %d\n", resp.StatusCode)
		return 1
	}

	fmt.Println("healthy")
	return 0
}
