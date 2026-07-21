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

func TestHandleNodeRuns_HappyPath(t *testing.T) {
	var gotOrg, gotNode string
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			// URL name is the CMM display name; chef slug differs.
			return datastore.Organisation{Name: name, OrgName: "chef-slug"}, nil
		},
		ListConvergeRunsForNodeFn: func(ctx context.Context, org, node string, limit int) ([]datastore.ConvergeRunView, error) {
			gotOrg, gotNode = org, node
			return []datastore.ConvergeRunView{
				{RunID: "r1", Status: "failure", Shape: "run_converge", Error: json.RawMessage(`{"class":"RuntimeError"}`)},
			}, nil
		},
	}
	r := newNodeRunsTestRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/runs/org-a/node-a.example.com", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	// The query must use the chef slug, not the display name.
	if gotOrg != "chef-slug" {
		t.Errorf("queried org = %q, want chef-slug (org.OrgName)", gotOrg)
	}
	if gotNode != "node-a.example.com" {
		t.Errorf("queried node = %q", gotNode)
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0]["run_id"] != "r1" || body.Data[0]["status"] != "failure" {
		t.Errorf("data = %v, want one failure run r1", body.Data)
	}
}

func TestHandleNodeRuns_OrgNotFound(t *testing.T) {
	store := &mockStore{
		GetOrganisationByNameFn: func(ctx context.Context, name string) (datastore.Organisation, error) {
			return datastore.Organisation{}, datastore.ErrNotFound
		},
	}
	r := newNodeRunsTestRouter(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/runs/ghost/node-a", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// newNodeRunsTestRouter builds a router with the Run events feature visible so
// the Node Detail Runs endpoint is reachable (it is gated on show_run_events).
func newNodeRunsTestRouter(store *mockStore) *Router {
	cfg := testConfig()
	on := true
	cfg.Ingest.ShowRunEvents = &on
	hub := NewEventHub()
	go hub.Run()
	return NewRouter(store, cfg, hub)
}

func TestHandleNodeRuns_MethodAndPath(t *testing.T) {
	r := newNodeRunsTestRouter(&mockStore{})
	// non-GET
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/nodes/runs/org-a/node-a", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", w.Code)
	}
	// missing node segment
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/nodes/runs/org-a", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing-node status = %d, want 400", w.Code)
	}
}
