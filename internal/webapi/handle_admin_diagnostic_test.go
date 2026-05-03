// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/perf"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newDiagnosticRouter(store *mockStore) *Router {
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()

	sessions := auth.NewSessionManager(mockSessionStore{}, 8*time.Hour)
	mw := auth.NewMiddleware(sessions)
	localAuth := auth.NewLocalAuthenticator(mockLocalAuthStore{}, 5)

	rec := perf.NewRecorder(300*time.Second, 200, 1000)

	return NewRouter(store, cfg, hub,
		WithAuth(localAuth, sessions, mw, nil),
		WithPerformance(rec),
	)
}

// newDiagnosticRouterNoAuth returns a Router without auth middleware configured.
func newDiagnosticRouterNoAuth(store *mockStore) *Router {
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	return NewRouter(store, cfg, hub)
}

// readZipFiles reads a ZIP from body and returns a map of filename → content.
func readZipFiles(t *testing.T, body []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("readZipFiles: zip.NewReader: %v", err)
	}
	files := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("readZipFiles: open %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("readZipFiles: read %s: %v", f.Name, err)
		}
		files[f.Name] = data
	}
	return files
}

// defaultDiagnosticStore returns a mockStore with all diagnostic stubs wired.
func defaultDiagnosticStore() *mockStore {
	return &mockStore{
		ListOrganisationsFn: func(_ context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{Name: "example-org"},
			}, nil
		},
		ListCollectionRunsFilteredFn: func(_ context.Context, _ datastore.CollectionRunFilter) ([]datastore.CollectionRunWithOrg, error) {
			return []datastore.CollectionRunWithOrg{
				{
					OrganisationName: "example-org",
					Run: datastore.CollectionRun{
						OrganisationName: "example-org",
						Status:           "completed",
						StartedAt:        time.Now().UTC().Add(-1 * time.Hour),
					},
				},
			}, nil
		},
		ListAppliedMigrationsFn: func(_ context.Context) ([]datastore.AppliedMigration, error) {
			return []datastore.AppliedMigration{
				{Version: 1, Name: "initial", AppliedAt: time.Now().UTC()},
			}, nil
		},
		InventoryStatsFn: func(_ context.Context, _ bool) (datastore.InventoryStatsResult, error) {
			return datastore.InventoryStatsResult{
				NodesByOrg:        map[string]int{"example-org": 5},
				CookbooksByOrg:    map[string]int{"example-org": 3},
				RolesByOrg:        map[string]int{"example-org": 2},
				RoleDepEdgesByOrg: map[string]int{"example-org": 4},
				GitRepoCount:      1,
			}, nil
		},
		DependencyDepthStatsFn: func(_ context.Context, _ bool) (datastore.DepthStatsResult, error) {
			return datastore.DepthStatsResult{
				RoleDepDepthByOrg:     map[string]datastore.OrgDepthStats{},
				CookbookDepDepthByOrg: map[string]datastore.OrgDepthStats{},
			}, nil
		},
		ListLogEntriesFn: func(_ context.Context, _ datastore.LogEntryFilter) ([]datastore.LogEntry, error) {
			return []datastore.LogEntry{
				{
					ID:        1,
					Timestamp: time.Now().UTC(),
					Severity:  "INFO",
					Scope:     "test",
					Message:   "test log message",
				},
			}, nil
		},
		DatabaseSizeFn: func(_ context.Context) (int64, error) {
			return 1024, nil
		},
		DatabaseTableSizesFn: func(_ context.Context) ([]datastore.TableSize, error) {
			return []datastore.TableSize{}, nil
		},
		CountNodePlatformDistributionFn: func(_ context.Context, _ datastore.NodeSnapshotFilter) (map[string]int, int, error) {
			return map[string]int{
				"windows 10.0.20348": 3,
				"redhat 8.10":        2,
			}, 5, nil
		},
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestHandleDiagnosticBundle_AuthRequired(t *testing.T) {
	store := defaultDiagnosticStore()
	r := newDiagnosticRouterNoAuth(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostic-bundle", nil)
	w := httptest.NewRecorder()
	r.handleDiagnosticBundle(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleDiagnosticBundle_MethodNotAllowed(t *testing.T) {
	store := defaultDiagnosticStore()
	r := newDiagnosticRouter(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/diagnostic-bundle", nil)
	w := httptest.NewRecorder()
	r.handleDiagnosticBundle(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleDiagnosticBundle_BasicBundle(t *testing.T) {
	store := defaultDiagnosticStore()
	r := newDiagnosticRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostic-bundle", nil)
	w := httptest.NewRecorder()
	r.handleDiagnosticBundle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "cmm-diagnostic-") {
		t.Errorf("Content-Disposition = %q, expected to contain cmm-diagnostic-", cd)
	}

	files := readZipFiles(t, w.Body.Bytes())

	required := []string{
		"bundle_info.json",
		"config_summary.json",
		"performance.json",
		"system_health.json",
		"migrations.json",
		"organisations.json",
		"collection_run_status.json",
		"performance_db.json",
		"inventory_stats.json",
		"platform_distribution.json",
		"logs_recent.json",
		"logs_errors.json",
		"errors.json",
	}
	for _, name := range required {
		if _, ok := files[name]; !ok {
			t.Errorf("expected %s in ZIP but not found; files: %v", name, keys(files))
		}
	}

	if _, ok := files["dependency_depth_stats.json"]; ok {
		t.Error("dependency_depth_stats.json should NOT be in ZIP when not requested")
	}
}

func TestHandleDiagnosticBundle_WithDepthStats(t *testing.T) {
	store := defaultDiagnosticStore()
	r := newDiagnosticRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostic-bundle?include_depth_stats=true", nil)
	w := httptest.NewRecorder()
	r.handleDiagnosticBundle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	files := readZipFiles(t, w.Body.Bytes())
	if _, ok := files["dependency_depth_stats.json"]; !ok {
		t.Error("expected dependency_depth_stats.json in ZIP when include_depth_stats=true")
	}
}

func TestHandleDiagnosticBundle_SourceError(t *testing.T) {
	store := defaultDiagnosticStore()
	store.ListAppliedMigrationsFn = func(_ context.Context) ([]datastore.AppliedMigration, error) {
		return nil, errors.New("db connection refused")
	}
	r := newDiagnosticRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostic-bundle", nil)
	w := httptest.NewRecorder()
	r.handleDiagnosticBundle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even on source error, got %d", w.Code)
	}

	files := readZipFiles(t, w.Body.Bytes())

	if _, ok := files["migrations.json"]; ok {
		t.Error("migrations.json should be absent when ListAppliedMigrations errors")
	}

	errData, ok := files["errors.json"]
	if !ok {
		t.Fatal("errors.json must always be present")
	}
	var errs map[string]string
	if err := json.Unmarshal(errData, &errs); err != nil {
		t.Fatalf("errors.json is not valid JSON: %v", err)
	}
	if _, ok := errs["migrations"]; !ok {
		t.Errorf("errors.json should contain 'migrations' key; got: %v", errs)
	}
}

func TestHandleDiagnosticBundle_NoProcessOutput(t *testing.T) {
	store := defaultDiagnosticStore()
	store.ListLogEntriesFn = func(_ context.Context, _ datastore.LogEntryFilter) ([]datastore.LogEntry, error) {
		return []datastore.LogEntry{
			{
				ID:            1,
				Timestamp:     time.Now().UTC(),
				Severity:      "INFO",
				Scope:         "test",
				Message:       "test message",
				ProcessOutput: "SECRET PROCESS OUTPUT THAT MUST NOT APPEAR",
			},
		}, nil
	}
	r := newDiagnosticRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostic-bundle", nil)
	w := httptest.NewRecorder()
	r.handleDiagnosticBundle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	files := readZipFiles(t, w.Body.Bytes())

	for _, name := range []string{"logs_recent.json", "logs_errors.json"} {
		data, ok := files[name]
		if !ok {
			continue
		}
		if strings.Contains(string(data), "SECRET PROCESS OUTPUT THAT MUST NOT APPEAR") {
			t.Errorf("%s contains process_output value that should have been stripped", name)
		}
		if strings.Contains(string(data), "process_output") {
			t.Errorf("%s contains process_output key that should have been stripped", name)
		}
	}
}

func TestHandleDiagnosticBundle_OrgAnonymisation(t *testing.T) {
	store := defaultDiagnosticStore()
	store.ListOrganisationsFn = func(_ context.Context) ([]datastore.Organisation, error) {
		return []datastore.Organisation{
			{Name: "prod-org"},
		}, nil
	}
	r := newDiagnosticRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostic-bundle", nil)
	// No include_identifiers param — anonymisation should be active.
	w := httptest.NewRecorder()
	r.handleDiagnosticBundle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	files := readZipFiles(t, w.Body.Bytes())
	orgData, ok := files["organisations.json"]
	if !ok {
		t.Fatal("organisations.json not found in ZIP")
	}
	if strings.Contains(string(orgData), "prod-org") {
		t.Errorf("organisations.json should not contain real org name 'prod-org' when include_identifiers is false; got: %s", orgData)
	}
}

// ---------------------------------------------------------------------------
// Internal helper
// ---------------------------------------------------------------------------

func keys(m map[string][]byte) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// ---------------------------------------------------------------------------
// buildBundlePlatformDistribution tests
// ---------------------------------------------------------------------------

func TestBuildBundlePlatformDistribution(t *testing.T) {
	dist := map[string]int{
		"windows 10.0.20348": 100,
		"redhat 8.10":        50,
		"ubuntu 22.04":       30,
	}

	result := buildBundlePlatformDistribution(dist, 180)

	if result.TotalNodes != 180 {
		t.Errorf("TotalNodes = %d, want 180", result.TotalNodes)
	}
	if len(result.Distribution) != 3 {
		t.Fatalf("len(Distribution) = %d, want 3", len(result.Distribution))
	}

	// Should be sorted by count descending.
	if result.Distribution[0].Count != 100 {
		t.Errorf("first entry count = %d, want 100", result.Distribution[0].Count)
	}
	if result.Distribution[1].Count != 50 {
		t.Errorf("second entry count = %d, want 50", result.Distribution[1].Count)
	}

	// Check resolved display names and groups.
	for _, e := range result.Distribution {
		switch e.Platform {
		case "windows":
			if e.DisplayName != "Win Server 2022" {
				t.Errorf("windows display = %q, want %q", e.DisplayName, "Win Server 2022")
			}
			if e.GroupDisplayName != "Windows Server 2022" {
				t.Errorf("windows group = %q, want %q", e.GroupDisplayName, "Windows Server 2022")
			}
		case "redhat":
			if e.DisplayName != "RHEL 8.10" {
				t.Errorf("redhat display = %q, want %q", e.DisplayName, "RHEL 8.10")
			}
			if e.GroupDisplayName != "RHEL 8" {
				t.Errorf("redhat group = %q, want %q", e.GroupDisplayName, "RHEL 8")
			}
		case "ubuntu":
			if e.GroupDisplayName != "Ubuntu 22.04" {
				t.Errorf("ubuntu group = %q, want %q", e.GroupDisplayName, "Ubuntu 22.04")
			}
		}
	}
}

func TestBuildBundlePlatformDistribution_Empty(t *testing.T) {
	result := buildBundlePlatformDistribution(map[string]int{}, 0)
	if result.TotalNodes != 0 {
		t.Errorf("TotalNodes = %d, want 0", result.TotalNodes)
	}
	if len(result.Distribution) != 0 {
		t.Errorf("len(Distribution) = %d, want 0", len(result.Distribution))
	}
}

func TestSplitPlatformLabel(t *testing.T) {
	cases := []struct {
		label    string
		wantPlat string
		wantVer  string
	}{
		{"windows 10.0.20348", "windows", "10.0.20348"},
		{"redhat 8.10", "redhat", "8.10"},
		{"unknown", "unknown", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		plat, ver := splitPlatformLabel(tc.label)
		if plat != tc.wantPlat || ver != tc.wantVer {
			t.Errorf("splitPlatformLabel(%q) = (%q, %q), want (%q, %q)",
				tc.label, plat, ver, tc.wantPlat, tc.wantVer)
		}
	}
}
