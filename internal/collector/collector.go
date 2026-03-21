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
)

// ClientFactory creates a chefapi.Client for a given organisation. This is
// injected as a dependency to allow testing with mock clients.
type ClientFactory func(ctx context.Context, org datastore.Organisation) (*chefapi.Client, error)

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
	kitchenScanner   *analysis.KitchenScanner
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

	// mu guards currentRunID to enforce the single-run constraint.
	mu           sync.Mutex
	currentRunID string
	running      bool
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

// WithKitchenScanner sets the Test Kitchen scanner for the collection cycle.
// When set, Test Kitchen runs after CookStyle scanning.
func WithKitchenScanner(s *analysis.KitchenScanner) Option {
	return func(c *Collector) { c.kitchenScanner = s }
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
		orgByID[org.ID] = org
	}

	for _, run := range interrupted {
		runLog := log

		// Check freshness.
		if run.StartedAt.Before(freshnessCutoff) {
			// Too old — abandon.
			reason := fmt.Sprintf("abandoned: interrupted run started at %s is older than two collection intervals (%s)",
				run.StartedAt.Format(time.RFC3339), collectionInterval)
			if _, abandonErr := c.db.AbandonCollectionRun(ctx, run.ID, reason); abandonErr != nil {
				result.Errors[run.ID] = abandonErr
				runLog.Warn(fmt.Sprintf("failed to abandon stale interrupted run %s: %v", run.ID, abandonErr))
			} else {
				result.Abandoned++
				runLog.Info(fmt.Sprintf("abandoned stale interrupted run %s (started %s, cutoff %s)",
					run.ID, run.StartedAt.Format(time.RFC3339), freshnessCutoff.Format(time.RFC3339)))
			}
			continue
		}

		// Fresh enough to resume. Determine which organisation this run
		// belongs to and whether it has already been completed by a
		// subsequent run.
		org, orgExists := orgByID[run.OrganisationID]
		if !orgExists {
			// Organisation was deleted since the run started — abandon.
			reason := "abandoned: organisation no longer exists"
			if _, abandonErr := c.db.AbandonCollectionRun(ctx, run.ID, reason); abandonErr != nil {
				result.Errors[run.ID] = abandonErr
			} else {
				result.Abandoned++
			}
			runLog.Info(fmt.Sprintf("abandoned interrupted run %s — organisation %s no longer exists",
				run.ID, run.OrganisationID))
			continue
		}

		// Check if this organisation already has a completed run since the
		// interrupted run started.
		completedRuns, cErr := c.db.ListCompletedRunsForOrganisation(ctx, run.OrganisationID, run.StartedAt)
		if cErr != nil {
			result.Errors[run.ID] = cErr
			runLog.Warn(fmt.Sprintf("failed to check completed runs for org %s: %v", org.Name, cErr))
			continue
		}

		if len(completedRuns) > 0 {
			// A newer completed run exists — this interrupted run's data is
			// superseded. Abandon it.
			reason := fmt.Sprintf("abandoned: organisation %s already has a completed run (%s) since this run started",
				org.Name, completedRuns[0].ID)
			if _, abandonErr := c.db.AbandonCollectionRun(ctx, run.ID, reason); abandonErr != nil {
				result.Errors[run.ID] = abandonErr
			} else {
				result.Abandoned++
			}
			runLog.Info(fmt.Sprintf("abandoned interrupted run %s for org %s — superseded by completed run %s",
				run.ID, org.Name, completedRuns[0].ID))
			continue
		}

		// This organisation needs re-collection. Mark the interrupted run
		// as abandoned (we'll create a fresh run for the organisation) and
		// queue the org for collection.
		reason := fmt.Sprintf("abandoned: will be re-collected as part of resume (checkpoint_start=%d)",
			run.CheckpointStart)
		if _, abandonErr := c.db.AbandonCollectionRun(ctx, run.ID, reason); abandonErr != nil {
			result.Errors[run.ID] = abandonErr
			runLog.Warn(fmt.Sprintf("failed to abandon interrupted run %s for re-collection: %v", run.ID, abandonErr))
			continue
		}

		orgsNeedingCollection[org.ID] = org
		result.Resumed++
		runLog.Info(fmt.Sprintf("will resume collection for org %s (interrupted run %s)",
			org.Name, run.ID))
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
		TotalOrgs: len(orgList),
		Errors:    make(map[string]error, len(orgList)),
	}

	// Collect organisations in parallel, bounded by the configured
	// concurrency limit.
	concurrency := c.cfg.Concurrency.OrganisationCollection
	if concurrency <= 0 {
		concurrency = 1
	}

	type orgResult struct {
		OrgName   string
		Nodes     int
		Cookbooks int
		Err       error
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

			nodes, cookbooks, orgErr := c.collectOrganisation(ctx, org)
			resultsCh <- orgResult{
				OrgName:   org.Name,
				Nodes:     nodes,
				Cookbooks: cookbooks,
				Err:       orgErr,
			}
		}(org)
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	for or := range resultsCh {
		if or.Err != nil {
			result.FailedOrgs++
			result.Errors[or.OrgName] = or.Err
			log.Error(fmt.Sprintf("organisation %q: resumed collection failed: %v", or.OrgName, or.Err))
		} else {
			result.SucceededOrgs++
			result.TotalNodes += or.Nodes
			result.TotalCookbooks += or.Cookbooks
			log.Info(fmt.Sprintf("organisation %q: resumed collection completed — %d nodes, %d cookbook versions",
				or.OrgName, or.Nodes, or.Cookbooks))
		}
	}

	result.Duration = time.Since(start)

	log.Info(fmt.Sprintf(
		"resumed collection run complete in %s: %d/%d orgs succeeded, %d nodes, %d cookbook versions",
		result.Duration.Round(time.Millisecond),
		result.SucceededOrgs, result.TotalOrgs,
		result.TotalNodes, result.TotalCookbooks,
	))

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
		TotalOrgs: len(orgs),
		Errors:    make(map[string]error, len(orgs)),
	}

	// Collect organisations in parallel, bounded by the configured
	// concurrency limit.
	concurrency := c.cfg.Concurrency.OrganisationCollection
	if concurrency <= 0 {
		concurrency = 1
	}

	type orgResult struct {
		OrgName   string
		Nodes     int
		Cookbooks int
		Err       error
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

			nodes, cookbooks, orgErr := c.collectOrganisation(ctx, org)
			resultsCh <- orgResult{
				OrgName:   org.Name,
				Nodes:     nodes,
				Cookbooks: cookbooks,
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
		if or.Err != nil {
			result.FailedOrgs++
			result.Errors[or.OrgName] = or.Err
			log.Error(fmt.Sprintf("organisation %q: collection failed: %v", or.OrgName, or.Err))
		} else {
			result.SucceededOrgs++
			result.TotalNodes += or.Nodes
			result.TotalCookbooks += or.Cookbooks
			log.Info(fmt.Sprintf("organisation %q: collected %d nodes, %d cookbook versions",
				or.OrgName, or.Nodes, or.Cookbooks))
		}
	}

	result.Duration = time.Since(start)

	log.Info(fmt.Sprintf(
		"collection run complete in %s: %d/%d orgs succeeded, %d nodes, %d cookbook versions",
		result.Duration.Round(time.Millisecond),
		result.SucceededOrgs, result.TotalOrgs,
		result.TotalNodes, result.TotalCookbooks,
	))

	// Purge old log entries if retention is configured.
	if c.cfg.Logging.RetentionDays > 0 {
		purged, purgeErr := c.db.PurgeLogEntriesOlderThanDays(ctx, c.cfg.Logging.RetentionDays)
		if purgeErr != nil {
			log.Warn(fmt.Sprintf("log retention purge failed: %v", purgeErr))
		} else if purged > 0 {
			log.Info(fmt.Sprintf("purged %d log entries older than %d days", purged, c.cfg.Logging.RetentionDays))
		}
	}

	return result, nil
}

// collectOrganisation runs the full collection sequence for a single
// organisation. It returns the number of nodes collected and cookbook
// versions upserted.
func (c *Collector) collectOrganisation(ctx context.Context, org datastore.Organisation) (nodes int, cookbooks int, err error) {
	log := c.logger.WithScope(logging.ScopeCollectionRun, logging.WithOrganisation(org.Name))

	// Step 1: Create a collection run row.
	run, err := c.db.CreateCollectionRun(ctx, datastore.CreateCollectionRunParams{
		OrganisationID: org.ID,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("creating collection run: %w", err)
	}

	log.Info(fmt.Sprintf("collection run %s started", run.ID),
		logging.WithCollectionRunID(run.ID))

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
				if _, intErr := c.db.InterruptCollectionRun(context.Background(), run.ID); intErr != nil {
					log.Error(fmt.Sprintf("failed to mark run %s as interrupted: %v", run.ID, intErr),
						logging.WithCollectionRunID(run.ID))
				} else {
					log.Warn(fmt.Sprintf("collection run %s interrupted", run.ID),
						logging.WithCollectionRunID(run.ID))
				}
				return
			}
			if _, failErr := c.db.FailCollectionRun(context.Background(), run.ID, errMsg); failErr != nil {
				log.Error(fmt.Sprintf("failed to mark run %s as failed: %v", run.ID, failErr),
					logging.WithCollectionRunID(run.ID))
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
		logging.WithCollectionRunID(run.ID))

	searchRows, err := client.CollectAllNodesConcurrent(ctx, 1000, pageConcurrency, cmdbSearchKeys)
	if err != nil {
		return 0, 0, fmt.Errorf("collecting nodes: %w", err)
	}

	log.Info(fmt.Sprintf("fetched %d nodes from Chef server", len(searchRows)),
		logging.WithCollectionRunID(run.ID))

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

	// Build NodeRecord slice for usage analysis (populated alongside snapshot params).
	nodeRecords := make([]analysis.NodeRecord, 0, len(searchRows))

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
		for cbName := range cbVersions {
			allCookbookNames[cbName] = true
			if !nodeIsStale {
				activeCookbookNames[cbName] = true
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

		snapshotParams = append(snapshotParams, datastore.InsertNodeSnapshotParams{
			CollectionRunID:  run.ID,
			OrganisationID:   org.ID,
			NodeName:         nd.Name(),
			ChefEnvironment:  nd.ChefEnvironment(),
			ChefVersion:      nd.ChefVersion(),
			Platform:         nd.Platform(),
			PlatformVersion:  nd.PlatformVersion(),
			PlatformFamily:   nd.PlatformFamily(),
			Filesystem:       fsJSON,
			Cookbooks:        cbJSON,
			RunList:          rlJSON,
			Roles:            rolesJSON,
			PolicyName:       nd.PolicyName(),
			PolicyGroup:      nd.PolicyGroup(),
			OhaiTime:         nd.OhaiTime(),
			CustomAttributes: customAttrsJSON,
			IsStale:          nodeIsStale,
			CollectedAt:      now,
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
		logging.WithCollectionRunID(run.ID))

	// Persist node snapshots in bulk.
	inserted, err := c.db.BulkUpsertNodeSnapshots(ctx, snapshotParams)
	if err != nil {
		return 0, 0, fmt.Errorf("persisting node snapshots: %w", err)
	}
	nodes = inserted

	// Update progress on the collection run.
	if _, err := c.db.UpdateCollectionRunProgress(ctx, datastore.UpdateCollectionRunProgressParams{
		ID:             run.ID,
		TotalNodes:     len(searchRows),
		NodesCollected: inserted,
	}); err != nil {
		log.Warn(fmt.Sprintf("failed to update collection run progress: %v", err),
			logging.WithCollectionRunID(run.ID))
	}

	// Step 4b: Complete the collection run early so the UI can show fresh
	// node data immediately. The remaining steps (cookbook inventory,
	// downloads, analysis, CookStyle, etc.) can take a very long time with
	// large fleets and the UI queries only show nodes from the latest
	// *completed* run. By completing now, users see up-to-date node/stale
	// status while the heavier cookbook operations continue in the background.
	if _, completeErr := c.db.CompleteCollectionRun(ctx, run.ID, len(searchRows), inserted); completeErr != nil {
		log.Error(fmt.Sprintf("failed to mark run %s as completed after node collection: %v", run.ID, completeErr),
			logging.WithCollectionRunID(run.ID))
		// Non-fatal — continue with cookbook operations even if the status
		// update failed. The deferred error handler will still attempt to
		// mark the run appropriately on exit.
	} else {
		runCompleted = true
		log.Info(fmt.Sprintf("collection run %s marked completed with %d nodes (continuing with cookbook operations)",
			run.ID, inserted),
			logging.WithCollectionRunID(run.ID))
	}

	// Step 5: Fetch cookbook inventory from the Chef server.
	log.Info("fetching cookbook inventory",
		logging.WithCollectionRunID(run.ID))

	serverCookbooks, err := client.GetCookbooks(ctx)
	if err != nil {
		// After early completion in Step 4b, cookbook inventory failures are
		// non-fatal — node data is already visible in the UI. Log and skip
		// the remaining cookbook operations.
		if runCompleted {
			log.Warn(fmt.Sprintf("fetching cookbook inventory failed (non-fatal, nodes already committed): %v", err),
				logging.WithCollectionRunID(run.ID))
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
				OrganisationID:  org.ID,
				Name:            cbName,
				Version:         ver.Version,
				IsActive:        isActive,
				IsStaleCookbook: false, // Will be updated below
				FirstSeenAt:     now,
				LastFetchedAt:   now,
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
				logging.WithCollectionRunID(run.ID))
			err = nil
			return nodes, 0, nil
		}
		return nodes, 0, fmt.Errorf("upserting cookbook metadata: %w", err)
	}
	cookbooks = upserted

	// Mark active/unused cookbooks for this organisation.
	activeNames := make([]string, 0, len(activeCookbookNames))
	for name := range activeCookbookNames {
		activeNames = append(activeNames, name)
	}
	if err := c.db.MarkServerCookbooksActiveForOrg(ctx, org.ID, activeNames); err != nil {
		log.Warn(fmt.Sprintf("failed to mark active cookbooks: %v", err),
			logging.WithCollectionRunID(run.ID))
	}

	// Step 7: Evaluate stale cookbook flag. A cookbook is stale if the most
	// recent version's first_seen_at is older than the configured threshold.
	// This is done via a database update for cookbooks belonging to this org.
	staleCookbookCutoff := now.Add(-staleCookbookThreshold)
	staleCount, staleErr := c.db.MarkStaleServerCookbooksForOrg(ctx, org.ID, staleCookbookCutoff)
	if staleErr != nil {
		log.Warn(fmt.Sprintf("failed to mark stale cookbooks: %v", staleErr),
			logging.WithCollectionRunID(run.ID))
	} else if staleCount > 0 {
		log.Info(fmt.Sprintf("marked %d cookbook(s) as stale (first seen before %s)",
			staleCount, staleCookbookCutoff.Format(time.RFC3339)),
			logging.WithCollectionRunID(run.ID))
	}

	// Step 7b: Server cookbook download, CookStyle scan, and cleanup.
	//
	// Uses a streaming pipeline that processes one cookbook at a time:
	// download → CookStyle scan → autocorrect preview → delete from disk.
	// This keeps disk usage to a single cookbook version at a time instead
	// of downloading thousands of versions that accumulate on disk.
	//
	// Server cookbooks already scanned are skipped automatically (the
	// scanner's immutability cache detects existing results). Autocorrect
	// previews are generated inline so the cookbook files can be deleted
	// immediately after processing.
	//
	// When collection.skip_server_cookbook_download is true, the pipeline
	// is skipped entirely — only git-sourced cookbooks will be scanned.
	// The cookbook inventory and metadata (Steps 5–7) are still collected
	// so the UI can display which server cookbooks exist and which nodes
	// use them, but no content is downloaded for analysis.
	fetchConcurrency := c.cfg.Concurrency.GitPull
	if fetchConcurrency <= 0 {
		fetchConcurrency = 1
	}

	if c.cfg.Collection.SkipServerCookbookDownload {
		log.Info("skipping Chef server cookbook download (collection.skip_server_cookbook_download is enabled)",
			logging.WithCollectionRunID(run.ID))
	} else {
		log.Info("running server cookbook pipeline (download → scan → delete)",
			logging.WithCollectionRunID(run.ID))

		pipelineResult := runServerCookbookPipeline(
			ctx, client, c.db, log, org,
			c.cookbookCacheDir,
			c.cfg.TargetChefVersions,
			c.cookstyleScanner,
			c.autocorrectGen,
		)

		if pipelineResult.Total == 0 {
			log.Info("no server cookbook versions need processing",
				logging.WithCollectionRunID(run.ID))
		} else {
			log.Info(fmt.Sprintf(
				"server cookbook pipeline complete: %d total, %d downloaded, %d scanned, %d skipped, %d failed, %d legacy cached cleaned in %s",
				pipelineResult.Total, pipelineResult.Downloaded, pipelineResult.Scanned,
				pipelineResult.Skipped, pipelineResult.Failed, pipelineResult.Cleaned,
				pipelineResult.Duration.Round(time.Millisecond)),
				logging.WithCollectionRunID(run.ID))
		}

		for _, fe := range pipelineResult.Errors {
			log.Warn(fmt.Sprintf("server cookbook pipeline error: %s/%s: %v", fe.Name, fe.Version, fe.Err),
				logging.WithCollectionRunID(run.ID))
		}
	}

	// Step 7c: Fetch cookbooks from git repositories. For each active
	// cookbook name, attempt to clone or pull from the configured git base
	// URLs. Git-sourced cookbooks include test suites and are eligible for
	// full compatibility testing. Operations run in parallel bounded by
	// the concurrency.git_pull worker pool setting. Failures are non-fatal.
	if len(c.cfg.GitBaseURLs) > 0 {
		gitLog := c.logger.WithScope(logging.ScopeGitOperation, logging.WithOrganisation(org.Name))

		gitLog.Info(fmt.Sprintf("fetching git cookbooks across %d base URL(s) for %d active cookbook(s)",
			len(c.cfg.GitBaseURLs), len(activeCookbookNames)),
			logging.WithCollectionRunID(run.ID))

		gitDir := c.gitCookbookDir
		if gitDir == "" {
			gitDir = filepath.Join(os.TempDir(), "chef-migration-metrics", "git-cookbooks")
		}
		gitMgr := NewGitCookbookManager(gitDir, nil)

		gitResult := fetchGitCookbooks(ctx, gitMgr, c.db, gitLog, c.cfg.GitBaseURLs, activeCookbookNames, fetchConcurrency, c.cfg.Ownership.Enabled)

		if gitResult.Total == 0 {
			gitLog.Info("no git cookbook candidates to fetch",
				logging.WithCollectionRunID(run.ID))
		} else {
			gitLog.Info(fmt.Sprintf(
				"git cookbook fetch complete: %d total, %d cloned, %d pulled, %d unchanged, %d failed in %s",
				gitResult.Total, gitResult.Cloned, gitResult.Pulled,
				gitResult.Unchanged, gitResult.Failed,
				gitResult.Duration.Round(time.Millisecond)),
				logging.WithCollectionRunID(run.ID))
		}
	}

	// Step 9: Build role dependency graph. Fetch each role's detail to
	// extract run_list entries (recipe[cookbook] and role[other_role]),
	// then persist the directed graph to the role_dependencies table.
	log.Info("building role dependency graph",
		logging.WithCollectionRunID(run.ID))

	roleNames, roleListErr := client.GetRoles(ctx)
	if roleListErr != nil {
		log.Warn(fmt.Sprintf("failed to list roles: %v", roleListErr),
			logging.WithCollectionRunID(run.ID))
		// Non-fatal — role graph is supplementary data.
	} else if len(roleNames) > 0 {
		roleDetails := make([]*chefapi.RoleDetail, 0, len(roleNames))
		for _, rn := range roleNames {
			rd, rdErr := client.GetRole(ctx, rn)
			if rdErr != nil {
				log.Warn(fmt.Sprintf("failed to fetch role %q: %v", rn, rdErr),
					logging.WithCollectionRunID(run.ID))
				continue
			}
			roleDetails = append(roleDetails, rd)
		}

		depParams := BuildRoleDependencies(org.ID, roleDetails)

		replaced, replaceErr := c.db.ReplaceRoleDependenciesForOrg(ctx, org.ID, depParams)
		if replaceErr != nil {
			log.Warn(fmt.Sprintf("failed to persist role dependency graph: %v", replaceErr),
				logging.WithCollectionRunID(run.ID))
		} else {
			log.Info(fmt.Sprintf("persisted role dependency graph: %d edge(s) from %d role(s)",
				replaced, len(roleDetails)),
				logging.WithCollectionRunID(run.ID))
		}
	} else {
		log.Info("no roles found — skipping dependency graph",
			logging.WithCollectionRunID(run.ID))
	}

	// Step 10: Cookbook usage analysis. Build the inventory entry list from
	// the serverCookbooks map already in scope, then run the three-phase
	// analysis and persist results. Non-fatal — failures are logged as WARN.
	log.Info("running cookbook usage analysis",
		logging.WithCollectionRunID(run.ID))

	inventoryEntries := make([]analysis.CookbookInventoryEntry, 0)
	for cbName, entry := range serverCookbooks {
		for _, ver := range entry.Versions {
			inventoryEntries = append(inventoryEntries, analysis.CookbookInventoryEntry{
				Name:    cbName,
				Version: ver.Version,
			})
		}
	}

	usageResult, usageErr := c.analyser.RunUsageAnalysis(ctx, org.ID, run.ID, nodeRecords, inventoryEntries)
	if usageErr != nil {
		log.Warn(fmt.Sprintf("cookbook usage analysis failed: %v", usageErr),
			logging.WithCollectionRunID(run.ID))
	} else {
		log.Info(fmt.Sprintf(
			"cookbook usage analysis complete: %d total, %d active, %d unused (%d detail rows) in %s",
			usageResult.TotalCookbooks, usageResult.ActiveCookbooks,
			usageResult.UnusedCookbooks, usageResult.DetailCount,
			usageResult.Duration.Round(time.Millisecond)),
			logging.WithCollectionRunID(run.ID))
	}

	// Step 11: CookStyle scanning for git-sourced cookbooks only.
	// Server cookbooks are now scanned inline during the Step 7b pipeline
	// (download → scan → delete) so their files don't persist on disk.
	// Git cookbooks live in persistent clones and are rescanned when the
	// HEAD commit changes. Skipped if the scanner is not configured or no
	// cookbook directory resolver is set. Non-fatal.
	if c.cookstyleScanner != nil && c.gitRepoDirFn != nil && len(c.cfg.TargetChefVersions) > 0 {
		gitRepos, gitListErr := c.db.ListGitRepos(ctx)
		if gitListErr != nil {
			log.Warn(fmt.Sprintf("failed to list git repos for CookStyle scanning: %v", gitListErr),
				logging.WithCollectionRunID(run.ID))
		} else if len(gitRepos) > 0 {
			log.Info(fmt.Sprintf("running CookStyle scanning on %d git repo(s)", len(gitRepos)),
				logging.WithCollectionRunID(run.ID))

			csBatch := c.cookstyleScanner.ScanGitRepos(ctx, gitRepos, c.cfg.TargetChefVersions, c.gitRepoDirFn)
			log.Info(fmt.Sprintf(
				"CookStyle batch complete (git repos): %d total, %d scanned, %d skipped, %d passed, %d failed, %d errors in %s",
				csBatch.Total, csBatch.Scanned, csBatch.Skipped,
				csBatch.Passed, csBatch.Failed, csBatch.Errors,
				csBatch.Duration.Round(time.Millisecond)),
				logging.WithCollectionRunID(run.ID))
		}
	} else if c.cookstyleScanner != nil && c.gitRepoDirFn == nil {
		log.Debug("skipping CookStyle scanning — no git repo directory resolver configured",
			logging.WithCollectionRunID(run.ID))
	}

	// Step 12: Test Kitchen. Run Test Kitchen on git-sourced cookbooks
	// that have test suites. Skipped if the scanner is not configured.
	// Non-fatal — failures are logged as WARN.
	if c.kitchenScanner != nil && c.gitRepoDirFn != nil && len(c.cfg.TargetChefVersions) > 0 {
		log.Info("running Test Kitchen",
			logging.WithCollectionRunID(run.ID))

		gitRepos, tkListErr := c.db.ListGitRepos(ctx)
		if tkListErr != nil {
			log.Warn(fmt.Sprintf("failed to list git repos for Test Kitchen: %v", tkListErr),
				logging.WithCollectionRunID(run.ID))
		} else {
			tkBatch := c.kitchenScanner.TestGitRepos(ctx, gitRepos, c.cfg.TargetChefVersions, c.gitRepoDirFn)
			log.Info(fmt.Sprintf(
				"Test Kitchen batch complete (git repos): %d total, %d tested, %d skipped, %d passed, %d failed, %d errors in %s",
				tkBatch.Total, tkBatch.Tested, tkBatch.Skipped,
				tkBatch.Passed, tkBatch.Failed, tkBatch.Errors,
				tkBatch.Duration.Round(time.Millisecond)),
				logging.WithCollectionRunID(run.ID))
		}
	} else if c.kitchenScanner != nil && c.gitRepoDirFn == nil {
		log.Debug("skipping Test Kitchen — no git repo directory resolver configured",
			logging.WithCollectionRunID(run.ID))
	}

	// Step 13: Autocorrect previews and complexity scoring. These depend
	// on CookStyle results already existing in the database. Skipped if
	// the respective components are not configured. Non-fatal.
	// Step 13: Autocorrect previews for git-sourced cookbooks only.
	// Server cookbook previews are now generated inline during the Step 7b
	// pipeline (before files are deleted from disk). Git cookbook previews
	// are generated here because git clones persist on disk.
	if c.autocorrectGen != nil && c.gitRepoDirFn != nil {
		gitReposForAC, gitRepoListErr := c.db.ListGitRepos(ctx)
		if gitRepoListErr != nil {
			log.Warn(fmt.Sprintf("failed to list git repos for autocorrect previews: %v", gitRepoListErr),
				logging.WithCollectionRunID(run.ID))
		} else if len(gitReposForAC) > 0 {
			var csResults []datastore.GitRepoCookstyleResult
			for _, repo := range gitReposForAC {
				repoResults, err := c.db.ListGitRepoCookstyleResults(ctx, repo.ID)
				if err != nil {
					log.Warn(fmt.Sprintf("failed to list CookStyle results for git repo %s: %v", repo.Name, err),
						logging.WithCollectionRunID(run.ID))
					continue
				}
				csResults = append(csResults, repoResults...)
			}

			if len(csResults) > 0 {
				log.Info(fmt.Sprintf("generating autocorrect previews for %d git CookStyle result(s)", len(csResults)),
					logging.WithCollectionRunID(run.ID))

				csInfos := make([]remediation.CookstyleResultInfo, 0, len(csResults))
				for _, csr := range csResults {
					csInfos = append(csInfos, remediation.CookstyleResultInfo{
						ResultID:          csr.ID,
						CookbookID:        csr.GitRepoID,
						TargetChefVersion: csr.TargetChefVersion,
						OffenseCount:      csr.OffenceCount,
						Passed:            csr.Passed,
						Source:            remediation.SourceGitRepo,
					})
				}

				repoByID := make(map[string]datastore.GitRepo, len(gitReposForAC))
				for _, repo := range gitReposForAC {
					repoByID[repo.ID] = repo
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
					logging.WithCollectionRunID(run.ID))
			}
		}
	}

	if c.complexityScorer != nil && len(c.cfg.TargetChefVersions) > 0 {
		log.Info("running complexity scoring",
			logging.WithCollectionRunID(run.ID))

		orgCBs, cxListErr := c.db.ListServerCookbooksByOrganisation(ctx, org.ID)
		if cxListErr != nil {
			log.Warn(fmt.Sprintf("failed to list server cookbooks for complexity scoring: %v", cxListErr),
				logging.WithCollectionRunID(run.ID))
		} else {
			cxBatch := c.complexityScorer.ScoreServerCookbooks(ctx, orgCBs, c.cfg.TargetChefVersions, org.ID)
			log.Info(fmt.Sprintf(
				"server cookbook complexity scoring complete: %d total, %d scored, %d skipped, %d errors in %s",
				cxBatch.Total, cxBatch.Scored, cxBatch.Skipped, cxBatch.Errors,
				cxBatch.Duration.Round(time.Millisecond)),
				logging.WithCollectionRunID(run.ID))
		}

		gitReposForCX, grCXListErr := c.db.ListGitRepos(ctx)
		if grCXListErr != nil {
			log.Warn(fmt.Sprintf("failed to list git repos for complexity scoring: %v", grCXListErr),
				logging.WithCollectionRunID(run.ID))
		} else if len(gitReposForCX) > 0 {
			grCXBatch := c.complexityScorer.ScoreGitRepos(ctx, gitReposForCX, c.cfg.TargetChefVersions, org.ID)
			log.Info(fmt.Sprintf(
				"git repo complexity scoring complete: %d total, %d scored, %d skipped, %d errors in %s",
				grCXBatch.Total, grCXBatch.Scored, grCXBatch.Skipped, grCXBatch.Errors,
				grCXBatch.Duration.Round(time.Millisecond)),
				logging.WithCollectionRunID(run.ID))
		}
	}

	// Step 14: Node readiness evaluation. Combines cookbook compatibility
	// data (from CookStyle + Test Kitchen) with disk space evaluation to
	// produce a per-node per-target-version readiness verdict. Skipped if
	// the evaluator is not configured. Non-fatal.
	if c.readinessEval != nil && len(c.cfg.TargetChefVersions) > 0 {
		log.Info("evaluating node readiness",
			logging.WithCollectionRunID(run.ID))

		readinessResults, readinessErr := c.readinessEval.EvaluateOrganisation(ctx, org.ID, org.Name, c.cfg.TargetChefVersions)
		if readinessErr != nil {
			log.Warn(fmt.Sprintf("node readiness evaluation failed: %v", readinessErr),
				logging.WithCollectionRunID(run.ID))
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
				logging.WithCollectionRunID(run.ID))
		}
	}

	// Step 15: Ownership auto-derivation. Evaluates configured rules against
	// the freshly collected data and creates/removes ownership assignments.
	// Skipped when the evaluator is not configured. Non-fatal.
	if c.ownershipEval != nil {
		log.Info("evaluating ownership auto-derivation rules",
			logging.WithCollectionRunID(run.ID))

		if ownerErr := c.ownershipEval.EvaluateAfterCollection(ctx, org.ID, org.Name); ownerErr != nil {
			log.Warn(fmt.Sprintf("ownership evaluation failed: %v", ownerErr),
				logging.WithCollectionRunID(run.ID))
		} else {
			log.Info("ownership evaluation complete",
				logging.WithCollectionRunID(run.ID))
		}
	}

	// Step 16: The collection run was already marked completed in Step 4b
	// after node snapshots were persisted, so the UI could show fresh data
	// while cookbook operations continued. Log final summary.
	log.Info(fmt.Sprintf("collection run %s post-completion processing finished: %d nodes, %d cookbook versions",
		run.ID, inserted, upserted),
		logging.WithCollectionRunID(run.ID))

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
	c.currentRunID = ""
}

// defaultClientFactory resolves credentials and builds a real Chef API client
// for the given organisation.
func (c *Collector) defaultClientFactory(ctx context.Context, org datastore.Organisation) (*chefapi.Client, error) {
	// Determine the credential source for the client key.
	src := secrets.CredentialSource{
		CredentialName: org.ClientKeyCredentialID,
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
