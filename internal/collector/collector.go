// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package collector implements the data collection orchestrator for Chef
// Migration Metrics. It periodically collects node data from configured Chef
// Infra Server organisations, fetches cookbook inventories, determines
// active/unused cookbooks, and flags stale nodes.
//
// The collector is the critical path between the Chef API client, the
// datastore, and the analysis pipeline. It supports:
//   - Multi-organisation parallel collection (bounded by concurrency config)
//   - Checkpoint/resume for interrupted runs
//   - Cron-scheduled and manually-triggered runs
//   - Graceful shutdown with in-progress run interruption
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/staleness"
)

// ClientFactory creates a chefapi.Client for a given organisation. This is
// injected as a dependency to allow testing with mock clients.
type ClientFactory func(ctx context.Context, org datastore.Organisation) (*chefapi.Client, error)

// orgResult holds the outcome of collecting a single organisation.
type orgResult struct {
	OrgName   string
	Nodes     int
	Cookbooks int
	Duration  time.Duration
	Err       error
}

// Collector orchestrates periodic data collection from Chef Infra Server
// organisations. It is safe for concurrent use — only one collection run
// may be active at a time.
type Collector struct {
	db            *datastore.DB
	cfg           *config.Config
	logger        *logging.Logger
	resolver      *secrets.CredentialResolver
	clientFactory ClientFactory
	analyser      *analysis.Analyser

	// Optional analysis pipeline components. When non-nil, the collector
	// runs these after cookbook usage analysis (Step 10) as part of the
	// collection cycle. When nil, the corresponding step is skipped.
	cookstyleScanner *analysis.CookstyleScanner
	kitchenAnalyser  *analysis.KitchenAnalyser
	autocorrectGen   *remediation.AutocorrectGenerator
	complexityScorer *remediation.ComplexityScorer
	readinessEval    *analysis.ReadinessEvaluator
	ownershipEval    *OwnershipEvaluator

	// serverCookbookDirFn resolves the filesystem path for a server
	// cookbook. Required by CookStyle scanning of server cookbooks and
	// autocorrect preview generation. When nil, those steps are skipped.
	serverCookbookDirFn func(cb datastore.ServerCookbook) string

	// gitRepoDirFn resolves the filesystem path for a git repo. Required
	// by CookStyle scanning, Test Kitchen, and autocorrect preview
	// generation for git-sourced cookbooks. When nil, those steps are skipped.
	gitRepoDirFn func(repo datastore.GitRepo) string

	// cookbookCacheDir is the base directory for extracting Chef server
	// cookbook files to disk. Files are written to
	// <cookbookCacheDir>/<org_id>/<name>/<version>/. When empty, file
	// extraction is skipped (only manifest fetch + status update).
	cookbookCacheDir string

	// gitCookbookDir is the base directory where git cookbook repositories
	// are cloned and pulled. Structure: <gitCookbookDir>/<cookbook_name>/.
	// When empty, falls back to $TMPDIR/chef-migration-metrics/git-cookbooks.
	gitCookbookDir string

	// mu guards currentRunOrgName to enforce the single-run constraint.
	mu                sync.Mutex
	currentRunOrgName string
	running           bool

	// configFn, when set, returns the current live config. This is used
	// to pick up config changes (e.g. git_base_urls) made via the admin
	// UI without requiring a restart. When nil, the static cfg is used.
	configFn func() *config.Config
}

// Option configures optional behaviour on a Collector.
type Option func(*Collector)

// WithClientFactory overrides the default client factory. This is intended
// for testing with mock Chef API clients.
func WithClientFactory(f ClientFactory) Option {
	return func(c *Collector) {
		if f != nil {
			c.clientFactory = f
		}
	}
}

// WithCookstyleScanner sets the CookStyle scanner for the collection cycle.
// When set, CookStyle scanning runs after cookbook fetching.
func WithCookstyleScanner(s *analysis.CookstyleScanner) Option {
	return func(c *Collector) { c.cookstyleScanner = s }
}

// WithKitchenAnalyser sets the Kitchen Analyser for config discovery.
// When set, kitchen config analysis runs after git clone/fetch.
func WithKitchenAnalyser(a *analysis.KitchenAnalyser) Option {
	return func(c *Collector) { c.kitchenAnalyser = a }
}

// WithAutocorrectGenerator sets the autocorrect preview generator.
// When set, autocorrect previews are generated after CookStyle scanning.
func WithAutocorrectGenerator(g *remediation.AutocorrectGenerator) Option {
	return func(c *Collector) { c.autocorrectGen = g }
}

// WithComplexityScorer sets the cookbook complexity scorer.
// When set, complexity scoring runs after CookStyle and Test Kitchen.
func WithComplexityScorer(s *remediation.ComplexityScorer) Option {
	return func(c *Collector) { c.complexityScorer = s }
}

// WithReadinessEvaluator sets the node readiness evaluator.
// When set, readiness evaluation runs at the end of the analysis pipeline.
func WithReadinessEvaluator(e *analysis.ReadinessEvaluator) Option {
	return func(c *Collector) { c.readinessEval = e }
}

// WithOwnershipEvaluator sets the ownership auto-derivation evaluator.
// When set, ownership rules are evaluated after each collection run.
func WithOwnershipEvaluator(e *OwnershipEvaluator) Option {
	return func(c *Collector) { c.ownershipEval = e }
}

// WithServerCookbookDirFn sets the function that resolves a server cookbook
// to its filesystem path. Required by CookStyle scanning and autocorrect
// preview generation for server cookbooks.
func WithServerCookbookDirFn(fn func(cb datastore.ServerCookbook) string) Option {
	return func(c *Collector) { c.serverCookbookDirFn = fn }
}

// WithGitRepoDirFn sets the function that resolves a git repo to its
// filesystem path. Required by CookStyle scanning, Test Kitchen, and
// autocorrect preview generation for git-sourced cookbooks.
func WithGitRepoDirFn(fn func(repo datastore.GitRepo) string) Option {
	return func(c *Collector) { c.gitRepoDirFn = fn }
}

// WithCookbookCacheDir sets the base directory for extracting Chef server
// cookbook files to disk during collection. When set, downloadCookbookVersion
// writes each file from the cookbook manifest to
// <dir>/<org_id>/<name>/<version>/. This is required for CookStyle scanning
// of Chef server cookbooks.
func WithCookbookCacheDir(dir string) Option {
	return func(c *Collector) { c.cookbookCacheDir = dir }
}

// WithGitCookbookDir sets the base directory for cloning git cookbook
// repositories during collection. When set, git operations use this path
// instead of the default $TMPDIR-based location.
func WithGitCookbookDir(dir string) Option {
	return func(c *Collector) { c.gitCookbookDir = dir }
}

// WithConfigFn sets a function that returns the current live config.
// When set, the collector reads config at the start of each run, picking
// up changes made via the admin UI (e.g. git_base_urls, target versions).
func WithConfigFn(fn func() *config.Config) Option {
	return func(c *Collector) { c.configFn = fn }
}

// effectiveConfig returns the current config — from configFn if set,
// otherwise the static cfg captured at construction.
func (c *Collector) effectiveConfig() *config.Config {
	if c.configFn != nil {
		return c.configFn()
	}
	return c.cfg
}

// New creates a new Collector with the given dependencies.
func New(
	db *datastore.DB,
	cfg *config.Config,
	logger *logging.Logger,
	resolver *secrets.CredentialResolver,
	opts ...Option,
) *Collector {
	// Use the node page fetching concurrency for analysis extraction as well,
	// since both are bounded per-node parallel operations.
	analysisConcurrency := 1
	if cfg != nil && cfg.Concurrency.NodePageFetching > 0 {
		analysisConcurrency = cfg.Concurrency.NodePageFetching
	}

	c := &Collector{
		db:       db,
		cfg:      cfg,
		logger:   logger,
		resolver: resolver,
		analyser: analysis.New(db, logger, analysisConcurrency),
	}

	// Default client factory resolves credentials and builds real clients.
	c.clientFactory = c.defaultClientFactory

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// IsRunning returns true if a collection run is currently in progress.
func (c *Collector) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// ResumeResult summarises the outcome of evaluating interrupted runs on
// startup.
type ResumeResult struct {
	// Evaluated is the number of interrupted runs that were inspected.
	Evaluated int

	// Resumed is the number of runs that were resumed (still fresh enough).
	Resumed int

	// Abandoned is the number of runs that were too old and marked as failed.
	Abandoned int

	// Errors contains per-run errors keyed by collection run ID.
	Errors map[string]error

	// ResumedRunResult holds the RunResult from the resumed collection, if
	// any run was actually resumed and executed. Nil if no runs were resumed
	// or if the resume itself failed.
	ResumedRunResult *RunResult
}

// ResumeInterruptedRuns evaluates interrupted collection runs from a previous
// process and either resumes or abandons them according to the specification:
//
//   - If the run's started_at is within the last two collection intervals,
//     the run is considered fresh enough to resume. The collector re-runs
//     collection for organisations that were NOT already completed since
//     the interrupted run started.
//   - If the run is older than two collection intervals, it is marked as
//     "failed" with an error message and the next scheduled run starts fresh.
//
// This method should be called once during application startup, after
// migrations have been applied and stale "running" runs have been marked
// as "interrupted".
func (c *Collector) ResumeInterruptedRuns(ctx context.Context) (*ResumeResult, error) {
	if c.db == nil {
		return nil, fmt.Errorf("collector: database is nil")
	}
	log := c.logger.WithScope(logging.ScopeCollectionRun)

	result := &ResumeResult{
		Errors: make(map[string]error),
	}

	// Fetch all interrupted runs.
	interrupted, err := c.db.GetInterruptedCollectionRuns(ctx)
	if err != nil {
		return nil, fmt.Errorf("collector: listing interrupted runs: %w", err)
	}

	if len(interrupted) == 0 {
		log.Debug("no interrupted collection runs to evaluate")
		return result, nil
	}

	result.Evaluated = len(interrupted)
	log.Info(fmt.Sprintf("evaluating %d interrupted collection run(s) for possible resume", len(interrupted)))

	// Compute the freshness cutoff: two collection intervals from now.
	// Parse the cron schedule to determine the interval.
	collectionInterval := c.estimateCollectionInterval()
	freshnessCutoff := time.Now().Add(-2 * collectionInterval)

	// Track which organisations need collection (those without a completed
	// run since the interrupted run started).
	orgsNeedingCollection := make(map[string]datastore.Organisation)

	// Load all organisations once.
	allOrgs, err := c.db.ListOrganisations(ctx)
	if err != nil {
		return nil, fmt.Errorf("collector: listing organisations for resume: %w", err)
	}
	orgByID := make(map[string]datastore.Organisation, len(allOrgs))
	for _, org := range allOrgs {
		orgByID[org.Name] = org
	}

	for _, run := range interrupted {
		runLog := log

		// Check freshness.
		if run.StartedAt.Before(freshnessCutoff) {
			// Too old — abandon.
			reason := fmt.Sprintf("abandoned: interrupted run started at %s is older than two collection intervals (%s)",
				run.StartedAt.Format(time.RFC3339), collectionInterval)
			if _, abandonErr := c.db.AbandonCollectionRun(ctx, run.OrganisationName, reason); abandonErr != nil {
				result.Errors[run.OrganisationName] = abandonErr
				runLog.Warn(fmt.Sprintf("failed to abandon stale interrupted run %s: %v", run.OrganisationName, abandonErr))
			} else {
				result.Abandoned++
				runLog.Info(fmt.Sprintf("abandoned stale interrupted run %s (started %s, cutoff %s)",
					run.OrganisationName, run.StartedAt.Format(time.RFC3339), freshnessCutoff.Format(time.RFC3339)))
			}
			continue
		}

		// Fresh enough to resume. Determine which organisation this run
		// belongs to and whether it has already been completed by a
		// subsequent run.
		org, orgExists := orgByID[run.OrganisationName]
		if !orgExists {
			// Organisation was deleted since the run started — abandon.
			reason := "abandoned: organisation no longer exists"
			if _, abandonErr := c.db.AbandonCollectionRun(ctx, run.OrganisationName, reason); abandonErr != nil {
				result.Errors[run.OrganisationName] = abandonErr
			} else {
				result.Abandoned++
			}
			runLog.Info(fmt.Sprintf("abandoned interrupted run %s — organisation %s no longer exists",
				run.OrganisationName, run.OrganisationName))
			continue
		}

		// Check if this organisation already has a completed run since the
		// interrupted run started.
		completedRuns, cErr := c.db.ListCompletedRunsForOrganisation(ctx, run.OrganisationName, run.StartedAt)
		if cErr != nil {
			result.Errors[run.OrganisationName] = cErr
			runLog.Warn(fmt.Sprintf("failed to check completed runs for org %s: %v", org.Name, cErr))
			continue
		}

		if len(completedRuns) > 0 {
			// A newer completed run exists — this interrupted run's data is
			// superseded. Abandon it.
			reason := fmt.Sprintf("abandoned: organisation %s already has a completed run (%s) since this run started",
				org.Name, completedRuns[0].OrganisationName)
			if _, abandonErr := c.db.AbandonCollectionRun(ctx, run.OrganisationName, reason); abandonErr != nil {
				result.Errors[run.OrganisationName] = abandonErr
			} else {
				result.Abandoned++
			}
			runLog.Info(fmt.Sprintf("abandoned interrupted run %s for org %s — superseded by completed run %s",
				run.OrganisationName, org.Name, completedRuns[0].OrganisationName))
			continue
		}

		// This organisation needs re-collection. Mark the interrupted run
		// as abandoned (we'll create a fresh run for the organisation) and
		// queue the org for collection.
		reason := fmt.Sprintf("abandoned: will be re-collected as part of resume (checkpoint_start=%d)",
			run.CheckpointStart)
		if _, abandonErr := c.db.AbandonCollectionRun(ctx, run.OrganisationName, reason); abandonErr != nil {
			result.Errors[run.OrganisationName] = abandonErr
			runLog.Warn(fmt.Sprintf("failed to abandon interrupted run %s for re-collection: %v", run.OrganisationName, abandonErr))
			continue
		}

		orgsNeedingCollection[org.Name] = org
		result.Resumed++
		runLog.Info(fmt.Sprintf("will resume collection for org %s (interrupted run %s)",
			org.Name, run.OrganisationName))
	}

	// If any organisations need re-collection, run a targeted collection
	// for just those orgs.
	if len(orgsNeedingCollection) > 0 {
		log.Info(fmt.Sprintf("resuming collection for %d organisation(s)", len(orgsNeedingCollection)))
		runResult, runErr := c.runForOrganisations(ctx, orgsNeedingCollection)
		result.ResumedRunResult = runResult
		if runErr != nil {
			log.Error(fmt.Sprintf("resumed collection failed: %v", runErr))
			return result, runErr
		}
		log.Info(fmt.Sprintf("resumed collection completed: %d/%d orgs succeeded, %d nodes",
			runResult.SucceededOrgs, runResult.TotalOrgs, runResult.TotalNodes))
	}

	return result, nil
}

// estimateCollectionInterval parses the configured cron schedule and returns
// an approximate interval between runs. This is used to determine the
// freshness cutoff for interrupted run evaluation. Falls back to 1 hour if
// the schedule cannot be parsed.
func (c *Collector) estimateCollectionInterval() time.Duration {
	sched, err := ParseSchedule(c.cfg.Collection.Schedule)
	if err != nil {
		return 1 * time.Hour // safe default
	}

	now := time.Now()
	next1 := sched.Next(now)
	if next1.IsZero() {
		return 1 * time.Hour
	}
	next2 := sched.Next(next1)
	if next2.IsZero() {
		return 1 * time.Hour
	}

	interval := next2.Sub(next1)
	if interval <= 0 {
		return 1 * time.Hour
	}
	return interval
}

// runForOrganisations executes a collection run for a specific subset of
// organisations. This is used by ResumeInterruptedRuns to re-collect only
// the organisations that were interrupted.
func (c *Collector) runForOrganisations(ctx context.Context, orgs map[string]datastore.Organisation) (*RunResult, error) {
	if !c.tryStartRun() {
		return nil, fmt.Errorf("collector: a collection run is already in progress")
	}
	defer c.finishRun()

	start := time.Now()
	log := c.logger.WithScope(logging.ScopeCollectionRun)

	orgList := make([]datastore.Organisation, 0, len(orgs))
	for _, org := range orgs {
		orgList = append(orgList, org)
	}

	log.Info(fmt.Sprintf("starting resumed collection run for %d organisation(s)", len(orgList)))

	result := &RunResult{
		TotalOrgs:    len(orgList),
		Errors:       make(map[string]error, len(orgList)),
		OrgDurations: make(map[string]time.Duration, len(orgList)),
	}

	// Collect organisations in parallel, bounded by the configured
	// concurrency limit.
	concurrency := c.cfg.Concurrency.OrganisationCollection
	if concurrency <= 0 {
		concurrency = 1
	}

	resultsCh := make(chan orgResult, len(orgList))
	sem := make(chan struct{}, concurrency)

	var wg sync.WaitGroup
	for _, org := range orgList {
		wg.Add(1)
		go func(org datastore.Organisation) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				resultsCh <- orgResult{OrgName: org.Name, Err: ctx.Err()}
				return
			}

			orgStart := time.Now()
			nodes, cookbooks, orgErr := c.collectOrganisation(ctx, org)
			resultsCh <- orgResult{
				OrgName:   org.Name,
				Nodes:     nodes,
				Cookbooks: cookbooks,
				Duration:  time.Since(orgStart),
				Err:       orgErr,
			}
		}(org)
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	for or := range resultsCh {
		result.OrgDurations[or.OrgName] = or.Duration
		if or.Err != nil {
			result.FailedOrgs++
			result.Errors[or.OrgName] = or.Err
			log.Error(fmt.Sprintf("organisation %q: resumed collection failed after %s: %v",
				or.OrgName, or.Duration.Round(time.Millisecond), or.Err))
		} else {
			result.SucceededOrgs++
			result.TotalNodes += or.Nodes
			result.TotalCookbooks += or.Cookbooks
			log.Info(fmt.Sprintf("organisation %q: resumed collection completed — %d nodes, %d cookbook versions in %s",
				or.OrgName, or.Nodes, or.Cookbooks, or.Duration.Round(time.Millisecond)))
		}
	}

	result.Duration = time.Since(start)

	log.Info(fmt.Sprintf(
		"resumed collection run complete in %s: %d/%d orgs succeeded, %d nodes, %d cookbook versions",
		result.Duration.Round(time.Millisecond),
		result.SucceededOrgs, result.TotalOrgs,
		result.TotalNodes, result.TotalCookbooks,
	))

	// Re-materialise the role_summary aggregate table after the resumed run.
	if result.SucceededOrgs > 0 {
		c.recomputeRoleSummaries(ctx, log)
	}

	return result, nil
}

// RunResult summarises the outcome of a collection run.
type RunResult struct {
	// TotalOrgs is the number of organisations that were processed.
	TotalOrgs int

	// SucceededOrgs is the number of organisations that completed without
	// error.
	SucceededOrgs int

	// FailedOrgs is the number of organisations that encountered errors.
	FailedOrgs int

	// TotalNodes is the total number of nodes collected across all orgs.
	TotalNodes int

	// TotalCookbooks is the total number of cookbook versions upserted.
	TotalCookbooks int

	// Duration is the wall-clock time the run took.
	Duration time.Duration

	// Errors contains per-organisation errors, keyed by organisation name.
	Errors map[string]error

	// OrgDurations is the wall-clock time each organisation took, keyed by
	// organisation name. This covers the whole per-org pipeline (Steps 1-16),
	// unlike collection_runs.completed_at which is stamped early at Step 4b
	// once node snapshots persist and so excludes cookbook, cookstyle, role
	// and readiness work — usually the bulk of the time.
	OrgDurations map[string]time.Duration
}

// Run executes a single collection run across all configured organisations.
// It enforces the single-run constraint — if a run is already in progress,
// it returns immediately with an error.
//
// The run proceeds through the following steps for each organisation:
//  1. Create a collection_runs row (status = "running")
//  2. Collect all nodes via partial search
//  3. Persist node snapshots to the datastore
//  4. Fetch the cookbook inventory from the Chef server
//  5. Determine active/unused cookbooks
//  6. Upsert cookbook metadata
//  7. Flag stale nodes
//  8. Mark the collection run as "completed"
//
// If the context is cancelled (e.g. during graceful shutdown), in-progress
// runs are marked as "interrupted" with their checkpoint preserved.
func (c *Collector) Run(ctx context.Context) (*RunResult, error) {
	if !c.tryStartRun() {
		return nil, fmt.Errorf("collector: a collection run is already in progress")
	}
	defer c.finishRun()

	// Snapshot the live config at the start of the run so that changes
	// made via the admin UI (git_base_urls, target_chef_versions, etc.)
	// take effect without a restart.
	if c.configFn != nil {
		c.cfg = c.configFn()
	}

	if c.db == nil {
		return nil, fmt.Errorf("collector: database is nil")
	}

	start := time.Now()
	log := c.logger.WithScope(logging.ScopeCollectionRun)

	// Load all organisations from the database (includes both config-synced
	// and API-created orgs).
	orgs, err := c.db.ListOrganisations(ctx)
	if err != nil {
		return nil, fmt.Errorf("collector: listing organisations: %w", err)
	}

	if len(orgs) == 0 {
		log.Info("no organisations configured — skipping collection")
		return &RunResult{Duration: time.Since(start)}, nil
	}

	log.Info(fmt.Sprintf("starting collection run for %d organisation(s)", len(orgs)))

	result := &RunResult{
		TotalOrgs:    len(orgs),
		Errors:       make(map[string]error, len(orgs)),
		OrgDurations: make(map[string]time.Duration, len(orgs)),
	}

	// Collect organisations in parallel, bounded by the configured
	// concurrency limit.
	concurrency := c.cfg.Concurrency.OrganisationCollection
	if concurrency <= 0 {
		concurrency = 1
	}

	resultsCh := make(chan orgResult, len(orgs))
	sem := make(chan struct{}, concurrency)

	var wg sync.WaitGroup
	for _, org := range orgs {
		wg.Add(1)
		go func(org datastore.Organisation) {
			defer wg.Done()

			// Acquire semaphore slot.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				resultsCh <- orgResult{OrgName: org.Name, Err: ctx.Err()}
				return
			}

			orgStart := time.Now()
			nodes, cookbooks, orgErr := c.collectOrganisation(ctx, org)
			resultsCh <- orgResult{
				OrgName:   org.Name,
				Nodes:     nodes,
				Cookbooks: cookbooks,
				Duration:  time.Since(orgStart),
				Err:       orgErr,
			}
		}(org)
	}

	// Close results channel when all goroutines finish.
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results.
	for or := range resultsCh {
		result.OrgDurations[or.OrgName] = or.Duration
		if or.Err != nil {
			result.FailedOrgs++
			result.Errors[or.OrgName] = or.Err
			log.Error(fmt.Sprintf("organisation %q: collection failed after %s: %v",
				or.OrgName, or.Duration.Round(time.Millisecond), or.Err))
		} else {
			result.SucceededOrgs++
			result.TotalNodes += or.Nodes
			result.TotalCookbooks += or.Cookbooks
			log.Info(fmt.Sprintf("organisation %q: collected %d nodes, %d cookbook versions in %s",
				or.OrgName, or.Nodes, or.Cookbooks, or.Duration.Round(time.Millisecond)))
		}
	}

	result.Duration = time.Since(start)

	log.Info(fmt.Sprintf(
		"collection run complete in %s: %d/%d orgs succeeded, %d nodes, %d cookbook versions",
		result.Duration.Round(time.Millisecond),
		result.SucceededOrgs, result.TotalOrgs,
		result.TotalNodes, result.TotalCookbooks,
	))

	// Re-materialise the role_summary aggregate table now role_dependencies,
	// node_snapshots, cookstyle and kitchen results have settled for this run.
	if result.SucceededOrgs > 0 {
		c.recomputeRoleSummaries(ctx, log)
	}

	// Purge old log entries if retention is configured.
	if c.cfg.Logging.RetentionDays > 0 {
		purged, purgeErr := c.db.PurgeLogEntriesOlderThanDays(ctx, c.cfg.Logging.RetentionDays)
		if purgeErr != nil {
			log.Warn(fmt.Sprintf("log retention purge failed: %v", purgeErr))
		} else if purged > 0 {
			log.Info(fmt.Sprintf("purged %d log entries older than %d days", purged, c.cfg.Logging.RetentionDays))
		}
	}

	// Purge old collection runs (keep only the latest terminal run per org).
	purgedRuns, purgeRunsErr := c.db.PurgeOldCollectionRuns(ctx)
	if purgeRunsErr != nil {
		log.Warn(fmt.Sprintf("collection run purge failed: %v", purgeRunsErr))
	} else if purgedRuns > 0 {
		log.Info(fmt.Sprintf("purged %d old collection run(s)", purgedRuns))
	}

	// Purge metric snapshots older than 90 days. These are small rows used
	// for dashboard trend charts; 90 days gives ample historical context.
	metricCutoff := time.Now().Add(-90 * 24 * time.Hour)
	purgedMetrics, purgeMetricsErr := c.db.PurgeMetricSnapshotsOlderThan(ctx, metricCutoff)
	if purgeMetricsErr != nil {
		log.Warn(fmt.Sprintf("metric snapshot purge failed: %v", purgeMetricsErr))
	} else if purgedMetrics > 0 {
		log.Info(fmt.Sprintf("purged %d metric snapshot(s) older than 90 days", purgedMetrics))
	}

	// Purge expired export job rows. The export cleanup ticker marks rows
	// as 'expired' after deleting files from disk; this removes the DB
	// rows after the log retention period so they don't accumulate
	// indefinitely.
	exportCutoff := time.Now().Add(-time.Duration(c.cfg.Logging.RetentionDays) * 24 * time.Hour)
	purgedExports, purgeExportsErr := c.db.DeleteExpiredExportJobRows(ctx, exportCutoff)
	if purgeExportsErr != nil {
		log.Warn(fmt.Sprintf("export job row purge failed: %v", purgeExportsErr))
	} else if purgedExports > 0 {
		log.Info(fmt.Sprintf("purged %d expired export job row(s) older than %d days", purgedExports, c.cfg.Logging.RetentionDays))
	}

	return result, nil
}

// recomputeRoleSummaries re-materialises the role_summary aggregate table after
// a collection run: structural columns (node_count, cookbook counts), plus the
// active-target compatibility and TK rollups. It is a single global bulk pass
// (not per-org), best-effort and non-fatal — the roles list reads these columns
// instead of expanding a recursive CTE over all roles per request. See
// internal/datastore/role_summary_recompute.go.
func (c *Collector) recomputeRoleSummaries(ctx context.Context, log *logging.ScopedLogger) {
	if err := c.db.RecomputeAllRoleStructural(ctx); err != nil {
		log.Warn(fmt.Sprintf("role_summary structural recompute failed: %v", err))
	}
	if target := c.cfg.TargetChefVersion; target != "" {
		if err := c.db.RecomputeAllRoleCompatStatus(ctx, target); err != nil {
			log.Warn(fmt.Sprintf("role_summary compat recompute failed: %v", err))
		}
	}
	if err := c.db.RecomputeAllRoleTKStatus(ctx); err != nil {
		log.Warn(fmt.Sprintf("role_summary TK recompute failed: %v", err))
	}
}

// nodeDiskVerdict computes the version-invariant disk-space verdict to store on a
// node snapshot. The required size is platform-only, so it is always returned;
// the sufficiency + available space are returned indeterminate (nil) for a stale
// node, whose reported free space is old — matching readiness evaluation.
func nodeDiskVerdict(filesystem json.RawMessage, platform string, stale bool, cfg analysis.DiskConfig) (sufficient *bool, availableMB *int, requiredMB *int) {
	v := analysis.EvaluateDisk(filesystem, platform, cfg)
	req := v.RequiredMB
	if stale {
		return nil, nil, &req
	}
	return v.Sufficient, v.AvailableMB, &req
}

// collectOrganisation runs the full collection sequence for a single
// organisation. It returns the number of nodes collected and cookbook
// versions upserted.
func (c *Collector) collectOrganisation(ctx context.Context, org datastore.Organisation) (nodes int, cookbooks int, err error) {
	log := c.logger.WithScope(logging.ScopeCollectionRun, logging.WithOrganisation(org.Name))

	// Step 1: Create a collection run row.
	run, err := c.db.CreateCollectionRun(ctx, datastore.CreateCollectionRunParams{
		OrganisationName: org.Name,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("creating collection run: %w", err)
	}

	log.Info(fmt.Sprintf("collection run %s started", run.OrganisationName),
		logging.WithCollectionRunID(run.OrganisationName))

	// Timers for the two phases an organisation splits into. The snapshot
	// phase ends at Step 4b, which is also where collection_runs.completed_at
	// is stamped — so that column reports only this first phase. The tail
	// (Steps 5-16: cookbooks, CookStyle, roles, readiness) is usually the
	// larger share and was previously invisible in both the logs and the DB.
	orgStart := time.Now()

	// runCompleted is set to true once the collection run has been marked
	// as completed in Step 4b (after node snapshots are persisted). Once
	// set, the deferred error handler must NOT overwrite the completed
	// status — post-completion errors in cookbook operations are non-fatal
	// and should not regress the run status.
	runCompleted := false

	// Ensure we mark the run as completed or failed on exit.
	defer func() {
		if err != nil && !runCompleted {
			errMsg := err.Error()
			if ctx.Err() != nil {
				// Context cancelled — mark as interrupted, not failed.
				if _, intErr := c.db.InterruptCollectionRun(context.Background(), run.OrganisationName); intErr != nil {
					log.Error(fmt.Sprintf("failed to mark run %s as interrupted: %v", run.OrganisationName, intErr),
						logging.WithCollectionRunID(run.OrganisationName))
				} else {
					log.Warn(fmt.Sprintf("collection run %s interrupted", run.OrganisationName),
						logging.WithCollectionRunID(run.OrganisationName))
				}
				return
			}
			if _, failErr := c.db.FailCollectionRun(context.Background(), run.OrganisationName, errMsg); failErr != nil {
				log.Error(fmt.Sprintf("failed to mark run %s as failed: %v", run.OrganisationName, failErr),
					logging.WithCollectionRunID(run.OrganisationName))
			}
		}
	}()

	// Step 2: Build a Chef API client for this organisation.
	client, err := c.clientFactory(ctx, org)
	if err != nil {
		return 0, 0, fmt.Errorf("creating Chef API client: %w", err)
	}

	// Step 3: Collect all nodes via concurrent partial search.
	pageConcurrency := c.cfg.Concurrency.NodePageFetching
	if pageConcurrency <= 0 {
		pageConcurrency = 1
	}

	// Compute any additional partial-search keys needed for CMDB ownership
	// attributes. When cmdb_attribute rules are configured, the search
	// request includes keys like "itil.cmdb.node" → ["itil","cmdb","node"]
	// so the Chef server returns the CMDB subtree for each node.
	cmdbSearchKeys := c.cfg.Ownership.CMDBSearchKeys()

	log.Info("collecting nodes via partial search",
		logging.WithCollectionRunID(run.OrganisationName))

	searchRows, err := client.CollectAllNodesConcurrent(ctx, 1000, pageConcurrency, cmdbSearchKeys)
	if err != nil {
		return 0, 0, fmt.Errorf("collecting nodes: %w", err)
	}

	log.Info(fmt.Sprintf("fetched %d nodes from Chef server", len(searchRows)),
		logging.WithCollectionRunID(run.OrganisationName))

	// Step 4: Convert search results to node snapshot params and persist.
	now := time.Now().UTC()
	staleThreshold := time.Duration(c.cfg.Collection.StaleNodeThresholdDays) * 24 * time.Hour
	staleCookbookThreshold := time.Duration(c.cfg.Collection.StaleCookbookThresholdDays) * 24 * time.Hour

	// Track which cookbook names are in active use by at least one node,
	// and record per-node cookbook versions for building usage records later.
	// We maintain two sets:
	//   - allCookbookNames: every cookbook referenced by any node (for usage records)
	//   - activeCookbookNames: only cookbooks referenced by non-stale nodes
	//     (for marking active status and triggering downloads)
	// This avoids downloading cookbooks that are only used by stale nodes,
	// which can be very expensive when there are many stale nodes.
	allCookbookNames := make(map[string]bool)
	activeCookbookNames := make(map[string]bool)
	activeCookbookVersions := make(map[string]map[string]bool) // name → set of versions

	// Build NodeRecord slice for usage analysis (populated alongside snapshot params).
	nodeRecords := make([]analysis.NodeRecord, 0, len(searchRows))

	// The disk-space verdict is version-invariant (filesystem + platform install
	// size only), so compute it once per node here and store it on the snapshot —
	// available regardless of whether any target Chef version is configured
	// (decoupled from per-target readiness evaluation). Read live from c.cfg.
	diskCfg := analysis.DiskConfig{
		InstallPathLinux:        c.cfg.Readiness.InstallPathLinux,
		InstallPathWindows:      c.cfg.Readiness.InstallPathWindows,
		InstallSizeMBLinux:      c.cfg.Readiness.InstallSizeMBLinux,
		InstallSizeMBWindows:    c.cfg.Readiness.InstallSizeMBWindows,
		MinRemainingFreePercent: c.cfg.Readiness.MinRemainingFreePercent,
	}

	snapshotParams := make([]datastore.InsertNodeSnapshotParams, 0, len(searchRows))
	for _, row := range searchRows {
		nd := chefapi.NewNodeData(row.Data)

		// Marshal complex fields to JSON for storage.
		fsJSON, _ := json.Marshal(nd.Filesystem())
		cbJSON, _ := json.Marshal(nd.Cookbooks())
		rlJSON, _ := json.Marshal(nd.RunList())
		rolesJSON, _ := json.Marshal(nd.Roles())

		// Track cookbooks and per-node cookbook versions. Compute staleness
		// up front so we can separate active-node cookbooks from stale-node
		// cookbooks. Only cookbooks referenced by non-stale nodes are
		// candidates for download — this avoids fetching thousands of
		// cookbook versions that are only used by nodes that haven't
		// checked in.
		nodeIsStale := nd.IsStale(staleThreshold)
		cbVersions := nd.CookbookVersions()
		for cbName, cbVer := range cbVersions {
			allCookbookNames[cbName] = true
			if !nodeIsStale {
				activeCookbookNames[cbName] = true
				if activeCookbookVersions[cbName] == nil {
					activeCookbookVersions[cbName] = make(map[string]bool)
				}
				activeCookbookVersions[cbName][cbVer] = true
			}
		}

		// Build a NodeRecord for usage analysis from the in-memory data,
		// avoiding a re-read from the database after persistence.
		nodeRecords = append(nodeRecords, analysis.NodeRecordFromCollectedData(
			nd.Name(),
			nd.Platform(),
			nd.PlatformVersion(),
			nd.PlatformFamily(),
			nd.Roles(),
			nd.PolicyName(),
			nd.PolicyGroup(),
			cbVersions,
		))

		// Build custom attributes from CMDB search keys and any other
		// extra attributes returned by the partial search. Each CMDB key
		// (e.g. "itil.cmdb.node") is stored as-is in the flat map so the
		// ownership evaluator can look up values by dot-separated path.
		var customAttrsJSON json.RawMessage
		if len(cmdbSearchKeys) > 0 {
			customAttrs := make(map[string]interface{})
			for key := range cmdbSearchKeys {
				if val, ok := nd.Raw[key]; ok && val != nil {
					customAttrs[key] = val
				}
			}
			if len(customAttrs) > 0 {
				customAttrsJSON, _ = json.Marshal(customAttrs)
			}
		}

		diskSufficient, diskAvailable, diskRequired := nodeDiskVerdict(fsJSON, nd.Platform(), nodeIsStale, diskCfg)

		snapshotParams = append(snapshotParams, datastore.InsertNodeSnapshotParams{
			CollectionRunOrg:     run.OrganisationName,
			OrganisationName:     org.Name,
			NodeName:             nd.Name(),
			ChefEnvironment:      nd.ChefEnvironment(),
			ChefVersion:          nd.ChefVersion(),
			Platform:             nd.Platform(),
			PlatformVersion:      nd.PlatformVersion(),
			PlatformFamily:       nd.PlatformFamily(),
			PlatformCaption:      nd.PlatformCaption(),
			Filesystem:           fsJSON,
			Cookbooks:            cbJSON,
			RunList:              rlJSON,
			Roles:                rolesJSON,
			Tags:                 nd.Tags(),
			PolicyName:           nd.PolicyName(),
			PolicyGroup:          nd.PolicyGroup(),
			OhaiTime:             nd.OhaiTime(),
			CustomAttributes:     customAttrsJSON,
			IsStale:              nodeIsStale,
			CollectedAt:          now,
			MigrationState:       nd.MigrationState(),
			ActiveChefVersion:    nd.ActiveChefVersion(),
			DormantInstalled:     boolPtr(nd.DormantInstalled(), nd.HasMigrationData()),
			DormantChefVersion:   nd.DormantChefVersion(),
			TargetVersion:        nd.TargetVersion(),
			TargetExecutionTime:  nd.TargetExecutionTime(),
			TargetConvergeStatus: nd.TargetConvergeStatus(),
			SufficientDiskSpace:  diskSufficient,
			AvailableDiskMB:      diskAvailable,
			RequiredDiskMB:       diskRequired,
		})
	}

	// Log the impact of stale-node filtering on cookbook counts so operators
	// can see how many cookbooks are skipped for download.
	staleOnlyCount := len(allCookbookNames) - len(activeCookbookNames)
	staleNodeCount := 0
	for _, p := range snapshotParams {
		if p.IsStale {
			staleNodeCount++
		}
	}
	log.Info(fmt.Sprintf(
		"node staleness summary: %d total nodes, %d stale, %d active; "+
			"cookbook names: %d total, %d from active nodes, %d only from stale nodes (will not be downloaded)",
		len(snapshotParams), staleNodeCount, len(snapshotParams)-staleNodeCount,
		len(allCookbookNames), len(activeCookbookNames), staleOnlyCount),
		logging.WithCollectionRunID(run.OrganisationName))

	// Deduplicate snapshot params before persisting. The Chef Server's
	// partial search can return the same node name more than once when
	// pagination boundaries shift between pages (a node is updated
	// mid-collection) or when a node is re-registered. PostgreSQL's
	// INSERT ... ON CONFLICT DO UPDATE rejects two rows with the same
	// conflict key in a single statement (error 21000), so we collapse
	// duplicates here — last occurrence wins (freshest data).
	snapshotParams, dupCount := deduplicateSnapshotParams(snapshotParams)
	if dupCount > 0 {
		log.Warn(fmt.Sprintf("deduplicated %d duplicate node snapshot(s) from Chef Server search results", dupCount),
			logging.WithCollectionRunID(run.OrganisationName))
	}

	// Persist node snapshots in bulk.
	inserted, err := c.db.BulkUpsertNodeSnapshots(ctx, snapshotParams)
	if err != nil {
		return 0, 0, fmt.Errorf("persisting node snapshots: %w", err)
	}
	nodes = inserted

	// Update progress on the collection run.
	if _, err := c.db.UpdateCollectionRunProgress(ctx, datastore.UpdateCollectionRunProgressParams{
		OrganisationName: run.OrganisationName,
		TotalNodes:       len(searchRows),
		NodesCollected:   inserted,
	}); err != nil {
		log.Warn(fmt.Sprintf("failed to update collection run progress: %v", err),
			logging.WithCollectionRunID(run.OrganisationName))
	}

	// Step 4a: Remove snapshots for nodes no longer on the Chef Server.
	// Nodes that were decommissioned won't appear in searchRows, so their
	// snapshot rows become orphaned. This reconciliation step keeps the
	// table proportional to the current fleet size.
	if len(snapshotParams) > 0 {
		activeNames := make([]string, len(snapshotParams))
		for i, p := range snapshotParams {
			activeNames[i] = p.NodeName
		}
		orphaned, orphanErr := c.db.DeleteOrphanedNodeSnapshots(ctx, org.Name, activeNames)
		if orphanErr != nil {
			log.Warn(fmt.Sprintf("failed to clean up orphaned node snapshots: %v", orphanErr),
				logging.WithCollectionRunID(run.OrganisationName))
		} else if orphaned > 0 {
			log.Info(fmt.Sprintf("removed %d orphaned node snapshot(s) (decommissioned nodes)", orphaned),
				logging.WithCollectionRunID(run.OrganisationName))
		}
	}

	// Step 4b: Complete the collection run early so the UI can show fresh
	// node data immediately. The remaining steps (cookbook inventory,
	// downloads, analysis, CookStyle, etc.) can take a very long time with
	// large fleets and the UI queries only show nodes from the latest
	// *completed* run. By completing now, users see up-to-date node/stale
	// status while the heavier cookbook operations continue in the background.
	if _, completeErr := c.db.CompleteCollectionRun(ctx, run.OrganisationName, len(searchRows), inserted); completeErr != nil {
		log.Error(fmt.Sprintf("failed to mark run %s as completed after node collection: %v", run.OrganisationName, completeErr),
			logging.WithCollectionRunID(run.OrganisationName))
		// Non-fatal — continue with cookbook operations even if the status
		// update failed. The deferred error handler will still attempt to
		// mark the run appropriately on exit.
	} else {
		runCompleted = true
		snapshotPhase := time.Since(orgStart)
		log.Info(fmt.Sprintf("collection run %s marked completed with %d nodes in %s (continuing with cookbook operations)",
			run.OrganisationName, inserted, snapshotPhase.Round(time.Millisecond)),
			logging.WithCollectionRunID(run.OrganisationName))
	}

	// Step 4c: Record metric snapshots for trend charts. These
	// pre-aggregated snapshots allow the dashboard to show historical
	// trends without scanning the (now current-state-only) node_snapshots
	// table.
	c.recordMetricSnapshots(ctx, log, run.OrganisationName, org.Name, snapshotParams)

	// Step 5: Fetch cookbook inventory from the Chef server.
	log.Info("fetching cookbook inventory",
		logging.WithCollectionRunID(run.OrganisationName))

	serverCookbooks, err := client.GetCookbooks(ctx)
	if err != nil {
		// After early completion in Step 4b, cookbook inventory failures are
		// non-fatal — node data is already visible in the UI. Log and skip
		// the remaining cookbook operations.
		if runCompleted {
			log.Warn(fmt.Sprintf("fetching cookbook inventory failed (non-fatal, nodes already committed): %v", err),
				logging.WithCollectionRunID(run.OrganisationName))
			err = nil
			return nodes, 0, nil
		}
		return nodes, 0, fmt.Errorf("fetching cookbook inventory: %w", err)
	}

	// Step 6: Upsert cookbook metadata and determine active/unused status.
	// Use activeCookbookNames (non-stale only) for the is_active flag,
	// consistent with MarkCookbooksActiveForOrg which overwrites it moments
	// later. This ensures a cookbook only used by stale nodes is never
	// transiently marked active between the upsert and the bulk update.
	cookbookParams := make([]datastore.UpsertServerCookbookParams, 0)
	for cbName, entry := range serverCookbooks {
		isActive := activeCookbookNames[cbName]

		for _, ver := range entry.Versions {
			// A cookbook is stale if it has not been updated in a long time.
			// We use FirstSeenAt as a proxy — on first insert, it records
			// when we first observed this version. The stale flag is evaluated
			// against the threshold on upsert.
			cookbookParams = append(cookbookParams, datastore.UpsertServerCookbookParams{
				OrganisationName: org.Name,
				Name:             cbName,
				Version:          ver.Version,
				IsActive:         isActive,
				IsStaleCookbook:  false, // Will be updated below
				FirstSeenAt:      now,
				LastFetchedAt:    now,
			})
		}
	}

	upserted, err := c.db.BulkUpsertServerCookbooks(ctx, cookbookParams)
	if err != nil {
		// After early completion in Step 4b, upsert failures are non-fatal —
		// node data is already visible in the UI. Log and skip the remaining
		// cookbook operations.
		if runCompleted {
			log.Warn(fmt.Sprintf("upserting cookbook metadata failed (non-fatal, nodes already committed): %v", err),
				logging.WithCollectionRunID(run.OrganisationName))
			err = nil
			return nodes, 0, nil
		}
		return nodes, 0, fmt.Errorf("upserting cookbook metadata: %w", err)
	}
	cookbooks = upserted

	// Mark active/unused cookbooks for this organisation.
	if err := c.db.MarkServerCookbooksActiveForOrg(ctx, org.Name, activeCookbookVersions); err != nil {
		log.Warn(fmt.Sprintf("failed to mark active cookbooks: %v", err),
			logging.WithCollectionRunID(run.OrganisationName))
	}

	// Step 7: Evaluate stale cookbook flag. A cookbook is stale if the most
	// recent version's first_seen_at is older than the configured threshold.
	// This is done via a database update for cookbooks belonging to this org.
	staleCookbookCutoff := now.Add(-staleCookbookThreshold)
	staleCount, staleErr := c.db.MarkStaleServerCookbooksForOrg(ctx, org.Name, staleCookbookCutoff)
	if staleErr != nil {
		log.Warn(fmt.Sprintf("failed to mark stale cookbooks: %v", staleErr),
			logging.WithCollectionRunID(run.OrganisationName))
	} else if staleCount > 0 {
		log.Info(fmt.Sprintf("marked %d cookbook(s) as stale (first seen before %s)",
			staleCount, staleCookbookCutoff.Format(time.RFC3339)),
			logging.WithCollectionRunID(run.OrganisationName))
	}

	// -----------------------------------------------------------------------
	// Concurrent processing groups
	//
	// Three independent groups of work run in parallel after node data is
	// committed (Step 4b). This maximises throughput by overlapping network
	// I/O (Chef Server downloads, git clones) with CPU work (CookStyle
	// scans) and lightweight DB operations (role deps, usage analysis).
	//
	// Group A: Server cookbook pipeline — download + scan + autocorrect
	// Group B: Git repo pipeline — clone/pull → CookStyle → TK → autocorrect
	// Group C: Role dependency graph + cookbook usage analysis
	//
	// Step 14 (readiness evaluation) runs after all groups complete because
	// it depends on CookStyle + TK results from groups A and B.
	// -----------------------------------------------------------------------

	fetchConcurrency := c.cfg.Concurrency.GitPull
	if fetchConcurrency <= 0 {
		fetchConcurrency = 1
	}

	var parallelWG sync.WaitGroup

	// -------------------------------------------------------------------
	// Group A: Server cookbook pipeline (download → scan → cleanup)
	// -------------------------------------------------------------------
	parallelWG.Add(1)
	go func() {
		defer parallelWG.Done()

		if c.cfg.Collection.SkipServerCookbookDownload {
			log.Info("skipping Chef server cookbook download (collection.skip_server_cookbook_download is enabled)",
				logging.WithCollectionRunID(run.OrganisationName))
			return
		}

		deleteAfterScan := c.cfg.Collection.DeleteServerCookbooksAfterScanEnabled()
		if deleteAfterScan {
			log.Info("running server cookbook pipeline (download → scan → delete)",
				logging.WithCollectionRunID(run.OrganisationName))
		} else {
			log.Info("running server cookbook pipeline (download → scan; retaining files on disk)",
				logging.WithCollectionRunID(run.OrganisationName))
		}

		pipelineResult := runServerCookbookPipeline(
			ctx, client, c.db, log, org,
			c.cookbookCacheDir,
			c.cfg.TargetChefVersionList(),
			c.cookstyleScanner,
			c.autocorrectGen,
			deleteAfterScan,
			c.cfg.Concurrency.CookbookDownload,
			c.cfg.Concurrency.CookstyleScan,
		)

		if pipelineResult.Total == 0 {
			log.Info("no server cookbook versions need processing",
				logging.WithCollectionRunID(run.OrganisationName))
		} else {
			if deleteAfterScan {
				log.Info(fmt.Sprintf(
					"server cookbook pipeline complete: %d total, %d downloaded, %d scanned, %d skipped, %d failed, %d legacy cached cleaned in %s",
					pipelineResult.Total, pipelineResult.Downloaded, pipelineResult.Scanned,
					pipelineResult.Skipped, pipelineResult.Failed, pipelineResult.Cleaned,
					pipelineResult.Duration.Round(time.Millisecond)),
					logging.WithCollectionRunID(run.OrganisationName))
			} else {
				log.Info(fmt.Sprintf(
					"server cookbook pipeline complete: %d total, %d downloaded, %d scanned, %d skipped, %d failed in %s",
					pipelineResult.Total, pipelineResult.Downloaded, pipelineResult.Scanned,
					pipelineResult.Skipped, pipelineResult.Failed,
					pipelineResult.Duration.Round(time.Millisecond)),
					logging.WithCollectionRunID(run.OrganisationName))
			}
		}

		for _, fe := range pipelineResult.Errors {
			log.Warn(fmt.Sprintf("server cookbook pipeline error: %s/%s: %v", fe.Name, fe.Version, fe.Err),
				logging.WithCollectionRunID(run.OrganisationName))
		}

		// Complexity scoring for server cookbooks runs after the pipeline
		// completes so CookStyle results are available.
		if c.complexityScorer != nil && c.cfg.TargetChefVersion != "" {
			orgCBs, cxListErr := c.db.ListServerCookbooksByOrganisation(ctx, org.Name)
			if cxListErr != nil {
				log.Warn(fmt.Sprintf("failed to list server cookbooks for complexity scoring: %v", cxListErr),
					logging.WithCollectionRunID(run.OrganisationName))
			} else {
				cxBatch := c.complexityScorer.ScoreServerCookbooks(ctx, orgCBs, c.cfg.TargetChefVersion, org.Name)
				log.Info(fmt.Sprintf(
					"server cookbook complexity scoring complete: %d total, %d scored, %d skipped, %d errors in %s",
					cxBatch.Total, cxBatch.Scored, cxBatch.Skipped, cxBatch.Errors,
					cxBatch.Duration.Round(time.Millisecond)),
					logging.WithCollectionRunID(run.OrganisationName))
			}
		}

		// Record complexity metric snapshots for trend charts.
		c.recordComplexitySnapshots(ctx, log, run.OrganisationName, org.Name, c.cfg.TargetChefVersionList())
	}()

	// -------------------------------------------------------------------
	// Group B: Git repo pipeline
	//   1. Clone/pull repos
	//   2. CookStyle scan (starts immediately for already-cloned repos,
	//      and for newly cloned repos as soon as clone completes)
	//   3. Test Kitchen
	//   4. Autocorrect previews + complexity scoring
	// -------------------------------------------------------------------
	parallelWG.Add(1)
	go func() {
		defer parallelWG.Done()

		// B.1: Scan already-cloned git repos immediately (before new
		// clones finish). Repos whose HEAD hasn't changed since the last
		// scan are skipped automatically by scanOneGitRepo.
		if c.cookstyleScanner != nil && c.gitRepoDirFn != nil && c.cfg.TargetChefVersion != "" {
			existingRepos, listErr := c.db.ListClonedGitRepos(ctx)
			if listErr != nil {
				log.Warn(fmt.Sprintf("failed to list existing git repos for immediate CookStyle scanning: %v", listErr),
					logging.WithCollectionRunID(run.OrganisationName))
			} else if len(existingRepos) > 0 {
				log.Info(fmt.Sprintf("scanning %d existing git repo(s) for CookStyle (before clone/pull)", len(existingRepos)),
					logging.WithCollectionRunID(run.OrganisationName))

				csBatch := c.cookstyleScanner.ScanGitRepos(ctx, existingRepos, c.cfg.TargetChefVersion, c.gitRepoDirFn)
				log.Info(fmt.Sprintf(
					"CookStyle pre-scan complete (existing git repos): %d total, %d scanned, %d skipped, %d passed, %d failed, %d errors in %s",
					csBatch.Total, csBatch.Scanned, csBatch.Skipped,
					csBatch.Passed, csBatch.Failed, csBatch.Errors,
					csBatch.Duration.Round(time.Millisecond)),
					logging.WithCollectionRunID(run.OrganisationName))
			}
		}

		// B.2: Clone/pull git repos.
		if len(c.cfg.GitBaseURLs) > 0 {
			gitLog := c.logger.WithScope(logging.ScopeGitOperation, logging.WithOrganisation(org.Name))

			gitLog.Info(fmt.Sprintf("fetching git cookbooks across %d base URL(s) for %d active cookbook(s)",
				len(c.cfg.GitBaseURLs), len(activeCookbookNames)),
				logging.WithCollectionRunID(run.OrganisationName))

			gitDir := c.gitCookbookDir
			if gitDir == "" {
				gitDir = filepath.Join(os.TempDir(), "chef-migration-metrics", "git-cookbooks")
			}
			gitMgr := NewGitCookbookManager(gitDir, nil)

			gitResult := fetchGitCookbooks(ctx, gitMgr, c.db, gitLog, c.cfg.GitBaseURLs, activeCookbookNames, fetchConcurrency)

			if gitResult.Total == 0 {
				gitLog.Info("no git cookbook candidates to fetch",
					logging.WithCollectionRunID(run.OrganisationName))
			} else {
				gitLog.Info(fmt.Sprintf(
					"git cookbook fetch complete: %d total, %d cloned, %d pulled, %d unchanged, %d failed in %s",
					gitResult.Total, gitResult.Cloned, gitResult.Pulled,
					gitResult.Unchanged, gitResult.Failed,
					gitResult.Duration.Round(time.Millisecond)),
					logging.WithCollectionRunID(run.OrganisationName))
			}
		}

		// B.3: CookStyle scan for newly cloned/pulled repos. The scanner's
		// skip logic detects repos already scanned in B.1 (same commit SHA)
		// and skips them, so only repos with new commits are re-scanned.
		if c.cookstyleScanner != nil && c.gitRepoDirFn != nil && c.cfg.TargetChefVersion != "" {
			gitRepos, gitListErr := c.db.ListClonedGitRepos(ctx)
			if gitListErr != nil {
				log.Warn(fmt.Sprintf("failed to list git repos for post-clone CookStyle scanning: %v", gitListErr),
					logging.WithCollectionRunID(run.OrganisationName))
			} else if len(gitRepos) > 0 {
				log.Info(fmt.Sprintf("scanning %d git repo(s) for CookStyle (post clone/pull)", len(gitRepos)),
					logging.WithCollectionRunID(run.OrganisationName))

				csBatch := c.cookstyleScanner.ScanGitRepos(ctx, gitRepos, c.cfg.TargetChefVersion, c.gitRepoDirFn)
				log.Info(fmt.Sprintf(
					"CookStyle post-scan complete (git repos): %d total, %d scanned, %d skipped, %d passed, %d failed, %d errors in %s",
					csBatch.Total, csBatch.Scanned, csBatch.Skipped,
					csBatch.Passed, csBatch.Failed, csBatch.Errors,
					csBatch.Duration.Round(time.Millisecond)),
					logging.WithCollectionRunID(run.OrganisationName))
			}
		} else if c.cookstyleScanner != nil && c.gitRepoDirFn == nil {
			log.Debug("skipping CookStyle scanning — no git repo directory resolver configured",
				logging.WithCollectionRunID(run.OrganisationName))
		}

		// B.3a: Kitchen config analysis — discover platforms, drivers, suites.
		if c.kitchenAnalyser != nil && c.gitRepoDirFn != nil {
			analysisRepos, analysisListErr := c.db.ListClonedGitRepos(ctx)
			if analysisListErr != nil {
				log.Warn(fmt.Sprintf("failed to list git repos for kitchen analysis: %v", analysisListErr),
					logging.WithCollectionRunID(run.OrganisationName))
			} else {
				log.Info(fmt.Sprintf("analysing kitchen configs for %d git repo(s)", len(analysisRepos)),
					logging.WithCollectionRunID(run.OrganisationName))

				batch := c.kitchenAnalyser.AnalyseAll(ctx, analysisRepos, c.gitRepoDirFn)
				log.Info(fmt.Sprintf(
					"kitchen analysis complete: %d total, %d analysed, %d skipped, %d errors in %s",
					batch.Total, batch.Analysed, batch.Skipped, batch.Errors,
					batch.Duration.Round(time.Millisecond)),
					logging.WithCollectionRunID(run.OrganisationName))
			}
		} else if c.kitchenAnalyser != nil && c.gitRepoDirFn == nil {
			log.Debug("skipping kitchen analysis — no git repo directory resolver configured",
				logging.WithCollectionRunID(run.OrganisationName))
		}

		// B.5: Autocorrect previews for git repos.
		if c.autocorrectGen != nil && c.gitRepoDirFn != nil {
			gitReposForAC, gitRepoListErr := c.db.ListClonedGitRepos(ctx)
			if gitRepoListErr != nil {
				log.Warn(fmt.Sprintf("failed to list git repos for autocorrect previews: %v", gitRepoListErr),
					logging.WithCollectionRunID(run.OrganisationName))
			} else if len(gitReposForAC) > 0 {
				var csResults []datastore.GitRepoCookstyleResult
				for _, repo := range gitReposForAC {
					repoResults, err := c.db.ListGitRepoCookstyleResults(ctx, repo.Name, repo.GitRepoURL)
					if err != nil {
						log.Warn(fmt.Sprintf("failed to list CookStyle results for git repo %s: %v", repo.Name, err),
							logging.WithCollectionRunID(run.OrganisationName))
						continue
					}
					csResults = append(csResults, repoResults...)
				}

				if len(csResults) > 0 {
					log.Info(fmt.Sprintf("generating autocorrect previews for %d git CookStyle result(s)", len(csResults)),
						logging.WithCollectionRunID(run.OrganisationName))

					csInfos := make([]remediation.CookstyleResultInfo, 0, len(csResults))
					for _, csr := range csResults {
						csInfos = append(csInfos, remediation.CookstyleResultInfo{
							CookbookName:      csr.GitRepoName,
							GitRepoURL:        csr.GitRepoURL,
							TargetChefVersion: csr.TargetChefVersion,
							OffenseCount:      csr.OffenceCount,
							Passed:            csr.Passed,
							Source:            remediation.SourceGitRepo,
						})
					}

					repoByID := make(map[string]datastore.GitRepo, len(gitReposForAC))
					for _, repo := range gitReposForAC {
						repoByID[repo.Name] = repo
					}
					dirFn := func(cookbookID string) string {
						repo, ok := repoByID[cookbookID]
						if !ok {
							return ""
						}
						return c.gitRepoDirFn(repo)
					}

					acBatch := c.autocorrectGen.GeneratePreviews(ctx, csInfos, dirFn)
					log.Info(fmt.Sprintf(
						"autocorrect previews complete: %d total, %d generated, %d skipped, %d errors in %s",
						acBatch.Total, acBatch.Generated, acBatch.Skipped, acBatch.Errors,
						acBatch.Duration.Round(time.Millisecond)),
						logging.WithCollectionRunID(run.OrganisationName))
				}
			}
		}

		// B.6: Complexity scoring for git repos.
		if c.complexityScorer != nil && c.cfg.TargetChefVersion != "" {
			gitReposForCX, grCXListErr := c.db.ListClonedGitRepos(ctx)
			if grCXListErr != nil {
				log.Warn(fmt.Sprintf("failed to list git repos for complexity scoring: %v", grCXListErr),
					logging.WithCollectionRunID(run.OrganisationName))
			} else if len(gitReposForCX) > 0 {
				grCXBatch := c.complexityScorer.ScoreGitRepos(ctx, gitReposForCX, c.cfg.TargetChefVersion, org.Name)
				log.Info(fmt.Sprintf(
					"git repo complexity scoring complete: %d total, %d scored, %d skipped, %d errors in %s",
					grCXBatch.Total, grCXBatch.Scored, grCXBatch.Skipped, grCXBatch.Errors,
					grCXBatch.Duration.Round(time.Millisecond)),
					logging.WithCollectionRunID(run.OrganisationName))
			}
		}
	}()

	// -------------------------------------------------------------------
	// Group C: Independent operations (role deps + usage analysis)
	// These only need the Chef API client and data from Steps 1–6 which
	// are already complete.
	// -------------------------------------------------------------------
	parallelWG.Add(1)
	go func() {
		defer parallelWG.Done()

		// C.1: Build role dependency graph.
		log.Info("building role dependency graph",
			logging.WithCollectionRunID(run.OrganisationName))

		// Role details come from the `role` search index — one request per
		// page rather than one per role. The per-role GET remains as the
		// fallback for gaps and for an unavailable index; the concurrency
		// setting now bounds both the page walk and that fallback.
		roleWorkers := c.cfg.Concurrency.RoleFetching
		if roleWorkers <= 0 {
			roleWorkers = 1
		}
		roleStart := time.Now()

		roleResult, roleErr := collectRoleDetails(ctx, client, 1000, roleWorkers)
		for _, warning := range roleResult.Warnings() {
			log.Warn(warning, logging.WithCollectionRunID(run.OrganisationName))
		}

		switch {
		case roleErr != nil:
			log.Warn(fmt.Sprintf("failed to collect roles: %v", roleErr),
				logging.WithCollectionRunID(run.OrganisationName))
		case len(roleResult.Roles) == 0:
			log.Info("no roles found — skipping dependency graph",
				logging.WithCollectionRunID(run.OrganisationName))
		default:
			roleDetails := roleResult.Roles
			log.Info(fmt.Sprintf("fetched %d role(s) in %s (%d from the search index, %d per-role)",
				len(roleDetails), time.Since(roleStart).Round(time.Millisecond),
				roleResult.FromIndex, roleResult.FromFallback),
				logging.WithCollectionRunID(run.OrganisationName))

			depParams := BuildRoleDependencies(org.Name, roleDetails)

			replaced, replaceErr := c.db.ReplaceRoleDependenciesForOrg(ctx, org.Name, depParams)
			if replaceErr != nil {
				log.Warn(fmt.Sprintf("failed to persist role dependency graph: %v", replaceErr),
					logging.WithCollectionRunID(run.OrganisationName))
			} else {
				log.Info(fmt.Sprintf("persisted role dependency graph: %d edge(s) from %d role(s)",
					replaced, len(roleDetails)),
					logging.WithCollectionRunID(run.OrganisationName))
			}
		}

		// C.2: Cookbook usage analysis.
		log.Info("running cookbook usage analysis",
			logging.WithCollectionRunID(run.OrganisationName))

		inventoryEntries := make([]analysis.CookbookInventoryEntry, 0)
		for cbName, entry := range serverCookbooks {
			for _, ver := range entry.Versions {
				inventoryEntries = append(inventoryEntries, analysis.CookbookInventoryEntry{
					Name:    cbName,
					Version: ver.Version,
				})
			}
		}

		usageResult, usageErr := c.analyser.RunUsageAnalysis(ctx, org.Name, run.OrganisationName, nodeRecords, inventoryEntries)
		if usageErr != nil {
			log.Warn(fmt.Sprintf("cookbook usage analysis failed: %v", usageErr),
				logging.WithCollectionRunID(run.OrganisationName))
		} else {
			log.Info(fmt.Sprintf(
				"cookbook usage analysis complete: %d total, %d active, %d unused (%d detail rows) in %s",
				usageResult.TotalCookbooks, usageResult.ActiveCookbooks,
				usageResult.UnusedCookbooks, usageResult.DetailCount,
				usageResult.Duration.Round(time.Millisecond)),
				logging.WithCollectionRunID(run.OrganisationName))
		}
	}()

	// -------------------------------------------------------------------
	// Wait for all three groups to finish before proceeding to readiness
	// evaluation, which depends on CookStyle and TK results.
	// -------------------------------------------------------------------
	parallelWG.Wait()

	// Step 13.5: Platform coverage analysis. For each git repo with a
	// .kitchen.yml, compute the coverage of kitchen platforms against
	// production platforms (from node_snapshots). This runs after all
	// three parallel groups complete because it needs:
	//   - Git repos cloned/pulled (Group B)
	//   - Node snapshots persisted (Step 4)
	// Non-fatal — errors are logged per repo and do not abort the run.
	if c.gitRepoDirFn != nil {
		coverageRepos, covListErr := c.db.ListClonedGitRepos(ctx)
		if covListErr != nil {
			log.Warn(fmt.Sprintf("failed to list git repos for platform coverage: %v", covListErr),
				logging.WithCollectionRunID(run.OrganisationName))
		} else if len(coverageRepos) > 0 {
			log.Info(fmt.Sprintf("computing platform coverage for %d git repo(s)", len(coverageRepos)),
				logging.WithCollectionRunID(run.OrganisationName))

			evaluated, covErrors := analysis.ComputeAllGitRepoCoverage(ctx, c.db, c.logger, coverageRepos, c.gitRepoDirFn)
			if covErrors > 0 {
				log.Warn(fmt.Sprintf(
					"platform coverage complete: %d evaluated, %d error(s)",
					evaluated, covErrors),
					logging.WithCollectionRunID(run.OrganisationName))
			} else {
				log.Info(fmt.Sprintf(
					"platform coverage complete: %d evaluated, 0 errors",
					evaluated),
					logging.WithCollectionRunID(run.OrganisationName))
			}
		}
	}

	// Step 14: Node readiness evaluation. Combines cookbook compatibility
	// data (from CookStyle + Test Kitchen) with disk space evaluation to
	// produce a per-node per-target-version readiness verdict. Skipped if
	// the evaluator is not configured. Non-fatal.
	if c.readinessEval != nil && c.cfg.TargetChefVersion != "" {
		log.Info("evaluating node readiness",
			logging.WithCollectionRunID(run.OrganisationName))

		readinessResults, readinessErr := c.readinessEval.EvaluateOrganisation(ctx, org.Name, org.Name, c.cfg.TargetChefVersion)
		if readinessErr != nil {
			log.Warn(fmt.Sprintf("node readiness evaluation failed: %v", readinessErr),
				logging.WithCollectionRunID(run.OrganisationName))
		} else {
			readyCount := 0
			blockedCount := 0
			for _, rr := range readinessResults {
				if rr.IsReady {
					readyCount++
				} else {
					blockedCount++
				}
			}
			log.Info(fmt.Sprintf(
				"node readiness evaluation complete: %d evaluated, %d ready, %d blocked",
				len(readinessResults), readyCount, blockedCount),
				logging.WithCollectionRunID(run.OrganisationName))

			// Step 14b: Record readiness metric snapshots for trend charts.
			// These pre-aggregated snapshots allow the dashboard to show
			// historical readiness trends without querying live
			// node_readiness rows.
			c.recordReadinessSnapshots(ctx, log, run.OrganisationName, org.Name, readinessResults, c.cfg.TargetChefVersionList())

			// Step 14c: Record unified node_metrics snapshot for enriched
			// trend views (staleness breakdown, blocking reasons, platform).
			c.recordNodeMetricsSnapshot(ctx, log, run.OrganisationName, org.Name, snapshotParams, readinessResults)
		}
	}

	// Step 15: Ownership auto-derivation. Evaluates configured rules against
	// the freshly collected data and creates/removes ownership assignments.
	// Skipped when the evaluator is not configured. Non-fatal.
	if c.ownershipEval != nil {
		log.Info("evaluating ownership auto-derivation rules",
			logging.WithCollectionRunID(run.OrganisationName))

		if ownerErr := c.ownershipEval.EvaluateAfterCollection(ctx, org.Name, org.Name); ownerErr != nil {
			log.Warn(fmt.Sprintf("ownership evaluation failed: %v", ownerErr),
				logging.WithCollectionRunID(run.OrganisationName))
		} else {
			log.Info("ownership evaluation complete",
				logging.WithCollectionRunID(run.OrganisationName))
		}
	}

	// Step 16: The collection run was already marked completed in Step 4b
	// after node snapshots were persisted, so the UI could show fresh data
	// while cookbook operations continued. Log final summary.
	log.Info(fmt.Sprintf("collection run %s post-completion processing finished: %d nodes, %d cookbook versions (organisation total %s)",
		run.OrganisationName, inserted, upserted, time.Since(orgStart).Round(time.Millisecond)),
		logging.WithCollectionRunID(run.OrganisationName))

	// Clear the deferred failure handler since we completed successfully.
	err = nil
	return nodes, cookbooks, nil
}

// tryStartRun atomically checks and sets the running flag. Returns true if
// the run was started, false if one is already in progress.
func (c *Collector) tryStartRun() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return false
	}
	c.running = true
	return true
}

// finishRun clears the running flag.
func (c *Collector) finishRun() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
	c.currentRunOrgName = ""
}

// defaultClientFactory resolves credentials and builds a real Chef API client
// for the given organisation.
func (c *Collector) defaultClientFactory(ctx context.Context, org datastore.Organisation) (*chefapi.Client, error) {
	// Determine the credential source for the client key.
	src := secrets.CredentialSource{
		CredentialName: org.ClientKeyCredentialName,
	}

	// Check if the config has a file path or env var for this org.
	for _, cfgOrg := range c.cfg.Organisations {
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

	resolved, err := c.resolver.Resolve(ctx, src)
	if err != nil {
		return nil, fmt.Errorf("resolving client key for org %q: %w", org.Name, err)
	}
	defer secrets.ZeroBytes(resolved.Plaintext)

	// Look up the SSLVerify setting from the config for this org.
	sslVerify := true
	for _, cfgOrg := range c.cfg.Organisations {
		if cfgOrg.Name == org.Name {
			sslVerify = cfgOrg.SSLVerifyEnabled()
			break
		}
	}

	client, err := chefapi.NewClient(chefapi.ClientConfig{
		ServerURL:     org.ChefServerURL,
		ClientName:    org.ClientName,
		PrivateKeyPEM: resolved.Plaintext,
		OrgName:       org.OrgName,
		SSLVerify:     &sslVerify,
	})
	if err != nil {
		return nil, fmt.Errorf("creating client for org %q: %w", org.Name, err)
	}

	return client, nil
}

// maxNodesInMetricSnapshot is the threshold above which per-node data is
// omitted from the metric snapshot JSONB payload to avoid excessive sizes.
const maxNodesInMetricSnapshot = 50000

// nodeVersionEntry is a single node's name and version, included in the
// metric snapshot payload to support ownership-filtered trend queries.
type nodeVersionEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// buildVersionDistributionPayload builds the JSONB payload for a
// chef_version_distribution metric snapshot from in-memory node snapshot
// params. The payload includes per-node data (name + version) so that
// ownership-filtered trend queries can be served from metric_snapshots
// instead of querying live node_snapshots mid-collection.
//
// For organisations exceeding maxNodesInMetricSnapshot nodes, per-node
// data is omitted and nodes_omitted is set to true.
func buildVersionDistributionPayload(snapshotParams []datastore.InsertNodeSnapshotParams, warningHours, criticalDays int) (json.RawMessage, error) {
	versionDist := make(map[string]int, 16)
	var totalStale, totalFresh int
	var totalWarning, totalCritical int

	// Pre-allocate the nodes slice unless the org is too large.
	omitNodes := len(snapshotParams) > maxNodesInMetricSnapshot
	var nodes []nodeVersionEntry
	if !omitNodes {
		nodes = make([]nodeVersionEntry, 0, len(snapshotParams))
	}

	now := time.Now()
	thresholds := staleness.Thresholds{WarningHours: warningHours, CriticalDays: criticalDays}

	for _, p := range snapshotParams {
		ver := p.ChefVersion
		if ver == "" {
			ver = "unknown"
		}
		versionDist[ver]++
		if p.IsStale {
			totalStale++
		} else {
			totalFresh++
		}

		var ohaiTime time.Time
		if p.OhaiTime > 0 {
			ohaiTime = time.Unix(int64(p.OhaiTime), 0)
		}
		tier := staleness.ComputeTier(ohaiTime, now, thresholds)
		switch tier {
		case staleness.Warning:
			totalWarning++
		case staleness.Critical:
			totalCritical++
		}

		if !omitNodes {
			nodes = append(nodes, nodeVersionEntry{Name: p.NodeName, Version: ver})
		}
	}

	payload := map[string]interface{}{
		"distribution":   versionDist,
		"total_nodes":    len(snapshotParams),
		"stale_nodes":    totalStale,
		"fresh_nodes":    totalFresh,
		"warning_nodes":  totalWarning,
		"critical_nodes": totalCritical,
		"nodes_omitted":  omitNodes,
	}
	if omitNodes {
		// Explicitly omit the nodes key to save space.
	} else {
		payload["nodes"] = nodes
	}

	return json.Marshal(payload)
}

// recordMetricSnapshots persists pre-aggregated metric snapshots for the
// organisation so that dashboard trend charts can display historical data
// without scanning the (now current-state-only) node_snapshots table.
func (c *Collector) recordMetricSnapshots(
	ctx context.Context,
	log *logging.ScopedLogger,
	collectionRunID string,
	organisationID string,
	snapshotParams []datastore.InsertNodeSnapshotParams,
) {
	now := time.Now().UTC()

	if len(snapshotParams) > maxNodesInMetricSnapshot {
		log.Warn(fmt.Sprintf("organisation %s has %d nodes (>%d) — per-node data omitted from metric snapshot; ownership-filtered trend unavailable",
			organisationID, len(snapshotParams), maxNodesInMetricSnapshot),
			logging.WithCollectionRunID(collectionRunID))
	}

	// chef_version_distribution metric snapshot — used by the version
	// distribution trend chart.
	distJSON, err := buildVersionDistributionPayload(snapshotParams, c.cfg.Collection.StaleNodeWarningHours, c.cfg.Collection.StaleNodeCriticalDays)
	if err != nil {
		log.Warn(fmt.Sprintf("failed to marshal chef_version_distribution metric: %v", err),
			logging.WithCollectionRunID(collectionRunID))
		return
	}

	if _, msErr := c.db.InsertMetricSnapshot(ctx, datastore.InsertMetricSnapshotParams{
		CollectionRunOrg: collectionRunID,
		OrganisationName: organisationID,
		SnapshotType:     "chef_version_distribution",
		Data:             distJSON,
		SnapshotAt:       now,
	}); msErr != nil {
		log.Warn(fmt.Sprintf("failed to record chef_version_distribution metric: %v", msErr),
			logging.WithCollectionRunID(collectionRunID))
	}
}

// readinessNodeEntry is a single node's readiness status, included in the
// readiness_summary metric snapshot payload to support ownership-filtered
// readiness trend queries.
type readinessNodeEntry struct {
	Name    string `json:"name"`
	IsReady bool   `json:"is_ready"`
}

// buildReadinessSnapshotPayload builds the JSONB payload for a
// readiness_summary metric snapshot from in-memory readiness results.
// The payload includes per-node data (name + is_ready) so that
// ownership-filtered trend queries can be served from metric_snapshots.
//
// For organisations exceeding maxNodesInMetricSnapshot nodes, per-node
// data is omitted and nodes_omitted is set to true.
func buildReadinessSnapshotPayload(results []analysis.ReadinessResult) (json.RawMessage, error) {
	var ready, blocked int

	omitNodes := len(results) > maxNodesInMetricSnapshot
	var nodes []readinessNodeEntry
	if !omitNodes {
		nodes = make([]readinessNodeEntry, 0, len(results))
	}

	for _, r := range results {
		if r.IsReady {
			ready++
		} else {
			blocked++
		}
		if !omitNodes {
			nodes = append(nodes, readinessNodeEntry{Name: r.NodeName, IsReady: r.IsReady})
		}
	}

	payload := map[string]interface{}{
		"total_nodes":   len(results),
		"ready":         ready,
		"blocked":       blocked,
		"nodes_omitted": omitNodes,
	}
	if omitNodes {
		// Explicitly omit the nodes key to save space.
	} else {
		payload["nodes"] = nodes
	}

	return json.Marshal(payload)
}

// recordReadinessSnapshots persists pre-aggregated readiness metric snapshots
// for each target Chef version so that the readiness trend chart can display
// historical data without querying live node_readiness rows.
func (c *Collector) recordReadinessSnapshots(
	ctx context.Context,
	log *logging.ScopedLogger,
	collectionRunID string,
	organisationID string,
	readinessResults []analysis.ReadinessResult,
	targetVersions []string,
) {
	now := time.Now().UTC()

	// Group results by target version.
	byVersion := make(map[string][]analysis.ReadinessResult, len(targetVersions))
	for i := range readinessResults {
		tv := readinessResults[i].TargetChefVersion
		byVersion[tv] = append(byVersion[tv], readinessResults[i])
	}

	for _, tv := range targetVersions {
		results := byVersion[tv]
		if len(results) == 0 {
			continue
		}

		payload, err := buildReadinessSnapshotPayload(results)
		if err != nil {
			log.Warn(fmt.Sprintf("failed to marshal readiness_summary metric for version %s: %v", tv, err),
				logging.WithCollectionRunID(collectionRunID))
			continue
		}

		if _, msErr := c.db.InsertMetricSnapshot(ctx, datastore.InsertMetricSnapshotParams{
			CollectionRunOrg:  collectionRunID,
			OrganisationName:  organisationID,
			SnapshotType:      "readiness_summary",
			TargetChefVersion: tv,
			Data:              payload,
			SnapshotAt:        now,
		}); msErr != nil {
			log.Warn(fmt.Sprintf("failed to record readiness_summary metric for version %s: %v", tv, msErr),
				logging.WithCollectionRunID(collectionRunID))
		}
	}
}

// buildComplexitySnapshotPayload builds the JSONB payload for a
// complexity_summary metric snapshot from server cookbook complexity records.
func buildComplexitySnapshotPayload(complexities []datastore.ServerCookbookComplexity) (json.RawMessage, error) {
	var totalScore, low, medium, high, critical int
	for _, cc := range complexities {
		totalScore += cc.ComplexityScore
		switch cc.ComplexityLabel {
		case "low":
			low++
		case "medium":
			medium++
		case "high":
			high++
		case "critical":
			critical++
		}
	}

	avg := 0.0
	if len(complexities) > 0 {
		avg = float64(totalScore) / float64(len(complexities))
	}

	payload := map[string]interface{}{
		"total_cookbooks": len(complexities),
		"total_score":     totalScore,
		"average_score":   avg,
		"low_count":       low,
		"medium_count":    medium,
		"high_count":      high,
		"critical_count":  critical,
	}
	return json.Marshal(payload)
}

// recordComplexitySnapshots persists pre-aggregated complexity metric
// snapshots for each target Chef version so that the complexity trend chart
// can display historical data.
func (c *Collector) recordComplexitySnapshots(
	ctx context.Context,
	log *logging.ScopedLogger,
	collectionRunID string,
	organisationID string,
	targetVersions []string,
) {
	allComplexities, err := c.db.ListServerCookbookComplexitiesByOrganisation(ctx, organisationID)
	if err != nil {
		log.Warn(fmt.Sprintf("failed to list complexities for snapshot: %v", err),
			logging.WithCollectionRunID(collectionRunID))
		return
	}

	// Group by target version.
	byVersion := make(map[string][]datastore.ServerCookbookComplexity, len(targetVersions))
	for i := range allComplexities {
		tv := allComplexities[i].TargetChefVersion
		byVersion[tv] = append(byVersion[tv], allComplexities[i])
	}

	now := time.Now().UTC()

	for _, tv := range targetVersions {
		ccs := byVersion[tv]
		if len(ccs) == 0 {
			continue
		}

		payload, pErr := buildComplexitySnapshotPayload(ccs)
		if pErr != nil {
			log.Warn(fmt.Sprintf("failed to marshal complexity_summary metric for version %s: %v", tv, pErr),
				logging.WithCollectionRunID(collectionRunID))
			continue
		}

		if _, msErr := c.db.InsertMetricSnapshot(ctx, datastore.InsertMetricSnapshotParams{
			CollectionRunOrg:  collectionRunID,
			OrganisationName:  organisationID,
			SnapshotType:      "complexity_summary",
			TargetChefVersion: tv,
			Data:              payload,
			SnapshotAt:        now,
		}); msErr != nil {
			log.Warn(fmt.Sprintf("failed to record complexity_summary metric for version %s: %v", tv, msErr),
				logging.WithCollectionRunID(collectionRunID))
		}
	}
}

// boolPtr returns a *bool set to value if present is true, or nil otherwise.
// Used to distinguish "field absent" (nil) from "field present with value false".
func boolPtr(value bool, present bool) *bool {
	if !present {
		return nil
	}
	return &value
}
