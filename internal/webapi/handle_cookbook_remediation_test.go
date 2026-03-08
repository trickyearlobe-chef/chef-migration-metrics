// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// handleCookbookRemediation — method checks
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_MethodNotAllowed_POST(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCookbookRemediation_MethodNotAllowed_PUT(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCookbookRemediation_MethodNotAllowed_DELETE(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCookbookRemediation_ContentType(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — missing path segments
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_MissingVersion(t *testing.T) {
	// /api/v1/cookbooks/apt/remediation — only 2 segments, last is "remediation"
	// but no version segment. Should NOT dispatch to remediation handler;
	// the pathSegments check requires >= 3 segments.
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			// If it falls through to the detail handler that's fine too.
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/remediation", nil)
	r.ServeHTTP(w, req)

	// This should fall through to the cookbook detail handler (not remediation),
	// which will treat "apt/remediation" as a cookbook name lookup.
	// It will likely return 404 since no cookbook named "apt/remediation" exists.
	// We just verify it doesn't panic and doesn't return 500.
	if w.Code == http.StatusInternalServerError {
		t.Errorf("expected non-500 status for missing version, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — no target version configured
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_NoTargetVersion(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{{ID: "cb-1", Name: "apt", Version: "1.0.0"}}, nil
		},
	}
	cfg := &config.Config{}
	wsEnabled := true
	cfg.Server.WebSocket.Enabled = &wsEnabled
	// No TargetChefVersions set

	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — cookbook not found
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_CookbookNotFound(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/nonexistent/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleCookbookRemediation_VersionNotFound(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "2.0.0"},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — happy path: no cookstyle result
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_NoCookstyleResult(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			return nil, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if body["cookbook_name"] != "apt" {
		t.Errorf("cookbook_name = %v, want %q", body["cookbook_name"], "apt")
	}
	if body["cookbook_version"] != "1.0.0" {
		t.Errorf("cookbook_version = %v, want %q", body["cookbook_version"], "1.0.0")
	}
	if body["target_chef_version"] != "18.0" {
		t.Errorf("target_chef_version = %v, want %q", body["target_chef_version"], "18.0")
	}

	// cookstyle_passed should be nil/null when no result exists.
	if body["cookstyle_passed"] != nil {
		t.Errorf("cookstyle_passed = %v, want nil", body["cookstyle_passed"])
	}

	stats, ok := body["statistics"].(map[string]any)
	if !ok {
		t.Fatalf("statistics is not a map: %T", body["statistics"])
	}
	if stats["total_offenses"] != float64(0) {
		t.Errorf("total_offenses = %v, want 0", stats["total_offenses"])
	}

	groups, ok := body["offense_groups"].([]any)
	if !ok {
		t.Fatalf("offense_groups is not an array: %T", body["offense_groups"])
	}
	if len(groups) != 0 {
		t.Errorf("len(offense_groups) = %d, want 0", len(groups))
	}

	acPreview, ok := body["autocorrect_preview"].(map[string]any)
	if !ok {
		t.Fatalf("autocorrect_preview is not a map: %T", body["autocorrect_preview"])
	}
	if acPreview["available"] != false {
		t.Errorf("autocorrect_preview.available = %v, want false", acPreview["available"])
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — happy path with offenses (file-based format)
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_WithOffenses_FileFormat(t *testing.T) {
	offensesJSON := `[
		{
			"path": "recipes/default.rb",
			"offenses": [
				{
					"cop_name": "Chef/Deprecations/ResourceWithoutUnifiedTrue",
					"severity": "warning",
					"message": "Set unified_mode true",
					"correctable": true,
					"location": {"start_line": 5, "start_column": 1, "last_line": 5, "last_column": 40}
				},
				{
					"cop_name": "Chef/Deprecations/ResourceWithoutUnifiedTrue",
					"severity": "warning",
					"message": "Set unified_mode true",
					"correctable": true,
					"location": {"start_line": 15, "start_column": 1, "last_line": 15, "last_column": 40}
				}
			]
		},
		{
			"path": "recipes/install.rb",
			"offenses": [
				{
					"cop_name": "Chef/Correctness/InvalidPlatformFamilyHelper",
					"severity": "error",
					"message": "Invalid platform family",
					"correctable": false,
					"location": {"start_line": 3, "start_column": 1, "last_line": 3, "last_column": 30}
				}
			]
		}
	]`

	now := time.Now().UTC()

	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			if cookbookID != "cb-1" {
				t.Errorf("GetCookstyleResult cookbookID = %q, want cb-1", cookbookID)
			}
			if targetChefVersion != "18.0" {
				t.Errorf("GetCookstyleResult targetChefVersion = %q, want 18.0", targetChefVersion)
			}
			return &datastore.CookstyleResult{
				ID:                "cs-1",
				CookbookID:        "cb-1",
				TargetChefVersion: "18.0",
				Passed:            false,
				OffenceCount:      3,
				Offences:          []byte(offensesJSON),
				ScannedAt:         now,
			}, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return []datastore.CookbookComplexity{
				{
					CookbookID:           "cb-1",
					TargetChefVersion:    "18.0",
					ComplexityScore:      42,
					ComplexityLabel:      "medium",
					AutoCorrectableCount: 2,
					ManualFixCount:       1,
					DeprecationCount:     2,
					ErrorCount:           1,
				},
			}, nil
		},
		GetAutocorrectPreviewFn: func(ctx context.Context, cookstyleResultID string) (*datastore.AutocorrectPreview, error) {
			return nil, nil // No preview
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Verify top-level fields.
	if body["cookbook_name"] != "apt" {
		t.Errorf("cookbook_name = %v, want %q", body["cookbook_name"], "apt")
	}
	if body["cookbook_version"] != "1.0.0" {
		t.Errorf("cookbook_version = %v, want %q", body["cookbook_version"], "1.0.0")
	}
	if body["target_chef_version"] != "18.0" {
		t.Errorf("target_chef_version = %v, want %q", body["target_chef_version"], "18.0")
	}
	if body["complexity_score"] != float64(42) {
		t.Errorf("complexity_score = %v, want 42", body["complexity_score"])
	}
	if body["complexity_label"] != "medium" {
		t.Errorf("complexity_label = %v, want %q", body["complexity_label"], "medium")
	}
	if body["cookstyle_passed"] != false {
		t.Errorf("cookstyle_passed = %v, want false", body["cookstyle_passed"])
	}

	// Verify statistics.
	stats, ok := body["statistics"].(map[string]any)
	if !ok {
		t.Fatalf("statistics is not a map: %T", body["statistics"])
	}
	if stats["total_offenses"] != float64(3) {
		t.Errorf("total_offenses = %v, want 3", stats["total_offenses"])
	}
	if stats["correctable_offenses"] != float64(2) {
		t.Errorf("correctable_offenses = %v, want 2", stats["correctable_offenses"])
	}
	if stats["remaining_offenses"] != float64(1) {
		t.Errorf("remaining_offenses = %v, want 1", stats["remaining_offenses"])
	}
	if stats["auto_correctable_count"] != float64(2) {
		t.Errorf("auto_correctable_count = %v, want 2", stats["auto_correctable_count"])
	}
	if stats["manual_fix_count"] != float64(1) {
		t.Errorf("manual_fix_count = %v, want 1", stats["manual_fix_count"])
	}
	if stats["deprecation_count"] != float64(2) {
		t.Errorf("deprecation_count = %v, want 2", stats["deprecation_count"])
	}
	if stats["error_count"] != float64(1) {
		t.Errorf("error_count = %v, want 1", stats["error_count"])
	}
	if stats["offense_groups"] != float64(2) {
		t.Errorf("offense_groups = %v, want 2", stats["offense_groups"])
	}

	// Verify offense groups.
	groups, ok := body["offense_groups"].([]any)
	if !ok {
		t.Fatalf("offense_groups is not an array: %T", body["offense_groups"])
	}
	if len(groups) != 2 {
		t.Fatalf("len(offense_groups) = %d, want 2", len(groups))
	}

	// First group: ChefDeprecations/ResourceWithoutUnifiedTrue (2 offenses).
	g0 := groups[0].(map[string]any)
	if g0["cop_name"] != "Chef/Deprecations/ResourceWithoutUnifiedTrue" {
		t.Errorf("group[0].cop_name = %v, want ChefDeprecations/ResourceWithoutUnifiedTrue", g0["cop_name"])
	}
	if g0["count"] != float64(2) {
		t.Errorf("group[0].count = %v, want 2", g0["count"])
	}
	if g0["correctable_count"] != float64(2) {
		t.Errorf("group[0].correctable_count = %v, want 2", g0["correctable_count"])
	}
	// This cop is in the embedded mapping, so remediation should be populated.
	if g0["remediation"] == nil {
		t.Error("group[0].remediation is nil, expected a mapping entry")
	} else {
		rem := g0["remediation"].(map[string]any)
		if rem["cop_name"] != "Chef/Deprecations/ResourceWithoutUnifiedTrue" {
			t.Errorf("group[0].remediation.cop_name = %v", rem["cop_name"])
		}
		if rem["description"] == nil || rem["description"] == "" {
			t.Error("group[0].remediation.description is empty")
		}
	}

	// Verify offenses within the first group include file paths.
	offenses0 := g0["offenses"].([]any)
	if len(offenses0) != 2 {
		t.Fatalf("group[0].offenses length = %d, want 2", len(offenses0))
	}
	off0 := offenses0[0].(map[string]any)
	loc0 := off0["location"].(map[string]any)
	if loc0["file"] != "recipes/default.rb" {
		t.Errorf("offense[0].location.file = %v, want recipes/default.rb", loc0["file"])
	}
	if loc0["start_line"] != float64(5) {
		t.Errorf("offense[0].location.start_line = %v, want 5", loc0["start_line"])
	}

	// Second group: ChefCorrectness/InvalidPlatformFamilyHelper (1 offense).
	g1 := groups[1].(map[string]any)
	if g1["cop_name"] != "Chef/Correctness/InvalidPlatformFamilyHelper" {
		t.Errorf("group[1].cop_name = %v, want ChefCorrectness/InvalidPlatformFamilyHelper", g1["cop_name"])
	}
	if g1["count"] != float64(1) {
		t.Errorf("group[1].count = %v, want 1", g1["count"])
	}
	if g1["correctable_count"] != float64(0) {
		t.Errorf("group[1].correctable_count = %v, want 0", g1["correctable_count"])
	}

	// Verify autocorrect preview.
	acPreview, ok := body["autocorrect_preview"].(map[string]any)
	if !ok {
		t.Fatalf("autocorrect_preview is not a map: %T", body["autocorrect_preview"])
	}
	if acPreview["available"] != false {
		t.Errorf("autocorrect_preview.available = %v, want false", acPreview["available"])
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — happy path with offenses (flat format)
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_WithOffenses_FlatFormat(t *testing.T) {
	offensesJSON := `[
		{
			"cop_name": "Chef/Deprecations/ResourceWithoutUnifiedTrue",
			"severity": "warning",
			"message": "Set unified_mode true",
			"correctable": true,
			"location": {"file": "recipes/default.rb", "start_line": 5, "start_column": 1, "last_line": 5, "last_column": 40}
		},
		{
			"cop_name": "Chef/Deprecations/ResourceWithoutUnifiedTrue",
			"severity": "warning",
			"message": "Set unified_mode true again",
			"correctable": false,
			"location": {"file": "recipes/install.rb", "start_line": 10, "start_column": 1, "last_line": 10, "last_column": 40}
		}
	]`

	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "nginx", Version: "2.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			return &datastore.CookstyleResult{
				ID:                "cs-2",
				CookbookID:        "cb-1",
				TargetChefVersion: "18.0",
				Passed:            false,
				OffenceCount:      2,
				Offences:          []byte(offensesJSON),
				ScannedAt:         time.Now().UTC(),
			}, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return nil, nil
		},
		GetAutocorrectPreviewFn: func(ctx context.Context, cookstyleResultID string) (*datastore.AutocorrectPreview, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/nginx/2.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	stats := body["statistics"].(map[string]any)
	if stats["total_offenses"] != float64(2) {
		t.Errorf("total_offenses = %v, want 2", stats["total_offenses"])
	}
	if stats["correctable_offenses"] != float64(1) {
		t.Errorf("correctable_offenses = %v, want 1", stats["correctable_offenses"])
	}
	if stats["remaining_offenses"] != float64(1) {
		t.Errorf("remaining_offenses = %v, want 1", stats["remaining_offenses"])
	}

	// Should be 1 group since both offenses are from the same cop.
	groups := body["offense_groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("len(offense_groups) = %d, want 1", len(groups))
	}
	g0 := groups[0].(map[string]any)
	if g0["count"] != float64(2) {
		t.Errorf("group[0].count = %v, want 2", g0["count"])
	}
	if g0["correctable_count"] != float64(1) {
		t.Errorf("group[0].correctable_count = %v, want 1", g0["correctable_count"])
	}

	// Check that file path is included in flat format.
	offenses := g0["offenses"].([]any)
	off0 := offenses[0].(map[string]any)
	loc := off0["location"].(map[string]any)
	if loc["file"] != "recipes/default.rb" {
		t.Errorf("offense[0].location.file = %v, want recipes/default.rb", loc["file"])
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — happy path with autocorrect preview
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_WithAutocorrectPreview(t *testing.T) {
	now := time.Now().UTC()
	diffOutput := `--- a/recipes/default.rb
+++ b/recipes/default.rb
@@ -5,1 +5,2 @@
-resource_name :my_thing
+resource_name :my_thing
+unified_mode true`

	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			return &datastore.CookstyleResult{
				ID:                "cs-1",
				CookbookID:        "cb-1",
				TargetChefVersion: "18.0",
				Passed:            false,
				OffenceCount:      1,
				Offences:          []byte(`[]`),
				ScannedAt:         now,
			}, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return nil, nil
		},
		GetAutocorrectPreviewFn: func(ctx context.Context, cookstyleResultID string) (*datastore.AutocorrectPreview, error) {
			if cookstyleResultID != "cs-1" {
				t.Errorf("GetAutocorrectPreview id = %q, want cs-1", cookstyleResultID)
			}
			return &datastore.AutocorrectPreview{
				ID:                  "acp-1",
				CookbookID:          "cb-1",
				CookstyleResultID:   "cs-1",
				TotalOffenses:       5,
				CorrectableOffenses: 3,
				RemainingOffenses:   2,
				FilesModified:       1,
				DiffOutput:          diffOutput,
				GeneratedAt:         now,
			}, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	acPreview := body["autocorrect_preview"].(map[string]any)
	if acPreview["available"] != true {
		t.Errorf("autocorrect_preview.available = %v, want true", acPreview["available"])
	}
	if acPreview["total_offenses"] != float64(5) {
		t.Errorf("autocorrect_preview.total_offenses = %v, want 5", acPreview["total_offenses"])
	}
	if acPreview["correctable_offenses"] != float64(3) {
		t.Errorf("autocorrect_preview.correctable_offenses = %v, want 3", acPreview["correctable_offenses"])
	}
	if acPreview["remaining_offenses"] != float64(2) {
		t.Errorf("autocorrect_preview.remaining_offenses = %v, want 2", acPreview["remaining_offenses"])
	}
	if acPreview["files_modified"] != float64(1) {
		t.Errorf("autocorrect_preview.files_modified = %v, want 1", acPreview["files_modified"])
	}
	if acPreview["diff_output"] != diffOutput {
		t.Errorf("autocorrect_preview.diff_output = %q, want %q", acPreview["diff_output"], diffOutput)
	}
	if acPreview["generated_at"] == nil || acPreview["generated_at"] == "" {
		t.Error("autocorrect_preview.generated_at is empty")
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — explicit target_chef_version query param
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_ExplicitTargetVersion(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			if targetChefVersion != "17.0" {
				t.Errorf("expected target 17.0, got %q", targetChefVersion)
			}
			return nil, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return []datastore.CookbookComplexity{
				{
					CookbookID:        "cb-1",
					TargetChefVersion: "17.0",
					ComplexityScore:   10,
					ComplexityLabel:   "low",
				},
				{
					CookbookID:        "cb-1",
					TargetChefVersion: "18.0",
					ComplexityScore:   90,
					ComplexityLabel:   "critical",
				},
			}, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation?target_chef_version=17.0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if body["target_chef_version"] != "17.0" {
		t.Errorf("target_chef_version = %v, want 17.0", body["target_chef_version"])
	}
	if body["complexity_score"] != float64(10) {
		t.Errorf("complexity_score = %v, want 10", body["complexity_score"])
	}
	if body["complexity_label"] != "low" {
		t.Errorf("complexity_label = %v, want low", body["complexity_label"])
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — DB error fetching cookbooks
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_DBError_ListCookbooks(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return nil, errors.New("database connection lost")
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleCookbookRemediation_DBError_GetCookstyleResult(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			return nil, errors.New("query timeout")
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — passed cookstyle with no offenses
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_PassedCookstyle(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			return &datastore.CookstyleResult{
				ID:                "cs-1",
				CookbookID:        "cb-1",
				TargetChefVersion: "18.0",
				Passed:            true,
				OffenceCount:      0,
				Offences:          []byte(`[]`),
				ScannedAt:         time.Now().UTC(),
			}, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return []datastore.CookbookComplexity{
				{
					CookbookID:        "cb-1",
					TargetChefVersion: "18.0",
					ComplexityScore:   0,
					ComplexityLabel:   "low",
				},
			}, nil
		},
		GetAutocorrectPreviewFn: func(ctx context.Context, cookstyleResultID string) (*datastore.AutocorrectPreview, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if body["cookstyle_passed"] != true {
		t.Errorf("cookstyle_passed = %v, want true", body["cookstyle_passed"])
	}

	stats := body["statistics"].(map[string]any)
	if stats["total_offenses"] != float64(0) {
		t.Errorf("total_offenses = %v, want 0", stats["total_offenses"])
	}

	groups := body["offense_groups"].([]any)
	if len(groups) != 0 {
		t.Errorf("len(offense_groups) = %d, want 0", len(groups))
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — malformed offenses JSON is handled gracefully
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_MalformedOffensesJSON(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			return &datastore.CookstyleResult{
				ID:                "cs-1",
				CookbookID:        "cb-1",
				TargetChefVersion: "18.0",
				Passed:            false,
				OffenceCount:      1,
				Offences:          []byte(`{invalid json}`),
				ScannedAt:         time.Now().UTC(),
			}, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return nil, nil
		},
		GetAutocorrectPreviewFn: func(ctx context.Context, cookstyleResultID string) (*datastore.AutocorrectPreview, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	// Should still return 200 with empty offense groups rather than 500.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	stats := body["statistics"].(map[string]any)
	if stats["total_offenses"] != float64(0) {
		t.Errorf("total_offenses = %v, want 0 (graceful degradation)", stats["total_offenses"])
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — empty offenses array
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_EmptyOffensesArray(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			return &datastore.CookstyleResult{
				ID:                "cs-1",
				CookbookID:        "cb-1",
				TargetChefVersion: "18.0",
				Passed:            true,
				Offences:          []byte(`[]`),
				ScannedAt:         time.Now().UTC(),
			}, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return nil, nil
		},
		GetAutocorrectPreviewFn: func(ctx context.Context, cookstyleResultID string) (*datastore.AutocorrectPreview, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	groups := body["offense_groups"].([]any)
	if len(groups) != 0 {
		t.Errorf("len(offense_groups) = %d, want 0", len(groups))
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — nil Offences bytes (no JSONB data at all)
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_NilOffencesBytes(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			return &datastore.CookstyleResult{
				ID:                "cs-1",
				CookbookID:        "cb-1",
				TargetChefVersion: "18.0",
				Passed:            false,
				Offences:          nil, // No JSONB data stored
				ScannedAt:         time.Now().UTC(),
			}, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return nil, nil
		},
		GetAutocorrectPreviewFn: func(ctx context.Context, cookstyleResultID string) (*datastore.AutocorrectPreview, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	stats := body["statistics"].(map[string]any)
	if stats["total_offenses"] != float64(0) {
		t.Errorf("total_offenses = %v, want 0", stats["total_offenses"])
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — multiple cookbook versions, selects correct one
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_MultipleVersions_SelectsCorrect(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
				{ID: "cb-2", Name: "apt", Version: "2.0.0"},
				{ID: "cb-3", Name: "apt", Version: "3.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			if cookbookID != "cb-2" {
				t.Errorf("expected cookbookID=cb-2, got %q", cookbookID)
			}
			return nil, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			if cookbookID != "cb-2" {
				t.Errorf("expected cookbookID=cb-2, got %q", cookbookID)
			}
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/2.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if body["cookbook_version"] != "2.0.0" {
		t.Errorf("cookbook_version = %v, want 2.0.0", body["cookbook_version"])
	}
}

// ---------------------------------------------------------------------------
// Route integration — verify the remediation path is dispatched correctly
// and doesn't break regular cookbook detail routes
// ---------------------------------------------------------------------------

func TestCookbookRemediationRoute_DoesNotBreakDetailRoute(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: name, Version: "1.0.0"},
			}, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return nil, nil
		},
		ListCookstyleResultsForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookstyleResult, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	// The regular detail route should still work.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET /cookbooks/apt status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if body["name"] != "apt" {
		t.Errorf("detail response name = %v, want apt", body["name"])
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — autocorrect preview DB error is handled
// gracefully (returns available=false instead of 500)
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_AutocorrectPreviewDBError(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			return &datastore.CookstyleResult{
				ID:                "cs-1",
				CookbookID:        "cb-1",
				TargetChefVersion: "18.0",
				Passed:            false,
				Offences:          []byte(`[]`),
				ScannedAt:         time.Now().UTC(),
			}, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return nil, nil
		},
		GetAutocorrectPreviewFn: func(ctx context.Context, cookstyleResultID string) (*datastore.AutocorrectPreview, error) {
			return nil, errors.New("disk I/O error")
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	// Should still return 200 — autocorrect preview errors are not fatal.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	acPreview := body["autocorrect_preview"].(map[string]any)
	if acPreview["available"] != false {
		t.Errorf("autocorrect_preview.available = %v, want false", acPreview["available"])
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — unknown cop name returns nil remediation
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_UnknownCop_NilRemediation(t *testing.T) {
	offensesJSON := `[
		{
			"path": "recipes/default.rb",
			"offenses": [
				{
					"cop_name": "SomeCustom/UnknownCop",
					"severity": "convention",
					"message": "Some custom message",
					"correctable": false,
					"location": {"start_line": 1, "start_column": 1, "last_line": 1, "last_column": 10}
				}
			]
		}
	]`

	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			return &datastore.CookstyleResult{
				ID:                "cs-1",
				CookbookID:        "cb-1",
				TargetChefVersion: "18.0",
				Passed:            false,
				OffenceCount:      1,
				Offences:          []byte(offensesJSON),
				ScannedAt:         time.Now().UTC(),
			}, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return nil, nil
		},
		GetAutocorrectPreviewFn: func(ctx context.Context, cookstyleResultID string) (*datastore.AutocorrectPreview, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	groups := body["offense_groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("len(offense_groups) = %d, want 1", len(groups))
	}

	g0 := groups[0].(map[string]any)
	if g0["cop_name"] != "SomeCustom/UnknownCop" {
		t.Errorf("group[0].cop_name = %v", g0["cop_name"])
	}
	// Unknown cops should have no remediation guidance.
	if g0["remediation"] != nil {
		t.Errorf("group[0].remediation = %v, want nil for unknown cop", g0["remediation"])
	}
}

// ---------------------------------------------------------------------------
// handleCookbookRemediation — complexity error doesn't break the response
// ---------------------------------------------------------------------------

func TestHandleCookbookRemediation_ComplexityError_Graceful(t *testing.T) {
	store := &mockStore{
		ListCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.Cookbook, error) {
			return []datastore.Cookbook{
				{ID: "cb-1", Name: "apt", Version: "1.0.0"},
			}, nil
		},
		GetCookstyleResultFn: func(ctx context.Context, cookbookID, targetChefVersion string) (*datastore.CookstyleResult, error) {
			return nil, nil
		},
		ListCookbookComplexitiesForCookbookFn: func(ctx context.Context, cookbookID string) ([]datastore.CookbookComplexity, error) {
			return nil, errors.New("complexity table missing")
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0"}
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	// Should still return 200 — complexity errors are non-fatal (logged as WARN).
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Complexity fields should be zero defaults when the query fails.
	if body["complexity_score"] != float64(0) {
		t.Errorf("complexity_score = %v, want 0", body["complexity_score"])
	}
	if body["complexity_label"] != "" {
		t.Errorf("complexity_label = %v, want empty", body["complexity_label"])
	}
}
