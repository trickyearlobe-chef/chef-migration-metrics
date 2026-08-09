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

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// A poly-method cop (Lint/DeprecatedClassMethods) with a removed variant
// (File.exists?) and a deprecation-only variant (Socket.gethostbyname) must
// split into two groups: one Blocker + File.exist? guidance, one Review +
// Addrinfo guidance. See journeys/scan-trust.md.
func TestHandleCookbookRemediation_PolyMethodCop_SplitsByVariant(t *testing.T) {
	offensesJSON := `[
		{
			"path": "recipes/default.rb",
			"offenses": [
				{
					"cop_name": "Lint/DeprecatedClassMethods",
					"severity": "warning",
					"message": "` + "`Socket.gethostbyname`" + ` is deprecated in favor of ` + "`Addrinfo.getaddrinfo`" + `.",
					"correctable": true,
					"location": {"start_line": 3, "start_column": 1, "last_line": 3, "last_column": 30}
				},
				{
					"cop_name": "Lint/DeprecatedClassMethods",
					"severity": "warning",
					"message": "` + "`File.exists?`" + ` is deprecated in favor of ` + "`File.exist?`" + `.",
					"correctable": true,
					"location": {"start_line": 7, "start_column": 1, "last_line": 7, "last_column": 30}
				}
			]
		}
	]`

	now := time.Now().UTC()
	store := &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{{Name: "apt", Version: "1.0.0"}}, nil
		},
		GetServerCookbookCookstyleResultFn: func(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*datastore.ServerCookbookCookstyleResult, error) {
			return &datastore.ServerCookbookCookstyleResult{
				OrganisationName:  orgName,
				CookbookName:      cookbookName,
				CookbookVersion:   cookbookVersion,
				TargetChefVersion: "19.0",
				Passed:            false,
				OffenceCount:      2,
				Offences:          []byte(offensesJSON),
				ScannedAt:         now,
			}, nil
		},
		ListServerCookbookComplexitiesByCookbookFn: func(ctx context.Context, orgName, cookbookName, cookbookVersion string) ([]datastore.ServerCookbookComplexity, error) {
			return nil, nil
		},
		GetServerCookbookAutocorrectPreviewFn: func(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*datastore.ServerCookbookAutocorrectPreview, error) {
			return nil, nil
		},
	}
	cfg := testConfig()
	cfg.TargetChefVersion = "19.0"
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/1.0.0/remediation", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response: %v", err)
	}

	groups, ok := body["offense_groups"].([]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("expected 2 offense_groups (one per variant), got %v", body["offense_groups"])
	}

	var review, blocker map[string]any
	for _, gi := range groups {
		g := gi.(map[string]any)
		if g["cop_name"] != "Lint/DeprecatedClassMethods" {
			t.Errorf("cop_name = %v, want the base cop name", g["cop_name"])
		}
		switch g["classification"] {
		case "review":
			review = g
		case "blocker":
			blocker = g
		}
	}
	if review == nil || blocker == nil {
		t.Fatalf("expected one review and one blocker group; got %+v", groups)
	}

	// Distinct group keys so the frontend keys/collapse-state don't collide.
	if review["group_key"] == blocker["group_key"] {
		t.Errorf("variant groups share group_key %v", review["group_key"])
	}

	// Review (Socket) group: no RemovedIn, Addrinfo guidance.
	if rem, _ := review["remediation"].(map[string]any); rem != nil {
		if rp, _ := rem["replacement_pattern"].(string); !strings.Contains(rp, "Addrinfo") {
			t.Errorf("review group remediation should mention Addrinfo, got %q", rp)
		}
	} else {
		t.Error("review group has no remediation")
	}
	if _, hasRemoved := review["removed_in"]; hasRemoved {
		t.Errorf("review (deprecation-only) group should have no removed_in, got %v", review["removed_in"])
	}

	// Blocker (File.exists?) group: RemovedIn 19.0, File.exist? guidance.
	if blocker["removed_in"] != "19.0" {
		t.Errorf("blocker group removed_in = %v, want 19.0", blocker["removed_in"])
	}
	if rem, _ := blocker["remediation"].(map[string]any); rem != nil {
		if rp, _ := rem["replacement_pattern"].(string); !strings.Contains(rp, "File.exist?") {
			t.Errorf("blocker group remediation should mention File.exist?, got %q", rp)
		}
	} else {
		t.Error("blocker group has no remediation")
	}
}
