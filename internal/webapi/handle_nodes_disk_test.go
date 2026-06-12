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

// The node list must surface a node-level disk_status derived from the snapshot's
// version-invariant verdict (migration 0037), NOT from readiness rows — so it is
// correct even with no target version configured and no readiness rows at all.
// Here BulkListNodeReadinessByNodeNames returns nothing, mirroring that state.
func TestHandleNodes_DiskStatus_FromSnapshotNotReadiness(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(ctx context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "prod"}}, nil
		},
		ListNodeSnapshotsFilteredFn: func(ctx context.Context, f datastore.NodeSnapshotFilter) ([]datastore.NodeSnapshot, int, error) {
			return []datastore.NodeSnapshot{
				{OrganisationName: "prod", NodeName: "ok1", Platform: "ubuntu", SufficientDiskSpace: boolPtr(true)},
				{OrganisationName: "prod", NodeName: "low1", Platform: "ubuntu", SufficientDiskSpace: boolPtr(false)},
				{OrganisationName: "prod", NodeName: "fresh1", Platform: "ubuntu"}, // nil verdict → unknown
			}, 3, nil
		},
		// No readiness rows for any node — the disk badge must still resolve.
		BulkListNodeReadinessByNodeNamesFn: func(ctx context.Context, organisationName string, nodeNames []string) (map[string][]datastore.NodeReadiness, error) {
			return map[string][]datastore.NodeReadiness{}, nil
		},
	}

	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(store, testConfig(), hub)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var body struct {
		Data []struct {
			NodeName   string `json:"node_name"`
			DiskStatus string `json:"disk_status"`
			Readiness  []any  `json:"readiness"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := map[string]string{
		"ok1":    DiskStatusSufficient,
		"low1":   DiskStatusInsufficient,
		"fresh1": DiskStatusUnknown,
	}
	if len(body.Data) != len(want) {
		t.Fatalf("data len = %d, want %d; body = %s", len(body.Data), len(want), w.Body.String())
	}
	for _, n := range body.Data {
		if n.DiskStatus != want[n.NodeName] {
			t.Errorf("%s: disk_status = %q, want %q", n.NodeName, n.DiskStatus, want[n.NodeName])
		}
		if len(n.Readiness) != 0 {
			t.Errorf("%s: expected no readiness rows in this scenario", n.NodeName)
		}
	}
}
