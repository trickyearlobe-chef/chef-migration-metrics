// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

// CookStyle rollup status — the classification-derived, single-source-of-truth
// verdict for a cookbook / repo / node × target version. These wire values are
// consumed by every read surface (lists, summary cards, detail headers, node
// readiness, exports, trends). See specifications/cop-classification.md
// (CookStyle Rollup Status).
const (
	StatusReady       = "ready"        // 🟢 no blockers, no review-level offenses
	StatusNeedsReview = "needs_review" // 🟠 no blockers, ≥1 review-level offense
	StatusBlocked     = "blocked"      // 🔴 ≥1 blocker, or unclassified offense that severity-fails
	// Untested (⚪, no scan result for this unit + target) reuses the existing
	// StatusUntested constant defined in readiness.go; it is caller-assigned
	// when no scan result exists, not produced by DeriveCookstyleStatus.
)

// DeriveCookstyleStatus computes the rollup status for a set of offenses using
// resolved cop classification, falling back to severity-based failure rules for
// unclassified cops only. This is the single derivation every surface consumes;
// the legacy passed boolean is a convenience = status != StatusBlocked.
//
// Rules (per spec):
//   - Blocked: any Blocker offense, OR any Unclassified offense that triggers
//     the severity failure rules. Blocked always dominates.
//   - Needs review: no blockers, but ≥1 Review offense.
//   - Ready: otherwise (clean, or only Noise / non-failing Unclassified).
//
// An empty offense slice is Ready (a scan that found nothing). Untested is the
// caller's concern when no scan result exists at all.
func DeriveCookstyleStatus(offenses []CookstyleOffense, rules CookstyleFailureRules, resolver *CopClassificationResolver) string {
	hasReview := false
	for i := range offenses {
		off := &offenses[i]
		switch resolver.Resolve(off.CopName).Classification {
		case ClassificationBlocker:
			return StatusBlocked
		case ClassificationReview:
			hasReview = true
		case ClassificationNoise:
			// Noise contributes nothing to the rollup.
		default: // Unclassified — fall back to severity-based failure rules.
			if offenseTriggersFailure(off, &rules) {
				return StatusBlocked
			}
		}
	}
	if hasReview {
		return StatusNeedsReview
	}
	return StatusReady
}
