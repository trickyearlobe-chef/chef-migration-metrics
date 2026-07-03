// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"fmt"
	"regexp"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// Curation-issue kinds. The curation linter is the durability mechanism for the
// curated verified-removal set: it cross-checks each curated RemovedIn against
// the shipped cookstyle binary and flags rot at the source instead of moving
// the data into an editable DB. See specifications/cop-classification.md
// (Curation linter) — it flags for manual resolution, it does NOT auto-demote.
const (
	// CurationStale — a curated RemovedIn entry whose cop the binary no longer
	// emits (removed or renamed upstream). Its mapping is dead until corrected.
	CurationStale = "stale"
	// CurationRemovalDisagreement — the shipped cop description states an
	// explicit removal version that disagrees with the curated RemovedIn.
	CurationRemovalDisagreement = "removal_disagreement"
)

// CurationIssue is one problem found by the curation linter.
type CurationIssue struct {
	CopName string
	Kind    string
	Detail  string
}

// CopDescriptionLister is the cop-registry subset the curation linter needs:
// look a cop up by name and read its shipped description. *CopRegistry
// satisfies it.
type CopDescriptionLister interface {
	Lookup(name string) (CopRegistryEntry, bool)
}

// descriptionRemovalRe extracts an explicit *removal* version from a cop
// description, e.g. "removed in Chef 14", "will be removed in Chef Infra Client
// 15", "removed in 16.0". It is deliberately anchored on "removed in" so it does
// not trip on "deprecated in"/"introduced in" versions (the deprecation-vs-
// removal trap noted in the 2026-07-03 --show-cops parseability spike).
var descriptionRemovalRe = regexp.MustCompile(`(?i)removed in (?:chef(?: infra)?(?: client)?\s+)?(\d+)`)

// ValidateCuratedRemovals cross-checks every curated RemovedIn mapping entry
// against the live cop registry:
//
//   - stale: the cop is absent from the binary (removed or renamed upstream).
//   - removal_disagreement: the shipped description states a removal version
//     whose major disagrees with the curated RemovedIn.
//
// A missing or unparseable removal version in the description is NOT an error
// (only ~30% of descriptions cleanly state one), so the check is conservative:
// it flags only clear contradictions and clear staleness. Entries with an empty
// RemovedIn are ignored — they are not verified-removal claims. Results are in
// mapping order.
func ValidateCuratedRemovals(mappings []remediation.CopMapping, reg CopDescriptionLister) []CurationIssue {
	var issues []CurationIssue
	for _, m := range mappings {
		if m.RemovedIn == "" {
			continue
		}
		entry, ok := reg.Lookup(m.CopName)
		if !ok {
			issues = append(issues, CurationIssue{
				CopName: m.CopName,
				Kind:    CurationStale,
				Detail:  "curated RemovedIn entry but the cop is not emitted by the cookstyle binary (removed or renamed upstream)",
			})
			continue
		}
		curatedMajor := parseMajorVersion(m.RemovedIn)
		if descMajor, found := parseDescriptionRemovalMajor(entry.Description); found && descMajor != curatedMajor {
			issues = append(issues, CurationIssue{
				CopName: m.CopName,
				Kind:    CurationRemovalDisagreement,
				Detail: fmt.Sprintf("curated RemovedIn %s (Chef %d) but the cop description implies removal in Chef %d",
					m.RemovedIn, curatedMajor, descMajor),
			})
		}
	}
	return issues
}

// parseDescriptionRemovalMajor returns the major Chef version an explicit
// "removed in …" clause in the description names, and whether one was found.
func parseDescriptionRemovalMajor(description string) (int, bool) {
	m := descriptionRemovalRe.FindStringSubmatch(description)
	if m == nil {
		return 0, false
	}
	return parseMajorVersion(m[1]), true
}
