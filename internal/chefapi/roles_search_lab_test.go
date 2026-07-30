// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package chefapi

import (
	"context"
	"os"
	"testing"
	"time"
)

// These tests run against a real Chef Infra Server. The unit tests pin what we
// believe the role index does; these pin what it actually does — the two
// properties that could not be verified from a fixture are whether the server
// caps `rows` below the request, and whether pagination boundaries duplicate
// rows.
//
// Requires a Chef server carrying the `cmm-test-*` role fixtures (see
// specifications/data-collection.md § 5.1). Configure with:
//
//	CMM_LAB_CHEF_URL=https://chef.example.com/organizations/myorg
//	CMM_LAB_CHEF_CLIENT=<client name>
//	CMM_LAB_CHEF_KEY=/path/to/client.pem
//	CMM_LAB_CHEF_INSECURE=1   # optional, for self-signed lab certs
//
//	go test ./internal/chefapi/ -tags functional -run Lab -v

func labClient(t *testing.T) *Client {
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
	client, err := NewClient(ClientConfig{
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

func labCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

// The index must return every role the /roles endpoint knows about. A shortfall
// here is exactly the silent data loss the per-role fallback exists to cover,
// so it is worth knowing whether the fallback ever actually fires.
func TestLabCollectAllRoles_MatchesTheRoleList(t *testing.T) {
	client := labClient(t)
	ctx, cancel := labCtx(t)
	defer cancel()

	names, err := client.GetRoles(ctx)
	if err != nil {
		t.Fatalf("listing roles: %v", err)
	}

	roles, err := client.CollectAllRoles(ctx, 1000, 4)
	if err != nil {
		t.Fatalf("collecting roles: %v", err)
	}

	found := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		found[r.Name] = struct{}{}
	}

	var missing []string
	for _, name := range names {
		if _, ok := found[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("index returned %d/%d roles; missing %v", len(roles), len(names), missing)
	}
	t.Logf("roles: %d listed, %d from the index", len(names), len(roles))
}

// Pagination must survive a page size far below the role count. This is the
// path that runs at customer scale, where 73,910 roles do not fit in one page.
//
// Measured 2026-07-30 against chef-server 15.10: the index returns the same
// *set* at every page size but in a *different order* each time — it applies no
// stable sort. This passes because CollectAllRoles sorts by name; without that
// it would fail, and it must not be "fixed" by relaxing to a set comparison.
func TestLabCollectAllRoles_SmallPagesMatchOnePage(t *testing.T) {
	client := labClient(t)
	ctx, cancel := labCtx(t)
	defer cancel()

	single, err := client.CollectAllRoles(ctx, 1000, 1)
	if err != nil {
		t.Fatalf("single-page collection: %v", err)
	}

	for _, pageSize := range []int{1, 2, 5, 7} {
		paged, err := client.CollectAllRoles(ctx, pageSize, 4)
		if err != nil {
			t.Fatalf("collection at pageSize=%d: %v", pageSize, err)
		}
		if len(paged) != len(single) {
			t.Errorf("pageSize=%d returned %d roles, want %d", pageSize, len(paged), len(single))
			continue
		}
		for i := range paged {
			if paged[i].Name != single[i].Name {
				t.Errorf("pageSize=%d: role %d is %q, want %q", pageSize, i, paged[i].Name, single[i].Name)
				break
			}
		}
	}
}

// Duplicates would inflate the dependency graph with repeated edges. The node
// index is known to repeat rows at page boundaries; this records whether the
// role index does too.
func TestLabCollectAllRoles_NoDuplicatesAcrossPageBoundaries(t *testing.T) {
	client := labClient(t)
	ctx, cancel := labCtx(t)
	defer cancel()

	for _, pageSize := range []int{1, 3, 5} {
		roles, err := client.CollectAllRoles(ctx, pageSize, 2)
		if err != nil {
			t.Fatalf("collection at pageSize=%d: %v", pageSize, err)
		}
		seen := map[string]int{}
		for _, r := range roles {
			seen[r.Name]++
		}
		for name, n := range seen {
			if n != 1 {
				t.Errorf("pageSize=%d: role %q returned %d times", pageSize, name, n)
			}
		}
	}
}

// The dependency graph is built from run_list and env_run_lists. Partial search
// must return the nested map and its arrays intact — this is the assumption the
// whole change rests on.
func TestLabCollectAllRoles_NestedShapesSurvivePartialSearch(t *testing.T) {
	client := labClient(t)
	ctx, cancel := labCtx(t)
	defer cancel()

	roles, err := client.CollectAllRoles(ctx, 1000, 4)
	if err != nil {
		t.Fatalf("collecting roles: %v", err)
	}

	byName := make(map[string]*RoleDetail, len(roles))
	for _, r := range roles {
		byName[r.Name] = r
	}

	fixture, ok := byName["cmm-test-env-runlists"]
	if !ok {
		t.Skip("cmm-test-env-runlists fixture not present on this server")
	}

	if len(fixture.RunList) != 2 {
		t.Errorf("run_list: got %v", fixture.RunList)
	}
	prod, ok := fixture.EnvRunLists["prod"]
	if !ok {
		t.Fatalf("env_run_lists missing 'prod': %v", fixture.EnvRunLists)
	}
	if len(prod) != 2 || prod[0] != "recipe[cmm-test::prod]" || prod[1] != "role[cmm-test-nested-child]" {
		t.Errorf("env_run_lists['prod']: got %v", prod)
	}
	if staging, ok := fixture.EnvRunLists["staging"]; !ok || len(staging) != 1 {
		t.Errorf("env_run_lists['staging']: got %v", fixture.EnvRunLists["staging"])
	}

	// A role with a genuinely empty run_list must not be confused with one
	// whose run_list failed to decode.
	if empty, ok := byName["cmm-test-empty"]; ok {
		if len(empty.RunList) != 0 || len(empty.EnvRunLists) != 0 {
			t.Errorf("cmm-test-empty: got run_list %v, env_run_lists %v", empty.RunList, empty.EnvRunLists)
		}
	}

	// A large run_list exercises row size in the index response.
	if large, ok := byName["cmm-test-large-runlist"]; ok {
		if len(large.RunList) != 60 {
			t.Errorf("cmm-test-large-runlist: got %d run_list entries, want 60", len(large.RunList))
		}
		if len(large.EnvRunLists["prod"]) != 30 {
			t.Errorf("cmm-test-large-runlist: got %d prod entries, want 30", len(large.EnvRunLists["prod"]))
		}
	}
}

// The measured claim behind the change: the index costs a fraction of the
// per-role fetch. This does not assert a threshold — lab and customer scale
// differ by three orders of magnitude — but it records both timings so a
// regression is visible.
func TestLabCollectAllRoles_IsFasterThanPerRoleFetch(t *testing.T) {
	client := labClient(t)
	ctx, cancel := labCtx(t)
	defer cancel()

	names, err := client.GetRoles(ctx)
	if err != nil {
		t.Fatalf("listing roles: %v", err)
	}

	indexStart := time.Now()
	roles, err := client.CollectAllRoles(ctx, 1000, 4)
	if err != nil {
		t.Fatalf("collecting roles: %v", err)
	}
	indexDur := time.Since(indexStart)

	perRoleStart := time.Now()
	details, errs := client.GetRolesConcurrent(ctx, names, 4)
	perRoleDur := time.Since(perRoleStart)
	if len(errs) > 0 {
		t.Logf("per-role fetch reported %d error(s)", len(errs))
	}

	t.Logf("index: %d roles in %s | per-role: %d roles in %s | speedup %.1fx",
		len(roles), indexDur.Round(time.Millisecond),
		len(details), perRoleDur.Round(time.Millisecond),
		float64(perRoleDur)/float64(indexDur))

	if indexDur > perRoleDur {
		t.Errorf("index (%s) was slower than the per-role fetch (%s)", indexDur, perRoleDur)
	}
}
