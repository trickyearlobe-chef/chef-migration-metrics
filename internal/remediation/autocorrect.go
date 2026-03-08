// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// ---------------------------------------------------------------------------
// Executor interface
// ---------------------------------------------------------------------------

// AutocorrectExecutor abstracts the execution of the cookstyle binary with
// --auto-correct so that tests can inject a fake without touching the
// filesystem.
type AutocorrectExecutor interface {
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
// Result types
// ---------------------------------------------------------------------------

// AutocorrectPreviewResult holds the outcome of generating an auto-correct
// preview for a single cookbook version.
type AutocorrectPreviewResult struct {
	// CookbookID is the datastore ID of the cookbook.
	CookbookID string

	// CookstyleResultID is the datastore ID of the CookStyle scan result
	// that triggered this preview.
	CookstyleResultID string

	// CookbookName is the cookbook's display name.
	CookbookName string

	// CookbookVersion is the version string.
	CookbookVersion string

	// TotalOffenses is the total number of offenses before auto-correct.
	TotalOffenses int

	// CorrectableOffenses is the number of offenses auto-correct can fix.
	CorrectableOffenses int

	// RemainingOffenses is the number of offenses that require manual fix.
	RemainingOffenses int

	// FilesModified is the number of files changed by auto-correct.
	FilesModified int

	// DiffOutput is the unified diff of all changes.
	DiffOutput string

	// GeneratedAt is the UTC timestamp when the preview was generated.
	GeneratedAt time.Time

	// Skipped is true when the preview was skipped (e.g. because an
	// existing preview was found, or the cookbook had zero offenses).
	Skipped bool

	// SkipReason describes why the preview was skipped.
	SkipReason string

	// Error is non-nil when the preview generation itself failed.
	Error error
}

// AutocorrectBatchResult summarises the outcome of generating auto-correct
// previews for a batch of cookbooks.
type AutocorrectBatchResult struct {
	Total     int
	Generated int
	Skipped   int
	Errors    int
	Duration  time.Duration
	Results   []AutocorrectPreviewResult
}

// ---------------------------------------------------------------------------
// Preview generator
// ---------------------------------------------------------------------------

// AutocorrectGenerator generates auto-correct previews by running
// cookstyle --auto-correct on temporary copies of cookbook directories
// and computing unified diffs.
type AutocorrectGenerator struct {
	db            *datastore.DB
	logger        *logging.Logger
	executor      AutocorrectExecutor
	timeout       time.Duration
	cookstylePath string
}

// AutocorrectGeneratorOption configures an AutocorrectGenerator.
type AutocorrectGeneratorOption func(*AutocorrectGenerator)

// WithAutocorrectExecutor overrides the command executor (for testing).
func WithAutocorrectExecutor(e AutocorrectExecutor) AutocorrectGeneratorOption {
	return func(g *AutocorrectGenerator) { g.executor = e }
}

// NewAutocorrectGenerator creates a new preview generator.
//
// Parameters:
//   - db: datastore for checking existing previews and persisting new ones
//   - logger: structured logger
//   - cookstylePath: resolved absolute path to the cookstyle binary
//   - timeoutMinutes: per-preview timeout
//   - opts: optional overrides
func NewAutocorrectGenerator(
	db *datastore.DB,
	logger *logging.Logger,
	cookstylePath string,
	timeoutMinutes int,
	opts ...AutocorrectGeneratorOption,
) *AutocorrectGenerator {
	if timeoutMinutes <= 0 {
		timeoutMinutes = 10
	}

	g := &AutocorrectGenerator{
		db:            db,
		logger:        logger,
		cookstylePath: cookstylePath,
		timeout:       time.Duration(timeoutMinutes) * time.Minute,
	}
	for _, o := range opts {
		o(g)
	}
	if g.executor == nil {
		g.executor = &defaultAutocorrectExecutor{path: cookstylePath}
	}
	return g
}

// ---------------------------------------------------------------------------
// CookstyleResultInfo carries the data needed to generate a preview
// without requiring a full CookstyleResult from the datastore.
// ---------------------------------------------------------------------------

// CookstyleResultInfo carries the minimal information about a CookStyle
// scan result needed to decide whether to generate a preview and to
// associate the preview with the result.
type CookstyleResultInfo struct {
	// ResultID is the datastore ID (primary key) of the cookstyle_results row.
	ResultID string

	// CookbookID is the datastore ID of the cookbook.
	CookbookID string

	// CookbookName is the cookbook's display name (for logging).
	CookbookName string

	// CookbookVersion is the version string (for logging).
	CookbookVersion string

	// TargetChefVersion is the Chef Client version the scan targeted.
	TargetChefVersion string

	// OffenseCount is the total number of offenses from the scan.
	OffenseCount int

	// Passed is true when there are zero error/fatal offenses.
	Passed bool
}

// GeneratePreviews generates auto-correct previews for all provided
// CookStyle results that have offenses. It runs sequentially — the caller
// is responsible for concurrency control (the spec says previews share
// the cookstyle_scan worker pool).
//
// cookbookDir maps a cookbook ID to its filesystem path.
func (g *AutocorrectGenerator) GeneratePreviews(
	ctx context.Context,
	results []CookstyleResultInfo,
	cookbookDir func(cookbookID string) string,
) AutocorrectBatchResult {
	start := time.Now()
	log := g.logger.WithScope(logging.ScopeRemediation)

	batch := AutocorrectBatchResult{
		Total:   len(results),
		Results: make([]AutocorrectPreviewResult, 0, len(results)),
	}

	for _, csResult := range results {
		if ctx.Err() != nil {
			break
		}

		pr := g.generateOne(ctx, csResult, cookbookDir)
		batch.Results = append(batch.Results, pr)

		switch {
		case pr.Skipped:
			batch.Skipped++
		case pr.Error != nil:
			batch.Errors++
			log.Error(fmt.Sprintf("autocorrect preview error: %s/%s: %v",
				pr.CookbookName, pr.CookbookVersion, pr.Error))
		default:
			batch.Generated++
		}
	}

	batch.Duration = time.Since(start)
	log.Info(fmt.Sprintf(
		"autocorrect previews complete: %d total, %d generated, %d skipped, %d errors in %s",
		batch.Total, batch.Generated, batch.Skipped, batch.Errors,
		batch.Duration.Round(time.Millisecond)))
	return batch
}

// generateOne generates a single auto-correct preview for one CookStyle
// result.
func (g *AutocorrectGenerator) generateOne(
	ctx context.Context,
	csResult CookstyleResultInfo,
	cookbookDir func(cookbookID string) string,
) AutocorrectPreviewResult {
	log := g.logger.WithScope(logging.ScopeRemediation,
		logging.WithCookbook(csResult.CookbookName, csResult.CookbookVersion))

	pr := AutocorrectPreviewResult{
		CookbookID:        csResult.CookbookID,
		CookstyleResultID: csResult.ResultID,
		CookbookName:      csResult.CookbookName,
		CookbookVersion:   csResult.CookbookVersion,
	}

	// Step 1: skip if zero offenses — no auto-correct needed.
	if csResult.OffenseCount == 0 {
		pr.Skipped = true
		pr.SkipReason = "zero offenses"
		log.Debug("skipping autocorrect preview — zero offenses")
		return pr
	}

	// Step 2: check for existing preview (immutability cache for server cookbooks).
	existing, err := g.db.GetAutocorrectPreview(ctx, csResult.ResultID)
	if err == nil && existing != nil {
		pr.Skipped = true
		pr.SkipReason = fmt.Sprintf("existing preview from %s",
			existing.GeneratedAt.Format(time.RFC3339))
		log.Debug(fmt.Sprintf("skipping — existing preview from %s",
			existing.GeneratedAt.Format(time.RFC3339)))
		return pr
	}

	// Step 3: resolve cookbook directory.
	dir := cookbookDir(csResult.CookbookID)
	if dir == "" {
		pr.Error = fmt.Errorf("cookbook directory not found for %s", csResult.CookbookID)
		return pr
	}

	// Step 4: create temporary copy.
	tmpDir, copyErr := copyDirectory(dir)
	if copyErr != nil {
		pr.Error = fmt.Errorf("creating temporary copy: %w", copyErr)
		return pr
	}
	defer os.RemoveAll(tmpDir)

	// Step 5: read original file contents before auto-correct.
	originals, readErr := readAllFiles(tmpDir)
	if readErr != nil {
		pr.Error = fmt.Errorf("reading original files: %w", readErr)
		return pr
	}

	// Step 6: run cookstyle --auto-correct on the copy.
	genCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	args := buildAutocorrectArgs(tmpDir, csResult.TargetChefVersion)
	stdout, _, _, execErr := g.executor.Run(genCtx, args...)

	if execErr != nil {
		if genCtx.Err() == context.DeadlineExceeded {
			pr.Error = fmt.Errorf("timed out after %s", g.timeout)
			return pr
		}
		// CookStyle may fail completely (no stdout) — treat as error.
		if stdout == "" {
			pr.Error = fmt.Errorf("cookstyle --auto-correct failed: %v", execErr)
			return pr
		}
		// Non-zero exit with stdout — fall through to parse.
	}

	// Step 7: parse the JSON output to get offense counts after auto-correct.
	var afterOutput autocorrectJSONOutput
	if parseErr := json.Unmarshal([]byte(stdout), &afterOutput); parseErr != nil {
		pr.Error = fmt.Errorf("parsing auto-correct JSON output: %w", parseErr)
		return pr
	}

	// Step 8: read modified files and generate diffs.
	modified, modReadErr := readAllFiles(tmpDir)
	if modReadErr != nil {
		pr.Error = fmt.Errorf("reading modified files: %w", modReadErr)
		return pr
	}

	diffOutput, filesChanged := generateUnifiedDiffs(originals, modified)

	// Step 9: compute statistics.
	// CookStyle's --auto-correct fixes offenses in-place. The remaining
	// offenses in the JSON output are the ones it couldn't fix.
	remainingAfterCorrect := afterOutput.Summary.OffenseCount
	correctableCount := csResult.OffenseCount - remainingAfterCorrect
	if correctableCount < 0 {
		correctableCount = 0
	}

	pr.TotalOffenses = csResult.OffenseCount
	pr.CorrectableOffenses = correctableCount
	pr.RemainingOffenses = remainingAfterCorrect
	pr.FilesModified = filesChanged
	pr.DiffOutput = diffOutput
	pr.GeneratedAt = time.Now().UTC()

	// Step 10: persist.
	g.persistPreview(ctx, pr)

	log.Info(fmt.Sprintf(
		"autocorrect preview: %d total, %d correctable, %d remaining, %d files modified",
		pr.TotalOffenses, pr.CorrectableOffenses, pr.RemainingOffenses, pr.FilesModified))

	return pr
}

// ---------------------------------------------------------------------------
// Argument construction
// ---------------------------------------------------------------------------

// buildAutocorrectArgs constructs the cookstyle CLI arguments for an
// auto-correct run. We always use --auto-correct (which modifies files
// in-place) and --format json to get machine-parseable output about
// remaining offenses.
func buildAutocorrectArgs(cookbookDir string, targetChefVersion string) []string {
	args := []string{"--auto-correct", "--format", "json"}

	if targetChefVersion != "" {
		// Set TargetChefVersion via a sidecar .rubocop_cmm.yml that we
		// point CookStyle at with --config. If the cookbook already has a
		// .rubocop.yml we inherit from it so its settings are preserved.
		// CookStyle does not accept a --target-chef-version CLI flag.
		configPath := writeAutocorrectTargetConfig(cookbookDir, targetChefVersion)
		if configPath != "" {
			args = append(args, "--config", configPath)
		}

		args = append(args, "--only", "Chef/Deprecations,Chef/Correctness")
	}

	args = append(args, cookbookDir)
	return args
}

// cmmConfigName is the sidecar config file written next to the cookbook's
// own .rubocop.yml (if any). Using a distinct name avoids overwriting the
// cookbook's configuration.
const cmmConfigName = ".rubocop_cmm.yml"

// writeAutocorrectTargetConfig writes a sidecar .rubocop_cmm.yml into the
// cookbook directory that sets AllCops.TargetChefVersion. If the cookbook
// already contains a .rubocop.yml the sidecar inherits from it so the
// cookbook's own configuration (excludes, custom cops, etc.) is preserved.
// When no cookbook config exists the sidecar explicitly requires cookstyle
// so that the TargetChefVersion parameter is recognised.
//
// Returns the absolute path to the written file, or "" on failure.
func writeAutocorrectTargetConfig(cookbookDir, targetChefVersion string) string {
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
// Persistence
// ---------------------------------------------------------------------------

func (g *AutocorrectGenerator) persistPreview(ctx context.Context, pr AutocorrectPreviewResult) {
	if pr.CookbookID == "" || pr.CookstyleResultID == "" {
		return
	}

	log := g.logger.WithScope(logging.ScopeRemediation,
		logging.WithCookbook(pr.CookbookName, pr.CookbookVersion))

	params := datastore.UpsertAutocorrectPreviewParams{
		CookbookID:          pr.CookbookID,
		CookstyleResultID:   pr.CookstyleResultID,
		TotalOffenses:       pr.TotalOffenses,
		CorrectableOffenses: pr.CorrectableOffenses,
		RemainingOffenses:   pr.RemainingOffenses,
		FilesModified:       pr.FilesModified,
		DiffOutput:          pr.DiffOutput,
		GeneratedAt:         pr.GeneratedAt,
	}

	if _, persistErr := g.db.UpsertAutocorrectPreview(ctx, params); persistErr != nil {
		log.Error(fmt.Sprintf("failed to persist autocorrect preview: %v", persistErr))
	}
}

// ---------------------------------------------------------------------------
// Reset
// ---------------------------------------------------------------------------

// ResetPreviews deletes existing auto-correct previews for the given
// cookbook, so they will be regenerated on the next analysis cycle.
func (g *AutocorrectGenerator) ResetPreviews(ctx context.Context, cookbookID string) error {
	return g.db.DeleteAutocorrectPreviewsForCookbook(ctx, cookbookID)
}

// ResetAllPreviews deletes all auto-correct previews for the given
// organisation.
func (g *AutocorrectGenerator) ResetAllPreviews(ctx context.Context, organisationID string) error {
	return g.db.DeleteAutocorrectPreviewsForOrganisation(ctx, organisationID)
}

// ---------------------------------------------------------------------------
// JSON parsing types (auto-correct output)
// ---------------------------------------------------------------------------

// autocorrectJSONOutput is the minimal subset of CookStyle JSON output
// needed to extract offense counts after auto-correct.
type autocorrectJSONOutput struct {
	Summary autocorrectSummary `json:"summary"`
}

type autocorrectSummary struct {
	OffenseCount       int `json:"offense_count"`
	TargetFileCount    int `json:"target_file_count"`
	InspectedFileCount int `json:"inspected_file_count"`
}

// ---------------------------------------------------------------------------
// Directory copy helpers
// ---------------------------------------------------------------------------

// copyDirectory creates a temporary copy of the source directory and
// returns the path to the temporary directory. The caller is responsible
// for removing it.
func copyDirectory(src string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "autocorrect-preview-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}

		destPath := filepath.Join(tmpDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}

		// Skip non-regular files (symlinks, etc.) to keep it simple and safe.
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		return copyFile(path, destPath, info.Mode())
	})

	if walkErr != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("walking source directory: %w", walkErr)
	}

	return tmpDir, nil
}

// copyFile copies a single file from src to dst preserving the given mode.
func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// ---------------------------------------------------------------------------
// File reading and diff generation
// ---------------------------------------------------------------------------

// fileContent represents the content of a single file for diffing.
type fileContent struct {
	RelPath string
	Content string
}

// readAllFiles reads all regular files under dir and returns them keyed by
// relative path.
func readAllFiles(dir string) (map[string]string, error) {
	files := make(map[string]string)

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		relPath, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		files[relPath] = string(content)
		return nil
	})

	return files, err
}

// generateUnifiedDiffs compares the original and modified file maps and
// generates a unified diff string. Returns the combined diff output and
// the number of files that were modified.
func generateUnifiedDiffs(originals, modified map[string]string) (string, int) {
	var buf bytes.Buffer
	filesChanged := 0

	// Collect all paths from both maps to handle additions/deletions.
	allPaths := make(map[string]bool)
	for p := range originals {
		allPaths[p] = true
	}
	for p := range modified {
		allPaths[p] = true
	}

	// Sort paths for deterministic output.
	sortedPaths := make([]string, 0, len(allPaths))
	for p := range allPaths {
		sortedPaths = append(sortedPaths, p)
	}
	sortStringSlice(sortedPaths)

	for _, relPath := range sortedPaths {
		origContent := originals[relPath]
		modContent := modified[relPath]

		if origContent == modContent {
			continue
		}

		filesChanged++

		origLines := splitLines(origContent)
		modLines := splitLines(modContent)

		diff := computeUnifiedDiff(relPath, origLines, modLines)
		buf.WriteString(diff)
	}

	return buf.String(), filesChanged
}

// splitLines splits content into lines, preserving the line endings for
// accurate diff output.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	// SplitAfter may produce a trailing empty string if the input ends
	// with a newline — remove it for cleaner diffs.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// computeUnifiedDiff generates a minimal unified diff between two sets of
// lines using a simple O(mn) LCS-based diff algorithm. This is adequate
// for cookbook files (typically a few hundred lines at most).
func computeUnifiedDiff(path string, a, b []string) string {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("--- a/%s\n", path))
	buf.WriteString(fmt.Sprintf("+++ b/%s\n", path))

	// Compute the edit script using the LCS matrix.
	edits := computeEdits(a, b)

	// Group edits into hunks with context lines.
	const contextLines = 3
	hunks := groupHunks(edits, len(a), len(b), contextLines)

	for _, hunk := range hunks {
		buf.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
			hunk.origStart+1, hunk.origCount,
			hunk.newStart+1, hunk.newCount))

		for _, line := range hunk.lines {
			buf.WriteString(line)
			// Ensure lines end with a newline for valid diff format.
			if !strings.HasSuffix(line, "\n") {
				buf.WriteString("\n\\ No newline at end of file\n")
			}
		}
	}

	return buf.String()
}

// editOp represents a single edit operation.
type editOp int

const (
	editEqual  editOp = iota // line unchanged
	editDelete               // line removed from original
	editInsert               // line added in modified
)

type edit struct {
	op   editOp
	line string
}

// computeEdits computes the sequence of edit operations to transform a
// into b using the LCS (Longest Common Subsequence) algorithm.
func computeEdits(a, b []string) []edit {
	m := len(a)
	n := len(b)

	// Build the LCS length table.
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if stripNewline(a[i]) == stripNewline(b[j]) {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	// Backtrack to produce the edit sequence.
	var edits []edit
	i, j := 0, 0
	for i < m && j < n {
		if stripNewline(a[i]) == stripNewline(b[j]) {
			edits = append(edits, edit{op: editEqual, line: b[j]})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			edits = append(edits, edit{op: editDelete, line: a[i]})
			i++
		} else {
			edits = append(edits, edit{op: editInsert, line: b[j]})
			j++
		}
	}
	for ; i < m; i++ {
		edits = append(edits, edit{op: editDelete, line: a[i]})
	}
	for ; j < n; j++ {
		edits = append(edits, edit{op: editInsert, line: b[j]})
	}

	return edits
}

// stripNewline removes the trailing newline from a line for comparison
// purposes. This prevents false mismatches due to differing line endings.
func stripNewline(s string) string {
	return strings.TrimRight(s, "\r\n")
}

// hunk represents a single diff hunk with context.
type hunk struct {
	origStart int
	origCount int
	newStart  int
	newCount  int
	lines     []string
}

// groupHunks converts a flat edit sequence into unified diff hunks with
// the specified number of context lines around each change.
func groupHunks(edits []edit, origLen, newLen, ctx int) []hunk {
	if len(edits) == 0 {
		return nil
	}

	// Find change positions (indices into the edits slice where op != equal).
	var changePositions []int
	for i, e := range edits {
		if e.op != editEqual {
			changePositions = append(changePositions, i)
		}
	}
	if len(changePositions) == 0 {
		return nil
	}

	// Group changes that are close enough to share context.
	type changeGroup struct {
		start, end int // indices into edits (inclusive start, exclusive end)
	}
	var groups []changeGroup

	groupStart := changePositions[0]
	groupEnd := changePositions[0] + 1
	for _, pos := range changePositions[1:] {
		// If this change is within 2*ctx lines of the previous group end,
		// merge them into the same hunk.
		if pos-groupEnd <= 2*ctx {
			groupEnd = pos + 1
		} else {
			groups = append(groups, changeGroup{groupStart, groupEnd})
			groupStart = pos
			groupEnd = pos + 1
		}
	}
	groups = append(groups, changeGroup{groupStart, groupEnd})

	// Convert each group into a hunk by adding context lines.
	var hunks []hunk
	for _, g := range groups {
		// Determine the range of edits to include (with context).
		hunkStart := g.start - ctx
		if hunkStart < 0 {
			hunkStart = 0
		}
		hunkEnd := g.end + ctx
		if hunkEnd > len(edits) {
			hunkEnd = len(edits)
		}

		// Compute line numbers and build the hunk lines.
		var h hunk
		var hLines []string

		// Count original and new line numbers up to hunkStart.
		origLine := 0
		newLine := 0
		for i := 0; i < hunkStart; i++ {
			switch edits[i].op {
			case editEqual:
				origLine++
				newLine++
			case editDelete:
				origLine++
			case editInsert:
				newLine++
			}
		}

		h.origStart = origLine
		h.newStart = newLine

		origCount := 0
		newCount := 0

		for i := hunkStart; i < hunkEnd; i++ {
			e := edits[i]
			switch e.op {
			case editEqual:
				hLines = append(hLines, " "+e.line)
				origCount++
				newCount++
			case editDelete:
				hLines = append(hLines, "-"+e.line)
				origCount++
			case editInsert:
				hLines = append(hLines, "+"+e.line)
				newCount++
			}
		}

		h.origCount = origCount
		h.newCount = newCount
		h.lines = hLines
		hunks = append(hunks, h)
	}

	return hunks
}

// sortStringSlice sorts a string slice in place using a simple insertion
// sort. We avoid importing "sort" for this trivial case.
func sortStringSlice(s []string) {
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j] > key {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
}

// ---------------------------------------------------------------------------
// Default executor
// ---------------------------------------------------------------------------

type defaultAutocorrectExecutor struct {
	path string
}

func (e *defaultAutocorrectExecutor) Run(ctx context.Context, args ...string) (string, string, int, error) {
	return executeAutocorrectCommand(ctx, e.path, args...)
}

// executeAutocorrectCommand runs an external command and returns stdout,
// stderr, exit code, and error. A non-zero exit code from a process that
// ran to completion is NOT returned as an error.
func executeAutocorrectCommand(ctx context.Context, name string, args ...string) (string, string, int, error) {
	cmd := makeAutocorrectCommand(ctx, name, args...)

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

// makeAutocorrectCommand creates an *exec.Cmd for the given program and
// arguments. This is a thin wrapper around exec.CommandContext, mirroring
// the pattern used by the analysis package's makeCommand helper.
func makeAutocorrectCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
