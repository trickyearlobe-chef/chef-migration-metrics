package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// savedFilterRequest issues a request against the saved-filter endpoints as the
// given user. An empty username means no session at all.
func savedFilterRequest(t *testing.T, r *Router, method, target, body, username string) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	if username != "" {
		req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.SessionInfo{
			Username: username,
			Role:     "viewer",
		}))
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandleSavedFilters_CreateStoresSelectionForSessionUser(t *testing.T) {
	var got datastore.InsertSavedFilterParams
	store := &mockStore{
		InsertSavedFilterFn: func(_ context.Context, p datastore.InsertSavedFilterParams) (datastore.SavedFilter, error) {
			got = p
			return datastore.SavedFilter{ID: "sf-1", Name: p.Name, View: p.View, Filters: p.Filters, OwnerUsername: p.OwnerUsername}, nil
		},
	}
	r := newTestRouterWithMock(store)

	body := `{"name":"All Windows OS","view":"nodes","filters":{"role":["win-base","win-iis"]}}`
	w := savedFilterRequest(t, r, http.MethodPost, "/api/v1/saved-filters", body, "alice")

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	if got.OwnerUsername != "alice" {
		t.Errorf("owner = %q, want the session username %q", got.OwnerUsername, "alice")
	}
	if got.View != "nodes" || got.Name != "All Windows OS" {
		t.Errorf("unexpected params: %+v", got)
	}
	if len(got.Filters["role"]) != 2 {
		t.Errorf("selection not passed through: %v", got.Filters)
	}

	var created datastore.SavedFilter
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if created.ID != "sf-1" {
		t.Errorf("response id = %q, want %q", created.ID, "sf-1")
	}
}

// The vocabulary is owned by the view: an unknown param is rejected at save
// time, never silently stored.
func TestHandleSavedFilters_CreateRejectsUnknownParam(t *testing.T) {
	called := false
	store := &mockStore{
		InsertSavedFilterFn: func(context.Context, datastore.InsertSavedFilterParams) (datastore.SavedFilter, error) {
			called = true
			return datastore.SavedFilter{}, nil
		},
	}
	r := newTestRouterWithMock(store)

	body := `{"name":"Bad","view":"nodes","filters":{"cluster_name":["prod"]}}`
	w := savedFilterRequest(t, r, http.MethodPost, "/api/v1/saved-filters", body, "alice")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if called {
		t.Error("an invalid selection must never reach the datastore")
	}
	if !strings.Contains(w.Body.String(), "cluster_name") {
		t.Errorf("response should name the offending param: %s", w.Body.String())
	}
}

func TestHandleSavedFilters_CreateRejectsGlobalLensParam(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	body := `{"name":"Lens","view":"nodes","filters":{"target_chef_version":["18.4.12"]}}`
	w := savedFilterRequest(t, r, http.MethodPost, "/api/v1/saved-filters", body, "alice")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

func TestHandleSavedFilters_CreateRejectsSortAndPagination(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	body := `{"name":"Sorted","view":"nodes","filters":{"sort":["name"],"role":["base"]}}`
	w := savedFilterRequest(t, r, http.MethodPost, "/api/v1/saved-filters", body, "alice")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

func TestHandleSavedFilters_CreateRequiresNameAndView(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	for _, body := range []string{
		`{"view":"nodes","filters":{}}`,
		`{"name":"","view":"nodes","filters":{}}`,
		`{"name":"No View","filters":{}}`,
		`{"name":"Bad View","view":"dashboard","filters":{}}`,
	} {
		w := savedFilterRequest(t, r, http.MethodPost, "/api/v1/saved-filters", body, "alice")
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
	}
}

func TestHandleSavedFilters_CreateDuplicateNameIsConflict(t *testing.T) {
	store := &mockStore{
		InsertSavedFilterFn: func(context.Context, datastore.InsertSavedFilterParams) (datastore.SavedFilter, error) {
			return datastore.SavedFilter{}, datastore.ErrAlreadyExists
		},
	}
	r := newTestRouterWithMock(store)

	body := `{"name":"Taken","view":"nodes","filters":{"role":["base"]}}`
	w := savedFilterRequest(t, r, http.MethodPost, "/api/v1/saved-filters", body, "alice")

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", w.Code, w.Body.String())
	}
}

func TestHandleSavedFilters_CreateWithoutSessionIsUnauthorized(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	body := `{"name":"Anon","view":"nodes","filters":{"role":["base"]}}`
	w := savedFilterRequest(t, r, http.MethodPost, "/api/v1/saved-filters", body, "")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
	}
}

func TestHandleSavedFilters_ListPassesSessionUserAndView(t *testing.T) {
	var got datastore.SavedFilterListFilter
	store := &mockStore{
		ListSavedFiltersFn: func(_ context.Context, f datastore.SavedFilterListFilter) ([]datastore.SavedFilter, error) {
			got = f
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)

	w := savedFilterRequest(t, r, http.MethodGet, "/api/v1/saved-filters?view=nodes", "", "alice")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if got.Username != "alice" || got.View != "nodes" {
		t.Errorf("list filter = %+v, want username alice and view nodes", got)
	}
	// A caller with no saved filters gets an empty array, not null.
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("body = %s, want []", w.Body.String())
	}
}

func TestHandleSavedFilters_ListRejectsUnknownView(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	w := savedFilterRequest(t, r, http.MethodGet, "/api/v1/saved-filters?view=dashboard", "", "alice")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

func TestHandleSavedFilter_OwnerCanUpdate(t *testing.T) {
	var got datastore.UpdateSavedFilterParams
	store := &mockStore{
		GetSavedFilterFn: func(_ context.Context, id string) (datastore.SavedFilter, error) {
			return datastore.SavedFilter{ID: id, View: "nodes", OwnerUsername: "alice"}, nil
		},
		UpdateSavedFilterFn: func(_ context.Context, id string, p datastore.UpdateSavedFilterParams) (datastore.SavedFilter, error) {
			got = p
			return datastore.SavedFilter{ID: id, Name: *p.Name, View: "nodes", OwnerUsername: "alice"}, nil
		},
	}
	r := newTestRouterWithMock(store)

	body := `{"name":"Renamed","shared":true}`
	w := savedFilterRequest(t, r, http.MethodPatch, "/api/v1/saved-filters/sf-1", body, "alice")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if got.Name == nil || *got.Name != "Renamed" {
		t.Errorf("name not passed to the store: %+v", got)
	}
	if got.Shared == nil || !*got.Shared {
		t.Errorf("shared flag not passed to the store: %+v", got)
	}
	if got.Filters != nil {
		t.Error("an absent selection must not be overwritten")
	}
}

// The update payload is validated against the *stored* view — a saved filter
// cannot be edited into carrying a param its view does not accept.
func TestHandleSavedFilter_UpdateRejectsUnknownParamForStoredView(t *testing.T) {
	store := &mockStore{
		GetSavedFilterFn: func(_ context.Context, id string) (datastore.SavedFilter, error) {
			return datastore.SavedFilter{ID: id, View: "git-repos", OwnerUsername: "alice"}, nil
		},
		UpdateSavedFilterFn: func(_ context.Context, _ string, _ datastore.UpdateSavedFilterParams) (datastore.SavedFilter, error) {
			t.Error("an invalid selection must never reach the datastore")
			return datastore.SavedFilter{}, nil
		},
	}
	r := newTestRouterWithMock(store)

	body := `{"filters":{"role":["base"]}}` // a nodes param, on a git-repos filter
	w := savedFilterRequest(t, r, http.MethodPatch, "/api/v1/saved-filters/sf-1", body, "alice")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

// Shared is read-only: a non-owner may see and apply a shared filter but never
// mutate it.
func TestHandleSavedFilter_NonOwnerCannotMutateSharedFilter(t *testing.T) {
	store := &mockStore{
		GetSavedFilterFn: func(_ context.Context, id string) (datastore.SavedFilter, error) {
			return datastore.SavedFilter{ID: id, View: "nodes", OwnerUsername: "alice", Shared: true}, nil
		},
		UpdateSavedFilterFn: func(context.Context, string, datastore.UpdateSavedFilterParams) (datastore.SavedFilter, error) {
			t.Error("a non-owner must never reach the update")
			return datastore.SavedFilter{}, nil
		},
		DeleteSavedFilterFn: func(context.Context, string) error {
			t.Error("a non-owner must never reach the delete")
			return nil
		},
	}
	r := newTestRouterWithMock(store)

	w := savedFilterRequest(t, r, http.MethodPatch, "/api/v1/saved-filters/sf-1", `{"name":"Hijacked"}`, "bob")
	if w.Code != http.StatusForbidden {
		t.Errorf("PATCH by non-owner: status = %d, want 403: %s", w.Code, w.Body.String())
	}

	w = savedFilterRequest(t, r, http.MethodDelete, "/api/v1/saved-filters/sf-1", "", "bob")
	if w.Code != http.StatusForbidden {
		t.Errorf("DELETE by non-owner: status = %d, want 403: %s", w.Code, w.Body.String())
	}
}

// Another user's private filter is invisible — it must not be distinguishable
// from one that does not exist.
func TestHandleSavedFilter_OtherUsersPrivateFilterIsNotFound(t *testing.T) {
	store := &mockStore{
		GetSavedFilterFn: func(_ context.Context, id string) (datastore.SavedFilter, error) {
			return datastore.SavedFilter{ID: id, View: "nodes", OwnerUsername: "alice", Shared: false}, nil
		},
	}
	r := newTestRouterWithMock(store)

	w := savedFilterRequest(t, r, http.MethodPatch, "/api/v1/saved-filters/sf-1", `{"name":"Hijacked"}`, "bob")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestHandleSavedFilter_OwnerCanDelete(t *testing.T) {
	deleted := ""
	store := &mockStore{
		GetSavedFilterFn: func(_ context.Context, id string) (datastore.SavedFilter, error) {
			return datastore.SavedFilter{ID: id, View: "nodes", OwnerUsername: "alice"}, nil
		},
		DeleteSavedFilterFn: func(_ context.Context, id string) error {
			deleted = id
			return nil
		},
	}
	r := newTestRouterWithMock(store)

	w := savedFilterRequest(t, r, http.MethodDelete, "/api/v1/saved-filters/sf-1", "", "alice")

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", w.Code, w.Body.String())
	}
	if deleted != "sf-1" {
		t.Errorf("deleted id = %q, want sf-1", deleted)
	}
}

func TestHandleSavedFilter_UnknownIsNotFound(t *testing.T) {
	store := &mockStore{
		GetSavedFilterFn: func(context.Context, string) (datastore.SavedFilter, error) {
			return datastore.SavedFilter{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)

	w := savedFilterRequest(t, r, http.MethodDelete, "/api/v1/saved-filters/nope", "", "alice")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestHandleSavedFilter_RenameCollisionIsConflict(t *testing.T) {
	store := &mockStore{
		GetSavedFilterFn: func(_ context.Context, id string) (datastore.SavedFilter, error) {
			return datastore.SavedFilter{ID: id, View: "nodes", OwnerUsername: "alice"}, nil
		},
		UpdateSavedFilterFn: func(context.Context, string, datastore.UpdateSavedFilterParams) (datastore.SavedFilter, error) {
			return datastore.SavedFilter{}, datastore.ErrAlreadyExists
		},
	}
	r := newTestRouterWithMock(store)

	w := savedFilterRequest(t, r, http.MethodPatch, "/api/v1/saved-filters/sf-1", `{"name":"Taken"}`, "alice")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", w.Code, w.Body.String())
	}
}

func TestHandleSavedFilter_UpdateWithNoFieldsIsBadRequest(t *testing.T) {
	store := &mockStore{
		GetSavedFilterFn: func(_ context.Context, id string) (datastore.SavedFilter, error) {
			return datastore.SavedFilter{ID: id, View: "nodes", OwnerUsername: "alice"}, nil
		},
	}
	r := newTestRouterWithMock(store)

	w := savedFilterRequest(t, r, http.MethodPatch, "/api/v1/saved-filters/sf-1", `{}`, "alice")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

func TestHandleSavedFilters_StoreErrorIsInternal(t *testing.T) {
	store := &mockStore{
		ListSavedFiltersFn: func(context.Context, datastore.SavedFilterListFilter) ([]datastore.SavedFilter, error) {
			return nil, errors.New("boom")
		},
	}
	r := newTestRouterWithMock(store)

	w := savedFilterRequest(t, r, http.MethodGet, "/api/v1/saved-filters", "", "alice")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
	}
}
