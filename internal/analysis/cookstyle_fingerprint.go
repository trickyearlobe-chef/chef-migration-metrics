// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// BuildOffenceFingerprint projects a result's offences into the minimal,
// change-deduped fingerprint stored in cookstyle_offence_fingerprints: per-cop
// (cop_name, severity, correctable) entries with an occurrence count, plus a
// stable content hash for cheap dedupe.
//
// The projection is classification-INDEPENDENT on purpose — it retains the raw
// inputs the classification resolver and complexity weighting consume, so a trend
// point can be re-derived under whatever classification is current at recompute
// time. It is the same set of offences DeriveCookstyleStatus consumes at scan time
// (sr.Offenses). Messages are intentionally dropped.
//
// Source locations are dropped too, but the ANSWER the location gives is not:
// each entry records how many of its occurrences sat outside cookbook code, so a
// re-derived verdict can honour scope without the paths themselves. Recording
// only the total would have made the recompute path re-block every cookbook the
// scan-time path correctly passed.
//
// The returned hash is order-independent: the same multiset of offences always
// yields the same entries and hash regardless of input ordering.
func BuildOffenceFingerprint(offenses []CookstyleOffense) ([]datastore.FingerprintCopEntry, string) {
	return BuildOffenceFingerprintInScope(offenses, DefaultScanScope())
}

// BuildOffenceFingerprintInScope is BuildOffenceFingerprint with an explicit
// scan scope.
func BuildOffenceFingerprintInScope(offenses []CookstyleOffense, scope *ScanScope) ([]datastore.FingerprintCopEntry, string) {
	type key struct {
		cop         string
		severity    string
		correctable bool
	}
	counts := make(map[key]int, len(offenses))
	excluded := make(map[key]int, len(offenses))
	for i := range offenses {
		off := &offenses[i]
		k := key{cop: off.CopName, severity: off.Severity, correctable: off.Corrected}
		counts[k]++
		if scope.ExcludesOffense(*off) {
			excluded[k]++
		}
	}

	entries := make([]datastore.FingerprintCopEntry, 0, len(counts))
	for k, n := range counts {
		entries = append(entries, datastore.FingerprintCopEntry{
			CopName:       k.cop,
			Count:         n,
			ExcludedCount: excluded[k],
			Severity:      k.severity,
			Correctable:   k.correctable,
		})
	}

	// Deterministic ordering so the hash (and stored JSON) is stable across runs.
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.CopName != b.CopName {
			return a.CopName < b.CopName
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if a.Correctable != b.Correctable {
			return !a.Correctable // false sorts before true
		}
		return a.Count < b.Count
	})

	return entries, hashFingerprintEntries(entries)
}

// hashFingerprintEntries returns the sha256 hex of the canonical JSON encoding of
// the (already sorted) entries. An empty slice hashes to the digest of "[]" so a
// clean scan has a stable, non-empty fingerprint.
func hashFingerprintEntries(entries []datastore.FingerprintCopEntry) string {
	canonical, err := json.Marshal(entries)
	if err != nil {
		// FingerprintCopEntry contains only string/int/bool fields, so marshalling
		// cannot fail; fall back to an empty-array digest rather than panicking.
		canonical = []byte("[]")
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
