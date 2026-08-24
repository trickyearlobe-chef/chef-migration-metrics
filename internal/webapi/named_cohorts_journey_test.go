//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// The journey suite for journeys/named-cohorts.md. Run it with `make journey`.
//
// One test per thing the journey says has to be in place. A green one is built;
// a red one is not yet. That makes this the todo list for the journey, and a
// todo list made of tests cannot go stale: nobody has to remember to update it,
// because running it recomputes it.
//
// It is deliberately OUTSIDE the gating suite. A red here is the normal state
// for most of a journey's life and must never block a build.
//
// Two rules:
//
//   - Assert the real thing, so building the feature turns the test green with
//     no edit.
//   - Name the journey line it comes from, in the journey's words, so the
//     reason outlives whoever wrote it.
//
// This is not where regressions go. Something that used to work and now fails
// is a broken build, not a todo.

// "To build a selection once, give it a name that means something to me, and
// pick it again from a list."
//
// The whole round trip, because either half alone proves nothing: a selection
// that can be stored but not listed is still rebuilt every morning.
func TestJourney_BuildASelectionOnceAndPickItAgainByName(t *testing.T) {
	var stored datastore.InsertSavedFilterParams
	store := &mockStore{
		InsertSavedFilterFn: func(_ context.Context, p datastore.InsertSavedFilterParams) (datastore.SavedFilter, error) {
			stored = p
			return datastore.SavedFilter{ID: "1", Name: p.Name, View: p.View, Filters: p.Filters}, nil
		},
		ListSavedFiltersFn: func(_ context.Context, _ datastore.SavedFilterListFilter) ([]datastore.SavedFilter, error) {
			return []datastore.SavedFilter{{ID: "1", Name: stored.Name, View: stored.View, Filters: stored.Filters}}, nil
		},
	}
	router := newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0"))

	body := `{"name":"the base roles","view":"nodes","filters":{"role":["base","base-linux"]}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/saved-filters", strings.NewReader(body))
	router.ServeHTTP(w, withAdminSession(req))
	if w.Code != http.StatusCreated {
		t.Fatalf("naming a selection answered %d: %s — it cannot be built once", w.Code, w.Body.String())
	}
	if stored.Name != "the base roles" || len(stored.Filters["role"]) != 2 {
		t.Errorf("the name and the selection are not both kept (name %q, selection %v)",
			stored.Name, stored.Filters)
	}

	w = httptest.NewRecorder()
	router.ServeHTTP(w, withAdminSession(httptest.NewRequest(http.MethodGet, "/api/v1/saved-filters", nil)))
	if w.Code != http.StatusOK {
		t.Fatalf("listing kept selections answered %d: %s", w.Code, w.Body.String())
	}
	var listed []datastore.SavedFilter
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decoding the list of kept selections: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "the base roles" {
		t.Errorf("a kept selection cannot be picked again by name (got %v)", listed)
	}
}

// "On the views where I work — machines, cookbooks, repositories."
func TestJourney_OnTheViewsWhereIWork(t *testing.T) {
	for view, selection := range map[string]map[string][]string{
		"nodes":     {"role": {"base"}},
		"cookbooks": {"name": {"apache"}},
		"git-repos": {"name": {"web"}},
	} {
		if err := validateSavedFilterSelection(view, selection); err != nil {
			t.Errorf("a cohort cannot be kept on the %s view: %v", view, err)
		}
	}
	// A view nothing serves is refused rather than stored, or the cohort could
	// only fail when somebody came back to use it.
	if err := validateSavedFilterSelection("machines", map[string][]string{"role": {"base"}}); err == nil {
		t.Error("a cohort can be kept against a view that does not exist")
	}
}

// "The same selection to mean the same thing wherever I use it, including when
// I export it. A cohort that filters the screen one way and the export another
// way is worse than not having it, because I will not notice."
//
// Asserted at the seam: the same query string is put to the list endpoint and
// to the export endpoint, and the selection each hands the store is compared.
// That the two then return the same ROWS needs a real database and is pinned by
// TestFunctional_NodeExport_FilterParity, outside this suite.
func TestJourney_TheSameSelectionMeansTheSameThingWhenIExportIt(t *testing.T) {
	for _, c := range []cohortParityCase{
		{
			view:       "nodes",
			listPath:   "/api/v1/nodes",
			exportType: "nodes",
			query:      "node_name=web&environment=prod&platform=windows&role=base&role=base-linux&tags=db&policy_name=p&policy_group=g&chef_version=17",
			wire: func(store *mockStore, record func(any)) {
				store.ListNodeSnapshotsFilteredFn = func(_ context.Context, f datastore.NodeSnapshotFilter) ([]datastore.NodeSnapshot, int, error) {
					record(f)
					return nil, 0, nil
				}
				store.ListNodeSnapshotsForExportFn = func(_ context.Context, f datastore.NodeSnapshotFilter, _ datastore.NodeSnapshotCursor, _ int) ([]datastore.NodeSnapshot, error) {
					record(f)
					return nil, nil
				}
			},
		},
		{
			view:       "cookbooks",
			listPath:   "/api/v1/cookbooks",
			exportType: "cookbooks",
			query:      "name=apache&active=true&compatibility=incompatible&cookstyle_status=clean&download_status=downloaded&tk_status=passed",
			wire: func(store *mockStore, record func(any)) {
				store.ListCookbooksFilteredFn = func(_ context.Context, f datastore.CookbookFilter) ([]datastore.CookbookFilterRow, int, error) {
					record(f)
					return nil, 0, nil
				}
			},
		},
		{
			view:       "git-repos",
			listPath:   "/api/v1/git-repos",
			exportType: "git_repos",
			query:      "name=web&compatibility=incompatible&cookstyle_status=clean&tk_status=passed&clone_status=cloned&has_test_suite=true&human_verdict=ok",
			wire: func(store *mockStore, record func(any)) {
				store.ListGitReposFilteredFn = func(_ context.Context, f datastore.GitRepoFilter) ([]datastore.GitRepo, int, error) {
					record(f)
					return nil, 0, nil
				}
			},
		},
	} {
		t.Run(c.view, func(t *testing.T) {
			onScreen := cohortSelectionAsked(t, c, http.MethodGet, c.listPath+"?"+c.query)
			exported := cohortSelectionAsked(t, c, http.MethodPost,
				"/api/v1/exports?export_type="+c.exportType+"&format=csv&"+c.query)

			if !reflect.DeepEqual(onScreen, exported) {
				t.Errorf("the %s view and its export ask for different sets from one selection:\n"+
					" screen: %+v\n export: %+v", c.view, onScreen, exported)
			}
		})
	}
}

// "To combine it with whatever else I am filtering by at the time, rather than
// it replacing my whole view."
//
// What is held here is the half that lives on the server: the global lens and
// the way the list is being read are not part of a cohort, so recalling one
// cannot move them. Which controls a recalled cohort clears on the page is a
// frontend decision (frontend/src/pages/nodeSavedFilters.test.ts).
func TestJourney_CombinesWithWhatIAmAlreadyFilteringBy(t *testing.T) {
	// The baseline first: a plain selection on this view is accepted, so a
	// refusal below cannot be a refusal of everything.
	if err := validateSavedFilterSelection("nodes", map[string][]string{"role": {"base"}}); err != nil {
		t.Fatalf("the fixture proves nothing: an ordinary nodes selection is refused anyway: %v", err)
	}

	for _, param := range []string{"target_chef_version", "stale", "stale_tiers", "sort", "order", "page", "per_page"} {
		err := validateSavedFilterSelection("nodes", map[string][]string{param: {"x"}})
		if err == nil {
			t.Errorf("a cohort can carry %q, so recalling one overrides what the operator "+
				"had set rather than combining with it", param)
		}
	}
}

// "This adds no new way to query. It remembers a selection that was already
// possible to make by hand. Anything I could not filter by before, I still
// cannot."
//
// Every param a cohort may carry is one the view's own list endpoint already
// declares it reads. A savable param that no view accepts is a new way to
// query, and it fails only when the cohort is used.
func TestJourney_AddsNoNewWayToQuery(t *testing.T) {
	routeByView := map[string]string{
		"nodes":     "/api/v1/nodes",
		"roles":     "/api/v1/roles",
		"cookbooks": "/api/v1/cookbooks",
		"git-repos": "/api/v1/git-repos",
	}

	declared := map[string]map[string]bool{}
	for _, rt := range journeyRouter().Routes() {
		for view, pattern := range routeByView {
			if rt.Pattern != pattern {
				continue
			}
			accepted := map[string]bool{}
			for _, p := range rt.Queries[http.MethodGet] {
				accepted[p.Name] = true
			}
			declared[view] = accepted
		}
	}

	for view, vocabulary := range savedFilterVocabulary {
		accepted, served := declared[view]
		if !served || len(accepted) == 0 {
			t.Errorf("cohorts can be kept for %q but no list endpoint declares what it filters by, "+
				"so nothing says the selection was ever possible by hand", view)
			continue
		}
		var unsupported []string
		for param := range vocabulary {
			if !accepted[param] {
				unsupported = append(unsupported, param)
			}
		}
		sort.Strings(unsupported)
		if len(unsupported) > 0 {
			t.Errorf("the %s view's cohorts may carry %v, which the view itself does not accept — "+
				"a cohort can ask for something no screen could", view, unsupported)
		}
	}
}

// "A selection the server does not understand must fail loudly, not quietly. If
// a saved cohort carries something the server will not accept, it has to say
// so. A filter that is silently dropped returns the unfiltered estate, and an
// unfiltered result looks exactly like a legitimate answer."
func TestJourney_ASelectionTheServerDoesNotUnderstandFailsLoudly(t *testing.T) {
	if err := validateSavedFilterSelection("nodes", map[string][]string{"role": {"base"}}); err != nil {
		t.Fatalf("the fixture proves nothing: a selection the view does accept is refused anyway: %v", err)
	}
	if err := validateSavedFilterSelection("nodes", map[string][]string{"datacentre": {"dc1"}}); err == nil {
		t.Error("a selection the view does not understand is accepted, so it is dropped in " +
			"silence and the whole estate comes back looking like a match")
	}
}

// "... with the same refusal enforced on the way in when a cohort is saved."
func TestJourney_ACohortIsRefusedWhenItIsSaved(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/saved-filters",
		strings.NewReader(`{"name":"mine","view":"nodes","filters":{"datacentre":["dc1"]}}`))
	journeyRouter().ServeHTTP(w, withAdminSession(req))
	if w.Code != http.StatusBadRequest {
		t.Errorf("saving a cohort carrying a param the view does not accept answered %d — "+
			"it is stored now and can only fail when somebody comes back to use it", w.Code)
	}
}

// "... and when an existing one is changed."
func TestJourney_ACohortIsRefusedWhenItIsChanged(t *testing.T) {
	store := &mockStore{
		GetSavedFilterFn: func(_ context.Context, id string) (datastore.SavedFilter, error) {
			return datastore.SavedFilter{ID: id, View: "nodes", OwnerUsername: "admin"}, nil
		},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/saved-filters/1",
		strings.NewReader(`{"filters":{"datacentre":["dc1"]}}`))
	newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0")).
		ServeHTTP(w, withAdminSession(req))
	if w.Code != http.StatusBadRequest {
		t.Errorf("editing a cohort into carrying a param its view does not accept answered %d — "+
			"the refusal at save time can be walked around by saving a good one and changing it", w.Code)
	}
}

// "I open the machines view, choose the cohort by name, and get the same set I
// got last week — and when I export it, the file matches what was on the
// screen."
func TestJourney_RecallingACohortByNameGivesTheSameSetOnBothPaths(t *testing.T) {
	t.Skip("The journey names this itself: nothing checks that a cohort RECALLED BY NAME " +
		"resolves to the same selection on the screen and in the export. Parity is held " +
		"for a selection built directly (TestJourney_TheSameSelectionMeansTheSameThingWhenIExportIt " +
		"at the seam, TestFunctional_NodeExport_FilterParity over rows); the recall step " +
		"between the stored cohort and those params is the step not covered.")
}

// cohortParityCase drives one view's list endpoint and its export with the same
// query, capturing the selection each hands the store.
type cohortParityCase struct {
	view       string
	listPath   string
	exportType string
	query      string
	// wire points the store at record, for whichever calls that view's two
	// paths make.
	wire func(store *mockStore, record func(any))
}

// cohortSelectionAsked returns the selection one path handed the store, with
// the parts that are not the cohort removed: paging is how the list is read,
// and the target version is a global lens neither path takes from the cohort.
func cohortSelectionAsked(t *testing.T, c cohortParityCase, method, target string) any {
	t.Helper()

	var seen []any
	store := &mockStore{}
	c.wire(store, func(f any) { seen = append(seen, f) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, nil)
	newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0")).
		ServeHTTP(w, withAdminSession(req))

	if len(seen) == 0 {
		t.Fatalf("%s %s asked the store for nothing (answered %d: %s), so the two paths "+
			"cannot be compared", method, target, w.Code, w.Body.String())
	}

	asked := cohortSelectionOnly(seen[0])
	// Two empty selections match each other, and would pass this while both
	// paths ignored the query entirely.
	if reflect.DeepEqual(asked, cohortSelectionOnly(reflect.Zero(reflect.TypeOf(seen[0])).Interface())) {
		t.Fatalf("%s %s carried a selection and asked the store for an unfiltered one, so a "+
			"comparison against the other path proves nothing", method, target)
	}
	return asked
}

// cohortSelectionOnly blanks the fields that are not part of a named cohort.
func cohortSelectionOnly(filter any) any {
	switch f := filter.(type) {
	case datastore.NodeSnapshotFilter:
		f.Limit, f.Offset, f.TargetChefVersion = 0, 0, ""
		return f
	case datastore.CookbookFilter:
		f.Limit, f.Offset, f.TargetChefVersion = 0, 0, ""
		return f
	case datastore.GitRepoFilter:
		f.Limit, f.Offset = 0, 0
		return f
	}
	return filter
}
