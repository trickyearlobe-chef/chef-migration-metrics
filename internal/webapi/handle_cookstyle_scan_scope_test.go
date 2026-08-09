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

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Which files a converge never executes is a decision somebody makes and can
// see — see journeys/scan-trust.md. "Can see" is the part these cover: the
// whole effective list is readable, every entry says where it came from, and
// nothing lands without a reason.

// TestScanScopeList_ShowsCuratedAndOperatorEntriesTogether — a reader must be
// able to see the entire list they are being judged by, not just the half
// somebody typed. Seeded entries and operator entries appear together, each
// saying which it is.
func TestScanScopeList_ShowsCuratedAndOperatorEntriesTogether(t *testing.T) {
	store := &mockStore{
		ListScanScopeExclusionsFn: func(context.Context) ([]datastore.ScanScopeExclusion, error) {
			return []datastore.ScanScopeExclusion{
				{Pattern: "tooling/ci/*", Excluded: true, Reason: "Invoked only by the build job."},
			}, nil
		},
	}

	r := newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/scan-scope", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp scanScopeListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	bySource := map[string]int{}
	var sawOperatorPattern bool
	for _, e := range resp.Data {
		bySource[e.Source]++
		if e.Pattern == "tooling/ci/*" {
			sawOperatorPattern = true
			if e.Source != "operator" {
				t.Errorf("an operator entry must say so, got source %q", e.Source)
			}
		}
		if e.Reason == "" {
			t.Errorf("entry %q carries no reason", e.Pattern)
		}
	}

	if !sawOperatorPattern {
		t.Error("the operator's own entry must appear in the list")
	}
	if bySource["curated"] == 0 {
		t.Error("the seeded entries must appear too, or a reader cannot see what judges them")
	}
}

// TestScanScopeList_ShowsWhenSomebodyOverturnedASeededEntry — a default that has
// been overturned must be visible as such. Silently vanishing from the list
// would leave nobody able to find the decision, let alone reverse it.
func TestScanScopeList_ShowsWhenSomebodyOverturnedASeededEntry(t *testing.T) {
	store := &mockStore{
		ListScanScopeExclusionsFn: func(context.Context) ([]datastore.ScanScopeExclusion, error) {
			return []datastore.ScanScopeExclusion{
				{Pattern: "test/*", Excluded: false, Reason: "Our converge loads shared helpers from here."},
			}, nil
		},
	}

	r := newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/scan-scope", nil))

	var resp scanScopeListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	var found *scanScopeEntry
	for i := range resp.Data {
		if resp.Data[i].Pattern == "test/*" {
			found = &resp.Data[i]
			break
		}
	}
	if found == nil {
		t.Fatal("an overturned seeded pattern must stay visible, not disappear")
	}
	if found.Excluded {
		t.Error("it must read as no longer excluded")
	}
	if found.Reason != "Our converge loads shared helpers from here." {
		t.Errorf("the reason shown must be the one somebody recorded, got %q", found.Reason)
	}
}

// TestScanScopePut_RequiresAReason is the discipline that stops this becoming
// the mechanism by which the blocked list is made to look good.
func TestScanScopePut_RequiresAReason(t *testing.T) {
	var saved bool
	store := &mockStore{
		UpsertScanScopeExclusionFn: func(context.Context, string, bool, string, string) error {
			saved = true
			return nil
		},
	}

	r := newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0"))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cookstyle/scan-scope",
		strings.NewReader(`{"pattern":"tooling/ci/*","excluded":true,"reason":"   "}`))
	r.ServeHTTP(w, withAdminSession(req))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a blank reason; body: %s", w.Code, w.Body.String())
	}
	if saved {
		t.Error("an exclusion with no recorded reason must not be saved")
	}
}

// TestScanScopePut_RejectsAnEmptyPattern — a pattern matching nothing is a
// decision that silently does nothing, which is worse than being refused.
func TestScanScopePut_RejectsAnEmptyPattern(t *testing.T) {
	r := newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0"))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cookstyle/scan-scope",
		strings.NewReader(`{"pattern":"","excluded":true,"reason":"because"}`))
	r.ServeHTTP(w, withAdminSession(req))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an empty pattern", w.Code)
	}
}

// TestScanScopePut_NonAdminForbidden — changing what counts as cookbook code
// changes every verdict in the estate, so it is not an ordinary user's button.
func TestScanScopePut_NonAdminForbidden(t *testing.T) {
	var saved bool
	store := &mockStore{
		UpsertScanScopeExclusionFn: func(context.Context, string, bool, string, string) error {
			saved = true
			return nil
		},
	}

	r := newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0"))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cookstyle/scan-scope",
		strings.NewReader(`{"pattern":"tooling/ci/*","excluded":true,"reason":"because"}`))
	r.ServeHTTP(w, withOperatorSession(req))

	if w.Code != http.StatusForbidden && w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401/403 for a non-admin", w.Code)
	}
	if saved {
		t.Error("a non-admin must not be able to change what counts as cookbook code")
	}
}

// TestScanScopePut_SavesWithTheEditorRecorded — who decided is part of the
// record, for the same reason the why is.
func TestScanScopePut_SavesWithTheEditorRecorded(t *testing.T) {
	var gotPattern, gotReason, gotBy string
	var gotExcluded bool
	store := &mockStore{
		UpsertScanScopeExclusionFn: func(_ context.Context, pattern string, excluded bool, reason, createdBy string) error {
			gotPattern, gotExcluded, gotReason, gotBy = pattern, excluded, reason, createdBy
			return nil
		},
	}

	r := newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0"))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cookstyle/scan-scope",
		strings.NewReader(`{"pattern":"tooling/ci/*","excluded":true,"reason":"Invoked only by the build job."}`))
	r.ServeHTTP(w, withAdminSession(req))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if gotPattern != "tooling/ci/*" || !gotExcluded {
		t.Errorf("saved pattern=%q excluded=%v, want tooling/ci/* true", gotPattern, gotExcluded)
	}
	if gotReason != "Invoked only by the build job." {
		t.Errorf("saved reason = %q", gotReason)
	}
	if gotBy == "" {
		t.Error("the person who decided must be recorded")
	}
}
