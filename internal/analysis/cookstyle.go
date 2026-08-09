// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"encoding/json"
	"fmt"
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
	Severity string `json:"severity"`
	Message  string `json:"message"`
	CopName  string `json:"cop_name"`

	// Correctable is the cop's static capability: whether CookStyle is able
	// to fix this offense at all. It is reported on every scan, correcting
	// or not.
	//
	// Corrected reports whether a correcting run actually changed the file.
	// It is always false on a plain scan, and stays false for corrections
	// RuboCop considers unsafe, because CMM runs --auto-correct rather than
	// --auto-correct-all. The two are therefore independent: neither can be
	// derived from the other. See internal/analysis/testdata/README.md.
	Correctable bool `json:"correctable"`
	Corrected   bool `json:"corrected"`

	Location CookstyleOffenseLocation `json:"location"`
	// File is the source file path. Not part of the RuboCop per-offense
	// JSON — it comes from the parent CookstyleFile.Path and is set by
	// the scan pipeline after unmarshalling.
	File string `json:"file,omitempty"`
}

// Path returns the repo-relative source path of this offense, whichever field
// carries it. A freshly scanned offense has it in File (set from the parent
// CookstyleFile.Path); one read back from the offences JSONB has it in
// Location.File. Callers deciding scope should use this rather than either
// field, because both shapes are in circulation.
func (o CookstyleOffense) Path() string {
	if o.File != "" {
		return o.File
	}
	return o.Location.File
}

// CookstyleOffenseLocation describes the source location of an offense.
type CookstyleOffenseLocation struct {
	// File is the repo-relative source path. RuboCop does not report it here —
	// it reports it once per file — but the persisted offence does
	// (remediation.OffenseLocation), so reading a stored offence back into this
	// struct recovers the path. Without it every read path lost the path and
	// nothing downstream could tell cookbook code from a helper task.
	File        string `json:"file,omitempty"`
	StartLine   int    `json:"start_line"`
	StartColumn int    `json:"start_column"`
	LastLine    int    `json:"last_line"`
	LastColumn  int    `json:"last_column"`
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
	// OrganisationName identifies the organisation (server cookbooks only).
	OrganisationName string

	// GitRepoURL is the repository URL (git repos only).
	GitRepoURL string

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

	// Passed is the back-compat boolean = CookstyleStatus != StatusBlocked.
	Passed bool

	// CookstyleStatus is the classification-derived rollup verdict for this
	// scan: StatusReady / StatusNeedsReview / StatusBlocked. It is the single
	// source of truth; Passed is derived from it.
	CookstyleStatus string

	// OffenseCount is the total number of offenses.
	OffenseCount int

	// DeprecationCount is the number of ChefDeprecations/* offenses.
	DeprecationCount int

	// CorrectnessCount is the number of ChefCorrectness/* offenses.
	CorrectnessCount int

	// CorrectableCount is the number of offenses CookStyle reports as
	// correctable — the static capability, not what a correcting run fixed.
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

	// ErrorMessage is a human-readable error description set when CookStyle
	// itself crashes (exit code >= 2) rather than finding offenses (exit
	// code 1). When non-empty, this result should not be treated as a
	// compatibility verdict — the cookbook is effectively untested.
	ErrorMessage string

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
	// dir is the working directory for the process. It MUST be set to the
	// cookbook/repo directory being scanned: RuboCop reports offence file
	// paths relative to the process CWD, so an empty dir under the systemd
	// default CWD of "/" makes it strip the leading two characters off every
	// absolute path (/var/lib → ar/lib). Pass "" only for invocations that do
	// not scan a cookbook (e.g. --show-cops).
	//
	// A non-zero exit code is NOT returned as an error when the process
	// ran to completion — CookStyle exits non-zero when offenses are
	// found. An error is returned only for failures to start the
	// process, context cancellation, or signal-based termination.
	Run(ctx context.Context, dir string, args ...string) (stdout, stderr string, exitCode int, err error)
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
	// concurrencyFn, when set, returns the live max-parallel-scans value, read
	// at the start of each batch so a config change takes effect on the next
	// collection run without a restart. Falls back to the baked concurrency.
	concurrencyFn func() int
	// classificationOverridesFn, when set, returns the operator classification
	// overrides for a target version, read at scan time. Falls back to loading
	// from the datastore (or empty overrides when no db is configured).
	classificationOverridesFn func(ctx context.Context, targetChefVersion string) map[string]string
	// addonCopPathsFn, when set, returns the live operator addon cop path entries
	// (analysis_tools.cookstyle_addon_cop_paths), read at scan time so a config
	// change takes effect on the next scan without a restart. Falls back to no
	// addon cops.
	addonCopPathsFn func() []string
}

// CookstyleScannerOption configures a CookstyleScanner.
type CookstyleScannerOption func(*CookstyleScanner)

// WithCookstyleExecutor overrides the command executor (for testing).
func WithCookstyleExecutor(e CookstyleExecutor) CookstyleScannerOption {
	return func(s *CookstyleScanner) { s.executor = e }
}

// WithCookstyleConcurrencyFunc sets a live provider for the scan concurrency.
// When set, the scanner reads the worker-pool size on each batch rather than
// using the value baked at construction, so concurrency.cookstyle_scan applies
// at the next run without a restart.
func WithCookstyleConcurrencyFunc(fn func() int) CookstyleScannerOption {
	return func(s *CookstyleScanner) { s.concurrencyFn = fn }
}

// WithCookstyleClassificationOverridesFn wires a live provider of operator
// classification overrides (cop_name → classification) for a target version,
// read at scan time. When unset, overrides are loaded from the datastore.
func WithCookstyleClassificationOverridesFn(fn func(ctx context.Context, targetChefVersion string) map[string]string) CookstyleScannerOption {
	return func(s *CookstyleScanner) { s.classificationOverridesFn = fn }
}

// WithCookstyleAddonCopPathsFn wires a live provider of operator addon cop path
// entries (analysis_tools.cookstyle_addon_cop_paths), read at scan time. When
// unset, no addon cops are loaded.
func WithCookstyleAddonCopPathsFn(fn func() []string) CookstyleScannerOption {
	return func(s *CookstyleScanner) { s.addonCopPathsFn = fn }
}

// effectiveConcurrency returns the live concurrency when a provider is wired
// (clamped to >= 1), otherwise the value baked at construction.
func (s *CookstyleScanner) effectiveConcurrency() int {
	if s.concurrencyFn != nil {
		if n := s.concurrencyFn(); n >= 1 {
			return n
		}
	}
	return s.concurrency
}

// buildResolver constructs a cop classification resolver for the given target
// version, loading operator overrides from the injected provider or the
// datastore. Safe with a nil datastore (empty overrides). The resolver still
// applies RemovedIn auto-seed and curated defaults, so classification works
// even with no operator overrides.
func (s *CookstyleScanner) buildResolver(ctx context.Context, targetChefVersion string) *CopClassificationResolver {
	if s.classificationOverridesFn != nil {
		overrides := s.classificationOverridesFn(ctx, targetChefVersion)
		if overrides == nil {
			overrides = map[string]string{}
		}
		return &CopClassificationResolver{OperatorOverrides: overrides, TargetChefVersion: targetChefVersion}
	}
	if s.db == nil {
		return &CopClassificationResolver{OperatorOverrides: map[string]string{}, TargetChefVersion: targetChefVersion}
	}
	return NewResolverFromStore(ctx, s.db, targetChefVersion)
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

// ScanGitRepos runs CookStyle against all provided git repos in parallel,
// against the given target Chef Client version. For each repo it:
//
//  1. Checks if a result already exists — git repos are skipped only when
//     the HEAD commit SHA has not changed since the last scan.
//  2. Runs `cookstyle --format json` on the repo directory, optionally
//     restricting to ChefDeprecations + ChefCorrectness via --only.
//  3. Parses the JSON output — every offense already carries the cop name,
//     severity, message, correctable flag, and location.
//  4. Persists the result to the git_repo_cookstyle_results table.
//
// repoDir maps a git repo to its filesystem path. The caller provides
// this because the directory layout depends on how repos were cloned.
func (s *CookstyleScanner) ScanGitRepos(
	ctx context.Context,
	repos []datastore.GitRepo,
	targetChefVersion string,
	repoDir func(gr datastore.GitRepo) string,
) CookstyleBatchResult {
	start := time.Now()
	log := s.logger.WithScope(logging.ScopeCookstyleScan)

	type workItem struct {
		Repo          datastore.GitRepo
		TargetVersion string
		Dir           string
	}

	var items []workItem
	for _, gr := range repos {
		dir := repoDir(gr)
		if dir == "" {
			continue
		}
		items = append(items, workItem{Repo: gr, TargetVersion: targetChefVersion, Dir: dir})
	}

	result := CookstyleBatchResult{
		Total:   len(items),
		Results: make([]CookstyleScanResult, 0, len(items)),
	}
	if len(items) == 0 {
		result.Duration = time.Since(start)
		return result
	}

	concurrency := s.effectiveConcurrency()
	log.Info(fmt.Sprintf("starting CookStyle scans (git repos): %d work items, concurrency %d",
		len(items), concurrency))

	sem := make(chan struct{}, concurrency)
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
					CookbookName:      wi.Repo.Name,
					GitRepoURL:        wi.Repo.GitRepoURL,
					TargetChefVersion: wi.TargetVersion,
					CommitSHA:         wi.Repo.HeadCommitSHA,
					Error:             ctx.Err(),
				}
				return
			}
			resultsCh <- s.scanOneGitRepo(ctx, wi.Repo, wi.TargetVersion, wi.Dir)
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
			log.Error(fmt.Sprintf("cookstyle error: %s (target %s): %v",
				sr.CookbookName, sr.TargetChefVersion, sr.Error))
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
		"CookStyle batch complete (git repos): %d total, %d scanned, %d skipped, %d passed, %d failed, %d errors in %s",
		result.Total, result.Scanned, result.Skipped,
		result.Passed, result.Failed, result.Errors,
		result.Duration.Round(time.Millisecond)))
	return result
}

// ScanSingleServerCookbook scans a single server cookbook against a single
// target Chef version and returns the result. This is used by the streaming
// download-scan-delete pipeline, where each cookbook is scanned immediately
// after download and deleted afterwards to minimise disk usage.
//
// The caller is responsible for checking whether the cookbook has already
// been scanned (IsDownloaded, etc.) — this method does NOT filter.
func (s *CookstyleScanner) ScanSingleServerCookbook(
	ctx context.Context,
	sc datastore.ServerCookbook,
	targetChefVersion string,
	cookbookDir string,
) CookstyleScanResult {
	return s.scanOneServerCookbook(ctx, sc, targetChefVersion, cookbookDir)
}

// ScanSingleGitRepo scans a single git repo against a single target Chef
// version and returns the result.
func (s *CookstyleScanner) ScanSingleGitRepo(
	ctx context.Context,
	gr datastore.GitRepo,
	targetChefVersion string,
	repoDir string,
) CookstyleScanResult {
	return s.scanOneGitRepo(ctx, gr, targetChefVersion, repoDir)
}

// ---------------------------------------------------------------------------
// Single server cookbook scan
// ---------------------------------------------------------------------------

func (s *CookstyleScanner) scanOneServerCookbook(
	ctx context.Context,
	sc datastore.ServerCookbook,
	targetChefVersion string,
	cookbookDir string,
) CookstyleScanResult {
	log := s.logger.WithScope(logging.ScopeCookstyleScan,
		logging.WithCookbook(sc.Name, sc.Version))

	sr := CookstyleScanResult{
		OrganisationName:  sc.OrganisationName,
		CookbookName:      sc.Name,
		CookbookVersion:   sc.Version,
		TargetChefVersion: targetChefVersion,
	}

	// Step 1: skip check.
	// Server cookbook versions are immutable — an existing result is always
	// valid. However, if the previous result was an error (exit code >= 2),
	// re-scan in case the issue has been resolved (e.g. CookStyle update).
	existing, err := s.db.GetServerCookbookCookstyleResult(ctx, sc.OrganisationName, sc.Name, sc.Version, targetChefVersion)
	if err == nil && existing != nil && existing.ErrorMessage == "" {
		log.Debug(fmt.Sprintf("skipping — already scanned at %s",
			existing.ScannedAt.Format(time.RFC3339)))
		sr.Skipped = true
		return sr
	}

	// Step 2+3: execute with timeout, injecting operator addon cops and
	// isolating any addon load failure (a broken .rb must not error the scan).
	scanStart := time.Now()
	scanCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	stdout, stderr, exitCode, addonInfo, execErr := s.runScanWithAddonIsolation(scanCtx, cookbookDir, targetChefVersion)
	logAddonScanInfo(log, addonInfo)
	sr.Duration = time.Since(scanStart)
	sr.ScannedAt = time.Now().UTC()
	sr.RawStdout = stdout
	sr.RawStderr = stderr

	// Step 4: handle execution failures.
	if execErr != nil {
		if scanCtx.Err() == context.DeadlineExceeded {
			sr.Error = fmt.Errorf("timed out after %s", s.timeout)
			log.Error(fmt.Sprintf("scan timed out after %s", s.timeout))
			s.persistServerCookbookResult(ctx, sr)
			return sr
		}
		if stdout == "" {
			sr.Error = fmt.Errorf("execution failed (exit %d): %v; stderr: %s",
				exitCode, execErr, strings.TrimSpace(stderr))
			log.Error(fmt.Sprintf("execution failed: %v", sr.Error))
			s.persistServerCookbookResult(ctx, sr)
			return sr
		}
		// Non-zero exit with stdout present — fall through to parse JSON.
	}

	// Step 4b: handle CookStyle errors (exit code >= 2).
	// RuboCop/CookStyle exit codes: 0 = success, 1 = offences found,
	// 2 = error (bad .rubocop.yaml, Ruby exception, gem load failure).
	// Exit code 2 is NOT a compatibility verdict — do not parse as JSON.
	if exitCode >= 2 {
		errMsg := strings.TrimSpace(stderr)
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout)
		}
		sr.ErrorMessage = fmt.Sprintf("CookStyle error (exit %d): %s", exitCode, errMsg)
		sr.Error = fmt.Errorf("cookstyle error (exit %d): %s", exitCode, errMsg)
		log.Warn(fmt.Sprintf("cookstyle error (exit %d), result recorded as error: %s", exitCode, errMsg))
		s.persistServerCookbookResult(ctx, sr)
		return sr
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
		s.persistServerCookbookResult(ctx, sr)
		return sr
	}

	// Step 6: classify offenses.
	sr.OffenseCount = output.Summary.OffenseCount
	sr.Passed = true

	for _, file := range output.Files {
		for _, off := range file.Offenses {
			off.File = relativeCookstylePath(file.Path, cookbookDir)
			sr.Offenses = append(sr.Offenses, off)

			if isDeprecation(off.CopName) {
				sr.DeprecationCount++
				sr.DeprecationWarnings = append(sr.DeprecationWarnings, off)
			}
			if isCorrectness(off.CopName) {
				sr.CorrectnessCount++
			}
			if off.Correctable {
				sr.CorrectableCount++
			}
		}
	}

	// Step 6b: run custom cop scanning.
	customOffenses := s.runCustomCopScan(ctx, cookbookDir, log)
	for _, off := range customOffenses {
		sr.Offenses = append(sr.Offenses, off)
		sr.OffenseCount++
	}

	resolver := s.buildResolver(ctx, targetChefVersion)
	sr.CookstyleStatus = DeriveCookstyleStatus(sr.Offenses, resolver)
	sr.Passed = sr.CookstyleStatus != StatusBlocked

	// Step 7: log outcome.
	if sr.Passed {
		log.Info(fmt.Sprintf("passed: %d offense(s), %d deprecation(s), %d correctness, %d correctable, %d custom in %s",
			sr.OffenseCount, sr.DeprecationCount, sr.CorrectnessCount, sr.CorrectableCount, len(customOffenses),
			sr.Duration.Round(time.Millisecond)))
	} else {
		log.Warn(fmt.Sprintf("failed: %d offense(s), %d deprecation(s), %d correctness, %d correctable, %d custom in %s",
			sr.OffenseCount, sr.DeprecationCount, sr.CorrectnessCount, sr.CorrectableCount, len(customOffenses),
			sr.Duration.Round(time.Millisecond)))
	}

	// Step 8: persist.
	s.persistServerCookbookResult(ctx, sr)
	return sr
}

// ---------------------------------------------------------------------------
// Single git repo scan
// ---------------------------------------------------------------------------

func (s *CookstyleScanner) scanOneGitRepo(
	ctx context.Context,
	gr datastore.GitRepo,
	targetChefVersion string,
	repoDir string,
) CookstyleScanResult {
	log := s.logger.WithScope(logging.ScopeCookstyleScan,
		logging.WithCookbook(gr.Name, ""))

	sr := CookstyleScanResult{
		CookbookName:      gr.Name,
		GitRepoURL:        gr.GitRepoURL,
		TargetChefVersion: targetChefVersion,
		CommitSHA:         gr.HeadCommitSHA,
	}

	// Step 1: skip check.
	// Git repos change with each commit — skip only when the HEAD commit
	// SHA matches the previously scanned commit.
	existing, err := s.db.GetGitRepoCookstyleResult(ctx, gr.Name, gr.GitRepoURL, targetChefVersion)
	if err == nil && existing != nil {
		if existing.CommitSHA != "" && existing.CommitSHA == gr.HeadCommitSHA {
			shaPreview := gr.HeadCommitSHA
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

	// Step 2+3: execute with timeout, injecting operator addon cops and
	// isolating any addon load failure (a broken .rb must not error the scan).
	scanStart := time.Now()
	scanCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	stdout, stderr, exitCode, addonInfo, execErr := s.runScanWithAddonIsolation(scanCtx, repoDir, targetChefVersion)
	logAddonScanInfo(log, addonInfo)
	sr.Duration = time.Since(scanStart)
	sr.ScannedAt = time.Now().UTC()
	sr.RawStdout = stdout
	sr.RawStderr = stderr

	// Step 4: handle execution failures.
	if execErr != nil {
		if scanCtx.Err() == context.DeadlineExceeded {
			sr.Error = fmt.Errorf("timed out after %s", s.timeout)
			log.Error(fmt.Sprintf("scan timed out after %s", s.timeout))
			s.persistGitRepoResult(ctx, sr)
			return sr
		}
		if stdout == "" {
			sr.Error = fmt.Errorf("execution failed (exit %d): %v; stderr: %s",
				exitCode, execErr, strings.TrimSpace(stderr))
			log.Error(fmt.Sprintf("execution failed: %v", sr.Error))
			s.persistGitRepoResult(ctx, sr)
			return sr
		}
		// Non-zero exit with stdout present — fall through to parse JSON.
	}

	// Step 4b: handle CookStyle errors (exit code >= 2).
	// RuboCop/CookStyle exit codes: 0 = success, 1 = offences found,
	// 2 = error (bad .rubocop.yaml, Ruby exception, gem load failure).
	// Exit code 2 is NOT a compatibility verdict — do not parse as JSON.
	if exitCode >= 2 {
		errMsg := strings.TrimSpace(stderr)
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout)
		}
		sr.ErrorMessage = fmt.Sprintf("CookStyle error (exit %d): %s", exitCode, errMsg)
		sr.Error = fmt.Errorf("cookstyle error (exit %d): %s", exitCode, errMsg)
		log.Warn(fmt.Sprintf("cookstyle error (exit %d), result recorded as error: %s", exitCode, errMsg))
		s.persistGitRepoResult(ctx, sr)
		return sr
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
		s.persistGitRepoResult(ctx, sr)
		return sr
	}

	// Step 6: classify offenses.
	sr.OffenseCount = output.Summary.OffenseCount
	sr.Passed = true

	for _, file := range output.Files {
		for _, off := range file.Offenses {
			off.File = relativeCookstylePath(file.Path, repoDir)
			sr.Offenses = append(sr.Offenses, off)

			if isDeprecation(off.CopName) {
				sr.DeprecationCount++
				sr.DeprecationWarnings = append(sr.DeprecationWarnings, off)
			}
			if isCorrectness(off.CopName) {
				sr.CorrectnessCount++
			}
			if off.Correctable {
				sr.CorrectableCount++
			}
		}
	}

	// Step 6b: run custom cop scanning.
	customOffenses := s.runCustomCopScan(ctx, repoDir, log)
	for _, off := range customOffenses {
		sr.Offenses = append(sr.Offenses, off)
		sr.OffenseCount++
	}

	resolver := s.buildResolver(ctx, targetChefVersion)
	sr.CookstyleStatus = DeriveCookstyleStatus(sr.Offenses, resolver)
	sr.Passed = sr.CookstyleStatus != StatusBlocked

	// Step 7: log outcome.
	if sr.Passed {
		log.Info(fmt.Sprintf("passed: %d offense(s), %d deprecation(s), %d correctness, %d correctable, %d custom in %s",
			sr.OffenseCount, sr.DeprecationCount, sr.CorrectnessCount, sr.CorrectableCount, len(customOffenses),
			sr.Duration.Round(time.Millisecond)))
	} else {
		log.Warn(fmt.Sprintf("failed: %d offense(s), %d deprecation(s), %d correctness, %d correctable, %d custom in %s",
			sr.OffenseCount, sr.DeprecationCount, sr.CorrectnessCount, sr.CorrectableCount, len(customOffenses),
			sr.Duration.Round(time.Millisecond)))
	}

	// Step 8: persist.
	s.persistGitRepoResult(ctx, sr)
	return sr
}

// relativeCookstylePath strips the cookbook directory prefix from a CookStyle
// file path so that persisted offense locations show developer-friendly
// relative paths (e.g. "recipes/default.rb") instead of opaque absolute
// paths like "/tmp/cmm-cb-apache2-5.0.1-abc123/recipes/default.rb".
//
// CookStyle/RuboCop reports absolute paths when the input directory is
// absolute, which it always is (temp dirs for server cookbooks, persistent
// clones for git cookbooks). Neither path is meaningful to an end-user.
//
// If the path does not start with cookbookDir (shouldn't happen in
// practice), it is returned unchanged as a defensive fallback.
func relativeCookstylePath(filePath, cookbookDir string) string {
	if cookbookDir == "" {
		return filePath
	}
	// Ensure the prefix ends with a separator so we don't match partial
	// directory names (e.g. "/tmp/cb" matching "/tmp/cb-extra/file.rb").
	prefix := cookbookDir
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if strings.HasPrefix(filePath, prefix) {
		return filePath[len(prefix):]
	}
	return filePath
}

// ---------------------------------------------------------------------------
// Argument construction
// ---------------------------------------------------------------------------

// buildCookstyleArgs constructs the cookstyle CLI arguments for a scan via the
// shared remediation.BuildCookstyleArgs helper.
//
// We always pass --format json for machine-parseable output. When a target
// Chef Client version is specified, a sidecar .rubocop_cmm.yml carrying
// AllCops.TargetChefVersion is written and pointed at with --config.
//
// The scan runs the FULL ruleset (onlyDepartments = ""): classification — not a
// department filter — decides the rollup verdict and complexity. The old
// --only Chef/Deprecations,Chef/Correctness narrowing silently hid every
// Blocker-classified cop outside those two departments (e.g. the curated
// default Lint/DeprecatedClassMethods), so the classification could claim a
// block the scan would never produce.
func buildCookstyleArgs(cookbookDir string, targetChefVersion string) []string {
	return buildCookstyleArgsWithAddons(cookbookDir, targetChefVersion, nil)
}

// buildCookstyleArgsWithAddons is buildCookstyleArgs with operator addon cop
// requires injected into the sidecar (see remediation.ResolveAddonCopFiles).
// The scan still runs the full ruleset (no --only).
func buildCookstyleArgsWithAddons(cookbookDir string, targetChefVersion string, addonCops []remediation.AddonCop) []string {
	return remediation.BuildCookstyleArgs(
		cookbookDir,
		targetChefVersion,
		[]string{"--format", "json"},
		"",
		addonCops,
	)
}

// effectiveAddonRequires resolves the live operator addon cop path entries into
// concrete addon cops (file + parsed cop names). Returns the resolved cops plus
// any path problems (missing file, empty dir, bad glob, unparseable cop) for the
// caller to surface. With no provider or no configured paths it returns nothing.
func (s *CookstyleScanner) effectiveAddonRequires() ([]remediation.AddonCop, []remediation.AddonCopProblem) {
	if s.addonCopPathsFn == nil {
		return nil, nil
	}
	paths := s.addonCopPathsFn()
	if len(paths) == 0 {
		return nil, nil
	}
	return remediation.ResolveAddonCopFiles(paths)
}

// addonScanInfo reports what happened to addon cops during a scan, so the
// caller can surface it (log) without the helper needing a logger.
type addonScanInfo struct {
	// problems are configured addon paths that could not be resolved.
	problems []remediation.AddonCopProblem
	// requires are the resolved addon cops that were injected.
	requires []remediation.AddonCop
	// loadFailed is true when cookstyle errored WITH addon cops loaded but
	// succeeded without them — i.e. a broken addon cop was isolated out and the
	// returned result comes from the clean (addon-free) retry.
	loadFailed bool
	// addonExit / cleanExit are the cookstyle exit codes from the addon and the
	// isolation-retry runs (only meaningful when loadFailed is true).
	addonExit int
	cleanExit int
}

// runScanWithAddonIsolation runs cookstyle for a scan with operator addon cops
// injected, isolating addon load failures: if cookstyle errors (exit >= 2) with
// addons loaded, it retries WITHOUT them. When the clean retry succeeds, that
// result is returned so a single broken addon .rb never marks every cookbook as
// errored — the failure is reported via the returned addonScanInfo instead.
func (s *CookstyleScanner) runScanWithAddonIsolation(
	scanCtx context.Context,
	cookbookDir, targetChefVersion string,
) (stdout, stderr string, exitCode int, info addonScanInfo, execErr error) {
	addonCops, problems := s.effectiveAddonRequires()
	info.problems = problems
	info.requires = addonCops

	args := buildCookstyleArgsWithAddons(cookbookDir, targetChefVersion, addonCops)
	stdout, stderr, exitCode, execErr = s.executor.Run(scanCtx, cookbookDir, args...)

	// Only attempt isolation when addons were actually injected and cookstyle
	// ran to completion with an error exit (a start/timeout failure is not an
	// addon problem and must surface as-is).
	if len(addonCops) > 0 && execErr == nil && exitCode >= 2 {
		cleanArgs := buildCookstyleArgs(cookbookDir, targetChefVersion)
		cStdout, cStderr, cExit, cErr := s.executor.Run(scanCtx, cookbookDir, cleanArgs...)
		if cErr == nil && cExit < 2 {
			info.loadFailed = true
			info.addonExit = exitCode
			info.cleanExit = cExit
			return cStdout, cStderr, cExit, info, cErr
		}
		// The clean run also errored — a genuine cookbook/cookstyle error, not an
		// addon load failure. Fall through with the original result.
	}
	return stdout, stderr, exitCode, info, execErr
}

// logAddonScanInfo surfaces addon path problems and load failures to the log.
func logAddonScanInfo(log *logging.ScopedLogger, info addonScanInfo) {
	for _, p := range info.problems {
		log.Warn(fmt.Sprintf("addon cop path %q could not be resolved: %s", p.Path, p.Reason))
	}
	if info.loadFailed {
		log.Error(fmt.Sprintf(
			"an addon cop failed to load (cookstyle exit %d with addons, %d without); cookbook scanned WITHOUT addon cops — verify these files: %v",
			info.addonExit, info.cleanExit, remediation.AddonCopPaths(info.requires)))
	}
}

// ---------------------------------------------------------------------------
// Custom cop scanning
// ---------------------------------------------------------------------------

// runCustomCopScan loads enabled custom cop definitions from the database and
// runs pattern matching against cookbook source files.
func (s *CookstyleScanner) runCustomCopScan(ctx context.Context, cookbookDir string, log *logging.ScopedLogger) []CookstyleOffense {
	defs, err := s.db.ListEnabledCustomCopDefinitions(ctx)
	if err != nil {
		log.Warn(fmt.Sprintf("failed to load custom cop definitions: %v", err))
		return nil
	}
	if len(defs) == 0 {
		return nil
	}

	offenses := ScanCustomCops(cookbookDir, defs)
	if len(offenses) > 0 {
		log.Debug(fmt.Sprintf("custom cops: %d offense(s) from %d definition(s)", len(offenses), len(defs)))
	}
	return offenses
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
			CopName:     off.CopName,
			Severity:    off.Severity,
			Message:     off.Message,
			Correctable: off.Correctable,
			Location: remediation.OffenseLocation{
				File:        off.File,
				StartLine:   off.Location.StartLine,
				StartColumn: off.Location.StartColumn,
				LastLine:    off.Location.LastLine,
				LastColumn:  off.Location.LastColumn,
			},
			Remediation: remediation.LookupCopForOffense(off.CopName, off.Message),
		}
	}
	return enriched
}

func (s *CookstyleScanner) persistServerCookbookResult(ctx context.Context, sr CookstyleScanResult) {
	if sr.CookbookName == "" {
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

	params := datastore.UpsertServerCookbookCookstyleResultParams{
		OrganisationName:    sr.OrganisationName,
		CookbookName:        sr.CookbookName,
		CookbookVersion:     sr.CookbookVersion,
		TargetChefVersion:   sr.TargetChefVersion,
		Passed:              sr.Passed,
		CookstyleStatus:     sr.CookstyleStatus,
		OffenceCount:        sr.OffenseCount,
		DeprecationCount:    sr.DeprecationCount,
		CorrectnessCount:    sr.CorrectnessCount,
		DeprecationWarnings: deprecationsJSON,
		Offences:            offensesJSON,
		ProcessStdout:       sr.RawStdout,
		ProcessStderr:       sr.RawStderr,
		DurationSeconds:     int(sr.Duration.Seconds()),
		ScannedAt:           sr.ScannedAt,
		ErrorMessage:        sr.ErrorMessage,
	}

	if _, persistErr := s.db.UpsertServerCookbookCookstyleResult(ctx, params); persistErr != nil {
		log.Error(fmt.Sprintf("failed to persist server cookbook result: %v", persistErr))
		return
	}

	// Append a change-deduped offence fingerprint so this scan's status/complexity
	// can be recomputed under future classification criteria. Skip errored scans:
	// they have no offences, and recording an empty fingerprint would falsely read
	// as "clean". See journeys/estate-progress.md.
	if sr.ErrorMessage == "" {
		entries, hash := BuildOffenceFingerprint(sr.Offenses)
		if _, fpErr := s.db.AppendCookstyleOffenceFingerprint(ctx, datastore.AppendCookstyleOffenceFingerprintParams{
			ResultKind:        datastore.FingerprintKindServerCookbook,
			OrganisationName:  sr.OrganisationName,
			CookbookName:      sr.CookbookName,
			CookbookVersion:   sr.CookbookVersion,
			TargetChefVersion: sr.TargetChefVersion,
			FingerprintHash:   hash,
			Cops:              entries,
			ScannedAt:         sr.ScannedAt,
		}); fpErr != nil {
			log.Warn(fmt.Sprintf("failed to append offence fingerprint: %v", fpErr))
		}
	}
}

func (s *CookstyleScanner) persistGitRepoResult(ctx context.Context, sr CookstyleScanResult) {
	if sr.CookbookName == "" {
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

	params := datastore.UpsertGitRepoCookstyleResultParams{
		GitRepoName:         sr.CookbookName,
		GitRepoURL:          sr.GitRepoURL,
		TargetChefVersion:   sr.TargetChefVersion,
		CommitSHA:           sr.CommitSHA,
		Passed:              sr.Passed,
		CookstyleStatus:     sr.CookstyleStatus,
		OffenceCount:        sr.OffenseCount,
		DeprecationCount:    sr.DeprecationCount,
		CorrectnessCount:    sr.CorrectnessCount,
		DeprecationWarnings: deprecationsJSON,
		Offences:            offensesJSON,
		ProcessStdout:       sr.RawStdout,
		ProcessStderr:       sr.RawStderr,
		DurationSeconds:     int(sr.Duration.Seconds()),
		ScannedAt:           sr.ScannedAt,
		ErrorMessage:        sr.ErrorMessage,
	}

	if _, persistErr := s.db.UpsertGitRepoCookstyleResult(ctx, params); persistErr != nil {
		log.Error(fmt.Sprintf("failed to persist git repo result: %v", persistErr))
		return
	}

	// Append a change-deduped offence fingerprint (see persistServerCookbookResult).
	if sr.ErrorMessage == "" {
		entries, hash := BuildOffenceFingerprint(sr.Offenses)
		if _, fpErr := s.db.AppendCookstyleOffenceFingerprint(ctx, datastore.AppendCookstyleOffenceFingerprintParams{
			ResultKind:        datastore.FingerprintKindGitRepo,
			GitRepoName:       sr.CookbookName,
			GitRepoURL:        sr.GitRepoURL,
			TargetChefVersion: sr.TargetChefVersion,
			FingerprintHash:   hash,
			Cops:              entries,
			ScannedAt:         sr.ScannedAt,
		}); fpErr != nil {
			log.Warn(fmt.Sprintf("failed to append offence fingerprint: %v", fpErr))
		}
	}
}

// ---------------------------------------------------------------------------
// Manual rescan
// ---------------------------------------------------------------------------

// ResetServerCookbookResults deletes existing CookStyle results for the
// given server cookbook, so they will be rescanned on the next analysis cycle.
func (s *CookstyleScanner) ResetServerCookbookResults(ctx context.Context, orgName, cookbookName, cookbookVersion string) error {
	return s.db.DeleteServerCookbookCookstyleResultsByCookbook(ctx, orgName, cookbookName, cookbookVersion)
}

// ResetServerCookbookResultsByOrganisation deletes all CookStyle results for
// server cookbooks belonging to the given organisation.
func (s *CookstyleScanner) ResetServerCookbookResultsByOrganisation(ctx context.Context, organisationID string) error {
	return s.db.DeleteServerCookbookCookstyleResultsByOrganisation(ctx, organisationID)
}

// ResetGitRepoResults deletes existing CookStyle results for the given git
// repo, so they will be rescanned on the next analysis cycle.
func (s *CookstyleScanner) ResetGitRepoResults(ctx context.Context, gitRepoName, gitRepoURL string) error {
	return s.db.DeleteGitRepoCookstyleResultsByRepo(ctx, gitRepoName, gitRepoURL)
}

// ---------------------------------------------------------------------------
// Default executor
// ---------------------------------------------------------------------------

// NewCookstyleExecutor returns a CookstyleExecutor that runs the cookstyle
// binary at the given path. It lets callers (e.g. the cop-registry provider
// wired in main) reuse the same execution path as the scanner without
// constructing a full scanner.
func NewCookstyleExecutor(path string) CookstyleExecutor {
	return &defaultCookstyleExecutor{path: path}
}

type defaultCookstyleExecutor struct {
	path string
}

func (e *defaultCookstyleExecutor) Run(ctx context.Context, dir string, args ...string) (string, string, int, error) {
	return executeCommand(ctx, e.path, dir, args...)
}

// executeCommand runs an external command and returns stdout, stderr, exit
// code, and error. A non-zero exit code from a process that ran to
// completion is NOT returned as an error — the caller inspects the exit
// code and stdout/stderr separately.
//
// dir, when non-empty, is set as the process working directory. This anchors
// RuboCop's CWD-relative offence path reporting to the scanned cookbook (see
// the CookstyleExecutor.Run doc comment).
func executeCommand(ctx context.Context, name, dir string, args ...string) (string, string, int, error) {
	cmd := makeCommand(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}

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
