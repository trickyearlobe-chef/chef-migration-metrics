// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package collector

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
)

// End-to-end over the role pipeline against a real Chef Infra Server: search
// index → gap fill → dependency graph. No datastore is involved, so this runs
// without the dev database.
//
//	CMM_LAB_CHEF_URL=https://chef.example.com/organizations/myorg
//	CMM_LAB_CHEF_CLIENT=<client name>
//	CMM_LAB_CHEF_KEY=/path/to/client.pem
//	CMM_LAB_CHEF_INSECURE=1   # optional, for self-signed lab certs
//
//	go test ./internal/collector/ -tags functional -run Lab -v

func labRoleClient(t *testing.T) *chefapi.Client {
	t.Helper()

	serverURL := os.Getenv("CMM_LAB_CHEF_URL")
	clientName := os.Getenv("CMM_LAB_CHEF_CLIENT")
	keyPath := os.Getenv("CMM_LAB_CHEF_KEY")
	if serverURL == "" || clientName == "" || keyPath == "" {
		t.Skip("set CMM_LAB_CHEF_URL, CMM_LAB_CHEF_CLIENT and CMM_LAB_CHEF_KEY to run lab tests")
	}

	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("reading client key: %v", err)
	}

	verify := os.Getenv("CMM_LAB_CHEF_INSECURE") == ""
	client, err := chefapi.NewClient(chefapi.ClientConfig{
		ServerURL:     serverURL,
		ClientName:    clientName,
		PrivateKeyPEM: key,
		AppVersion:    "lab-test",
		SSLVerify:     &verify,
	})
	if err != nil {
		t.Fatalf("creating lab client: %v", err)
	}
	return client
}

// A healthy index should answer in full, leaving the per-role fallback idle.
// If FromFallback is non-zero against a quiet lab server, the index is not
// returning everything and the customer-scale assumption needs revisiting.
func TestLabCollectRoleDetails_IndexAnswersInFull(t *testing.T) {
	client := labRoleClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	names, err := client.GetRoles(ctx)
	if err != nil {
		t.Fatalf("listing roles: %v", err)
	}

	result, err := collectRoleDetails(ctx, client, 1000, 4)
	if err != nil {
		t.Fatalf("collecting role details: %v", err)
	}

	if len(result.Roles) != len(names) {
		t.Errorf("collected %d roles, /roles lists %d", len(result.Roles), len(names))
	}
	if result.FromFallback != 0 {
		t.Errorf("expected the index to answer in full, %d role(s) needed the per-role fallback", result.FromFallback)
	}
	if len(result.Warnings()) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings())
	}
	t.Logf("%d roles: %d from the index, %d per-role", len(result.Roles), result.FromIndex, result.FromFallback)
}

// The point of collecting roles at all. Verifies the graph built from real
// index data contains the edges the cmm-test fixtures were created to produce,
// including one that only exists inside env_run_lists.
func TestLabCollectRoleDetails_BuildsExpectedGraphEdges(t *testing.T) {
	client := labRoleClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := collectRoleDetails(ctx, client, 1000, 4)
	if err != nil {
		t.Fatalf("collecting role details: %v", err)
	}

	deps := BuildRoleDependencies("lab-org", result.Roles)

	type edge struct{ role, depType, depName string }
	got := make(map[edge]bool, len(deps))
	for _, d := range deps {
		got[edge{d.RoleName, d.DependencyType, d.DependencyName}] = true
	}

	want := []edge{
		// Nested role reference from the default run_list.
		{"cmm-test-nested-parent", "role", "cmm-test-nested-child"},
		{"cmm-test-nested-parent", "cookbook", "cmm-test"},
		// Leaf role, including a bare `recipe[cookbook]` entry.
		{"cmm-test-nested-child", "cookbook", "cmm-test"},
		{"cmm-test-nested-child", "cookbook", "cmm-test-other"},
		// This edge exists ONLY in env_run_lists — if partial search dropped
		// the nested map, this is the assertion that catches it.
		{"cmm-test-env-runlists", "role", "cmm-test-nested-child"},
		{"cmm-test-env-runlists", "cookbook", "cmm-test"},
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing edge: %s -> %s:%s", w.role, w.depType, w.depName)
		}
	}

	// The empty-run-list fixture must contribute nothing rather than a
	// phantom edge.
	for e := range got {
		if e.role == "cmm-test-empty" {
			t.Errorf("cmm-test-empty produced an edge: %v", e)
		}
	}

	// The large fixture's run_list and env_run_lists overlap by cookbook name,
	// so the de-dup inside BuildRoleDependencies should leave 60 distinct
	// cookbook edges, not 90.
	large := 0
	for e := range got {
		if e.role == "cmm-test-large-runlist" {
			large++
		}
	}
	if large != 60 {
		t.Errorf("cmm-test-large-runlist produced %d edges, want 60", large)
	}

	t.Logf("built %d edges from %d roles", len(deps), len(result.Roles))
}
