// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Result-kind discriminators for cookstyle_offence_fingerprints. They select
// which identity columns are populated on a row.
const (
	FingerprintKindServerCookbook = "server_cookbook"
	FingerprintKindGitRepo        = "git_repo"
)

// FingerprintCopEntry is one per-cop entry in a result's offence fingerprint:
// the minimal projection of the offences JSONB needed to re-derive rollup status
// and weighted complexity under the current classification. It deliberately omits
// offence messages and source locations (re-derivation does not consume them).
// See specifications/estate-progress.md.
type FingerprintCopEntry struct {
	CopName     string `json:"cop_name"`
	Count       int    `json:"count"`
	Severity    string `json:"severity"`
	Correctable bool   `json:"correctable"`
}

// CookstyleOffenceFingerprint represents a row in the
// cookstyle_offence_fingerprints table — one result's fingerprint as of a scan,
// valid from ScannedAt until the next row for the same result.
type CookstyleOffenceFingerprint struct {
	ID                string                `json:"id"`
	ResultKind        string                `json:"result_kind"`
	OrganisationName  string                `json:"organisation_name,omitempty"`
	CookbookName      string                `json:"cookbook_name,omitempty"`
	CookbookVersion   string                `json:"cookbook_version,omitempty"`
	GitRepoName       string                `json:"git_repo_name,omitempty"`
	GitRepoURL        string                `json:"git_repo_url,omitempty"`
	TargetChefVersion string                `json:"target_chef_version,omitempty"`
	FingerprintHash   string                `json:"fingerprint_hash"`
	Cops              []FingerprintCopEntry `json:"cops"`
	ScannedAt         time.Time             `json:"scanned_at"`
	CreatedAt         time.Time             `json:"created_at"`
}

// AppendCookstyleOffenceFingerprintParams identifies a result and carries the
// fingerprint to append. Exactly one identity (server-cookbook or git-repo) is
// populated, selected by ResultKind.
type AppendCookstyleOffenceFingerprintParams struct {
	ResultKind        string
	OrganisationName  string
	CookbookName      string
	CookbookVersion   string
	GitRepoName       string
	GitRepoURL        string
	TargetChefVersion string
	FingerprintHash   string
	Cops              []FingerprintCopEntry
	ScannedAt         time.Time
}

// ---------------------------------------------------------------------------
// Append (change-deduped)
// ---------------------------------------------------------------------------

// AppendCookstyleOffenceFingerprint appends a fingerprint row for a result only
// when it DIFFERS from that result's most recent stored fingerprint. An identical
// rescan appends nothing (the existing row's validity simply extends forward).
// Returns true when a row was appended, false when deduped.
func (db *DB) AppendCookstyleOffenceFingerprint(ctx context.Context, p AppendCookstyleOffenceFingerprintParams) (bool, error) {
	if p.ResultKind != FingerprintKindServerCookbook && p.ResultKind != FingerprintKindGitRepo {
		return false, fmt.Errorf("datastore: invalid fingerprint result_kind %q", p.ResultKind)
	}

	lastHash, err := db.latestFingerprintHash(ctx, p)
	if err != nil {
		return false, err
	}
	if lastHash == p.FingerprintHash {
		return false, nil // deduped — fingerprint unchanged since last scan.
	}

	copsJSON, err := json.Marshal(p.Cops)
	if err != nil {
		return false, fmt.Errorf("datastore: marshalling fingerprint cops: %w", err)
	}

	const query = `
		INSERT INTO cookstyle_offence_fingerprints (
			result_kind, organisation_name, cookbook_name, cookbook_version,
			git_repo_name, git_repo_url, target_chef_version,
			fingerprint_hash, cops, scanned_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err = db.q().ExecContext(ctx, query,
		p.ResultKind,
		nullString(p.OrganisationName),
		nullString(p.CookbookName),
		nullString(p.CookbookVersion),
		nullString(p.GitRepoName),
		nullString(p.GitRepoURL),
		nullString(p.TargetChefVersion),
		p.FingerprintHash,
		copsJSON,
		p.ScannedAt,
	)
	if err != nil {
		return false, fmt.Errorf("datastore: appending cookstyle offence fingerprint: %w", err)
	}
	return true, nil
}

// latestFingerprintHash returns the fingerprint_hash of the most recent row for
// the result identified by p, or "" when no row exists yet.
func (db *DB) latestFingerprintHash(ctx context.Context, p AppendCookstyleOffenceFingerprintParams) (string, error) {
	var (
		query string
		args  []any
	)
	switch p.ResultKind {
	case FingerprintKindServerCookbook:
		query = `
			SELECT fingerprint_hash
			  FROM cookstyle_offence_fingerprints
			 WHERE result_kind = 'server_cookbook'
			   AND organisation_name = $1
			   AND cookbook_name = $2
			   AND cookbook_version = $3
			   AND (target_chef_version = $4 OR ($4 = '' AND target_chef_version IS NULL))
			 ORDER BY scanned_at DESC, created_at DESC
			 LIMIT 1
		`
		args = []any{p.OrganisationName, p.CookbookName, p.CookbookVersion, p.TargetChefVersion}
	default: // git_repo
		query = `
			SELECT fingerprint_hash
			  FROM cookstyle_offence_fingerprints
			 WHERE result_kind = 'git_repo'
			   AND git_repo_name = $1
			   AND git_repo_url = $2
			   AND (target_chef_version = $3 OR ($3 = '' AND target_chef_version IS NULL))
			 ORDER BY scanned_at DESC, created_at DESC
			 LIMIT 1
		`
		args = []any{p.GitRepoName, p.GitRepoURL, p.TargetChefVersion}
	}

	var hash string
	err := db.q().QueryRowContext(ctx, query, args...).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("datastore: reading latest fingerprint hash: %w", err)
	}
	return hash, nil
}

// ---------------------------------------------------------------------------
// List (chronological history for a single result)
// ---------------------------------------------------------------------------

// ListServerCookbookOffenceFingerprints returns every stored fingerprint for the
// given server-cookbook result, oldest first. Trend recompute selects the one
// valid at a point in time (latest with scanned_at <= T) from this history.
func (db *DB) ListServerCookbookOffenceFingerprints(ctx context.Context, org, cookbookName, cookbookVersion, targetChefVersion string) ([]CookstyleOffenceFingerprint, error) {
	const query = `
		SELECT id, result_kind, organisation_name, cookbook_name, cookbook_version,
		       git_repo_name, git_repo_url, target_chef_version,
		       fingerprint_hash, cops, scanned_at, created_at
		  FROM cookstyle_offence_fingerprints
		 WHERE result_kind = 'server_cookbook'
		   AND organisation_name = $1
		   AND cookbook_name = $2
		   AND cookbook_version = $3
		   AND (target_chef_version = $4 OR ($4 = '' AND target_chef_version IS NULL))
		 ORDER BY scanned_at ASC, created_at ASC
	`
	return scanCookstyleOffenceFingerprints(db.q().QueryContext(ctx, query, org, cookbookName, cookbookVersion, targetChefVersion))
}

// ListGitRepoOffenceFingerprints returns every stored fingerprint for the given
// git-repo result, oldest first.
func (db *DB) ListGitRepoOffenceFingerprints(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) ([]CookstyleOffenceFingerprint, error) {
	const query = `
		SELECT id, result_kind, organisation_name, cookbook_name, cookbook_version,
		       git_repo_name, git_repo_url, target_chef_version,
		       fingerprint_hash, cops, scanned_at, created_at
		  FROM cookstyle_offence_fingerprints
		 WHERE result_kind = 'git_repo'
		   AND git_repo_name = $1
		   AND git_repo_url = $2
		   AND (target_chef_version = $3 OR ($3 = '' AND target_chef_version IS NULL))
		 ORDER BY scanned_at ASC, created_at ASC
	`
	return scanCookstyleOffenceFingerprints(db.q().QueryContext(ctx, query, gitRepoName, gitRepoURL, targetChefVersion))
}

// ---------------------------------------------------------------------------
// Bulk list for trend recompute
// ---------------------------------------------------------------------------

// ListOffenceFingerprintsByTarget returns every stored fingerprint row for the
// given target Chef version across ALL results, ordered by result identity then
// scanned_at ascending. This is the bulk feed for trend recompute: the caller
// groups consecutive rows by identity into per-result histories (oldest-first)
// rather than issuing one query per result. An empty target matches rows whose
// target_chef_version is NULL/empty.
//
// The ORDER BY mirrors the per-result identity columns so a single linear pass
// can split the rows into contiguous per-result runs.
func (db *DB) ListOffenceFingerprintsByTarget(ctx context.Context, targetChefVersion string) ([]CookstyleOffenceFingerprint, error) {
	const query = `
		SELECT id, result_kind, organisation_name, cookbook_name, cookbook_version,
		       git_repo_name, git_repo_url, target_chef_version,
		       fingerprint_hash, cops, scanned_at, created_at
		  FROM cookstyle_offence_fingerprints
		 WHERE (target_chef_version = $1 OR ($1 = '' AND target_chef_version IS NULL))
		 ORDER BY result_kind,
		          organisation_name, cookbook_name, cookbook_version,
		          git_repo_name, git_repo_url,
		          scanned_at ASC, created_at ASC
	`
	return scanCookstyleOffenceFingerprints(db.q().QueryContext(ctx, query, targetChefVersion))
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanCookstyleOffenceFingerprint(row interface{ Scan(dest ...any) error }) (CookstyleOffenceFingerprint, error) {
	var r CookstyleOffenceFingerprint
	var org, cbName, cbVer, repoName, repoURL, target sql.NullString
	var cops []byte

	err := row.Scan(
		&r.ID,
		&r.ResultKind,
		&org,
		&cbName,
		&cbVer,
		&repoName,
		&repoURL,
		&target,
		&r.FingerprintHash,
		&cops,
		&r.ScannedAt,
		&r.CreatedAt,
	)
	if err != nil {
		return CookstyleOffenceFingerprint{}, err
	}

	r.OrganisationName = stringFromNull(org)
	r.CookbookName = stringFromNull(cbName)
	r.CookbookVersion = stringFromNull(cbVer)
	r.GitRepoName = stringFromNull(repoName)
	r.GitRepoURL = stringFromNull(repoURL)
	r.TargetChefVersion = stringFromNull(target)
	if len(cops) > 0 {
		if err := json.Unmarshal(cops, &r.Cops); err != nil {
			return CookstyleOffenceFingerprint{}, fmt.Errorf("datastore: unmarshalling fingerprint cops: %w", err)
		}
	}
	return r, nil
}

func scanCookstyleOffenceFingerprints(rows *sql.Rows, err error) ([]CookstyleOffenceFingerprint, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying cookstyle offence fingerprints: %w", err)
	}
	defer rows.Close()

	var results []CookstyleOffenceFingerprint
	for rows.Next() {
		r, err := scanCookstyleOffenceFingerprint(rows)
		if err != nil {
			return nil, fmt.Errorf("datastore: scanning cookstyle offence fingerprint row: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookstyle offence fingerprint rows: %w", err)
	}
	return results, nil
}
