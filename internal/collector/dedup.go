// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import "github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"

// deduplicateSnapshotParams removes duplicate entries from a slice of
// InsertNodeSnapshotParams, keyed by (OrganisationID, NodeName). When
// duplicates exist the last occurrence wins — this matches the semantics
// of the Chef Server partial search, where later pages may contain
// fresher data for the same node.
//
// PostgreSQL's INSERT ... ON CONFLICT DO UPDATE rejects two rows with the
// same conflict key in a single statement (error 21000). This function
// prevents that by collapsing duplicates before the batch is sent to the
// database.
//
// The returned slice preserves the relative order of first-seen keys
// (stable output) with the values from the last occurrence.
func deduplicateSnapshotParams(params []datastore.InsertNodeSnapshotParams) ([]datastore.InsertNodeSnapshotParams, int) {
	if len(params) == 0 {
		return params, 0
	}

	type dedupKey struct {
		OrgID    string
		NodeName string
	}

	// First pass: record the index of each key's last occurrence and
	// capture the value we want to keep.
	lastIndex := make(map[dedupKey]int, len(params))
	for i, p := range params {
		lastIndex[dedupKey{p.OrganisationID, p.NodeName}] = i
	}

	// No duplicates — fast path.
	if len(lastIndex) == len(params) {
		return params, 0
	}

	// Second pass: walk forward, emitting each key only at its last
	// position. This preserves the original ordering of keys (by their
	// final occurrence) and keeps the last value for each key.
	seen := make(map[dedupKey]bool, len(lastIndex))
	result := make([]datastore.InsertNodeSnapshotParams, 0, len(lastIndex))

	for i := len(params) - 1; i >= 0; i-- {
		k := dedupKey{params[i].OrganisationID, params[i].NodeName}
		if seen[k] {
			continue
		}
		seen[k] = true
		result = append(result, params[i])
	}

	// Reverse to restore forward order.
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}

	duplicates := len(params) - len(result)
	return result, duplicates
}
