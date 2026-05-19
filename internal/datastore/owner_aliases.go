// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// OwnerAlias represents a row in the owner_aliases table.
type OwnerAlias struct {
	ID         string    `json:"id"`
	OwnerName  string    `json:"owner_name"`
	AliasType  string    `json:"alias_type"`
	AliasValue string    `json:"alias_value"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
}

// InsertOwnerAliasParams holds the fields for creating an owner alias.
type InsertOwnerAliasParams struct {
	OwnerName  string
	AliasType  string
	AliasValue string
	Source     string
}

// AliasSuggestion represents a fuzzy match result.
type AliasSuggestion struct {
	OwnerName  string  `json:"owner_name"`
	AliasValue string  `json:"alias_value"`
	AliasType  string  `json:"alias_type"`
	Similarity float64 `json:"similarity"`
}

const ownerAliasColumns = `id, owner_name, alias_type, alias_value, source, created_at`

func scanOwnerAlias(row interface{ Scan(dest ...any) error }) (OwnerAlias, error) {
	var a OwnerAlias
	err := row.Scan(&a.ID, &a.OwnerName, &a.AliasType, &a.AliasValue, &a.Source, &a.CreatedAt)
	if err != nil {
		return OwnerAlias{}, err
	}
	return a, nil
}

// InsertOwnerAlias creates a new alias for an owner. Returns ErrAlreadyExists
// if the (alias_type, alias_value) pair is already taken.
func (db *DB) InsertOwnerAlias(ctx context.Context, p InsertOwnerAliasParams) (OwnerAlias, error) {
	if p.OwnerName == "" {
		return OwnerAlias{}, errors.New("datastore: owner_name is required")
	}
	if p.AliasType == "" {
		return OwnerAlias{}, errors.New("datastore: alias_type is required")
	}
	if p.AliasValue == "" {
		return OwnerAlias{}, errors.New("datastore: alias_value is required")
	}

	source := p.Source
	if source == "" {
		source = "manual"
	}

	query := `
		INSERT INTO owner_aliases (owner_name, alias_type, alias_value, source)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + ownerAliasColumns

	row := db.pool.QueryRowContext(ctx, query, p.OwnerName, p.AliasType, p.AliasValue, source)
	a, err := scanOwnerAlias(row)
	if err != nil {
		if isUniqueViolation(err) {
			return OwnerAlias{}, ErrAlreadyExists
		}
		return OwnerAlias{}, fmt.Errorf("datastore: inserting owner alias: %w", err)
	}
	return a, nil
}

// GetOwnerAliasesByOwner returns all aliases for a given owner name.
func (db *DB) GetOwnerAliasesByOwner(ctx context.Context, ownerName string) ([]OwnerAlias, error) {
	query := `SELECT ` + ownerAliasColumns + ` FROM owner_aliases WHERE owner_name = $1 ORDER BY alias_type, alias_value`
	rows, err := db.pool.QueryContext(ctx, query, ownerName)
	if err != nil {
		return nil, fmt.Errorf("datastore: querying owner aliases: %w", err)
	}
	defer rows.Close()

	var aliases []OwnerAlias
	for rows.Next() {
		a, scanErr := scanOwnerAlias(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("datastore: scanning owner alias: %w", scanErr)
		}
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}

// ResolveOwnerByAlias finds the owner name for a given alias type and value.
// Returns ErrNotFound if no matching alias exists.
func (db *DB) ResolveOwnerByAlias(ctx context.Context, aliasType, aliasValue string) (string, error) {
	var ownerName string
	err := db.pool.QueryRowContext(ctx,
		`SELECT owner_name FROM owner_aliases WHERE alias_type = $1 AND alias_value = $2`,
		aliasType, aliasValue,
	).Scan(&ownerName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("datastore: resolving owner by alias: %w", err)
	}
	return ownerName, nil
}

// DeleteOwnerAlias removes an alias by its ID. Returns ErrNotFound if no
// such alias exists.
func (db *DB) DeleteOwnerAlias(ctx context.Context, id string) error {
	res, err := db.pool.ExecContext(ctx, `DELETE FROM owner_aliases WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting owner alias: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SuggestOwnerAliases uses trigram similarity to find potential alias matches.
// Returns up to `limit` suggestions sorted by similarity (highest first).
// Requires the pg_trgm extension.
func (db *DB) SuggestOwnerAliases(ctx context.Context, input string, limit int) ([]AliasSuggestion, error) {
	if input == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT owner_name, alias_value, alias_type,
		       similarity(alias_value, $1) AS sim
		FROM owner_aliases
		WHERE similarity(alias_value, $1) > 0.3
		ORDER BY sim DESC
		LIMIT $2`

	rows, err := db.pool.QueryContext(ctx, query, input, limit)
	if err != nil {
		return nil, fmt.Errorf("datastore: suggesting owner aliases: %w", err)
	}
	defer rows.Close()

	var suggestions []AliasSuggestion
	for rows.Next() {
		var s AliasSuggestion
		if err := rows.Scan(&s.OwnerName, &s.AliasValue, &s.AliasType, &s.Similarity); err != nil {
			return nil, fmt.Errorf("datastore: scanning alias suggestion: %w", err)
		}
		suggestions = append(suggestions, s)
	}
	return suggestions, rows.Err()
}
