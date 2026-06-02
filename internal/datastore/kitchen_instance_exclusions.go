// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// KitchenInstanceExclusion represents a row in kitchen_instance_exclusions.
type KitchenInstanceExclusion struct {
	ID           string    `json:"id"`
	GitRepoName  string    `json:"git_repo_name"`
	GitRepoURL   string    `json:"git_repo_url"`
	SuiteName    string    `json:"suite_name"`
	PlatformName string    `json:"platform_name"`
	Reason       string    `json:"reason"`
	ExcludedBy   string    `json:"excluded_by"`
	CreatedAt    time.Time `json:"created_at"`
}

// CreateKitchenExclusionParams holds the fields needed to create an exclusion.
type CreateKitchenExclusionParams struct {
	GitRepoName  string
	GitRepoURL   string
	SuiteName    string
	PlatformName string
	Reason       string
	ExcludedBy   string
}

const kieColumns = `id, git_repo_name, git_repo_url, suite_name, platform_name, reason, excluded_by, created_at`

// CreateKitchenExclusion inserts a new exclusion record.
func (db *DB) CreateKitchenExclusion(ctx context.Context, p CreateKitchenExclusionParams) (KitchenInstanceExclusion, error) {
	query := `INSERT INTO kitchen_instance_exclusions
		(git_repo_name, git_repo_url, suite_name, platform_name, reason, excluded_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + kieColumns

	var e KitchenInstanceExclusion
	err := db.q().QueryRowContext(ctx, query,
		p.GitRepoName, p.GitRepoURL, p.SuiteName, p.PlatformName, p.Reason, p.ExcludedBy,
	).Scan(&e.ID, &e.GitRepoName, &e.GitRepoURL, &e.SuiteName, &e.PlatformName,
		&e.Reason, &e.ExcludedBy, &e.CreatedAt)
	if err != nil {
		return KitchenInstanceExclusion{}, fmt.Errorf("create kitchen exclusion: %w", err)
	}
	// Instance exclusion changes active results → recompute TK status.
	_ = db.RecomputeGitRepoTKStatusByName(ctx, p.GitRepoName)
	return e, nil
}

// ListKitchenExclusions returns exclusions, optionally filtered by repo name.
func (db *DB) ListKitchenExclusions(ctx context.Context, repoName string) ([]KitchenInstanceExclusion, error) {
	var rows *sql.Rows
	var err error

	if repoName != "" {
		query := `SELECT ` + kieColumns + ` FROM kitchen_instance_exclusions
			WHERE git_repo_name = $1 ORDER BY created_at DESC`
		rows, err = db.q().QueryContext(ctx, query, repoName)
	} else {
		query := `SELECT ` + kieColumns + ` FROM kitchen_instance_exclusions ORDER BY created_at DESC`
		rows, err = db.q().QueryContext(ctx, query)
	}
	if err != nil {
		return nil, fmt.Errorf("list kitchen exclusions: %w", err)
	}
	defer rows.Close()

	var results []KitchenInstanceExclusion
	for rows.Next() {
		var e KitchenInstanceExclusion
		if err := rows.Scan(&e.ID, &e.GitRepoName, &e.GitRepoURL, &e.SuiteName, &e.PlatformName,
			&e.Reason, &e.ExcludedBy, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan kitchen exclusion: %w", err)
		}
		results = append(results, e)
	}
	return results, rows.Err()
}

// DeleteKitchenExclusion removes an exclusion by ID. Returns false if not found.
func (db *DB) DeleteKitchenExclusion(ctx context.Context, id string) (bool, error) {
	// Look up repo name before deletion for recomputation.
	var repoName string
	_ = db.q().QueryRowContext(ctx,
		`SELECT git_repo_name FROM kitchen_instance_exclusions WHERE id = $1`, id,
	).Scan(&repoName)

	result, err := db.q().ExecContext(ctx,
		`DELETE FROM kitchen_instance_exclusions WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete kitchen exclusion: %w", err)
	}
	n, _ := result.RowsAffected()
	if n > 0 && repoName != "" {
		_ = db.RecomputeGitRepoTKStatusByName(ctx, repoName)
	}
	return n > 0, nil
}
