// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import "sort"

// StaticCopSource names a cop that appears in a static (compiled Go) table,
// tagged with which table it lives in. The caller assembles these from the
// curated defaults and the RemovedIn mapping so this package need not import
// the remediation mapping table.
type StaticCopSource struct {
	CopName string
	Source  string
}

// Provenance of a static cop entry, reported on stale findings so an operator
// knows which table to edit. The verified-removal mapping is the only
// enumerable static table (the structural-Noise rules classify whole
// namespaces and have no concrete cop names to go stale).
const (
	StaticSourceMapping = "removed_in_mapping"
)

// StaleCopEntry is a static-table cop the running binary no longer emits — a
// mapping/curated entry that has outlived the cop it describes.
type StaleCopEntry struct {
	CopName string `json:"cop_name"`
	Source  string `json:"source"`
}

// CoverageGapEntry is a live Chef/* cop that nothing specifically classifies —
// it falls through to the Review default. Surfacing it lets an operator triage
// a newly shipped cop rather than leaving it silently on the default.
type CoverageGapEntry struct {
	CopName    string `json:"cop_name"`
	Department string `json:"department"`
	Enabled    bool   `json:"enabled"`
}

// CopDriftReport cross-references the live cop registry against the static
// classification tables: stale entries the binary dropped, and Chef/* coverage
// gaps the binary added but nothing classifies.
type CopDriftReport struct {
	RegistryAvailable bool               `json:"registry_available"`
	RegistryVersion   string             `json:"registry_version"`
	Stale             []StaleCopEntry    `json:"stale"`
	CoverageGaps      []CoverageGapEntry `json:"coverage_gaps"`
}

// ComputeCopDrift produces the drift report. A nil registry (binary unavailable
// / --show-cops failed) degrades to RegistryAvailable=false with no findings —
// the static universe still stands, drift is simply unknown this run. Results
// are sorted by cop name for stable output.
func ComputeCopDrift(reg *CopRegistry, resolver *CopClassificationResolver, static []StaticCopSource) CopDriftReport {
	if reg == nil {
		return CopDriftReport{RegistryAvailable: false}
	}

	report := CopDriftReport{RegistryAvailable: true, RegistryVersion: reg.Version()}

	// Stale: a static-table cop the binary no longer emits. De-dupe by name so a
	// cop present in both the curated and mapping tables reports once (curated
	// wins as the more specific provenance, matching resolver precedence).
	seen := make(map[string]bool)
	for _, s := range static {
		if reg.Has(s.CopName) || seen[s.CopName] {
			continue
		}
		seen[s.CopName] = true
		report.Stale = append(report.Stale, StaleCopEntry(s))
	}

	// Coverage gaps: live Chef/* cops that nothing specifically classifies —
	// they fall through to the Review default.
	for _, e := range reg.ChefCops() {
		if resolver.Resolve(e.CopName).Source == SourceReviewDefault {
			report.CoverageGaps = append(report.CoverageGaps, CoverageGapEntry{
				CopName:    e.CopName,
				Department: e.Department,
				Enabled:    e.Enabled,
			})
		}
	}

	sort.Slice(report.Stale, func(i, j int) bool { return report.Stale[i].CopName < report.Stale[j].CopName })
	sort.Slice(report.CoverageGaps, func(i, j int) bool { return report.CoverageGaps[i].CopName < report.CoverageGaps[j].CopName })
	return report
}
