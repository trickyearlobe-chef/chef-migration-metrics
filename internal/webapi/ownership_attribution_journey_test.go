//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// The journey suite for journeys/ownership-attribution.md — "Knowing who has
// to do the work". Run it with `make journey`.
//
// One test per thing the journey says has to be in place. A green one is
// built; a red one is not yet, and that makes this the journey's todo list.
//
// Where the depth needs a real database — the two repo-keying contracts and the
// export parity contract the journey names — the test here asserts the seam a
// person actually reaches and names the functional test that holds the rest.

// ---------------------------------------------------------------------------
// What I need
// ---------------------------------------------------------------------------

// "For a person or a team, everything that is theirs and what state it is in —
// the repositories, the cookbooks, the machines — so I can send one message
// that is entirely about their work."
func TestJourney_EverythingThatIsTheirsAndWhatStateItIsIn(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(_ context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{Name: name, OwnerType: "individual"}, nil
		},
		CountAssignmentsByOwnerFn: func(_ context.Context, _ string) (map[string]int, error) {
			return map[string]int{"git_repo": 4, "cookbook": 9, "node": 120}, nil
		},
		GetOwnerReadinessSummaryFn: func(_ context.Context, _, _ string) (datastore.OwnerReadinessSummary, error) {
			return datastore.OwnerReadinessSummary{}, nil
		},
		GetOwnerCookbookSummaryFn: func(_ context.Context, _, _ string) (datastore.OwnerCookbookSummary, error) {
			return datastore.OwnerCookbookSummary{}, nil
		},
		GetOwnerGitRepoSummaryFn: func(_ context.Context, _, _ string) (datastore.OwnerGitRepoSummary, error) {
			return datastore.OwnerGitRepoSummary{}, nil
		},
	}
	w := httptest.NewRecorder()
	ownershipRouter(store).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/api/v1/owners/alice.brown", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("an owner's own page answered %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the owner: %v", err)
	}
	// The three things the journey names, each with what state it is in.
	for field, thing := range map[string]string{
		"git_repo_summary":  "the repositories",
		"cookbook_summary":  "the cookbooks",
		"readiness_summary": "the machines",
	} {
		if _, ok := body[field]; !ok {
			t.Errorf("%s are missing from an owner's page, so a message about their work "+
				"cannot be assembled from it", thing)
		}
	}
}

// "For a person or a team."
func TestJourney_ForAPersonOrATeam(t *testing.T) {
	var created []string
	store := &mockStore{
		InsertOwnerFn: func(_ context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error) {
			created = append(created, p.OwnerType)
			return datastore.Owner{Name: p.Name, OwnerType: p.OwnerType}, nil
		},
		InsertAuditEntryFn: func(_ context.Context, _ datastore.InsertAuditEntryParams) error { return nil },
	}
	r := ownershipRouter(store)
	for _, kind := range []string{"individual", "team"} {
		body := `{"name":"platform-` + kind + `","owner_type":"` + kind + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/owners", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, withAdminSession(req))
		if w.Code != http.StatusOK && w.Code != http.StatusCreated {
			t.Errorf("work cannot be attached to a %s (answered %d): %s", kind, w.Code, w.Body.String())
		}
	}
	if len(created) != 2 {
		t.Errorf("owner types that reached the record: %v — a team is as much an owner as "+
			"a person, or a team's work has nobody against it", created)
	}
}

// "For a piece of work, who owns it."
func TestJourney_ForAPieceOfWorkWhoOwnsIt(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{
		LookupAssignmentOwnersByEntityFn: func(_ context.Context, _ string, keys []string) (map[string][]datastore.EntityAssignment, error) {
			out := map[string][]datastore.EntityAssignment{}
			for _, k := range keys {
				out[k] = []datastore.EntityAssignment{{OwnerName: "alice.brown"}}
			}
			return out, nil
		},
	})
	got, err := r.ownersForEntities(context.Background(), "git_repo", []string{"acme-apache"})
	if err != nil {
		t.Fatalf("asking who owns a repo: %v", err)
	}
	if len(got["acme-apache"].Owners) != 1 || got["acme-apache"].Owners[0] != "alice.brown" {
		t.Errorf("a piece of work does not say who owns it: %v", got["acme-apache"].Owners)
	}
}

// "and if nobody does, to see that plainly rather than have it sit invisibly in
// a total."
func TestJourney_NobodyOwnsThisIsSaidPlainly(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{
		LookupAssignmentOwnersByEntityFn: func(_ context.Context, _ string, _ []string) (map[string][]datastore.EntityAssignment, error) {
			return map[string][]datastore.EntityAssignment{}, nil
		},
	})
	got, err := r.ownersForEntities(context.Background(), "git_repo", []string{"acme-orphan"})
	if err != nil {
		t.Fatalf("asking who owns a repo: %v", err)
	}
	entry, present := got["acme-orphan"]
	if !present {
		t.Fatal("a repo nobody owns is absent from the answer entirely, which reads as " +
			"'not checked' rather than 'nobody'")
	}
	if entry.Owners == nil {
		t.Error("a repo nobody owns answers null rather than an empty list, so a view " +
			"cannot tell 'nobody' from 'nothing was asked'")
	}
}

// "Ownership is attached to the thing, not to the report. It has to reach
// whatever anybody looks at — a list, a detail page, a total, an export — or
// the numbers disagree with each other."
//
// The detail page, the total and the export all carry it. The list does not:
// a row says nothing about who owns it, so the one screen somebody works
// through is the one that cannot answer the journey's second requirement.
func TestJourney_OwnershipReachesTheListAsWellAsTheDetailPage(t *testing.T) {
	store := gitRepoOwnershipStore([]datastore.OwnershipAssignment{
		{OwnerName: "alice.brown", EntityType: "git_repo", EntityKey: "acme-apache"},
	})
	w := httptest.NewRecorder()
	ownershipRouter(store).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("listing repos answered %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the list: %v", err)
	}
	if len(body.Data) == 0 {
		t.Fatal("the fixture proves nothing: the list came back empty")
	}
	for _, row := range body.Data {
		_, hasOwnership := row["ownership"]
		_, hasOwners := row["owners"]
		if !hasOwnership && !hasOwners {
			t.Error("a row in the repo list does not say who owns it, so the list — the " +
				"screen the work is dispatched from — cannot answer 'who owns this, and " +
				"if nobody, say so'. The detail page can.")
			break
		}
	}
}

// "The unowned pile, as a first-class thing. Work nobody has been made
// responsible for is the single most useful list here."
func TestJourney_TheUnownedPileIsAListOfItsOwn(t *testing.T) {
	store := gitRepoOwnershipStore([]datastore.OwnershipAssignment{
		{OwnerName: "alice.brown", EntityType: "git_repo", EntityKey: "acme-apache"},
	})
	w := httptest.NewRecorder()
	ownershipRouter(store).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?unowned=true", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("asking for the unowned answered %d: %s", w.Code, w.Body.String())
	}
	got := gitRepoNames(t, w.Body.Bytes())
	if len(got) == 0 {
		t.Fatal("the unowned list came back empty, so the fixture proves nothing")
	}
	for _, name := range got {
		if name == "acme-apache" {
			t.Error("a repo somebody owns appeared in the unowned list, so the parameter " +
				"is being ignored rather than answered")
		}
	}
}

// "It must not be hidden behind a filter that defaults to hiding it."
func TestJourney_TheUnownedPileIsNotHiddenByDefault(t *testing.T) {
	store := gitRepoOwnershipStore([]datastore.OwnershipAssignment{
		{OwnerName: "alice.brown", EntityType: "git_repo", EntityKey: "acme-apache"},
	})
	w := httptest.NewRecorder()
	ownershipRouter(store).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/api/v1/git-repos", nil))

	got := gitRepoNames(t, w.Body.Bytes())
	unowned := 0
	for _, name := range got {
		if name != "acme-apache" {
			unowned++
		}
	}
	if unowned != 2 {
		t.Errorf("asking for nothing in particular returned %v — work nobody owns is "+
			"hidden unless it is asked for", got)
	}
}

// "To cut every view down to one owner's work, and to have that survive into an
// export, because what I actually send somebody is a file."
//
// The parity of a filter between screen and file is held for one view by
// internal/datastore/node_snapshot_export_functional_test.go
// #TestFunctional_NodeExport_FilterParity. What is asserted here is that the
// file somebody sends actually contains one person's work and not the estate.
func TestJourney_OneOwnersWorkSurvivesIntoAnExport(t *testing.T) {
	store := gitRepoOwnershipStore([]datastore.OwnershipAssignment{
		{OwnerName: "alice.brown", EntityType: "git_repo", EntityKey: "acme-apache"},
	})
	w := httptest.NewRecorder()
	newTestRouterWithMockAndConfig(store, exportTestConfig()).ServeHTTP(w,
		httptest.NewRequest(http.MethodPost,
			"/api/v1/exports?export_type=git_repos&format=csv&owner=alice.brown", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("exporting one owner's work answered %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "acme-apache") {
		t.Errorf("the owner's own repo is missing from the file:\n%s", body)
	}
	for _, somebodyElses := range []string{"acme-nginx", "acme-mysql"} {
		if strings.Contains(body, somebodyElses) {
			t.Errorf("the file sent to one person contains %s, which is not theirs:\n%s",
				somebodyElses, body)
		}
	}
}

// "Where nobody has claimed something, a hint at who plausibly should — the
// people who have actually been changing it."
func TestJourney_AHintAtWhoPlausiblyShouldOwnIt(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(_ context.Context, _ string) (string, error) {
			return "https://git.example.com/apache.git", nil
		},
		ListCommittersByRepoFn: func(_ context.Context, _ datastore.CommitterListFilter) ([]datastore.GitRepoCommitter, int, error) {
			return []datastore.GitRepoCommitter{
				{AuthorName: "Alice Brown", AuthorEmail: "alice.b@example.com", CommitCount: 42},
			}, 1, nil
		},
		GetOwnerEmailsForGitRepoFn: func(_ context.Context, _ string) (map[string]bool, error) {
			return map[string]bool{}, nil
		},
	}
	w := httptest.NewRecorder()
	ownershipRouter(store).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/acme-apache/committers", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("asking who has been changing a repo answered %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "alice.b@example.com") {
		t.Errorf("nothing suggests who to ask about an unclaimed repo:\n%s", w.Body.String())
	}
}

// "Who has committed to something is evidence, not a verdict. It suggests who
// to ask. Making it an automatic assignment would attribute work to whoever
// last fixed a typo."
func TestJourney_WhoHasCommittedIsEvidenceNotAVerdict(t *testing.T) {
	var assigned []datastore.InsertAssignmentParams
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(_ context.Context, _ string) (string, error) {
			return "https://git.example.com/apache.git", nil
		},
		ListCommittersByRepoFn: func(_ context.Context, _ datastore.CommitterListFilter) ([]datastore.GitRepoCommitter, int, error) {
			return []datastore.GitRepoCommitter{
				{AuthorName: "Alice Brown", AuthorEmail: "alice.b@example.com", CommitCount: 1},
			}, 1, nil
		},
		GetOwnerEmailsForGitRepoFn: func(_ context.Context, _ string) (map[string]bool, error) {
			return map[string]bool{}, nil
		},
		InsertAssignmentFn: func(_ context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			assigned = append(assigned, p)
			return datastore.OwnershipAssignment{ID: 1}, nil
		},
	}
	w := httptest.NewRecorder()
	ownershipRouter(store).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/acme-apache/committers", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("listing committers answered %d: %s", w.Code, w.Body.String())
	}
	if len(assigned) != 0 {
		t.Errorf("looking at who has committed assigned the work to them: %+v — a person "+
			"who fixed a typo now owns the repo", assigned)
	}
}

// "A repository is identified by its name, not by where it is hosted.
// Addresses change ... If ownership is keyed on the address then a routine
// infrastructure change quietly un-owns work that somebody had claimed."
//
// The two readers are held by
// internal/datastore/ownership_git_repo_key_functional_test.go
// #TestFunctional_OwnerGitRepoSummary_ResolvesByRepoName and
// #TestFunctional_CookbookInheritsRepoOwnerByName. What is asserted here is the
// list: a repo claimed under its name is that person's, and one claimed under
// its address is not found by any screen.
func TestJourney_ARepositoryIsIdentifiedByItsNameNotWhereItIsHosted(t *testing.T) {
	byName := gitRepoOwnershipStore([]datastore.OwnershipAssignment{
		{OwnerName: "alice.brown", EntityType: "git_repo", EntityKey: "acme-apache"},
	})
	w := httptest.NewRecorder()
	ownershipRouter(byName).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?owner=alice.brown", nil))
	got := gitRepoNames(t, w.Body.Bytes())
	if len(got) != 1 || got[0] != "acme-apache" {
		t.Fatalf("a repo claimed under its name is not in its owner's list: %v", got)
	}

	// The baseline that makes the assertion above mean something: the address
	// is not what the list reads, so an assignment keyed on it finds nothing.
	byURL := gitRepoOwnershipStore([]datastore.OwnershipAssignment{
		{OwnerName: "alice.brown", EntityType: "git_repo", EntityKey: "https://git.example.com/apache.git"},
	})
	w = httptest.NewRecorder()
	ownershipRouter(byURL).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/api/v1/git-repos?owner=alice.brown", nil))
	if got := gitRepoNames(t, w.Body.Bytes()); len(got) != 0 {
		t.Errorf("the list matched on the address as well as the name (%v), so two keys "+
			"are in use and they can disagree", got)
	}
}

// ---------------------------------------------------------------------------
// What the journey says nothing can prove
// ---------------------------------------------------------------------------

// "Nothing proves an owner's cookbook verdict. The compatible, incompatible and
// untested counts on an owner's page are not asserted anywhere, and they are
// derived rather than stored."
func TestJourney_NothingProvesAnOwnersCookbookVerdict(t *testing.T) {
	t.Skip("TODO: the compatible / incompatible / untested counts on an owner's page are " +
		"derived and unasserted — the least trustworthy numbers in this journey. Pinning " +
		"them needs a real database and belongs beside the other owner-summary contracts " +
		"in internal/datastore. Establish what they actually return before quoting them.")
}

// "Nothing proves the unowned pile is complete. 'Unowned' is the absence of a
// record, and absence is exactly what a wrong join produces as well."
func TestJourney_NothingProvesTheUnownedPileIsComplete(t *testing.T) {
	t.Skip("Deliberate and structural: a repo owned but keyed wrongly and one with no " +
		"owner at all look identical on the screen. No test of the unowned list can " +
		"distinguish them; only the keying contracts above can.")
}

// "The load-bearing assumption: that every place ownership is read resolves it
// the same way. There is no single reader to point at, so this cannot be pinned
// by one test — it is a property of not having a fourth reader appear that
// invents its own rule."
func TestJourney_EveryPlaceOwnershipIsReadResolvesItTheSameWay(t *testing.T) {
	t.Skip("Cannot be pinned by one test, by the journey's own account. The nearest " +
		"thing to a guard is that ownersForEntities and resolveOwnershipFilter are the " +
		"only two derivations; if a new view starts reporting ownership, check it " +
		"against the repo-keying contracts before believing it.")
}
