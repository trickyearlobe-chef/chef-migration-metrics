// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// OwnerDuplicateCandidate is one pair of owners that may be the same person.
// It is a lead for somebody to recognise, never a match: the pair is reported
// with the text that made it look similar so a reader can judge it.
type OwnerDuplicateCandidate struct {
	OwnerA string `json:"owner_a"`
	OwnerB string `json:"owner_b"`

	// MatchedOn is "name" when the two owner names are similar,
	// "display_name" when the names people see are, and "alias" when two of
	// their recorded identities are.
	MatchedOn string `json:"matched_on"`

	// ValueA and ValueB are the two strings that matched.
	ValueA string `json:"value_a"`
	ValueB string `json:"value_b"`

	Similarity float64 `json:"similarity"`

	// AssignmentsA and AssignmentsB say how much work each side holds, which
	// is what decides which way round a merge should go.
	AssignmentsA int `json:"assignments_a"`
	AssignmentsB int `json:"assignments_b"`
}

// OwnerDuplicateFilter bounds a read of the stored candidates.
type OwnerDuplicateFilter struct {
	MinSimilarity float64
	Limit         int
	Offset        int
}

// OwnerDuplicateScan records when the catalogue was last scanned.
type OwnerDuplicateScan struct {
	ScannedAt  time.Time `json:"scanned_at"`
	PairsFound int       `json:"pairs_found"`
}

// duplicateScanNeighbours is how many nearest candidates each owner and each
// alias contributes.
//
// This is what makes the scan bounded. Comparing everything with everything
// is quadratic in how densely names cluster, and owner names cluster hard —
// shared surnames, and committer-derived names that are email localparts. A
// live sweep of a ten-thousand-owner catalogue was measured in minutes; the
// nearest-neighbour form is seconds and does not degrade with density.
//
// The cost of the bound: a person with more than this many near-twins keeps
// only the closest few. That is the right trade for a list somebody reads —
// the twentieth-best guess is noise — but it is a bound, not completeness.
const duplicateScanNeighbours = 5

// duplicateScanFloor is the similarity below which a pair is not worth
// recording. pg_trgm's own default threshold is the same value.
const duplicateScanFloor = 0.3

// RecomputeOwnerDuplicateCandidates rebuilds the stored candidate list and
// returns how many pairs it found.
//
// Two signals, both bounded to the nearest few per row: similar owner names,
// which covers every owner including those carrying no alias at all, and
// similar alias values, which catches people whose names share nothing but
// who are recorded under near-identical identities.
func (db *DB) RecomputeOwnerDuplicateCandidates(ctx context.Context) (int, error) {
	const insertQuery = `
		INSERT INTO owner_duplicate_candidates
			(owner_a, owner_b, matched_on, value_a, value_b, similarity)
		SELECT DISTINCT ON (owner_a, owner_b)
			owner_a, owner_b, matched_on, value_a, value_b, sim
		FROM (
			SELECT LEAST(a.name, near.name)    AS owner_a,
			       GREATEST(a.name, near.name) AS owner_b,
			       'name'                      AS matched_on,
			       LEAST(a.name, near.name)    AS value_a,
			       GREATEST(a.name, near.name) AS value_b,
			       near.sim                    AS sim
			FROM owners a
			CROSS JOIN LATERAL (
				SELECT b.name, similarity(a.name, b.name) AS sim
				FROM owners b
				WHERE b.name <> a.name
				ORDER BY b.name <-> a.name
				LIMIT $1
			) near
			WHERE near.sim >= $2

			UNION ALL

			-- Two owners under one display name. The committer path produces
			-- exactly this: one person committing under two addresses becomes
			-- two owners with unrelated names and one identical display name,
			-- which neither of the other two signals can see.
			SELECT LEAST(a.name, near.name),
			       GREATEST(a.name, near.name),
			       'display_name',
			       CASE WHEN a.name < near.name THEN a.display_name ELSE near.display_name END,
			       CASE WHEN a.name < near.name THEN near.display_name ELSE a.display_name END,
			       near.sim
			FROM owners a
			CROSS JOIN LATERAL (
				SELECT b.name, b.display_name,
				       similarity(a.display_name, b.display_name) AS sim
				FROM owners b
				WHERE b.name <> a.name AND b.display_name IS NOT NULL
				ORDER BY b.display_name <-> a.display_name
				LIMIT $1
			) near
			WHERE a.display_name IS NOT NULL AND near.sim >= $2

			UNION ALL

			-- Compared on the part that identifies a person, not the part
			-- every colleague shares.
			--
			-- Everyone at one company has the same email domain, and a shared
			-- domain is most of a shared string: three names with nothing in
			-- common scored 0.33-0.49 against a 0.3 floor purely on
			-- "@example-corp.test". Left whole, this signal pairs every owner
			-- with its nearest few and drowns the real duplicates it exists to
			-- surface — worse the larger the catalogue, which is backwards.
			--
			-- The nearest-neighbour ordering still uses the whole value, so the
			-- GiST index is still what bounds the scan; only the score that
			-- decides whether a pair is worth recording is taken on the
			-- localpart. The values reported are the originals, because a
			-- reader needs to see the addresses to judge the pair.
			SELECT LEAST(x.owner_name, near.owner_name),
			       GREATEST(x.owner_name, near.owner_name),
			       'alias',
			       CASE WHEN x.owner_name < near.owner_name THEN x.alias_value ELSE near.alias_value END,
			       CASE WHEN x.owner_name < near.owner_name THEN near.alias_value ELSE x.alias_value END,
			       near.sim
			FROM owner_aliases x
			CROSS JOIN LATERAL (
				SELECT z.owner_name, z.alias_value,
				       similarity(split_part(x.alias_value, '@', 1),
				                  split_part(z.alias_value, '@', 1)) AS sim
				FROM owner_aliases z
				WHERE z.owner_name <> x.owner_name
				ORDER BY z.alias_value <-> x.alias_value
				LIMIT $1
			) near
			WHERE near.sim >= $2
		) candidates
		ORDER BY owner_a, owner_b, sim DESC
	`

	// An owner created since the last scan may carry a contact address that
	// has never reached the alias table, and the scan is the moment that
	// matters — an identity we hold but do not index is invisible to it.
	if _, err := db.SeedAliasesFromContactEmails(ctx); err != nil {
		return 0, err
	}

	var found int
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		// Readers keep the previous result until this commits, so the list is
		// never briefly empty while a scan is running.
		if _, err := tx.ExecContext(ctx, `DELETE FROM owner_duplicate_candidates`); err != nil {
			return fmt.Errorf("clearing previous duplicate candidates: %w", err)
		}

		res, err := tx.ExecContext(ctx, insertQuery, duplicateScanNeighbours, duplicateScanFloor)
		if err != nil {
			return fmt.Errorf("scanning for duplicate candidates: %w", err)
		}
		inserted, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("counting duplicate candidates: %w", err)
		}
		found = int(inserted)

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO owner_duplicate_scans (only_row, scanned_at, pairs_found)
			VALUES (TRUE, now(), $1)
			ON CONFLICT (only_row) DO UPDATE
			SET scanned_at = EXCLUDED.scanned_at, pairs_found = EXCLUDED.pairs_found
		`, found); err != nil {
			return fmt.Errorf("recording the duplicate scan: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("datastore: %w", err)
	}
	return found, nil
}

// GetOwnerDuplicateScan returns when the catalogue was last scanned. Returns
// ErrNotFound if it never has been, which is not the same as a scan that
// found nothing.
func (db *DB) GetOwnerDuplicateScan(ctx context.Context) (OwnerDuplicateScan, error) {
	var s OwnerDuplicateScan
	err := db.pool.QueryRowContext(ctx,
		`SELECT scanned_at, pairs_found FROM owner_duplicate_scans WHERE only_row`,
	).Scan(&s.ScannedAt, &s.PairsFound)
	if errors.Is(err, sql.ErrNoRows) {
		return OwnerDuplicateScan{}, ErrNotFound
	}
	if err != nil {
		return OwnerDuplicateScan{}, fmt.Errorf("datastore: reading the duplicate scan record: %w", err)
	}
	return s, nil
}

// ListOwnerDuplicateCandidates returns stored candidate pairs, strongest
// first, with the total number stored above the filter's floor.
func (db *DB) ListOwnerDuplicateCandidates(ctx context.Context, f OwnerDuplicateFilter) ([]OwnerDuplicateCandidate, int, error) {
	minSimilarity := f.MinSimilarity
	if minSimilarity < duplicateScanFloor {
		minSimilarity = duplicateScanFloor
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 25
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	var total int
	// The total has to agree with the rows, so it excludes dismissals too.
	if err := db.pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM owner_duplicate_candidates d
		 WHERE d.similarity >= $1
		   AND NOT EXISTS (
		       SELECT 1 FROM owner_duplicate_dismissals x
		       WHERE x.owner_a = d.owner_a AND x.owner_b = d.owner_b
		   )`,
		minSimilarity,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("datastore: counting duplicate owner candidates: %w", err)
	}

	const dataQuery = `
		WITH counts AS (
			SELECT owner_name, COUNT(*) AS n FROM ownership_assignments GROUP BY owner_name
		)
		SELECT d.owner_a, d.owner_b, d.matched_on, d.value_a, d.value_b, d.similarity,
		       COALESCE(ca.n, 0), COALESCE(cb.n, 0)
		FROM owner_duplicate_candidates d
		LEFT JOIN counts ca ON ca.owner_name = d.owner_a
		LEFT JOIN counts cb ON cb.owner_name = d.owner_b
		WHERE d.similarity >= $1
		  AND NOT EXISTS (
		      SELECT 1 FROM owner_duplicate_dismissals x
		      WHERE x.owner_a = d.owner_a AND x.owner_b = d.owner_b
		  )
		ORDER BY d.similarity DESC, d.owner_a, d.owner_b
		LIMIT $2 OFFSET $3
	`

	rows, err := db.pool.QueryContext(ctx, dataQuery, minSimilarity, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: listing duplicate owner candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []OwnerDuplicateCandidate
	for rows.Next() {
		var c OwnerDuplicateCandidate
		if err := rows.Scan(&c.OwnerA, &c.OwnerB, &c.MatchedOn, &c.ValueA, &c.ValueB,
			&c.Similarity, &c.AssignmentsA, &c.AssignmentsB); err != nil {
			return nil, 0, fmt.Errorf("datastore: scanning duplicate owner candidate: %w", err)
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// CountOwnersMissingAliases returns the number of owners and how many of them
// have no alias recorded at all.
//
// The scan compares owner names as well as aliases, so an owner with no alias
// is still visible — but only under the one name it was created with. The
// counts let a reader see how much of the catalogue that applies to rather
// than being told the report is complete.
func (db *DB) CountOwnersMissingAliases(ctx context.Context) (total, missing int, err error) {
	const query = `
		SELECT COUNT(*),
		       COUNT(*) FILTER (
		           WHERE NOT EXISTS (SELECT 1 FROM owner_aliases a WHERE a.owner_name = o.name)
		       )
		FROM owners o
	`
	if err := db.pool.QueryRowContext(ctx, query).Scan(&total, &missing); err != nil {
		return 0, 0, fmt.Errorf("datastore: counting owners without aliases: %w", err)
	}
	return total, missing, nil
}

// DismissOwnerDuplicate records that two owners are not the same person.
//
// Kept apart from the candidate table on purpose. The scan deletes and rebuilds
// that table on every run, so a dismissal stored there would be swept away and
// the pair would return — which is the complaint this answers. A dismissal is a
// judgement about the scan's output rather than part of it, and it outlives any
// number of rescans.
//
// The pair is ordered before storing, so it matches however the caller happens
// to name the two. Saying it twice is not an error: a reader clicking again has
// changed nothing, and failing there would be noise.
func (db *DB) DismissOwnerDuplicate(ctx context.Context, ownerA, ownerB, reason, dismissedBy string) error {
	if ownerA == ownerB {
		return errors.New("datastore: an owner cannot be dismissed as a duplicate of itself")
	}
	if strings.TrimSpace(dismissedBy) == "" {
		return errors.New("datastore: dismissing needs to say who decided it")
	}

	// Same ordering the candidate table uses.
	if ownerA > ownerB {
		ownerA, ownerB = ownerB, ownerA
	}

	_, err := db.pool.ExecContext(ctx, `
		INSERT INTO owner_duplicate_dismissals (owner_a, owner_b, reason, dismissed_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (owner_a, owner_b) DO UPDATE
		SET reason = EXCLUDED.reason,
		    dismissed_by = EXCLUDED.dismissed_by,
		    dismissed_at = now()
	`, ownerA, ownerB, nullStringPtr(strings.TrimSpace(reason)), dismissedBy)
	if err != nil {
		return fmt.Errorf("datastore: dismissing a duplicate pair: %w", err)
	}
	return nil
}

// CountOwnerDuplicateDismissals returns how many pairs have been rejected, so a
// reader can tell an empty list that was worked down from one nobody has looked
// at.
func (db *DB) CountOwnerDuplicateDismissals(ctx context.Context) (int, error) {
	var n int
	if err := db.pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM owner_duplicate_dismissals`).Scan(&n); err != nil {
		return 0, fmt.Errorf("datastore: counting dismissed duplicate pairs: %w", err)
	}
	return n, nil
}

// OwnerDuplicateDismissal is a pair somebody has said are different people.
type OwnerDuplicateDismissal struct {
	OwnerA      string    `json:"owner_a"`
	OwnerB      string    `json:"owner_b"`
	Reason      string    `json:"reason,omitempty"`
	DismissedBy string    `json:"dismissed_by"`
	DismissedAt time.Time `json:"dismissed_at"`
}

// ListOwnerDuplicateDismissals returns every rejected pair, most recent first.
//
// A dismissed pair is hidden from the candidate list, so without this there is
// nothing to click to undo one — a mis-click would suppress a pair permanently
// and invisibly, which is worse than the problem dismissing solved.
func (db *DB) ListOwnerDuplicateDismissals(ctx context.Context) ([]OwnerDuplicateDismissal, error) {
	rows, err := db.pool.QueryContext(ctx, `
		SELECT owner_a, owner_b, COALESCE(reason, ''), dismissed_by, dismissed_at
		FROM owner_duplicate_dismissals
		ORDER BY dismissed_at DESC, owner_a, owner_b
	`)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing dismissed duplicate pairs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []OwnerDuplicateDismissal
	for rows.Next() {
		var d OwnerDuplicateDismissal
		if err := rows.Scan(&d.OwnerA, &d.OwnerB, &d.Reason, &d.DismissedBy, &d.DismissedAt); err != nil {
			return nil, fmt.Errorf("datastore: scanning a dismissed pair: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RestoreOwnerDuplicate undoes a dismissal, so the pair is offered again if the
// scan still considers the two similar.
//
// Removing the dismissal is all this does: it does not assert the pair is a
// duplicate, only that nobody has ruled on it. If the scan no longer pairs them
// — because the data or the scoring has moved on — the pair stays absent, which
// is the honest outcome.
//
// Undoing something already undone is not an error: a second click has changed
// nothing, and failing there would be noise.
func (db *DB) RestoreOwnerDuplicate(ctx context.Context, ownerA, ownerB string) error {
	if ownerA > ownerB {
		ownerA, ownerB = ownerB, ownerA
	}
	if _, err := db.pool.ExecContext(ctx,
		`DELETE FROM owner_duplicate_dismissals WHERE owner_a = $1 AND owner_b = $2`,
		ownerA, ownerB,
	); err != nil {
		return fmt.Errorf("datastore: undoing a dismissal: %w", err)
	}
	return nil
}
