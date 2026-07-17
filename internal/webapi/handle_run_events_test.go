// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

func newRunEventsTestRouter(store *mockStore) *Router {
	cfg := testConfig()
	on := true
	cfg.Ingest.ShowRunEvents = &on // feature visible for these tests
	hub := NewEventHub()
	go hub.Run()
	return NewRouter(store, cfg, hub)
}

// The Run events surfaces are hidden (404) when ingest.show_run_events is off —
// the feature is dormant in reserve, not just missing from the nav.
func TestRunEvents_HiddenWhenDisabled(t *testing.T) {
	store := &mockStore{
		ListConvergeRunNodesFilteredFn: func(context.Context, datastore.ConvergeRunFilter) ([]datastore.ConvergeRunListItem, int, error) {
			t.Fatal("datastore must not be queried when the feature is off")
			return nil, 0, nil
		},
	}
	cfg := testConfig() // ShowRunEvents nil → false (default)
	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(store, cfg, hub)

	for _, path := range []string{
		"/api/v1/run-events/nodes",
		"/api/v1/run-events/runs",
		"/api/v1/run-events/nodes/org/node",
		"/api/v1/filters/run-organisations",
		"/api/v1/filters/run-chef-versions",
		"/api/v1/nodes/runs/org/node",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404 when feature disabled", path, w.Code)
		}
	}
}

// The Nodes tab passes view-level filters (incl. the as-of `until` anchor)
// straight into the datastore filter, and returns a paginated envelope.
func TestHandleRunEventNodes_FilterPassthroughAndEnvelope(t *testing.T) {
	var got datastore.ConvergeRunFilter
	store := &mockStore{
		ListConvergeRunNodesFilteredFn: func(_ context.Context, f datastore.ConvergeRunFilter) ([]datastore.ConvergeRunListItem, int, error) {
			got = f
			return []datastore.ConvergeRunListItem{
				{Organisation: "dmz-org", NodeName: "dmz01", Status: "failure", ChefVersion: "19.0.12"},
			}, 1, nil
		},
	}
	r := newRunEventsTestRouter(store)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/run-events/nodes?organisation=dmz-org&status=failure&chef_version=19.0.12&cookbook=prereq&failure_message=not+enough+space&until=2026-07-17T00:00:00Z&page=2&per_page=25", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got.Organisation != "dmz-org" || got.Status != "failure" || got.ChefVersion != "19.0.12" {
		t.Errorf("filter passthrough wrong: %+v", got)
	}
	if got.Cookbook != "prereq" || got.FailureMessage != "not enough space" {
		t.Errorf("cookbook/failure_message passthrough wrong: %+v", got)
	}
	if got.EndTimeTo.IsZero() {
		t.Errorf("as-of `until` anchor not parsed into EndTimeTo")
	}
	if got.Limit != 25 || got.Offset != 25 {
		t.Errorf("pagination wrong: limit=%d offset=%d, want 25/25", got.Limit, got.Offset)
	}

	var resp struct {
		Data       []map[string]any `json:"data"`
		Pagination map[string]any   `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 || resp.Pagination["total_items"].(float64) != 1 {
		t.Errorf("envelope wrong: %+v", resp)
	}
}

// The node detail path keys on the DELIVERED org name directly — NO
// organisations-table resolution — so an ingest-only DMZ org resolves.
func TestHandleRunEventNodeDetail_UsesDeliveredOrgNoOrgTable(t *testing.T) {
	var gotOrg, gotNode string
	orgResolverCalled := false
	store := &mockStore{
		ListConvergeRunsForNodeFn: func(_ context.Context, org, node string, _ int) ([]datastore.ConvergeRunView, error) {
			gotOrg, gotNode = org, node
			return []datastore.ConvergeRunView{{RunID: "r1", Status: "failure"}}, nil
		},
		GetOrganisationByNameFn: func(_ context.Context, _ string) (datastore.Organisation, error) {
			orgResolverCalled = true
			return datastore.Organisation{}, datastore.ErrNotFound
		},
	}
	r := newRunEventsTestRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/run-events/nodes/dmz-org/dmz01.example.com", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if gotOrg != "dmz-org" || gotNode != "dmz01.example.com" {
		t.Errorf("detail keyed wrong: org=%q node=%q", gotOrg, gotNode)
	}
	if orgResolverCalled {
		t.Errorf("detail must NOT resolve via the organisations table (breaks DMZ orgs)")
	}
}

// The viewer /features endpoint reports run_events per the live config, so the
// frontend can hide the nav entry + Node Detail Runs tab.
func TestHandleFeatures_ReflectsShowRunEvents(t *testing.T) {
	check := func(show bool, want bool) {
		cfg := testConfig()
		cfg.Ingest.ShowRunEvents = &show
		hub := NewEventHub()
		go hub.Run()
		r := NewRouter(&mockStore{}, cfg, hub)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/features", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var resp struct {
			RunEvents bool `json:"run_events"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.RunEvents != want {
			t.Errorf("run_events = %v, want %v", resp.RunEvents, want)
		}
	}
	check(true, true)
	check(false, false)
}

// Org filter options come from converge_runs, not the organisations table.
func TestHandleFilterRunOrganisations(t *testing.T) {
	store := &mockStore{
		ListConvergeRunOrganisationsFn: func(_ context.Context) ([]string, error) {
			return []string{"clustered-org", "dmz-org"}, nil
		},
	}
	r := newRunEventsTestRouter(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/filters/run-organisations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []string `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 2 || resp.Data[1] != "dmz-org" {
		t.Errorf("org options wrong: %v", resp.Data)
	}
}
