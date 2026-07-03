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

func recomputeTestRouter(store *mockStore, target string) *Router {
	cfg := testConfig()
	cfg.TargetChefVersion = target
	return newTestRouterWithMockAndConfig(store, cfg)
}

// The recompute trend re-derives the rollup from fingerprints under the CURRENT
// classification. With X reclassified to blocker, a frozen fingerprint whose only
// cop is X recomputes to Blocked, and the response reports the frozen/recomputable
// boundary.
func TestDashboardCookstyleRecomputeTrend_HappyPath(t *testing.T) {
	scan := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOffenceFingerprintsByTargetFn: func(_ context.Context, target string) ([]datastore.CookstyleOffenceFingerprint, error) {
			if target != "19.3.15" {
				return nil, nil
			}
			return []datastore.CookstyleOffenceFingerprint{{
				ResultKind:        datastore.FingerprintKindServerCookbook,
				OrganisationName:  "org-a",
				CookbookName:      "cb",
				CookbookVersion:   "1.0.0",
				TargetChefVersion: "19.3.15",
				ScannedAt:         scan,
				Cops:              []datastore.FingerprintCopEntry{{CopName: "Op/X", Count: 2, Severity: "warning"}},
			}}, nil
		},
		ListCopClassificationsFn: func(_ context.Context, target string) ([]datastore.CopClassification, error) {
			return []datastore.CopClassification{{CopName: "Op/X", Classification: "blocker"}}, nil
		},
		// The cookbook is still LIVE — present in the current cookstyle result set,
		// so it survives the current-membership intersection.
		ListOrganisationsFn: func(_ context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "org-a"}}, nil
		},
		ListServerCookbookCookstyleResultsByOrganisationFn: func(_ context.Context, org string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return []datastore.ServerCookbookCookstyleResult{{
				OrganisationName:  "org-a",
				CookbookName:      "cb",
				CookbookVersion:   "1.0.0",
				TargetChefVersion: "19.3.15",
			}}, nil
		},
	}
	r := recomputeTestRouter(store, "19.3.15")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookstyle/recompute-trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		RecomputeAvailableFrom *string `json:"recompute_available_from"`
		Data                   []struct {
			TargetChefVersion string `json:"target_chef_version"`
			Source            string `json:"source"`
			CompletedAt       string `json:"completed_at"`
			TotalResults      int    `json:"total_results"`
			Ready             int    `json:"ready"`
			NeedsReview       int    `json:"needs_review"`
			Blocked           int    `json:"blocked"`
			Untested          int    `json:"untested"`
			TotalComplexity   int    `json:"total_complexity"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if body.RecomputeAvailableFrom == nil || *body.RecomputeAvailableFrom != "2026-06-01T12:00:00Z" {
		t.Errorf("recompute_available_from = %v, want 2026-06-01T12:00:00Z", body.RecomputeAvailableFrom)
	}
	if len(body.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(body.Data))
	}
	pt := body.Data[0]
	if pt.Source != "server" {
		t.Errorf("source = %q, want server", pt.Source)
	}
	if pt.Blocked != 1 || pt.TotalResults != 1 || pt.Ready != 0 || pt.NeedsReview != 0 {
		t.Errorf("point = %+v, want blocked=1 total=1", pt)
	}
	if pt.TotalComplexity == 0 {
		t.Error("expected non-zero recomputed complexity for a blocker")
	}
}

// The recompute trend separates server-cookbook results from git-repo results
// into distinct per-source series, so the dashboard can chart them apart.
func TestDashboardCookstyleRecomputeTrend_SplitsBySource(t *testing.T) {
	scan := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOffenceFingerprintsByTargetFn: func(_ context.Context, target string) ([]datastore.CookstyleOffenceFingerprint, error) {
			if target != "19.3.15" {
				return nil, nil
			}
			return []datastore.CookstyleOffenceFingerprint{
				{
					ResultKind:        datastore.FingerprintKindServerCookbook,
					OrganisationName:  "org-a",
					CookbookName:      "cb",
					CookbookVersion:   "1.0.0",
					TargetChefVersion: "19.3.15",
					ScannedAt:         scan,
					Cops:              []datastore.FingerprintCopEntry{{CopName: "Op/Block", Count: 1, Severity: "warning"}},
				},
				{
					ResultKind:        datastore.FingerprintKindGitRepo,
					GitRepoName:       "repo",
					GitRepoURL:        "https://git.example.com/repo",
					TargetChefVersion: "19.3.15",
					ScannedAt:         scan,
					Cops:              []datastore.FingerprintCopEntry{{CopName: "Op/Review", Count: 1, Severity: "warning"}},
				},
			}, nil
		},
		ListCopClassificationsFn: func(_ context.Context, _ string) ([]datastore.CopClassification, error) {
			return []datastore.CopClassification{
				{CopName: "Op/Block", Classification: "blocker"},
				{CopName: "Op/Review", Classification: "review"},
			}, nil
		},
		ListOrganisationsFn: func(_ context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "org-a"}}, nil
		},
		ListServerCookbookCookstyleResultsByOrganisationFn: func(_ context.Context, _ string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return []datastore.ServerCookbookCookstyleResult{{
				OrganisationName: "org-a", CookbookName: "cb", CookbookVersion: "1.0.0", TargetChefVersion: "19.3.15",
			}}, nil
		},
		ListGitRepoCookstyleResultsByTargetVersionFn: func(_ context.Context, _ string) ([]datastore.GitRepoCookstyleResult, error) {
			return []datastore.GitRepoCookstyleResult{{
				GitRepoName: "repo", GitRepoURL: "https://git.example.com/repo", TargetChefVersion: "19.3.15",
			}}, nil
		},
	}
	r := recomputeTestRouter(store, "19.3.15")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookstyle/recompute-trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", w.Code, http.StatusOK, w.Body.String())
	}
	var body struct {
		Data []struct {
			Source      string `json:"source"`
			Ready       int    `json:"ready"`
			NeedsReview int    `json:"needs_review"`
			Blocked     int    `json:"blocked"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	bySource := map[string]struct {
		ready, review, blocked int
	}{}
	for _, p := range body.Data {
		s := bySource[p.Source]
		s.ready += p.Ready
		s.review += p.NeedsReview
		s.blocked += p.Blocked
		bySource[p.Source] = s
	}
	if len(bySource) != 2 {
		t.Fatalf("got sources %v, want exactly server + git", bySource)
	}
	if bySource["server"].blocked != 1 || bySource["server"].review != 0 {
		t.Errorf("server series = %+v, want blocked=1", bySource["server"])
	}
	if bySource["git"].review != 1 || bySource["git"].blocked != 0 {
		t.Errorf("git series = %+v, want needs_review=1", bySource["git"])
	}
}

// A result that still has fingerprint history but is no longer in the live
// cookstyle result set (cookbook deleted) MUST be excluded from the recomputed
// series — recompute is bounded to current membership.
func TestDashboardCookstyleRecomputeTrend_ExcludesRemovedResult(t *testing.T) {
	scan := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		ListOffenceFingerprintsByTargetFn: func(_ context.Context, target string) ([]datastore.CookstyleOffenceFingerprint, error) {
			return []datastore.CookstyleOffenceFingerprint{{
				ResultKind:        datastore.FingerprintKindServerCookbook,
				OrganisationName:  "org-a",
				CookbookName:      "removed-cb",
				CookbookVersion:   "1.0.0",
				TargetChefVersion: "19.3.15",
				ScannedAt:         scan,
				Cops:              []datastore.FingerprintCopEntry{{CopName: "Op/X", Count: 1, Severity: "warning"}},
			}}, nil
		},
		// Live membership is determinable (orgs load) but the cookbook is GONE from
		// the current result set → it must not contribute any recomputed point.
		ListOrganisationsFn: func(_ context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "org-a"}}, nil
		},
		ListServerCookbookCookstyleResultsByOrganisationFn: func(_ context.Context, _ string) ([]datastore.ServerCookbookCookstyleResult, error) {
			return nil, nil // no live results
		},
	}
	r := recomputeTestRouter(store, "19.3.15")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookstyle/recompute-trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 0 {
		t.Errorf("len(data) = %d, want 0 (removed result excluded)", len(body.Data))
	}
}

// No fingerprint history → empty series and a null boundary (still the frozen era).
func TestDashboardCookstyleRecomputeTrend_NoData(t *testing.T) {
	store := &mockStore{} // ListOffenceFingerprintsByTargetFn nil → returns nil
	r := recomputeTestRouter(store, "19.3.15")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/cookstyle/recompute-trend", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		RecomputeAvailableFrom *string           `json:"recompute_available_from"`
		Data                   []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.RecomputeAvailableFrom != nil {
		t.Errorf("recompute_available_from = %v, want null", *body.RecomputeAvailableFrom)
	}
	if len(body.Data) != 0 {
		t.Errorf("len(data) = %d, want 0", len(body.Data))
	}
}

// POST is rejected (route exists, method-checked).
func TestDashboardCookstyleRecomputeTrend_MethodNotAllowed(t *testing.T) {
	r := testRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/cookstyle/recompute-trend", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}
