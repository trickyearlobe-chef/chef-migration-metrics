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
// the data into an editable DB. See journeys/scan-trust.md
// (Curation linter) — it flags for manual resolution, it does NOT auto-demote.
const (
	// CurationStale — a curated entry (any, not only verified-removals) whose cop
	// the binary no longer emits (removed or renamed upstream). Its mapping is
	// dead until corrected: a real offence from the correctly-named cop gets no
	// remediation doc. Checked for every entry so Review-level mappings can't rot
	// silently (the 2026-07-16 audit found 8 stale Review entries this guard now
	// catches).
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

// ValidateCuratedRemovals cross-checks every curated mapping entry against the
// live cop registry:
//
//   - stale: the cop is absent from the binary (removed or renamed upstream).
//     Checked for EVERY entry — a dead cop name means the mapping never matches a
//     real offence, so its remediation doc silently never shows. (Review-level
//     entries, with an empty RemovedIn, were previously exempt and rotted
//     undetected; the 2026-07-16 audit found 8.)
//   - removal_disagreement: the shipped description states a removal version
//     whose major disagrees with the curated RemovedIn. Only meaningful for a
//     verified-removal claim, so it is checked only when RemovedIn is set.
//
// A missing or unparseable removal version in the description is NOT an error
// (only ~30% of descriptions cleanly state one), so the disagreement check is
// conservative: it flags only clear contradictions. Results are in mapping order.
func ValidateCuratedRemovals(mappings []remediation.CopMapping, reg CopDescriptionLister) []CurationIssue {
	var issues []CurationIssue
	for _, m := range mappings {
		entry, ok := reg.Lookup(m.CopName)
		if !ok {
			issues = append(issues, CurationIssue{
				CopName: m.CopName,
				Kind:    CurationStale,
				Detail:  "curated cop is not emitted by the cookstyle binary (removed or renamed upstream); its remediation mapping is dead until corrected",
			})
			continue
		}
		// Beyond staleness, only a verified-removal claim (RemovedIn set) can
		// disagree with the description's stated removal version.
		if m.RemovedIn == "" {
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
