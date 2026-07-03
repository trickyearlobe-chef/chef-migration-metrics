// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"encoding/json"
	"fmt"
)

// CookstyleResultRef is a lightweight projection of a cookstyle result that
// references a particular cop. It carries the natural key plus the offences and
// current verdict needed to re-derive the rollup status after a classification
// or custom-cop change. For server results the git fields are empty and vice
// versa.
type CookstyleResultRef struct {
	OrganisationName  string
	CookbookName      string
	CookbookVersion   string
	GitRepoName       string
	GitRepoURL        string
	TargetChefVersion string
	Offences          []byte
	Passed            bool
	CookstyleStatus   string
	ErrorMessage      string
}

// copContainmentArg returns the JSONB containment operand for matching offence
// rows that include the given cop. The offences column is a flat JSONB array of
// enriched offences ([{"cop_name": ...}, ...]); `offences @> '[{"cop_name":"X"}]'`
// is index-friendly and matches any row containing the cop.
func copContainmentArg(copName string) ([]byte, error) {
	return json.Marshal([]map[string]string{{"cop_name": copName}})
}

// ListServerCookbookCookstyleResultsWithCop returns the server cookbook
// cookstyle results for the given target version whose stored offences include
// the named cop. Used to scope re-evaluation to a cop's affected closure.
func (db *DB) ListServerCookbookCookstyleResultsWithCop(ctx context.Context, copName, targetChefVersion string) ([]CookstyleResultRef, error) {
	arg, err := copContainmentArg(copName)
	if err != nil {
		return nil, fmt.Errorf("datastore: building cop containment arg: %w", err)
	}
	const query = `
		SELECT organisation_name, cookbook_name, cookbook_version,
		       COALESCE(target_chef_version, ''),
		       COALESCE(error_message, ''), passed, offences, cookstyle_status
		FROM server_cookbook_cookstyle_results
		WHERE target_chef_version = $1
		  AND offences @> $2::jsonb`

	rows, err := db.q().QueryContext(ctx, query, targetChefVersion, arg)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing server results with cop: %w", err)
	}
	defer rows.Close()

	var refs []CookstyleResultRef
	for rows.Next() {
		var r CookstyleResultRef
		if err := rows.Scan(&r.OrganisationName, &r.CookbookName, &r.CookbookVersion,
			&r.TargetChefVersion, &r.ErrorMessage, &r.Passed, &r.Offences, &r.CookstyleStatus); err != nil {
			return nil, fmt.Errorf("datastore: scanning server result with cop: %w", err)
		}
		refs = append(refs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating server results with cop: %w", err)
	}
	return refs, nil
}

// ListGitRepoCookstyleResultsWithCop returns the git repo cookstyle results for
// the given target version whose stored offences include the named cop.
func (db *DB) ListGitRepoCookstyleResultsWithCop(ctx context.Context, copName, targetChefVersion string) ([]CookstyleResultRef, error) {
	arg, err := copContainmentArg(copName)
	if err != nil {
		return nil, fmt.Errorf("datastore: building cop containment arg: %w", err)
	}
	const query = `
		SELECT git_repo_name, git_repo_url,
		       COALESCE(target_chef_version, ''),
		       COALESCE(error_message, ''), passed, offences, cookstyle_status
		FROM git_repo_cookstyle_results
		WHERE target_chef_version = $1
		  AND offences @> $2::jsonb`

	rows, err := db.q().QueryContext(ctx, query, targetChefVersion, arg)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing git results with cop: %w", err)
	}
	defer rows.Close()

	var refs []CookstyleResultRef
	for rows.Next() {
		var r CookstyleResultRef
		if err := rows.Scan(&r.GitRepoName, &r.GitRepoURL,
			&r.TargetChefVersion, &r.ErrorMessage, &r.Passed, &r.Offences, &r.CookstyleStatus); err != nil {
			return nil, fmt.Errorf("datastore: scanning git result with cop: %w", err)
		}
		refs = append(refs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating git results with cop: %w", err)
	}
	return refs, nil
}

// UpdateServerCookbookCookstyleVerdict updates the materialised verdict (the
// back-compat passed boolean and the classification-derived rollup status) for a
// single server cookbook cookstyle result identified by its natural key.
func (db *DB) UpdateServerCookbookCookstyleVerdict(ctx context.Context, organisationName, cookbookName, cookbookVersion, targetChefVersion string, passed bool, status string) error {
	const query = `
		UPDATE server_cookbook_cookstyle_results
		SET passed = $5, cookstyle_status = $6
		WHERE organisation_name = $1
		  AND cookbook_name = $2
		  AND cookbook_version = $3
		  AND (target_chef_version = $4 OR ($4 = '' AND target_chef_version IS NULL))`
	if _, err := db.q().ExecContext(ctx, query,
		organisationName, cookbookName, cookbookVersion, targetChefVersion, passed, status); err != nil {
		return fmt.Errorf("datastore: updating server cookstyle verdict: %w", err)
	}
	return nil
}

// UpdateGitRepoCookstyleVerdict updates the materialised verdict (passed boolean
// and rollup status) for a single git repo cookstyle result identified by its
// natural key.
func (db *DB) UpdateGitRepoCookstyleVerdict(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string, passed bool, status string) error {
	const query = `
		UPDATE git_repo_cookstyle_results
		SET passed = $4, cookstyle_status = $5
		WHERE git_repo_name = $1
		  AND git_repo_url = $2
		  AND (target_chef_version = $3 OR ($3 = '' AND target_chef_version IS NULL))`
	if _, err := db.q().ExecContext(ctx, query,
		gitRepoName, gitRepoURL, targetChefVersion, passed, status); err != nil {
		return fmt.Errorf("datastore: updating git repo cookstyle verdict: %w", err)
	}
	return nil
}
