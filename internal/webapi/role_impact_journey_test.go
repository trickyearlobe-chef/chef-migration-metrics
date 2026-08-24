//go:build journey

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

// The journey suite for journeys/role-impact.md. Run it with `make journey`.
//
// One test per thing the journey says has to be in place. A green one is
// built; a red one is not yet. Running it recomputes the todo list, so nobody
// has to remember to keep one true.
//
// It is deliberately OUTSIDE the gating suite. Red is the normal state for
// most of a journey's life and must never stop a build.
//
// Two rules, same as every other suite here:
//
//   - Assert the real thing, so building the feature turns the test green
//     with no edit. A test that says "not implemented" has to be rewritten by
//     the person it was meant to help.
//   - Name the journey line it comes from, in the journey's words.
//
// A note on what these can and cannot reach. The counting itself — how many
// machines carry a role, and the worst-of roll-up that decides a role's
// status — is derived in SQL (internal/datastore/role_summary_recompute.go,
// internal/datastore/role_filter.go) and needs a database to exercise. What
// is asserted from here is that the answer reaches the reader with the parts
// the journey asks for in it, which is a different claim and is worded as
// one.
//
// This is not where regressions go. Something that used to work and now
// fails is a broken build, not a todo.

// roleImpactRouter builds a router over a store the test controls. Named for
// this journey so it cannot collide with the shared journeyRouter.
func roleImpactRouter(store *mockStore) *Router {
	return newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0"))
}

// roleImpactGet issues a signed-in GET and returns the recorder.
func roleImpactGet(t *testing.T, store *mockStore, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	roleImpactRouter(store).ServeHTTP(w, withAdminSession(httptest.NewRequest(http.MethodGet, path, nil)))
	return w
}

// "Which roles are ready and which are not"
func TestJourney_WhichRolesAreReadyAndWhichAreNot(t *testing.T) {
	store := &mockStore{
		ListRolesFilteredFn: func(_ context.Context, _ datastore.RoleFilter) ([]datastore.RoleFilterRow, int, datastore.RoleFilterSummary, error) {
			return []datastore.RoleFilterRow{
					{RoleName: "base", CompatibilityStatus: "incompatible", IncompatibleCount: 4},
					{RoleName: "webserver", CompatibilityStatus: "compatible", CompatibleCount: 9},
				}, 2, datastore.RoleFilterSummary{
					CompatibleRoles: 1, IncompatibleRoles: 1, TotalRoles: 2,
				}, nil
		},
	}
	w := roleImpactGet(t, store, "/api/v1/roles")
	if w.Code != http.StatusOK {
		t.Fatalf("listing roles answered %d: %s", w.Code, w.Body.String())
	}
	var body roleListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the role list: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("the role list came back with %d roles, want 2", len(body.Data))
	}
	got := map[string]string{}
	for _, role := range body.Data {
		got[role.RoleName] = role.CompatibilityStatus
	}
	if got["base"] != "incompatible" || got["webserver"] != "compatible" {
		t.Errorf("the list does not say which roles are ready and which are not: %v", got)
	}
	if body.Summary.IncompatibleRoles != 1 {
		t.Errorf("the list does not say how many roles are blocked (got %d), so the size of "+
			"the job can only be found by paging through it", body.Summary.IncompatibleRoles)
	}
}

// "knowing that a role is only as good as its worst dependency — one bad
// cookbook anywhere in the chain blocks it"
func TestJourney_ARoleIsOnlyAsGoodAsItsWorstDependency(t *testing.T) {
	t.Skip("The worst-of roll-up is derived in SQL, not in Go: it lives in the recompute " +
		"and the list query in internal/datastore. Asserting it needs a live database, " +
		"so it cannot be answered from the API layer. The journey records the same gap.")
}

// "For a role I care about: what it pulls in"
func TestJourney_WhatARolePullsIn(t *testing.T) {
	store := &mockStore{
		GetRoleDetailFn: func(_ context.Context, name, _ string) (*datastore.RoleDetail, error) {
			return &datastore.RoleDetail{
				RoleName:            name,
				Organisations:       []string{"org-a"},
				DirectCookbooks:     []string{"ntp"},
				DirectRoles:         []string{"common"},
				TransitiveCookbooks: []string{"ntp", "openssl", "sudo"},
			}, nil
		},
	}
	w := roleImpactGet(t, store, "/api/v1/roles/base")
	if w.Code != http.StatusOK {
		t.Fatalf("asking for one role answered %d: %s", w.Code, w.Body.String())
	}
	var detail datastore.RoleDetail
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decoding the role detail: %v", err)
	}
	if len(detail.DirectCookbooks) == 0 || len(detail.DirectRoles) == 0 {
		t.Error("a role does not say what it pulls in directly")
	}
	if len(detail.TransitiveCookbooks) <= len(detail.DirectCookbooks) {
		t.Error("a role reports only what it names itself, so what the chain adds is invisible")
	}
}

// "including what it inherits from roles nested inside it, because that chain
// is where the surprises live and nobody holds it in their head"
//
// Asserted on the walk the API layer does itself: a cookbook reachable only
// through a role nested two levels down has to come back, not just the first
// level. The equivalent walk for a machine's run list is pinned separately in
// internal/nodekitchen.
func TestJourney_WhatItInheritsFromRolesNestedInside(t *testing.T) {
	store := roleImpactChainStore([]datastore.RoleDependency{
		{OrganisationName: "org-a", RoleName: "base", DependencyType: "role", DependencyName: "common"},
		{OrganisationName: "org-a", RoleName: "common", DependencyType: "role", DependencyName: "hardening"},
		{OrganisationName: "org-a", RoleName: "hardening", DependencyType: "cookbook", DependencyName: "openssl"},
	})
	w := roleImpactGet(t, store, "/api/v1/roles/base/dependency-graph")
	if w.Code != http.StatusOK {
		t.Fatalf("asking for the chain below one role answered %d: %s", w.Code, w.Body.String())
	}
	var graph roleDependencyGraphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &graph); err != nil {
		t.Fatalf("decoding the role chain: %v", err)
	}
	var foundInherited bool
	for _, n := range graph.Nodes {
		if n.Type == "cookbook" && n.Name == "openssl" {
			foundInherited = true
		}
	}
	if !foundInherited {
		t.Error("the chain stops before what a nested role brings in, so a cookbook two " +
			"roles down is invisible to the person deciding what to fix")
	}
}

// "A role that includes itself, directly or round a longer loop, does not
// recurse forever."
//
// The journey pins this for a machine's run list expansion. The role chain is
// a second walk over the same shape, so it is asserted here too: a loop must
// answer rather than run until the test binary is killed.
func TestJourney_ARoleThatIncludesItselfDoesNotRecurseForever(t *testing.T) {
	store := roleImpactChainStore([]datastore.RoleDependency{
		{OrganisationName: "org-a", RoleName: "base", DependencyType: "role", DependencyName: "common"},
		{OrganisationName: "org-a", RoleName: "common", DependencyType: "role", DependencyName: "base"},
		{OrganisationName: "org-a", RoleName: "common", DependencyType: "cookbook", DependencyName: "openssl"},
	})

	done := make(chan int, 1)
	go func() {
		done <- roleImpactGet(t, store, "/api/v1/roles/base/dependency-graph").Code
	}()
	select {
	case code := <-done:
		if code != http.StatusOK {
			t.Fatalf("a role chain containing a loop answered %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a role that includes itself round a loop never finished being expanded")
	}
}

// "Which of those are the ones actually blocking it."
func TestJourney_WhichOnesAreActuallyBlockingIt(t *testing.T) {
	store := &mockStore{
		GetRoleDetailFn: func(_ context.Context, name, _ string) (*datastore.RoleDetail, error) {
			return &datastore.RoleDetail{
				RoleName:            name,
				Organisations:       []string{"org-a"},
				TransitiveCookbooks: []string{"ntp", "openssl", "sudo"},
				BlockingCookbooks: []datastore.BlockingCookbook{{
					CookbookName:    "openssl",
					ComplexityLabel: "high",
					DependencyPath:  []string{"base", "common", "openssl"},
				}},
			}, nil
		},
	}
	w := roleImpactGet(t, store, "/api/v1/roles/base")
	if w.Code != http.StatusOK {
		t.Fatalf("asking for one role answered %d: %s", w.Code, w.Body.String())
	}
	var detail datastore.RoleDetail
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decoding the role detail: %v", err)
	}
	if len(detail.BlockingCookbooks) == 0 {
		t.Fatal("a role lists what it pulls in but never says which of them is blocking it, " +
			"so the whole chain has to be checked by hand")
	}
	if len(detail.BlockingCookbooks[0].DependencyPath) == 0 {
		t.Error("a blocking cookbook is named without saying how the role reaches it, so " +
			"there is no way to tell which nested role brought it in")
	}
}

// "How much rides on it — how many machines"
//
// This asserts the number reaches the reader, not that it is right. Whether
// the count is correct is decided by the SQL that materialises it, which the
// journey records as unproven.
func TestJourney_HowManyMachinesRideOnIt(t *testing.T) {
	store := &mockStore{
		GetRoleDetailFn: func(_ context.Context, name, _ string) (*datastore.RoleDetail, error) {
			return &datastore.RoleDetail{
				RoleName:      name,
				Organisations: []string{"org-a"},
				NodeCount:     4213,
			}, nil
		},
	}
	w := roleImpactGet(t, store, "/api/v1/roles/base")
	if w.Code != http.StatusOK {
		t.Fatalf("asking for one role answered %d: %s", w.Code, w.Body.String())
	}
	var detail datastore.RoleDetail
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decoding the role detail: %v", err)
	}
	if detail.NodeCount != 4213 {
		t.Errorf("a role does not say how many machines ride on it (got %d), so a fix "+
			"cannot be told from a corner case", detail.NodeCount)
	}
}

// "and which parts of the estate. That is what tells me whether this is a fix
// worth doing first or a corner case."
func TestJourney_AndWhichPartsOfTheEstate(t *testing.T) {
	store := &mockStore{
		GetRoleDetailFn: func(_ context.Context, name, _ string) (*datastore.RoleDetail, error) {
			return &datastore.RoleDetail{
				RoleName:            name,
				Organisations:       []string{"org-a", "org-b"},
				NodeCount:           30,
				NodesByOrganisation: []datastore.OrgCount{{Organisation: "org-a", Count: 20}, {Organisation: "org-b", Count: 10}},
				NodesByEnvironment:  []datastore.EnvCount{{Environment: "production", Count: 25}},
				NodesByPlatform:     []datastore.PlatformCount{{Platform: "rhel", PlatformVersion: "8", Count: 30}},
			}, nil
		},
	}
	w := roleImpactGet(t, store, "/api/v1/roles/base")
	if w.Code != http.StatusOK {
		t.Fatalf("asking for one role answered %d: %s", w.Code, w.Body.String())
	}
	var detail datastore.RoleDetail
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decoding the role detail: %v", err)
	}
	if len(detail.NodesByOrganisation) < 2 {
		t.Error("a role gives one total and no breakdown, so which parts of the estate it " +
			"reaches cannot be seen")
	}
	if len(detail.NodesByEnvironment) == 0 || len(detail.NodesByPlatform) == 0 {
		t.Error("the breakdown does not cover environment and platform, so a fix cannot be " +
			"judged against where the machines actually are")
	}
}

// "The chain for one role or one machine is a few dozen things and seeing it
// laid out makes the shape obvious."
func TestJourney_APictureScopedToOneRole(t *testing.T) {
	store := roleImpactChainStore([]datastore.RoleDependency{
		{OrganisationName: "org-a", RoleName: "base", DependencyType: "cookbook", DependencyName: "ntp"},
	})
	w := roleImpactGet(t, store, "/api/v1/roles/base/dependency-graph")
	if w.Code == http.StatusNotFound {
		t.Fatal("there is no way to see one role's chain laid out")
	}
	var graph roleDependencyGraphResponse
	if err := json.Unmarshal(w.Body.Bytes(), &graph); err != nil {
		t.Fatalf("decoding the role chain: %v", err)
	}
	if len(graph.Nodes) == 0 || len(graph.Edges) == 0 {
		t.Error("the picture for one role comes back with nothing in it")
	}
}

// "The same drawing for the entire estate is thousands of things and is
// unreadable — it looks impressive and tells me nothing. Small and scoped, or
// not at all."
func TestJourney_NoPictureOfTheWholeEstate(t *testing.T) {
	t.Skip("The journey says outright that nothing proves this judgement — that a scoped " +
		"chain is worth drawing and the whole estate is not. It is a decision to keep, " +
		"not a property to test.")
}

// "I can name the handful of fixes that between them unblock most of the
// estate, and say how many machines each one frees, without building it by
// hand from a list of servers."
func TestJourney_CanNameTheFixesThatFreeTheMostMachines(t *testing.T) {
	store := &mockStore{
		ListOrganisationsFn: func(_ context.Context) ([]datastore.Organisation, error) {
			return []datastore.Organisation{{Name: "org-a"}}, nil
		},
		ListServerCookbookComplexitiesByOrganisationFn: func(_ context.Context, _ string) ([]datastore.ServerCookbookComplexity, error) {
			return []datastore.ServerCookbookComplexity{
				{OrganisationName: "org-a", CookbookName: "niche", CookbookVersion: "1.0.0",
					TargetChefVersion: "19.0", ComplexityScore: 10, AffectedNodeCount: 5},
				{OrganisationName: "org-a", CookbookName: "openssl", CookbookVersion: "2.0.0",
					TargetChefVersion: "19.0", ComplexityScore: 10, AffectedNodeCount: 4000},
			}, nil
		},
	}
	w := roleImpactGet(t, store, "/api/v1/remediation/priority")
	if w.Code != http.StatusOK {
		t.Fatalf("asking which fixes free the most answered %d: %s", w.Code, w.Body.String())
	}
	var body remediationPriorityResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the priority list: %v", err)
	}
	if len(body.Data) == 0 {
		t.Fatal("there is no ranked list of fixes, so the handful worth doing first can only " +
			"be assembled by hand")
	}
	if body.Data[0].CookbookName != "openssl" {
		t.Errorf("the fix that frees the most machines is not first (got %q), so the list "+
			"does not answer what to do first", body.Data[0].CookbookName)
	}
	if body.Data[0].AffectedNodeCount == 0 {
		t.Error("a ranked fix does not say how many machines it frees")
	}
}

// "A role that is referenced but not found fails the expansion outright rather
// than resolving what it can and saying which part is missing ... a partial
// answer with a gap named in it would be more use here."
func TestJourney_OneRoleWeCannotReadDoesNotLoseTheWholeChain(t *testing.T) {
	t.Skip("The journey states this as a preference, not a settled requirement, and the " +
		"current all-or-nothing behaviour is deliberately pinned by an existing test in " +
		"internal/nodekitchen. Changing it is a decision for the owner, not a gap to close here.")
}

// roleImpactChainStore builds a store whose only role chain is the one given,
// in org-a, with no cookbook-to-cookbook edges.
func roleImpactChainStore(deps []datastore.RoleDependency) *mockStore {
	return &mockStore{
		GetRoleDetailFn: func(_ context.Context, name, _ string) (*datastore.RoleDetail, error) {
			return &datastore.RoleDetail{RoleName: name, Organisations: []string{"org-a"}}, nil
		},
		ListRoleDependenciesByOrgFn: func(_ context.Context, _ string) ([]datastore.RoleDependency, error) {
			return deps, nil
		},
		ListCookbookDependenciesByOrgFn: func(_ context.Context, _ string) (map[string][]string, error) {
			return map[string][]string{}, nil
		},
	}
}
