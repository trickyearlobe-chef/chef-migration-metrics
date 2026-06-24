// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// CopClassificationLevel represents the migration impact of a cop.
const (
	ClassificationBlocker      = "blocker"
	ClassificationReview       = "review"
	ClassificationNoise        = "noise"
	ClassificationUnclassified = "unclassified"
)

// ClassificationSource describes how a classification was determined.
const (
	SourceOperatorOverride = "operator_override"
	SourceRemovedIn        = "removed_in"
	SourceCuratedDefault   = "curated_default"
	SourceUnclassified     = "unclassified"
)

// ResolvedClassification holds the result of classification resolution for a cop.
type ResolvedClassification struct {
	Classification string // blocker, review, noise, unclassified
	Source         string // operator_override, removed_in, curated_default, unclassified
}

// CopClassificationResolver resolves the effective classification for a cop
// at a given target version. It checks sources in priority order:
// 1. Operator overrides (from DB)
// 2. RemovedIn auto-seed (from cop mapping)
// 3. Curated defaults (shipped with application)
// 4. Unclassified (fallback)
type CopClassificationResolver struct {
	// OperatorOverrides maps cop_name → classification for the active target version.
	OperatorOverrides map[string]string
	// TargetChefVersion is the version being migrated to.
	TargetChefVersion string
}

// Resolve determines the classification for a given cop name.
func (r *CopClassificationResolver) Resolve(copName string) ResolvedClassification {
	// 1. Operator override (highest priority)
	if class, ok := r.OperatorOverrides[copName]; ok {
		return ResolvedClassification{Classification: class, Source: SourceOperatorOverride}
	}

	// 2. RemovedIn from cop mapping: if removed_in <= target version, it's a blocker
	if mapping := remediation.LookupCop(copName); mapping != nil && mapping.RemovedIn != "" {
		if versionLessOrEqual(mapping.RemovedIn, r.TargetChefVersion) {
			return ResolvedClassification{Classification: ClassificationBlocker, Source: SourceRemovedIn}
		}
	}

	// 3. Curated defaults
	if entry, ok := curatedDefaults[copName]; ok {
		if entry.MinTargetVersion == "" || versionLessOrEqual(entry.MinTargetVersion, r.TargetChefVersion) {
			return ResolvedClassification{Classification: entry.Classification, Source: SourceCuratedDefault}
		}
	}

	// 4. Unclassified
	return ResolvedClassification{Classification: ClassificationUnclassified, Source: SourceUnclassified}
}

// IsBlocker returns true if the given cop is classified as a blocker at the target version.
func (r *CopClassificationResolver) IsBlocker(copName string) bool {
	return r.Resolve(copName).Classification == ClassificationBlocker
}

// EvaluatePassFailWithClassification evaluates whether a set of offenses
// passes, using cop classification when available and falling back to
// severity-based failure rules for unclassified cops.
func EvaluatePassFailWithClassification(offenses []CookstyleOffense, rules CookstyleFailureRules, resolver *CopClassificationResolver) bool {
	for i := range offenses {
		off := &offenses[i]

		resolved := resolver.Resolve(off.CopName)
		switch resolved.Classification {
		case ClassificationBlocker:
			return false
		case ClassificationReview, ClassificationNoise:
			continue
		default:
			// Unclassified: fall back to severity-based rules
			if offenseTriggersFailure(off, &rules) {
				return false
			}
		}
	}
	return true
}

// versionLessOrEqual compares two Chef version strings. Returns true if a <= b.
// Uses simple numeric comparison of major versions for robustness.
func versionLessOrEqual(a, b string) bool {
	aMajor := parseMajorVersion(a)
	bMajor := parseMajorVersion(b)
	if aMajor != bMajor {
		return aMajor <= bMajor
	}
	// Same major: compare full string lexicographically as a rough minor comparison.
	return a <= b
}

// parseMajorVersion extracts the leading integer from a version string.
func parseMajorVersion(v string) int {
	n := 0
	for _, ch := range v {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int(ch-'0')
		} else {
			break
		}
	}
	return n
}
