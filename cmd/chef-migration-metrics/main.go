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
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth/jit"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth/samlsp"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/backup"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/collector"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/embedded"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/frontend"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/kitchenqueue"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/nodekitchen"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/perf"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/serverctl"
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

	// Bootstrap listen target (from the bootstrap YAML / env). listen_address
	// and port are normally sourced from the DB (server.listen section); these
	// are retained as the bind-failure fallback so a bad DB-sourced value can
	// never permanently lock out the UI (encrypted-config-store.md).
	bootstrapListenAddr string
	bootstrapPort       int

	// Auth components.
	localAuth      *auth.LocalAuthenticator
	sessionMgr     *auth.SessionManager
	authMiddleware *auth.Middleware
	samlHandler    *webapi.SAMLHandler

	// Secrets components.
	encryptor    *secrets.Encryptor
	credStore    secrets.CredentialStore
	credResolver *secrets.CredentialResolver

	// Collector components.
	coll        *collector.Collector
	sched       *collector.Scheduler
	kitchenPath string // path to kitchen binary, set during setupCollector

	// Export cleanup stop function.
	stopExportCleanup func()

	// Kitchen queue manager (bounded concurrency for TK runs).
	kitchenQueue *kitchenqueue.Manager

	// Backup scheduler (for stopping during restore). backupMu guards
	// backupSched: the config-PUT reconciler and the restore hook both mutate
	// it from separate request goroutines.
	backupMu                sync.Mutex
	backupSched             *backup.Scheduler
	schemaVersion           int
	stopKitchenQueueCleanup func()

	// tlsStatus records whether static TLS failed at startup and the server
	// fell open to plain HTTP (tls.md § 2.4). Shared with the webapi router so
	// the /api/v1/server/tls-status endpoint and UI banner can report it.
	tlsStatus *webapi.TLSStatusHolder

	// tlsReload lets the admin TLS save path swap the running cert_source: db
	// certificate in place (no restart). Populated with the running listener's
	// CertManager once the static listener is built; nil/empty otherwise.
	tlsReload *webapi.TLSReloadHolder

	// listenerRebind lets the admin server save rebind the running listener in
	// place when listen_address/port changes (no restart). Wired into the router
	// up front; populated by setupAndServeHTTP with a serverctl.Controller for
	// plain-HTTP and healthy static-TLS modes. Left unset (so the no-rebinder
	// fallback applies → restart_required) for active auto-443, ACME, and the
	// degraded fallbacks until H4. See configuration-live-reload.md.
	listenerRebind *webapi.ListenerRebindHolder

	// listenerController owns the live HTTP(S) listener for the rebind-capable
	// modes (plain off, static, acme — H4c-1). After a rebind the boot serverResult
	// no longer references the serving listener, so awaitShutdown drains via this
	// controller instead. Nil only on the degraded fail-open paths that did not
	// adopt one.
	listenerController *serverctl.Controller

	// acmeTrigger forwards an admin ACME config save to the running renewer so
	// hostname registration / issuance re-run immediately (tls-acme.md § 3.14).
	// Bound to the renewer once setupACME builds it; a no-op before then.
	acmeTrigger *acmeTriggerHolder

	// restartCh signals an admin-requested graceful restart (POST
	// /api/v1/admin/restart). awaitShutdown selects on it, drains gracefully,
	// and returns a non-zero restart exit code so the supervisor
	// (systemd Restart=on-failure) starts a fresh process that re-reads config.
	// See configuration-live-reload.md § Apply & Restart.
	restartCh chan struct{}

	// auto443Listen binds the standard HTTPS lifeboat port (443) for automatic
	// HTTPS (tls.md § 1.5). It is a field so tests can simulate "443
	// available/unavailable" deterministically without needing privilege. Nil ⇒
	// a real bind on 443.
	auto443Listen func(listenAddr string) (net.Listener, error)

	// autoHTTPSActive records that boot bound the automatic-443 lifeboat (tls.md
	// § 1.5: HTTPS on a separate 443 listener with the configured port redirecting
	// to it). When set, the listener controller re-plans that topology in place on a
	// same-mode static change (H4b-3) rather than collapsing to a single HTTPS
	// listener on the configured port. autoHTTPSPort is the actual bound lifeboat
	// port (443 in production; a free port via the auto443Listen test seam), used as
	// the live HTTPS bind target so a rebind reclaims it. Both are read-only after
	// setupAndServeHTTP.
	autoHTTPSActive bool
	autoHTTPSPort   int

	// acmeActive records that boot came up serving the full acme topology (HTTPS
	// listener + renewer + port-80 challenge/redirect server, adopted into the
	// listener controller as one composite Instance — H4c-1). While set, the applier
	// refuses every server-config save (restart_required) because leaving or
	// re-planning the acme topology in place is deferred to H4c-2. Read-only after
	// setupAndServeHTTP.
	acmeActive bool
}

// exitCodeRestart is the process exit code used for an admin-requested restart.
// It is non-zero so that systemd (Restart=on-failure) restarts the process; a
// clean SIGTERM/systemctl stop still exits 0 and is not restarted.
const exitCodeRestart = 2

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
	// goroutine; it will be stopped during graceful shutdown. Limits come
	// from server.websocket.* and are reconfigured live on save.
	app.hub = webapi.NewEventHub(
		webapi.WithMaxConnections(app.cfg.Server.WebSocket.MaxConnections),
		webapi.WithSendBufferSize(app.cfg.Server.WebSocket.SendBufferSize),
	)
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
		app.schemaVersion = ver
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
		// Live session_expiry: read from the holder at session creation so a
		// config change applies without a restart.
		auth.WithSessionLifetimeFunc(func() time.Duration {
			return auth.ParseDuration(app.configHolder.Get().Auth.SessionExpiry, 8*time.Hour)
		}),
	)

	app.localAuth = auth.NewLocalAuthenticator(app.db, app.cfg.Auth.LockoutAttempts,
		auth.WithLocalAuthLogger(authLogFn),
		auth.WithTrustedProxy(app.cfg.Server.TrustedProxy),
		// Live lockout_attempts: read from the holder on each login attempt.
		auth.WithLockoutAttemptsFunc(func() int {
			return app.configHolder.Get().Auth.LockoutAttempts
		}),
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

// setupSAML configures the SAML Service Provider if a SAML auth provider is
// present in the configuration. Requires setupAuth to have run first.
func (app *serverApp) setupSAML(ctx context.Context) {
	// Always create the handler (provider may be nil) so the SAML routes are
	// wired and a later config change can enable/rebuild SAML live without a
	// restart. With a nil provider the request handlers return 501.
	provider, endpoints, err := app.buildSAMLProvider(ctx, app.cfg)
	if err != nil {
		app.startup.Error(fmt.Sprintf("SAML: %v — SAML login disabled until the config is corrected", err))
	}

	app.samlHandler = webapi.NewSAMLHandler(
		provider,
		jit.New(app.db, app.authScopedLogFn()),
		app.sessionMgr,
		app.db,
		app.cfg.Server.TrustedProxy,
		app.authScopedLogFn(),
	)
	app.samlHandler.SetEndpoints(endpoints)

	if provider != nil {
		app.startup.Info(fmt.Sprintf("SAML SSO configured: entity_id=%s", endpoints.EntityID))
	}
}

// authScopedLogFn returns a level/message log callback scoped to the auth area.
func (app *serverApp) authScopedLogFn() func(level, msg string) {
	authLog := app.logger.WithScope(logging.ScopeAuth)
	return func(level, msg string) {
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
}

// buildSAMLProvider constructs the SAML provider and SP endpoint URLs from cfg.
// Returns (nil, zero, nil) when no SAML provider is configured. The endpoint URLs
// are derived from the same base URL fed to the provider, so they match the
// advertised SP metadata exactly. Reused at boot and on each live auth reload.
func (app *serverApp) buildSAMLProvider(ctx context.Context, cfg *config.Config) (*samlsp.Provider, webapi.SAMLEndpoints, error) {
	var samlCfg *config.AuthProvider
	for i := range cfg.Auth.Providers {
		if cfg.Auth.Providers[i].Type == "saml" {
			samlCfg = &cfg.Auth.Providers[i]
			break
		}
	}
	if samlCfg == nil {
		return nil, webapi.SAMLEndpoints{}, nil
	}

	if app.credResolver == nil {
		return nil, webapi.SAMLEndpoints{}, fmt.Errorf("credential resolver is nil")
	}

	logFn := app.authScopedLogFn()

	certCred, err := app.credResolver.Resolve(ctx, secrets.CredentialSource{
		CredentialName: samlCfg.SPCertificateCredential,
	})
	if err != nil {
		return nil, webapi.SAMLEndpoints{}, fmt.Errorf("resolving SP certificate %q: %w", samlCfg.SPCertificateCredential, err)
	}
	keyCred, err := app.credResolver.Resolve(ctx, secrets.CredentialSource{
		CredentialName: samlCfg.SPPrivateKeyCredential,
	})
	if err != nil {
		return nil, webapi.SAMLEndpoints{}, fmt.Errorf("resolving SP private key %q: %w", samlCfg.SPPrivateKeyCredential, err)
	}

	// Derive the externally-reachable base URL for ACS/SLO/metadata endpoints.
	scheme := "http"
	if cfg.Server.TLS.Mode == "static" || cfg.Server.TLS.Mode == "acme" {
		scheme = "https"
	}
	httpsPort, _ := app.effectiveTLSTopology(cfg.Server)
	baseURL := resolveSPBaseURL(samlCfg.SPBaseURL, samlCfg.SPEntityID, scheme, httpsPort)

	spCfg := samlsp.Config{
		IDPMetadataURL:    samlCfg.IDPMetadataURL,
		IDPMetadataPath:   samlCfg.IDPMetadataPath,
		IDPMetadataXML:    []byte(samlCfg.IDPMetadataXML),
		SPEntityID:        samlCfg.SPEntityID,
		ACSURL:            baseURL + "/api/v1/auth/saml/acs",
		SLOURL:            baseURL + "/api/v1/auth/saml/slo",
		MetadataURL:       baseURL + "/api/v1/auth/saml/metadata",
		Certificate:       certCred.Plaintext,
		PrivateKey:        keyCred.Plaintext,
		UsernameAttr:      samlCfg.UsernameAttr,
		EmailAttr:         samlCfg.EmailAttr,
		DisplayNameAttr:   samlCfg.DisplayNameAttr,
		GroupsAttr:        samlCfg.GroupsAttr,
		RoleAttr:          samlCfg.RoleAttr,
		RoleMapping:       samlCfg.RoleMapping,
		AllowIDPInitiated: samlCfg.AllowIDPInitiated,
		SignRequests:      samlCfg.SignRequests,
		Logger:            logFn,
	}

	logFn("INFO", fmt.Sprintf("SAML config: groups_attr=%q role_attr=%q role_mapping=%v",
		samlCfg.GroupsAttr, samlCfg.RoleAttr, samlCfg.RoleMapping))

	provider, err := samlsp.New(spCfg)
	if err != nil {
		return nil, webapi.SAMLEndpoints{}, fmt.Errorf("creating provider: %w", err)
	}

	return provider, webapi.SAMLEndpoints{
		ACSURL:      spCfg.ACSURL,
		SLOURL:      spCfg.SLOURL,
		MetadataURL: spCfg.MetadataURL,
		EntityID:    spCfg.SPEntityID,
	}, nil
}

// resolveSPBaseURL determines the externally-reachable base URL the SP metadata
// advertises (the ACS/SLO/metadata Locations). Precedence:
//  1. an explicit sp_base_url (canonical; the admin UI defaults it to the
//     browser origin so it reflects how users actually reach the server),
//  2. the host of an http(s) sp_entity_id,
//  3. a scheme://localhost:<httpsPort> fallback (standard ports omitted).
//
// httpsPort must be the effective HTTPS listen port (443 when automatic HTTPS is
// active), never the http-redirect port — that is what made the old hardcoded
// "https://localhost:8080" both unreachable and pointed at the wrong service.
func resolveSPBaseURL(spBaseURL, spEntityID, scheme string, httpsPort int) string {
	if spBaseURL != "" {
		return strings.TrimRight(spBaseURL, "/")
	}
	if strings.HasPrefix(spEntityID, "http") {
		if u, err := url.Parse(spEntityID); err == nil && u.Host != "" {
			return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
		}
	}
	if (scheme == "https" && httpsPort == 443) || (scheme == "http" && httpsPort == 80) {
		return scheme + "://localhost"
	}
	return fmt.Sprintf("%s://localhost:%d", scheme, httpsPort)
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

	// Warn on overly permissive key file permissions.
	app.checkKeyFilePermissions(secretsLog)

	return nil
}

// ---------------------------------------------------------------------------
// Phase: config store migration and assembly.
// ---------------------------------------------------------------------------

func (app *serverApp) setupConfigStore(ctx context.Context) error {
	csLog := app.logger.WithScope(logging.ScopeStartup)

	// Capture the bootstrap listen target (from YAML/env) before any DB
	// assembly overwrites app.cfg. This is the bind-failure fallback.
	app.bootstrapListenAddr = app.cfg.Server.ListenAddress
	app.bootstrapPort = app.cfg.Server.Port

	// Run legacy data migration (runtime_settings → config_store).
	migrateResult, err := configstore.MigrateFromLegacy(ctx, app.db.Pool(), app.cfgStore)
	if err != nil {
		csLog.Error(fmt.Sprintf("legacy data migration failed: %v", err))
		return err
	}
	if migrateResult.Skipped {
		csLog.Info(fmt.Sprintf("legacy data migration skipped: %s", migrateResult.SkipReason))
	} else {
		csLog.Info(fmt.Sprintf("legacy data migration complete: %d runtime setting(s) migrated",
			migrateResult.RuntimeSettingsMigrated))
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

		// Carry over the bootstrap database URL (never stored in the DB).
		assembled.Datastore.URL = app.cfg.Datastore.URL

		// listen_address/port are DB-managed (server.listen section). When the
		// DB has them, AssembleConfig already populated `assembled` and the DB
		// value wins. Only carry over the bootstrap value when the section is
		// absent, so an existing deployment keeps its YAML listen target until
		// it is edited in the UI.
		hasListen, hlErr := configstore.HasKey(ctx, app.cfgStore, configstore.KeyServerListen)
		if hlErr != nil {
			csLog.Error(fmt.Sprintf("checking %s: %v", configstore.KeyServerListen, hlErr))
			return hlErr
		}
		if !hasListen {
			assembled.Server.ListenAddress = app.bootstrapListenAddr
			assembled.Server.Port = app.bootstrapPort
		}

		app.cfg = assembled
		csLog.Info("configuration assembled from database")
	} else {
		csLog.Info("config_store is empty — using YAML configuration")
	}

	// Wire up the CredentialStoreAdapter backed by config_store.
	app.credStore = configstore.NewCredentialStoreAdapter(app.cfgStore, &dbRefChecker{db: app.db})
	app.credResolver = secrets.NewCredentialResolver(app.credStore)
	csLog.Info("credential store adapter configured (backed by config_store)")

	// Create the ConfigHolder for concurrent-safe config access.
	app.configHolder = configstore.NewConfigHolder(app.cfg, app.cfgStore)

	return nil
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
	// Read from live config so this is correct both at boot and when re-run
	// after an admin org-config change (the holder is reloaded on PUT). Falls
	// back to the boot snapshot if the holder is not yet wired.
	liveOrgs := app.cfg.Organisations
	if app.configHolder != nil {
		liveOrgs = app.configHolder.Get().Organisations
	}
	orgParams := make([]datastore.UpsertOrganisationParams, 0, len(liveOrgs))
	for _, org := range liveOrgs {
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
}

// ---------------------------------------------------------------------------
// Phase: analysis pipeline and collector setup.
// ---------------------------------------------------------------------------

func (app *serverApp) setupCollector(ctx context.Context) error {
	toolResolver := embedded.NewResolver()
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
			analysis.WithCookstyleConcurrencyFunc(func() int {
				return app.configHolder.Get().Concurrency.CookstyleScan
			}),
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

	kitchenAnalyser := analysis.NewKitchenAnalyser(app.db, app.logger, 0)
	collOpts = append(collOpts, collector.WithKitchenAnalyser(kitchenAnalyser))
	app.startup.Info("kitchen config analyser enabled")

	cxScorer := remediation.NewComplexityScorer(app.db, app.logger)
	collOpts = append(collOpts, collector.WithComplexityScorer(cxScorer))

	readinessEval := analysis.NewReadinessEvaluatorFromConfig(
		app.db, app.logger,
		app.cfg.Concurrency.ReadinessEvaluation,
		analysis.ReadinessEvalConfig{
			InstallPathLinux:        app.cfg.Readiness.InstallPathLinux,
			InstallPathWindows:      app.cfg.Readiness.InstallPathWindows,
			InstallSizeMBLinux:      app.cfg.Readiness.InstallSizeMBLinux,
			InstallSizeMBWindows:    app.cfg.Readiness.InstallSizeMBWindows,
			MinRemainingFreePercent: app.cfg.Readiness.MinRemainingFreePercent,
		},
		analysis.WithConfigFunc(func() analysis.ReadinessEvalConfig {
			cfg := app.configHolder.Get()
			return analysis.ReadinessEvalConfig{
				InstallPathLinux:        cfg.Readiness.InstallPathLinux,
				InstallPathWindows:      cfg.Readiness.InstallPathWindows,
				InstallSizeMBLinux:      cfg.Readiness.InstallSizeMBLinux,
				InstallSizeMBWindows:    cfg.Readiness.InstallSizeMBWindows,
				MinRemainingFreePercent: cfg.Readiness.MinRemainingFreePercent,
			}
		}),
		analysis.WithReadinessConcurrencyFunc(func() int {
			return app.configHolder.Get().Concurrency.ReadinessEvaluation
		}),
	)
	collOpts = append(collOpts, collector.WithReadinessEvaluator(readinessEval))

	ownershipEval := collector.NewOwnershipEvaluator(app.db, app.cfg.Ownership, app.logger)
	collOpts = append(collOpts, collector.WithOwnershipEvaluator(ownershipEval))

	// Ensure storage directories exist.
	if err := app.ensureStorageDirs(); err != nil {
		return err
	}

	// Cookbook directory resolvers.
	collOpts = append(collOpts, app.cookbookDirOpts()...)

	// Live config: collector picks up admin UI changes each run.
	collOpts = append(collOpts, collector.WithConfigFn(app.configHolder.Get))

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
	// Resume interrupted runs asynchronously so the HTTP server can start
	// immediately — a large org (50k+ nodes) can take minutes to resume.
	go func() {
		log := app.logger.WithScope(logging.ScopeCollectionRun)
		resumeResult, resumeErr := app.coll.ResumeInterruptedRuns(ctx)
		if resumeErr != nil {
			_ = log.Warn(fmt.Sprintf("failed to resume interrupted collection runs: %v", resumeErr))
		} else if resumeResult != nil && resumeResult.Evaluated > 0 {
			_ = log.Info(fmt.Sprintf(
				"interrupted run evaluation: %d evaluated, %d resumed, %d abandoned",
				resumeResult.Evaluated, resumeResult.Resumed, resumeResult.Abandoned,
			))
			if resumeResult.ResumedRunResult != nil {
				rr := resumeResult.ResumedRunResult
				_ = log.Info(fmt.Sprintf(
					"resumed collection completed: %d/%d orgs succeeded, %d nodes, %d cookbook versions in %s",
					rr.SucceededOrgs, rr.TotalOrgs, rr.TotalNodes, rr.TotalCookbooks,
					rr.Duration.Round(time.Millisecond),
				))
			}
			for runID, runErr := range resumeResult.Errors {
				_ = log.Warn(fmt.Sprintf("resume error for run %s: %v", runID, runErr))
			}
		}
	}()

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
	app.stopExportCleanup = export.StartCleanupTicker(app.db, 1*time.Hour, exportCleanupLog)
	app.startup.Info("export cleanup ticker started (interval: 1h)")
	return nil
}

// ---------------------------------------------------------------------------
// Phase: HTTP server setup and serve.
// ---------------------------------------------------------------------------

type serverResult struct {
	errCh       <-chan error
	tlsListener *apptls.Listener
	plainSrv    *http.Server

	// challengeSrv is the ACME http-01 challenge/redirect server bound to the
	// redirect port (port 80) in mode: acme. Nil in all other modes.
	challengeSrv *http.Server

	// renewerCancel stops the background ACME renewal loop in mode: acme. Nil in
	// all other modes.
	renewerCancel context.CancelFunc
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
		webapi.WithSchemaVersion(app.schemaVersion),
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
		webapi.WithLogLevelSetter(func(level string) error {
			sev, err := logging.ParseSeverity(level)
			if err != nil {
				return err
			}
			logger.SetLevel(sev)
			return nil
		}),
		webapi.WithCollectionRescheduler(func(schedule string) error {
			parsed, err := collector.ParseSchedule(schedule)
			if err != nil {
				return err
			}
			sched.Reschedule(parsed)
			return nil
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
		// After an organisations config save, reconcile the operational org
		// table from live config and trigger a collection — so a newly added
		// org takes effect without a restart (configuration-live-reload.md).
		webapi.WithOrganisationsChanged(func(ctx context.Context) error {
			if err := app.syncOrganisations(ctx); err != nil {
				return err
			}
			// Best-effort, non-blocking. Background context so it outlives the
			// PUT request; skipped if a run is already in progress.
			if sched != nil && coll != nil && !coll.IsRunning() {
				go func() {
					triggerLog := logger.WithScope(logging.ScopeCollectionRun)
					triggerLog.Info("organisations changed — triggering collection")
					if _, err := sched.TriggerNow(context.Background()); err != nil {
						triggerLog.Error(fmt.Sprintf("post-org-change collection failed: %v", err))
					}
				}()
			}
			return nil
		}),
	}

	// Holder for degraded-TLS state. Wired into the router up front so that a
	// later static-TLS fallback (see the switch below) can flip it and have the
	// status endpoint + UI banner report INSECURE without restarting.
	app.tlsStatus = webapi.NewTLSStatusHolder()
	routerOpts = append(routerOpts, webapi.WithTLSStatus(app.tlsStatus))

	// Holder for the running listener's in-place cert reloader. Wired up front
	// so the admin TLS save path can swap a cert_source: db certificate without
	// a restart once the static listener (below) populates it.
	app.tlsReload = webapi.NewTLSReloadHolder()
	routerOpts = append(routerOpts, webapi.WithTLSReload(app.tlsReload))
	// Promoting a real certificate in place over a degraded self-signed listener
	// (an admin save, or ACME issuance) must clear the degraded banner and resume
	// HSTS without a restart (tls.md § 6.3).
	app.tlsReload.SetOnReload(app.tlsStatus.SetHealthy)

	// Holder for the ACME renewer's immediate re-assert trigger. Wired up front
	// like tlsReload; setupACME binds it to the renewer once built (tls-acme.md
	// § 3.14). A no-op in non-ACME modes or before binding.
	app.acmeTrigger = &acmeTriggerHolder{}
	routerOpts = append(routerOpts, webapi.WithACMETrigger(app.acmeTrigger.Trigger))

	// Holder for the in-place listener rebinder. Wired up front like tlsReload;
	// the serve switch below adopts a serverctl.Controller into it for the modes
	// that support live listen_address/port rebind (configuration-live-reload.md
	// listener-rebind H2). Unset modes fall back to restart-required.
	app.listenerRebind = webapi.NewListenerRebindHolder()
	routerOpts = append(routerOpts, webapi.WithListenerRebinder(app.listenerRebind))

	if recorder != nil {
		routerOpts = append(routerOpts, webapi.WithPerformance(recorder))
	}

	if app.credStore != nil {
		routerOpts = append(routerOpts, webapi.WithCredentialStore(app.credStore))
	}

	if app.cfgStore != nil && app.configHolder != nil {
		routerOpts = append(routerOpts, webapi.WithConfigStore(app.cfgStore, app.configHolder))
	}

	// Wire credential resolver so the router can build hypervisor clients
	// on demand from live config — no restart needed after config changes.
	if app.credResolver != nil {
		routerOpts = append(routerOpts, webapi.WithCredentialResolver(app.credResolver))
	}

	// The SAML handler is always created (provider may be nil); wiring it makes
	// the SAML routes live so a config change can enable/rebuild SAML without a
	// restart. The reconciler rebuilds the provider from the reloaded config and
	// swaps it in (the subsystem half of the auth applier).
	if app.samlHandler != nil {
		routerOpts = append(routerOpts, webapi.WithSAML(app.samlHandler))
		routerOpts = append(routerOpts, webapi.WithSAMLReconciler(func() error {
			rctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			provider, endpoints, err := app.buildSAMLProvider(rctx, app.configHolder.Get())
			if err != nil {
				return err
			}
			// Swap together: nil provider + zero endpoints when SAML was removed.
			app.samlHandler.SetProvider(provider)
			app.samlHandler.SetEndpoints(endpoints)
			return nil
		}))
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
			Executor:     &nodekitchen.DefaultExecutor{Path: app.kitchenPath},
			CredResolver: &nodekitchen.AnalysisCredentialAdapter{Resolver: app.credResolver},
			Logger:       nkLogger,
			TKConfigFn: func() config.TestKitchenConfig {
				return app.configHolder.Get().AnalysisTools.TestKitchen
			},
			GitCookbookDir: app.cfg.Storage.GitCookbookDir,
			ConcurrencyFn: func() int {
				return app.configHolder.Get().Concurrency.CookbookDownload
			},
		}
		routerOpts = append(routerOpts, webapi.WithNodeKitchenRunner(factory))
		app.startup.Info("Node Kitchen runner factory enabled")

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
				return app.configHolder.Get().AnalysisTools.TestKitchen
			},
		})
		// Global VM start-rate limiter. Reads window/max live from config on
		// every wait, so on-site tuning takes effect with no restart. Disabled
		// (pass-through) until both values are configured.
		startLimiter := kitchenqueue.NewRateLimiter(func() (time.Duration, int) {
			window, max, enabled := app.configHolder.Get().AnalysisTools.TestKitchen.StartRateLimit()
			if !enabled {
				return 0, 0
			}
			return window, max
		})
		app.kitchenQueue = kitchenqueue.New(app.db, gitExecutor,
			kitchenqueue.WithWorkerCount(app.cfg.AnalysisTools.TestKitchen.EffectiveMaxConcurrentVMs()),
			kitchenqueue.WithRateLimiter(startLimiter),
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
		app.startup.Info(fmt.Sprintf("Kitchen queue started with %d workers", app.cfg.AnalysisTools.TestKitchen.EffectiveMaxConcurrentVMs()))

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

	// --- Backup service (always available — "enabled" flag controls scheduler only) ---
	{
		backupConn, parseErr := backup.ParseConnString(app.cfg.Datastore.URL)
		if parseErr != nil {
			app.startup.Error(fmt.Sprintf("backup: failed to parse DB URL: %v", parseErr))
		} else {
			pgTools, toolsErr := backup.NewPgTools(app.cfg.Backup.PgDumpPath, app.cfg.Backup.PgRestorePath)
			if toolsErr != nil {
				app.startup.Warn(fmt.Sprintf("backup: pg_dump/pg_restore not available: %v", toolsErr))
			} else {
				backupDir := app.cfg.BackupDir()
				maxGen := app.cfg.BackupMaxGenerations()
				svc, svcErr := backup.NewService(backupDir, backupConn, pgTools, version, app.schemaVersion, maxGen)
				if svcErr != nil {
					app.startup.Error(fmt.Sprintf("backup: failed to create service: %v", svcErr))
				} else {
					backupLogger := logger.WithScope(logging.ScopeBackup)
					svc.SetLogFunc(func(level, msg string) {
						switch level {
						case "error":
							backupLogger.Error(msg)
						case "warn":
							backupLogger.Warn(msg)
						default:
							backupLogger.Info(msg)
						}
					})
					routerOpts = append(routerOpts, webapi.WithBackupService(svc))
					routerOpts = append(routerOpts, webapi.WithRestoreHook(func() {
						app.startup.Info("restore: stopping collection scheduler")
						app.sched.Stop()
						if app.kitchenQueue != nil {
							app.startup.Info("restore: stopping kitchen queue workers")
							app.kitchenQueue.Stop(15 * time.Second)
						}
						app.backupMu.Lock()
						if app.backupSched != nil {
							app.startup.Info("restore: stopping backup scheduler")
							app.backupSched.Stop()
							app.backupSched = nil
						}
						app.backupMu.Unlock()
						if app.stopExportCleanup != nil {
							app.startup.Info("restore: stopping export cleanup")
							app.stopExportCleanup()
						}
						app.startup.Info("restore: all background workers stopped")
					}))
					app.startup.Info(fmt.Sprintf("backup service ready (dir=%s, max_generations=%d)", backupDir, maxGen))

					backupLog := func(level, msg string) {
						switch level {
						case "error":
							backupLogger.Error(msg)
						default:
							backupLogger.Info(msg)
						}
					}

					// Boot-time start when enabled. Live enable/disable/reschedule
					// is handled by the reconciler wired below.
					if app.cfg.Backup.Enabled {
						cronExpr := app.cfg.BackupSchedule()
						sched, schedErr := backup.NewScheduler(svc, cronExpr, backupLog)
						if schedErr != nil {
							app.startup.Error(fmt.Sprintf("backup: invalid schedule: %v", schedErr))
						} else {
							sched.Start(context.Background())
							app.backupSched = sched
							app.startup.Info(fmt.Sprintf("backup scheduler started (schedule=%q)", cronExpr))
						}
					}

					// Reconcile the running backup scheduler to the live config on
					// each backup PUT (the backup.{enabled,schedule} subsystem
					// applier): start when newly enabled, stop when disabled,
					// reschedule in place on a schedule change. Reads the reloaded
					// holder, so the schedule default lives only in BackupSchedule().
					routerOpts = append(routerOpts, webapi.WithBackupReconciler(func() error {
						cfg := app.configHolder.Get()
						enabled := cfg.Backup.Enabled
						cronExpr := cfg.BackupSchedule()

						app.backupMu.Lock()
						defer app.backupMu.Unlock()

						switch {
						case enabled && app.backupSched == nil:
							sched, err := backup.NewScheduler(svc, cronExpr, backupLog)
							if err != nil {
								return fmt.Errorf("backup: invalid schedule: %w", err)
							}
							sched.Start(context.Background())
							app.backupSched = sched
							app.startup.Info(fmt.Sprintf("backup scheduler started (schedule=%q)", cronExpr))
						case enabled && app.backupSched != nil:
							parsed, err := collector.ParseSchedule(cronExpr)
							if err != nil {
								return fmt.Errorf("backup: invalid schedule: %w", err)
							}
							app.backupSched.Reschedule(parsed)
							app.startup.Info(fmt.Sprintf("backup scheduler rescheduled (schedule=%q)", cronExpr))
						case !enabled && app.backupSched != nil:
							app.backupSched.Stop()
							app.backupSched = nil
							app.startup.Info("backup scheduler stopped (backup disabled)")
						}
						return nil
					}))
				}
			}
		}
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

	// Wire the admin-requested restart trigger. The closure is non-blocking:
	// it signals awaitShutdown, which drains gracefully and exits with the
	// restart code so the supervisor starts a fresh process.
	app.restartCh = make(chan struct{}, 1)
	routerOpts = append(routerOpts, webapi.WithRestartTrigger(func() {
		select {
		case app.restartCh <- struct{}{}:
		default: // a restart is already pending — ignore duplicate requests
		}
	}))

	apiRouter := webapi.NewRouter(app.db, app.cfg, app.hub, routerOpts...)
	app.startup.Info("webapi router initialised with all API routes")

	shutdownTimeout := time.Duration(app.cfg.Server.GracefulShutdownSeconds) * time.Second
	if shutdownTimeout <= 0 {
		shutdownTimeout = 15 * time.Second
	}

	tlsLog := app.tlsLog

	var res serverResult

	switch app.cfg.Server.TLS.Mode {
	case "static":
		app.startup.Info("TLS mode: static (operator-managed certificate)")

		lcfg := apptls.ListenerConfig{
			ListenAddress:           app.cfg.Server.ListenAddress,
			Port:                    app.cfg.Server.Port,
			CertSource:              app.cfg.Server.TLS.CertSource,
			CertPath:                app.cfg.Server.TLS.CertPath,
			KeyPath:                 app.cfg.Server.TLS.KeyPath,
			CAPath:                  app.cfg.Server.TLS.CAPath,
			MinVersion:              app.cfg.Server.TLS.MinVersion,
			HTTPRedirectPort:        app.cfg.Server.TLS.HTTPRedirectPort,
			GracefulShutdownTimeout: shutdownTimeout,
			TrustedProxy:            app.cfg.Server.TrustedProxy,
			HSTSEnabled:             app.hstsEnabledFn(),
		}

		if app.cfg.Server.TLS.CertSource == "db" {
			app.startup.Info("TLS certificate source: db (encrypted config store)")
			certPEM, keyPEM, loadErr := app.loadDBCertKey(context.Background())
			if loadErr != nil {
				// Fail open (tls-static.md § 2.4): a missing or unreadable DB
				// certificate falls open to a self-signed HTTPS listener exactly
				// like a missing file, so it can never lock the operator out.
				return app.degradeToSelfSigned(apiRouter, nil, loadErr), nil
			}
			lcfg.CertPEM = certPEM
			lcfg.KeyPEM = keyPEM
		}

		// Automatic HTTPS on 443 (tls.md § 1.5): only when TLS is healthy at
		// startup. The configured port is redirected to 443; on a 443 bind failure
		// we fall back to serving HTTPS on the configured port with no redirect.
		var https443Ln net.Listener
		if app.tlsHealthy(lcfg) {
			lcfg.Port, lcfg.RedirectPorts, https443Ln = app.planAutoHTTPS(app.cfg.Server.TLS.HTTPRedirectPort)
			lcfg.HTTPRedirectPort = 0 // folded into RedirectPorts by planAutoHTTPS
		}

		tlsListener, tlsErr := apptls.NewListener(apiRouter, lcfg, tlsLog)
		if tlsErr != nil {
			// Fail open (tls-static.md § 2.4): a bad certificate must never
			// prevent reaching the UI to fix it. Record degraded state and serve
			// a self-signed cert (encrypted) instead of exiting.
			if https443Ln != nil {
				_ = https443Ln.Close()
			}
			return app.degradeToSelfSigned(apiRouter, nil, tlsErr), nil
		}
		if https443Ln != nil {
			tlsListener.SetHTTPSListener(https443Ln)
		}

		app.startup.Info(fmt.Sprintf("TLS certificate: %s", tlsListener.CertSummary()))
		app.startup.Info(fmt.Sprintf("TLS min version: %s", tlsListener.MinTLSVersionString()))
		if tlsListener.IsMTLSEnabled() {
			app.startup.Info("mutual TLS (mTLS) enabled — client certificates required")
		}

		// File source: poll for on-disk changes (no-op for the db source).
		// DB source: register the CertManager so the admin save path can swap
		// the certificate in place on a config change (tls-static.md § 2.3).
		tlsListener.CertManager().WatchForChanges(30 * time.Second)
		if app.tlsReload != nil {
			app.tlsReload.Set(tlsListener.CertManager())
		}
		res.errCh = tlsListener.Serve()
		res.tlsListener = tlsListener

		// Adopt the listener controller so a saved listen_address/port, an
		// off↔static mode toggle (H4a), a same-mode static field change (min_version
		// / mTLS CA / cert source-or-paths — H4b-1), or an http_redirect_port change
		// (H4b-2) rebinds in place (no restart). When the auto-443 lifeboat is active
		// (https443Ln != nil) HTTPS serves on 443 with the configured port as a
		// redirect; record that topology so the controller re-plans 443 + redirects in
		// place on a same-mode static change (H4b-3) instead of collapsing to a single
		// HTTPS listener on the configured port. autoHTTPSPort is the actual bound
		// lifeboat port (443 in production), captured so a live rebind reclaims it.
		if https443Ln != nil {
			app.autoHTTPSActive = true
			app.autoHTTPSPort = listenerPort(https443Ln)
		}
		app.adoptListenerController(apiRouter,
			&serverctl.Instance{Addr: tlsListener.Addr(), Shutdown: tlsListener.Shutdown},
			app.cfg.Server)

	case "acme":
		return app.setupACME(apiRouter, app.cfgStore, shutdownTimeout)

	default:
		res = app.servePlainHTTP(apiRouter)
		if res.plainSrv != nil {
			app.adoptListenerController(apiRouter,
				&serverctl.Instance{Addr: res.plainSrv.Addr, Shutdown: res.plainSrv.Shutdown},
				app.cfg.Server)
		}
	}

	return res, nil
}

// rebindLog adapts the application logger to the serverctl.LogFunc seam, scoping
// messages to the startup scope.
func (app *serverApp) rebindLog(level, msg string) {
	scoped := app.logger.WithScope(logging.ScopeStartup)
	switch level {
	case "ERROR":
		scoped.Error(msg)
	default:
		scoped.Info(msg)
	}
}

// serverListenerVariant maps a TLS mode to the listener topology the controller
// builds for it: static → a TLS listener, acme → the acme topology (adopted as a
// composite Instance, H4c-1), anything else (off/"") → plain HTTP. A distinct acme
// variant keeps the adopted acme key from colliding with off; in-place acme
// transitions remain refused until H4c-2 (so the exact acme key beyond the variant
// does not yet drive any rebind — H4c-2 will fold the effective HTTPS port and an
// acme fingerprint into it).
func serverListenerVariant(mode string) string {
	switch mode {
	case "static":
		return "tls"
	case "acme":
		return "acme"
	default:
		return "plain"
	}
}

// resolveListen returns the effective bind address and port for cfg, applying the
// same defaults the listeners use (empty address → 0.0.0.0, zero port → 8080) so
// the controller's no-op key is stable across an unchanged save.
func resolveListen(cfg config.ServerConfig) (string, int) {
	addr := cfg.ListenAddress
	if addr == "" {
		addr = "0.0.0.0"
	}
	port := cfg.Port
	if port == 0 {
		port = 8080
	}
	return addr, port
}

// tlsTopologyFingerprint captures the static-TLS listener-affecting config fields
// — cert source/paths, mTLS CA, min version, and http_redirect_port — so a
// same-port change to any of them yields a different controller key and rebinds the
// HTTPS (+ redirect) listener topology in place (H4b-1 fields, H4b-2 redirect). It
// is empty for non-static modes: plain HTTP has no TLS topology, and the acme /
// auto-443 topologies are not rebound in place yet (acme refused by
// applyServerListener before the key is computed). cert_source "" normalises to
// "file" to match the listener default, so a cosmetic round-trip of the default is
// not seen as a change.
func tlsTopologyFingerprint(cfg config.ServerConfig) string {
	if cfg.TLS.Mode != "static" {
		return ""
	}
	src := cfg.TLS.CertSource
	if src == "" {
		src = "file"
	}
	return fmt.Sprintf("src=%s;cert=%s;key=%s;ca=%s;min=%s;redirect=%d",
		src, cfg.TLS.CertPath, cfg.TLS.KeyPath, cfg.TLS.CAPath, cfg.TLS.MinVersion, cfg.TLS.HTTPRedirectPort)
}

// serverListenerKey is the opaque controller target fingerprint for cfg:
// "<variant>|<addr:port>|<tls-topology>". off↔static on the same port yields a
// different key (different variant), a listen change yields a different bind
// target, and a same-mode static field change (min_version / mTLS CA / cert
// source-or-paths) yields a different tls-topology segment — each triggers a
// rebuild. An unchanged save yields the same key and is a no-op.
func serverListenerKey(cfg config.ServerConfig) string {
	addr, port := resolveListen(cfg)
	return fmt.Sprintf("%s|%s:%d|%s", serverListenerVariant(cfg.TLS.Mode), addr, port, tlsTopologyFingerprint(cfg))
}

// listenerKey is the controller target fingerprint for cfg, accounting for the
// auto-443 lifeboat. When the lifeboat is active and the target is static, HTTPS
// lives on the lifeboat port (autoHTTPSPort) and the configured port is only a
// redirect, so the key's listen target is "<addr>:<autoHTTPSPort>" and the
// configured port is folded into the fingerprint — a server.port /
// http_redirect_port / min_version / CA / cert change yields a new key while the
// HTTPS target stays the same (→ a same-target RebindInPlace that swaps the
// redirects). Otherwise it is the standard single-listener key.
func (app *serverApp) listenerKey(cfg config.ServerConfig) string {
	if app.autoHTTPSActive && cfg.TLS.Mode == "static" {
		addr, port := resolveListen(cfg)
		return fmt.Sprintf("tls443|%s:%d|%s;cfgport=%d",
			addr, app.autoHTTPSPort, tlsTopologyFingerprint(cfg), port)
	}
	return serverListenerKey(cfg)
}

// effectiveTLSTopology returns the HTTPS port and redirect ports a static-TLS
// listener should bind for cfg. With the auto-443 lifeboat active, HTTPS serves on
// the lifeboat port and both the configured port and http_redirect_port redirect to
// it; otherwise HTTPS serves on the configured port with http_redirect_port as the
// only redirect. Zero/duplicate/HTTPS-equal redirect ports are filtered by the
// listener.
func (app *serverApp) effectiveTLSTopology(cfg config.ServerConfig) (httpsPort int, redirectPorts []int) {
	_, port := resolveListen(cfg)
	if app.autoHTTPSActive {
		return app.autoHTTPSPort, []int{port, cfg.TLS.HTTPRedirectPort}
	}
	return port, []int{cfg.TLS.HTTPRedirectPort}
}

// listenerPort extracts the bound TCP port from a listener's address. Used to
// capture the actual auto-443 lifeboat port (443 in production, a free port under
// the auto443Listen test seam) so a live rebind reclaims that exact port.
func listenerPort(ln net.Listener) int {
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return 0
	}
	p, _ := strconv.Atoi(portStr)
	return p
}

// keyListenTarget returns the address:port segment of a serverListenerKey (between
// the first and second "|"), so two keys can be compared for the same bind target
// regardless of variant or tls topology. A same target means bind-new-first is
// impossible (the old listener holds the port) and the rebind must release-first.
func keyListenTarget(key string) string {
	parts := strings.SplitN(key, "|", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return key
}

// adoptListenerController wires the single serverctl.Controller that rebinds the
// live listener in place across listen_address/port changes and off↔static TLS
// mode transitions (H4a). boot is the Instance already serving at startup;
// bootCfg seeds the initial no-op key from the configured (not the bound) target,
// so an unchanged save is a no-op. handler is rebuilt onto the new listener on
// each rebind. No-op when no boot Instance or no rebind holder is wired.
func (app *serverApp) adoptListenerController(handler http.Handler, boot *serverctl.Instance, bootCfg config.ServerConfig) {
	if boot == nil || app.listenerRebind == nil {
		return
	}
	ctrl := serverctl.New(app.resolveShutdownTimeout, app.rebindLog)
	ctrl.Adopt(boot, app.listenerKey(bootCfg))
	app.listenerController = ctrl
	app.listenerRebind.Set(func(cfg config.ServerConfig) (webapi.ReloadGranularity, error) {
		return app.applyServerListener(handler, ctrl, cfg)
	})
}

// inPlaceBindAttempts / inPlaceBindRetryDelay bound the bind retry on a
// same-target (off↔static, same port) rebind, where the new listener must reclaim
// the port the old one just released. SO_REUSEADDR (Go's default) makes the
// reclaim succeed; the retry only absorbs the brief OS release lag. ~2s total.
const (
	inPlaceBindAttempts   = 40
	inPlaceBindRetryDelay = 50 * time.Millisecond
)

// applyServerListener rebuilds the live listener for the desired cfg via the
// controller, dispatching on the target TLS mode: off → plain, static → a
// static-TLS HTTPS listener on the configured port plus an optional
// http_redirect_port redirect listener (H4b-2). The only target not yet supported
// in place is ACME (the port-80 challenge/redirect + renewer rebuild is H4c) —
// returning ErrNoListenerRebinder so the save is reported restart_required and
// applied on the next restart. When the auto-443 lifeboat is active (H4b-3), a
// same-mode static change re-plans HTTPS-on-the-lifeboat-port + its redirects in
// place via effectiveTLSTopology; leaving that topology (a mode or listen_address
// change) is refused and stays restart-required.
//
// A different-target change binds the new listener before retiring the old
// (bind-new-first), so a bind clash keeps the old serving. A same-address:port
// change (an off↔static toggle on one port, or a same-mode static field change —
// min_version / mTLS CA / cert source-or-paths — that keeps the port) cannot
// bind-new-first, so it validates the replacement is constructible first — a bad
// certificate is caught with the old listener untouched — then releases the old
// and binds the new on the freed port, retrying briefly to absorb the OS release
// lag.
func (app *serverApp) applyServerListener(handler http.Handler, ctrl *serverctl.Controller, cfg config.ServerConfig) (webapi.ReloadGranularity, error) {
	// Entering acme (target mode), or re-planning the live acme topology in place
	// (an acme-internal save while app.acmeActive), is deferred to H4c-2b — refuse
	// so the save is reported restart_required. Leaving acme for off/static (H4c-2a)
	// is handled below: the controller swap drains the whole acme Instance (HTTPS +
	// renewer + port-80 challenge) and binds the off/static replacement.
	if cfg.TLS.Mode == "acme" {
		return webapi.ReloadProcess, webapi.ErrNoListenerRebinder
	}
	exitingACME := app.acmeActive // mode != acme here, so this is an acme→off/static exit

	newKey := app.listenerKey(cfg)
	cur := ctrl.CurrentKey()
	if cur == newKey {
		return webapi.ReloadApplied, nil // unchanged target — no-op
	}

	// Auto-443 lifeboat (H4b-3): only a same-mode, same-HTTPS-target static change is
	// re-planned in place. Leaving the auto-443 topology (a mode change) or moving the
	// HTTPS bind (a listen_address change) needs a full topology re-plan and stays
	// restart-required — the no-rebinder fallback.
	if app.autoHTTPSActive && (cfg.TLS.Mode != "static" || keyListenTarget(cur) != keyListenTarget(newKey)) {
		return webapi.ReloadProcess, webapi.ErrNoListenerRebinder
	}

	// An acme exit always releases first: the live acme topology holds several ports
	// (HTTPS + port-80 challenge + any redirect), so draining the whole Instance
	// before binding the replacement avoids a bind-new-first clash with a port the
	// old topology still holds. The replacement binds on the freed port(s) with the
	// same release-lag retry as a same-target rebind. Otherwise use same-target
	// detection (a same-address:port variant change cannot bind-new-first).
	inPlace := exitingACME || keyListenTarget(cur) == keyListenTarget(newKey)

	addr, port := resolveListen(cfg)
	attempts := 1
	if inPlace {
		attempts = inPlaceBindAttempts
	}

	var build serverctl.BuildFunc
	if cfg.TLS.Mode == "static" {
		// Construct (but do not bind) the TLS listener up front so a cert/config
		// error fails here, before the old listener is disturbed. With the auto-443
		// lifeboat active HTTPS targets the lifeboat port and the configured port +
		// http_redirect_port redirect to it; otherwise HTTPS targets the configured
		// port with http_redirect_port as the only redirect.
		httpsPort, redirectPorts := app.effectiveTLSTopology(cfg)
		listener, err := app.newTLSListener(handler, cfg, addr, httpsPort, redirectPorts)
		if err != nil {
			return webapi.ReloadProcess, err
		}
		build = func() (*serverctl.Instance, error) {
			return app.serveTLSListener(listener, addr, httpsPort, attempts)
		}
	} else {
		build = func() (*serverctl.Instance, error) {
			return app.buildPlainInstance(handler, addr, port, attempts)
		}
	}

	var changed bool
	var err error
	if inPlace {
		changed, err = ctrl.RebindInPlace(newKey, build)
	} else {
		changed, err = ctrl.Rebind(newKey, build)
	}
	if err != nil {
		return webapi.ReloadProcess, err
	}
	if !changed {
		return webapi.ReloadApplied, nil
	}
	if exitingACME {
		// Now serving plain/static off the acme topology — the renewer + challenge
		// drained with the old Instance. Clear acmeActive so subsequent saves take
		// the normal off/static path (re-entering acme stays restart-required —
		// H4c-2b). Stale tlsReload pointer on acme→off mirrors the H4a residual.
		app.acmeActive = false
	}
	return webapi.ReloadListener, nil
}

// listenTCP binds addr:port, retrying up to attempts times (attempts<=1 = a single
// try) with a fixed delay. The retry absorbs the OS port-release lag when the new
// listener must reclaim a port the old one just released on a same-target rebind.
func listenTCP(addr string, port, attempts int) (net.Listener, error) {
	return listenTCPTarget(net.JoinHostPort(addr, strconv.Itoa(port)), attempts)
}

// listenTCPTarget binds a "host:port" target, retrying up to attempts times
// (attempts<=1 = a single try) with a fixed delay. It is the redirect-listener
// analogue of listenTCP, letting serveTLSListener pre-bind each redirect port with
// the same release-lag retry as the HTTPS port on a same-target rebind.
func listenTCPTarget(target string, attempts int) (net.Listener, error) {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		ln, err := net.Listen("tcp", target)
		if err == nil {
			return ln, nil
		}
		lastErr = err
		if i < attempts-1 {
			time.Sleep(inPlaceBindRetryDelay)
		}
	}
	return nil, lastErr
}

// buildPlainInstance binds a new plain-HTTP listener at addr:port (retrying
// attempts times) and starts serving handler on it. A bind failure returns the
// error with nothing bound.
func (app *serverApp) buildPlainInstance(handler http.Handler, addr string, port, attempts int) (*serverctl.Instance, error) {
	if addr == "" {
		addr = "0.0.0.0"
	}
	if port == 0 {
		port = 8080
	}
	ln, err := listenTCP(addr, port, attempts)
	if err != nil {
		return nil, err
	}
	srv := apptls.NewPlainListener(handler, "", 0)
	srv.Addr = ln.Addr().String()
	go func() {
		app.startup.Info(fmt.Sprintf("HTTP server listening on %s (rebound)", ln.Addr()))
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			app.logger.WithScope(logging.ScopeWebAPI).Error(fmt.Sprintf("rebound HTTP server %s: %v", ln.Addr(), serveErr))
		}
	}()
	return &serverctl.Instance{Addr: ln.Addr().String(), Shutdown: srv.Shutdown}, nil
}

// newTLSListener assembles the apptls.ListenerConfig from the desired server
// config and constructs the static-TLS listener WITHOUT binding any port — so the
// certificate/TLS config is validated (off→static toggle, min_version/CA change)
// before the live listener is disturbed. For cert_source: db it fetches the
// current cert/key from the encrypted store so a prior hot-swap is preserved.
// httpsPort/redirectPorts come from effectiveTLSTopology (the configured port and
// its http_redirect_port, or the lifeboat port and its redirect set under auto-443);
// redirectPorts is passed via RedirectPorts (HTTPRedirectPort folds into it, so 0
// here is equivalent for the single-redirect case).
func (app *serverApp) newTLSListener(handler http.Handler, cfg config.ServerConfig, addr string, httpsPort int, redirectPorts []int) (*apptls.Listener, error) {
	lcfg := apptls.ListenerConfig{
		ListenAddress:           addr,
		Port:                    httpsPort,
		CertSource:              cfg.TLS.CertSource,
		CertPath:                cfg.TLS.CertPath,
		KeyPath:                 cfg.TLS.KeyPath,
		CAPath:                  cfg.TLS.CAPath,
		MinVersion:              cfg.TLS.MinVersion,
		RedirectPorts:           redirectPorts,
		GracefulShutdownTimeout: app.resolveShutdownTimeout(),
		TrustedProxy:            cfg.TrustedProxy,
		HSTSEnabled:             app.hstsEnabledFn(),
	}
	if lcfg.CertSource == "db" && app.cfgStore != nil {
		certPEM, keyPEM, loadErr := app.loadDBCertKey(context.Background())
		if loadErr != nil {
			return nil, loadErr
		}
		lcfg.CertPEM = certPEM
		lcfg.KeyPEM = keyPEM
	}
	return apptls.NewListener(handler, lcfg, app.tlsLog)
}

// serveTLSListener binds the already-constructed listener at addr:port (retrying
// attempts times to absorb a same-target release race), starts serving, and wires
// the in-place cert reloader and watch. The HTTPS listener is served on the
// configured port; an explicit http_redirect_port is served on a secondary
// redirect listener pre-bound here with the same retry (H4b-2) — the old process
// holds that port across a same-target RebindInPlace drain, so the bind must
// reclaim it. Under the auto-443 lifeboat (H4b-3) the same path serves HTTPS on the
// lifeboat port with the configured port + http_redirect_port as the redirect set
// (assembled by effectiveTLSTopology into RedirectPorts). A bind failure closes
// whatever was bound and returns the error (nothing left serving the target on a
// same-target rebind).
func (app *serverApp) serveTLSListener(listener *apptls.Listener, addr string, port, attempts int) (*serverctl.Instance, error) {
	ln, err := listenTCP(addr, port, attempts)
	if err != nil {
		return nil, err
	}
	listener.SetHTTPSListener(ln)

	// Pre-bind each redirect listener (http_redirect_port) with the same retry, so
	// a same-target rebind can reclaim the redirect port the old process is still
	// draining. A bind failure unwinds the HTTPS and any redirect already bound.
	if redirectAddrs := listener.RedirectAddrs(); len(redirectAddrs) > 0 {
		redirectLns := make([]net.Listener, 0, len(redirectAddrs))
		for _, raddr := range redirectAddrs {
			rln, rerr := listenTCPTarget(raddr, attempts)
			if rerr != nil {
				_ = ln.Close()
				for _, bound := range redirectLns {
					_ = bound.Close()
				}
				return nil, rerr
			}
			redirectLns = append(redirectLns, rln)
		}
		listener.SetRedirectListeners(redirectLns)
	}

	listener.CertManager().WatchForChanges(30 * time.Second)
	if app.tlsReload != nil {
		app.tlsReload.Set(listener.CertManager())
	}
	errCh := listener.Serve()
	go func() {
		if serveErr := <-errCh; serveErr != nil {
			app.logger.WithScope(logging.ScopeTLS).Error(fmt.Sprintf("rebound HTTPS server %s: %v", listener.Addr(), serveErr))
		}
	}()
	return &serverctl.Instance{Addr: listener.Addr(), Shutdown: listener.Shutdown}, nil
}

// loadDBCertKey fetches the cert_source: db certificate and private key from
// the encrypted config store. The certificate is non-secret; the private key
// is secret. Both are stored as JSON-encoded PEM strings by the admin save
// path. It returns an error if the store is unavailable or either entry is
// missing/undecodable, which the caller treats as a fail-open condition.
func (app *serverApp) loadDBCertKey(ctx context.Context) (certPEM, keyPEM []byte, err error) {
	if app.cfgStore == nil {
		return nil, nil, fmt.Errorf("cert_source is db but the config store is unavailable (set CMM_CREDENTIAL_ENCRYPTION_KEY)")
	}

	certRaw, err := app.cfgStore.Get(ctx, configstore.KeyServerTLSCertificate)
	if err != nil {
		return nil, nil, fmt.Errorf("reading TLS certificate from config store: %w", err)
	}
	var certStr string
	if err := json.Unmarshal(certRaw, &certStr); err != nil {
		return nil, nil, fmt.Errorf("decoding stored TLS certificate: %w", err)
	}

	keyRaw, err := app.cfgStore.GetSecret(ctx, configstore.KeyServerTLSPrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("reading TLS private key from config store: %w", err)
	}
	var keyStr string
	if err := json.Unmarshal(keyRaw, &keyStr); err != nil {
		return nil, nil, fmt.Errorf("decoding stored TLS private key: %w", err)
	}

	return []byte(certStr), []byte(keyStr), nil
}

// tlsHealthy reports whether the static-mode certificate material loads cleanly
// — the same check the listener performs at startup (tls-static.md § 2.4/§ 2.6).
// Automatic HTTPS on 443 (tls.md § 1.5) is gated on this: 443 is bound only when
// TLS is healthy, never on the fail-open self-signed path.
func (app *serverApp) tlsHealthy(lcfg apptls.ListenerConfig) bool {
	if lcfg.CertSource == "db" {
		return apptls.ValidateStaticPairBytes(lcfg.CertPEM, lcfg.KeyPEM, lcfg.CAPath) == nil
	}
	return apptls.ValidateStaticPair(lcfg.CertPath, lcfg.KeyPath, lcfg.CAPath) == nil
}

// planAutoHTTPS resolves the automatic-443 listening layout for a healthy TLS
// listener (tls.md § 1.5) and pre-binds 443 when it applies, so a 443 bind
// failure falls back to the configured port synchronously rather than surfacing
// asynchronously after the listener has started. It returns the effective HTTPS
// port, the redirect ports that 301 to it, and the pre-bound 443 listener (nil
// unless 443 was bound here — including when server.port is already 443, where
// the listener binds it itself).
//
// httpRedirectPort is the explicit server.tls.http_redirect_port that the HTTPS
// Listener should also run as a redirect; pass 0 in ACME mode, where the port-80
// challenge server owns that redirect instead.
func (app *serverApp) planAutoHTTPS(httpRedirectPort int) (httpsPort int, redirectPorts []int, ln443 net.Listener) {
	addr := app.cfg.Server.ListenAddress
	if addr == "" {
		addr = "0.0.0.0"
	}
	plan := apptls.ResolveAutoHTTPS(app.cfg.Server.Port, httpRedirectPort, func() bool {
		l, err := app.listen443(addr)
		if err != nil {
			fallbackPort := app.cfg.Server.Port
			if fallbackPort == 0 {
				fallbackPort = 8080
			}
			app.startup.Error(fmt.Sprintf(
				"automatic HTTPS on 443 unavailable: %v — serving HTTPS on the configured port %d with no 443 redirect",
				err, fallbackPort))
			if rem := apptls.BindPermissionRemediation(addr, 443, err); rem != "" {
				app.startup.Error(rem)
			}
			return false
		}
		ln443 = l
		return true
	})
	if plan.BoundTo443 {
		app.startup.Info(fmt.Sprintf(
			"automatic HTTPS on 443 enabled (redirecting %v → 443)", plan.RedirectPorts))
	}
	return plan.HTTPSPort, plan.RedirectPorts, ln443
}

// listen443 binds the lifeboat 443 port, honouring the auto443Listen test seam.
func (app *serverApp) listen443(listenAddr string) (net.Listener, error) {
	if app.auto443Listen != nil {
		return app.auto443Listen(listenAddr)
	}
	return net.Listen("tcp", net.JoinHostPort(listenAddr, "443"))
}

// listenTarget is a resolved (address, port) the server may bind.
type listenTarget struct {
	addr string
	port int
}

func (t listenTarget) String() string {
	return fmt.Sprintf("%s:%d", t.addr, t.port)
}

// listenCandidates returns the ordered, de-duplicated list of listen targets to
// try, starting with the configured (DB-sourced) target and falling back to the
// bootstrap target then the hardwired 0.0.0.0:8080 default. A bad DB-sourced
// listen_address/port can therefore never permanently lock out the UI — the
// server binds the first usable fallback and runs in degraded mode.
func (app *serverApp) listenCandidates() []listenTarget {
	norm := func(addr string, port int) listenTarget {
		if addr == "" {
			addr = "0.0.0.0"
		}
		if port == 0 {
			port = 8080
		}
		return listenTarget{addr: addr, port: port}
	}

	ordered := []listenTarget{
		norm(app.cfg.Server.ListenAddress, app.cfg.Server.Port),
		norm(app.bootstrapListenAddr, app.bootstrapPort),
		norm("0.0.0.0", 8080),
	}

	seen := make(map[listenTarget]bool, len(ordered))
	out := ordered[:0]
	for _, t := range ordered {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// servePlainHTTP starts a plain HTTP listener on the configured
// listen_address:port. If that target cannot be bound (e.g. a bad DB-sourced
// port), it falls back to the bootstrap then default target and flags degraded
// mode (reusing Chunk 2's TLS-status holder). Shared by plain-HTTP mode and the
// degraded TLS fallback (degradeToPlainHTTP).
func (app *serverApp) servePlainHTTP(handler http.Handler) serverResult {
	candidates := app.listenCandidates()

	var firstErr error
	for i, target := range candidates {
		ln, err := net.Listen("tcp", target.String())
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			app.startup.Error(fmt.Sprintf("cannot bind HTTP listener on %s: %v", target, err))
			if rem := apptls.BindPermissionRemediation(target.addr, target.port, err); rem != "" {
				app.startup.Error(rem)
			}
			continue
		}
		if i > 0 {
			// The configured target failed; we bound a fallback. Flag degraded
			// so the operator sees the banner and fixes listen_address/port.
			reason := fmt.Sprintf("cannot bind configured listen address %s (%v) — running on fallback %s",
				candidates[0], firstErr, target)
			app.startup.Error(reason + "; fix listen_address/port and restart")
			if app.tlsStatus != nil {
				app.tlsStatus.SetDegraded(reason)
			}
		}
		return app.serveOnListener(handler, ln)
	}

	// No candidate could be bound — return a fatal error result so run() exits.
	errCh := make(chan error, 1)
	errCh <- fmt.Errorf("HTTP listener bind failed on all candidates: %w", firstErr)
	close(errCh)
	return serverResult{errCh: errCh}
}

// serveOnListener serves the handler on an already-bound listener and returns
// the running server plus its fatal-error channel.
func (app *serverApp) serveOnListener(handler http.Handler, ln net.Listener) serverResult {
	plainSrv := apptls.NewPlainListener(handler, "", 0)
	plainSrv.Addr = ln.Addr().String()
	plainErrCh := make(chan error, 1)
	go func() {
		app.startup.Info(fmt.Sprintf("HTTP server listening on %s", ln.Addr()))
		if err := plainSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			plainErrCh <- err
		}
		close(plainErrCh)
	}()
	return serverResult{errCh: plainErrCh, plainSrv: plainSrv}
}

// tlsLog adapts the application logger to the internal/tls + internal/acme
// LogFunc seam, scoping messages to the tls log scope.
func (app *serverApp) tlsLog(level, msg string) {
	scoped := app.logger.WithScope(logging.ScopeTLS)
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

// hstsEnabledFn returns the live predicate a listener consults before emitting
// HSTS: enabled whenever TLS is healthy, suppressed while the listener is in a
// degraded self-signed fallback (tls-static.md § 2.4). It resumes automatically
// once a real certificate is promoted in place (which clears the degraded state).
func (app *serverApp) hstsEnabledFn() func() bool {
	return func() bool {
		return app.tlsStatus == nil || !app.tlsStatus.IsDegraded()
	}
}

// degradeToSelfSigned records the degraded TLS state and serves an ephemeral
// self-signed certificate over HTTPS, keeping the recovery UI on an encrypted
// channel rather than cleartext (tls-static.md § 2.4, tls-acme.md § 3.11). HSTS is
// suppressed while the self-signed cert is in use. The self-signed CertManager is
// registered for in-place reload so a later valid certificate (an admin save or
// ACME issuance) promotes it without a restart. If the self-signed listener
// cannot be generated or built, it falls back to plain HTTP as a last resort.
//
// hosts are the names placed in the self-signed cert SANs (nil ⇒ localhost).
func (app *serverApp) degradeToSelfSigned(handler http.Handler, hosts []string, cause error) serverResult {
	reason := fmt.Sprintf("TLS listener setup failed: %v", cause)

	certPEM, keyPEM, genErr := apptls.GenerateSelfSigned(hosts)
	if genErr != nil {
		app.startup.Error(fmt.Sprintf("self-signed fallback generation failed: %v — falling back to plain HTTP", genErr))
		return app.degradeToPlainHTTP(handler, cause)
	}

	shutdownTimeout := time.Duration(app.cfg.Server.GracefulShutdownSeconds) * time.Second
	if shutdownTimeout <= 0 {
		shutdownTimeout = 15 * time.Second
	}

	// No CAPath here: a degraded listener must never require client certs, or an
	// mTLS misconfig would re-lock the very UI we are trying to keep reachable.
	lcfg := apptls.ListenerConfig{
		ListenAddress:           app.cfg.Server.ListenAddress,
		Port:                    app.cfg.Server.Port,
		CertSource:              "db",
		CertPEM:                 certPEM,
		KeyPEM:                  keyPEM,
		MinVersion:              app.cfg.Server.TLS.MinVersion,
		GracefulShutdownTimeout: shutdownTimeout,
		TrustedProxy:            app.cfg.Server.TrustedProxy,
		HSTSEnabled:             app.hstsEnabledFn(),
	}

	selfListener, err := apptls.NewListener(handler, lcfg, app.tlsLog)
	if err != nil {
		app.startup.Error(fmt.Sprintf("self-signed fallback listener failed: %v — falling back to plain HTTP", err))
		return app.degradeToPlainHTTP(handler, cause)
	}

	app.startup.Error(reason +
		" — serving an untrusted self-signed certificate over HTTPS (degraded); fix the certificate and restart")
	if app.tlsStatus != nil {
		app.tlsStatus.SetDegradedKind(webapi.DegradedKindSelfSigned, reason)
	}
	if app.tlsReload != nil {
		// Let an admin save / ACME issuance promote a real cert in place.
		app.tlsReload.Set(selfListener.CertManager())
	}

	return serverResult{errCh: selfListener.Serve(), tlsListener: selfListener}
}

// degradeToPlainHTTP records the degraded TLS state and starts a plain HTTP
// listener as a last-resort fallback (tls.md § 6.3), used only when even the
// self-signed degraded listener cannot be brought up. The operator-facing reason
// never includes private key material — it is the listener-setup error, which
// reports file paths and parse failures only.
func (app *serverApp) degradeToPlainHTTP(handler http.Handler, cause error) serverResult {
	reason := fmt.Sprintf("TLS listener setup failed: %v", cause)
	app.startup.Error(reason +
		" — falling back to plain HTTP (INSECURE); fix the certificate and restart")
	if app.tlsStatus != nil {
		app.tlsStatus.SetDegraded(reason)
	}
	return app.servePlainHTTP(handler)
}

// ---------------------------------------------------------------------------
// Phase: signal handling loop and graceful shutdown.
// ---------------------------------------------------------------------------

// resolveShutdownTimeout returns the graceful-shutdown drain budget, read live
// from the config holder when one is wired so a saved graceful_shutdown_seconds
// change applies at the next shutdown without a restart (config live-reload H1).
// It falls back to the boot config when no holder is set, and a non-positive
// value defaults to 15s.
func (app *serverApp) resolveShutdownTimeout() time.Duration {
	secs := app.cfg.Server.GracefulShutdownSeconds
	if app.configHolder != nil {
		secs = app.configHolder.Get().Server.GracefulShutdownSeconds
	}
	timeout := time.Duration(secs) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return timeout
}

func (app *serverApp) awaitShutdown(srv serverResult) int {
	shutdownTimeout := app.resolveShutdownTimeout()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	running := true
	restartRequested := false
	for running {
		select {
		case <-app.restartCh:
			app.startup.Info("restart requested via admin API, shutting down gracefully for supervisor restart...")
			restartRequested = true
			running = false
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

	// Stop the ACME renewal loop first (mode: acme) so no new issuance starts
	// while we drain. This is the fallback path: when the controller adopted the
	// acme topology (H4c-1, rebinder wired), renewerCancel/challengeSrv are nil
	// here and the renewer + challenge server drain via the composite Instance in
	// the controller.Shutdown branch below.
	if srv.renewerCancel != nil {
		app.startup.Info("stopping ACME renewal loop...")
		srv.renewerCancel()
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

	// Drain the ACME http-01 challenge/redirect server (mode: acme) alongside
	// the HTTPS listener. A shutdown error here is logged but not fatal.
	if srv.challengeSrv != nil {
		app.startup.Info("shutting down ACME challenge/redirect server...")
		if err := srv.challengeSrv.Shutdown(shutdownCtx); err != nil {
			app.startup.Error(fmt.Sprintf("ACME challenge server shutdown: %v", err))
		}
	}

	// In rebind-capable modes the controller owns the live listener (which a
	// rebind may have swapped out from under the boot serverResult), so drain via
	// it. Otherwise fall back to the boot listener captured in srv.
	switch {
	case app.listenerController != nil:
		if err := app.listenerController.Shutdown(shutdownCtx); err != nil {
			app.startup.Error(fmt.Sprintf("HTTP(S) server shutdown: %v", err))
			return 1
		}
	case srv.tlsListener != nil:
		if err := srv.tlsListener.Shutdown(shutdownCtx); err != nil {
			app.startup.Error(fmt.Sprintf("TLS server shutdown: %v", err))
			return 1
		}
	case srv.plainSrv != nil:
		if err := srv.plainSrv.Shutdown(shutdownCtx); err != nil {
			app.startup.Error(fmt.Sprintf("HTTP server shutdown: %v", err))
			return 1
		}
	}

	if restartRequested {
		app.startup.Info(fmt.Sprintf("server stopped cleanly; exiting %d for supervisor restart", exitCodeRestart))
		return exitCodeRestart
	}

	app.startup.Info("server stopped cleanly")
	return 0
}

// ---------------------------------------------------------------------------
// run orchestrates all startup phases in order.
// ---------------------------------------------------------------------------

func run() int {
	// Repair subcommands are dispatched before flag parsing: `tls reset` /
	// `tls clear-ca` are host-side lockout recovery (tls.md § 6.3) and do not
	// share the server's flag set.
	if len(os.Args) > 1 && os.Args[1] == "tls" {
		return runTLSCommand(os.Args[2:])
	}

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

	// Phase 9b: SAML SSO (optional, needs credential resolver from Phase 9).
	app.setupSAML(ctx)

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
