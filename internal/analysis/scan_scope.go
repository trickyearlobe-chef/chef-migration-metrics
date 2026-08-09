// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"path"
	"strings"
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

// DefaultScanScopeExclusions is the seed list. Every entry except the Test
// Kitchen dotfile variant is a path that Chef's own cookbook generator ships or
// ignores, which is where the provenance comes from: these are files Chef
// itself treats as developer tooling rather than cookbook content.
//
// It is deliberately short. Notably absent is the pipeline definition that
// motivated this work in the first place — a Jenkinsfile is not something Chef
// ships, so asserting it for every customer would be us guessing. That is an
// entry an operator adds, and the reason it has to be editable.
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

// DefaultScanScope is the curated scope every derivation uses until an operator
// list is wired in front of it.
func DefaultScanScope() *ScanScope {
	return NewScanScope(DefaultScanScopeExclusions())
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
