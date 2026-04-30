// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package main is the entrypoint for the Chef Migration Metrics application.
// It loads configuration, connects to the database, runs migrations, syncs
// organisations from config, and starts the HTTP server with graceful
// shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"encoding/json"
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
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/collector"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/embedded"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/frontend"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/hypervisor"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/gitkitchen"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/kitchenqueue"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/nodekitchen"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/perf"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
	apptls "github.com/trickyearlobe-chef/chef-migration-metrics/internal/tls"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/webapi"
	migrations "github.com/trickyearlobe-chef/chef-migration-metrics/migrations"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	os.Exit(run())
}

// ---------------------------------------------------------------------------
// CLI flags — parsed before any application setup.
// ---------------------------------------------------------------------------

type cliFlags struct {
	configPath     string
	migrationsDir  string
	showVersion    bool
	healthcheck    bool
	healthcheckURL string
}

func parseCLI() cliFlags {
	var f cliFlags
	flag.StringVar(&f.configPath, "config", "", "Path to configuration file (or set CHEF_MIGRATION_METRICS_CONFIG)")
	flag.StringVar(&f.migrationsDir, "migrations-dir", "", "Path to SQL migrations directory (default: ./migrations or /usr/share/chef-migration-metrics/migrations)")
	flag.BoolVar(&f.showVersion, "version", false, "Print version and exit")
	flag.BoolVar(&f.healthcheck, "healthcheck", false, "Run health check against a running instance and exit")
	flag.StringVar(&f.healthcheckURL, "healthcheck-url", "", "URL for health check (default: http://localhost:<port>/api/v1/health)")
	flag.Parse()
	return f
}

// ---------------------------------------------------------------------------
// serverApp holds state that flows between startup phases.
// ---------------------------------------------------------------------------

type serverApp struct {
	cfg             *config.Config
	configuredLevel logging.Severity
	logger          *logging.Logger
	startup         *logging.ScopedLogger
	stdoutWriter    logging.Writer
	db              *datastore.DB
	hub             *webapi.EventHub

	// Config store components.
	configPath   string // path to the YAML config file
	isFullYAML   bool   // true when the loaded YAML has organisations (not bootstrap-only)
	cfgStore     *configstore.Store
	configHolder *configstore.ConfigHolder

	// Auth components.
	localAuth      *auth.LocalAuthenticator
	sessionMgr     *auth.SessionManager
	authMiddleware *auth.Middleware

	// Secrets components.
	encryptor       *secrets.Encryptor
	legacyCredStore *secrets.DBCredentialStore
	credStore       secrets.CredentialStore
	credResolver    *secrets.CredentialResolver

	// Collector components.
	coll        *collector.Collector
	sched       *collector.Scheduler
	kitchenPath string // path to kitchen binary, set during setupCollector

	// Export cleanup stop function.
	stopExportCleanup func()

	// Kitchen queue manager (bounded concurrency for TK runs).
	kitchenQueue            *kitchenqueue.Manager
	stopKitchenQueueCleanup func()
}

// dbRefChecker implements configstore.CredentialReferenceChecker by querying
// the organisations table for credentials referenced via client_key_credential_name.
type dbRefChecker struct {
	db *datastore.DB
}

func (r *dbRefChecker) CheckCredentialReferences(ctx context.Context, name string) ([]secrets.CredentialReference, error) {
	query := `
		SELECT o.name
		FROM organisations o
		WHERE o.client_key_credential_name = $1
		ORDER BY o.name`

	rows, err := r.db.Pool().QueryContext(ctx, query, name)
	if err != nil {
		return nil, fmt.Errorf("check credential references for %q: %w", name, err)
	}
	defer rows.Close()

	var refs []secrets.CredentialReference
	for rows.Next() {
		var orgName string
		if err := rows.Scan(&orgName); err != nil {
			return nil, fmt.Errorf("scan credential reference: %w", err)
		}
		refs = append(refs, secrets.CredentialReference{
			EntityType: "organisation",
			EntityName: orgName,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate credential references: %w", err)
	}

	return refs, nil
}

// ---------------------------------------------------------------------------
// Phase: bootstrap logger (stdout only, INFO level).
// ---------------------------------------------------------------------------

func (app *serverApp) setupBootstrapLogger() {
	app.stdoutWriter = logging.NewStdoutWriter()
	app.logger = logging.New(logging.Options{
		Level:   logging.INFO,
		Writers: []logging.Writer{app.stdoutWriter},
	})
	app.startup = app.logger.WithScope(logging.ScopeStartup)
}

// ---------------------------------------------------------------------------
// Phase: load configuration and reconfigure logger.
// ---------------------------------------------------------------------------

func (app *serverApp) loadConfig(configPath string) error {
	// Resolve the config path the same way config.Load does, so we can
	// store it for the YAML auto-migration phase later.
	if configPath == "" {
		configPath = os.Getenv("CHEF_MIGRATION_METRICS_CONFIG")
	}
	app.configPath = configPath

	// Use LoadRaw (no validation) first. Bootstrap YAML files written
	// after YAML-to-DB migration contain only database_url,
	// listen_address, listen_port and would fail full validation.
	// Validation is deferred: full YAML is validated here; bootstrap
	// YAML is validated later on the assembled-from-DB config in
	// setupConfigStore → AssembleConfig.
	cfg, err := config.LoadRaw(configPath)
	if err != nil {
		app.startup.Error(fmt.Sprintf("loading configuration: %v", err))
		return err
	}

	app.configuredLevel = logging.INFO
	if cfg.Logging.Level != "" {
		parsed, parseErr := logging.ParseSeverity(cfg.Logging.Level)
		if parseErr != nil {
			app.startup.Warn(fmt.Sprintf("config: %v", parseErr))
		}
		app.configuredLevel = parsed
	}
	app.logger = logging.New(logging.Options{
		Level:   app.configuredLevel,
		Writers: []logging.Writer{app.stdoutWriter},
	})
	app.startup = app.logger.WithScope(logging.ScopeStartup)

	// Detect whether the YAML is a full config (has organisations) or
	// bootstrap-only. This determines whether YAML auto-migration runs.
	app.isFullYAML = configstore.IsFullYAML(cfg)
	if app.isFullYAML {
		// Full YAML must be validated now — MigrateFromYAML expects a
		// validated config, and this is the only config the app will use
		// if config_store is empty.
		warnings, valErr := cfg.Validate()
		if valErr != nil {
			app.startup.Error(fmt.Sprintf("loading configuration: %v", valErr))
			return valErr
		}
		if warnings != nil {
			for _, w := range warnings.Messages {
				app.startup.Warn(fmt.Sprintf("config: %s", w))
			}
		}
		app.startup.Info("configuration loaded successfully (full YAML detected)")
	} else {
		app.startup.Info("configuration loaded successfully (bootstrap YAML detected)")
	}

	app.cfg = cfg
	return nil
}

// ---------------------------------------------------------------------------
// Phase: database connection, pool configuration, event hub, DB log writer.
// ---------------------------------------------------------------------------

func (app *serverApp) setupDatabase() error {
	dbURL := app.cfg.Datastore.URL
	if dbURL == "" {
		dbURL = os.Getenv("CMM_DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		app.startup.Error("no database URL configured (set datastore.url in config, CMM_DATABASE_URL, or DATABASE_URL)")
		return fmt.Errorf("no database URL configured")
	}

	db, err := datastore.Open(dbURL)
	if err != nil {
		app.startup.Error(fmt.Sprintf("connecting to database: %v", err))
		return err
	}

	db.Configure(
		app.cfg.Datastore.MaxOpenConns,
		app.cfg.Datastore.MaxIdleConns,
		time.Duration(app.cfg.Datastore.ConnMaxLifetimeMinutes)*time.Minute,
		time.Duration(app.cfg.Datastore.ConnMaxIdleTimeMinutes)*time.Minute,
	)
	app.startup.Info("database connection established")
	app.db = db

	// EventHub — create early so the DBWriter broadcast callback can
	// capture it. The run loop starts immediately in a background
	// goroutine; it will be stopped during graceful shutdown.
	app.hub = webapi.NewEventHub()
	go app.hub.Run()

	return nil
}

// ---------------------------------------------------------------------------
// Phase: attach DB log writer for persisting log entries.
// ---------------------------------------------------------------------------

func (app *serverApp) attachDBWriter() {
	dbAdapter := logging.NewDatastoreAdapter(
		func(ctx context.Context, p logging.LogEntryParams) (string, error) {
			entry, dsErr := app.db.InsertLogEntry(ctx, datastore.InsertLogEntryParams{
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
				CollectionRunOrg:    p.CollectionRunID,
				NotificationChannel: p.NotificationChannel,
				ExportJobID:         p.ExportJobID,
				TLSDomain:           p.TLSDomain,
			})
			if dsErr != nil {
				return "", dsErr
			}
			return fmt.Sprintf("%d", entry.ID), nil
		},
	)
	hub := app.hub
	dbWriter := logging.NewDBWriter(dbAdapter,
		logging.WithContext(context.Background()),
		logging.WithOnError(func(entry logging.Entry, dbErr error) {
			log.Printf("WARN: failed to persist log entry to database: %v", dbErr)
		}),
		logging.WithOnBroadcast(func(entry logging.Entry) {
			hub.Broadcast(webapi.NewEvent(webapi.EventLogEntry, map[string]any{
				"severity":             entry.Severity.String(),
				"scope":                string(entry.Scope),
				"message":              entry.Message,
				"timestamp":            entry.Timestamp.Format(time.RFC3339Nano),
				"organisation":         entry.Organisation,
				"cookbook_name":        entry.CookbookName,
				"cookbook_version":     entry.CookbookVersion,
				"commit_sha":           entry.CommitSHA,
				"chef_client_version":  entry.ChefClientVersion,
				"collection_run_id":    entry.CollectionRunID,
				"notification_channel": entry.NotificationChannel,
				"export_job_id":        entry.ExportJobID,
				"tls_domain":           entry.TLSDomain,
				// process_output intentionally omitted — too large for
				// WebSocket frames. Clients fetch full detail via REST.
			}))
		}),
	)

	app.logger = logging.New(logging.Options{
		Level:   app.configuredLevel,
		Writers: []logging.Writer{app.stdoutWriter, dbWriter},
	})
	app.startup = app.logger.WithScope(logging.ScopeStartup)
	app.startup.Debug("database log writer attached")
}

// ---------------------------------------------------------------------------
// Phase: run database migrations.
// ---------------------------------------------------------------------------

func (app *serverApp) runMigrations(ctx context.Context, migrationsDir string) error {
	var applied int
	var err error
	if migrationsDir != "" {
		migDir := resolveMigrationsDir(migrationsDir)
		if migDir == "" {
			app.startup.Error("migrations directory not found — pass a valid -migrations-dir path")
			return fmt.Errorf("migrations directory not found")
		}
		app.startup.Info(fmt.Sprintf("using disk migrations from %s", migDir))
		applied, err = app.db.MigrateUp(ctx, migDir)
	} else {
		app.startup.Info("using embedded migrations")
		applied, err = app.db.MigrateUpFS(ctx, migrations.FS())
	}
	if err != nil {
		app.startup.Error(fmt.Sprintf("running database migrations: %v", err))
		return err
	}
	if applied > 0 {
		app.startup.Info(fmt.Sprintf("applied %d database migration(s)", applied))
	} else {
		app.startup.Info("database schema is up to date")
	}

	ver, verErr := app.db.MigrationVersion(ctx)
	if verErr != nil {
		app.startup.Warn(fmt.Sprintf("could not read migration version: %v", verErr))
	} else {
		app.startup.Info(fmt.Sprintf("database schema version: %d", ver))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Phase: authentication setup (local auth, sessions, middleware, admin seed).
// ---------------------------------------------------------------------------

func (app *serverApp) setupAuth(ctx context.Context) error {
	authLog := app.logger.WithScope(logging.ScopeAuth)
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

	sessionLifetime := auth.ParseDuration(app.cfg.Auth.SessionExpiry, 8*time.Hour)

	app.sessionMgr = auth.NewSessionManager(app.db, sessionLifetime,
		auth.WithSessionLogger(authLogFn),
	)

	app.localAuth = auth.NewLocalAuthenticator(app.db, app.cfg.Auth.LockoutAttempts,
		auth.WithLocalAuthLogger(authLogFn),
	)

	app.authMiddleware = auth.NewMiddleware(app.sessionMgr,
		auth.WithMiddlewareLogger(authLogFn),
	)

	app.startup.Info(fmt.Sprintf("authentication configured: session_expiry=%s, lockout_attempts=%d, min_password_length=%d",
		sessionLifetime, app.cfg.Auth.LockoutAttempts, app.cfg.Auth.MinPasswordLength))

	// Seed default admin user if no users exist yet.
	defaultAdminPassword := os.Getenv("CMM_DEFAULT_ADMIN_PASSWORD")
	if defaultAdminPassword == "" {
		defaultAdminPassword = "ChefMigrate1"
	}
	defaultAdminHash, err := auth.HashPassword(defaultAdminPassword)
	if err != nil {
		app.startup.Error(fmt.Sprintf("hashing default admin password: %v", err))
		return err
	}
	seeded, err := app.db.EnsureDefaultAdmin(ctx, defaultAdminHash)
	if err != nil {
		app.startup.Error(fmt.Sprintf("seeding default admin user: %v", err))
		return err
	}
	if seeded {
		app.startup.Info("default admin user created (username: admin) — change the password immediately")
	} else {
		app.startup.Debug("admin user already exists — skipping seed")
	}

	// Start periodic expired session cleanup (runs immediately, then hourly).
	auth.StartSessionCleanup(ctx, app.sessionMgr)
	app.startup.Info("session cleanup started (interval: 1h)")
	return nil
}

// ---------------------------------------------------------------------------
// Phase: mark interrupted collection runs from previous process.
// ---------------------------------------------------------------------------

func (app *serverApp) markInterruptedRuns(ctx context.Context) {
	staleRuns, err := app.db.GetRunningCollectionRuns(ctx)
	if err != nil {
		app.startup.Warn(fmt.Sprintf("could not check for interrupted collection runs: %v", err))
		return
	}
	for _, r := range staleRuns {
		if _, err := app.db.InterruptCollectionRun(ctx, r.OrganisationName); err != nil {
			app.startup.Warn(fmt.Sprintf("could not mark collection run %s as interrupted: %v", r.OrganisationName, err))
		} else {
			app.startup.Info(fmt.Sprintf("marked stale collection run %s as interrupted", r.OrganisationName))
		}
	}
}

// ---------------------------------------------------------------------------
// Phase: secrets — master key, credential store, rotation, validation.
// ---------------------------------------------------------------------------

func (app *serverApp) setupSecrets(ctx context.Context) error {
	secretsLog := app.logger.WithScope(logging.ScopeSecrets)

	masterKeyEnvName := app.cfg.CredentialEncryptionKeyEnv
	if masterKeyEnvName == "" {
		masterKeyEnvName = "CMM_CREDENTIAL_ENCRYPTION_KEY"
	}

	masterKeyBase64 := os.Getenv(masterKeyEnvName)
	if masterKeyBase64 == "" {
		secretsLog.Error(fmt.Sprintf("%s environment variable is required but not set", masterKeyEnvName))
		return fmt.Errorf("encryption key %s is required", masterKeyEnvName)
	}

	enc, mkErr := secrets.NewEncryptor(masterKeyBase64)
	if mkErr != nil {
		secretsLog.Error(fmt.Sprintf("master encryption key from %s is invalid: %v", masterKeyEnvName, mkErr))
		return mkErr
	}
	app.encryptor = enc
	secretsLog.Info(fmt.Sprintf("master encryption key loaded from %s", masterKeyEnvName))

	// Create the config store backed by the database and encryptor.
	app.cfgStore = configstore.NewStore(app.db, app.encryptor)

	// Create a legacy DBCredentialStore — used for rotation and validation
	// of credentials that have not yet been migrated to config_store.
	app.legacyCredStore = secrets.NewDBCredentialStore(app.db.Pool(), app.encryptor)

	credCount, credCountErr := app.legacyCredStore.CredentialCount(ctx)
	if credCountErr != nil {
		secretsLog.Warn(fmt.Sprintf("could not count stored credentials: %v", credCountErr))
	} else if credCount > 0 {
		secretsLog.Info(fmt.Sprintf("%d stored credential(s) found in legacy table", credCount))
	}

	// Master key rotation on legacy credentials (if previous key is provided).
	// Rotation must happen before MigrateFromLegacy re-encrypts them.
	if err := app.rotateSecrets(ctx, secretsLog); err != nil {
		return err
	}

	// Validate all stored credentials can be decrypted.
	if credCount > 0 {
		app.validateCredentials(ctx, secretsLog)
	}

	// Warn on overly permissive key file permissions.
	app.checkKeyFilePermissions(secretsLog)

	return nil
}

// ---------------------------------------------------------------------------
// Phase: config store migration and assembly.
// ---------------------------------------------------------------------------

func (app *serverApp) setupConfigStore(ctx context.Context) error {
	csLog := app.logger.WithScope(logging.ScopeStartup)

	// Run legacy data migration (credentials + runtime_settings → config_store).
	migrateResult, err := configstore.MigrateFromLegacy(ctx, app.db.Pool(), app.cfgStore, app.encryptor)
	if err != nil {
		csLog.Error(fmt.Sprintf("legacy data migration failed: %v", err))
		return err
	}
	if migrateResult.Skipped {
		csLog.Info(fmt.Sprintf("legacy data migration skipped: %s", migrateResult.SkipReason))
	} else {
		csLog.Info(fmt.Sprintf("legacy data migration complete: %d credential(s), %d runtime setting(s) migrated",
			migrateResult.CredentialsMigrated, migrateResult.RuntimeSettingsMigrated))
	}

	// Run YAML auto-migration if a full YAML was detected.
	if app.isFullYAML {
		yamlResult, yamlErr := configstore.MigrateFromYAML(ctx, app.cfgStore, app.cfg, app.configPath)
		if yamlErr != nil {
			csLog.Error(fmt.Sprintf("YAML auto-migration failed: %v", yamlErr))
			return yamlErr
		}
		if yamlResult.Skipped {
			csLog.Warn(fmt.Sprintf("YAML config file contains settings beyond bootstrap values — these are ignored; config is managed in the database (%s)", yamlResult.SkipReason))
		} else {
			csLog.Info(fmt.Sprintf("configuration migrated to database: %d section(s). Original saved as %s.migrated",
				yamlResult.SectionsMigrated, app.configPath))
		}
	}

	// Assemble config from DB if config_store has config section keys.
	// Credential-only entries (from MigrateFromLegacy) don't count — we
	// need actual config sections before we can replace the YAML config.
	hasSections, hsErr := configstore.HasConfigSections(ctx, app.cfgStore)
	if hsErr != nil {
		csLog.Error(fmt.Sprintf("checking config_store sections: %v", hsErr))
		return hsErr
	}

	if hasSections {
		assembled, warnings, assembleErr := configstore.AssembleConfig(ctx, app.cfgStore)
		if assembleErr != nil {
			csLog.Error(fmt.Sprintf("assembling config from database: %v", assembleErr))
			return assembleErr
		}
		if warnings != nil {
			for _, w := range warnings.Messages {
				csLog.Warn(fmt.Sprintf("config (from DB): %s", w))
			}
		}

		// Carry over bootstrap values from the YAML-loaded config.
		assembled.Datastore.URL = app.cfg.Datastore.URL
		assembled.Server.ListenAddress = app.cfg.Server.ListenAddress
		assembled.Server.Port = app.cfg.Server.Port

		app.cfg = assembled
		csLog.Info("configuration assembled from database")
	} else {
		csLog.Info("config_store is empty — using YAML configuration")
	}

	// Wire up the CredentialStoreAdapter to replace the legacy DBCredentialStore.
	app.credStore = configstore.NewCredentialStoreAdapter(app.cfgStore, &dbRefChecker{db: app.db})
	app.credResolver = secrets.NewCredentialResolver(app.credStore)
	csLog.Info("credential store adapter configured (backed by config_store)")

	// Create the ConfigHolder for concurrent-safe config access.
	app.configHolder = configstore.NewConfigHolder(app.cfg, app.cfgStore)

	return nil
}

func (app *serverApp) rotateSecrets(ctx context.Context, secretsLog *logging.ScopedLogger) error {
	if !secrets.NeedsRotation(os.LookupEnv) {
		return nil
	}

	if app.encryptor == nil {
		secretsLog.Error("CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS is set but no current master key is configured — cannot rotate")
		return fmt.Errorf("previous key set without current key")
	}

	prevKeyBase64, _ := os.LookupEnv("CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS")
	prevEncryptor, prevErr := secrets.NewEncryptor(prevKeyBase64)
	if prevErr != nil {
		secretsLog.Error(fmt.Sprintf("previous master encryption key is invalid: %v", prevErr))
		return prevErr
	}
	defer prevEncryptor.Close()

	secretsLog.Info("master key rotation requested — re-encrypting stored credentials")

	rotationRows, rrErr := app.legacyCredStore.ListRotationRows(ctx)
	if rrErr != nil {
		secretsLog.Error(fmt.Sprintf("failed to read credentials for rotation: %v", rrErr))
		return rrErr
	}

	rotationWriter := func(wCtx context.Context, row secrets.RotatedRow) error {
		return app.legacyCredStore.UpdateEncryptedValueRaw(wCtx, row.Name, row.NewEncryptedValue)
	}

	result, rotErr := secrets.RotateMasterKey(ctx, rotationRows, app.encryptor, prevEncryptor, rotationWriter)
	if rotErr != nil {
		secretsLog.Error(fmt.Sprintf("master key rotation failed: %v", rotErr))
		return rotErr
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
	return nil
}

func (app *serverApp) validateCredentials(ctx context.Context, secretsLog *logging.ScopedLogger) {
	rotationRows, rrErr := app.legacyCredStore.ListRotationRows(ctx)
	if rrErr != nil {
		secretsLog.Warn(fmt.Sprintf("could not validate stored credentials: %v", rrErr))
		return
	}
	decryptFailures := 0
	for _, row := range rotationRows {
		aad, aadErr := secrets.BuildAAD(row.CredentialType, row.Name)
		if aadErr != nil {
			secretsLog.Error(fmt.Sprintf("credential %q: failed to build AAD: %v", row.Name, aadErr))
			decryptFailures++
			continue
		}
		plaintext, decErr := app.encryptor.Decrypt(row.EncryptedValue, aad)
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

func (app *serverApp) checkKeyFilePermissions(secretsLog *logging.ScopedLogger) {
	for _, org := range app.cfg.Organisations {
		if org.ClientKeyPath == "" {
			continue
		}
		info, statErr := os.Stat(org.ClientKeyPath)
		if statErr != nil {
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
}

// ---------------------------------------------------------------------------
// Phase: sync organisations from configuration.
// ---------------------------------------------------------------------------

func (app *serverApp) syncOrganisations(ctx context.Context) error {
	orgParams := make([]datastore.UpsertOrganisationParams, 0, len(app.cfg.Organisations))
	for _, org := range app.cfg.Organisations {
		orgParams = append(orgParams, datastore.UpsertOrganisationParams{
			Name:          org.Name,
			ChefServerURL: org.ChefServerURL,
			OrgName:       org.OrgName,
			ClientName:    org.ClientName,
		})
	}

	orgs, err := app.db.SyncOrganisationsFromConfig(ctx, orgParams)
	if err != nil {
		app.startup.Error(fmt.Sprintf("syncing organisations from config: %v", err))
		return err
	}
	app.startup.Info(fmt.Sprintf("%d organisation(s) synced from configuration", len(orgs)))
	for _, org := range orgs {
		app.startup.Info(fmt.Sprintf("  - %s (%s)", org.Name, org.ChefServerURL))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Phase: reconcile stale target version data.
// ---------------------------------------------------------------------------

func (app *serverApp) reconcileTargetVersions(ctx context.Context) {
	versions := app.cfg.TargetChefVersions
	if len(versions) == 0 {
		app.startup.Warn("no target_chef_versions configured — skipping stale version reconciliation")
		return
	}

	result, err := app.db.PurgeStaleTargetVersionData(ctx, versions)
	if err != nil {
		app.startup.Warn(fmt.Sprintf("failed to purge stale target version data: %v", err))
		return
	}

	if result.Total() == 0 {
		app.startup.Info("target version reconciliation: no stale data found")
		return
	}

	app.startup.Info(fmt.Sprintf(
		"target version reconciliation: purged %d stale record(s) for versions not in %v",
		result.Total(), versions))

	if result.NodeReadiness > 0 {
		app.startup.Info(fmt.Sprintf("  - node_readiness: %d", result.NodeReadiness))
	}
	if result.ServerCookbookCookstyleResults > 0 {
		app.startup.Info(fmt.Sprintf("  - server_cookbook_cookstyle_results: %d", result.ServerCookbookCookstyleResults))
	}
	if result.ServerCookbookComplexity > 0 {
		app.startup.Info(fmt.Sprintf("  - server_cookbook_complexity: %d", result.ServerCookbookComplexity))
	}
	if result.ServerCookbookAutocorrectPreviews > 0 {
		app.startup.Info(fmt.Sprintf("  - server_cookbook_autocorrect_previews: %d", result.ServerCookbookAutocorrectPreviews))
	}
	if result.GitRepoCookstyleResults > 0 {
		app.startup.Info(fmt.Sprintf("  - git_repo_cookstyle_results: %d", result.GitRepoCookstyleResults))
	}
	if result.GitRepoComplexity > 0 {
		app.startup.Info(fmt.Sprintf("  - git_repo_complexity: %d", result.GitRepoComplexity))
	}
	if result.GitRepoAutocorrectPreviews > 0 {
		app.startup.Info(fmt.Sprintf("  - git_repo_autocorrect_previews: %d", result.GitRepoAutocorrectPreviews))
	}
	if result.MetricSnapshots > 0 {
		app.startup.Info(fmt.Sprintf("  - metric_snapshots: %d", result.MetricSnapshots))
	}
}

// ---------------------------------------------------------------------------
// Phase: analysis pipeline and collector setup.
// ---------------------------------------------------------------------------

func (app *serverApp) setupCollector(ctx context.Context) error {
	toolResolver := embedded.NewResolver(app.cfg.AnalysisTools.EmbeddedBinDir)
	toolResult := toolResolver.ValidateAll(ctx)

	if toolResult.Git.Available {
		app.startup.Info(fmt.Sprintf("git available: %s (version %s)", toolResult.Git.Path, toolResult.Git.Version))
	} else {
		app.startup.Warn(fmt.Sprintf("git not available: %s — git cookbook fetching will fail", toolResult.Git.Error))
	}
	if toolResult.Cookstyle.Available {
		app.startup.Info(fmt.Sprintf("cookstyle available: %s (version %s)", toolResult.Cookstyle.Path, toolResult.Cookstyle.Version))
	} else {
		app.startup.Info(fmt.Sprintf("cookstyle not available: %s — CookStyle scanning disabled", toolResult.Cookstyle.Error))
	}
	if toolResult.Kitchen.Available {
		app.startup.Info(fmt.Sprintf("kitchen available: %s (version %s)", toolResult.Kitchen.Path, toolResult.Kitchen.Version))
	} else {
		app.startup.Info(fmt.Sprintf("kitchen not available: %s — Test Kitchen testing disabled", toolResult.Kitchen.Error))
	}
	if !toolResult.CookstyleEnabled && !toolResult.KitchenEnabled {
		app.startup.Warn("neither CookStyle nor Test Kitchen available — no cookbook compatibility testing will be performed")
	}

	var collOpts []collector.Option

	if toolResult.CookstyleEnabled && app.cfg.AnalysisTools.IsCookstyleEnabled() {
		csScanner := analysis.NewCookstyleScanner(
			app.db, app.logger, toolResult.Cookstyle.Path,
			app.cfg.Concurrency.CookstyleScan,
			app.cfg.AnalysisTools.CookstyleTimeoutMinutes,
		)
		collOpts = append(collOpts, collector.WithCookstyleScanner(csScanner))
		app.startup.Info("CookStyle scanner enabled")

		acGen := remediation.NewAutocorrectGenerator(
			app.db, app.logger, toolResult.Cookstyle.Path,
			app.cfg.AnalysisTools.CookstyleTimeoutMinutes,
		)
		collOpts = append(collOpts, collector.WithAutocorrectGenerator(acGen))
		app.startup.Info("autocorrect preview generator enabled")
	} else if toolResult.CookstyleEnabled && !app.cfg.AnalysisTools.IsCookstyleEnabled() {
		app.startup.Info("CookStyle disabled via configuration (analysis_tools.cookstyle_enabled: false)")
	}

	if toolResult.KitchenEnabled {
		app.kitchenPath = toolResult.Kitchen.Path
	}

	cxScorer := remediation.NewComplexityScorer(app.db, app.logger)
	collOpts = append(collOpts, collector.WithComplexityScorer(cxScorer))

	readinessEval := analysis.NewReadinessEvaluator(
		app.db, app.logger,
		app.cfg.Concurrency.ReadinessEvaluation,
		app.cfg.Readiness.MinFreeDiskMB,
	)
	collOpts = append(collOpts, collector.WithReadinessEvaluator(readinessEval))

	if app.cfg.Ownership.Enabled {
		ownershipEval := collector.NewOwnershipEvaluator(app.db, app.cfg.Ownership, app.logger)
		collOpts = append(collOpts, collector.WithOwnershipEvaluator(ownershipEval))
		app.startup.Info("ownership evaluator enabled")
	}

	// Ensure storage directories exist.
	if err := app.ensureStorageDirs(); err != nil {
		return err
	}

	// Cookbook directory resolvers.
	collOpts = append(collOpts, app.cookbookDirOpts()...)

	app.startup.Info("analysis pipeline configured: complexity scorer and readiness evaluator always enabled")

	app.coll = collector.New(app.db, app.cfg, app.logger, app.credResolver, collOpts...)
	return nil
}

func (app *serverApp) ensureStorageDirs() error {
	for _, dir := range []struct{ name, path string }{
		{"data_dir", app.cfg.Storage.DataDir},
		{"cookbook_cache_dir", app.cfg.Storage.CookbookCacheDir},
		{"git_cookbook_dir", app.cfg.Storage.GitCookbookDir},
	} {
		if err := os.MkdirAll(dir.path, 0o750); err != nil {
			app.startup.Error(fmt.Sprintf("creating %s %s: %v", dir.name, dir.path, err))
			return err
		}
	}
	return nil
}

func (app *serverApp) cookbookDirOpts() []collector.Option {
	gitCookbookDir := app.cfg.Storage.GitCookbookDir
	cookbookCacheDir := app.cfg.Storage.CookbookCacheDir
	deleteAfterScan := app.cfg.Collection.DeleteServerCookbooksAfterScanEnabled()

	opts := []collector.Option{
		collector.WithCookbookCacheDir(cookbookCacheDir),
		collector.WithGitCookbookDir(gitCookbookDir),
		collector.WithServerCookbookDirFn(func(sc datastore.ServerCookbook) string {
			if deleteAfterScan {
				return ""
			}
			return filepath.Join(cookbookCacheDir, sc.OrganisationName, sc.Name, sc.Version)
		}),
		collector.WithGitRepoDirFn(func(repo datastore.GitRepo) string {
			return filepath.Join(gitCookbookDir, repo.Name)
		}),
	}

	app.startup.Info(fmt.Sprintf("storage paths: data_dir=%s, cookbook_cache=%s, git_cookbooks=%s",
		app.cfg.Storage.DataDir, cookbookCacheDir, gitCookbookDir))
	if deleteAfterScan {
		app.startup.Info("server cookbook files will be deleted after scanning (delete_server_cookbooks_after_scan: true)")
	} else {
		app.startup.Info(fmt.Sprintf("server cookbook files will be retained in %s for re-scanning", cookbookCacheDir))
	}
	return opts
}

// ---------------------------------------------------------------------------
// Phase: ownership startup tasks.
// ---------------------------------------------------------------------------

func (app *serverApp) setupOwnership(ctx context.Context) {
	if !app.cfg.Ownership.Enabled {
		return
	}
	if err := collector.CleanupRemovedAutoRules(ctx, app.db, app.cfg.Ownership, app.logger); err != nil {
		app.startup.Warn(fmt.Sprintf("ownership auto-rule cleanup failed: %v", err))
	}
	collector.StartAuditLogPurge(ctx, app.db, app.cfg.Ownership.AuditLog.RetentionDays, app.logger)
	app.startup.Info(fmt.Sprintf("ownership audit log purge enabled (retention: %d days)", app.cfg.Ownership.AuditLog.RetentionDays))
}

// ---------------------------------------------------------------------------
// Phase: resume interrupted runs and start collection scheduler.
// ---------------------------------------------------------------------------

func (app *serverApp) startScheduler(ctx context.Context) error {
	resumeResult, resumeErr := app.coll.ResumeInterruptedRuns(ctx)
	if resumeErr != nil {
		app.startup.Warn(fmt.Sprintf("failed to resume interrupted collection runs: %v", resumeErr))
	} else if resumeResult != nil && resumeResult.Evaluated > 0 {
		app.startup.Info(fmt.Sprintf(
			"interrupted run evaluation: %d evaluated, %d resumed, %d abandoned",
			resumeResult.Evaluated, resumeResult.Resumed, resumeResult.Abandoned,
		))
		if resumeResult.ResumedRunResult != nil {
			rr := resumeResult.ResumedRunResult
			app.startup.Info(fmt.Sprintf(
				"resumed collection completed: %d/%d orgs succeeded, %d nodes, %d cookbook versions in %s",
				rr.SucceededOrgs, rr.TotalOrgs, rr.TotalNodes, rr.TotalCookbooks,
				rr.Duration.Round(time.Millisecond),
			))
		}
		for runID, runErr := range resumeResult.Errors {
			app.startup.Warn(fmt.Sprintf("resume error for run %s: %v", runID, runErr))
		}
	}

	schedule, schedErr := collector.ParseSchedule(app.cfg.Collection.Schedule)
	if schedErr != nil {
		app.startup.Error(fmt.Sprintf("invalid collection schedule %q: %v", app.cfg.Collection.Schedule, schedErr))
		return schedErr
	}

	app.sched = collector.NewScheduler(app.coll, schedule, app.logger)
	if err := app.sched.Start(); err != nil {
		app.startup.Error(fmt.Sprintf("starting collection scheduler: %v", err))
		return err
	}
	app.startup.Info(fmt.Sprintf("collection scheduler started (schedule: %s)", app.cfg.Collection.Schedule))
	return nil
}

// ---------------------------------------------------------------------------
// Phase: export output directory and cleanup ticker.
// ---------------------------------------------------------------------------

func (app *serverApp) setupExports() error {
	exportOutputDir := app.cfg.Exports.OutputDirectory
	if exportOutputDir == "" {
		exportOutputDir = "/var/lib/chef-migration-metrics/exports"
	}
	if err := os.MkdirAll(exportOutputDir, 0o750); err != nil {
		app.startup.Error(fmt.Sprintf("creating export output directory %s: %v", exportOutputDir, err))
		return err
	}
	app.startup.Info(fmt.Sprintf("export output directory: %s", exportOutputDir))

	exportCleanupLog := func(level, msg string) {
		scoped := app.logger.WithScope(logging.ScopeExportJob)
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
	app.stopExportCleanup = export.StartCleanupTicker(app.db, exportOutputDir, 1*time.Hour, exportCleanupLog)
	app.startup.Info("export cleanup ticker started (interval: 1h)")
	return nil
}

// ---------------------------------------------------------------------------
// Phase: hypervisor client construction.
// ---------------------------------------------------------------------------

func (app *serverApp) buildHypervisorClient() (hypervisor.Hypervisor, error) {
	// Read TK config from runtime_settings (where the UI saves it) rather
	// than the stale config_store assembly.
	tk, err := app.loadTestKitchenRuntime()
	if err != nil {
		return nil, fmt.Errorf("loading runtime TK config: %w", err)
	}
	if tk == nil {
		// No runtime setting saved yet — fall back to assembled config.
		fallback := app.cfg.AnalysisTools.TestKitchen
		tk = &fallback
	}

	hypType := tk.EffectiveHypervisorType()
	if hypType == "" {
		return nil, nil
	}

	// Resolve driver secrets needed for hypervisor auth.
	resolvedSecrets := make(map[string]string, len(tk.DriverSecrets))
	for key, credName := range tk.DriverSecrets {
		resolved, err := app.credResolver.Resolve(context.Background(), secrets.CredentialSource{
			CredentialName: credName,
		})
		if err != nil {
			return nil, fmt.Errorf("resolving driver secret %q (credential %q): %w", key, credName, err)
		}
		resolvedSecrets[key] = string(resolved.Plaintext)
		secrets.ZeroBytes(resolved.Plaintext)
	}

	return hypervisor.NewFromConfig(hypType, tk.DriverSettings, resolvedSecrets)
}

// loadTestKitchenRuntime reads the TK config from runtime_settings (the source
// the UI writes to). Returns nil when no runtime setting exists.
func (app *serverApp) loadTestKitchenRuntime() (*config.TestKitchenConfig, error) {
	setting, err := app.db.GetRuntimeSetting(context.Background(), "test_kitchen")
	if err != nil {
		return nil, err
	}
	if setting == nil {
		return nil, nil
	}
	var tk config.TestKitchenConfig
	if err := json.Unmarshal(setting.Value, &tk); err != nil {
		return nil, fmt.Errorf("unmarshalling test_kitchen runtime setting: %w", err)
	}
	return &tk, nil
}

// ---------------------------------------------------------------------------
// Phase: HTTP server setup and serve.
// ---------------------------------------------------------------------------

type serverResult struct {
	errCh       <-chan error
	tlsListener *apptls.Listener
	plainSrv    *http.Server
}

func (app *serverApp) setupAndServeHTTP() (serverResult, error) {
	var recorder *perf.Recorder
	if app.cfg.Performance.IsEnabled() {
		windowSec := app.cfg.Performance.WindowSeconds
		if windowSec <= 0 {
			windowSec = 300
		}
		recorder = perf.NewRecorder(time.Duration(windowSec)*time.Second, 200, 1000)
		app.startup.Info(fmt.Sprintf("performance instrumentation enabled (window=%ds)", windowSec))
		if app.cfg.Performance.PprofEnabled {
			app.startup.Warn("pprof endpoints enabled — do not use in production without auth")
		}
	}

	coll := app.coll
	sched := app.sched
	logger := app.logger
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
		webapi.WithAuth(app.localAuth, app.sessionMgr, app.authMiddleware, app.db),
		webapi.WithCollectionTrigger(func(ctx context.Context) error {
			if coll.IsRunning() {
				return fmt.Errorf("a collection run is already in progress")
			}
			go func() {
				triggerLog := logger.WithScope(logging.ScopeCollectionRun)
				triggerLog.Info("manually triggered collection run (via rescan)")
				if _, err := sched.TriggerNow(ctx); err != nil {
					triggerLog.Error(fmt.Sprintf("triggered collection run failed: %v", err))
				}
			}()
			return nil
		}),
	}

	if recorder != nil {
		routerOpts = append(routerOpts, webapi.WithPerformance(recorder))
	}

	if app.credStore != nil {
		routerOpts = append(routerOpts, webapi.WithCredentialStore(app.credStore))
	}

	if app.cfgStore != nil && app.configHolder != nil {
		routerOpts = append(routerOpts, webapi.WithConfigStore(app.cfgStore, app.configHolder))
	}

	// Wire hypervisor client for template discovery and orphan sweep.
	if hyp, hypErr := app.buildHypervisorClient(); hypErr != nil {
		app.startup.Warn(fmt.Sprintf("hypervisor client not available: %v", hypErr))
	} else if hyp != nil {
		routerOpts = append(routerOpts, webapi.WithHypervisor(hyp))
		tkRT, _ := app.loadTestKitchenRuntime()
		if tkRT != nil {
			app.startup.Info(fmt.Sprintf("hypervisor client initialised (type=%s)", tkRT.EffectiveHypervisorType()))
		} else {
			app.startup.Info("hypervisor client initialised")
		}
	} else {
		app.startup.Info("hypervisor not configured (no driver/hypervisor_type in runtime settings)")
	}

	// Wire Node Kitchen runner factory when kitchen binary is available.
	if app.kitchenPath != "" {
		nkScoped := logger.WithScope(logging.ScopeTestKitchenRun)
		nkLogger := &nodekitchen.ScopedLoggerAdapter{
			Info_:  func(msg string) { _ = nkScoped.Info(msg) },
			Warn_:  func(msg string) { _ = nkScoped.Warn(msg) },
			Error_: func(msg string) { _ = nkScoped.Error(msg) },
		}
		factory := &nodekitchen.RunnerFactory{
			DB:          app.db,
			DepResolver: &nodekitchen.DBCookbookDependencyResolver{DB: app.db},
			ClientFactory: func(ctx context.Context, orgName string) (*chefapi.Client, error) {
				org, err := app.db.GetOrganisation(ctx, orgName)
				if err != nil {
					return nil, fmt.Errorf("looking up organisation %q: %w", orgName, err)
				}

				src := secrets.CredentialSource{
					CredentialName: org.ClientKeyCredentialName,
				}
				for _, cfgOrg := range app.cfg.Organisations {
					if cfgOrg.Name == org.Name {
						if src.CredentialName == "" {
							src.CredentialName = cfgOrg.ClientKeyCredential
						}
						if src.FilePath == "" {
							src.FilePath = cfgOrg.ClientKeyPath
						}
						break
					}
				}

				resolved, err := app.credResolver.Resolve(ctx, src)
				if err != nil {
					return nil, fmt.Errorf("resolving client key for org %q: %w", orgName, err)
				}
				defer secrets.ZeroBytes(resolved.Plaintext)

				sslVerify := true
				for _, cfgOrg := range app.cfg.Organisations {
					if cfgOrg.Name == org.Name {
						sslVerify = cfgOrg.SSLVerifyEnabled()
						break
					}
				}

				return chefapi.NewClient(chefapi.ClientConfig{
					ServerURL:     org.ChefServerURL,
					ClientName:    org.ClientName,
					PrivateKeyPEM: resolved.Plaintext,
					OrgName:       org.OrgName,
					SSLVerify:     &sslVerify,
				})
			},
			Executor:       &nodekitchen.DefaultExecutor{Path: app.kitchenPath},
			CredResolver:   &nodekitchen.AnalysisCredentialAdapter{Resolver: app.credResolver},
			Logger:         nkLogger,
			TKConfigFn: func() config.TestKitchenConfig {
				if tk, err := app.loadTestKitchenRuntime(); err == nil && tk != nil {
					return *tk
				}
				return app.cfg.AnalysisTools.TestKitchen
			},
			GitCookbookDir: app.cfg.Storage.GitCookbookDir,
			Concurrency:    app.cfg.Concurrency.CookbookDownload,
		}
		routerOpts = append(routerOpts, webapi.WithNodeKitchenRunner(factory))
		app.startup.Info("Node Kitchen runner factory enabled")

		// Wire Git Kitchen scheduler using the same kitchen binary and credentials.
		gitKitchenSched := gitkitchen.NewScheduler(
			&nodekitchen.DefaultExecutor{Path: app.kitchenPath},
			&nodekitchen.AnalysisCredentialAdapter{Resolver: app.credResolver},
			app.db,
			func(name, _ string) string {
				return filepath.Join(app.cfg.Storage.GitCookbookDir, name)
			},
		)
		routerOpts = append(routerOpts, webapi.WithGitKitchenScheduler(gitKitchenSched))
		app.startup.Info("Git Kitchen scheduler enabled")

		// Wire kitchen run queue (bounded concurrency worker pool).
		queueScoped := logger.WithScope(logging.ScopeTestKitchenRun)
		gitExecutor := kitchenqueue.NewGitKitchenExecutor(kitchenqueue.GitKitchenExecutorConfig{
			KitchenExecutor: &nodekitchen.DefaultExecutor{Path: app.kitchenPath},
			CredResolver:    &nodekitchen.AnalysisCredentialAdapter{Resolver: app.credResolver},
			Store:           app.db,
			RepoDirFn: func(name, _ string) string {
				return filepath.Join(app.cfg.Storage.GitCookbookDir, name)
			},
			TKConfigFn: func() config.TestKitchenConfig {
				if tk, err := app.loadTestKitchenRuntime(); err == nil && tk != nil {
					return *tk
				}
				return app.cfg.AnalysisTools.TestKitchen
			},
		})
		app.kitchenQueue = kitchenqueue.New(app.db, gitExecutor,
			kitchenqueue.WithWorkerCount(app.cfg.Concurrency.TestKitchenRun),
			kitchenqueue.WithLogFunc(func(level, msg string, args ...any) {
				formatted := fmt.Sprintf(msg, args...)
				switch level {
				case "WARN":
					queueScoped.Warn(formatted)
				case "ERROR":
					queueScoped.Error(formatted)
				default:
					queueScoped.Info(formatted)
				}
			}),
			kitchenqueue.WithEventListener(func(item *datastore.KitchenQueueItem) {
				app.hub.Broadcast(webapi.NewEvent("kitchen_queue_update", map[string]any{
					"id":            item.ID,
					"status":        item.Status,
					"git_repo_name": item.GitRepoName,
					"run_type":      item.RunType,
					"instance_name": item.InstanceName,
				}))
			}),
		)
		if err := app.kitchenQueue.Start(context.Background()); err != nil {
			app.startup.Error(fmt.Sprintf("kitchen queue start failed: %v", err))
			return serverResult{}, err
		}
		routerOpts = append(routerOpts, webapi.WithKitchenQueue(app.kitchenQueue))
		app.startup.Info(fmt.Sprintf("Kitchen queue started with %d workers", app.cfg.Concurrency.TestKitchenRun))

		// Start periodic cleanup of completed queue items (24h retention).
		queueCleanupLog := func(level, msg string) {
			switch level {
			case "ERROR":
				queueScoped.Error(msg)
			default:
				queueScoped.Info(msg)
			}
		}
		app.stopKitchenQueueCleanup = kitchenqueue.StartCleanupTicker(app.db, 1*time.Hour, 24*time.Hour, queueCleanupLog)
		app.startup.Info("kitchen queue cleanup ticker started (retention: 24h)")
	} else {
		app.startup.Info("Node Kitchen runner not available (kitchen binary not found)")
	}

	if frontendFS := frontend.FS(frontend.DistDir); frontendFS != nil {
		routerOpts = append(routerOpts, webapi.WithFrontendFS(frontendFS))
		if frontend.HasEmbed() {
			app.startup.Info("frontend SPA assets loaded from embedded binary")
		} else {
			app.startup.Info(fmt.Sprintf("frontend SPA assets loaded from disk: %s", frontend.DistDir))
		}
	} else {
		app.startup.Info(fmt.Sprintf("frontend SPA assets not found (checked embedded binary and %s) — serving plain-text placeholder", frontend.DistDir))
	}

	apiRouter := webapi.NewRouter(app.db, app.cfg, app.hub, routerOpts...)
	app.startup.Info("webapi router initialised with all API routes")

	shutdownTimeout := time.Duration(app.cfg.Server.GracefulShutdownSeconds) * time.Second
	if shutdownTimeout <= 0 {
		shutdownTimeout = 15 * time.Second
	}

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

	var res serverResult

	switch app.cfg.Server.TLS.Mode {
	case "static":
		app.startup.Info("TLS mode: static (operator-managed certificate)")

		tlsListener, tlsErr := apptls.NewListener(apiRouter, apptls.ListenerConfig{
			ListenAddress:           app.cfg.Server.ListenAddress,
			Port:                    app.cfg.Server.Port,
			CertPath:                app.cfg.Server.TLS.CertPath,
			KeyPath:                 app.cfg.Server.TLS.KeyPath,
			CAPath:                  app.cfg.Server.TLS.CAPath,
			MinVersion:              app.cfg.Server.TLS.MinVersion,
			HTTPRedirectPort:        app.cfg.Server.TLS.HTTPRedirectPort,
			GracefulShutdownTimeout: shutdownTimeout,
		}, tlsLog)
		if tlsErr != nil {
			app.startup.Error(fmt.Sprintf("TLS listener setup failed: %v", tlsErr))
			return res, tlsErr
		}

		app.startup.Info(fmt.Sprintf("TLS certificate: %s", tlsListener.CertSummary()))
		app.startup.Info(fmt.Sprintf("TLS min version: %s", tlsListener.MinTLSVersionString()))
		if tlsListener.IsMTLSEnabled() {
			app.startup.Info("mutual TLS (mTLS) enabled — client certificates required")
		}

		tlsListener.CertManager().WatchForChanges(30 * time.Second)
		res.errCh = tlsListener.Serve()
		res.tlsListener = tlsListener

	case "acme":
		app.startup.Error("TLS mode 'acme' is not yet implemented")
		return res, fmt.Errorf("TLS mode 'acme' is not yet implemented")

	default:
		listenAddr := app.cfg.Server.ListenAddress
		if listenAddr == "" {
			listenAddr = "0.0.0.0"
		}
		port := app.cfg.Server.Port
		if port == 0 {
			port = 8080
		}

		plainSrv := apptls.NewPlainListener(apiRouter, listenAddr, port)
		plainErrCh := make(chan error, 1)
		go func() {
			app.startup.Info(fmt.Sprintf("HTTP server listening on %s", plainSrv.Addr))
			if err := plainSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				plainErrCh <- err
			}
			close(plainErrCh)
		}()
		res.errCh = plainErrCh
		res.plainSrv = plainSrv
	}

	return res, nil
}

// ---------------------------------------------------------------------------
// Phase: signal handling loop and graceful shutdown.
// ---------------------------------------------------------------------------

func (app *serverApp) awaitShutdown(srv serverResult) int {
	shutdownTimeout := time.Duration(app.cfg.Server.GracefulShutdownSeconds) * time.Second
	if shutdownTimeout <= 0 {
		shutdownTimeout = 15 * time.Second
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	running := true
	for running {
		select {
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGHUP:
				if srv.tlsListener != nil {
					app.startup.Info("received SIGHUP — reloading TLS certificate")
					if reloadErr := srv.tlsListener.CertManager().Reload(); reloadErr != nil {
						app.startup.Error(fmt.Sprintf("TLS certificate reload failed: %v", reloadErr))
					} else {
						app.startup.Info(fmt.Sprintf("TLS certificate reloaded: %s", srv.tlsListener.CertSummary()))
					}
				} else {
					app.startup.Info("received SIGHUP — no TLS certificate to reload in plain HTTP mode")
				}
			default:
				app.startup.Info(fmt.Sprintf("received signal %s, shutting down gracefully...", sig))
				running = false
			}
		case err := <-srv.errCh:
			if err != nil {
				app.startup.Error(fmt.Sprintf("server failed: %v", err))
				return 1
			}
			running = false
		}
	}

	// Graceful shutdown — stop the scheduler first so no new collection
	// runs start, then shut down the HTTP server.
	app.startup.Info("stopping collection scheduler...")
	app.sched.Stop()
	app.startup.Info("collection scheduler stopped")

	// Stop kitchen queue workers (drain running items with timeout).
	if app.kitchenQueue != nil {
		app.startup.Info("stopping kitchen queue workers...")
		app.kitchenQueue.Stop(shutdownTimeout)
		app.startup.Info("kitchen queue stopped")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if srv.tlsListener != nil {
		if err := srv.tlsListener.Shutdown(shutdownCtx); err != nil {
			app.startup.Error(fmt.Sprintf("TLS server shutdown: %v", err))
			return 1
		}
	} else if srv.plainSrv != nil {
		if err := srv.plainSrv.Shutdown(shutdownCtx); err != nil {
			app.startup.Error(fmt.Sprintf("HTTP server shutdown: %v", err))
			return 1
		}
	}

	app.startup.Info("server stopped cleanly")
	return 0
}

// ---------------------------------------------------------------------------
// run orchestrates all startup phases in order.
// ---------------------------------------------------------------------------

func run() int {
	flags := parseCLI()

	if flags.showVersion {
		fmt.Println("chef-migration-metrics", version)
		return 0
	}

	if flags.healthcheck {
		return runHealthcheck(flags.healthcheckURL)
	}

	app := &serverApp{}

	// Phase 1: bootstrap logger.
	app.setupBootstrapLogger()

	// Phase 2: load configuration (full or bootstrap YAML).
	if err := app.loadConfig(flags.configPath); err != nil {
		return 1
	}

	// Phase 3: database connection.
	if err := app.setupDatabase(); err != nil {
		return 1
	}
	defer func() { _ = app.db.Close() }()

	// Phase 4: attach DB log writer.
	app.attachDBWriter()

	// Phase 5: run migrations.
	ctx := context.Background()
	if err := app.runMigrations(ctx, flags.migrationsDir); err != nil {
		return 1
	}

	// Phase 6: authentication.
	if err := app.setupAuth(ctx); err != nil {
		return 1
	}

	// Phase 7: mark interrupted collection runs.
	app.markInterruptedRuns(ctx)

	// Phase 8: secrets and encryption key (mandatory).
	if err := app.setupSecrets(ctx); err != nil {
		return 1
	}
	defer app.encryptor.Close()

	// Phase 9: config store — migrate legacy data, migrate YAML, assemble
	// config from DB, wire credential adapter and config holder.
	if err := app.setupConfigStore(ctx); err != nil {
		return 1
	}

	// Phase 10: sync organisations.
	if err := app.syncOrganisations(ctx); err != nil {
		return 1
	}

	// Phase 11: reconcile stale target version data.
	app.reconcileTargetVersions(ctx)

	// Phase 12: analysis pipeline and collector.
	if err := app.setupCollector(ctx); err != nil {
		return 1
	}

	// Phase 13: ownership startup tasks.
	app.setupOwnership(ctx)

	// Phase 14: collection scheduler.
	if err := app.startScheduler(ctx); err != nil {
		return 1
	}
	defer app.sched.Stop()

	// Phase 15: exports.
	if err := app.setupExports(); err != nil {
		return 1
	}
	defer app.stopExportCleanup()
	defer func() {
		if app.stopKitchenQueueCleanup != nil {
			app.stopKitchenQueueCleanup()
		}
	}()

	// Phase 16: HTTP server.
	srv, err := app.setupAndServeHTTP()
	if err != nil {
		return 1
	}

	// Phase 17: signal handling and graceful shutdown.
	return app.awaitShutdown(srv)
}

// resolveMigrationsDir finds the migrations directory. It checks, in order:
// 1. The exact path given.
// 2. ./migrations relative to the current working directory.
// 3. /usr/share/chef-migration-metrics/migrations (Linux package install).
// Returns "" if no valid directory is found.
func resolveMigrationsDir(hint string) string {
	candidates := []string{hint}
	if hint != "migrations" {
		candidates = append(candidates, "migrations")
	}
	candidates = append(candidates, "/usr/share/chef-migration-metrics/migrations")

	for _, dir := range candidates {
		info, err := os.Stat(dir)
		if err == nil && info.IsDir() {
			abs, absErr := filepath.Abs(dir)
			if absErr == nil {
				return abs
			}
			return dir
		}
	}
	return ""
}

// runHealthcheck performs a health check against a running instance and exits
// with code 0 (healthy) or 1 (unhealthy). When healthcheckURL is empty, it
// defaults to http://localhost:8080/api/v1/health.
func runHealthcheck(healthcheckURL string) int {
	if healthcheckURL == "" {
		healthcheckURL = "http://localhost:8080/api/v1/health"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(healthcheckURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "health check failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Println("healthy")
		return 0
	}

	fmt.Fprintf(os.Stderr, "health check failed: HTTP %d\n", resp.StatusCode)
	return 1
}
