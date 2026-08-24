// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// The repository is not the cookbook — see journeys/scan-trust.md.
//
// The estate-wide cop view is where a migration lead reads prevalence, and one
// copied Rakefile can make most cookbooks look broken. So the correction has to
// land here too: the same cop must be counted separately depending on whether
// it can block, and neither count may be dropped.

// TestCopAggregation_SplitsBlockingFromOutsideCookbookCode builds three
// cookbooks carrying one cop between them:
//
//	cb-recipe   — in a recipe only            → blocks
//	cb-both     — in a recipe AND a Rakefile   → blocks (the recipe decides)
//	cb-rakefile — in a Rakefile only           → does not block, still counted
func TestCopAggregation_SplitsBlockingFromOutsideCookbookCode(t *testing.T) {
	const cop = "Lint/DeprecatedClassMethods"

	inRecipe := mustMarshalCops(t, []map[string]any{{
		"path":     "recipes/default.rb",
		"offenses": []map[string]any{{"cop_name": cop, "severity": "warning", "correctable": true}},
	}})
	inBoth := mustMarshalCops(t, []map[string]any{
		{
			"path":     "recipes/default.rb",
			"offenses": []map[string]any{{"cop_name": cop, "severity": "warning", "correctable": true}},
		},
		{
			"path":     "Rakefile",
			"offenses": []map[string]any{{"cop_name": cop, "severity": "warning", "correctable": true}},
		},
	})
	inRakefile := mustMarshalCops(t, []map[string]any{{
		"path":     "Rakefile",
		"offenses": []map[string]any{{"cop_name": cop, "severity": "warning", "correctable": true}},
	}})

	scanned := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(_ context.Context, tv string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return []datastore.ServerCookbookCookstyleResult{
				{CookbookName: "cb-recipe", CookbookVersion: "1.0.0", OrganisationName: "example-org", TargetChefVersion: tv, OffenceCount: 1, Offences: inRecipe, ScannedAt: scanned},
				{CookbookName: "cb-both", CookbookVersion: "1.0.0", OrganisationName: "example-org", TargetChefVersion: tv, OffenceCount: 2, Offences: inBoth, ScannedAt: scanned},
				{CookbookName: "cb-rakefile", CookbookVersion: "1.0.0", OrganisationName: "example-org", TargetChefVersion: tv, OffenceCount: 1, Offences: inRakefile, ScannedAt: scanned},
			}, nil
		},
		ListGitRepoCookstyleResultsByTargetVersionFn: func(_ context.Context, _ string) ([]datastore.GitRepoCookstyleResult, error) {
			return nil, nil
		},
	}

	r := newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0"))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/cookstyle/cops?target_chef_version=19.0&triggered_only=true", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp copAggregationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	var item *copAggregateItem
	for i := range resp.Data {
		if resp.Data[i].CopName == cop {
			item = &resp.Data[i]
			break
		}
	}
	if item == nil {
		t.Fatalf("%s not found in response", cop)
	}

	// Two cookbooks carry it where it can actually block. Before scope this read
	// as three, which is the inflation that made the list unbelievable.
	if item.CookbooksAffected != 2 {
		t.Errorf("cookbooks_affected = %d, want 2 (cb-recipe and cb-both)", item.CookbooksAffected)
	}

	// The third is not lost — it is reported as prevalence, which is the thing
	// most worth knowing about a finding nobody has to fix to migrate.
	if item.CookbooksExcludedOnly != 1 {
		t.Errorf("cookbooks_excluded_only = %d, want 1 (cb-rakefile)", item.CookbooksExcludedOnly)
	}

	// cb-both is counted once, as affected. Counting it in both columns would
	// make them stop meaning anything when added together.
	if item.TotalOffences != 4 {
		t.Errorf("total_offences = %d, want 4 (every occurrence still counted)", item.TotalOffences)
	}
	if item.ExcludedOffences != 2 {
		t.Errorf("excluded_offences = %d, want 2 (the two Rakefile copies)", item.ExcludedOffences)
	}
}

// TestCopDrillDown_OmitsCookbooksAffectedOnlyOutsideCookbookCode keeps the
// drill-down and the header agreeing. The list under a cop is documented to
// total its cookbooks_affected count, so a cookbook whose only copy is in a
// Rakefile must not appear there — its prevalence is the cop row's
// cookbooks_excluded_only instead.
func TestCopDrillDown_OmitsCookbooksAffectedOnlyOutsideCookbookCode(t *testing.T) {
	const cop = "Lint/DeprecatedClassMethods"

	inRecipe := mustMarshalCops(t, []map[string]any{{
		"path":     "recipes/default.rb",
		"offenses": []map[string]any{{"cop_name": cop, "severity": "warning", "correctable": true}},
	}})
	inRakefile := mustMarshalCops(t, []map[string]any{{
		"path":     "Rakefile",
		"offenses": []map[string]any{{"cop_name": cop, "severity": "warning", "correctable": true}},
	}})

	scanned := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(_ context.Context, _ string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return nil, nil
		},
		ListGitRepoCookstyleResultsByTargetVersionFn: func(_ context.Context, tv string) ([]datastore.GitRepoCookstyleResult, error) {
			return []datastore.GitRepoCookstyleResult{
				{GitRepoName: "repo-recipe", TargetChefVersion: tv, OffenceCount: 1, Offences: inRecipe, ScannedAt: scanned},
				{GitRepoName: "repo-rakefile", TargetChefVersion: tv, OffenceCount: 1, Offences: inRakefile, ScannedAt: scanned},
			}, nil
		},
	}

	r := newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0"))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/cookstyle/cops/"+cop+"/cookbooks?target_chef_version=19.0&source=git", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp copCookbookResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("want 1 repo in the drill-down, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "repo-recipe" {
		t.Errorf("drill-down listed %q; only the repo where the cop can block belongs here", resp.Data[0].Name)
	}
}
