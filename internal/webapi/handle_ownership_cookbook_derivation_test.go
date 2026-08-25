// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// A cookbook's owner is whoever owns the git repo it is built from. Git is the
// code; a server cookbook is the deployed artefact, and a fix is made in the
// repo. People say "cookbook" in standup and mean the repo.
//
// Ownership is recorded against repos only, so filtering by a person on the
// cookbook list has to derive it. Recording it twice would be two truths that
// can disagree.

func ownershipTestRouter(assignments map[string][]string, ownedByType map[string]map[string]bool) *Router {
	store := &mockStore{
		ListAssignmentsByOwnerFn: func(_ context.Context, f datastore.AssignmentListFilter) ([]datastore.OwnershipAssignment, int, error) {
			var out []datastore.OwnershipAssignment
			for _, key := range assignments[f.OwnerName+"|"+f.EntityType] {
				out = append(out, datastore.OwnershipAssignment{
					OwnerName: f.OwnerName, EntityType: f.EntityType, EntityKey: key,
				})
			}
			return out, len(out), nil
		},
		ListOwnedEntityKeysFn: func(_ context.Context, entityType string) (map[string]bool, error) {
			return ownedByType[entityType], nil
		},
	}
	return newTestRouterWithMock(store)
}

func TestResolveOwnershipFilter_CookbookOwnershipComesFromTheRepo(t *testing.T) {
	r := ownershipTestRouter(map[string][]string{
		// Only the repo side is recorded, which is how the estate looks.
		"alice|git_repo": {"nginx", "apache"},
		"alice|cookbook": {},
	}, nil)

	keys, err := r.resolveOwnershipFilter(context.Background(),
		ownerFilter{Active: true, OwnerNames: []string{"alice"}}, "cookbook")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if !keys["nginx"] || !keys["apache"] {
		t.Errorf("cookbooks owned via their repos = %v, want nginx and apache", keys)
	}
}

// A cookbook assigned directly still counts. The two sources are a union, not a
// replacement — an estate may name a cookbook that has no repo.
func TestResolveOwnershipFilter_DirectCookbookAssignmentStillCounts(t *testing.T) {
	r := ownershipTestRouter(map[string][]string{
		"alice|git_repo": {"nginx"},
		"alice|cookbook": {"vendored-thing"},
	}, nil)

	keys, err := r.resolveOwnershipFilter(context.Background(),
		ownerFilter{Active: true, OwnerNames: []string{"alice"}}, "cookbook")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if !keys["nginx"] || !keys["vendored-thing"] {
		t.Errorf("keys = %v, want both the derived and the direct cookbook", keys)
	}
}

// The reverse must not happen: a cookbook assignment does not make somebody the
// owner of a repo. The repo is where the code lives and where ownership is
// recorded; deriving backwards would invent authority over the source.
func TestResolveOwnershipFilter_RepoOwnershipIsNotDerivedFromCookbooks(t *testing.T) {
	r := ownershipTestRouter(map[string][]string{
		"alice|git_repo": {},
		"alice|cookbook": {"vendored-thing"},
	}, nil)

	keys, err := r.resolveOwnershipFilter(context.Background(),
		ownerFilter{Active: true, OwnerNames: []string{"alice"}}, "git_repo")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if keys["vendored-thing"] {
		t.Error("a cookbook assignment made somebody the owner of a repo")
	}
}

// The unowned question has to derive the same way, or the two halves of one
// control disagree: a cookbook whose repo has an owner is not unowned.
func TestResolveOwnershipFilter_UnownedCookbooksExcludeThoseOwnedViaTheirRepo(t *testing.T) {
	r := ownershipTestRouter(nil, map[string]map[string]bool{
		"cookbook": {"vendored-thing": true},
		"git_repo": {"nginx": true},
	})

	keys, err := r.resolveOwnershipFilter(context.Background(),
		ownerFilter{Active: true, Unowned: true}, "cookbook")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	// These are the keys that count as owned; the filter excludes them.
	if !keys["nginx"] {
		t.Error("a cookbook whose repo has an owner was treated as unowned")
	}
	if !keys["vendored-thing"] {
		t.Error("a directly assigned cookbook was treated as unowned")
	}
}

// Nodes are unaffected: a node is not built from a repo.
func TestResolveOwnershipFilter_NodesAreNotDerived(t *testing.T) {
	r := ownershipTestRouter(map[string][]string{
		"alice|git_repo": {"nginx"},
		"alice|node":     {"web-01"},
	}, nil)

	keys, err := r.resolveOwnershipFilter(context.Background(),
		ownerFilter{Active: true, OwnerNames: []string{"alice"}}, "node")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if keys["nginx"] {
		t.Error("a repo assignment leaked into node ownership")
	}
	if !keys["web-01"] {
		t.Errorf("node ownership lost: %v", keys)
	}
}
