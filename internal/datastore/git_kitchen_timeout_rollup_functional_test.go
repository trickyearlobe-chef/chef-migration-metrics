// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
	"time"
)

// A Test Kitchen timeout is a statement about the lab, not about the cookbook.
// It used to be counted as a cookbook failure (`passed = false OR timed_out`),
// which rolled up to tk_status = 'failed' and made readiness call the cookbook
// incompatible — overriding a CookStyle pass and blocking every node running
// it. On an estate where most runs time out on DHCP, that blocked real nodes
// for a reason that was never about the cookbook.
//
// These tests pin the rule: a timeout counts as neither a pass nor a failure,
// so a repo whose only evidence is a timeout is untested, not failed.

// seedKitchenResult writes one instance result for a repo, creating the repo.
func seedKitchenResult(t *testing.T, db *DB, name, target, platform string, passed *bool, timedOut bool) {
	t.Helper()
	ctx := context.Background()
	url := "git@example.com:org-a/" + name
	if _, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{
		Name: name, GitRepoURL: url, LastFetchedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert repo %s: %v", name, err)
	}
	if _, err := db.UpsertGitKitchenResult(ctx, UpsertGitKitchenResultParams{
		GitRepoName: name, GitRepoURL: url, TargetChefVersion: target,
		PlatformName: platform, SuiteName: "default",
		InstanceName: "default-" + platform,
		Passed:       passed, TimedOut: timedOut,
	}); err != nil {
		t.Fatalf("upsert kitchen result for %s: %v", name, err)
	}
}

func boolp(b bool) *bool { return &b }

func TestFunctional_TimedOutKitchenRun_IsNotACookbookFailure(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	const target = "19.3.15"

	cleanupTestData(t, db,
		"DELETE FROM git_kitchen_results WHERE git_repo_name LIKE 'tkto-%'",
		"DELETE FROM git_repos WHERE name LIKE 'tkto-%'",
	)

	// A run that timed out never reached a verdict: passed is NULL.
	seedKitchenResult(t, db, "tkto-only-timeout", target, "rhel-9", nil, true)
	// A repo that passed on one platform and timed out on another.
	seedKitchenResult(t, db, "tkto-pass-and-timeout", target, "rhel-9", boolp(true), false)
	seedKitchenResult(t, db, "tkto-pass-and-timeout", target, "win-2019", nil, true)
	// A cookbook that genuinely failed to converge must still fail.
	seedKitchenResult(t, db, "tkto-real-failure", target, "rhel-9", boolp(false), false)

	want := map[string]struct {
		status string
		passed int
		total  int
	}{
		"tkto-only-timeout":     {"untested", 0, 0},
		"tkto-pass-and-timeout": {"passed", 1, 1},
		"tkto-real-failure":     {"failed", 0, 1},
	}

	for name, w := range want {
		repos, err := db.ListGitReposByName(ctx, name)
		if err != nil || len(repos) != 1 {
			t.Fatalf("listing %s: %v (n=%d)", name, err, len(repos))
		}
		if repos[0].TKStatus != w.status {
			t.Errorf("%s: tk_status = %q, want %q", name, repos[0].TKStatus, w.status)
		}
		if repos[0].TKPassed != w.passed || repos[0].TKTotal != w.total {
			t.Errorf("%s: tk_passed/tk_total = %d/%d, want %d/%d",
				name, repos[0].TKPassed, repos[0].TKTotal, w.passed, w.total)
		}
	}
}

// The counts the readiness evaluator reads. A timeout must not arrive there as
// a failure either, or readiness blocks nodes the list view calls untested.
func TestFunctional_KitchenCountsByTargetVersion_ExcludeTimeouts(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	const target = "19.3.16"

	cleanupTestData(t, db,
		"DELETE FROM git_kitchen_results WHERE git_repo_name LIKE 'tkcnt-%'",
		"DELETE FROM git_repos WHERE name LIKE 'tkcnt-%'",
	)

	seedKitchenResult(t, db, "tkcnt-only-timeout", target, "rhel-9", nil, true)
	seedKitchenResult(t, db, "tkcnt-real-failure", target, "rhel-9", boolp(false), false)

	counts, err := db.ListGitKitchenCountsByTargetVersions(ctx, []string{target})
	if err != nil {
		t.Fatalf("ListGitKitchenCountsByTargetVersions: %v", err)
	}

	// A repo with nothing but a timeout has no evidence at all, so it should
	// not appear — appearing with Failed > 0 is what blocks nodes.
	if c, ok := counts["tkcnt-only-timeout|"+target]; ok {
		t.Errorf("timed-out repo reported counts %+v, want no entry", c)
	}
	if c := counts["tkcnt-real-failure|"+target]; c.Failed != 1 {
		t.Errorf("genuine failure reported %+v, want Failed=1", c)
	}
}
