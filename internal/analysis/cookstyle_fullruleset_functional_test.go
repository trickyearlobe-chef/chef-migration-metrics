// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package analysis_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// TestFunctional_FullRuleset_DeprecatedClassMethodsBlocks is the end-to-end
// proof that Chunk A closed the gap: with the --only narrowing dropped, a real
// cookstyle scan of a cookbook using File.exists? at target >= 18 actually
// PRODUCES the Lint/DeprecatedClassMethods offence (curated Blocker), which
// lives outside Chef/Deprecations,Chef/Correctness and so was previously
// hidden. Classification then drives the rollup to blocked.
//
// This exercises the whole pipeline against the live cookstyle binary:
// buildCookstyleArgs (no --only) -> real cookstyle -> JSON parse ->
// curated-default resolver -> DeriveCookstyleStatus -> persisted result.
func TestFunctional_FullRuleset_DeprecatedClassMethodsBlocks(t *testing.T) {
	cookstylePath, err := exec.LookPath("cookstyle")
	if err != nil {
		t.Skip("cookstyle binary not found on PATH — skipping functional scan test")
	}

	db := backfillTestDB(t)
	ctx := context.Background()

	const repoName = "func-fullruleset-repo"
	const repoURL = "https://git.example.com/func-fullruleset.git"
	const targetVersion = "18.0"
	cleanup(t, db,
		"DELETE FROM cookstyle_offence_fingerprints WHERE git_repo_name = '"+repoName+"'",
		"DELETE FROM git_repo_cookstyle_results WHERE git_repo_name = '"+repoName+"'",
		"DELETE FROM git_repos WHERE name = '"+repoName+"'",
	)

	// Parent row for the git_repo_cookstyle_results FK.
	gr, err := db.UpsertGitRepo(ctx, datastore.UpsertGitRepoParams{
		Name:          repoName,
		GitRepoURL:    repoURL,
		HeadCommitSHA: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		DefaultBranch: "main",
		LastFetchedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed git repo: %v", err)
	}

	// Fixture cookbook whose only offence is the deprecated File.exists?
	// (removed in Ruby 3 — curated Blocker at target >= 18).
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "recipes"), 0o755); err != nil {
		t.Fatalf("mkdir recipes: %v", err)
	}
	recipe := "if File.exists?('/etc/passwd')\n  puts 'present'\nend\n"
	if err := os.WriteFile(filepath.Join(repoDir, "recipes", "default.rb"), []byte(recipe), 0o644); err != nil {
		t.Fatalf("write recipe: %v", err)
	}

	logger := logging.New(logging.Options{Level: logging.ERROR, Writers: []logging.Writer{logging.NewMemoryWriter()}})
	scanner := analysis.NewCookstyleScanner(db, logger, cookstylePath, 1, 5)

	sr := scanner.ScanSingleGitRepo(ctx, gr, targetVersion, repoDir)
	if sr.Error != nil {
		t.Fatalf("scan returned error: %v (stderr: %s)", sr.Error, sr.RawStderr)
	}
	if sr.ErrorMessage != "" {
		t.Fatalf("scan recorded a cookstyle error: %s", sr.ErrorMessage)
	}

	// The full-ruleset scan must have produced the previously-hidden offence.
	var found bool
	for _, off := range sr.Offenses {
		if off.CopName == "Lint/DeprecatedClassMethods" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a Lint/DeprecatedClassMethods offence from the full-ruleset scan, got offences: %+v", sr.Offenses)
	}

	// Classification drives the rollup: a curated Blocker => blocked / not passed.
	if sr.CookstyleStatus != analysis.StatusBlocked {
		t.Errorf("rollup status = %q, want %q", sr.CookstyleStatus, analysis.StatusBlocked)
	}
	if sr.Passed {
		t.Error("passed must be false when a Blocker fires")
	}

	// The verdict was persisted, not just returned in-memory.
	stored, err := db.GetGitRepoCookstyleResult(ctx, repoName, repoURL, targetVersion)
	if err != nil {
		t.Fatalf("get stored result: %v", err)
	}
	if stored == nil {
		t.Fatal("expected a persisted git repo cookstyle result")
	}
	if stored.CookstyleStatus != analysis.StatusBlocked {
		t.Errorf("persisted status = %q, want %q", stored.CookstyleStatus, analysis.StatusBlocked)
	}
}
