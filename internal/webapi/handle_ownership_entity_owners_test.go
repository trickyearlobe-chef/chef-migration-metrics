// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"errors"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// A list view and a detail view of the same thing must not answer "who owns
// this" differently. They disagree when each looks it up its own way, so both
// read it from ownersForEntities and the derivation lives there once.

func ownersTestRouter(byType map[string]map[string][]datastore.EntityAssignment, fail bool) *Router {
	return newTestRouterWithMock(&mockStore{
		LookupAssignmentOwnersByEntityFn: func(_ context.Context, entityType string, keys []string) (map[string][]datastore.EntityAssignment, error) {
			if fail {
				return nil, errors.New("database is unreachable")
			}
			out := map[string][]datastore.EntityAssignment{}
			for _, k := range keys {
				if a, ok := byType[entityType][k]; ok {
					out[k] = a
				}
			}
			return out, nil
		},
	})
}

func TestOwnersForEntities_ReportsTheOwner(t *testing.T) {
	r := ownersTestRouter(map[string]map[string][]datastore.EntityAssignment{
		"git_repo": {"nginx": {{OwnerName: "alice"}}},
	}, false)

	got, err := r.ownersForEntities(context.Background(), "git_repo", []string{"nginx"})
	if err != nil {
		t.Fatalf("looking up: %v", err)
	}
	if len(got["nginx"].Owners) != 1 || got["nginx"].Owners[0] != "alice" {
		t.Errorf("owners = %v, want [alice]", got["nginx"].Owners)
	}
	if got["nginx"].Derived {
		t.Error("a direct assignment was reported as derived")
	}
}

// "Nobody owns this" is a statement a view has to be able to make. An absent
// key would leave it saying nothing at all, which reads as "not checked".
func TestOwnersForEntities_SaysNobodyRatherThanNothing(t *testing.T) {
	r := ownersTestRouter(nil, false)

	got, err := r.ownersForEntities(context.Background(), "git_repo", []string{"orphan"})
	if err != nil {
		t.Fatalf("looking up: %v", err)
	}
	entry, present := got["orphan"]
	if !present {
		t.Fatal("an unowned entity was missing from the result entirely")
	}
	if entry.Owners == nil {
		t.Error("owners was nil; it must be an empty list so the JSON says [] not null")
	}
}

// A cookbook is owned by whoever owns the repo it is built from, and the view
// should be able to say the ownership came from there.
func TestOwnersForEntities_CookbookOwnershipComesFromTheRepoAndSaysSo(t *testing.T) {
	r := ownersTestRouter(map[string]map[string][]datastore.EntityAssignment{
		"git_repo": {"nginx": {{OwnerName: "alice"}}},
	}, false)

	got, err := r.ownersForEntities(context.Background(), "cookbook", []string{"nginx"})
	if err != nil {
		t.Fatalf("looking up: %v", err)
	}
	if len(got["nginx"].Owners) != 1 || got["nginx"].Owners[0] != "alice" {
		t.Errorf("owners = %v, want [alice] via the repo", got["nginx"].Owners)
	}
	if !got["nginx"].Derived {
		t.Error("ownership taken from the repo was not marked as derived")
	}
}

// A cookbook assigned in its own right is not overridden by the repo, and is
// not derived.
func TestOwnersForEntities_ADirectCookbookAssignmentWins(t *testing.T) {
	r := ownersTestRouter(map[string]map[string][]datastore.EntityAssignment{
		"cookbook": {"nginx": {{OwnerName: "bob"}}},
		"git_repo": {"nginx": {{OwnerName: "alice"}}},
	}, false)

	got, err := r.ownersForEntities(context.Background(), "cookbook", []string{"nginx"})
	if err != nil {
		t.Fatalf("looking up: %v", err)
	}
	if len(got["nginx"].Owners) != 1 || got["nginx"].Owners[0] != "bob" {
		t.Errorf("owners = %v, want [bob] — the direct assignment", got["nginx"].Owners)
	}
	if got["nginx"].Derived {
		t.Error("a direct cookbook assignment was reported as derived")
	}
}

// A node is not built from a repo, so nothing is derived for it.
func TestOwnersForEntities_NodesAreNotDerived(t *testing.T) {
	r := ownersTestRouter(map[string]map[string][]datastore.EntityAssignment{
		"git_repo": {"web-01": {{OwnerName: "alice"}}},
	}, false)

	got, err := r.ownersForEntities(context.Background(), "node", []string{"web-01"})
	if err != nil {
		t.Fatalf("looking up: %v", err)
	}
	if len(got["web-01"].Owners) != 0 {
		t.Errorf("a repo assignment leaked into node ownership: %v", got["web-01"].Owners)
	}
}

func TestOwnersForEntities_ReportsAFailureRatherThanClaimingNobodyOwnsIt(t *testing.T) {
	r := ownersTestRouter(nil, true)

	if _, err := r.ownersForEntities(context.Background(), "git_repo", []string{"nginx"}); err == nil {
		t.Fatal("a lookup failure was reported as an empty result, which reads as unowned")
	}
}
