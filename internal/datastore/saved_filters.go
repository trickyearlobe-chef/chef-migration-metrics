// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SavedFilter is a named, owned, view-scoped set of filter query params.
//
// Filters holds the view's own request contract — param name to values, exactly
// as the view's URL and its filter parser speak it. The vocabulary is validated
// against the target view at save time (internal/webapi/saved_filter_params.go);
// this layer stores the selection verbatim and never normalises it.
type SavedFilter struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	View          string              `json:"view"`
	Filters       map[string][]string `json:"filters"`
	OwnerUsername string              `json:"owner_username"`
	Shared        bool                `json:"shared"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// InsertSavedFilterParams are the fields needed to create a saved filter.
type InsertSavedFilterParams struct {
	OwnerUsername string
	View          string
	Name          string
	Filters       map[string][]string
	Shared        bool
}

// UpdateSavedFilterParams carries the fields to change. Nil fields are left
// alone, so a rename, a new selection, and a share toggle are all the same call.
type UpdateSavedFilterParams struct {
	Name    *string
	Filters *map[string][]string
	Shared  *bool
}

// SavedFilterListFilter selects the saved filters visible to a user: their own,
// plus every shared one. View is optional — empty lists across all views.
type SavedFilterListFilter struct {
	Username string
	View     string
}

const savedFilterColumns = `id, name, view_name, filters, owner_username, shared, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSavedFilter(row rowScanner) (SavedFilter, error) {
	var (
		f           SavedFilter
		filtersJSON []byte
	)
	err := row.Scan(
		&f.ID,
		&f.Name,
		&f.View,
		&filtersJSON,
		&f.OwnerUsername,
		&f.Shared,
		&f.CreatedAt,
		&f.UpdatedAt,
	)
	if err != nil {
		return SavedFilter{}, err
	}
	if len(filtersJSON) > 0 {
		if err := json.Unmarshal(filtersJSON, &f.Filters); err != nil {
			return SavedFilter{}, fmt.Errorf("datastore: decoding saved filter selection: %w", err)
		}
	}
	if f.Filters == nil {
		f.Filters = map[string][]string{}
	}
	return f, nil
}

// InsertSavedFilter creates a saved filter. Returns ErrAlreadyExists if the
// owner already has a filter of that name on that view.
func (db *DB) InsertSavedFilter(ctx context.Context, p InsertSavedFilterParams) (SavedFilter, error) {
	filtersJSON, err := marshalSavedFilterSelection(p.Filters)
	if err != nil {
		return SavedFilter{}, err
	}

	query := `
		INSERT INTO saved_filters (owner_username, view_name, name, filters, shared)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + savedFilterColumns

	row := db.pool.QueryRowContext(ctx, query, p.OwnerUsername, p.View, p.Name, filtersJSON, p.Shared)

	f, err := scanSavedFilter(row)
	if err != nil {
		if isUniqueViolation(err) {
			return SavedFilter{}, ErrAlreadyExists
		}
		return SavedFilter{}, fmt.Errorf("datastore: inserting saved filter: %w", err)
	}
	return f, nil
}

// GetSavedFilter returns the saved filter with the given id. Returns ErrNotFound
// if no such filter exists.
func (db *DB) GetSavedFilter(ctx context.Context, id string) (SavedFilter, error) {
	query := `SELECT ` + savedFilterColumns + ` FROM saved_filters WHERE id = $1`

	f, err := scanSavedFilter(db.pool.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SavedFilter{}, ErrNotFound
		}
		return SavedFilter{}, fmt.Errorf("datastore: getting saved filter: %w", err)
	}
	return f, nil
}

// ListSavedFilters returns the saved filters visible to a user — their own plus
// every shared one — ordered by name.
func (db *DB) ListSavedFilters(ctx context.Context, f SavedFilterListFilter) ([]SavedFilter, error) {
	query := `
		SELECT ` + savedFilterColumns + `
		FROM saved_filters
		WHERE (owner_username = $1 OR shared)
		  AND ($2 = '' OR view_name = $2)
		ORDER BY name ASC`

	rows, err := db.pool.QueryContext(ctx, query, f.Username, f.View)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing saved filters: %w", err)
	}
	defer rows.Close()

	var filters []SavedFilter
	for rows.Next() {
		sf, err := scanSavedFilter(rows)
		if err != nil {
			return nil, fmt.Errorf("datastore: scanning saved filter: %w", err)
		}
		filters = append(filters, sf)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating saved filter rows: %w", err)
	}
	return filters, nil
}

// UpdateSavedFilter applies the non-nil fields of p to the saved filter with the
// given id. Returns ErrNotFound if no such filter exists, or ErrAlreadyExists if
// a rename collides with another of the owner's filters on the same view.
func (db *DB) UpdateSavedFilter(ctx context.Context, id string, p UpdateSavedFilterParams) (SavedFilter, error) {
	sets := []string{"updated_at = now()"}
	args := []any{}
	argIdx := 1

	if p.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *p.Name)
		argIdx++
	}
	if p.Filters != nil {
		filtersJSON, err := marshalSavedFilterSelection(*p.Filters)
		if err != nil {
			return SavedFilter{}, err
		}
		sets = append(sets, fmt.Sprintf("filters = $%d", argIdx))
		args = append(args, filtersJSON)
		argIdx++
	}
	if p.Shared != nil {
		sets = append(sets, fmt.Sprintf("shared = $%d", argIdx))
		args = append(args, *p.Shared)
		argIdx++
	}

	query := fmt.Sprintf(
		`UPDATE saved_filters SET %s WHERE id = $%d RETURNING %s`,
		joinStrings(sets, ", "), argIdx, savedFilterColumns,
	)
	args = append(args, id)

	f, err := scanSavedFilter(db.pool.QueryRowContext(ctx, query, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SavedFilter{}, ErrNotFound
		}
		if isUniqueViolation(err) {
			return SavedFilter{}, ErrAlreadyExists
		}
		return SavedFilter{}, fmt.Errorf("datastore: updating saved filter: %w", err)
	}
	return f, nil
}

// DeleteSavedFilter removes a saved filter by id. Returns ErrNotFound if no such
// filter exists.
func (db *DB) DeleteSavedFilter(ctx context.Context, id string) error {
	res, err := db.pool.ExecContext(ctx, `DELETE FROM saved_filters WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting saved filter: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// marshalSavedFilterSelection encodes a selection for the jsonb column. A nil
// selection is stored as an empty object, not JSON null.
func marshalSavedFilterSelection(filters map[string][]string) ([]byte, error) {
	if filters == nil {
		filters = map[string][]string{}
	}
	b, err := json.Marshal(filters)
	if err != nil {
		return nil, fmt.Errorf("datastore: encoding saved filter selection: %w", err)
	}
	return b, nil
}
