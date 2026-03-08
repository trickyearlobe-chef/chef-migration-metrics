// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// ---------------------------------------------------------------------------
// Kitchen executor interface (for testing)
// ---------------------------------------------------------------------------

// KitchenExecutor abstracts running the `kitchen` CLI so tests can inject a
// fake without touching the filesystem or requiring Docker / Vagrant.
type KitchenExecutor interface {
	// Run executes the kitchen binary with the given arguments in the
	// specified working directory. It returns stdout, stderr, the exit
	// code, and any execution error. A non-zero exit code is NOT returned
	// as an error when the process ran to completion — Test Kitchen exits
	// non-zero when converge or verify fails. An error is returned only
	// for failures to start the process, context cancellation, or
	// signal-based termination.
	Run(ctx context.Context, dir string, args ...string) (stdout, stderr string, exitCode int, err error)
}

// defaultKitchenExecutor shells out to the real kitchen binary.
type defaultKitchenExecutor struct {
	path string
}

func (e *defaultKitchenExecutor) Run(ctx context.Context, dir string, args ...string) (string, string, int, error) {
	cmd := makeCommand(ctx, e.path, args...)
	cmd.Dir = dir

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		// Try to extract exit code from the error.
		var exitErr *exec.ExitError
		if ok := errors.As(err, &exitErr); ok {
			exitCode = exitErr.ExitCode()
			err = nil // Process ran to completion, just non-zero exit.
		}
	}
	return stdoutBuf.String(), stderrBuf.String(), exitCode, err
}

// ---------------------------------------------------------------------------
// Kitchen instance (parsed from `kitchen list --format json`)
// ---------------------------------------------------------------------------

// KitchenInstance represents a single Test Kitchen instance as returned by
// `kitchen list --format json`.
type KitchenInstance struct {
	Instance    string `json:"instance"`
	Driver      string `json:"driver"`
	Provisioner string `json:"provisioner"`
	Verifier    string `json:"verifier"`
	Transport   string `json:"transport"`
	LastAction  string `json:"last_action"`
}

// ---------------------------------------------------------------------------
// Phase result (internal)
// ---------------------------------------------------------------------------

type phaseResult struct {
	Name     string
	Passed   bool
	Output   string // combined stdout + stderr
	ExitCode int
	TimedOut bool
	Err      error
}

// ---------------------------------------------------------------------------
// Kitchen run result (per cookbook × target version)
// ---------------------------------------------------------------------------

// KitchenRunResult holds the outcome of a single Test Kitchen run for one
// cookbook against one target Chef Client version.
type KitchenRunResult struct {
	CookbookID        string
	CookbookName      string
	TargetChefVersion string
	CommitSHA         string
	ConvergePassed    bool
	TestsPassed       bool
	Compatible        bool
	TimedOut          bool
	ConvergeOutput    string
	VerifyOutput      string
	DestroyOutput     string
	DriverUsed        string
	PlatformTested    string
	OverridesApplied  bool
	Duration          time.Duration
	StartedAt         time.Time
	CompletedAt       time.Time
	Skipped           bool
	SkipReason        string
	Error             error
}

// ---------------------------------------------------------------------------
// Batch result
// ---------------------------------------------------------------------------

// KitchenBatchResult summarises the outcome of running Test Kitchen across
// a batch of cookbooks and target versions.
type KitchenBatchResult struct {
	Total    int
	Tested   int
	Skipped  int
	Passed   int
	Failed   int
	Errors   int
	TimedOut int
	Duration time.Duration
	Results  []KitchenRunResult
}

// ---------------------------------------------------------------------------
// Kitchen scanner (the main runner)
// ---------------------------------------------------------------------------

// KitchenScanner runs Test Kitchen against git-sourced cookbooks that have
// test suites. It supports driver and platform overrides so cookbooks can be
// tested against the actual infrastructure and platforms used in production.
type KitchenScanner struct {
	db          *datastore.DB
	logger      *logging.Logger
	executor    KitchenExecutor
	concurrency int
	timeout     time.Duration
	kitchenPath string
	tkConfig    config.TestKitchenConfig
}

// KitchenScannerOption configures a KitchenScanner.
type KitchenScannerOption func(*KitchenScanner)

// WithKitchenExecutor overrides the command executor (for testing).
func WithKitchenExecutor(e KitchenExecutor) KitchenScannerOption {
	return func(s *KitchenScanner) { s.executor = e }
}

// NewKitchenScanner creates a scanner.
//
// Parameters:
//   - db: datastore for checking existing results and persisting new ones
//   - logger: structured logger
//   - kitchenPath: resolved absolute path to the kitchen binary
//   - concurrency: max parallel Test Kitchen runs (worker pool size)
//   - timeoutMinutes: per-run timeout (converge + verify combined)
//   - tkConfig: driver/platform override configuration
//   - opts: optional overrides (e.g. mock executor for testing)
func NewKitchenScanner(
	db *datastore.DB,
	logger *logging.Logger,
	kitchenPath string,
	concurrency int,
	timeoutMinutes int,
	tkConfig config.TestKitchenConfig,
	opts ...KitchenScannerOption,
) *KitchenScanner {
	if concurrency <= 0 {
		concurrency = 1
	}
	if timeoutMinutes <= 0 {
		timeoutMinutes = 30
	}

	s := &KitchenScanner{
		db:          db,
		logger:      logger,
		kitchenPath: kitchenPath,
		concurrency: concurrency,
		timeout:     time.Duration(timeoutMinutes) * time.Minute,
		tkConfig:    tkConfig,
	}
	for _, o := range opts {
		o(s)
	}
	if s.executor == nil {
		s.executor = &defaultKitchenExecutor{path: kitchenPath}
	}
	return s
}

// ---------------------------------------------------------------------------
// Batch execution
// ---------------------------------------------------------------------------

// TestCookbooks runs Test Kitchen against all provided git-sourced cookbooks
// in parallel, once per target Chef Client version. For each combination it:
//
//  1. Checks if a result already exists for the same commit SHA (skip if unchanged).
//  2. Verifies the cookbook has a .kitchen.yml.
//  3. Generates a .kitchen.local.yml with driver, provisioner, and platform overrides.
//  4. Runs converge → verify → destroy.
//  5. Persists the result.
//
// cookbookDir maps a cookbook to its local filesystem clone directory.
func (s *KitchenScanner) TestCookbooks(
	ctx context.Context,
	cookbooks []datastore.Cookbook,
	targetChefVersions []string,
	cookbookDir func(cb datastore.Cookbook) string,
) KitchenBatchResult {
	start := time.Now()
	log := s.logger.WithScope(logging.ScopeTestKitchenRun)

	type workItem struct {
		Cookbook      datastore.Cookbook
		TargetVersion string
		Dir           string
	}

	// Build work items: only git-sourced cookbooks with a test suite.
	var items []workItem
	for _, cb := range cookbooks {
		if !cb.IsGit() || !cb.HasTestSuite {
			continue
		}
		if cb.HeadCommitSHA == "" {
			continue
		}
		dir := cookbookDir(cb)
		if dir == "" {
			continue
		}
		if len(targetChefVersions) == 0 {
			// No target versions configured — skip TK testing entirely.
			continue
		}
		for _, tv := range targetChefVersions {
			items = append(items, workItem{Cookbook: cb, TargetVersion: tv, Dir: dir})
		}
	}

	result := KitchenBatchResult{
		Total:   len(items),
		Results: make([]KitchenRunResult, 0, len(items)),
	}
	if len(items) == 0 {
		result.Duration = time.Since(start)
		return result
	}

	log.Info(fmt.Sprintf("starting Test Kitchen runs: %d work items, concurrency %d",
		len(items), s.concurrency))

	sem := make(chan struct{}, s.concurrency)
	resultsCh := make(chan KitchenRunResult, len(items))

	var wg sync.WaitGroup
	for _, item := range items {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(wi workItem) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				resultsCh <- KitchenRunResult{
					CookbookID:        wi.Cookbook.ID,
					CookbookName:      wi.Cookbook.Name,
					TargetChefVersion: wi.TargetVersion,
					CommitSHA:         wi.Cookbook.HeadCommitSHA,
					Error:             ctx.Err(),
				}
				return
			}
			resultsCh <- s.testOne(ctx, wi.Cookbook, wi.TargetVersion, wi.Dir)
		}(item)
	}
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	for rr := range resultsCh {
		result.Results = append(result.Results, rr)
		switch {
		case rr.Skipped:
			result.Skipped++
		case rr.Error != nil:
			result.Errors++
			log.Error(fmt.Sprintf("test kitchen error: %s (target %s): %v",
				rr.CookbookName, rr.TargetChefVersion, rr.Error))
		case rr.TimedOut:
			result.TimedOut++
			result.Failed++
			result.Tested++
			log.Error(fmt.Sprintf("test kitchen timed out: %s (target %s) after %s",
				rr.CookbookName, rr.TargetChefVersion, rr.Duration.Round(time.Second)))
		default:
			result.Tested++
			if rr.Compatible {
				result.Passed++
			} else {
				result.Failed++
			}
		}
	}

	result.Duration = time.Since(start)
	log.Info(fmt.Sprintf(
		"Test Kitchen batch complete: %d total, %d tested, %d skipped, %d passed, %d failed (%d timed out), %d errors in %s",
		result.Total, result.Tested, result.Skipped,
		result.Passed, result.Failed, result.TimedOut,
		result.Errors, result.Duration.Round(time.Millisecond)))
	return result
}

// ---------------------------------------------------------------------------
// Single cookbook × target version test run
// ---------------------------------------------------------------------------

func (s *KitchenScanner) testOne(ctx context.Context, cb datastore.Cookbook, targetVersion, dir string) KitchenRunResult {
	log := s.logger.WithScope(logging.ScopeTestKitchenRun,
		logging.WithOrganisation(cb.Name))

	result := KitchenRunResult{
		CookbookID:        cb.ID,
		CookbookName:      cb.Name,
		TargetChefVersion: targetVersion,
		CommitSHA:         cb.HeadCommitSHA,
		StartedAt:         time.Now().UTC(),
	}

	// Guard: if the datastore is not configured, we cannot check skip
	// conditions or persist results. Return an error immediately.
	if s.db == nil {
		result.Error = fmt.Errorf("datastore not configured")
		return result
	}

	// Step 1: Check skip condition — if a result exists for this commit SHA,
	// skip the run.
	existing, err := s.db.GetLatestTestKitchenResult(ctx, cb.ID, targetVersion)
	if err != nil {
		result.Error = fmt.Errorf("checking existing result: %w", err)
		return result
	}
	if existing != nil && existing.CommitSHA == cb.HeadCommitSHA {
		log.Info(fmt.Sprintf("skipping %s (target %s): commit %s already tested",
			cb.Name, targetVersion, truncSHA(cb.HeadCommitSHA)))
		result.Skipped = true
		result.SkipReason = fmt.Sprintf("commit %s already tested", truncSHA(cb.HeadCommitSHA))
		return result
	}

	// Step 2: Check for .kitchen.yml.
	kitchenYMLPath := findKitchenYML(dir)
	if kitchenYMLPath == "" {
		log.Warn(fmt.Sprintf("skipping %s: no .kitchen.yml found in %s", cb.Name, dir))
		result.Skipped = true
		result.SkipReason = "no .kitchen.yml"
		return result
	}

	// Step 3: Detect the driver in use from the cookbook's .kitchen.yml.
	detectedDriver := detectDriver(kitchenYMLPath)

	// Step 4: Generate .kitchen.local.yml overlay.
	overlayPath := filepath.Join(dir, ".kitchen.local.yml")
	overlayContent := s.buildOverlay(targetVersion, detectedDriver)
	overridesApplied := overlayContent != ""
	result.OverridesApplied = overridesApplied

	if overridesApplied {
		if err := os.WriteFile(overlayPath, []byte(overlayContent), 0644); err != nil {
			result.Error = fmt.Errorf("writing .kitchen.local.yml: %w", err)
			return result
		}
	}

	// Always clean up overlay and destroy instances on exit.
	defer func() {
		if overridesApplied {
			_ = os.Remove(overlayPath)
		}
	}()

	// Determine what driver and platform we're actually using for metadata.
	result.DriverUsed = s.effectiveDriver(detectedDriver)
	result.PlatformTested = s.effectivePlatformSummary()

	// Step 5: Discover instances via `kitchen list --format json`.
	instances, listErr := s.listInstances(ctx, dir)
	if listErr != nil {
		result.Error = fmt.Errorf("kitchen list: %w", listErr)
		s.destroyBestEffort(ctx, dir, &result)
		return result
	}
	if len(instances) == 0 {
		log.Error(fmt.Sprintf("no Test Kitchen instances defined for %s", cb.Name))
		result.Error = fmt.Errorf("no Test Kitchen instances defined")
		s.destroyBestEffort(ctx, dir, &result)
		return result
	}

	log.Info(fmt.Sprintf("testing %s (target %s, commit %s): %d instance(s), driver=%s",
		cb.Name, targetVersion, truncSHA(cb.HeadCommitSHA), len(instances), result.DriverUsed))

	// Step 6: Run converge with timeout.
	convergeCtx, convergeCancel := context.WithTimeout(ctx, s.timeout)
	convergeResult := s.runPhase(convergeCtx, dir, "converge")
	convergeCancel()

	result.ConvergeOutput = convergeResult.Output
	result.ConvergePassed = convergeResult.Passed

	if convergeResult.TimedOut {
		result.TimedOut = true
		result.ConvergePassed = false
		result.TestsPassed = false
		result.Compatible = false
		result.CompletedAt = time.Now().UTC()
		result.Duration = result.CompletedAt.Sub(result.StartedAt)
		s.destroyBestEffort(ctx, dir, &result)
		s.persistResult(ctx, cb, result)
		return result
	}

	if convergeResult.Err != nil && !convergeResult.Passed {
		result.Error = fmt.Errorf("converge execution error: %w", convergeResult.Err)
		result.TestsPassed = false
		result.Compatible = false
		result.CompletedAt = time.Now().UTC()
		result.Duration = result.CompletedAt.Sub(result.StartedAt)
		s.destroyBestEffort(ctx, dir, &result)
		s.persistResult(ctx, cb, result)
		return result
	}

	// Step 7: Run verify (only if converge passed).
	if convergeResult.Passed {
		verifyCtx, verifyCancel := context.WithTimeout(ctx, s.timeout)
		verifyResult := s.runPhase(verifyCtx, dir, "verify")
		verifyCancel()

		result.VerifyOutput = verifyResult.Output
		result.TestsPassed = verifyResult.Passed

		if verifyResult.TimedOut {
			result.TimedOut = true
			result.TestsPassed = false
		}
	} else {
		result.TestsPassed = false
	}

	result.Compatible = result.ConvergePassed && result.TestsPassed

	// Step 8: Destroy (always, regardless of pass/fail).
	s.destroyBestEffort(ctx, dir, &result)

	result.CompletedAt = time.Now().UTC()
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	// Step 9: Persist result.
	s.persistResult(ctx, cb, result)

	// Step 10: Log outcome.
	if result.Compatible {
		log.Info(fmt.Sprintf("PASS: %s (target %s, commit %s) in %s",
			cb.Name, targetVersion, truncSHA(cb.HeadCommitSHA),
			result.Duration.Round(time.Second)))
	} else {
		log.Error(fmt.Sprintf("FAIL: %s (target %s, commit %s) converge=%v verify=%v in %s",
			cb.Name, targetVersion, truncSHA(cb.HeadCommitSHA),
			result.ConvergePassed, result.TestsPassed,
			result.Duration.Round(time.Second)))
	}

	return result
}

// ---------------------------------------------------------------------------
// Phase execution
// ---------------------------------------------------------------------------

func (s *KitchenScanner) runPhase(ctx context.Context, dir, phase string) phaseResult {
	args := []string{phase, "--concurrency=1", "--log-level=info"}

	stdout, stderr, exitCode, err := s.executor.Run(ctx, dir, args...)

	combined := stdout
	if stderr != "" {
		if combined != "" {
			combined += "\n"
		}
		combined += stderr
	}

	pr := phaseResult{
		Name:     phase,
		Output:   combined,
		ExitCode: exitCode,
		Passed:   exitCode == 0 && err == nil,
	}

	if err != nil {
		pr.Err = err
		pr.Passed = false
		// Check if this was a timeout.
		if ctx.Err() == context.DeadlineExceeded {
			pr.TimedOut = true
		}
	}

	return pr
}

// destroyBestEffort runs `kitchen destroy` and records the output in the
// result but never fails the overall run. Uses a fresh context with a
// 5-minute timeout so that destroy can proceed even if the parent context
// was cancelled.
func (s *KitchenScanner) destroyBestEffort(ctx context.Context, dir string, result *KitchenRunResult) {
	destroyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	stdout, stderr, _, err := s.executor.Run(destroyCtx, dir, "destroy", "--concurrency=1")

	combined := stdout
	if stderr != "" {
		if combined != "" {
			combined += "\n"
		}
		combined += stderr
	}
	result.DestroyOutput = combined

	if err != nil {
		log := s.logger.WithScope(logging.ScopeTestKitchenRun)
		log.Warn(fmt.Sprintf("kitchen destroy failed for %s: %v", result.CookbookName, err))
	}
}

// ---------------------------------------------------------------------------
// Instance discovery
// ---------------------------------------------------------------------------

func (s *KitchenScanner) listInstances(ctx context.Context, dir string) ([]KitchenInstance, error) {
	stdout, _, _, err := s.executor.Run(ctx, dir, "list", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("failed to run kitchen list: %w", err)
	}

	stdout = strings.TrimSpace(stdout)
	if stdout == "" || stdout == "[]" {
		return nil, nil
	}

	var instances []KitchenInstance
	if err := json.Unmarshal([]byte(stdout), &instances); err != nil {
		return nil, fmt.Errorf("failed to parse kitchen list JSON: %w", err)
	}
	return instances, nil
}

// ---------------------------------------------------------------------------
// .kitchen.local.yml overlay generation
// ---------------------------------------------------------------------------

// buildOverlay generates the content of a .kitchen.local.yml file that
// overrides the provisioner (target Chef version), driver, and platforms.
// Returns an empty string if no overlay is needed (no target version, no
// driver override, no platform overrides, no extra YAML).
func (s *KitchenScanner) buildOverlay(targetVersion, detectedDriver string) string {
	var buf bytes.Buffer

	buf.WriteString("# .kitchen.local.yml — generated by chef-migration-metrics\n")
	buf.WriteString("# DO NOT EDIT — this file is overwritten on each test run\n")

	hasContent := false

	// --- Driver override ---
	if s.tkConfig.DriverOverride != "" {
		buf.WriteString("\ndriver:\n")
		buf.WriteString(fmt.Sprintf("  name: %s\n", s.tkConfig.DriverOverride))
		// Merge driver_config entries.
		for k, v := range s.tkConfig.DriverConfig {
			buf.WriteString(fmt.Sprintf("  %s: %s\n", k, yamlScalar(v)))
		}
		hasContent = true
	} else if len(s.tkConfig.DriverConfig) > 0 {
		// No driver name override but there is driver config to merge.
		buf.WriteString("\ndriver:\n")
		for k, v := range s.tkConfig.DriverConfig {
			buf.WriteString(fmt.Sprintf("  %s: %s\n", k, yamlScalar(v)))
		}
		hasContent = true
	}

	// --- Provisioner override (target Chef version) ---
	if targetVersion != "" {
		effectiveDriver := s.effectiveDriver(detectedDriver)
		buf.WriteString("\nprovisioner:\n")
		if effectiveDriver == "dokken" {
			buf.WriteString(fmt.Sprintf("  chef_version: %q\n", targetVersion))
		} else {
			buf.WriteString(fmt.Sprintf("  product_version: %q\n", targetVersion))
		}
		hasContent = true
	}

	// --- Platform overrides ---
	if len(s.tkConfig.PlatformOverrides) > 0 {
		buf.WriteString("\nplatforms:\n")
		for _, p := range s.tkConfig.PlatformOverrides {
			buf.WriteString(fmt.Sprintf("  - name: %s\n", p.Name))
			if len(p.Driver) > 0 {
				buf.WriteString("    driver:\n")
				for k, v := range p.Driver {
					buf.WriteString(fmt.Sprintf("      %s: %s\n", k, yamlScalar(v)))
				}
			}
			if len(p.Attributes) > 0 {
				buf.WriteString("    attributes:\n")
				writeAttributes(&buf, p.Attributes, 6)
			}
		}
		hasContent = true
	}

	// --- Extra YAML (escape hatch) ---
	if s.tkConfig.ExtraYAML != "" {
		buf.WriteString("\n")
		buf.WriteString(s.tkConfig.ExtraYAML)
		if !strings.HasSuffix(s.tkConfig.ExtraYAML, "\n") {
			buf.WriteString("\n")
		}
		hasContent = true
	}

	if !hasContent {
		return ""
	}
	return buf.String()
}

// effectiveDriver returns the driver that will actually be used, considering
// the override. If no override is set, falls back to what was detected in
// the cookbook's .kitchen.yml.
func (s *KitchenScanner) effectiveDriver(detectedDriver string) string {
	if s.tkConfig.DriverOverride != "" {
		return s.tkConfig.DriverOverride
	}
	if detectedDriver != "" {
		return detectedDriver
	}
	return "unknown"
}

// effectivePlatformSummary returns a short human-readable summary of the
// platforms being tested. If overrides are set, lists their names;
// otherwise returns "cookbook-defined".
func (s *KitchenScanner) effectivePlatformSummary() string {
	if len(s.tkConfig.PlatformOverrides) == 0 {
		return "cookbook-defined"
	}
	names := make([]string, 0, len(s.tkConfig.PlatformOverrides))
	for _, p := range s.tkConfig.PlatformOverrides {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

func (s *KitchenScanner) persistResult(ctx context.Context, cb datastore.Cookbook, rr KitchenRunResult) {
	// Combine per-phase outputs into the legacy process_stdout field for
	// backward compatibility with the original schema.
	var combinedStdout bytes.Buffer
	if rr.ConvergeOutput != "" {
		combinedStdout.WriteString("=== CONVERGE ===\n")
		combinedStdout.WriteString(rr.ConvergeOutput)
		combinedStdout.WriteString("\n")
	}
	if rr.VerifyOutput != "" {
		combinedStdout.WriteString("=== VERIFY ===\n")
		combinedStdout.WriteString(rr.VerifyOutput)
		combinedStdout.WriteString("\n")
	}
	if rr.DestroyOutput != "" {
		combinedStdout.WriteString("=== DESTROY ===\n")
		combinedStdout.WriteString(rr.DestroyOutput)
		combinedStdout.WriteString("\n")
	}

	var processStderr string
	if rr.Error != nil {
		processStderr = rr.Error.Error()
	}

	params := datastore.UpsertTestKitchenResultParams{
		CookbookID:        cb.ID,
		TargetChefVersion: rr.TargetChefVersion,
		CommitSHA:         rr.CommitSHA,
		ConvergePassed:    rr.ConvergePassed,
		TestsPassed:       rr.TestsPassed,
		Compatible:        rr.Compatible,
		TimedOut:          rr.TimedOut,
		ProcessStdout:     combinedStdout.String(),
		ProcessStderr:     processStderr,
		ConvergeOutput:    rr.ConvergeOutput,
		VerifyOutput:      rr.VerifyOutput,
		DestroyOutput:     rr.DestroyOutput,
		DriverUsed:        rr.DriverUsed,
		PlatformTested:    rr.PlatformTested,
		OverridesApplied:  rr.OverridesApplied,
		DurationSeconds:   int(rr.Duration.Seconds()),
		StartedAt:         rr.StartedAt,
		CompletedAt:       rr.CompletedAt,
	}

	if _, err := s.db.UpsertTestKitchenResult(ctx, params); err != nil {
		log := s.logger.WithScope(logging.ScopeTestKitchenRun)
		log.Error(fmt.Sprintf("failed to persist test kitchen result for %s (target %s): %v",
			cb.Name, rr.TargetChefVersion, err))
	}
}

// ResetResults deletes all test kitchen results for the given cookbook,
// forcing a full retest on the next analysis cycle regardless of commit SHA.
func (s *KitchenScanner) ResetResults(cookbookID string) error {
	return s.db.DeleteTestKitchenResultsForCookbook(context.Background(), cookbookID)
}

// ResetAllResults deletes all test kitchen results for all git-sourced
// cookbooks. This is the nuclear option.
func (s *KitchenScanner) ResetAllResults() error {
	// There's no single "delete all" method, but we can list and delete.
	// For now, surface the cookbook-level delete through the scanner.
	return fmt.Errorf("not implemented: use DeleteTestKitchenResultsForCookbook per cookbook")
}

// ---------------------------------------------------------------------------
// .kitchen.yml detection and driver parsing
// ---------------------------------------------------------------------------

// findKitchenYML returns the path to the kitchen configuration file in the
// given directory. It checks for .kitchen.yml, .kitchen.yaml, kitchen.yml,
// and kitchen.yaml in that order. Returns empty string if none found.
func findKitchenYML(dir string) string {
	candidates := []string{
		".kitchen.yml",
		".kitchen.yaml",
		"kitchen.yml",
		"kitchen.yaml",
	}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

// detectDriver reads a .kitchen.yml file and extracts the driver name.
// Returns the driver name (e.g. "dokken", "vagrant") or empty string if
// the driver cannot be determined.
//
// This is a best-effort parser — it looks for the `driver:` top-level key
// and the `name:` sub-key. It does NOT use a full YAML parser because the
// only information we need is the driver name for overlay generation, and
// a simple line-scan avoids importing a YAML dependency into the analysis
// package.
func detectDriver(kitchenYMLPath string) string {
	data, err := os.ReadFile(kitchenYMLPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	inDriverBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Check for top-level `driver:` key (no leading whitespace).
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if strings.HasPrefix(trimmed, "driver:") {
				// Inline form: `driver: { name: dokken }` or `driver:`
				after := strings.TrimPrefix(trimmed, "driver:")
				after = strings.TrimSpace(after)
				if after != "" && after != "{" {
					// Could be `driver: dokken` (shorthand, not valid TK but handle it)
					return ""
				}
				inDriverBlock = true
				continue
			}
			inDriverBlock = false
			continue
		}

		if inDriverBlock {
			if strings.HasPrefix(trimmed, "name:") {
				val := strings.TrimPrefix(trimmed, "name:")
				val = strings.TrimSpace(val)
				val = strings.Trim(val, `"'`)
				return val
			}
			// If we hit another top-level key (no extra indentation relative
			// to driver), stop.
			indent := countLeadingSpaces(line)
			if indent == 0 {
				inDriverBlock = false
			}
		}
	}
	return ""
}

// countLeadingSpaces returns the number of leading space characters in s.
func countLeadingSpaces(s string) int {
	count := 0
	for _, ch := range s {
		if ch == ' ' {
			count++
		} else if ch == '\t' {
			count += 2 // Treat tabs as 2 spaces.
		} else {
			break
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// YAML helpers (minimal — avoids importing a full YAML library)
// ---------------------------------------------------------------------------

// yamlScalar formats a string as a YAML scalar value. If the value contains
// characters that are special in YAML, it is double-quoted. Otherwise it is
// written bare.
func yamlScalar(v string) string {
	if v == "" {
		return `""`
	}
	// Only quote if the value contains characters that are genuinely
	// ambiguous or special as YAML *values*. Hyphens, dots, slashes, and
	// equals signs are safe inside scalar values — they only cause trouble
	// as leading characters in flow/block contexts that don't apply here.
	special := `:{}[]&*#?|!@` + "`" + `"' `
	for _, ch := range v {
		if strings.ContainsRune(special, ch) {
			return fmt.Sprintf("%q", v)
		}
	}
	return v
}

// writeAttributes writes a map of attributes as indented YAML. The indent
// parameter is the number of leading spaces for each key.
func writeAttributes(buf *bytes.Buffer, attrs map[string]interface{}, indent int) {
	prefix := strings.Repeat(" ", indent)
	for k, v := range attrs {
		switch val := v.(type) {
		case map[string]interface{}:
			buf.WriteString(fmt.Sprintf("%s%s:\n", prefix, k))
			writeAttributes(buf, val, indent+2)
		default:
			buf.WriteString(fmt.Sprintf("%s%s: %s\n", prefix, k, yamlScalar(fmt.Sprintf("%v", val))))
		}
	}
}

// truncSHA returns the first 8 characters of a SHA string for log messages.
// If the SHA is shorter than 8 characters, it is returned as-is.
func truncSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
