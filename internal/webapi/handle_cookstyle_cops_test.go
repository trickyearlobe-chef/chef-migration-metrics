// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// GET /api/v1/cookstyle/cops — basic aggregation
// ---------------------------------------------------------------------------

func TestHandleCookstyleCops_BasicAggregation(t *testing.T) {
	offences := mustMarshalCops(t, []map[string]any{
		{
			"path": "recipes/default.rb",
			"offenses": []map[string]any{
				{"cop_name": "Chef/Deprecations/NodeSet", "severity": "warning", "corrected": true},
				{"cop_name": "Chef/Deprecations/NodeSet", "severity": "warning", "corrected": false},
				{"cop_name": "Lint/DeprecatedClassMethods", "severity": "warning", "corrected": true},
			},
		},
	})

	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(_ context.Context, tv string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return []datastore.ServerCookbookCookstyleResult{
				{
					CookbookName:      "cb-alpha",
					CookbookVersion:   "1.0.0",
					OrganisationName:  "example-org",
					TargetChefVersion: tv,
					Passed:            false,
					OffenceCount:      3,
					Offences:          offences,
					ScannedAt:         time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC),
				},
			}, nil
		},
		ListGitRepoCookstyleResultsByTargetVersionFn: func(_ context.Context, _ string) ([]datastore.GitRepoCookstyleResult, error) {
			return nil, nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/cops?target_chef_version=18.0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp copAggregationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 cop items, got %d", len(resp.Data))
	}

	// Find NodeSet item.
	var nodeSet *copAggregateItem
	for i := range resp.Data {
		if resp.Data[i].CopName == "Chef/Deprecations/NodeSet" {
			nodeSet = &resp.Data[i]
			break
		}
	}
	if nodeSet == nil {
		t.Fatal("Chef/Deprecations/NodeSet not found in response")
	}

	if nodeSet.TotalOffences != 2 {
		t.Errorf("total_offences = %d, want 2", nodeSet.TotalOffences)
	}
	if nodeSet.CookbooksAffected != 1 {
		t.Errorf("cookbooks_affected = %d, want 1", nodeSet.CookbooksAffected)
	}
	if nodeSet.AutoCorrectablePct != 50 {
		t.Errorf("auto_correctable_pct = %v, want 50", nodeSet.AutoCorrectablePct)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/cookstyle/cops — classification filter
// ---------------------------------------------------------------------------

func TestHandleCookstyleCops_ClassificationFilter(t *testing.T) {
	offences := mustMarshalCops(t, []map[string]any{
		{
			"path": "recipes/default.rb",
			"offenses": []map[string]any{
				{"cop_name": "Chef/Deprecations/NodeSet", "severity": "warning", "corrected": false},
				{"cop_name": "Chef/Style/FileMode", "severity": "convention", "corrected": false},
			},
		},
	})

	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(_ context.Context, _ string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return []datastore.ServerCookbookCookstyleResult{
				{
					CookbookName:      "cb-a",
					CookbookVersion:   "1.0.0",
					TargetChefVersion: "18.0",
					Offences:          offences,
					ScannedAt:         time.Now(),
				},
			}, nil
		},
		ListGitRepoCookstyleResultsByTargetVersionFn: func(_ context.Context, _ string) ([]datastore.GitRepoCookstyleResult, error) {
			return nil, nil
		},
		// Chef/Deprecations/NodeSet is in curated defaults as blocker.
		// Chef/Style/FileMode is in curated defaults as noise.
		ListCopClassificationsFn: func(_ context.Context, _ string) ([]datastore.CopClassification, error) {
			return nil, nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/cops?target_chef_version=18.0&classification=noise", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}

	var resp copAggregationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Only noise cops should appear in data.
	for _, item := range resp.Data {
		if item.Classification != "noise" {
			t.Errorf("item %s has classification %s, expected noise", item.CopName, item.Classification)
		}
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/cookstyle/cops — missing target version
// ---------------------------------------------------------------------------

func TestHandleCookstyleCops_MissingTargetVersion(t *testing.T) {
	r := newTestRouterWithMockAndConfig(&mockStore{}, testConfigNoVersions())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/cops", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/cookstyle/cops/:cop/cookbooks — drill-down
// ---------------------------------------------------------------------------

func TestHandleCookstyleCopCookbooks_DrillDown(t *testing.T) {
	offences := mustMarshalCops(t, []map[string]any{
		{
			"path": "recipes/default.rb",
			"offenses": []map[string]any{
				{"cop_name": "Chef/Deprecations/NodeSet", "severity": "warning", "corrected": true},
				{"cop_name": "Chef/Deprecations/NodeSet", "severity": "warning", "corrected": false},
				{"cop_name": "Chef/Style/FileMode", "severity": "convention", "corrected": false},
			},
		},
	})

	store := &mockStore{
		ListAllServerCookbookCookstyleResultsByTargetVersionFn: func(_ context.Context, _ string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return []datastore.ServerCookbookCookstyleResult{
				{
					CookbookName:      "cb-one",
					CookbookVersion:   "2.0.0",
					OrganisationName:  "example-org",
					TargetChefVersion: "18.0",
					Offences:          offences,
					ScannedAt:         time.Now(),
				},
			}, nil
		},
		ListGitRepoCookstyleResultsByTargetVersionFn: func(_ context.Context, _ string) ([]datastore.GitRepoCookstyleResult, error) {
			return nil, nil
		},
		ListCopClassificationsFn: func(_ context.Context, _ string) ([]datastore.CopClassification, error) {
			return nil, nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/cops/Chef/Deprecations/NodeSet/cookbooks?target_chef_version=18.0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}

	var resp copCookbookResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.CopName != "Chef/Deprecations/NodeSet" {
		t.Errorf("cop_name = %q, want Chef/Deprecations/NodeSet", resp.CopName)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Data))
	}
	if resp.Data[0].OffenceCount != 2 {
		t.Errorf("offence_count = %d, want 2", resp.Data[0].OffenceCount)
	}
	if resp.Data[0].AutoCorrectable != 1 {
		t.Errorf("auto_correctable = %d, want 1", resp.Data[0].AutoCorrectable)
	}
}

// ---------------------------------------------------------------------------
// PUT /api/v1/cookstyle/cops/:cop/classification
// ---------------------------------------------------------------------------

func TestHandleCookstyleCopClassification_Put(t *testing.T) {
	var savedCop, savedClass, savedReason string
	store := &mockStore{
		UpsertCopClassificationFn: func(_ context.Context, copName, tv, class, reason, _ string) error {
			savedCop = copName
			savedClass = class
			savedReason = reason
			return nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	body := `{"target_chef_version":"18.0","classification":"blocker","reason":"crashes at runtime"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cookstyle/cops/Lint/DeprecatedClassMethods/classification", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	if savedCop != "Lint/DeprecatedClassMethods" {
		t.Errorf("cop_name = %q, want Lint/DeprecatedClassMethods", savedCop)
	}
	if savedClass != "blocker" {
		t.Errorf("classification = %q, want blocker", savedClass)
	}
	if savedReason != "crashes at runtime" {
		t.Errorf("reason = %q, want 'crashes at runtime'", savedReason)
	}
}

func TestHandleCookstyleCopClassification_Put_TriggersPropagationAndAudit(t *testing.T) {
	var auditAction, auditCop string
	store := &mockStore{
		UpsertCopClassificationFn: func(_ context.Context, copName, tv, class, reason, _ string) error {
			return nil
		},
		InsertCookstyleAuditEntryFn: func(_ context.Context, p datastore.InsertCookstyleAuditParams) error {
			auditAction = p.Action
			auditCop = p.CopName
			return nil
		},
	}

	cfg := testConfigWithTargetVersions("18")
	r := newTestRouterWithMockAndConfig(store, cfg)

	// Wire a propagator over a mock closure: one server cookbook carries the cop
	// and is currently passing; reclassifying to blocker must flip + re-score.
	propStore := &mockPropagationStore{
		serverRefs: []datastore.CookstyleResultRef{
			{OrganisationName: "org-a", CookbookName: "web", CookbookVersion: "1.0.0", TargetChefVersion: "18", Offences: offJSONForCop("Chef/Style/Foo", "warning"), Passed: true},
		},
		classifications: map[string][]datastore.CopClassification{
			"18": {{CopName: "Chef/Style/Foo", Classification: "blocker"}},
		},
	}
	scorer := &mockComplexityRescorer{}
	readiness := &mockReadinessRecomputer{}
	r.cookstylePropagator = NewCookstylePropagator(propStore, scorer, readiness, defaultRulesFn, nil)

	w := httptest.NewRecorder()
	body := `{"target_chef_version":"18","classification":"blocker","reason":"breaks"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cookstyle/cops/Chef/Style/Foo/classification", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	if len(propStore.serverPassedUpdates) != 1 || propStore.serverPassedUpdates[0].passed != false {
		t.Errorf("expected verdict flip to false, got %+v", propStore.serverPassedUpdates)
	}
	if len(scorer.serverCalls) != 1 {
		t.Errorf("expected complexity re-score, got %d calls", len(scorer.serverCalls))
	}
	if len(readiness.orgs) != 1 || readiness.orgs[0] != "org-a" {
		t.Errorf("expected readiness recompute for org-a, got %v", readiness.orgs)
	}
	if auditAction != "cop_reclassified" || auditCop != "Chef/Style/Foo" {
		t.Errorf("audit = (%q,%q), want (cop_reclassified, Chef/Style/Foo)", auditAction, auditCop)
	}
}

func TestHandleCookstyleCopClassification_NonAdminForbidden(t *testing.T) {
	var upserted bool
	store := &mockStore{
		UpsertCopClassificationFn: func(_ context.Context, _, _, _, _, _ string) error {
			upserted = true
			return nil
		},
	}
	cfg := testConfigWithTargetVersions("18")
	r := newTestRouterWithMockAndConfig(store, cfg)

	for _, role := range []string{"viewer", "operator"} {
		w := httptest.NewRecorder()
		body := `{"target_chef_version":"18","classification":"blocker"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/cookstyle/cops/Chef/Style/Foo/classification", strings.NewReader(body))
		req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.SessionInfo{Username: "u", Role: role}))
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("role %q: status = %d, want 403; body: %s", role, w.Code, w.Body.String())
		}
	}
	if upserted {
		t.Error("non-admin reclassification must not reach the datastore")
	}
}

func TestHandleCookstyleCopClassification_AdminAllowed(t *testing.T) {
	var upserted bool
	store := &mockStore{
		UpsertCopClassificationFn: func(_ context.Context, _, _, _, _, _ string) error {
			upserted = true
			return nil
		},
	}
	cfg := testConfigWithTargetVersions("18")
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	body := `{"target_chef_version":"18","classification":"blocker"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cookstyle/cops/Chef/Style/Foo/classification", strings.NewReader(body))
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.SessionInfo{Username: "admin", Role: "admin"}))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("admin: status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if !upserted {
		t.Error("admin reclassification should reach the datastore")
	}
}

func TestHandleCookstyleCopClassification_InvalidClassification(t *testing.T) {
	store := &mockStore{}
	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	body := `{"target_chef_version":"18.0","classification":"unknown"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cookstyle/cops/Lint/Foo/classification", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/cookstyle/cops/:cop/classification
// ---------------------------------------------------------------------------

func TestHandleCookstyleCopClassification_Delete(t *testing.T) {
	var deletedCop, deletedVersion string
	store := &mockStore{
		DeleteCopClassificationFn: func(_ context.Context, copName, tv string) error {
			deletedCop = copName
			deletedVersion = tv
			return nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cookstyle/cops/Chef/Deprecations/NodeSet/classification?target_chef_version=18.0", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	if deletedCop != "Chef/Deprecations/NodeSet" {
		t.Errorf("cop = %q, want Chef/Deprecations/NodeSet", deletedCop)
	}
	if deletedVersion != "18.0" {
		t.Errorf("version = %q, want 18.0", deletedVersion)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/cookstyle/custom-cops
// ---------------------------------------------------------------------------

func TestHandleCookstyleCustomCops_Create(t *testing.T) {
	var created datastore.CustomCopDefinition
	store := &mockStore{
		CreateCustomCopDefinitionFn: func(_ context.Context, d datastore.CustomCopDefinition) (string, error) {
			created = d
			return "generated-uuid", nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	body := `{"cop_name":"Custom/Ruby3/NilMatch","description":"nil.=~ removed","pattern_type":"regex","pattern":"=~","file_glob":"*.rb","classification":"blocker","enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookstyle/custom-cops", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	if created.CopName != "Custom/Ruby3/NilMatch" {
		t.Errorf("cop_name = %q, want Custom/Ruby3/NilMatch", created.CopName)
	}
	if created.PatternType != "regex" {
		t.Errorf("pattern_type = %q, want regex", created.PatternType)
	}
}

func TestHandleCookstyleCustomCops_Create_ValidationError(t *testing.T) {
	store := &mockStore{}
	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	// Missing "Custom/" prefix
	body := `{"cop_name":"NilMatch","description":"test","pattern_type":"regex","pattern":"=~","classification":"blocker"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookstyle/custom-cops", strings.NewReader(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/cookstyle/custom-cops
// ---------------------------------------------------------------------------

func TestHandleCookstyleCustomCops_List(t *testing.T) {
	store := &mockStore{
		ListCustomCopDefinitionsFn: func(_ context.Context) ([]datastore.CustomCopDefinition, error) {
			return []datastore.CustomCopDefinition{
				{ID: "id-1", CopName: "Custom/Test/One", Description: "test"},
			}, nil
		},
	}

	cfg := testConfigWithTargetVersions("18.0")
	r := newTestRouterWithMockAndConfig(store, cfg)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/custom-cops", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Path extraction helpers
// ---------------------------------------------------------------------------

func TestExtractCopNameFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/cookstyle/cops/Chef/Deprecations/NodeSet/cookbooks", "Chef/Deprecations/NodeSet"},
		{"/api/v1/cookstyle/cops/Lint/DeprecatedClassMethods/cookbooks", "Lint/DeprecatedClassMethods"},
		{"/api/v1/cookstyle/cops/cookbooks", ""},
		{"/other/path", ""},
	}
	for _, tt := range tests {
		got := extractCopNameFromPath(tt.path)
		if got != tt.want {
			t.Errorf("extractCopNameFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestExtractCopNameFromClassificationPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/cookstyle/cops/Chef/Deprecations/NodeSet/classification", "Chef/Deprecations/NodeSet"},
		{"/api/v1/cookstyle/cops/Lint/Foo/classification", "Lint/Foo"},
		{"/api/v1/cookstyle/cops/classification", ""},
	}
	for _, tt := range tests {
		got := extractCopNameFromClassificationPath(tt.path)
		if got != tt.want {
			t.Errorf("extractCopNameFromClassificationPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustMarshalCops(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

func testConfigNoVersions() *config.Config {
	wsEnabled := true
	cfg := &config.Config{}
	cfg.Server.WebSocket.Enabled = &wsEnabled
	return cfg
}
