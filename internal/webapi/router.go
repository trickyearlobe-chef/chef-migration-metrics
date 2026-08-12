// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/pprof"
	"sync"
	"sync/atomic"

	"path"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/backup"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/kitchenqueue"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/nodekitchen"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/platform"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/hypervisor"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/perf"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// CollectionTriggerFunc is a function that triggers an immediate collection
// run in the background. The Router calls this after invalidating cached
// results so the rescan starts immediately instead of waiting for the next
// scheduled collection cycle. The function should be non-blocking (i.e.
// launch the run in a goroutine) and return a non-nil error only if the
// run cannot be started (e.g. one is already in progress).
type CollectionTriggerFunc func(ctx context.Context) error

// Router is the top-level HTTP handler for the Chef Migration Metrics API.
// It assembles the ServeMux with all API routes, the WebSocket endpoint,
// health/version endpoints, and the frontend static asset fallback.
type Router struct {
	mux           *http.ServeMux
	hub           *EventHub
	db            DataStore
	cfg           *config.Config
	version       string
	schemaVersion int

	// routes is every address registered, recorded as it is registered. It is
	// what the API description is generated from, and what a drift test checks
	// that description against in both directions. See routes.go.
	routes []Route

	// duplicateScanRunning guards the possible-duplicate-owners scan. The scan
	// walks the whole owner catalogue, which takes tens of seconds on a large
	// one, so it runs detached from the request that asked for it and only one
	// runs at a time.
	duplicateScanRunning atomic.Bool

	// tlsStatus reports whether the server fell back to plain HTTP because the
	// configured static TLS listener could not be started (see tls.md § 2.4).
	// nil on plain-HTTP/ACME deployments — the status endpoint then reports
	// healthy. Set via WithTLSStatus.
	tlsStatus *TLSStatusHolder

	// frontendFS holds the built React SPA assets (index.html, JS, CSS).
	// When non-nil, the frontend fallback handler serves files from this
	// filesystem instead of returning a plain-text placeholder. Set via
	// WithFrontendFS at construction time.
	frontendFS fs.FS

	// logger is an optional callback for logging request-level events.
	// If nil, events are silently discarded. The webapi package does not
	// import the logging package to avoid circular dependencies — the
	// caller provides a logging function at construction time.
	logger func(level, msg string)

	// logLevelSetter re-applies the minimum log level on the running logger
	// (the logging.level subsystem applier). The value is the validated level
	// string (DEBUG/INFO/WARN/ERROR); the callback parses and applies it. nil
	// when no logger is wired — the logging section then has no applier and
	// stays at the pessimistic process default. Kept as a string callback so
	// webapi need not import the logging package (see logger above). Set via
	// WithLogLevelSetter.
	logLevelSetter func(level string) error

	// collectionRescheduler re-applies the collection.schedule cron to the
	// running collection scheduler (the collection.schedule subsystem applier).
	// The value is the validated cron string; the callback parses and reschedules
	// in place. nil when no scheduler is wired — the schedule half of the
	// collection section then has no applier (its live-read thresholds still
	// apply, but a schedule change silently needs a restart). Kept as a string
	// callback so webapi need not import the collector package. Set via
	// WithCollectionRescheduler.
	collectionRescheduler func(schedule string) error

	// backupReconciler reconciles the running backup scheduler to the stored
	// backup config (the backup.{enabled,schedule} subsystem applier): start it
	// when newly enabled, stop it when disabled, reschedule it in place on a
	// schedule change. It reads the reloaded live config itself, so it takes no
	// arguments and the schedule default is not duplicated here. nil when no
	// backup scheduler is wired — the backup section then has no applier and
	// stays at the pessimistic process default. Kept as a plain callback so
	// webapi need not import the backup package. Set via WithBackupReconciler.
	backupReconciler func() error

	// readinessReconciler triggers a recompute of node readiness for all
	// organisations after a readiness config change (e.g. flipping
	// readiness.review_blocks_readiness or a disk threshold). It reads the
	// reloaded live config itself and should kick the recompute in the
	// background, returning promptly so the admin PUT does not block on a
	// full re-evaluation. nil when no evaluator is wired — the readiness
	// section then applies live-per-request only (next collection cycle picks
	// up the change). Set via WithReadinessReconciler.
	readinessReconciler func() error

	// --- Authentication components (set via WithAuth) ---

	// localAuth handles local username/password authentication with
	// brute-force protection. Nil when no local provider is configured.
	localAuth *auth.LocalAuthenticator

	// sessions manages session creation, validation, and invalidation.
	// Nil when authentication is not configured.
	sessions *auth.SessionManager

	// credentials issues and validates the credentials a person makes for a
	// tool they are holding. Nil where they are not wired, in which case the
	// endpoints under /api/v1/auth/me/tokens answer "not implemented" rather
	// than not existing — an assistant working from the description needs to
	// tell "switched off here" from "I guessed the address".
	credentials *auth.CredentialManager

	// authMiddleware provides RequireAuth and RequireAdmin HTTP middleware.
	// Nil when authentication is not configured.
	authMiddleware *auth.Middleware

	// authStore provides direct user CRUD for the admin user-management
	// endpoints. Nil when authentication is not configured.
	authStore AuthStore

	// --- SAML authentication components (set via WithSAML) ---

	// samlHandler holds the SAML SSO/SLO HTTP handlers.
	// Nil when SAML is not configured.
	samlHandler *SAMLHandler

	// samlReconciler rebuilds the SAML provider from the freshly-stored auth
	// config and swaps it into the running samlHandler (the SAML half of the
	// auth subsystem applier). It reads the reloaded live config itself, so it
	// takes no arguments. nil when no SAML handler is wired — the auth section
	// then has no subsystem applier and reports applied (session/lockout are
	// still live reads). Kept as a plain callback so webapi need not import the
	// samlsp/credential construction. Set via WithSAMLReconciler.
	samlReconciler func() error
	// is requested. Nil when not wired up — rescan handlers fall back to
	// the "will run on next collection cycle" behaviour.
	triggerCollection CollectionTriggerFunc

	// recorder holds the in-memory request latency circular buffer used
	// by the timing middleware and the GET /api/v1/admin/performance
	// endpoint. Nil when performance instrumentation is disabled.
	recorder *perf.Recorder

	// timingHandler wraps the mux with the request timing middleware.
	// Populated by NewRouter when a recorder is configured. Nil when
	// performance instrumentation is disabled (requests go directly
	// through r.mux).
	timingHandler http.Handler

	// credentialStore provides encrypted credential CRUD for the admin
	// credential management endpoints. Nil when the master encryption
	// key (CMM_CREDENTIAL_ENCRYPTION_KEY) is not configured — credential
	// endpoints return 503.
	credentialStore secrets.CredentialStore

	// configStore provides encrypted config CRUD for the admin config
	// section endpoints. Nil when CMM_CREDENTIAL_ENCRYPTION_KEY is not set.
	configStore *configstore.Store
	// configHolder provides live config access and reload for the admin
	// config section endpoints. Nil when not wired up.
	configHolder *configstore.ConfigHolder

	// tlsReload triggers an in-place swap of the running static-TLS
	// certificate when a new cert_source: db pair is saved, so the listener
	// serves it without a restart (tls-static.md § 2.3). Nil on plain-HTTP
	// deployments or when the running listener is not a DB source — the save
	// still persists and a restart applies it. Set via WithTLSReload.
	tlsReload *TLSReloadHolder

	// listenerRebind rebinds the running HTTP/TLS listener in place when a
	// changed server.listen_address/port is saved, so the change applies without
	// a restart (configuration-live-reload.md listener-rebind H2). Nil/unset on
	// deployments where no rebinder is wired (tests, active auto-443, ACME, or a
	// degraded fallback) — the save then reports restart_required. Set via
	// WithListenerRebinder.
	listenerRebind *ListenerRebindHolder

	// acmeReRegister, when set, is called after an ACME config save to wake the
	// renewer so hostname registration and an issuance check re-run immediately
	// rather than waiting out the renewal interval (tls-acme.md § 3.14). Nil in
	// non-ACME deployments. Non-blocking. Set via WithACMETrigger.
	acmeReRegister func()

	// onOrganisationsChanged, when set, is called after a successful write to
	// the organisations config section. It reconciles the operational
	// `organisations` table from live config and triggers a collection, so a
	// newly added org takes effect without a restart (configuration-live-reload.md;
	// web-api-organisations.md). An error fails the PUT (500). Nil when not wired.
	onOrganisationsChanged func(context.Context) error

	// hypervisor provides template discovery, VM inventory, and orphan
	// cleanup. When nil, buildHypervisor() builds one on demand from live
	// config. A static value (from WithHypervisor) takes precedence — this
	// is used in tests with mock clients.
	hypervisor hypervisor.Hypervisor

	// credResolver resolves credential secrets from the encrypted store,
	// environment variables, or files. Used to build hypervisor clients on
	// demand from live config without requiring a restart.
	credResolver *secrets.CredentialResolver

	// nodeKitchenRunner orchestrates on-demand Node Kitchen runs.
	// Nil when not configured — the trigger endpoint returns 503.
	nodeKitchenRunner NodeKitchenRunner

	// kitchenQueue manages the DB-backed queue and worker pool for all
	// kitchen runs (git and node). Nil when not configured — handlers
	// return 503.
	kitchenQueue *kitchenqueue.Manager

	// cookstylePropagator runs the scoped recompute closure after a cop
	// reclassification or custom-cop change (re-derive verdicts → compat →
	// complexity → dependent-node readiness). Nil when not wired — changes are
	// persisted but not propagated. Set via WithCookstylePropagator.
	cookstylePropagator *CookstylePropagator

	// reclassQueue runs cop-reclassification reassessments asynchronously and
	// coalesced, so saving a classification returns instantly. Lazily created.
	reclassQueue     *reclassificationQueue
	reclassQueueOnce sync.Once

	// copRegistry supplies the live `cookstyle --show-cops` cop registry for the
	// drift report and the cop-list universe (Chef/* cops listable before they
	// trigger). Nil when cookstyle is unavailable — the drift report degrades to
	// registry_available=false and the cop list falls back to the static
	// universe. Set via WithCopRegistry.
	copRegistry CopRegistryProvider

	// batchMu guards runningBatch. Only held for fast map reads/writes.
	batchMu sync.Mutex
	// runningBatch maps batch ID to its cancel function for active batches.
	// Entries are added when a batch launches and removed only when the
	// background goroutine finishes — NOT on cancel (so in-flight workers
	// that are draining still block new batch starts).
	runningBatch map[string]context.CancelFunc

	// roleCompatCache holds in-memory cached results from GetRoleCompatSummary,
	// keyed by org+name+targetVersion. Each entry expires after 60 seconds.
	roleCompatCache sync.Map // key: string → *roleCompatCacheEntry

	// backupService provides database backup and restore operations.
	// Nil when backup is not configured — backup handlers return 503.
	backupService *backup.Service

	// restoreHook is called before restore to stop background workers.
	// Provided by main.go. If nil, restore skips worker shutdown.
	restoreHook func()

	// exitFunc is called after a successful restore to terminate the process.
	// Defaults to os.Exit. Overridable for testing.
	exitFunc func(code int)

	// restartFunc signals the main goroutine to perform a graceful restart
	// (drain workers + HTTP server, then exit with a supervisor-restart code).
	// Provided by main via WithRestartTrigger. Nil when no supervisor is wired
	// — the POST /admin/restart endpoint then returns 503.
	restartFunc func()

	// maintenanceMode is set to true during restore operations.
	// When true, all API routes except health and backup status return 503.
	maintenanceMode atomic.Bool

	// maintenanceMessage describes the current maintenance operation.
	maintenanceMessage atomic.Value // stores string
}

// AuthStore is the interface consumed by admin user-management handlers. It
// abstracts the concrete *datastore.DB so that handlers can be tested with
// stubs. The signatures match the corresponding methods on *datastore.DB.
type AuthStore interface {
	InsertUser(ctx context.Context, p datastore.InsertUserParams) (datastore.User, error)
	GetUserByUsername(ctx context.Context, username string) (datastore.User, error)
	ListUsers(ctx context.Context) ([]datastore.User, error)
	CountUsers(ctx context.Context) (int, error)
	UpdateUser(ctx context.Context, username string, p datastore.UpdateUserParams) (datastore.User, error)
	UpdateUserPassword(ctx context.Context, username, passwordHash string) error
	DeleteUser(ctx context.Context, username string) error
	IncrementFailedLoginAttempts(ctx context.Context, username string) (int, error)
	LockUser(ctx context.Context, username string) error
	RecordLoginSuccess(ctx context.Context, username string) error
}

// RouterOption is a functional option for NewRouter.
type RouterOption func(*Router)

// WithVersion sets the application version reported by /api/v1/version and
// /api/v1/health.
func WithVersion(v string) RouterOption {
	return func(r *Router) {
		r.version = v
	}
}

// WithSchemaVersion sets the database schema version reported by /api/v1/version
// and /api/v1/health.
func WithSchemaVersion(v int) RouterOption {
	return func(r *Router) {
		r.schemaVersion = v
	}
}

// WithLogger sets a logging callback for API lifecycle events.
// The level parameter is one of "DEBUG", "INFO", "WARN", "ERROR".
func WithLogger(fn func(level, msg string)) RouterOption {
	return func(r *Router) {
		r.logger = fn
	}
}

// WithLogLevelSetter wires the live log-level apply point for the logging.level
// config section. fn receives the validated level string and applies it to the
// running logger; when set, a logging PUT re-applies the level in place
// (subsystem) instead of requiring a restart. Without it the section keeps the
// pessimistic process default.
func WithLogLevelSetter(fn func(level string) error) RouterOption {
	return func(r *Router) {
		r.logLevelSetter = fn
	}
}

// WithCollectionRescheduler wires the live reschedule apply point for the
// collection.schedule config field. fn receives the validated cron string and
// reschedules the running collection scheduler in place; when set, a collection
// PUT applies a schedule change without a restart (subsystem). Without it a
// schedule change is persisted but only takes effect on the next restart.
func WithCollectionRescheduler(fn func(schedule string) error) RouterOption {
	return func(r *Router) {
		r.collectionRescheduler = fn
	}
}

// WithBackupReconciler wires the live apply point for the backup config section.
// fn reconciles the running backup scheduler to the reloaded live config —
// starting it when newly enabled, stopping it when disabled, rescheduling it in
// place on a schedule change. When set, a backup PUT applies without a restart
// (subsystem). Without it the section keeps the pessimistic process default.
func WithBackupReconciler(fn func() error) RouterOption {
	return func(r *Router) {
		r.backupReconciler = fn
	}
}

// WithReadinessReconciler wires the live apply point for the readiness config
// section. fn recomputes node readiness for all organisations against the
// reloaded live config (so a review_blocks_readiness or disk-threshold change
// takes effect immediately rather than waiting for the next collection cycle).
// fn should launch the recompute in the background and return promptly. When
// set, a readiness PUT applies without a restart (subsystem). Without it the
// section reads live-per-request only.
func WithReadinessReconciler(fn func() error) RouterOption {
	return func(r *Router) {
		r.readinessReconciler = fn
	}
}

// WithFrontendFS sets the filesystem containing the built React SPA
// assets (typically the Vite output directory). When set, all non-API
// requests are served from this filesystem, with a fallback to
// index.html for client-side routing. When nil, a plain-text
// placeholder is returned instead.
func WithFrontendFS(fsys fs.FS) RouterOption {
	return func(r *Router) {
		r.frontendFS = fsys
	}
}

// WithAuth wires in the authentication components — local authenticator,
// session manager, auth middleware, and user store. When set, the auth
// placeholder routes are replaced with real handlers and protected endpoints
// are wrapped with session enforcement middleware.
func WithAuth(
	localAuth *auth.LocalAuthenticator,
	sessions *auth.SessionManager,
	mw *auth.Middleware,
	store AuthStore,
) RouterOption {
	return func(r *Router) {
		r.localAuth = localAuth
		r.sessions = sessions
		r.authMiddleware = mw
		r.authStore = store
	}
}

// WithCredentialManager wires the credentials a person makes for a tool they
// are holding. Separate from WithAuth because a deployment can have sessions
// without them — and because a nil one is a legitimate state that the routes
// report honestly rather than by not existing.
func WithCredentialManager(c *auth.CredentialManager) RouterOption {
	return func(r *Router) {
		r.credentials = c
	}
}

// WithCollectionTrigger sets the function used by rescan handlers to kick
// off an immediate collection run after invalidating cached results. When
// nil, rescan handlers only invalidate and wait for the next scheduled
// collection cycle.
func WithCollectionTrigger(fn CollectionTriggerFunc) RouterOption {
	return func(r *Router) {
		r.triggerCollection = fn
	}
}

// WithCredentialStore sets the encrypted credential store used by the admin
// credential management endpoints. When nil, credential endpoints return
// 503 Service Unavailable.
func WithCredentialStore(store secrets.CredentialStore) RouterOption {
	return func(r *Router) {
		r.credentialStore = store
	}
}

// WithConfigStore wires in the encrypted config store and config holder
// used by the admin config section endpoints. When nil, config section
// endpoints return 503 Service Unavailable.
func WithConfigStore(store *configstore.Store, holder *configstore.ConfigHolder) RouterOption {
	return func(r *Router) {
		r.configStore = store
		r.configHolder = holder
	}
}

// WithOrganisationsChanged sets the callback invoked after a successful write
// to the organisations config section. The caller reconciles the operational
// `organisations` table from live config and triggers a collection, so a newly
// added org is collected without a restart. A returned error fails the PUT.
func WithOrganisationsChanged(fn func(context.Context) error) RouterOption {
	return func(r *Router) { r.onOrganisationsChanged = fn }
}

// WithHypervisor sets a static hypervisor client. When set, this takes
// precedence over building one dynamically from live config. Primarily
// used in tests with mock hypervisor clients.
func WithHypervisor(h hypervisor.Hypervisor) RouterOption {
	return func(r *Router) { r.hypervisor = h }
}

// WithCredentialResolver sets the credential resolver used to build
// hypervisor clients on demand from live config.
func WithCredentialResolver(cr *secrets.CredentialResolver) RouterOption {
	return func(r *Router) { r.credResolver = cr }
}

// NodeKitchenRunner abstracts the Node Kitchen run orchestrator so that
// callers can inject either a single-org runner or a multi-org factory.
type NodeKitchenRunner interface {
	Run(ctx context.Context, req nodekitchen.RunRequest) nodekitchen.RunResult
}

// WithNodeKitchenRunner sets the runner used by the Node Kitchen trigger endpoint.
func WithNodeKitchenRunner(runner NodeKitchenRunner) RouterOption {
	return func(r *Router) { r.nodeKitchenRunner = runner }
}

// WithKitchenQueue sets the kitchen queue manager for bounded-concurrency
// kitchen execution. When set, handlers enqueue items instead of spawning
// goroutines directly.
func WithKitchenQueue(m *kitchenqueue.Manager) RouterOption {
	return func(r *Router) { r.kitchenQueue = m }
}

// WithCookstylePropagator wires the re-evaluation propagator used after a cop
// reclassification or custom-cop change to run the scoped recompute closure
// (re-derive verdicts → compat → complexity → dependent-node readiness). When
// unset, classification/custom-cop changes are persisted but not propagated.
func WithCookstylePropagator(p *CookstylePropagator) RouterOption {
	return func(r *Router) { r.cookstylePropagator = p }
}

// CopRegistryProvider supplies the live cookstyle cop registry. Implemented by
// *analysis.CopRegistryProvider; an interface here so tests can inject a
// hand-built registry and so webapi does not depend on the binary at runtime.
type CopRegistryProvider interface {
	Registry(ctx context.Context) (*analysis.CopRegistry, error)
}

// WithCopRegistry wires the live cop registry provider used by the drift report
// and the cop-list universe. When unset, the drift report reports the registry
// unavailable and the cop list uses only the static tables.
func WithCopRegistry(p CopRegistryProvider) RouterOption {
	return func(r *Router) { r.copRegistry = p }
}

// WithSAML wires in the SAML SSO/SLO handler. When set, the SAML placeholder
// routes are replaced with real handlers for metadata, login, ACS, and SLO.
func WithSAML(h *SAMLHandler) RouterOption {
	return func(r *Router) { r.samlHandler = h }
}

// WithSAMLReconciler wires the live apply point for the SAML half of the auth
// config section: on save it rebuilds the provider from the reloaded config and
// swaps it into the running handler. Without it the auth section has no
// subsystem applier (session/lockout still reload as applied reads).
func WithSAMLReconciler(fn func() error) RouterOption {
	return func(r *Router) { r.samlReconciler = fn }
}

// WithBackupService sets the backup service used by admin backup endpoints.
func WithBackupService(svc *backup.Service) RouterOption {
	return func(r *Router) { r.backupService = svc }
}

// WithRestoreHook sets a callback invoked before restore to stop background
// workers (collector, kitchen queue, etc.). The hook should block until
// workers are drained.
func WithRestoreHook(fn func()) RouterOption {
	return func(r *Router) { r.restoreHook = fn }
}

// WithExitFunc overrides the function called after a successful restore.
// Defaults to os.Exit. Intended for testing.
func WithExitFunc(fn func(int)) RouterOption {
	return func(r *Router) { r.exitFunc = fn }
}

// WithRestartTrigger wires the function used by POST /admin/restart to request
// a graceful restart. The function must be non-blocking: it signals the main
// goroutine, which performs the graceful shutdown and process exit. When nil,
// the restart endpoint returns 503.
func WithRestartTrigger(fn func()) RouterOption {
	return func(r *Router) { r.restartFunc = fn }
}

// WithACMETrigger wires the function called after a successful ACME config save
// to wake the renewal loop immediately, so hostname registration and an
// issuance check re-run without waiting out the renewal interval (tls-acme.md
// § 3.14). Typically the running Renewer's Trigger method. Must be non-blocking;
// nil disables the immediate re-assert (the next scheduled cycle still picks up
// the change).
func WithACMETrigger(fn func()) RouterOption {
	return func(r *Router) { r.acmeReRegister = fn }
}

// NewRouter creates a new Router with all routes registered. The EventHub
// must already be running (via go hub.Run()) before requests are served.
//
// If WebSocket is disabled in the configuration, the /api/v1/ws endpoint
// returns 404.
func NewRouter(db DataStore, cfg *config.Config, hub *EventHub, opts ...RouterOption) *Router {
	r := &Router{
		mux:          http.NewServeMux(),
		hub:          hub,
		db:           db,
		cfg:          cfg,
		version:      "dev",
		runningBatch: make(map[string]context.CancelFunc),
	}
	for _, opt := range opts {
		opt(r)
	}

	// Log which optional components are wired in. This makes it obvious
	// from startup output when a component was defined but never passed
	// to NewRouter — the most common class of silent wiring bug with
	// functional options.
	r.logf("INFO", "router optional components: logger=%t frontend=%t auth=%t credentials=%t config_store=%t perf=%t collection_trigger=%t cred_resolver=%t hypervisor_static=%t node_kitchen=%t kitchen_queue=%t",
		r.logger != nil,
		r.frontendFS != nil,
		r.authMiddleware != nil,
		r.credentialStore != nil,
		r.configStore != nil,
		r.recorder != nil,
		r.triggerCollection != nil,
		r.credResolver != nil,
		r.hypervisor != nil,
		r.nodeKitchenRunner != nil,
		r.kitchenQueue != nil,
	)

	r.registerRoutes()

	// Clean up batches stranded by a previous process crash/restart.
	if r.db != nil {
		n, err := r.db.CancelStaleBatches(context.Background(), time.Now().UTC())
		if err != nil {
			r.logf("ERROR", "router: failed to cancel stale batches: %v", err)
		} else if n > 0 {
			r.logf("INFO", "router: cancelled %d stale batches from previous run", n)
		}
	}

	// Wrap the entire mux with the timing middleware when a recorder is
	// present. This captures latency for all routes (protect + adminOnly)
	// while excluding nothing — the overhead is <1µs per request.
	if r.recorder != nil && r.cfg.Performance.IsEnabled() {
		mw := perf.NewMiddleware(r.recorder)
		r.timingHandler = mw.Wrap(r.mux)
	}

	return r
}

// ServeHTTP implements http.Handler, delegating to the internal ServeMux.
// When a perf.Recorder is configured, every request is wrapped by the
// timing middleware so that latency is captured for all mux-routed paths.
// During maintenance mode, API routes return 503 except health and backup status.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if r.maintenanceMode.Load() && r.isMaintenanceBlocked(req.URL.Path) {
			msg := "Database restore in progress. Please wait."
			if v := r.maintenanceMessage.Load(); v != nil {
				if s, ok := v.(string); ok && s != "" {
					msg = s
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"error":"maintenance","message":%q}`, msg)
			return
		}
		if r.timingHandler != nil {
			r.timingHandler.ServeHTTP(w, req)
		} else {
			r.mux.ServeHTTP(w, req)
		}
	})).ServeHTTP(w, req)
}

// isMaintenanceBlocked returns true if the path should be blocked during
// maintenance mode. Health, version, and backup status endpoints remain
// accessible. Frontend assets are also served normally.
func (r *Router) isMaintenanceBlocked(urlPath string) bool {
	if !strings.HasPrefix(urlPath, "/api/") {
		return false
	}
	switch {
	case urlPath == "/api/v1/health":
		return false
	case urlPath == "/api/v1/version":
		return false
	case urlPath == "/api/v1/admin/backups/status":
		return false
	case urlPath == "/api/v1/server/tls-status":
		return false
	default:
		return true
	}
}

// Hub returns the EventHub so callers (main, collector, etc.) can broadcast
// events.
func (r *Router) Hub() *EventHub {
	return r.hub
}

// registerRoutes wires all API endpoints into the ServeMux. Routes are
// grouped by concern.
//
// Every registration goes through the recording funnel in routes.go —
// public, protect, operatorOnly, adminOnly — and nothing here touches the mux
// directly. That is what lets the API describe itself: a route registered
// around the funnel would be served and undescribed, so a test reads this
// function and fails if the mux appears in it.
func (r *Router) registerRoutes() {
	// -----------------------------------------------------------------
	// Health & version (public — no auth required)
	// -----------------------------------------------------------------
	// The description of everything below. Behind a session: it names every
	// address this deployment serves, which is not something to hand out
	// unauthenticated inside a customer estate.
	r.protect(openAPIPath, r.handleOpenAPI)

	// The assistant surface. Served by this binary rather than beside it,
	// because a second process deployed next to this one cannot happen inside
	// a customer estate. Behind a session for the same reason the description
	// is: it names what this deployment can be asked for. See mcp.go.
	r.protect(mcpPath, r.handleMCP, methods("POST"),
		takes("POST", jsonRPCMessage{}),
		answers("POST", jsonRPCResponse{}))

	r.public("/api/v1/health", r.handleHealth)
	r.public("/api/v1/version", r.handleVersion, answers("GET", versionResponse{}))
	// Event ingest sink — INTENTIONALLY UNAUTHENTICATED (MVP tech debt). Passive
	// receiver for Chef run telemetry; gated at runtime by ingest.enabled.
	r.public("/api/v1/ingest", r.handleIngest, methods("POST"))
	r.public("/api/v1/server/tls-status", r.handleServerTLSStatus,
		answers("GET", TLSStatus{}))

	// -----------------------------------------------------------------
	// WebSocket real-time events
	// -----------------------------------------------------------------
	if r.cfg.Server.WebSocket.IsEnabled() {
		wsHandler := NewWebSocketHandler(r.hub, r.webSocketOpts()...)
		r.protect("/api/v1/ws", wsHandler.ServeHTTP)
		r.logf("INFO", "WebSocket endpoint enabled at /api/v1/ws (max_connections=%d)",
			r.cfg.Server.WebSocket.MaxConnections)
	} else {
		r.public("/api/v1/ws", func(w http.ResponseWriter, req *http.Request) {
			WriteError(w, http.StatusNotFound, ErrCodeNotFound,
				"WebSocket endpoint is disabled in server configuration.")
		})
		r.logf("INFO", "WebSocket endpoint disabled by configuration")
	}

	// -----------------------------------------------------------------
	// Authentication endpoints (public — no session required for login)
	// -----------------------------------------------------------------
	r.public("/api/v1/auth/info", r.handleAuthInfo,
		answers("GET", authInfoResponse{}))
	if r.localAuth != nil && r.sessions != nil {
		r.public("/api/v1/auth/login", r.handleLogin, methods("POST"),
			takes("POST", loginRequest{}))
		r.public("/api/v1/auth/logout", r.handleLogout, methods("POST"))
		r.protect("/api/v1/auth/me", r.handleMe, answers("GET", meResponse{}))
	} else {
		r.public("/api/v1/auth/login", r.handleNotImplemented, methods("POST"),
			takes("POST", loginRequest{}))
		r.public("/api/v1/auth/logout", r.handleNotImplemented, methods("POST"))
		r.public("/api/v1/auth/me", r.handleNotImplemented)
	}
	// A credential from my own record, without raising a ticket. Under
	// /auth/me because that is whose record it comes from: there is no address
	// here for anybody else's, not for an administrator either.
	//
	// Registered whatever this deployment has wired, so that a deployment
	// without credentials answers "not available here" rather than "no such
	// address". An assistant working from the description has to be able to
	// tell a feature that is off from an address it guessed wrong — one makes
	// it ask a person, the other makes it invent.
	r.protect("/api/v1/auth/me/tokens", r.handleMyTokens, methods("GET", "POST"),
		takes("POST", createTokenRequest{}))
	r.protect("/api/v1/auth/me/tokens/", r.handleMyTokenByID, sub("{id}", "DELETE"))

	// SAML endpoints — wired when a SAML provider is configured.
	if r.samlHandler != nil {
		r.public("/api/v1/auth/saml/metadata", r.samlHandler.HandleMetadata)
		r.public("/api/v1/auth/saml/login", r.samlHandler.HandleLogin,
			takesQuery("GET", queryParam{Name: "returnTo"}))
		r.public("/api/v1/auth/saml/acs", r.samlHandler.HandleACS)
		r.public("/api/v1/auth/saml/slo", r.samlHandler.HandleSLO)
	} else {
		r.public("/api/v1/auth/saml/acs", r.handleNotImplemented)
		r.public("/api/v1/auth/saml/metadata", r.handleNotImplemented)
		r.public("/api/v1/auth/saml/login", r.handleNotImplemented,
			takesQuery("GET", queryParam{Name: "returnTo"}))
		r.public("/api/v1/auth/saml/slo", r.handleNotImplemented)
	}

	// -----------------------------------------------------------------
	// Dashboard endpoints (viewer — any authenticated user)
	// -----------------------------------------------------------------
	r.protect("/api/v1/dashboard/version-distribution", r.handleDashboardVersionDistribution,
		answers("GET", versionDistributionResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "owner"},
			queryParam{Name: "unowned"}))
	r.protect("/api/v1/dashboard/version-distribution/trend", r.handleDashboardVersionDistributionTrend,
		answers("GET", versionDistTrendResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "owner"},
			queryParam{Name: "unowned"}))
	r.protect("/api/v1/dashboard/readiness", r.handleDashboardReadiness,
		answers("GET", readinessResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "owner"},
			queryParam{Name: "unowned"}))
	r.protect("/api/v1/dashboard/readiness/trend", r.handleDashboardReadinessTrend,
		answers("GET", readinessTrendResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "owner"},
			queryParam{Name: "stale"}, queryParam{Name: "unowned"}))
	r.protect("/api/v1/dashboard/complexity/trend", r.handleDashboardComplexityTrend,
		answers("GET", complexityTrendResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}))
	r.protect("/api/v1/dashboard/cookstyle/recompute-trend", r.handleDashboardCookstyleRecomputeTrend,
		answers("GET", cookstyleRecomputeTrendResponse{}))
	r.protect("/api/v1/dashboard/stale/trend", r.handleDashboardStaleTrend,
		answers("GET", staleTrendResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}))
	r.protect("/api/v1/dashboard/deployment/trend", r.handleDashboardDeploymentTrend,
		answers("GET", deploymentTrendResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}))
	r.protect("/api/v1/dashboard/deployment/status", r.handleDashboardDeploymentStatus,
		answers("GET", deploymentStatusResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}))
	r.protect("/api/v1/dashboard/platform-distribution", r.handleDashboardPlatformDistribution,
		answers("GET", platformDistributionResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "owner"},
			queryParam{Name: "unowned"}))
	r.protect("/api/v1/dashboard/cookbook-compatibility", r.handleDashboardCookbookCompatibility,
		answers("GET", cookbookCompatibilityResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "owner"},
			queryParam{Name: "unowned"}))
	r.protect("/api/v1/dashboard/git-repo-compatibility", r.handleDashboardGitRepoCompatibility,
		answers("GET", gitRepoCompatibilityResponse{}),
		takesQuery("GET", queryParam{Name: "owner"}, queryParam{Name: "unowned"}))
	r.protect("/api/v1/dashboard/test-kitchen-compatibility", r.handleDashboardTestKitchenCompatibility,
		answers("GET", testKitchenCompatibilityResponse{}),
		takesQuery("GET", queryParam{Name: "owner"}, queryParam{Name: "unowned"}))
	r.protect("/api/v1/dashboard/cookbook-download-status", r.handleDashboardCookbookDownloadStatus,
		answers("GET", cookbookDownloadStatusResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}))

	// -----------------------------------------------------------------
	// Node endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/nodes", r.handleNodes, paginated(),
		answersPage("GET", nodeResp{}),
		takesQuery("GET", queryParam{Name: "chef_version"}, queryParam{Name: "cookstyle_status"},
			queryParam{Name: "environment"}, queryParam{Name: "kitchen_status"},
			queryParam{Name: "migration_state"}, queryParam{Name: "node_name"},
			queryParam{Name: "organisation"}, queryParam{Name: "owner"},
			queryParam{Name: "platform"}, queryParam{Name: "policy_group"},
			queryParam{Name: "policy_name"}, queryParam{Name: "readiness_filter"},
			queryParam{Name: "ready_to_activate"}, queryParam{Name: "role"},
			queryParam{Name: "stale"}, queryParam{Name: "tags"},
			queryParam{Name: "target_chef_version"}, queryParam{Name: "target_converge_status"},
			queryParam{Name: "target_version"}, queryParam{Name: "unowned"}))
	r.protect("/api/v1/nodes/by-version/", r.handleNodesByVersion,
		sub("{chef_version}"), subAnswers("{chef_version}", "GET", nodesByVersionResponse{}))
	r.protect("/api/v1/nodes/by-cookbook/", r.handleNodesByCookbook,
		sub("{cookbook_name}"),
		subAnswers("{cookbook_name}", "GET", nodesByCookbookResponse{}))
	r.protect("/api/v1/nodes/disks/", r.handleNodeDisks,
		sub("{organisation}/{name}"),
		subAnswers("{organisation}/{name}", "GET", diskResponse{}),
		subTakesQuery("{organisation}/{name}", "GET",
			queryParam{Name: "show_all"}))
	// Capped, not paged: the store call behind this takes a limit and no
	// offset, so page does nothing. See cappedNotPaged.
	r.protect("/api/v1/nodes/runs/", r.handleNodeRuns,
		sub("{organisation}/{name}"), subCappedNotPaged("{organisation}/{name}"),
		subAnswers("{organisation}/{name}", "GET", nodeRunsResponse{}))
	// Node detail: /api/v1/nodes/:organisation/:name — uses a prefix
	// pattern and the handler extracts path segments.
	r.protect("/api/v1/nodes/", r.handleNodeDetail,
		sub("{organisation}/{name}"), sub("{organisation}/{name}/dependency-graph"),
		subAnswers("{organisation}/{name}", "GET", nodeDetailResponse{}),
		subAnswers("{organisation}/{name}/dependency-graph", "GET", nodeDependencyGraphResponse{}))

	// -----------------------------------------------------------------
	// Cookbook endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/cookbooks", r.handleCookbooks, paginated(),
		answersPage("GET", cookbookResp{}),
		takesQuery("GET", queryParam{Name: "target_chef_version"},
			queryParam{Name: "active"}, queryParam{Name: "compatibility"},
			queryParam{Name: "cookstyle_status"}, queryParam{Name: "download_status"},
			queryParam{Name: "name"}, queryParam{Name: "organisation"},
			queryParam{Name: "owner"}, queryParam{Name: "tk_status"},
			queryParam{Name: "unowned"}))
	r.protect("/api/v1/cookbooks/", r.handleCookbookDetail,
		sub("{name}"), sub("{name}/rescan", "POST"), sub("{name}/reset-git", "POST"),
		sub("{name}/platform-coverage"), sub("{name}/committers"),
		sub("{name}/committers/assign", "POST"), sub("{name}/{version}/remediation"),
		subTakes("{name}/committers/assign", "POST", assignCommittersRequest{}),
		subAnswers("{name}", "GET", cookbookDetailResponse{}),
		subAnswers("{name}/platform-coverage", "GET", coverageAPIResponse{}))

	// -----------------------------------------------------------------
	// Role endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/roles", r.handleRoles, paginated(),
		answers("GET", roleListResponse{}),
		takesQuery("GET", queryParam{Name: "name"}, queryParam{Name: "compatibility_status"}, queryParam{Name: "tk_status"}, queryParam{Name: "target_chef_version"},
			queryParam{Name: "organisation"}))
	r.protect("/api/v1/roles/", r.handleRoleDetail,
		sub("{name}"), sub("{name}/dependency-graph"),
		subAnswers("{name}", "GET", datastore.RoleDetail{}),
		subAnswers("{name}/dependency-graph", "GET", roleDependencyGraphResponse{}))

	// -----------------------------------------------------------------
	// Git repo endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/git-repos", r.handleGitRepos, paginated(),
		answersPage("GET", gitRepoResp{}),
		takesQuery("GET", queryParam{Name: "clone_status"}, queryParam{Name: "human_verdict"}, queryParam{Name: "name"}, queryParam{Name: "compatibility"}, queryParam{Name: "cookstyle_status"}, queryParam{Name: "tk_status"}, queryParam{Name: "target_chef_version"},
			queryParam{Name: "has_test_suite"}, queryParam{Name: "owner"},
			queryParam{Name: "unowned"}))
	r.protect("/api/v1/git-repos/", r.handleGitRepoDetail,
		sub("excluded"), sub("{name}"), sub("{name}/exclude", "POST", "DELETE"),
		sub("{name}/rescan", "POST"), sub("{name}/reset", "POST"), sub("{name}/committers"),
		sub("{name}/committers/assign", "POST"), sub("{name}/files"),
		sub("{name}/files/content"), sub("{name}/{version}/remediation"),
		subTakes("{name}/exclude", "POST", excludeGitRepoRequest{}),
		subTakes("{name}/committers/assign", "POST", assignCommittersRequest{}),
		subPaginated("{name}/committers"),
		subAnswers("excluded", "GET", []datastore.GitRepo{}),
		subAnswers("{name}", "GET", gitRepoDetailResponse{}),
		subAnswers("{name}/files", "GET", []fileEntry{}),
		subAnswers("{name}/files/content", "GET", fileContentResponse{}),
		subTakesQuery("{name}/files", "GET", queryParam{Name: "path"}),
		subTakesQuery("{name}/files/content", "GET",
			queryParam{Name: "path", Required: true}))

	// -----------------------------------------------------------------
	// Run events endpoints (viewer) — ingest telemetry over converge_runs.
	// Two tabs (nodes rollup / flat runs) + per-node detail. See
	// journeys/run-history.md and handle_run_events.go.
	// -----------------------------------------------------------------
	r.protect("/api/v1/run-events/nodes", r.handleRunEventNodes, paginated(),
		answersPage("GET", datastore.ConvergeRunListItem{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "status"}, queryParam{Name: "node"}, queryParam{Name: "chef_version"}, queryParam{Name: "cookbook"}, queryParam{Name: "failure_message"},
			queryParam{Name: "since"}, queryParam{Name: "until"}))
	// Measured, not assumed: this one honours per_page and returns no
	// pagination metadata at all, so a caller cannot tell from an answer that
	// it was bounded. Describing it is the only thing that says so today.
	r.protect("/api/v1/run-events/nodes/", r.handleRunEventNodeDetail,
		sub("{organisation}/{name}"), subCappedNotPaged("{organisation}/{name}"),
		subAnswers("{organisation}/{name}", "GET", nodeRunEventsResponse{}))
	r.protect("/api/v1/run-events/runs", r.handleRunEventRuns, paginated(),
		answersPage("GET", datastore.ConvergeRunListItem{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "status"}, queryParam{Name: "node"}, queryParam{Name: "chef_version"}, queryParam{Name: "cookbook"}, queryParam{Name: "failure_message"},
			queryParam{Name: "since"}, queryParam{Name: "until"}))

	// Viewer-readable UI feature flags (so the frontend can hide gated surfaces).
	r.protect("/api/v1/features", r.handleFeatures,
		answers("GET", featuresResponse{}))

	// -----------------------------------------------------------------
	// Remediation endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/remediation/priority", r.handleRemediationPriority, paginated(),
		answers("GET", remediationPriorityResponse{}),
		takesQuery("GET", queryParam{Name: "complexity_label"}, queryParam{Name: "target_chef_version"},
			queryParam{Name: "organisation"}, queryParam{Name: "owner"},
			queryParam{Name: "unowned"}))
	r.protect("/api/v1/remediation/summary", r.handleRemediationSummary,
		answers("GET", remediationSummaryResponse{}),
		takesQuery("GET", queryParam{Name: "complexity_label"}, queryParam{Name: "target_chef_version"},
			queryParam{Name: "organisation"}, queryParam{Name: "owner"},
			queryParam{Name: "unowned"}))

	// -----------------------------------------------------------------
	// Cookstyle cop analysis & classification
	// -----------------------------------------------------------------
	r.protect("/api/v1/cookstyle/cops", r.handleCookstyleCops, paginated(),
		answers("GET", copAggregationResponse{}),
		takesQuery("GET", queryParam{Name: "target_chef_version"},
			queryParam{Name: "source"}, queryParam{Name: "classification"},
			queryParam{Name: "triggered_only"}))
	r.protect("/api/v1/cookstyle/cop-drift", r.handleCookstyleCopDrift,
		answers("GET", analysis.CopDriftReport{}),
		takesQuery("GET", queryParam{Name: "target_chef_version"}))
	r.protect("/api/v1/cookstyle/cops/", r.handleCookstyleCopSubroute,
		sub("{cop_name}/cookbooks"), sub("{cop_name}/classification", "GET", "PUT"),
		subPaginated("{cop_name}/cookbooks"),
		subTakes("{cop_name}/classification", "PUT", classificationPutRequest{}),
		subAnswers("{cop_name}/cookbooks", "GET",
			copCookbookGroupResponse{}, copCookbookResponse{}),
		subTakesQuery("{cop_name}/cookbooks", "GET",
			queryParam{Name: "target_chef_version"}, queryParam{Name: "source"}))
	r.protect("/api/v1/cookstyle/scan-scope", r.handleCookstyleScanScope,
		methods("GET", "PUT", "DELETE"), takes("PUT", scanScopePutRequest{}),
		answers("GET", scanScopeListResponse{}))
	r.protect("/api/v1/cookstyle/custom-cops", r.handleCookstyleCustomCops,
		methods("GET", "POST"), takes("POST", datastore.CustomCopDefinition{}))
	r.protect("/api/v1/cookstyle/custom-cops/", r.handleCookstyleCustomCop,
		sub("{name}", "GET", "PUT", "DELETE"),
		subTakes("{name}", "PUT", datastore.CustomCopDefinition{}))

	// -----------------------------------------------------------------
	// Export endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/exports", r.handleExports, methods("POST"),
		takesQuery("POST", queryParam{Name: "export_type", Required: true}, queryParam{Name: "format", Required: true},
			queryParam{Name: "owner"}, queryParam{Name: "unowned"}))
	r.protect("/api/v1/exports/", r.handleExportStatus,
		sub("{job_id}"), sub("{job_id}/download"))

	// -----------------------------------------------------------------
	// Organisation endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/organisations", r.handleOrganisations,
		answers("GET", organisationListResponse{}))
	r.protect("/api/v1/organisations/", r.handleOrganisationDetail,
		sub("{name}"), subAnswers("{name}", "GET", datastore.Organisation{}))

	// -----------------------------------------------------------------
	// Filter option endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/filters/environments", r.handleFilterEnvironments,
		answers("GET", filterValuesResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "q"}))
	r.protect("/api/v1/filters/roles", r.handleFilterRoles,
		answers("GET", filterValuesResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "q"}))
	r.protect("/api/v1/filters/tags", r.handleFilterTags,
		answers("GET", filterValuesResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "q"}))
	r.protect("/api/v1/filters/policy-names", r.handleFilterPolicyNames,
		answers("GET", filterValuesResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "q"}))
	r.protect("/api/v1/filters/policy-groups", r.handleFilterPolicyGroups,
		answers("GET", filterValuesResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "q"}))
	r.protect("/api/v1/filters/platforms", r.handleFilterPlatforms,
		answers("GET", platformFilterResponse{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "q"}))
	r.protect("/api/v1/filters/target-chef-versions", r.handleFilterTargetChefVersions,
		answers("GET", filterValuesResponse{}))
	r.protect("/api/v1/filters/complexity-labels", r.handleFilterComplexityLabels,
		answers("GET", filterValuesResponse{}))
	// Run events filter options — sourced from converge_runs, NOT the
	// organisations table (so ingest-only DMZ orgs are selectable).
	r.protect("/api/v1/filters/run-organisations", r.handleFilterRunOrganisations,
		answers("GET", filterValuesResponse{}))
	r.protect("/api/v1/filters/run-chef-versions", r.handleFilterRunChefVersions,
		answers("GET", filterValuesResponse{}))

	// -----------------------------------------------------------------
	// Log endpoints (viewer)
	// -----------------------------------------------------------------
	r.protect("/api/v1/logs", r.handleLogs, paginated(),
		answersPage("GET", datastore.LogEntry{}),
		takesQuery("GET", queryParam{Name: "collection_run_id"}, queryParam{Name: "cookbook_name"},
			queryParam{Name: "min_severity"}, queryParam{Name: "organisation"},
			queryParam{Name: "scope"}, queryParam{Name: "severity"},
			queryParam{Name: "since"}, queryParam{Name: "until"}))
	r.protect("/api/v1/logs/collection-runs", r.handleCollectionRuns, paginated(),
		answersPage("GET", datastore.CollectionRunWithOrg{}),
		takesQuery("GET", queryParam{Name: "organisation"}, queryParam{Name: "status"}))
	r.protect("/api/v1/logs/", r.handleLogDetail,
		sub("{id}"), subAnswers("{id}", "GET", datastore.LogEntry{}))

	// -----------------------------------------------------------------
	// Admin endpoints (admin role required)
	// -----------------------------------------------------------------
	// -----------------------------------------------------------------
	// Ownership endpoints (viewer for reads, operator/admin for writes)
	// -----------------------------------------------------------------
	r.protect("/api/v1/owners", r.handleOwners, methods("GET", "POST"),
		takes("POST", createOwnerRequest{}), paginated(),
		answersPage("GET", ownerResp{}))
	r.protect("/api/v1/owners/", r.handleOwners,
		sub("{name}", "GET", "PUT", "DELETE"), sub("{name}/assignments", "GET", "POST"),
		sub("{name}/assignments/{id}", "DELETE"),
		subTakes("{name}", "PUT", updateOwnerRequest{}),
		subTakes("{name}/assignments", "POST", createAssignmentsRequest{}),
		subPaginated("{name}/assignments"),
		subAnswersPage("{name}/assignments", "GET", datastore.OwnershipAssignment{}))
	r.protect("/api/v1/ownership/reassign", r.handleOwnershipEndpoints, methods("POST"),
		takes("POST", reassignOwnershipRequest{}))
	r.protect("/api/v1/ownership/lookup", r.handleOwnershipEndpoints)
	r.protect("/api/v1/ownership/audit-log", r.handleOwnershipEndpoints, paginated(),
		answersPage("GET", datastore.OwnershipAuditEntry{}))
	r.protect("/api/v1/ownership/import", r.handleOwnershipEndpoints, methods("POST"),
		takesForm("POST", formField{Name: "format"}, formField{Name: "file", File: true}))
	// Discovery-driven intake. Registered as exact patterns beside the
	// fixed-header route above, which stays in service unchanged.
	// Every case in handleOwnershipIntake's dispatch switch needs an entry
	// here; TestOwnershipIntakeDispatchCasesAreRouted holds the two in step.
	// The rows an import could not use, as a worklist rather than only an export.
	r.protect("/api/v1/ownership/import/rejections", r.handleOwnershipIntake, paginated())
	r.protect("/api/v1/ownership/import/tables", r.handleOwnershipIntake, methods("POST"),
		takesForm("POST", formField{Name: "db_driver"}, formField{Name: "db_credential"},
			formField{Name: "db_tls_mode"}))
	r.protect("/api/v1/ownership/import/profile", r.handleOwnershipIntake, methods("POST"),
		takesForm("POST", formField{Name: "source_type"}, formField{Name: "file", File: true},
			formField{Name: "delimiter"}, formField{Name: "mapping_id"},
			formField{Name: "db_driver"}, formField{Name: "db_credential"},
			formField{Name: "db_tls_mode"},
			formField{Name: "db_query"}))
	r.protect("/api/v1/ownership/import/preview", r.handleOwnershipIntake, methods("POST"),
		takesForm("POST", formField{Name: "source_type"}, formField{Name: "file", File: true},
			formField{Name: "delimiter"}, formField{Name: "mapping_id"},
			formField{Name: "db_driver"}, formField{Name: "db_credential"},
			formField{Name: "db_tls_mode"},
			formField{Name: "db_query"},
			formField{Name: "field_map"}, formField{Name: "filter_column"},
			formField{Name: "filter_value"}, formField{Name: "create_owners"}))
	r.protect("/api/v1/ownership/import/commit", r.handleOwnershipIntake, methods("POST"),
		takesForm("POST", formField{Name: "source_type"}, formField{Name: "file", File: true},
			formField{Name: "delimiter"}, formField{Name: "mapping_id"},
			formField{Name: "db_driver"}, formField{Name: "db_credential"},
			formField{Name: "db_tls_mode"},
			formField{Name: "db_query"},
			formField{Name: "field_map"}, formField{Name: "filter_column"},
			formField{Name: "filter_value"}, formField{Name: "create_owners"}))
	r.protect("/api/v1/ownership/import/mappings", r.handleOwnershipIntake,
		methods("GET", "POST"), takes("POST", importMappingRequest{}), paginated())
	r.protect("/api/v1/ownership/import/mappings/", r.handleOwnershipIntake,
		sub("{id}", "GET", "PUT", "DELETE"), sub("{id}/run", "POST"),
		subTakes("{id}", "PUT", importMappingRequest{}))
	r.protect("/api/v1/ownership/import/clear", r.handleOwnershipIntake, methods("GET", "POST"))
	r.protect("/api/v1/ownership/aliases", r.handleOwnershipAliases,
		methods("GET", "POST", "DELETE"), takes("POST", createOwnerAliasRequest{}))
	r.protect("/api/v1/ownership/aliases/", r.handleOwnershipAliases,
		sub("{id}", "GET", "POST", "DELETE"),
		subTakes("{id}", "POST", createOwnerAliasRequest{}))
	r.protect("/api/v1/ownership/aliases/import", r.handleOwnershipAliasesImport,
		methods("POST"),
		takesForm("POST", formField{Name: "format"}, formField{Name: "file", File: true}))
	r.protect("/api/v1/ownership/aliases/suggest", r.handleOwnershipAliasSuggest,
		takesQuery("GET", queryParam{Name: "q", Required: true}, queryParam{Name: "limit"}))
	// Identity management: recognising a duplicate person, and folding one
	// into another so the correction survives the next ingest.
	r.protect("/api/v1/ownership/duplicates", r.handleOwnershipDuplicates, paginated(),
		takesQuery("GET", queryParam{Name: "min_similarity"}))
	r.protect("/api/v1/ownership/duplicates/rescan", r.handleOwnershipDuplicatesRescan, methods("POST"))
	r.protect("/api/v1/ownership/duplicates/dismiss", r.handleOwnershipDuplicatesDismiss,
		methods("POST"), takes("POST", dismissDuplicateRequest{}))
	r.protect("/api/v1/ownership/duplicates/dismissed", r.handleOwnershipDuplicatesDismissed)
	r.protect("/api/v1/ownership/duplicates/restore", r.handleOwnershipDuplicatesRestore,
		methods("POST"), takes("POST", restoreDuplicateRequest{}))
	r.protect("/api/v1/ownership/merge", r.handleOwnershipMerge, methods("POST"),
		takes("POST", mergeOwnersRequest{}))

	// The failure register: a person's verdict on whether a cookbook actually
	// works, which outranks CookStyle and Test Kitchen. Reads are viewer;
	// recording, revising and resolving need operator or admin.
	r.protect("/api/v1/failure-register", r.handleFailureRegister, methods("GET", "POST"),
		takes("POST", recordVerdictRequest{}), paginated(),
		answers("GET", failureRegisterListResponse{}))
	r.protect("/api/v1/failure-register/", r.handleFailureRegister,
		sub("{id}", "GET", "PATCH"), sub("{id}/resolve", "POST"),
		sub("subject/{subject_type}/{subject_name}"),
		subTakes("{id}", "PATCH", reviseFailureEntryRequest{}),
		subTakes("{id}/resolve", "POST", resolveFailureEntryRequest{}),
		subAnswers("{id}", "GET", datastore.FailureRegisterEntry{}),
		subAnswers("subject/{subject_type}/{subject_name}", "GET",
			failureRegisterHistoryResponse{}))

	// -----------------------------------------------------------------
	// Admin endpoints (admin role required)
	// -----------------------------------------------------------------
	r.adminOnly("/api/v1/admin/credentials", r.handleCredentials, methods("GET", "POST"),
		takes("POST", createCredentialRequest{}), paginated(),
		answersPage("GET", credentialResponse{}))
	r.adminOnly("/api/v1/admin/credentials/", r.handleCredentials,
		sub("{name}", "PUT", "DELETE"), sub("{name}/test", "POST"),
		subTakes("{name}", "PUT", rotateCredentialRequest{}))
	r.adminOnly("/api/v1/admin/config/organisations", r.handleAdminConfigOrganisations,
		methods("GET", "PUT"), takes("PUT", []config.Organisation{}),
		answers("GET", []config.Organisation{}),
		answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/config/collection", r.handleAdminConfigCollection,
		methods("GET", "PUT"), takes("PUT", config.CollectionConfig{}),
		answers("GET", config.CollectionConfig{}),
		answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/config/target-versions", r.handleAdminConfigTargetVersions,
		methods("GET", "PUT"), takes("PUT", []string{}),
		answers("GET", []string{}),
		answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/config/git-urls", r.handleAdminConfigGitURLs,
		methods("GET", "PUT"), takes("PUT", []string{}),
		answers("GET", []string{}),
		answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/config/concurrency", r.handleAdminConfigConcurrency,
		methods("GET", "PUT"), takes("PUT", config.ConcurrencyConfig{}),
		answers("GET", config.ConcurrencyConfig{}),
		answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/config/logging", r.handleAdminConfigLogging,
		methods("GET", "PUT"), takes("PUT", config.LoggingConfig{}),
		answers("GET", config.LoggingConfig{}),
		answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/config/analysis-tools", r.handleAdminConfigAnalysisTools,
		methods("GET", "PUT"), takes("PUT", config.AnalysisToolsConfig{}),
		answers("GET", analysisToolsResponse{}),
		answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/config/test-kitchen", r.handleAdminConfigTestKitchen,
		methods("GET", "PUT", "DELETE"), takes("PUT", config.TestKitchenConfig{}),
		answers("GET", config.TestKitchenConfig{}),
		answers("PUT", putConfigResponse{}))
	// One body, read into three types: the settings section itself, plus the
	// certificate and ACME credentials that are routed to encrypted storage
	// rather than into the section. A caller has to be told about all three.
	r.adminOnly("/api/v1/admin/config/server", r.handleAdminConfigServer,
		methods("GET", "PUT"), takes("PUT", config.ServerConfig{},
			dbCertKeySubmission{}, acmeRoute53CredSubmission{}),
		answers("GET", serverConfigResponse{}), answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/config/server/generate-csr", r.handleAdminConfigServerGenerateCSR,
		methods("POST"), takes("POST", generateCSRRequest{}))
	r.adminOnly("/api/v1/admin/config/auth", r.handleAdminConfigAuth,
		methods("GET", "PUT"), takes("PUT", config.AuthConfig{}),
		answers("GET", config.AuthConfig{}),
		answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/config/exports", r.handleAdminConfigExports,
		methods("GET", "PUT"), takes("PUT", config.ExportsConfig{}),
		answers("GET", config.ExportsConfig{}),
		answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/config/readiness", r.handleAdminConfigReadiness,
		methods("GET", "PUT"), takes("PUT", config.ReadinessConfig{}),
		answers("GET", config.ReadinessConfig{}),
		answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/config/backup", r.handleAdminConfigBackup,
		methods("GET", "PUT"), takes("PUT", config.BackupConfig{}),
		answers("GET", config.BackupConfig{}),
		answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/config/ingest", r.handleAdminConfigIngest,
		methods("GET", "PUT"), takes("PUT", config.IngestConfig{}),
		answers("GET", config.IngestConfig{}),
		answers("PUT", putConfigResponse{}))
	r.adminOnly("/api/v1/admin/saml/generate-keypair", r.handleSAMLGenerateKeypair, methods("POST"))
	r.adminOnly("/api/v1/admin/saml/sp-certificate", r.handleSAMLGetCertificate)
	r.adminOnly("/api/v1/admin/saml/endpoints", r.handleSAMLEndpoints)
	r.adminOnly("/api/v1/admin/platform-mapping/status", r.handlePlatformMappingStatus)

	// Kitchen analysis endpoints (viewer — any authenticated user)
	r.protect("/api/v1/kitchen/analysis/summary", r.handleKitchenAnalysisSummary,
		answers("GET", datastore.KitchenAnalysisSummary{}))
	r.protect("/api/v1/kitchen/analysis/platforms", r.handleKitchenAnalysisPlatforms,
		takesQuery("GET", queryParam{Name: "min_count"}, queryParam{Name: "os_family"}),
		answers("GET", []datastore.KitchenDiscoveredPlatform{}))
	r.protect("/api/v1/kitchen/analysis/cookbooks", r.handleKitchenAnalysisCookbooksRouter,
		answers("GET", []datastore.KitchenAnalysisResult{}))
	r.protect("/api/v1/kitchen/analysis/cookbooks/", r.handleKitchenAnalysisCookbooksRouter,
		sub("{name}"), subAnswers("{name}", "GET", datastore.KitchenAnalysisResult{}))

	// Kitchen analysis trigger (operator — operational action)
	r.operatorOnly("/api/v1/kitchen/analysis/trigger", r.handleKitchenAnalysisTrigger, methods("POST"))

	// -----------------------------------------------------------------
	// Hypervisor endpoints (viewer for reads, operator for operational,
	// admin for destructive/config)
	// -----------------------------------------------------------------
	r.protect("/api/v1/hypervisor/templates", r.handleHypervisorTemplates,
		answers("GET", []hypervisor.Template{}))
	r.protect("/api/v1/hypervisor/vms", r.handleHypervisorVMs,
		takesQuery("GET", queryParam{Name: "status"}),
		answers("GET", []datastore.TrackedVM{}))
	r.operatorOnly("/api/v1/hypervisor/vms/", r.handleHypervisorDestroyVM,
		sub("{id}/destroy", "DELETE", "POST"))
	r.operatorOnly("/api/v1/hypervisor/cleanup", r.handleHypervisorCleanup, methods("POST"))
	r.adminOnly("/api/v1/admin/hypervisor/test-connection", r.handleHypervisorTestConnection, methods("POST"))
	r.operatorOnly("/api/v1/kitchen/orphan-sweep", r.handleOrphanSweep, methods("POST"),
		takesQuery("POST", queryParam{Name: "dry_run"}))

	// -----------------------------------------------------------------
	// Node Kitchen endpoints (operator for triggers)
	// -----------------------------------------------------------------
	r.operatorOnly("/api/v1/kitchen/node-run", r.handleNodeKitchenTrigger, methods("POST"),
		takes("POST", nodekitchen.RunRequest{}))
	r.protect("/api/v1/kitchen/node-runs", r.handleNodeKitchenRuns,
		takesQuery("GET", queryParam{Name: "org", Required: true}, queryParam{Name: "node"}))
	r.protect("/api/v1/kitchen/node-runs/", r.handleNodeKitchenRunDetail,
		sub("{id}", "GET", "DELETE"))

	// -----------------------------------------------------------------
	// Kitchen Batch endpoints (operator for management)
	// -----------------------------------------------------------------
	r.operatorOnly("/api/v1/kitchen/batches", r.handleKitchenBatches, methods("GET", "POST"),
		takes("POST", createBatchRequest{}))
	r.protect("/api/v1/kitchen/batches/", r.handleKitchenBatchDetail,
		sub("{id}", "GET", "PUT", "DELETE"), sub("{id}/progress"), sub("{id}/instances"),
		sub("{id}/run", "POST"), sub("{id}/cancel", "POST"),
		subTakes("{id}", "PUT", createBatchRequest{}))

	// -----------------------------------------------------------------
	// Git Kitchen endpoints (operator for triggers)
	// -----------------------------------------------------------------
	r.protect("/api/v1/kitchen/git/instances", r.handleGitKitchenInstances,
		takesQuery("GET", queryParam{Name: "repo", Required: true}))
	r.protect("/api/v1/kitchen/git/results", r.handleGitKitchenResults,
		takesQuery("GET", queryParam{Name: "repo"}))
	r.operatorOnly("/api/v1/kitchen/git/run", r.handleGitKitchenRun, methods("POST"),
		takes("POST", gitKitchenRunRequest{}))
	r.operatorOnly("/api/v1/kitchen/git/run-all", r.handleGitKitchenRunAll, methods("POST"),
		takes("POST", gitKitchenRunAllRequest{}))
	r.protect("/api/v1/kitchen/git/exclusions", r.handleKitchenExclusions, methods("GET", "POST"),
		takes("POST", createKitchenExclusionRequest{}),
		takesQuery("GET", queryParam{Name: "repo"}))
	r.protect("/api/v1/kitchen/git/exclusions/", r.handleDeleteKitchenExclusion,
		sub("{id}", "DELETE"))

	// Kitchen queue endpoints
	r.protect("/api/v1/kitchen/queue", r.handleKitchenQueueList,
		takesQuery("GET", queryParam{Name: "repo"}, queryParam{Name: "status"},
			queryParam{Name: "type"}))
	r.protect("/api/v1/kitchen/queue/stats", r.handleKitchenQueueStats)
	r.protect("/api/v1/kitchen/queue/", r.handleKitchenQueueRouting,
		sub("{id}"), sub("{id}/cancel", "POST"), sub("{id}/retry", "POST"),
		subAnswers("{id}", "GET", datastore.KitchenQueueItem{}))

	// -----------------------------------------------------------------
	// Saved filter endpoints — any authenticated user manages their own;
	// mutations are owner-only, enforced in the handler.
	// -----------------------------------------------------------------
	r.protect("/api/v1/saved-filters", r.handleSavedFilters, methods("GET", "POST"),
		takes("POST", savedFilterCreateRequest{}),
		answers("GET", []datastore.SavedFilter{}),
		takesQuery("GET", queryParam{Name: "view"}))
	r.protect("/api/v1/saved-filters/", r.handleSavedFilter,
		sub("{id}", "PATCH", "DELETE"),
		subTakes("{id}", "PATCH", savedFilterUpdateRequest{}))

	if r.authStore != nil {
		r.adminOnly("/api/v1/admin/users", r.handleAdminUsers, methods("GET", "POST"),
			takes("POST", createUserRequest{}), paginated())
		r.adminOnly("/api/v1/admin/users/", r.handleAdminUsers,
			sub("{username}", "PUT", "DELETE"), sub("{username}/password", "PUT"),
			subTakes("{username}", "PUT", updateUserRequest{}),
			subTakes("{username}/password", "PUT", resetPasswordRequest{}))
	} else {
		r.adminOnly("/api/v1/admin/users", r.handleNotImplemented, methods("GET", "POST"),
			takes("POST", createUserRequest{}), paginated())
		r.adminOnly("/api/v1/admin/users/", r.handleNotImplemented,
			sub("{username}", "PUT", "DELETE"), sub("{username}/password", "PUT"),
			subTakes("{username}", "PUT", updateUserRequest{}),
			subTakes("{username}/password", "PUT", resetPasswordRequest{}))
	}
	r.adminOnly("/api/v1/admin/restart", r.handleAdminRestart, methods("POST"))
	r.adminOnly("/api/v1/admin/status", r.handleAdminStatus)
	r.adminOnly("/api/v1/admin/system-health", r.handleAdminSystemHealth)
	r.adminOnly("/api/v1/admin/diagnostic-bundle", r.handleDiagnosticBundle,
		takesQuery("GET", queryParam{Name: "include_depth_stats"},
			queryParam{Name: "include_identifiers"}, queryParam{Name: "log_days"}))
	r.adminOnly("/api/v1/admin/backups", r.handleAdminBackups, methods("GET", "POST"))
	r.adminOnly("/api/v1/admin/backups/", r.handleAdminBackups,
		sub("{id}", "GET", "DELETE"), sub("{id}/restore", "POST"),
		subTakes("{id}/restore", "POST", restoreRequest{}))
	r.adminOnly("/api/v1/admin/backups/status", r.handleAdminBackupStatus)
	r.operatorOnly("/api/v1/admin/rescan-all-cookstyle", r.handleAdminRescanAllCookstyle, methods("POST"))
	r.adminOnly("/api/v1/admin/platform-display-names", r.handlePlatformDisplayNames,
		methods("GET", "PUT"), takes("PUT", []platform.DisplayNameMapping{}))
	r.adminOnly("/api/v1/admin/platform-display-names/reset", r.handlePlatformDisplayNamesReset, methods("POST"))

	// Performance diagnostics (admin-only, gated on config + recorder).
	if r.cfg.Performance.IsEnabled() && r.recorder != nil {
		r.adminOnly("/api/v1/admin/performance", r.handlePerformance, methods("GET", "DELETE"))
		r.adminOnly("/api/v1/admin/performance/db", r.handlePerformanceDB, methods("GET", "DELETE"))
	}

	// Database maintenance (always available to admins).
	r.adminOnly("/api/v1/admin/performance/vacuum", r.handleVacuumFull, methods("POST"))

	// EXPLAIN runner (always available to admins — does not depend on the request
	// recorder or performance.enabled).
	r.adminOnly("/api/v1/admin/performance/explain", r.handleExplain, methods("POST"),
		takes("POST", explainRequest{}))
	r.adminOnly("/api/v1/admin/performance/explain/catalog", r.handleExplainCatalog)

	// pprof endpoints (admin-only, only registered when explicitly enabled).
	if r.cfg.Performance.PprofEnabled {
		r.adminOnly("/debug/pprof/", func(w http.ResponseWriter, req *http.Request) {
			pprof.Index(w, req)
		})
		r.adminOnly("/debug/pprof/cmdline", func(w http.ResponseWriter, req *http.Request) {
			pprof.Cmdline(w, req)
		})
		r.adminOnly("/debug/pprof/profile", func(w http.ResponseWriter, req *http.Request) {
			pprof.Profile(w, req)
		})
		r.adminOnly("/debug/pprof/symbol", func(w http.ResponseWriter, req *http.Request) {
			pprof.Symbol(w, req)
		})
		r.adminOnly("/debug/pprof/trace", func(w http.ResponseWriter, req *http.Request) {
			pprof.Trace(w, req)
		})
		r.logf("WARN", "pprof endpoints enabled at /debug/pprof/ — do not use in production without auth")
	}

	// -----------------------------------------------------------------
	// Frontend SPA fallback — serves index.html for client-side routing.
	// Public so the login page can be served without a session.
	// -----------------------------------------------------------------
	r.public("/", r.handleFrontendFallback)
}

// webSocketOpts builds the WebSocketHandler options from the loaded config.
func (r *Router) webSocketOpts() []WebSocketHandlerOption {
	// Pull timeouts live so a server.websocket.* save applies to connections
	// opened afterwards without a restart (configuration-live-reload.md:
	// subsystem). secondsToDuration maps an unset (0) field to the default.
	opts := []WebSocketHandlerOption{
		WithWebSocketConfigFunc(func() WebSocketConfig {
			ws := r.liveConfig().Server.WebSocket
			return WebSocketConfig{
				WriteTimeout: secondsToDuration(ws.WriteTimeoutSeconds),
				PingInterval: secondsToDuration(ws.PingIntervalSeconds),
				PongTimeout:  secondsToDuration(ws.PongTimeoutSeconds),
			}
		}),
	}
	if r.logger != nil {
		opts = append(opts, WithWebSocketLogger(r.logger))
	}
	return opts
}

// -----------------------------------------------------------------
// Health & version handlers
// -----------------------------------------------------------------

func (r *Router) handleHealth(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Health endpoint requires GET.")
		return
	}
	if err := r.db.Ping(req.Context()); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"unhealthy","error":%q}`, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"status":            "healthy",
		"version":           r.version,
		"schema_version":    r.schemaVersion,
		"websocket_enabled": r.cfg.Server.WebSocket.IsEnabled(),
		"websocket_clients": r.hub.ClientCount(),
	})
}

// versionResponse is what the version call answers with. Named rather than
// assembled inline so the description can be reflected off it — see answers().
type versionResponse struct {
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version"`
}

func (r *Router) handleVersion(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Version endpoint requires GET.")
		return
	}
	WriteJSON(w, http.StatusOK, versionResponse{
		Version:       r.version,
		SchemaVersion: r.schemaVersion,
	})
}

// -----------------------------------------------------------------
// Placeholder handler for unimplemented endpoints
// -----------------------------------------------------------------

func (r *Router) handleNotImplemented(w http.ResponseWriter, req *http.Request) {
	WriteError(w, http.StatusNotImplemented, "not_implemented",
		fmt.Sprintf("Endpoint %s %s is not yet implemented.", req.Method, req.URL.Path))
}

// -----------------------------------------------------------------
// Frontend SPA fallback
// -----------------------------------------------------------------

func (r *Router) handleFrontendFallback(w http.ResponseWriter, req *http.Request) {
	// If the path starts with /api/ but didn't match any registered route,
	// say the endpoint is missing rather than falling through to the SPA.
	//
	// This is checked before the method, and the order is the whole point: an
	// unrouted POST answered with "method not allowed" describes an endpoint
	// that exists and refuses the verb, which sends the reader hunting for a
	// permissions or verb problem instead of a missing registration. That is
	// exactly how an unregistered import path read as a permissions failure.
	if len(req.URL.Path) >= 5 && req.URL.Path[:5] == "/api/" {
		WriteNotFound(w, fmt.Sprintf("API endpoint %s not found.", req.URL.Path))
		return
	}

	// Everything else is the single-page app, which only serves documents —
	// so here a non-GET/HEAD really is a method error.
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Method not allowed.")
		return
	}

	// If the frontend FS is available, serve static assets from it.
	// For paths that don't match a real file, serve index.html so that
	// React Router can handle client-side routes.
	if r.frontendFS != nil {
		r.serveFrontendAsset(w, req)
		return
	}

	// No frontend built — return a plain-text placeholder.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "Chef Migration Metrics %s\n\nFrontend not yet built. API available at /api/v1/\n", r.version)
}

// serveFrontendAsset serves a file from the frontend FS if it exists,
// otherwise falls back to index.html for SPA client-side routing.
func (r *Router) serveFrontendAsset(w http.ResponseWriter, req *http.Request) {
	// Clean and strip leading slash to get the FS-relative path.
	p := strings.TrimPrefix(path.Clean(req.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}

	// Try to open the requested path as a real file.
	f, err := r.frontendFS.Open(p)
	if err == nil {
		defer func() { _ = f.Close() }()
		// Check it's not a directory — if it is, fall through to index.html.
		if stat, statErr := f.Stat(); statErr == nil && !stat.IsDir() {
			http.ServeFileFS(w, req, r.frontendFS, p)
			return
		}
	}

	// Path didn't match a static asset — serve index.html for SPA routing.
	http.ServeFileFS(w, req, r.frontendFS, "index.html")
}

// -----------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------

// secondsToDuration converts an integer number of seconds to a time.Duration.
// If n is zero or negative, it returns a sensible default of 10 seconds.
func secondsToDuration(n int) time.Duration {
	if n <= 0 {
		return 10 * time.Second
	}
	return time.Duration(n) * time.Second
}

// liveConfig returns the current live config from the ConfigHolder when
// available, otherwise falls back to the static config set at construction.
func (r *Router) liveConfig() *config.Config {
	if r.configHolder != nil {
		return r.configHolder.Get()
	}
	return r.cfg
}

// installPathForNode returns the configured install path based on node platform.
func (r *Router) installPathForNode(platform string) string {
	cfg := r.liveConfig()
	if strings.EqualFold(platform, "windows") {
		return cfg.Readiness.InstallPathWindows
	}
	return cfg.Readiness.InstallPathLinux
}

// buildHypervisor returns a hypervisor client. If a static client was
// injected via WithHypervisor (e.g. in tests), it is returned directly.
// Otherwise, a fresh client is built from the current live config and
// resolved credentials — meaning config changes via the UI take effect
// immediately without a restart.
func (r *Router) buildHypervisor(ctx context.Context) (hypervisor.Hypervisor, error) {
	if r.hypervisor != nil {
		return r.hypervisor, nil
	}
	return BuildHypervisorFromConfig(ctx, r.liveConfig().AnalysisTools.TestKitchen, r.credResolver)
}

// BuildHypervisorFromConfig builds a hypervisor client from the given live
// Test Kitchen config and resolves its driver secrets via resolver. Returns
// (nil, nil) when no hypervisor type is configured. Shared by the router's
// on-demand client builder and the scheduled orphan-sweep ticker so both
// resolve credentials identically from live config — config changes take
// effect with no restart.
func BuildHypervisorFromConfig(ctx context.Context, tk config.TestKitchenConfig, resolver *secrets.CredentialResolver) (hypervisor.Hypervisor, error) {
	hypType := tk.EffectiveHypervisorType()
	if hypType == "" {
		return nil, nil
	}

	if resolver == nil {
		return nil, fmt.Errorf("credential resolver not configured")
	}

	resolvedSecrets := make(map[string]string, len(tk.DriverSecrets))
	for key, credName := range tk.DriverSecrets {
		resolved, err := resolver.Resolve(ctx, secrets.CredentialSource{
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

// logf logs a formatted message if a logger is configured.
func (r *Router) logf(level, format string, args ...any) {
	if r.logger != nil {
		r.logger(level, fmt.Sprintf(format, args...))
	}
}

// defaultTargetVersion returns the highest configured target Chef version
// (by semver comparison), or an empty string if none are configured.
// Handlers use this as the fallback when no target_chef_version query
// parameter is supplied.
func (r *Router) defaultTargetVersion() string {
	return r.liveConfig().TargetChefVersion
}
