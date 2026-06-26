// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/tkstatus"
)

// ---------------------------------------------------------------------------
// Cookbook compatibility status constants
// ---------------------------------------------------------------------------

const (
	// StatusCompatible means Test Kitchen passed for this cookbook × target.
	StatusCompatible = "compatible"

	// StatusCompatibleCookstyleOnly means CookStyle passed but no Test
	// Kitchen result exists. The cookbook has no detected errors but has
	// not been integration-tested.
	StatusCompatibleCookstyleOnly = "compatible_cookstyle_only"

	// StatusIncompatible means Test Kitchen failed or CookStyle reported
	// error/fatal offenses.
	StatusIncompatible = "incompatible"

	// StatusUntested means no test or scan results exist for this cookbook
	// × target version.
	StatusUntested = "untested"
)

// Compatibility source constants — record how the verdict was determined.
const (
	SourceTestKitchen = "test_kitchen"
	SourceCookstyle   = "cookstyle"
	SourceNone        = "none"
)

// Multi-source verdict constants — identify which specific source produced a verdict.
const (
	SourceServerCookstyle = "server_cookstyle"
	SourceGitCookstyle    = "git_cookstyle"
	SourceGitTestKitchen  = "git_test_kitchen"
)

// CookbookSourceVerdict records the compatibility result from one source.
type CookbookSourceVerdict struct {
	Source          string `json:"source"`               // "server_cookstyle", "git_cookstyle", "git_test_kitchen"
	Status          string `json:"status"`               // "compatible", "incompatible", "untested"
	Version         string `json:"version,omitempty"`    // server version or "HEAD" for git
	CommitSHA       string `json:"commit_sha,omitempty"` // git HEAD SHA (git sources only)
	ComplexityScore int    `json:"complexity_score,omitempty"`
	ComplexityLabel string `json:"complexity_label,omitempty"`
}

// ---------------------------------------------------------------------------
// BlockingCookbook — one entry in the blocking_cookbooks JSONB array
// ---------------------------------------------------------------------------

// BlockingCookbook describes a single cookbook that is preventing a node from
// being ready for upgrade.
type BlockingCookbook struct {
	Name            string                  `json:"name"`
	Version         string                  `json:"version"`
	Reason          string                  `json:"reason"`           // StatusIncompatible or StatusUntested
	Source          string                  `json:"source"`           // SourceTestKitchen, SourceCookstyle, or SourceNone
	ComplexityScore int                     `json:"complexity_score"` // 0 if no complexity data
	ComplexityLabel string                  `json:"complexity_label"` // "" if no complexity data
	Verdicts        []CookbookSourceVerdict `json:"verdicts,omitempty"`
}

// ---------------------------------------------------------------------------
// ReadinessResult — the output for one node × target version
// ---------------------------------------------------------------------------

// ReadinessResult holds the evaluation outcome for a single node × target
// Chef Client version pair.
type ReadinessResult struct {
	OrganisationName       string
	NodeName               string
	TargetChefVersion      string
	IsReady                bool
	AllCookbooksCompatible bool
	SufficientDiskSpace    *bool // nil = unknown
	BlockingCookbooks      []BlockingCookbook
	ReviewCookbooks        []BlockingCookbook // needs-review cookbooks (toggle-on only)
	AvailableDiskMB        *int               // nil = unknown
	RequiredDiskMB         int
	StaleData              bool
	// Status is the node rollup verdict: StatusReady / StatusNeedsReview /
	// StatusBlocked. It mirrors the CookStyle rollup vocabulary and is the
	// authoritative 3-state readiness signal. IsReady = (Status == StatusReady).
	Status          string
	CookstyleStatus string // "passed", "failed", "unknown"
	KitchenStatus   string // "passed", "failed", "partial", "unknown"
	EvaluatedAt     time.Time
}

// ---------------------------------------------------------------------------
// ReadinessDataStore — interface for testability
// ---------------------------------------------------------------------------

// ReadinessDataStore is the subset of datastore.DB methods needed by the
// readiness evaluator. Accepting an interface allows tests to inject fakes
// without a live PostgreSQL database.
type ReadinessDataStore interface {
	// Node snapshots
	ListNodeSnapshotsByOrganisation(ctx context.Context, organisationID string) ([]datastore.NodeSnapshot, error)

	// Server cookbooks — list all for an org (used to build the cookbook ID map)
	ListServerCookbooksByOrganisation(ctx context.Context, organisationName string) ([]datastore.ServerCookbook, error)

	// Git repos — used to resolve cookbook name to git repo for CookStyle cross-lookup
	GetGitRepoByName(ctx context.Context, name string) (datastore.GitRepo, error)

	// CookStyle results (server cookbook)
	GetServerCookbookCookstyleResult(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*datastore.ServerCookbookCookstyleResult, error)

	// CookStyle results (git repo)
	GetGitRepoCookstyleResult(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) (*datastore.GitRepoCookstyleResult, error)

	// Server cookbook complexity
	GetServerCookbookComplexity(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*datastore.ServerCookbookComplexity, error)

	// Git repo complexity
	GetGitRepoComplexity(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) (*datastore.GitRepoComplexity, error)

	// Persistence
	UpsertNodeReadiness(ctx context.Context, p datastore.UpsertNodeReadinessParams) (*datastore.NodeReadiness, error)

	// Bulk-load methods — used by EvaluateOrganisation to replace N+1 queries
	// with a small number of bulk queries + in-memory lookups.
	ListGitRepos(ctx context.Context) ([]datastore.GitRepo, error)
	ListGitRepoCookstyleResultsByTargetVersions(ctx context.Context, targetChefVersions []string) ([]datastore.GitRepoCookstyleResult, error)
	ListServerCookbookCookstyleResultsByOrganisationAndVersions(ctx context.Context, organisationID string, targetChefVersions []string) ([]datastore.ServerCookbookCookstyleResult, error)
	ListServerCookbookComplexities(ctx context.Context, organisationID string, targetChefVersions []string) ([]datastore.ServerCookbookComplexity, error)
	ListGitRepoComplexities(ctx context.Context, targetChefVersions []string) ([]datastore.GitRepoComplexity, error)

	// Bulk-load git Test Kitchen aggregate counts
	ListGitKitchenCountsByTargetVersions(ctx context.Context, targetChefVersions []string) (map[string]tkstatus.Counts, error)
}

// ---------------------------------------------------------------------------
// readinessCache — pre-loaded lookup data for in-memory evaluation
// ---------------------------------------------------------------------------

// readinessCache holds all test/scan/complexity data needed to evaluate
// cookbook compatibility, loaded in bulk at the start of EvaluateOrganisation.
// All maps are built once and shared read-only across goroutines (maps are
// safe for concurrent reads).
type readinessCache struct {
	gitRepos         map[string]datastore.GitRepo                        // name → repo
	gitCSResults     map[string]*datastore.GitRepoCookstyleResult        // gitRepoID|target → result
	serverCSResults  map[string]*datastore.ServerCookbookCookstyleResult // cookbookID|target → result
	serverComplexity map[string]*datastore.ServerCookbookComplexity      // cookbookID|target → complexity
	gitComplexity    map[string]*datastore.GitRepoComplexity             // gitRepoID|target → complexity
	gitTKStatuses    map[string]string                                   // repoName|target → "passed"/"failed"/"partial"
	// reviewBlocksReadiness mirrors the live readiness toggle, snapshotted at
	// cache-build time so every node in the batch evaluates against one
	// consistent value. Off: Review cookbooks resolve to compatible.
	reviewBlocksReadiness bool
}

// cacheKey builds a lookup key from two components (e.g. ID + target version).
func cacheKey(a, b string) string {
	return a + "|" + b
}

// buildReadinessCache loads all lookup data needed for readiness evaluation
// in bulk. Returns an error if any bulk query fails — partial caches are not
// used because they would produce incorrect compatibility verdicts.
func buildReadinessCache(
	ctx context.Context,
	db ReadinessDataStore,
	organisationID string,
	targetChefVersions []string,
	reviewBlocksReadiness bool,
) (*readinessCache, error) {
	cache := &readinessCache{
		gitRepos:              make(map[string]datastore.GitRepo),
		gitCSResults:          make(map[string]*datastore.GitRepoCookstyleResult),
		serverCSResults:       make(map[string]*datastore.ServerCookbookCookstyleResult),
		serverComplexity:      make(map[string]*datastore.ServerCookbookComplexity),
		gitComplexity:         make(map[string]*datastore.GitRepoComplexity),
		gitTKStatuses:         make(map[string]string),
		reviewBlocksReadiness: reviewBlocksReadiness,
	}

	// 1. Git repos (all — small table)
	gitRepos, err := db.ListGitRepos(ctx)
	if err != nil {
		return nil, fmt.Errorf("readiness: bulk-loading git repos: %w", err)
	}
	for _, gr := range gitRepos {
		cache.gitRepos[gr.Name] = gr
	}

	// 2. Git repo CookStyle results
	gitCSResults, err := db.ListGitRepoCookstyleResultsByTargetVersions(ctx, targetChefVersions)
	if err != nil {
		return nil, fmt.Errorf("readiness: bulk-loading git CookStyle results: %w", err)
	}
	for i := range gitCSResults {
		r := &gitCSResults[i]
		cache.gitCSResults[cacheKey(r.GitRepoName, r.TargetChefVersion)] = r
	}

	// 3. Server cookbook CookStyle results (org-scoped, includes NULL target versions)
	serverCSResults, err := db.ListServerCookbookCookstyleResultsByOrganisationAndVersions(ctx, organisationID, targetChefVersions)
	if err != nil {
		return nil, fmt.Errorf("readiness: bulk-loading server CookStyle results: %w", err)
	}
	for i := range serverCSResults {
		r := &serverCSResults[i]
		cache.serverCSResults[cacheKey(r.OrganisationName+"/"+r.CookbookName+"/"+r.CookbookVersion, r.TargetChefVersion)] = r
	}

	// 4. Server cookbook complexity (org-scoped)
	serverComplexities, err := db.ListServerCookbookComplexities(ctx, organisationID, targetChefVersions)
	if err != nil {
		return nil, fmt.Errorf("readiness: bulk-loading server complexities: %w", err)
	}
	for i := range serverComplexities {
		c := &serverComplexities[i]
		cache.serverComplexity[cacheKey(c.OrganisationName+"/"+c.CookbookName+"/"+c.CookbookVersion, c.TargetChefVersion)] = c
	}

	// 5. Git repo complexity
	gitComplexities, err := db.ListGitRepoComplexities(ctx, targetChefVersions)
	if err != nil {
		return nil, fmt.Errorf("readiness: bulk-loading git complexities: %w", err)
	}
	for i := range gitComplexities {
		c := &gitComplexities[i]
		cache.gitComplexity[cacheKey(c.GitRepoName, c.TargetChefVersion)] = c
	}

	// 6. Git Test Kitchen aggregate statuses (convert counts → status at fill time)
	gitTKCounts, err := db.ListGitKitchenCountsByTargetVersions(ctx, targetChefVersions)
	if err != nil {
		return nil, fmt.Errorf("readiness: bulk-loading git TK counts: %w", err)
	}
	for k, c := range gitTKCounts {
		if s := c.Status(); s != "" {
			cache.gitTKStatuses[k] = s
		}
	}

	return cache, nil
}

// ---------------------------------------------------------------------------
// ReadinessEvaluator
// ---------------------------------------------------------------------------

// ReadinessEvaluator computes per-node per-target-version upgrade readiness.
type ReadinessEvaluator struct {
	db                      ReadinessDataStore
	logger                  *logging.Logger
	concurrency             int
	installPathLinux        string
	installPathWindows      string
	installSizeMBLinux      int
	installSizeMBWindows    int
	minRemainingFreePercent int
	reviewBlocksReadiness   bool
	// configFn, when set, returns the current readiness config dynamically.
	// This allows the evaluator to pick up config changes without a restart.
	configFn func() ReadinessEvalConfig
	// concurrencyFn, when set, returns the live max-parallel-evaluations value,
	// read at the start of each evaluation so concurrency.readiness_evaluation
	// applies at the next run without a restart. Falls back to baked concurrency.
	concurrencyFn func() int
}

// ReadinessEvaluatorOption configures a ReadinessEvaluator.
type ReadinessEvaluatorOption func(*ReadinessEvaluator)

// WithReadinessDataStore overrides the datastore (for testing).
func WithReadinessDataStore(ds ReadinessDataStore) ReadinessEvaluatorOption {
	return func(e *ReadinessEvaluator) { e.db = ds }
}

// WithConfigFunc sets a dynamic config provider. When set, the evaluator
// reads readiness config on each evaluation rather than using baked-in values.
func WithConfigFunc(fn func() ReadinessEvalConfig) ReadinessEvaluatorOption {
	return func(e *ReadinessEvaluator) { e.configFn = fn }
}

// WithReadinessConcurrencyFunc sets a live provider for the evaluation
// concurrency. When set, the evaluator reads the worker-pool size on each
// evaluation rather than using the value baked at construction, so
// concurrency.readiness_evaluation applies at the next run without a restart.
func WithReadinessConcurrencyFunc(fn func() int) ReadinessEvaluatorOption {
	return func(e *ReadinessEvaluator) { e.concurrencyFn = fn }
}

// effectiveConcurrency returns the live concurrency when a provider is wired
// (clamped to >= 1), otherwise the value baked at construction.
func (e *ReadinessEvaluator) effectiveConcurrency() int {
	if e.concurrencyFn != nil {
		if n := e.concurrencyFn(); n >= 1 {
			return n
		}
	}
	return e.concurrency
}

// NewReadinessEvaluator creates an evaluator.
//
// Parameters:
//   - db: datastore for reading test results and persisting readiness
//   - logger: structured logger (may be nil for silent operation)
//   - concurrency: max parallel node evaluations (worker pool size)
//   - minFreeDiskMB: minimum free disk in MB required for Habitat bundle
//     (deprecated — prefer NewReadinessEvaluatorFromConfig)
//   - opts: optional overrides
func NewReadinessEvaluator(
	db ReadinessDataStore,
	logger *logging.Logger,
	concurrency int,
	minFreeDiskMB int,
	opts ...ReadinessEvaluatorOption,
) *ReadinessEvaluator {
	if concurrency <= 0 {
		concurrency = 1
	}
	if minFreeDiskMB <= 0 {
		minFreeDiskMB = 2048
	}

	e := &ReadinessEvaluator{
		db:                      db,
		logger:                  logger,
		concurrency:             concurrency,
		installPathLinux:        "/hab",
		installPathWindows:      `C:\hab`,
		installSizeMBLinux:      minFreeDiskMB,
		installSizeMBWindows:    minFreeDiskMB,
		minRemainingFreePercent: 20,
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// ReadinessEvalConfig holds the disk-space evaluation parameters passed to
// NewReadinessEvaluatorFromConfig.
type ReadinessEvalConfig struct {
	InstallPathLinux        string
	InstallPathWindows      string
	InstallSizeMBLinux      int
	InstallSizeMBWindows    int
	MinRemainingFreePercent int
	// ReviewBlocksReadiness gates whether Review-level cookbooks block node
	// readiness. Off (default): Review resolves to compatible. On: Review-only
	// nodes become "needs review". See config.ReadinessConfig.
	ReviewBlocksReadiness bool
}

// NewReadinessEvaluatorFromConfig creates an evaluator with full per-platform
// disk space configuration.
func NewReadinessEvaluatorFromConfig(
	db ReadinessDataStore,
	logger *logging.Logger,
	concurrency int,
	cfg ReadinessEvalConfig,
	opts ...ReadinessEvaluatorOption,
) *ReadinessEvaluator {
	if concurrency <= 0 {
		concurrency = 1
	}
	if cfg.InstallPathLinux == "" {
		cfg.InstallPathLinux = "/hab"
	}
	if cfg.InstallPathWindows == "" {
		cfg.InstallPathWindows = `C:\hab`
	}
	if cfg.InstallSizeMBLinux <= 0 {
		cfg.InstallSizeMBLinux = 3072
	}
	if cfg.InstallSizeMBWindows <= 0 {
		cfg.InstallSizeMBWindows = 6144
	}
	if cfg.MinRemainingFreePercent <= 0 {
		cfg.MinRemainingFreePercent = 20
	}

	e := &ReadinessEvaluator{
		db:                      db,
		logger:                  logger,
		concurrency:             concurrency,
		installPathLinux:        cfg.InstallPathLinux,
		installPathWindows:      cfg.InstallPathWindows,
		installSizeMBLinux:      cfg.InstallSizeMBLinux,
		installSizeMBWindows:    cfg.InstallSizeMBWindows,
		minRemainingFreePercent: cfg.MinRemainingFreePercent,
		reviewBlocksReadiness:   cfg.ReviewBlocksReadiness,
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// ---------------------------------------------------------------------------
// Batch evaluation
// ---------------------------------------------------------------------------

// workItem pairs a node snapshot with a target Chef version for fan-out.
type workItem struct {
	snapshot          datastore.NodeSnapshot
	targetChefVersion string
}

// EvaluateOrganisation evaluates readiness for all nodes in the given
// organisation across all specified target Chef Client versions.
//
// The method:
//  1. Loads the latest node snapshots for the organisation
//  2. Pre-loads the server cookbook ID map for efficient lookups
//  3. Fans out work items (node × target version) across a bounded worker pool
//  4. Persists each result to the node_readiness table
//
// Returns the collected results and any error that prevented evaluation from
// starting. Individual node evaluation errors are logged but do not abort the
// batch.
func (e *ReadinessEvaluator) EvaluateOrganisation(
	ctx context.Context,
	organisationID string,
	orgName string,
	targetChefVersions []string,
) ([]ReadinessResult, error) {
	if len(targetChefVersions) == 0 {
		return nil, nil
	}

	// Step 1: Load latest node snapshots for the organisation.
	snapshots, err := e.db.ListNodeSnapshotsByOrganisation(ctx, organisationID)
	if err != nil {
		return nil, fmt.Errorf("readiness: listing node snapshots: %w", err)
	}
	if len(snapshots) == 0 {
		e.logInfo(orgName, "no node snapshots found — skipping readiness evaluation")
		return nil, nil
	}

	// Step 2: Build the cookbook ID map from server cookbooks.
	// The composite natural key (org/name/version) serves as the cookbook ID
	// for looking up CookStyle results and complexity scores.
	serverCookbooks, err := e.db.ListServerCookbooksByOrganisation(ctx, organisationID)
	if err != nil {
		return nil, fmt.Errorf("readiness: listing server cookbooks: %w", err)
	}
	cookbookIDMap := buildCookbookIDMap(serverCookbooks)

	// Step 3: Bulk-load all lookup data into an in-memory cache.
	// This replaces ~12M individual DB queries with ~5 bulk queries.
	cache, err := buildReadinessCache(ctx, e.db, organisationID, targetChefVersions, e.reviewBlocksReadinessNow())
	if err != nil {
		return nil, fmt.Errorf("readiness: building cache: %w", err)
	}

	// Step 4: Build work items.
	items := make([]workItem, 0, len(snapshots)*len(targetChefVersions))
	for _, snap := range snapshots {
		for _, tv := range targetChefVersions {
			items = append(items, workItem{
				snapshot:          snap,
				targetChefVersion: tv,
			})
		}
	}

	e.logInfo(orgName, fmt.Sprintf("evaluating %d nodes × %d target versions = %d work items",
		len(snapshots), len(targetChefVersions), len(items)))

	// Step 5: Fan out. evaluateOne uses only in-memory lookups (no DB calls),
	// so context cancellation cannot corrupt evaluations.
	sem := make(chan struct{}, e.effectiveConcurrency())
	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make([]ReadinessResult, 0, len(items))

	for _, item := range items {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{} // acquire
		go func(wi workItem) {
			defer wg.Done()
			defer func() { <-sem }() // release

			result := e.evaluateOne(wi.snapshot, wi.targetChefVersion, cookbookIDMap, cache)

			// Persist with a background context so that a cancelled parent
			// context does not prevent saving results we have already
			// computed. Each node's readiness is independent — partial
			// progress is better than losing everything.
			persistCtx, persistCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if persistErr := e.persistResult(persistCtx, result); persistErr != nil {
				e.logError(orgName,
					fmt.Sprintf("failed to persist readiness for node %s target %s: %v",
						wi.snapshot.NodeName, wi.targetChefVersion, persistErr))
			}
			persistCancel()

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(item)
	}

	wg.Wait()

	e.logInfo(orgName, fmt.Sprintf("readiness evaluation complete: %d results", len(results)))
	return results, nil
}

// ---------------------------------------------------------------------------
// Single node evaluation
// ---------------------------------------------------------------------------

// evaluateOne computes readiness for one node × target version.
// All lookups use the pre-loaded cache — no database calls are made.
func (e *ReadinessEvaluator) evaluateOne(
	snapshot datastore.NodeSnapshot,
	targetChefVersion string,
	cookbookIDMap map[string]map[string]string,
	cache *readinessCache,
) ReadinessResult {
	now := time.Now().UTC()

	result := ReadinessResult{
		OrganisationName:  snapshot.OrganisationName,
		NodeName:          snapshot.NodeName,
		TargetChefVersion: targetChefVersion,
		StaleData:         snapshot.IsStale,
		RequiredDiskMB:    e.installSizeForPlatform(snapshot.Platform),
		EvaluatedAt:       now,
	}

	// --- Cookbook compatibility ---
	blockingCookbooks, reviewCookbooks, tkStats := e.evaluateCookbooks(snapshot, targetChefVersion, cookbookIDMap, cache)
	result.BlockingCookbooks = blockingCookbooks
	result.ReviewCookbooks = reviewCookbooks
	result.AllCookbooksCompatible = len(blockingCookbooks) == 0

	// --- Disk space (version-invariant; computed by analysis/disk.go) ---
	if snapshot.IsStale {
		// Stale nodes: disk space treated as unknown.
		result.SufficientDiskSpace = nil
		result.AvailableDiskMB = nil
	} else {
		// The verdict depends only on the filesystem + platform install size, never
		// the target version — the same EvaluateDisk the collector stores per node.
		v := EvaluateDisk(snapshot.Filesystem, snapshot.Platform, e.diskConfig())
		result.SufficientDiskSpace = v.Sufficient
		result.AvailableDiskMB = v.AvailableMB
		// result.RequiredDiskMB was set at construction (== v.RequiredMB).
	}

	// --- Overall readiness (node rollup status) ---
	// Blocked: any incompatible/untested cookbook OR disk insufficient/unknown
	//   (unknown disk blocks, erring on the side of caution).
	// Needs review: not blocked, but ≥1 needs-review cookbook (toggle-on only).
	// Ready: cookbook list clean AND disk sufficient.
	diskOK := result.SufficientDiskSpace != nil && *result.SufficientDiskSpace
	switch {
	case !diskOK || len(blockingCookbooks) > 0:
		result.Status = StatusBlocked
	case len(reviewCookbooks) > 0:
		result.Status = StatusNeedsReview
	default:
		result.Status = StatusReady
	}
	result.IsReady = result.Status == StatusReady

	// --- Materialised check statuses ---
	result.CookstyleStatus = deriveCookstyleStatusFromBlocking(
		result.AllCookbooksCompatible, result.StaleData, blockingCookbooks)
	result.KitchenStatus = deriveKitchenStatusFromBlocking(
		result.AllCookbooksCompatible, result.StaleData, blockingCookbooks, tkStats)

	return result
}

// ---------------------------------------------------------------------------
// Cookbook compatibility evaluation
// ---------------------------------------------------------------------------

// nodeCookbookEntry represents one entry from automatic.cookbooks JSON.
// The JSON format is: {"name": {"version": "1.2.3", ...}, ...}
// We only need the version field.
type nodeCookbookEntry struct {
	Version string `json:"version"`
}

// tkCoverageStats tracks Test Kitchen coverage across all cookbooks on a node.
type tkCoverageStats struct {
	totalCookbooks int
	tkEligible     int // cookbooks with HasTestSuite && !KitchenExcluded
	tkTested       int // cookbooks where TK result exists (any status)
	tkPassed       int
	tkFailed       int
}

// evaluateCookbooks checks all cookbooks on the node against the target
// Chef Client version. Returns the list of blocking cookbooks and TK coverage stats.
func (e *ReadinessEvaluator) evaluateCookbooks(
	snapshot datastore.NodeSnapshot,
	targetChefVersion string,
	cookbookIDMap map[string]map[string]string,
	cache *readinessCache,
) (blocking, review []BlockingCookbook, stats tkCoverageStats) {
	var tkStats tkCoverageStats
	if len(snapshot.Cookbooks) == 0 {
		return nil, nil, tkStats
	}

	// Parse the automatic.cookbooks attribute.
	cookbooks := parseCookbooksAttribute(snapshot.Cookbooks)
	if len(cookbooks) == 0 {
		return nil, nil, tkStats
	}

	for cbName, cbVersion := range cookbooks {
		tkStats.totalCookbooks++

		// Track TK coverage for this cookbook.
		if gitRepo, ok := cache.gitRepos[cbName]; ok && gitRepo.Name != "" && gitRepo.HasTestSuite && !gitRepo.KitchenExcluded {
			tkStats.tkEligible++
			tkStatus := cache.gitTKStatuses[cacheKey(gitRepo.Name, targetChefVersion)]
			switch tkStatus {
			case "passed":
				tkStats.tkTested++
				tkStats.tkPassed++
			case "failed", "partial":
				tkStats.tkTested++
				tkStats.tkFailed++
			}
		}

		status, source, verdicts := checkCookbookCompatibility(cbName, cbVersion, targetChefVersion, cookbookIDMap, cache)

		switch status {
		case StatusCompatible, StatusCompatibleCookstyleOnly:
			// Not blocking, not needs-review.
			continue
		case StatusNeedsReview:
			review = append(review, e.buildBlockingCookbook(cbName, cbVersion, status, source, verdicts, targetChefVersion, cookbookIDMap, cache))
		case StatusIncompatible, StatusUntested:
			blocking = append(blocking, e.buildBlockingCookbook(cbName, cbVersion, status, source, verdicts, targetChefVersion, cookbookIDMap, cache))
		}
	}

	return blocking, review, tkStats
}

// buildBlockingCookbook assembles a BlockingCookbook entry (used for both the
// blocking and needs-review lists) enriched with complexity data from the
// highest-confidence source and per-verdict complexity.
func (e *ReadinessEvaluator) buildBlockingCookbook(
	cbName, cbVersion, status, source string,
	verdicts []CookbookSourceVerdict,
	targetChefVersion string,
	cookbookIDMap map[string]map[string]string,
	cache *readinessCache,
) BlockingCookbook {
	bc := BlockingCookbook{
		Name:     cbName,
		Version:  cbVersion,
		Reason:   status,
		Source:   source,
		Verdicts: verdicts,
	}

	// Try to enrich with server cookbook complexity data.
	cookbookID := lookupCookbookID(cookbookIDMap, cbName, cbVersion)
	if cookbookID != "" {
		if cc := cache.serverComplexity[cacheKey(cookbookID, targetChefVersion)]; cc != nil {
			bc.ComplexityScore = cc.ComplexityScore
			bc.ComplexityLabel = cc.ComplexityLabel
		}
	}

	// Enrich verdicts with complexity data.
	for i := range bc.Verdicts {
		switch bc.Verdicts[i].Source {
		case SourceServerCookstyle:
			if cookbookID != "" {
				if cc := cache.serverComplexity[cacheKey(cookbookID, targetChefVersion)]; cc != nil {
					bc.Verdicts[i].ComplexityScore = cc.ComplexityScore
					bc.Verdicts[i].ComplexityLabel = cc.ComplexityLabel
				}
			}
		case SourceGitCookstyle:
			if gitRepo, ok := cache.gitRepos[cbName]; ok && gitRepo.Name != "" {
				if gc := cache.gitComplexity[cacheKey(gitRepo.Name, targetChefVersion)]; gc != nil {
					bc.Verdicts[i].ComplexityScore = gc.ComplexityScore
					bc.Verdicts[i].ComplexityLabel = gc.ComplexityLabel
				}
			}
		}
	}

	return bc
}

// deriveCookstyleStatusFromBlocking computes the CookStyle check status from
// the evaluated blocking cookbooks list.
func deriveCookstyleStatusFromBlocking(allCompatible, stale bool, blocking []BlockingCookbook) string {
	if stale {
		return "unknown"
	}
	if allCompatible && len(blocking) == 0 {
		return "passed"
	}

	csFailCount := 0
	hasCookstyleVerdict := false
	for _, b := range blocking {
		for _, v := range b.Verdicts {
			if isCookstyleVerdictSource(v.Source) {
				hasCookstyleVerdict = true
				if v.Status == StatusIncompatible {
					csFailCount++
					break
				}
			}
		}
	}

	if csFailCount > 0 {
		return "failed"
	}

	if len(blocking) > 0 && !hasCookstyleVerdict {
		allTKOnly := true
		for _, b := range blocking {
			if !hasOnlyTKFailure(b) {
				allTKOnly = false
				break
			}
		}
		if allTKOnly {
			return "passed"
		}
		return "unknown"
	}

	if hasCookstyleVerdict && csFailCount == 0 {
		return "passed"
	}
	return "unknown"
}

// deriveKitchenStatusFromBlocking computes the Test Kitchen check status from
// the evaluated blocking cookbooks list.
func deriveKitchenStatusFromBlocking(allCompatible, stale bool, blocking []BlockingCookbook, tkStats tkCoverageStats) string {
	if stale {
		return "unknown"
	}
	if tkStats.tkFailed > 0 {
		return "failed"
	}
	if tkStats.tkPassed > 0 && tkStats.tkTested < tkStats.tkEligible {
		return "partial"
	}
	if tkStats.tkEligible > 0 && tkStats.tkTested == tkStats.tkEligible && tkStats.tkFailed == 0 {
		return "passed"
	}
	if tkStats.tkEligible == 0 {
		if allCompatible && len(blocking) == 0 {
			return "passed"
		}
		// No TK-eligible cookbooks but some blocking — unknown
		if len(blocking) > 0 {
			return "unknown"
		}
		return "passed"
	}
	if allCompatible && len(blocking) == 0 {
		return "passed"
	}
	return "unknown"
}

// isCookstyleVerdictSource returns true if the verdict source is a cookstyle check.
func isCookstyleVerdictSource(source string) bool {
	return source == SourceServerCookstyle || source == SourceGitCookstyle
}

// hasOnlyTKFailure returns true if the blocking entry has only a
// git_test_kitchen incompatible verdict and no cookstyle failures.
func hasOnlyTKFailure(b BlockingCookbook) bool {
	hasTKFail := false
	for _, v := range b.Verdicts {
		if isCookstyleVerdictSource(v.Source) && v.Status == StatusIncompatible {
			return false
		}
		if v.Source == SourceGitTestKitchen && v.Status == StatusIncompatible {
			hasTKFail = true
		}
	}
	return hasTKFail
}

// parseCookbooksAttribute parses the automatic.cookbooks JSONB into a map
// of cookbook_name → version. It handles both the standard Ohai format
// (map of name → object with "version" key) and simpler formats.
func parseCookbooksAttribute(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}

	// Standard format: {"cb_name": {"version": "1.2.3", ...}, ...}
	var structured map[string]nodeCookbookEntry
	if err := json.Unmarshal(raw, &structured); err == nil && len(structured) > 0 {
		result := make(map[string]string, len(structured))
		for name, entry := range structured {
			if entry.Version != "" {
				result[name] = entry.Version
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	// Fallback: {"cb_name": "version_string", ...}
	var simple map[string]string
	if err := json.Unmarshal(raw, &simple); err == nil && len(simple) > 0 {
		return simple
	}

	return nil
}

// checkCookbookCompatibility determines the compatibility status of a single
// cookbook × version against the target Chef Client version using multi-source
// evaluation and the pre-loaded cache.
//
// Algorithm:
//  1. Check Git repo CookStyle result → verdict
//  2. Check Server cookbook CookStyle result → verdict
//  3. Aggregate: any compatible → compatible_cookstyle_only; all tested
//     incompatible → incompatible; no results → untested
//
// This is a package-level function (not a method) because it uses only
// in-memory cache lookups — no database calls.
func checkCookbookCompatibility(
	cookbookName string,
	cookbookVersion string,
	targetChefVersion string,
	cookbookIDMap map[string]map[string]string,
	cache *readinessCache,
) (status, source string, verdicts []CookbookSourceVerdict) {
	cookbookID := lookupCookbookID(cookbookIDMap, cookbookName, cookbookVersion)

	var anyCSCompatible bool  // at least one CookStyle source is Ready
	var anyCSNeedsReview bool // at least one CookStyle source is Needs review (toggle-on)
	var anyTested bool

	// --- Source 1: Git repo CookStyle ---
	gitRepo, hasGitRepo := cache.gitRepos[cookbookName]
	if hasGitRepo && gitRepo.Name != "" {
		gitCSResult := cache.gitCSResults[cacheKey(gitRepo.Name, targetChefVersion)]
		if gitCSResult != nil && gitCSResult.ErrorMessage == "" {
			anyTested = true
			v := CookbookSourceVerdict{
				Source:    SourceGitCookstyle,
				Version:   "HEAD",
				CommitSHA: gitRepo.HeadCommitSHA,
			}
			v.Status = cookstyleVerdict(gitCSResult.CookstyleStatus, gitCSResult.Passed, cache.reviewBlocksReadiness)
			markCSAggregate(v.Status, &anyCSCompatible, &anyCSNeedsReview)
			verdicts = append(verdicts, v)
		}
	}

	// --- Source 2: Server cookbook CookStyle ---
	if cookbookID != "" {
		csResult := cache.serverCSResults[cacheKey(cookbookID, targetChefVersion)]
		if csResult == nil || csResult.ErrorMessage != "" {
			// Also check CookStyle without a target version — server-sourced
			// cookbooks may have been scanned without a target version profile.
			csResult = cache.serverCSResults[cacheKey(cookbookID, "")]
		}
		if csResult != nil && csResult.ErrorMessage == "" {
			anyTested = true
			v := CookbookSourceVerdict{
				Source:  SourceServerCookstyle,
				Version: cookbookVersion,
			}
			v.Status = cookstyleVerdict(csResult.CookstyleStatus, csResult.Passed, cache.reviewBlocksReadiness)
			markCSAggregate(v.Status, &anyCSCompatible, &anyCSNeedsReview)
			verdicts = append(verdicts, v)
		}
	}

	// --- Source 3: Git Test Kitchen ---
	hasTK := false
	anyTKFail := false
	if hasGitRepo && gitRepo.Name != "" && gitRepo.HasTestSuite && !gitRepo.KitchenExcluded {
		tkStatus := cache.gitTKStatuses[cacheKey(gitRepo.Name, targetChefVersion)]
		if tkStatus != "" {
			hasTK = true
			anyTested = true
			v := CookbookSourceVerdict{
				Source:  SourceGitTestKitchen,
				Version: "HEAD",
			}
			switch tkStatus {
			case "passed":
				v.Status = StatusCompatible
			case "failed":
				v.Status = StatusIncompatible
				anyTKFail = true
			case "partial":
				v.Status = StatusIncompatible
				anyTKFail = true
			}
			verdicts = append(verdicts, v)
		}
	}

	// --- Determine overall status ---
	// TK failure makes the cookbook incompatible even if CookStyle passed.
	if anyTKFail {
		return StatusIncompatible, SourceGitTestKitchen, verdicts
	}
	// CookStyle must pass for the cookbook to be compatible.
	if anyCSCompatible && hasTK {
		return StatusCompatible, SourceCookstyle, verdicts
	}
	if anyCSCompatible {
		return StatusCompatibleCookstyleOnly, SourceCookstyle, verdicts
	}
	// No compatible source, but a Review-level source exists (toggle-on only):
	// the cookbook needs review — it does not block, but the node is not ready.
	if anyCSNeedsReview {
		return StatusNeedsReview, SourceCookstyle, verdicts
	}
	if anyTested {
		return StatusIncompatible, SourceCookstyle, verdicts
	}
	return StatusUntested, SourceNone, verdicts
}

// cookstyleVerdict maps a materialised CookStyle rollup status to a cookbook
// compatibility verdict, honouring the review-blocks-readiness toggle. Falls
// back to the legacy passed boolean for results predating status materialisation
// (cookstyleStatus == "").
func cookstyleVerdict(cookstyleStatus string, passed bool, reviewBlocks bool) string {
	switch cookstyleStatus {
	case StatusReady:
		return StatusCompatible
	case StatusBlocked:
		return StatusIncompatible
	case StatusNeedsReview:
		if reviewBlocks {
			return StatusNeedsReview
		}
		return StatusCompatible
	default:
		// Unmaterialised result — fall back to the back-compat boolean.
		if passed {
			return StatusCompatible
		}
		return StatusIncompatible
	}
}

// markCSAggregate records a CookStyle source verdict into the compatible /
// needs-review aggregate flags used to compute the overall cookbook status.
func markCSAggregate(verdict string, anyCSCompatible, anyCSNeedsReview *bool) {
	switch verdict {
	case StatusCompatible:
		*anyCSCompatible = true
	case StatusNeedsReview:
		*anyCSNeedsReview = true
	}
}

// lookupCookbookID resolves a cookbook name + version to its database ID
// using the pre-loaded map.
func lookupCookbookID(idMap map[string]map[string]string, name, version string) string {
	if idMap == nil {
		return ""
	}
	versions, ok := idMap[name]
	if !ok {
		return ""
	}
	return versions[version]
}

// buildCookbookIDMap constructs a name → version → compositeID lookup map
// from a slice of ServerCookbook structs. The composite ID is
// "organisationName/name/version", matching the key format used by CookStyle
// scanning and complexity scoring after the natural-keys migration.
func buildCookbookIDMap(cookbooks []datastore.ServerCookbook) map[string]map[string]string {
	idMap := make(map[string]map[string]string, len(cookbooks))
	for _, cb := range cookbooks {
		versions, ok := idMap[cb.Name]
		if !ok {
			versions = make(map[string]string)
			idMap[cb.Name] = versions
		}
		versions[cb.Version] = cb.OrganisationName + "/" + cb.Name + "/" + cb.Version
	}
	return idMap
}

// ---------------------------------------------------------------------------
// Disk space evaluation
// ---------------------------------------------------------------------------

// filesystemEntry represents one entry in the automatic.filesystem JSON.
// Values may be strings or integers depending on the Chef Client version.
type filesystemEntry struct {
	KBSize      interface{} `json:"kb_size"`
	KBUsed      interface{} `json:"kb_used"`
	KBAvailable interface{} `json:"kb_available"`
	PercentUsed interface{} `json:"percent_used"`
	Mount       interface{} `json:"mount"`
}

// diskConfig assembles the version-invariant DiskConfig for EvaluateDisk from the
// live config (configFn) or the values baked at construction.
func (e *ReadinessEvaluator) diskConfig() DiskConfig {
	if e.configFn != nil {
		cfg := e.configFn()
		return DiskConfig{
			InstallPathLinux:        cfg.InstallPathLinux,
			InstallPathWindows:      cfg.InstallPathWindows,
			InstallSizeMBLinux:      cfg.InstallSizeMBLinux,
			InstallSizeMBWindows:    cfg.InstallSizeMBWindows,
			MinRemainingFreePercent: cfg.MinRemainingFreePercent,
		}
	}
	return DiskConfig{
		InstallPathLinux:        e.installPathLinux,
		InstallPathWindows:      e.installPathWindows,
		InstallSizeMBLinux:      e.installSizeMBLinux,
		InstallSizeMBWindows:    e.installSizeMBWindows,
		MinRemainingFreePercent: e.minRemainingFreePercent,
	}
}

// reviewBlocksReadinessNow returns the live review-blocks-readiness toggle from
// configFn when wired, otherwise the value baked at construction.
func (e *ReadinessEvaluator) reviewBlocksReadinessNow() bool {
	if e.configFn != nil {
		return e.configFn().ReviewBlocksReadiness
	}
	return e.reviewBlocksReadiness
}

// evaluateDiskSpace determines the available disk space on the installation
// target mount point and returns (available MB, total MB, known). It delegates
// to resolveDiskUsage (the shared parser EvaluateDisk uses) so the readiness
// path and the node-level disk verdict never drift.
func (e *ReadinessEvaluator) evaluateDiskSpace(snapshot datastore.NodeSnapshot) (availableMB int, totalMB int, known bool) {
	return resolveDiskUsage(snapshot.Filesystem, snapshot.Platform, e.installPathForPlatform(snapshot.Platform))
}

// installPathForPlatform returns the configured install path for the platform.
func (e *ReadinessEvaluator) installPathForPlatform(platform string) string {
	if e.configFn != nil {
		cfg := e.configFn()
		if strings.ToLower(platform) == "windows" {
			return cfg.InstallPathWindows
		}
		return cfg.InstallPathLinux
	}
	if strings.ToLower(platform) == "windows" {
		return e.installPathWindows
	}
	return e.installPathLinux
}

// installSizeForPlatform returns the required install size in MB for the platform.
func (e *ReadinessEvaluator) installSizeForPlatform(platform string) int {
	if e.configFn != nil {
		cfg := e.configFn()
		if strings.ToLower(platform) == "windows" {
			return cfg.InstallSizeMBWindows
		}
		return cfg.InstallSizeMBLinux
	}
	if strings.ToLower(platform) == "windows" {
		return e.installSizeMBWindows
	}
	return e.installSizeMBLinux
}

// parseFilesystemAttribute parses the automatic.filesystem JSONB into a map
// of device/mount-name → filesystemEntry.
//
// It handles two Ohai formats:
//
//  1. Legacy (Ohai < 14): top-level keys are device paths (e.g. "/dev/sda1"),
//     each mapping directly to a filesystemEntry with kb_size, kb_available,
//     mount, etc.
//
//  2. New (Ohai 14+): top-level keys are "by_pair", "by_device",
//     "by_mountpoint", each containing a nested map of entries. We prefer
//     "by_pair" because it contains the most complete data (mount + device
//     in the key, all size fields in the value).
func parseFilesystemAttribute(raw json.RawMessage) map[string]filesystemEntry {
	if len(raw) == 0 {
		return nil
	}

	// Try the new Ohai 14+ format first: { "by_pair": { ... }, "by_device": { ... }, "by_mountpoint": { ... } }
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err == nil {
		// Check for the new format by looking for known top-level keys.
		if byPair, ok := nested["by_pair"]; ok {
			var pairMap map[string]filesystemEntry
			if err := json.Unmarshal(byPair, &pairMap); err == nil && len(pairMap) > 0 {
				return pairMap
			}
		}
		if byMount, ok := nested["by_mountpoint"]; ok {
			var mountMap map[string]filesystemEntry
			if err := json.Unmarshal(byMount, &mountMap); err == nil && len(mountMap) > 0 {
				return mountMap
			}
		}
		if byDev, ok := nested["by_device"]; ok {
			var devMap map[string]filesystemEntry
			if err := json.Unmarshal(byDev, &devMap); err == nil && len(devMap) > 0 {
				return devMap
			}
		}
	}

	// Legacy format: top-level keys are device paths mapping directly to entries.
	var fsMap map[string]filesystemEntry
	if err := json.Unmarshal(raw, &fsMap); err != nil {
		return nil
	}
	return fsMap
}

// determineInstallPath returns the installation target path for the Habitat
// bundle based on the platform.
func determineInstallPath(platform string) string {
	p := strings.ToLower(platform)
	if p == "windows" {
		return `C:\hab`
	}
	return "/hab"
}

// findBestMount finds the filesystem entry whose mount is the longest prefix
// match for the given installation path. Returns the mount path and entry.
//
// For Windows, we match on the drive letter (e.g. "C:").
// For Linux, we do longest prefix match on the mount path.
func findBestMount(
	fsMap map[string]filesystemEntry,
	installPath string,
	platform string,
) (string, *filesystemEntry) {
	isWindows := strings.ToLower(platform) == "windows"

	if isWindows {
		return findBestMountWindows(fsMap, installPath)
	}
	return findBestMountLinux(fsMap, installPath)
}

// findBestMountLinux finds the filesystem entry whose mount field is the
// longest prefix of the install path.
func findBestMountLinux(
	fsMap map[string]filesystemEntry,
	installPath string,
) (string, *filesystemEntry) {
	var bestMount string
	var bestEntry *filesystemEntry
	bestLen := -1

	for key, entry := range fsMap {
		mount := toString(entry.Mount)
		if mount == "" {
			// Some entries might use the key as the device name (e.g. "/dev/sda1")
			// but have no mount field — skip those.
			continue
		}

		// Check if the mount is a prefix of the install path.
		if isPathPrefix(mount, installPath) {
			if len(mount) > bestLen {
				bestLen = len(mount)
				bestMount = key
				e := entry // copy
				bestEntry = &e
			}
		}
	}

	return bestMount, bestEntry
}

// findBestMountWindows finds the filesystem entry matching the drive letter
// of the install path.
func findBestMountWindows(
	fsMap map[string]filesystemEntry,
	installPath string,
) (string, *filesystemEntry) {
	// Extract drive letter from installPath (e.g. "C" from "C:\hab").
	targetDrive := ""
	if len(installPath) >= 2 && installPath[1] == ':' {
		targetDrive = strings.ToUpper(installPath[:1])
	}
	if targetDrive == "" {
		// Can't determine drive letter — try C: as default.
		targetDrive = "C"
	}

	// First try to find exact drive match by key (e.g. "C:" or "C:\").
	for key, entry := range fsMap {
		keyUpper := strings.ToUpper(strings.TrimRight(key, `\/`))
		if keyUpper == targetDrive+":" {
			e := entry
			return key, &e
		}
	}

	// Fallback: check mount fields.
	for key, entry := range fsMap {
		mount := toString(entry.Mount)
		mountUpper := strings.ToUpper(strings.TrimRight(mount, `\/`))
		if mountUpper == targetDrive+":" {
			e := entry
			return key, &e
		}
	}

	return "", nil
}

// isPathPrefix returns true if prefix is a filesystem path prefix of path.
// This handles the subtlety that "/opt" is a prefix of "/opt/hab" but NOT
// of "/optional".
func isPathPrefix(prefix, path string) bool {
	if prefix == "/" {
		return true // root is always a prefix
	}
	if prefix == path {
		return true
	}
	// prefix must end at a path separator boundary.
	if strings.HasPrefix(path, prefix) {
		if len(path) > len(prefix) && path[len(prefix)] == '/' {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Value conversion helpers
// ---------------------------------------------------------------------------

// toInt64 converts an interface{} (string or numeric) to int64.
// Returns -1 if the value cannot be parsed.
func toInt64(v interface{}) int64 {
	if v == nil {
		return -1
	}
	switch val := v.(type) {
	case string:
		// Strip surrounding quotes and whitespace.
		s := strings.TrimSpace(val)
		if s == "" {
			return -1
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			// Try parsing as float (some systems report "12345.0").
			f, fErr := strconv.ParseFloat(s, 64)
			if fErr != nil {
				return -1
			}
			return int64(math.Floor(f))
		}
		return n
	case float64:
		return int64(math.Floor(val))
	case float32:
		return int64(math.Floor(float64(val)))
	case int:
		return int64(val)
	case int64:
		return val
	case int32:
		return int64(val)
	case json.Number:
		n, err := val.Int64()
		if err != nil {
			f, fErr := val.Float64()
			if fErr != nil {
				return -1
			}
			return int64(math.Floor(f))
		}
		return n
	default:
		return -1
	}
}

// toString converts an interface{} to a string. Returns "" for nil.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

// persistResult writes a ReadinessResult to the node_readiness table.
func (e *ReadinessEvaluator) persistResult(ctx context.Context, result ReadinessResult) error {
	var blockingJSON json.RawMessage
	if len(result.BlockingCookbooks) > 0 {
		b, err := json.Marshal(result.BlockingCookbooks)
		if err != nil {
			return fmt.Errorf("readiness: marshalling blocking cookbooks: %w", err)
		}
		blockingJSON = b
	}

	var reviewJSON json.RawMessage
	if len(result.ReviewCookbooks) > 0 {
		b, err := json.Marshal(result.ReviewCookbooks)
		if err != nil {
			return fmt.Errorf("readiness: marshalling review cookbooks: %w", err)
		}
		reviewJSON = b
	}

	requiredDiskMB := result.RequiredDiskMB
	_, err := e.db.UpsertNodeReadiness(ctx, datastore.UpsertNodeReadinessParams{
		OrganisationName:       result.OrganisationName,
		NodeName:               result.NodeName,
		TargetChefVersion:      result.TargetChefVersion,
		IsReady:                result.IsReady,
		AllCookbooksCompatible: result.AllCookbooksCompatible,
		SufficientDiskSpace:    result.SufficientDiskSpace,
		BlockingCookbooks:      blockingJSON,
		ReviewCookbooks:        reviewJSON,
		AvailableDiskMB:        result.AvailableDiskMB,
		RequiredDiskMB:         &requiredDiskMB,
		StaleData:              result.StaleData,
		Status:                 result.Status,
		CookstyleStatus:        result.CookstyleStatus,
		KitchenStatus:          result.KitchenStatus,
		EvaluatedAt:            result.EvaluatedAt,
	})
	return err
}

// ---------------------------------------------------------------------------
// Logging helpers
// ---------------------------------------------------------------------------

func (e *ReadinessEvaluator) logInfo(orgName, msg string) {
	if e.logger == nil {
		return
	}
	e.logger.Info(logging.ScopeReadinessEvaluation, msg, logging.WithOrganisation(orgName))
}

func (e *ReadinessEvaluator) logError(orgName, msg string) {
	if e.logger == nil {
		return
	}
	e.logger.Error(logging.ScopeReadinessEvaluation, msg, logging.WithOrganisation(orgName))
}
