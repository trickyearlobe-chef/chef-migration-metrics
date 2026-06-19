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

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// newStatusRouter builds a Router wired for /api/v1/admin/status tests: a stub
// DataStore, a known version, a valid collection cron (so next_run_at is
// computable), and an optional credential store.
func newStatusRouter(store *mockStore, credStore *mockCredentialStore) *Router {
	cfg := testConfig()
	cfg.Collection.Schedule = "0 * * * *"
	hub := NewEventHub()
	go hub.Run()
	opts := []RouterOption{WithVersion("test-9.9.9")}
	if credStore != nil {
		opts = append(opts, WithCredentialStore(credStore))
	}
	return NewRouter(store, cfg, hub, opts...)
}

// fullyAppliedMigrations returns an applied-migration slice the same length as
// the embedded migration set, so pending_migrations computes to zero.
func fullyAppliedMigrations() []datastore.AppliedMigration {
	n := countExpectedMigrations()
	out := make([]datastore.AppliedMigration, n)
	for i := range out {
		out[i] = datastore.AppliedMigration{Version: i + 1}
	}
	return out
}

func getAdminStatus(t *testing.T, r *Router) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/status", nil)
	r.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode status body: %v (raw: %s)", err, w.Body.String())
		}
		return w.Code, body
	}
	return w.Code, nil
}

func TestAdminStatus_MethodNotAllowed(t *testing.T) {
	r := newStatusRouter(&mockStore{}, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/status", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", w.Code)
	}
}

func TestAdminStatus_KeyNotConfigured(t *testing.T) {
	store := &mockStore{
		ListAppliedMigrationsFn: func(context.Context) ([]datastore.AppliedMigration, error) {
			return fullyAppliedMigrations(), nil
		},
	}
	r := newStatusRouter(store, nil) // no credential store

	code, body := getAdminStatus(t, r)
	if code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", code)
	}
	if body["status"] != "healthy" {
		t.Errorf("status = %v, want healthy", body["status"])
	}
	if body["version"] != "test-9.9.9" {
		t.Errorf("version = %v, want test-9.9.9", body["version"])
	}
	ds := body["datastore"].(map[string]any)
	if ds["status"] != "connected" {
		t.Errorf("datastore.status = %v, want connected", ds["status"])
	}
	if ds["pending_migrations"].(float64) != 0 {
		t.Errorf("pending_migrations = %v, want 0", ds["pending_migrations"])
	}
	cs := body["credential_storage"].(map[string]any)
	if cs["encryption_key_configured"] != false {
		t.Errorf("encryption_key_configured = %v, want false", cs["encryption_key_configured"])
	}
	if cs["total_credentials"].(float64) != 0 {
		t.Errorf("total_credentials = %v, want 0", cs["total_credentials"])
	}
	// credential_types must serialise as an object, never null.
	if _, ok := cs["credential_types"].(map[string]any); !ok {
		t.Errorf("credential_types = %v, want an object", cs["credential_types"])
	}
}

func TestAdminStatus_WithCredentials_CountsTypesAndOrphans(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "org-a-key", "generic", "secret-a")
	mustCreateCredential(t, cs, "org-b-key", "generic", "secret-b")
	mustCreateCredential(t, cs, "lonely-key", "generic", "secret-c")
	// Two are referenced by an org; lonely-key is orphaned.
	cs.AddOrgReference("org-a-key", "org-a")
	cs.AddOrgReference("org-b-key", "org-b")

	store := &mockStore{
		ListAppliedMigrationsFn: func(context.Context) ([]datastore.AppliedMigration, error) {
			return fullyAppliedMigrations(), nil
		},
	}
	r := newStatusRouter(store, cs)

	code, body := getAdminStatus(t, r)
	if code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", code)
	}
	credStorage := body["credential_storage"].(map[string]any)
	if credStorage["encryption_key_configured"] != true {
		t.Errorf("encryption_key_configured = %v, want true", credStorage["encryption_key_configured"])
	}
	if credStorage["total_credentials"].(float64) != 3 {
		t.Errorf("total_credentials = %v, want 3", credStorage["total_credentials"])
	}
	types := credStorage["credential_types"].(map[string]any)
	if types["generic"].(float64) != 3 {
		t.Errorf("credential_types[generic] = %v, want 3", types["generic"])
	}
	if credStorage["orphaned_credentials"].(float64) != 1 {
		t.Errorf("orphaned_credentials = %v, want 1", credStorage["orphaned_credentials"])
	}
}

func TestAdminStatus_OrganisationsAndCollection(t *testing.T) {
	completedAt := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)

	store := &mockStore{
		ListAppliedMigrationsFn: func(context.Context) ([]datastore.AppliedMigration, error) {
			return fullyAppliedMigrations(), nil
		},
		ListOrganisationsFn: func(context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{
				{Name: "org-file"}, // no credential → file
				{Name: "org-db", ClientKeyCredentialName: "org-db-key"}, // credential → database
				{Name: "org-never"}, // never collected
			}, nil
		},
		GetLatestCollectionRunFn: func(_ context.Context, org string) (datastore.CollectionRun, error) {
			switch org {
			case "org-file":
				return datastore.CollectionRun{
					OrganisationName: org, Status: "completed",
					StartedAt: startedAt, CompletedAt: completedAt, TotalNodes: 2000,
				}, nil
			case "org-db":
				return datastore.CollectionRun{
					OrganisationName: org, Status: "completed",
					StartedAt: startedAt, CompletedAt: completedAt, TotalNodes: 500,
				}, nil
			default:
				return datastore.CollectionRun{}, datastore.ErrNotFound
			}
		},
	}
	r := newStatusRouter(store, nil)

	code, body := getAdminStatus(t, r)
	if code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", code)
	}

	orgs := body["organisations"].([]any)
	if len(orgs) != 3 {
		t.Fatalf("organisations len = %d, want 3", len(orgs))
	}
	byName := map[string]map[string]any{}
	for _, o := range orgs {
		om := o.(map[string]any)
		byName[om["name"].(string)] = om
	}
	if byName["org-file"]["credential_source"] != "file" {
		t.Errorf("org-file credential_source = %v, want file", byName["org-file"]["credential_source"])
	}
	if byName["org-db"]["credential_source"] != "database" {
		t.Errorf("org-db credential_source = %v, want database", byName["org-db"]["credential_source"])
	}
	if byName["org-file"]["node_count"].(float64) != 2000 {
		t.Errorf("org-file node_count = %v, want 2000", byName["org-file"]["node_count"])
	}
	if byName["org-never"]["status"] != "never_collected" {
		t.Errorf("org-never status = %v, want never_collected", byName["org-never"]["status"])
	}

	coll := body["collection"].(map[string]any)
	if coll["next_run_at"] == nil {
		t.Error("collection.next_run_at should be set when a cron schedule is configured")
	}
	if coll["last_run_status"] != "completed" {
		t.Errorf("collection.last_run_status = %v, want completed", coll["last_run_status"])
	}
	if coll["last_run_at"] == nil {
		t.Error("collection.last_run_at should be set when runs exist")
	}
}

func TestAdminStatus_DegradedWhenPendingMigrations(t *testing.T) {
	store := &mockStore{
		ListAppliedMigrationsFn: func(context.Context) ([]datastore.AppliedMigration, error) {
			full := fullyAppliedMigrations()
			return full[:len(full)-1], nil // one migration unapplied
		},
	}
	r := newStatusRouter(store, nil)

	code, body := getAdminStatus(t, r)
	if code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", code)
	}
	ds := body["datastore"].(map[string]any)
	if ds["pending_migrations"].(float64) != 1 {
		t.Errorf("pending_migrations = %v, want 1", ds["pending_migrations"])
	}
	if body["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", body["status"])
	}
}

func TestAdminStatus_DegradedWhenDBDown(t *testing.T) {
	store := &mockStore{
		PingFn: func(context.Context) error { return errors.New("connection refused") },
		ListAppliedMigrationsFn: func(context.Context) ([]datastore.AppliedMigration, error) {
			return fullyAppliedMigrations(), nil
		},
	}
	r := newStatusRouter(store, nil)

	code, body := getAdminStatus(t, r)
	if code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", code)
	}
	ds := body["datastore"].(map[string]any)
	if ds["status"] != "error" {
		t.Errorf("datastore.status = %v, want error", ds["status"])
	}
	if body["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", body["status"])
	}
}
