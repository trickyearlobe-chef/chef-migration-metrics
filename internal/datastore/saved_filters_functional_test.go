// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// seedSavedFilterUsers creates the two users the saved-filter tests own filters
// as, and registers cleanup of them and their filters (filters cascade).
func seedSavedFilterUsers(t *testing.T, db *DB, usernames ...string) {
	t.Helper()
	ctx := context.Background()

	for _, u := range usernames {
		cleanupTestData(t, db, "DELETE FROM users WHERE username = '"+u+"'")
		if _, err := db.InsertUser(ctx, InsertUserParams{
			Username:     u,
			Role:         "viewer",
			AuthProvider: "local",
			PasswordHash: "x",
		}); err != nil && !errors.Is(err, ErrAlreadyExists) {
			t.Fatalf("seeding user %q: %v", u, err)
		}
	}
}

// The driving use case: a ~20-role "All Windows OS" cohort must survive a
// round-trip verbatim — the stored selection is kept as-is, never normalised.
func TestFunctional_SavedFilter_RoundTrips(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const owner = "func-sf-owner"
	seedSavedFilterUsers(t, db, owner)

	roles := []string{
		"win-base", "win-iis", "win-sql", "win-dc", "win-file", "win-print",
		"win-rdp", "win-app", "win-mq", "win-monitor",
	}

	created, err := db.InsertSavedFilter(ctx, InsertSavedFilterParams{
		OwnerUsername: owner,
		View:          "nodes",
		Name:          "All Windows OS",
		Filters:       map[string][]string{"role": roles},
	})
	if err != nil {
		t.Fatalf("inserting saved filter: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected the created filter to carry a generated id")
	}
	if created.Shared {
		t.Error("a new saved filter must default to private")
	}

	got, err := db.GetSavedFilter(ctx, created.ID)
	if err != nil {
		t.Fatalf("getting saved filter: %v", err)
	}
	if got.Name != "All Windows OS" || got.View != "nodes" || got.OwnerUsername != owner {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if !reflect.DeepEqual(got.Filters["role"], roles) {
		t.Errorf("selection not preserved verbatim:\n got: %v\nwant: %v", got.Filters["role"], roles)
	}
}

func TestFunctional_SavedFilter_GetUnknownIsNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetSavedFilter(context.Background(), "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for an unknown id, got: %v", err)
	}
}

// Name is unique per (owner, view) — and only per (owner, view).
func TestFunctional_SavedFilter_NameUniquePerOwnerAndView(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const ownerA = "func-sf-uniq-a"
	const ownerB = "func-sf-uniq-b"
	seedSavedFilterUsers(t, db, ownerA, ownerB)

	base := InsertSavedFilterParams{
		OwnerUsername: ownerA,
		View:          "nodes",
		Name:          "Windows",
		Filters:       map[string][]string{"role": {"win-base"}},
	}
	if _, err := db.InsertSavedFilter(ctx, base); err != nil {
		t.Fatalf("inserting first filter: %v", err)
	}

	// Same owner, same view, same name — collides.
	if _, err := db.InsertSavedFilter(ctx, base); !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists on a duplicate (owner, view, name), got: %v", err)
	}

	// Same name on a different view — allowed.
	otherView := base
	otherView.View = "roles"
	otherView.Filters = map[string][]string{"name": {"win"}}
	if _, err := db.InsertSavedFilter(ctx, otherView); err != nil {
		t.Errorf("same name on a different view must be allowed, got: %v", err)
	}

	// Same name owned by a different user — allowed.
	otherOwner := base
	otherOwner.OwnerUsername = ownerB
	if _, err := db.InsertSavedFilter(ctx, otherOwner); err != nil {
		t.Errorf("same name for a different owner must be allowed, got: %v", err)
	}
}

// Visibility: own filters (private or shared) plus other users' shared ones —
// never another user's private filter.
func TestFunctional_ListSavedFilters_VisibleToUser(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const me = "func-sf-vis-me"
	const them = "func-sf-vis-them"
	seedSavedFilterUsers(t, db, me, them)

	mine, err := db.InsertSavedFilter(ctx, InsertSavedFilterParams{
		OwnerUsername: me, View: "nodes", Name: "Mine Private",
		Filters: map[string][]string{"role": {"base"}},
	})
	if err != nil {
		t.Fatalf("inserting my filter: %v", err)
	}
	theirsShared, err := db.InsertSavedFilter(ctx, InsertSavedFilterParams{
		OwnerUsername: them, View: "nodes", Name: "Theirs Shared",
		Filters: map[string][]string{"role": {"base"}}, Shared: true,
	})
	if err != nil {
		t.Fatalf("inserting their shared filter: %v", err)
	}
	theirsPrivate, err := db.InsertSavedFilter(ctx, InsertSavedFilterParams{
		OwnerUsername: them, View: "nodes", Name: "Theirs Private",
		Filters: map[string][]string{"role": {"base"}},
	})
	if err != nil {
		t.Fatalf("inserting their private filter: %v", err)
	}
	// A filter on another view — must not appear when listing the nodes view.
	otherView, err := db.InsertSavedFilter(ctx, InsertSavedFilterParams{
		OwnerUsername: me, View: "roles", Name: "Mine On Roles",
		Filters: map[string][]string{"name": {"web"}},
	})
	if err != nil {
		t.Fatalf("inserting my roles filter: %v", err)
	}

	got, err := db.ListSavedFilters(ctx, SavedFilterListFilter{Username: me, View: "nodes"})
	if err != nil {
		t.Fatalf("listing saved filters: %v", err)
	}

	visible := map[string]bool{}
	for _, f := range got {
		visible[f.ID] = true
	}
	if !visible[mine.ID] {
		t.Error("my own private filter must be visible to me")
	}
	if !visible[theirsShared.ID] {
		t.Error("another user's shared filter must be visible")
	}
	if visible[theirsPrivate.ID] {
		t.Error("another user's private filter must NOT be visible")
	}
	if visible[otherView.ID] {
		t.Error("a filter on another view must not appear when listing the nodes view")
	}

	// Unfiltered by view: my roles filter shows up too.
	all, err := db.ListSavedFilters(ctx, SavedFilterListFilter{Username: me})
	if err != nil {
		t.Fatalf("listing all saved filters: %v", err)
	}
	foundOtherView := false
	for _, f := range all {
		if f.ID == otherView.ID {
			foundOtherView = true
		}
	}
	if !foundOtherView {
		t.Error("listing without a view filter must return filters from every view")
	}
}

func TestFunctional_UpdateSavedFilter_RenameSelectionAndShare(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const owner = "func-sf-update"
	seedSavedFilterUsers(t, db, owner)

	created, err := db.InsertSavedFilter(ctx, InsertSavedFilterParams{
		OwnerUsername: owner, View: "nodes", Name: "Draft",
		Filters: map[string][]string{"role": {"base"}},
	})
	if err != nil {
		t.Fatalf("inserting saved filter: %v", err)
	}

	newName := "All Windows OS"
	newFilters := map[string][]string{"role": {"win-base", "win-iis"}, "platform": {"windows"}}
	shared := true

	updated, err := db.UpdateSavedFilter(ctx, created.ID, UpdateSavedFilterParams{
		Name:    &newName,
		Filters: &newFilters,
		Shared:  &shared,
	})
	if err != nil {
		t.Fatalf("updating saved filter: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("name = %q, want %q", updated.Name, newName)
	}
	if !reflect.DeepEqual(updated.Filters, newFilters) {
		t.Errorf("filters = %v, want %v", updated.Filters, newFilters)
	}
	if !updated.Shared {
		t.Error("shared flag was not set")
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("updated_at was not bumped (created %v, updated %v)", created.UpdatedAt, updated.UpdatedAt)
	}

	// A partial update leaves the untouched fields alone.
	unshared := false
	partial, err := db.UpdateSavedFilter(ctx, created.ID, UpdateSavedFilterParams{Shared: &unshared})
	if err != nil {
		t.Fatalf("partial update: %v", err)
	}
	if partial.Shared {
		t.Error("shared flag was not cleared")
	}
	if partial.Name != newName || !reflect.DeepEqual(partial.Filters, newFilters) {
		t.Errorf("partial update clobbered untouched fields: %+v", partial)
	}
}

func TestFunctional_UpdateSavedFilter_RenameCollision(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const owner = "func-sf-collide"
	seedSavedFilterUsers(t, db, owner)

	if _, err := db.InsertSavedFilter(ctx, InsertSavedFilterParams{
		OwnerUsername: owner, View: "nodes", Name: "Taken",
		Filters: map[string][]string{"role": {"base"}},
	}); err != nil {
		t.Fatalf("inserting first filter: %v", err)
	}
	second, err := db.InsertSavedFilter(ctx, InsertSavedFilterParams{
		OwnerUsername: owner, View: "nodes", Name: "Second",
		Filters: map[string][]string{"role": {"base"}},
	})
	if err != nil {
		t.Fatalf("inserting second filter: %v", err)
	}

	taken := "Taken"
	_, err = db.UpdateSavedFilter(ctx, second.ID, UpdateSavedFilterParams{Name: &taken})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists renaming onto a taken name, got: %v", err)
	}
}

func TestFunctional_UpdateSavedFilter_UnknownIsNotFound(t *testing.T) {
	db := testDB(t)

	name := "x"
	_, err := db.UpdateSavedFilter(context.Background(),
		"00000000-0000-0000-0000-000000000000",
		UpdateSavedFilterParams{Name: &name},
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound updating an unknown filter, got: %v", err)
	}
}

func TestFunctional_DeleteSavedFilter(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const owner = "func-sf-delete"
	seedSavedFilterUsers(t, db, owner)

	created, err := db.InsertSavedFilter(ctx, InsertSavedFilterParams{
		OwnerUsername: owner, View: "nodes", Name: "Doomed",
		Filters: map[string][]string{"role": {"base"}},
	})
	if err != nil {
		t.Fatalf("inserting saved filter: %v", err)
	}

	if err := db.DeleteSavedFilter(ctx, created.ID); err != nil {
		t.Fatalf("deleting saved filter: %v", err)
	}
	if _, err := db.GetSavedFilter(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected the filter to be gone, got: %v", err)
	}
	if err := db.DeleteSavedFilter(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound deleting twice, got: %v", err)
	}
}

// Deleting the owner takes their saved filters with them (FK cascade) — a saved
// filter has no meaning without an owner.
func TestFunctional_SavedFilter_CascadesOnOwnerDelete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const owner = "func-sf-cascade"
	seedSavedFilterUsers(t, db, owner)

	created, err := db.InsertSavedFilter(ctx, InsertSavedFilterParams{
		OwnerUsername: owner, View: "nodes", Name: "Orphan",
		Filters: map[string][]string{"role": {"base"}},
	})
	if err != nil {
		t.Fatalf("inserting saved filter: %v", err)
	}

	if err := db.DeleteUser(ctx, owner); err != nil {
		t.Fatalf("deleting owner: %v", err)
	}
	if _, err := db.GetSavedFilter(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected the saved filter to cascade away with its owner, got: %v", err)
	}
}
