// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// ---------------------------------------------------------------------------
// CookStyle JSON output structures (RuboCop JSON formatter)
// ---------------------------------------------------------------------------
//
// CookStyle's --format json produces RuboCop-compatible JSON. Every offense
// already contains the cop name (which encodes the namespace), severity,
// message, whether it is auto-correctable, and source location. There is no
// need to maintain a separate cop mapping — the JSON output is the single
// source of truth.

// CookstyleOutput represents the top-level JSON output from
// `cookstyle --format json`.
type CookstyleOutput struct {
	Metadata CookstyleMetadata `json:"metadata"`
	Files    []CookstyleFile   `json:"files"`
	Summary  CookstyleSummary  `json:"summary"`
}

// CookstyleMetadata contains version information about the CookStyle/RuboCop
// runtime.
type CookstyleMetadata struct {
	RubocopVersion string `json:"rubocop_version"`
	RubyEngine     string `json:"ruby_engine"`
	RubyVersion    string `json:"ruby_version"`
}

// CookstyleFile represents a single inspected file and its offenses.
type CookstyleFile struct {
	Path     string             `json:"path"`
	Offenses []CookstyleOffense `json:"offenses"`
}

// CookstyleOffense represents a single offense found by CookStyle. The
// struct mirrors the RuboCop JSON formatter output exactly so that
// json.Unmarshal works without any transformation.
type CookstyleOffense struct {
	Severity  string                   `json:"severity"`
	Message   string                   `json:"message"`
	CopName   string                   `json:"cop_name"`
	Corrected bool                     `json:"corrected"`
	Location  CookstyleOffenseLocation `json:"location"`
	// File is the source file path. Not part of the RuboCop per-offense
	// JSON — it comes from the parent CookstyleFile.Path and is set by
	// the scan pipeline after unmarshalling.
	File string `json:"file,omitempty"`
}

// CookstyleOffenseLocation describes the source location of an offense.
type CookstyleOffenseLocation struct {
	StartLine   int `json:"start_line"`
	StartColumn int `json:"start_column"`
	LastLine    int `json:"last_line"`
	LastColumn  int `json:"last_column"`
}

// CookstyleSummary contains aggregate counts from the CookStyle run.
type CookstyleSummary struct {
	OffenseCount       int `json:"offense_count"`
	TargetFileCount    int `json:"target_file_count"`
	InspectedFileCount int `json:"inspected_file_count"`
}

// ---------------------------------------------------------------------------
// Namespace helpers
// ---------------------------------------------------------------------------

// Cop namespace prefixes used for classification. The cop_name field from
// CookStyle JSON starts with one of these followed by a "/".
const (
	nsDeprecations = "Chef/Deprecations/"
	nsCorrectness  = "Chef/Correctness/"
	nsStyle        = "Chef/Style/"
	nsModernize    = "Chef/Modernize/"
)

// isDeprecation returns true if the cop is in the Chef/Deprecations namespace.
func isDeprecation(copName string) bool { return strings.HasPrefix(copName, nsDeprecations) }

// isCorrectness returns true if the cop is in the Chef/Correctness namespace.
func isCorrectness(copName string) bool { return strings.HasPrefix(copName, nsCorrectness) }

// isErrorOrFatal returns true if the severity indicates a hard failure.
func isErrorOrFatal(severity string) bool {
	return severity == "error" || severity == "fatal"
}

// ---------------------------------------------------------------------------
// Scan result
// ---------------------------------------------------------------------------

// CookstyleScanResult holds the outcome of scanning a single cookbook
// version with CookStyle.
type CookstyleScanResult struct {
	// CookbookID is the datastore ID of the scanned cookbook.
	CookbookID string

	// OrganisationID is the owning organisation.
	OrganisationID string

	// CookbookName is the cookbook's display name.
	CookbookName string

	// CookbookVersion is the version string.
	CookbookVersion string

	// TargetChefVersion is the Chef Client version the scan was profiled
	// against. Empty when no version profile was applied.
	TargetChefVersion string

	// CommitSHA is the HEAD commit SHA of the cookbook at the time of
	// scanning. Set for git-sourced cookbooks; empty for server-sourced.
	CommitSHA string

	// Passed is true when there are zero offenses with severity "error"
	// or "fatal".
	Passed bool

	// OffenseCount is the total number of offenses.
	OffenseCount int

	// DeprecationCount is the number of ChefDeprecations/* offenses.
	DeprecationCount int

	// CorrectnessCount is the number of ChefCorrectness/* offenses.
	CorrectnessCount int

	// CorrectableCount is the number of offenses marked corrected/correctable.
	CorrectableCount int

	// Offenses is the full offense list exactly as CookStyle reported it.
	Offenses []CookstyleOffense

	// DeprecationWarnings is the subset in the ChefDeprecations namespace
	// (for prominent dashboard display).
	DeprecationWarnings []CookstyleOffense

	// RawStdout is the raw stdout from the cookstyle process.
	RawStdout string

	// RawStderr is the raw stderr from the cookstyle process.
	RawStderr string

	// Duration is the wall-clock time for this scan.
	Duration time.Duration

	// ScannedAt is the UTC timestamp when the scan completed.
	ScannedAt time.Time

	// Skipped is true when the scan was skipped because an existing
	// result was found in the datastore (immutability optimisation).
	Skipped bool

	// Error is non-nil when the scan itself failed (crash, timeout,
	// invalid JSON). A non-zero exit code with valid JSON is NOT an
	// error — CookStyle exits non-zero whenever offenses are found.
	Error error
}

// ---------------------------------------------------------------------------
// Executor interface
// ---------------------------------------------------------------------------

// CookstyleExecutor abstracts the execution of the cookstyle binary so
// that tests can inject a fake without touching the filesystem.
type CookstyleExecutor interface {
	// Run executes cookstyle with the given arguments and returns
	// stdout, stderr, the exit code, and any execution error.
	//
	// A non-zero exit code is NOT returned as an error when the process
	// ran to completion — CookStyle exits non-zero when offenses are
	// found. An error is returned only for failures to start the
	// process, context cancellation, or signal-based termination.
	Run(ctx context.Context, args ...string) (stdout, stderr string, exitCode int, err error)
}

// ---------------------------------------------------------------------------
// Scanner
// ---------------------------------------------------------------------------

// CookstyleScanner runs CookStyle scans on cookbooks from any source
// (Chef server or git).
type CookstyleScanner struct {
	db            *datastore.DB
	logger        *logging.Logger
	executor      CookstyleExecutor
	concurrency   int
	timeout       time.Duration
	cookstylePath string
}

// CookstyleScannerOption configures a CookstyleScanner.
type CookstyleScannerOption func(*CookstyleScanner)

// WithCookstyleExecutor overrides the command executor (for testing).
func WithCookstyleExecutor(e CookstyleExecutor) CookstyleScannerOption {
	return func(s *CookstyleScanner) { s.executor = e }
}

// NewCookstyleScanner creates a scanner.
//
// Parameters:
//   - db: datastore for checking existing results and persisting new ones
//   - logger: structured logger
//   - cookstylePath: resolved absolute path to the cookstyle binary
//   - concurrency: max parallel scans (worker pool size)
//   - timeoutMinutes: per-scan timeout
//   - opts: optional overrides
func NewCookstyleScanner(
	db *datastore.DB,
	logger *logging.Logger,
	cookstylePath string,
	concurrency int,
	timeoutMinutes int,
	opts ...CookstyleScannerOption,
) *CookstyleScanner {
	if concurrency <= 0 {
		concurrency = 1
	}
	if timeoutMinutes <= 0 {
		timeoutMinutes = 10
	}

	s := &CookstyleScanner{
		db:            db,
		logger:        logger,
		cookstylePath: cookstylePath,
		concurrency:   concurrency,
		timeout:       time.Duration(timeoutMinutes) * time.Minute,
	}
	for _, o := range opts {
		o(s)
	}
	if s.executor == nil {
		s.executor = &defaultCookstyleExecutor{path: cookstylePath}
	}
	return s
}

// ---------------------------------------------------------------------------
// Batch scanning
// ---------------------------------------------------------------------------

// CookstyleBatchResult summarises the outcome of scanning a batch of
// cookbook versions.
type CookstyleBatchResult struct {
	Total    int
	Scanned  int
	Skipped  int
	Passed   int
	Failed   int
	Errors   int
	Duration time.Duration
	Results  []CookstyleScanResult
}

// ScanCookbooks runs CookStyle against all provided cookbooks in parallel,
// once per target Chef Client version. For each combination it:
//
//  1. Checks if a result already exists — server cookbook versions are
//     immutable so an existing result is always valid; git cookbooks are
//     skipped only when the HEAD commit SHA has not changed.
//  2. Runs `cookstyle --format json` on the cookbook directory, optionally
//     restricting to ChefDeprecations + ChefCorrectness via --only.
//  3. Parses the JSON output — every offense already carries the cop name,
//     severity, message, correctable flag, and location.
//  4. Persists the result to the cookstyle_results table.
//
// Server cookbooks with download_status != 'ok' are silently excluded.
// Git cookbooks are always considered available (their content lives in the
// local clone directory).
//
// cookbookDir maps a cookbook to its filesystem path. The caller provides
// this because the directory layout depends on how cookbooks were fetched.
func (s *CookstyleScanner) ScanCookbooks(
	ctx context.Context,
	cookbooks []datastore.Cookbook,
	targetChefVersions []string,
	cookbookDir func(cb datastore.Cookbook) string,
) CookstyleBatchResult {
	start := time.Now()
	log := s.logger.WithScope(logging.ScopeCookstyleScan)

	type workItem struct {
		Cookbook      datastore.Cookbook
		TargetVersion string
		Dir           string
	}

	var items []workItem
	for _, cb := range cookbooks {
		if cb.IsChefServer() && !cb.IsDownloaded() {
			continue
		}
		if !cb.IsChefServer() && !cb.IsGit() {
			continue
		}
		dir := cookbookDir(cb)
		if dir == "" {
			continue
		}
		if len(targetChefVersions) == 0 {
			items = append(items, workItem{Cookbook: cb, Dir: dir})
		} else {
			for _, tv := range targetChefVersions {
				items = append(items, workItem{Cookbook: cb, TargetVersion: tv, Dir: dir})
			}
		}
	}

	result := CookstyleBatchResult{
		Total:   len(items),
		Results: make([]CookstyleScanResult, 0, len(items)),
	}
	if len(items) == 0 {
		result.Duration = time.Since(start)
		return result
	}

	log.Info(fmt.Sprintf("starting CookStyle scans: %d work items, concurrency %d",
		len(items), s.concurrency))

	sem := make(chan struct{}, s.concurrency)
	resultsCh := make(chan CookstyleScanResult, len(items))

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
				resultsCh <- CookstyleScanResult{
					CookbookID:        wi.Cookbook.ID,
					OrganisationID:    wi.Cookbook.OrganisationID,
					CookbookName:      wi.Cookbook.Name,
					CookbookVersion:   wi.Cookbook.Version,
					TargetChefVersion: wi.TargetVersion,
					Error:             ctx.Err(),
				}
				return
			}
			resultsCh <- s.scanOne(ctx, wi.Cookbook, wi.TargetVersion, wi.Dir)
		}(item)
	}
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	for sr := range resultsCh {
		result.Results = append(result.Results, sr)
		switch {
		case sr.Skipped:
			result.Skipped++
		case sr.Error != nil:
			result.Errors++
			log.Error(fmt.Sprintf("cookstyle error: %s/%s (target %s): %v",
				sr.CookbookName, sr.CookbookVersion, sr.TargetChefVersion, sr.Error))
		default:
			result.Scanned++
			if sr.Passed {
				result.Passed++
			} else {
				result.Failed++
			}
		}
	}

	result.Duration = time.Since(start)
	log.Info(fmt.Sprintf(
		"CookStyle batch complete: %d total, %d scanned, %d skipped, %d passed, %d failed, %d errors in %s",
		result.Total, result.Scanned, result.Skipped,
		result.Passed, result.Failed, result.Errors,
		result.Duration.Round(time.Millisecond)))
	return result
}

// ---------------------------------------------------------------------------
// Single cookbook scan
// ---------------------------------------------------------------------------

func (s *CookstyleScanner) scanOne(
	ctx context.Context,
	cb datastore.Cookbook,
	targetChefVersion string,
	cookbookDir string,
) CookstyleScanResult {
	log := s.logger.WithScope(logging.ScopeCookstyleScan,
		logging.WithCookbook(cb.Name, cb.Version))

	sr := CookstyleScanResult{
		CookbookID:        cb.ID,
		OrganisationID:    cb.OrganisationID,
		CookbookName:      cb.Name,
		CookbookVersion:   cb.Version,
		TargetChefVersion: targetChefVersion,
		CommitSHA:         cb.HeadCommitSHA,
	}

	// Step 1: skip check.
	// Server cookbook versions are immutable — an existing result is always valid.
	// Git cookbooks change with each commit — skip only when the HEAD commit
	// SHA matches the previously scanned commit.
	existing, err := s.db.GetCookstyleResult(ctx, cb.ID, targetChefVersion)
	if err == nil && existing != nil {
		if cb.IsChefServer() {
			log.Debug(fmt.Sprintf("skipping — already scanned at %s",
				existing.ScannedAt.Format(time.RFC3339)))
			sr.Skipped = true
			return sr
		}
		if cb.IsGit() && existing.CommitSHA != "" && existing.CommitSHA == cb.HeadCommitSHA {
			shaPreview := cb.HeadCommitSHA
			if len(shaPreview) > 8 {
				shaPreview = shaPreview[:8]
			}
			log.Debug(fmt.Sprintf("skipping — commit %s already scanned at %s",
				shaPreview,
				existing.ScannedAt.Format(time.RFC3339)))
			sr.Skipped = true
			return sr
		}
	}

	// Step 2: build arguments.
	args := buildCookstyleArgs(cookbookDir, targetChefVersion)

	// Step 3: execute with timeout.
	scanStart := time.Now()
	scanCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	stdout, stderr, exitCode, execErr := s.executor.Run(scanCtx, args...)
	sr.Duration = time.Since(scanStart)
	sr.ScannedAt = time.Now().UTC()
	sr.RawStdout = stdout
	sr.RawStderr = stderr

	// Step 4: handle execution failures.
	if execErr != nil {
		if scanCtx.Err() == context.DeadlineExceeded {
			sr.Error = fmt.Errorf("timed out after %s", s.timeout)
			log.Error(fmt.Sprintf("scan timed out after %s", s.timeout))
			s.persistResult(ctx, sr)
			return sr
		}
		if stdout == "" {
			sr.Error = fmt.Errorf("execution failed (exit %d): %v; stderr: %s",
				exitCode, execErr, strings.TrimSpace(stderr))
			log.Error(fmt.Sprintf("execution failed: %v", sr.Error))
			s.persistResult(ctx, sr)
			return sr
		}
		// Non-zero exit with stdout present — fall through to parse JSON.
	}

	// Step 5: parse JSON output.
	var output CookstyleOutput
	if parseErr := json.Unmarshal([]byte(stdout), &output); parseErr != nil {
		if exitCode != 0 {
			sr.Error = fmt.Errorf("exit %d with invalid JSON: %v; stderr: %s",
				exitCode, parseErr, strings.TrimSpace(stderr))
		} else {
			sr.Error = fmt.Errorf("invalid JSON output: %v", parseErr)
		}
		log.Error(fmt.Sprintf("parse error: %v", sr.Error))
		s.persistResult(ctx, sr)
		return sr
	}

	// Step 6: classify offenses using the data CookStyle already provides.
	// Every offense carries cop_name, severity, message, corrected flag,
	// and location — no external mapping needed.
	sr.OffenseCount = output.Summary.OffenseCount
	sr.Passed = true

	for _, file := range output.Files {
		for _, off := range file.Offenses {
			off.File = file.Path
			sr.Offenses = append(sr.Offenses, off)

			if isDeprecation(off.CopName) {
				sr.DeprecationCount++
				sr.DeprecationWarnings = append(sr.DeprecationWarnings, off)
			}
			if isCorrectness(off.CopName) {
				sr.CorrectnessCount++
			}
			if off.Corrected {
				sr.CorrectableCount++
			}
			if isErrorOrFatal(off.Severity) {
				sr.Passed = false
			}
		}
	}

	// Step 7: log outcome.
	if sr.Passed {
		log.Info(fmt.Sprintf("passed: %d offense(s), %d deprecation(s), %d correctness, %d correctable in %s",
			sr.OffenseCount, sr.DeprecationCount, sr.CorrectnessCount, sr.CorrectableCount,
			sr.Duration.Round(time.Millisecond)))
	} else {
		log.Warn(fmt.Sprintf("failed: %d offense(s), %d deprecation(s), %d correctness, %d correctable in %s",
			sr.OffenseCount, sr.DeprecationCount, sr.CorrectnessCount, sr.CorrectableCount,
			sr.Duration.Round(time.Millisecond)))
	}

	// Step 8: persist.
	s.persistResult(ctx, sr)
	return sr
}

// ---------------------------------------------------------------------------
// Argument construction
// ---------------------------------------------------------------------------

// buildCookstyleArgs constructs the cookstyle CLI arguments.
//
// We always pass --format json for machine-parseable output. When a target
// Chef Client version is specified we restrict the scan to the
// ChefDeprecations and ChefCorrectness namespaces via --only, since those
// are the namespaces that directly affect migration compatibility. CookStyle
// handles version-relevance filtering internally within those namespaces.
//
// When no target version is specified the full default rule set runs so
// that the dashboard can display style and modernisation suggestions too.
func buildCookstyleArgs(cookbookDir string, targetChefVersion string) []string {
	args := []string{"--format", "json"}

	if targetChefVersion != "" {
		// Set TargetChefVersion via a sidecar .rubocop_cmm.yml that we
		// point CookStyle at with --config. If the cookbook already has a
		// .rubocop.yml we inherit from it so its settings are preserved.
		// CookStyle does not accept a --target-chef-version CLI flag.
		configPath := writeCookstyleTargetConfig(cookbookDir, targetChefVersion)
		if configPath != "" {
			args = append(args, "--config", configPath)
		}

		// Restrict to the two migration-critical namespaces. CookStyle
		// cops already carry version metadata in their own source —
		// enabling only these namespaces avoids noise from Chef/Style and
		// Chef/Modernize cops that don't affect compatibility.
		args = append(args, "--only", "Chef/Deprecations,Chef/Correctness")
	}

	args = append(args, cookbookDir)
	return args
}

// cmmConfigName is the sidecar config file written next to the cookbook's
// own .rubocop.yml (if any). Using a distinct name avoids overwriting the
// cookbook's configuration.
const cmmConfigName = ".rubocop_cmm.yml"

// writeCookstyleTargetConfig writes a sidecar .rubocop_cmm.yml into the
// cookbook directory that sets AllCops.TargetChefVersion. If the cookbook
// already contains a .rubocop.yml the sidecar inherits from it so the
// cookbook's own configuration (excludes, custom cops, etc.) is preserved.
// When no cookbook config exists the sidecar explicitly requires cookstyle
// so that the TargetChefVersion parameter is recognised.
//
// Returns the absolute path to the written file, or "" on failure.
func writeCookstyleTargetConfig(cookbookDir, targetChefVersion string) string {
	var buf strings.Builder

	existingConfig := filepath.Join(cookbookDir, ".rubocop.yml")
	if _, err := os.Stat(existingConfig); err == nil {
		// Cookbook has its own config — inherit from it (which also
		// picks up any `require: cookstyle` it contains).
		buf.WriteString("inherit_from: .rubocop.yml\n\n")
	} else {
		// No cookbook config — require cookstyle ourselves so the
		// TargetChefVersion AllCops parameter is registered.
		buf.WriteString("require:\n  - cookstyle\n\n")
	}

	fmt.Fprintf(&buf, "AllCops:\n  TargetChefVersion: %s\n", targetChefVersion)

	outPath := filepath.Join(cookbookDir, cmmConfigName)
	if err := os.WriteFile(outPath, []byte(buf.String()), 0644); err != nil {
		return ""
	}
	return outPath
}

// ---------------------------------------------------------------------------
// Result persistence
// ---------------------------------------------------------------------------

// enrichOffenses converts raw CookStyle offenses to enriched offenses that
// include remediation guidance from the embedded cop mapping table. Each
// offense is looked up by cop_name; if a mapping exists the Remediation
// field is populated, otherwise it is nil (omitted from JSON).
func enrichOffenses(offenses []CookstyleOffense) []remediation.EnrichedOffense {
	if len(offenses) == 0 {
		return nil
	}
	enriched := make([]remediation.EnrichedOffense, len(offenses))
	for i, off := range offenses {
		enriched[i] = remediation.EnrichedOffense{
			CopName:  off.CopName,
			Severity: off.Severity,
			Message:  off.Message,
			Location: remediation.OffenseLocation{
				File:        off.File,
				StartLine:   off.Location.StartLine,
				StartColumn: off.Location.StartColumn,
				LastLine:    off.Location.LastLine,
				LastColumn:  off.Location.LastColumn,
			},
			Remediation: remediation.LookupCop(off.CopName),
		}
	}
	return enriched
}

func (s *CookstyleScanner) persistResult(ctx context.Context, sr CookstyleScanResult) {
	if sr.CookbookID == "" {
		return
	}

	log := s.logger.WithScope(logging.ScopeCookstyleScan,
		logging.WithCookbook(sr.CookbookName, sr.CookbookVersion))

	offensesJSON, err := json.Marshal(enrichOffenses(sr.Offenses))
	if err != nil {
		log.Warn(fmt.Sprintf("failed to marshal offenses: %v", err))
		offensesJSON = []byte("[]")
	}
	deprecationsJSON, err := json.Marshal(enrichOffenses(sr.DeprecationWarnings))
	if err != nil {
		log.Warn(fmt.Sprintf("failed to marshal deprecations: %v", err))
		deprecationsJSON = []byte("[]")
	}

	params := datastore.UpsertCookstyleResultParams{
		CookbookID:          sr.CookbookID,
		TargetChefVersion:   sr.TargetChefVersion,
		CommitSHA:           sr.CommitSHA,
		Passed:              sr.Passed,
		OffenceCount:        sr.OffenseCount,
		DeprecationCount:    sr.DeprecationCount,
		CorrectnessCount:    sr.CorrectnessCount,
		DeprecationWarnings: deprecationsJSON,
		Offences:            offensesJSON,
		ProcessStdout:       sr.RawStdout,
		ProcessStderr:       sr.RawStderr,
		DurationSeconds:     int(sr.Duration.Seconds()),
		ScannedAt:           sr.ScannedAt,
	}

	if _, persistErr := s.db.UpsertCookstyleResult(ctx, params); persistErr != nil {
		log.Error(fmt.Sprintf("failed to persist result: %v", persistErr))
	}
}

// ---------------------------------------------------------------------------
// Manual rescan
// ---------------------------------------------------------------------------

// ResetResults deletes existing CookStyle results for the given cookbook,
// so they will be rescanned on the next analysis cycle.
func (s *CookstyleScanner) ResetResults(ctx context.Context, cookbookID string) error {
	return s.db.DeleteCookstyleResultsForCookbook(ctx, cookbookID)
}

// ResetAllResults deletes all CookStyle results for the given organisation.
func (s *CookstyleScanner) ResetAllResults(ctx context.Context, organisationID string) error {
	return s.db.DeleteCookstyleResultsForOrganisation(ctx, organisationID)
}

// ---------------------------------------------------------------------------
// Default executor
// ---------------------------------------------------------------------------

type defaultCookstyleExecutor struct {
	path string
}

func (e *defaultCookstyleExecutor) Run(ctx context.Context, args ...string) (string, string, int, error) {
	return executeCommand(ctx, e.path, args...)
}

// executeCommand runs an external command and returns stdout, stderr, exit
// code, and error. A non-zero exit code from a process that ran to
// completion is NOT returned as an error — the caller inspects the exit
// code and stdout/stderr separately.
func executeCommand(ctx context.Context, name string, args ...string) (string, string, int, error) {
	cmd := makeCommand(ctx, name, args...)

	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	// Process ran to completion but exited non-zero — normal for cookstyle
	// when offenses are found. Return nil error.
	if err != nil && cmd.ProcessState != nil && exitCode != 0 {
		return stdoutBuf.String(), stderrBuf.String(), exitCode, nil
	}

	return stdoutBuf.String(), stderrBuf.String(), exitCode, err
}
