// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"path"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Scan scope — the repository is not the cookbook. See journeys/scan-trust.md.
//
// A CookStyle scan covers the whole cloned repository, but a repository holds
// more than the cookbook: the pipeline definition, the helper tasks somebody
// wrote to run the tests, the test suites themselves. None of those execute on
// a machine during a converge, so a finding in one cannot decide whether the
// cookbook survives the upgrade.
//
// Two tempting shortcuts are both wrong and neither is implemented here:
//
//   - Judging by what Chef's upload ships. It uploads very nearly everything —
//     Rakefile, Jenkinsfile, Gemfile, spec/, test/ and arbitrary directories
//     all reach the server. The only automatic exclusion is a top-level
//     dot-directory.
//   - Inferring the set of files a converge could reach. Code can load code, so
//     any allowlist quietly discards whatever nobody thought of — the direction
//     that hides a real blocker.
//
// What is implemented is the third option: an explicit, curated list of files
// we assert do not run, each with a recorded reason, small enough to be argued
// with. A repository's own chefignore is deliberately NOT read — it is
// frequently wrong in ways nobody notices, and reading it would import somebody
// else's mistake and present it as our verdict.
//
// Exclusion is never deletion. An excluded finding stays on the cookbook as
// non-blocking work and is counted across the estate, because how widespread it
// is the most useful thing about it.

// ScanScopeExclusion is one curated assertion that a file does not execute
// during a converge. The reason is not documentation — it is the thing that
// makes the assertion checkable, the same discipline that applies to calling a
// finding harmless. Without it this list becomes the mechanism by which the
// blocked list is made to look good.
type ScanScopeExclusion struct {
	// Pattern is matched against the repo-relative path of the offending file.
	// A pattern ending in "/*" covers that directory and everything beneath it
	// at any depth; any other pattern is a shell glob anchored at the
	// repository root.
	Pattern string

	// Reason states why this file cannot run on a converging node.
	Reason string
}

// DefaultScanScopeExclusions is the seed list. Most entries are paths Chef's
// own cookbook generator ships or ignores, which is where the provenance comes
// from: these are files Chef itself treats as developer tooling rather than
// cookbook content. The Test Kitchen dotfile spelling and the pipeline
// definition are added on top, both being files no converge can reach.
//
// It is deliberately short, and being short is not the same as being enough.
// What this list CANNOT reach is an ordinary script that only ever runs because
// a build job invokes it: it can live at any path, under any name, and nothing
// in the file itself says so. That is the same "code can load code" problem the
// journey names, arriving from the CI system rather than from Chef, and no
// curated filename list solves it. Such a file is excluded by an operator who
// knows what runs it — which is why the list has to be editable, not merely
// well chosen.
func DefaultScanScopeExclusions() []ScanScopeExclusion {
	return []ScanScopeExclusion{
		{
			Pattern: "Rakefile",
			Reason:  "A developer task runner. Chef never loads it during a converge; it runs on a workstation or a build agent.",
		},
		{
			Pattern: "Gemfile",
			Reason:  "Declares the developer's Ruby toolchain, not the cookbook. Chef Infra Client does not read it on a node.",
		},
		{
			Pattern: "Gemfile.lock",
			Reason:  "The resolved developer toolchain that accompanies the Gemfile. Not loaded during a converge.",
		},
		{
			Pattern: "spec/*",
			Reason:  "ChefSpec and RSpec unit tests. They execute on a workstation or in CI against a simulated run, never on a converging node.",
		},
		{
			Pattern: "test/*",
			Reason:  "Integration test suites. They execute against a converged node from outside it, not as part of the converge.",
		},
		{
			Pattern: "kitchen.yml*",
			Reason:  "Test Kitchen configuration, consumed by the test harness on a workstation or build agent. Never read by Chef Infra Client.",
		},
		{
			Pattern: ".kitchen.yml*",
			Reason:  "The older dotfile spelling of the Test Kitchen configuration, same reason as kitchen.yml.",
		},
		{
			Pattern: ".github/*",
			Reason:  "CI workflow definitions. They run on the build system, not on a machine Chef converges.",
		},
		{
			Pattern: "Jenkinsfile*",
			Reason:  "A pipeline definition. It runs on the build agent, not on a machine Chef converges.",
		},
	}
}

// ScanScope decides whether a given file is cookbook code for the purposes of a
// verdict. It is a value, not a global: the default list is curated here, and an
// operator's edited list is passed in instead.
type ScanScope struct {
	exclusions []ScanScopeExclusion
}

// NewScanScope builds a scope from an explicit exclusion list. A nil or empty
// list means nothing is excluded — every finding counts towards the verdict,
// which is the previous behaviour and the safe direction to fail in.
func NewScanScope(exclusions []ScanScopeExclusion) *ScanScope {
	return &ScanScope{exclusions: exclusions}
}

// DefaultScanScope is the curated scope alone, with no operator decisions
// layered over it. It is the seed and the fallback, not the live scope — use
// NewScanScopeFromStore anywhere an operator's list should apply.
func DefaultScanScope() *ScanScope {
	return NewScanScope(DefaultScanScopeExclusions())
}

// ScanScopeExclusionLister reads the operator's decisions. Declared here rather
// than taking *datastore.DB so the merge stays testable without a database.
type ScanScopeExclusionLister interface {
	ListScanScopeExclusions(ctx context.Context) ([]datastore.ScanScopeExclusion, error)
}

// NewScanScopeFromStore is the live scope: the curated seed list with the
// operator's decisions layered over it, keyed by pattern.
//
// An operator row for a seeded pattern replaces it — including its reason, so a
// reader sees the justification somebody actually stands behind rather than the
// prose it replaced. A row recorded as not-excluded removes that pattern from
// the effective list, which is how somebody disagrees with a default: a
// customer whose test directory really does ship code that runs must be able to
// say so and be believed.
//
// If the decisions cannot be read, the curated list stands. That direction is
// chosen deliberately: falling back to "nothing is excluded" would flood every
// verdict with helper-task findings, and falling back to "everything is" would
// hide real blockers, which is the failure nobody reports.
func NewScanScopeFromStore(ctx context.Context, store ScanScopeExclusionLister) *ScanScope {
	curated := DefaultScanScopeExclusions()
	if store == nil {
		return NewScanScope(curated)
	}
	rows, err := store.ListScanScopeExclusions(ctx)
	if err != nil {
		return NewScanScope(curated)
	}
	return NewScanScope(mergeScanScopeExclusions(curated, rows))
}

// mergeScanScopeExclusions layers operator decisions over the curated list.
// Curated ordering is preserved so the effective list reads the same way twice,
// with operator-only patterns appended in the order the store returned them.
func mergeScanScopeExclusions(curated []ScanScopeExclusion, rows []datastore.ScanScopeExclusion) []ScanScopeExclusion {
	decision := make(map[string]datastore.ScanScopeExclusion, len(rows))
	for _, row := range rows {
		decision[row.Pattern] = row
	}

	merged := make([]ScanScopeExclusion, 0, len(curated)+len(rows))
	seen := make(map[string]bool, len(curated))
	for _, ex := range curated {
		seen[ex.Pattern] = true
		row, overridden := decision[ex.Pattern]
		if !overridden {
			merged = append(merged, ex)
			continue
		}
		if !row.Excluded {
			// Somebody has established this really is code that runs here.
			continue
		}
		merged = append(merged, ScanScopeExclusion{Pattern: row.Pattern, Reason: row.Reason})
	}

	for _, row := range rows {
		if seen[row.Pattern] || !row.Excluded {
			// A not-excluded row for a pattern we never seeded asserts that
			// something is cookbook code, which is already the default; there is
			// nothing to remove.
			continue
		}
		merged = append(merged, ScanScopeExclusion{Pattern: row.Pattern, Reason: row.Reason})
	}
	return merged
}

// Exclusions returns the list this scope is asserting, so a reader can see —
// and disagree with — every exclusion and its reason.
func (s *ScanScope) Exclusions() []ScanScopeExclusion {
	if s == nil {
		return nil
	}
	return s.exclusions
}

// Excluded reports whether filePath is a file we assert does not execute during
// a converge, and which exclusion said so.
//
// An unknown or empty path is NOT excluded. Being wrong is not symmetrical: a
// wrong exclusion hides a real blocker until production finds it, and nobody
// reports it because nothing looked wrong.
func (s *ScanScope) Excluded(filePath string) (ScanScopeExclusion, bool) {
	if s == nil || filePath == "" {
		return ScanScopeExclusion{}, false
	}
	norm := normaliseScanPath(filePath)
	if norm == "" {
		return ScanScopeExclusion{}, false
	}
	for _, ex := range s.exclusions {
		if matchesScanPattern(ex.Pattern, norm) {
			return ex, true
		}
	}
	return ScanScopeExclusion{}, false
}

// ExcludesPath is Excluded reduced to the boolean, satisfying
// remediation.ScanScoper so complexity scoring can honour scope without the
// remediation package importing this one.
func (s *ScanScope) ExcludesPath(filePath string) bool {
	_, excluded := s.Excluded(filePath)
	return excluded
}

// ExcludesOffense reports whether an offence sits in a file outside cookbook
// code. An offence with no recorded path counts as cookbook code, for the same
// asymmetry reason as Excluded.
func (s *ScanScope) ExcludesOffense(off CookstyleOffense) bool {
	_, excluded := s.Excluded(off.Path())
	return excluded
}

// normaliseScanPath puts a stored path into the repo-relative, forward-slash
// form the patterns are written against. Paths are persisted repo-relative
// already (see relativeCookstylePath), so this only tidies the edges.
func normaliseScanPath(filePath string) string {
	p := strings.ReplaceAll(filePath, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	return path.Clean(p)
}

// matchesScanPattern applies the two pattern forms described on
// ScanScopeExclusion.Pattern.
//
// The "/*" form matches at any depth on purpose. chefignore's own globs do not
// — a chefignore listing "spec/*" leaves spec/unit/foo_spec.rb in place, which
// is exactly the trap that leaves test files on the Chef server in repositories
// whose authors believed they had excluded them. We are not reproducing that
// mistake in our own list.
func matchesScanPattern(pattern, filePath string) bool {
	if pattern == "" {
		return false
	}
	if dir, ok := strings.CutSuffix(pattern, "/*"); ok {
		return filePath == dir || strings.HasPrefix(filePath, dir+"/")
	}
	matched, err := path.Match(pattern, filePath)
	return err == nil && matched
}
