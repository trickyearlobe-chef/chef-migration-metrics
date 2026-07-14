// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Chunk A: full-ruleset scanning — the scan must NOT pass --only.
//
// The --only Chef/Deprecations,Chef/Correctness narrowing predated the
// classification system and silently hid every Blocker-classified cop that
// lives outside those two departments (e.g. the curated default
// Lint/DeprecatedClassMethods). With classification driving the verdict, the
// scan now runs the full ruleset and lets classification decide.
// ---------------------------------------------------------------------------

// TestBuildCookstyleArgs_DropsOnly_FullRuleset proves the scan no longer
// restricts to two departments: with a target version set we still write the
// sidecar (--config) but must NOT emit --only.
func TestBuildCookstyleArgs_DropsOnly_FullRuleset(t *testing.T) {
	cookbookDir := t.TempDir()
	args := buildCookstyleArgs(cookbookDir, "18.0")

	for _, a := range args {
		if a == "--only" {
			t.Fatalf("scan must run the full ruleset — --only must not be present, got %v", args)
		}
	}

	// The sidecar config (TargetChefVersion) is still required.
	assertContains(t, args, "--config")
	assertContains(t, args, "--format")
	assertContains(t, args, "json")
	if args[len(args)-1] != cookbookDir {
		t.Errorf("last arg should be cookbook dir, got %q", args[len(args)-1])
	}
}

// TestDeriveStatus_BlockerOutsideDepartments_Blocked proves that once the
// scan stops hiding it, a Blocker-classified cop OUTSIDE Deprecations/
// Correctness (Lint/DeprecatedClassMethods, curated Blocker at target >= 18)
// drives the rollup to Blocked under the default failure rules.
func TestDeriveStatus_BlockerOutsideDepartments_Blocked(t *testing.T) {
	offenses := []CookstyleOffense{
		// File.exists? — curated Blocker at >= 18.0, severity is a mere warning,
		// so the default error/fatal rules alone would NOT catch it. Only the
		// classification makes it block — and only if the scan produced it.
		{Severity: "warning", CopName: "Lint/DeprecatedClassMethods", Message: "File.exists? is deprecated"},
	}
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	// Sanity: the cop is classified Blocker by a curated default.
	if got := resolver.Classify("Lint/DeprecatedClassMethods"); got != ClassificationBlocker {
		t.Fatalf("Lint/DeprecatedClassMethods should classify as blocker at 18.0, got %q", got)
	}

	status := DeriveCookstyleStatus(offenses, resolver)
	if status != StatusBlocked {
		t.Errorf("a Blocker cop outside the two departments should yield blocked, got %q", status)
	}
}

// TestDeriveStatus_CosmeticStyleCop_Ready proves widening the ruleset does not
// turn cookbooks red: a cosmetic generic Style cop seeds to Noise via the
// Chunk B department-prefix default, is non-failing under the default rules,
// and yields Ready.
func TestDeriveStatus_CosmeticStyleCop_Ready(t *testing.T) {
	offenses := []CookstyleOffense{
		{Severity: "convention", CopName: "Style/StringLiterals", Message: "Prefer single-quoted strings"},
	}
	resolver := &CopClassificationResolver{
		OperatorOverrides: map[string]string{},
		TargetChefVersion: "18.0",
	}

	if got := resolver.Classify("Style/StringLiterals"); got != ClassificationNoise {
		t.Fatalf("a generic Style cop should seed to noise via the curated prefix, got %q", got)
	}

	status := DeriveCookstyleStatus(offenses, resolver)
	if status != StatusReady {
		t.Errorf("a cosmetic convention-severity cop must not block, want %q got %q", StatusReady, status)
	}
}

// TestBuildCookstyleArgs_NoOnly_EvenWithCookbookConfig guards the cookbook-config
// branch too — inheriting a cookbook's .rubocop.yml must not reintroduce --only.
func TestBuildCookstyleArgs_NoOnly_EvenWithCookbookConfig(t *testing.T) {
	cookbookDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cookbookDir, ".rubocop.yml"), []byte("require:\n  - cookstyle\n"), 0644); err != nil {
		t.Fatal(err)
	}
	args := buildCookstyleArgs(cookbookDir, "17.0")
	if strings.Contains(strings.Join(args, " "), "--only") {
		t.Errorf("scan must not pass --only even with a cookbook config, got %v", args)
	}
}
