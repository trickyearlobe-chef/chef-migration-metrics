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
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/collector"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/embedded"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/frontend"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
	apptls "github.com/trickyearlobe-chef/chef-migration-metrics/internal/tls"
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
	// Authentication: local authenticator, session manager, middleware
	// -------------------------------------------------------------------
	authLog := logger.WithScope(logging.ScopeAuth)
	authLogFn := func(level, msg string) {
		switch level {
		case "DEBUG":
			authLog.Debug(msg)
		case "WARN":
			authLog.Warn(msg)
		case "ERROR":
			authLog.Error(msg)
		default:
			authLog.Info(msg)
		}
	}

	sessionLifetime := auth.ParseDuration(cfg.Auth.SessionExpiry, 8*time.Hour)

	sessionMgr := auth.NewSessionManager(db, sessionLifetime,
		auth.WithSessionLogger(authLogFn),
	)

	localAuth := auth.NewLocalAuthenticator(db, cfg.Auth.LockoutAttempts,
		auth.WithLocalAuthLogger(authLogFn),
	)

	authMiddleware := auth.NewMiddleware(sessionMgr,
		auth.WithMiddlewareLogger(authLogFn),
	)

	startup.Info(fmt.Sprintf("authentication configured: session_expiry=%s, lockout_attempts=%d, min_password_length=%d",
		sessionLifetime, cfg.Auth.LockoutAttempts, cfg.Auth.MinPasswordLength))

	// Seed default admin user if no users exist yet. The default password
	// is "ChefMigrate1" — operators MUST change it on first login.
	defaultAdminPassword := os.Getenv("CMM_DEFAULT_ADMIN_PASSWORD")
	if defaultAdminPassword == "" {
		defaultAdminPassword = "ChefMigrate1"
	}
	defaultAdminHash, err := auth.HashPassword(defaultAdminPassword)
	if err != nil {
		startup.Error(fmt.Sprintf("hashing default admin password: %v", err))
		return 1
	}
	seeded, err := db.EnsureDefaultAdmin(ctx, defaultAdminHash)
	if err != nil {
		startup.Error(fmt.Sprintf("seeding default admin user: %v", err))
		return 1
	}
	if seeded {
		startup.Info("default admin user created (username: admin) — change the password immediately")
	} else {
		startup.Debug("admin user already exists — skipping seed")
	}

	// Clean up any expired sessions left over from a previous process.
	if n, cleanErr := sessionMgr.CleanupExpired(ctx); cleanErr != nil {
		startup.Warn(fmt.Sprintf("failed to clean up expired sessions at startup: %v", cleanErr))
	} else if n > 0 {
		startup.Info(fmt.Sprintf("cleaned up %d expired session(s) at startup", n))
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
		startup.Info(fmt.Sprintf("  - %s (%s)", org.Name, org.ChefServerURL))
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
	if toolResult.CookstyleEnabled && cfg.AnalysisTools.IsCookstyleEnabled() {
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
	} else if toolResult.CookstyleEnabled && !cfg.AnalysisTools.IsCookstyleEnabled() {
		startup.Info("CookStyle disabled via configuration (analysis_tools.cookstyle_enabled: false)")
	}

	// Test Kitchen scanner (requires both kitchen and docker, and config enabled).
	if toolResult.KitchenEnabled && cfg.AnalysisTools.TestKitchen.IsEnabled() {
		tkScanner := analysis.NewKitchenScanner(
			db, logger, toolResult.Kitchen.Path,
			cfg.Concurrency.TestKitchenRun,
			cfg.AnalysisTools.TestKitchenTimeoutMinutes,
			cfg.AnalysisTools.TestKitchen,
		)
		collOpts = append(collOpts, collector.WithKitchenScanner(tkScanner))
		startup.Info("Test Kitchen scanner enabled")
	} else if toolResult.KitchenEnabled && !cfg.AnalysisTools.TestKitchen.IsEnabled() {
		startup.Info("Test Kitchen disabled via configuration (analysis_tools.test_kitchen.enabled: false)")
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

	// Ownership evaluator — always available (reads from DB, no external tool).
	// Auto-derivation rules are evaluated after each collection run when
	// ownership tracking is enabled.
	if cfg.Ownership.Enabled {
		ownershipEval := collector.NewOwnershipEvaluator(db, cfg.Ownership, logger)
		collOpts = append(collOpts, collector.WithOwnershipEvaluator(ownershipEval))
		startup.Info("ownership evaluator enabled")
	}

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

	// -------------------------------------------------------------------
	// Ownership startup tasks
	// -------------------------------------------------------------------
	if cfg.Ownership.Enabled {
		// Remove assignments from auto-rules that have been deleted from config.
		if err := collector.CleanupRemovedAutoRules(ctx, db, cfg.Ownership, logger); err != nil {
			startup.Warn(fmt.Sprintf("ownership auto-rule cleanup failed: %v", err))
		}

		// Start daily audit log purge (runs immediately once, then every 24h).
		collector.StartAuditLogPurge(ctx, db, cfg.Ownership.AuditLog.RetentionDays, logger)
		startup.Info(fmt.Sprintf("ownership audit log purge enabled (retention: %d days)", cfg.Ownership.AuditLog.RetentionDays))
	}

	// -------------------------------------------------------------------
	// Resume interrupted collection runs from previous process
	// -------------------------------------------------------------------
	resumeResult, resumeErr := coll.ResumeInterruptedRuns(ctx)
	if resumeErr != nil {
		startup.Warn(fmt.Sprintf("failed to resume interrupted collection runs: %v", resumeErr))
	} else if resumeResult != nil && resumeResult.Evaluated > 0 {
		startup.Info(fmt.Sprintf(
			"interrupted run evaluation: %d evaluated, %d resumed, %d abandoned",
			resumeResult.Evaluated, resumeResult.Resumed, resumeResult.Abandoned,
		))
		if resumeResult.ResumedRunResult != nil {
			rr := resumeResult.ResumedRunResult
			startup.Info(fmt.Sprintf(
				"resumed collection completed: %d/%d orgs succeeded, %d nodes, %d cookbook versions in %s",
				rr.SucceededOrgs, rr.TotalOrgs, rr.TotalNodes, rr.TotalCookbooks,
				rr.Duration.Round(time.Millisecond),
			))
		}
		for runID, runErr := range resumeResult.Errors {
			startup.Warn(fmt.Sprintf("resume error for run %s: %v", runID, runErr))
		}
	}

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
	// Export output directory and cleanup ticker
	// -------------------------------------------------------------------
	exportOutputDir := cfg.Exports.OutputDirectory
	if exportOutputDir == "" {
		exportOutputDir = "/var/lib/chef-migration-metrics/exports"
	}
	if err := os.MkdirAll(exportOutputDir, 0o750); err != nil {
		startup.Error(fmt.Sprintf("creating export output directory %s: %v", exportOutputDir, err))
		return 1
	}
	startup.Info(fmt.Sprintf("export output directory: %s", exportOutputDir))

	exportCleanupLog := func(level, msg string) {
		scoped := logger.WithScope(logging.ScopeExportJob)
		switch level {
		case "DEBUG":
			scoped.Debug(msg)
		case "WARN":
			scoped.Warn(msg)
		case "ERROR":
			scoped.Error(msg)
		default:
			scoped.Info(msg)
		}
	}
	stopExportCleanup := export.StartCleanupTicker(db, exportOutputDir, 1*time.Hour, exportCleanupLog)
	defer stopExportCleanup()
	startup.Info("export cleanup ticker started (interval: 1h)")

	// -------------------------------------------------------------------
	// HTTP server — wire up the webapi.Router which owns all API routes,
	// WebSocket endpoint, health/version endpoints, and SPA fallback.
	// -------------------------------------------------------------------
	hub := webapi.NewEventHub()
	go hub.Run()

	// Attempt to load the built React frontend assets from disk.
	// The Vite build outputs to frontend/dist/. In Docker this is at
	// /src/frontend/dist during the build stage; at runtime, the binary
	// and assets are both in the image so we check the default path.
	routerOpts := []webapi.RouterOption{
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
		webapi.WithAuth(localAuth, sessionMgr, authMiddleware, db),
	}

	if frontendFS := frontend.FS(frontend.DistDir); frontendFS != nil {
		routerOpts = append(routerOpts, webapi.WithFrontendFS(frontendFS))
		if frontend.HasEmbed() {
			startup.Info("frontend SPA assets loaded from embedded binary")
		} else {
			startup.Info(fmt.Sprintf("frontend SPA assets loaded from disk: %s", frontend.DistDir))
		}
	} else {
		startup.Info(fmt.Sprintf("frontend SPA assets not found (checked embedded binary and %s) — serving plain-text placeholder", frontend.DistDir))
	}

	apiRouter := webapi.NewRouter(db, cfg, hub, routerOpts...)
	startup.Info("webapi router initialised with all API routes")

	shutdownTimeout := time.Duration(cfg.Server.GracefulShutdownSeconds) * time.Second
	if shutdownTimeout <= 0 {
		shutdownTimeout = 15 * time.Second
	}

	// tlsLog bridges the internal/tls package's LogFunc to the structured
	// logger using the ScopeTLS scope.
	tlsLog := func(level, msg string) {
		scoped := logger.WithScope(logging.ScopeTLS)
		switch level {
		case "DEBUG":
			scoped.Debug(msg)
		case "WARN":
			scoped.Warn(msg)
		case "ERROR":
			scoped.Error(msg)
		default:
			scoped.Info(msg)
		}
	}

	// -------------------------------------------------------------------
	// Server start — TLS-aware listener selection
	// -------------------------------------------------------------------
	var errCh <-chan error
	var tlsListener *apptls.Listener // non-nil only in TLS mode
	var plainSrv *http.Server        // non-nil only in plain HTTP mode

	switch cfg.Server.TLS.Mode {
	case "static":
		startup.Info("TLS mode: static (operator-managed certificate)")

		var tlsErr error
		tlsListener, tlsErr = apptls.NewListener(apiRouter, apptls.ListenerConfig{
			ListenAddress:           cfg.Server.ListenAddress,
			Port:                    cfg.Server.Port,
			CertPath:                cfg.Server.TLS.CertPath,
			KeyPath:                 cfg.Server.TLS.KeyPath,
			CAPath:                  cfg.Server.TLS.CAPath,
			MinVersion:              cfg.Server.TLS.MinVersion,
			HTTPRedirectPort:        cfg.Server.TLS.HTTPRedirectPort,
			GracefulShutdownTimeout: shutdownTimeout,
		}, tlsLog)
		if tlsErr != nil {
			startup.Error(fmt.Sprintf("TLS listener setup failed: %v", tlsErr))
			return 1
		}

		startup.Info(fmt.Sprintf("TLS certificate: %s", tlsListener.CertSummary()))
		startup.Info(fmt.Sprintf("TLS min version: %s", tlsListener.MinTLSVersionString()))
		if tlsListener.IsMTLSEnabled() {
			startup.Info("mutual TLS (mTLS) enabled — client certificates required")
		}

		// Start filesystem watcher for automatic certificate reload
		// (e.g. cert-manager in Kubernetes). Poll every 30 seconds.
		tlsListener.CertManager().WatchForChanges(30 * time.Second)

		errCh = tlsListener.Serve()

	case "acme":
		startup.Error("TLS mode 'acme' is not yet implemented")
		return 1

	default:
		// mode: off — plain HTTP
		listenAddr := cfg.Server.ListenAddress
		if listenAddr == "" {
			listenAddr = "0.0.0.0"
		}
		port := cfg.Server.Port
		if port == 0 {
			port = 8080
		}

		plainSrv = apptls.NewPlainListener(apiRouter, listenAddr, port)

		plainErrCh := make(chan error, 1)
		go func() {
			startup.Info(fmt.Sprintf("HTTP server listening on %s", plainSrv.Addr))
			if err := plainSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				plainErrCh <- err
			}
			close(plainErrCh)
		}()
		errCh = plainErrCh
	}

	// -------------------------------------------------------------------
	// Signal handling — SIGINT/SIGTERM for shutdown, SIGHUP for cert reload
	// -------------------------------------------------------------------
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	running := true
	for running {
		select {
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGHUP:
				if tlsListener != nil {
					startup.Info("received SIGHUP — reloading TLS certificate")
					if reloadErr := tlsListener.CertManager().Reload(); reloadErr != nil {
						startup.Error(fmt.Sprintf("TLS certificate reload failed: %v", reloadErr))
					} else {
						startup.Info(fmt.Sprintf("TLS certificate reloaded: %s", tlsListener.CertSummary()))
					}
				} else {
					startup.Info("received SIGHUP — no TLS certificate to reload in plain HTTP mode")
				}
			default:
				startup.Info(fmt.Sprintf("received signal %s, shutting down gracefully...", sig))
				running = false
			}
		case err := <-errCh:
			if err != nil {
				startup.Error(fmt.Sprintf("server failed: %v", err))
				return 1
			}
			running = false
		}
	}

	// Graceful shutdown — stop the scheduler first so no new collection
	// runs start, then shut down the HTTP server.
	startup.Info("stopping collection scheduler...")
	sched.Stop()
	startup.Info("collection scheduler stopped")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if tlsListener != nil {
		if err := tlsListener.Shutdown(shutdownCtx); err != nil {
			startup.Error(fmt.Sprintf("TLS server shutdown: %v", err))
			return 1
		}
	} else if plainSrv != nil {
		if err := plainSrv.Shutdown(shutdownCtx); err != nil {
			startup.Error(fmt.Sprintf("HTTP server shutdown: %v", err))
			return 1
		}
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
