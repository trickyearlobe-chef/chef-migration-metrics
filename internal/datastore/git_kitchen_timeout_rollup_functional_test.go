// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/tkstatus"
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
	seedKitchenResultKind(t, db, name, target, platform, passed, timedOut, "")
}

// seedKitchenResultKind is seedKitchenResult with an explicit failure kind.
func seedKitchenResultKind(t *testing.T, db *DB, name, target, platform string, passed *bool, timedOut bool, kind string) {
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
		Passed:       passed, TimedOut: timedOut, FailureKind: kind,
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

// The larger half of the same fault. A lab failure normally exits non-zero
// rather than timing out — the VM never got built, the credentials were wrong,
// the tooling fell over — and all of those were stored as `passed = false`,
// indistinguishable from a cookbook that will not converge. The recorded
// failure kind is what tells them apart.
func TestFunctional_LabFailure_DoesNotBlockTheCookbook(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	const target = "19.3.17"

	cleanupTestData(t, db,
		"DELETE FROM git_kitchen_results WHERE git_repo_name LIKE 'tkfk-%'",
		"DELETE FROM git_repos WHERE name LIKE 'tkfk-%'",
	)

	// The lab never produced a machine to converge on.
	seedKitchenResultKind(t, db, "tkfk-create", target, "rhel-9", boolp(false), false, "create_failed")
	// The run died before Chef started.
	seedKitchenResultKind(t, db, "tkfk-no-converge", target, "rhel-9", boolp(false), false, "no_converge")
	// Converged fine, then failed to tear the VM down: a leak, not a verdict.
	seedKitchenResultKind(t, db, "tkfk-destroy", target, "rhel-9", boolp(false), false, "destroy_failed")
	// The cookbook was converged and did not come up.
	seedKitchenResultKind(t, db, "tkfk-converge", target, "rhel-9", boolp(false), false, "converge_failed")
	// It converged; its own tests failed. Still the cookbook's problem.
	seedKitchenResultKind(t, db, "tkfk-verify", target, "rhel-9", boolp(false), false, "verify_failed")
	// A failure nobody could classify keeps counting, so nothing is unblocked
	// by accident.
	seedKitchenResultKind(t, db, "tkfk-unknown", target, "rhel-9", boolp(false), false, "unknown")
	// An older binary writing after a rollback leaves no kind at all.
	seedKitchenResultKind(t, db, "tkfk-legacy", target, "rhel-9", boolp(false), false, "")
	// One platform blocked by the lab, another by the cookbook.
	seedKitchenResultKind(t, db, "tkfk-mixed", target, "rhel-9", boolp(true), false, "")
	seedKitchenResultKind(t, db, "tkfk-mixed", target, "win-2019", boolp(false), false, "create_failed")

	want := map[string]string{
		"tkfk-create":      "untested",
		"tkfk-no-converge": "untested",
		"tkfk-destroy":     "untested",
		"tkfk-converge":    "failed",
		"tkfk-verify":      "failed",
		"tkfk-unknown":     "failed",
		"tkfk-legacy":      "failed",
		"tkfk-mixed":       "passed",
	}
	for name, wantStatus := range want {
		repos, err := db.ListGitReposByName(ctx, name)
		if err != nil || len(repos) != 1 {
			t.Fatalf("listing %s: %v (n=%d)", name, err, len(repos))
		}
		if repos[0].TKStatus != wantStatus {
			t.Errorf("%s: tk_status = %q, want %q", name, repos[0].TKStatus, wantStatus)
		}
	}

	// And the same rule reaches the counts readiness reads.
	counts, err := db.ListGitKitchenCountsByTargetVersions(ctx, []string{target})
	if err != nil {
		t.Fatalf("ListGitKitchenCountsByTargetVersions: %v", err)
	}
	if c, ok := counts["tkfk-create|"+target]; ok {
		t.Errorf("lab failure reported counts %+v to readiness, want no entry", c)
	}
	if c := counts["tkfk-converge|"+target]; c.Failed != 1 {
		t.Errorf("converge failure reported %+v, want Failed=1", c)
	}
}

// The migration classifies results captured before the column existed, using
// the same rule as the Go classifier. If the two drift, a repo is re-verdicted
// on deploy by a rule nobody tested — so they are pinned to each other here.
func TestFunctional_BackfillClassification_MatchesTheGoClassifier(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cases := []struct {
		name     string
		output   string
		passed   *bool
		timedOut bool
	}{
		{"create", ">>>>>>     Failed to complete #create action: [Task timeout after 300s] on default-alma-9", boolp(false), false},
		{"converge", "Converging 3 resources\n>>>>>>     Failed to complete #converge action: [x] on default-rocky-9", boolp(false), false},
		{"verify", "Converging 3 resources\n>>>>>>     Failed to complete #verify action: [x] on default-rocky-9", boolp(false), false},
		{"destroy", "Converging 3 resources\n>>>>>>     Failed to complete #destroy action: [x] on default-rocky-9", boolp(false), false},
		{"no converge", ">>>>>> Class: Kitchen::UserError\n>>>>>> Message: Cannot use remote lifecycle hooks", boolp(false), false},
		{"network timeout", "-----> Creating <default-rocky-9>...", nil, true},
		{"timeout while converging", "Starting Chef Infra Client, version 19.3.15", nil, true},
		{"unknown", "resolving cookbooks for run list\nsomething nobody has seen", boolp(false), false},
		{"passed", "Converging 3 resources", boolp(true), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var fromSQL string
			if err := db.pool.QueryRowContext(ctx,
				`SELECT gkr_failure_kind($1, $2, $3)`, c.output, c.passed, c.timedOut,
			).Scan(&fromSQL); err != nil {
				t.Fatalf("gkr_failure_kind: %v", err)
			}
			// The Go classifier derives network-vs-plain timeout from the same
			// stored evidence the SQL has: a timeout with no sign of Chef.
			fromGo := tkstatus.ClassifyFailure(c.output, c.passed, c.timedOut, false)
			if fromSQL != fromGo {
				t.Errorf("SQL said %q, Go said %q", fromSQL, fromGo)
			}
		})
	}
}
