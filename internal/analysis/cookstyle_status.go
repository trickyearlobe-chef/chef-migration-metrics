// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

// CookStyle rollup status — the classification-derived, single-source-of-truth
// verdict for a cookbook / repo / node × target version. These wire values are
// consumed by every read surface (lists, summary cards, detail headers, node
// readiness, exports, trends). See specifications/scan-trust.md
// (CookStyle Rollup Status).
const (
	StatusReady       = "ready"        // 🟢 no blockers, no review-level offenses
	StatusNeedsReview = "needs_review" // 🟠 no blockers, ≥1 review-level offense
	StatusBlocked     = "blocked"      // 🔴 ≥1 blocker offense (severity is never a source)
	// Untested (⚪, no scan result for this unit + target) reuses the existing
	// StatusUntested constant defined in readiness.go; it is caller-assigned
	// when no scan result exists, not produced by DeriveCookstyleStatus.
)

// SeverityFatal is the cookstyle severity for a file that will not parse (a
// syntax/parse failure). It is surfaced as a separate "won't parse — fix first"
// data-quality flag carried ALONGSIDE the rollup status, never folded into the
// migration classification (see specifications/scan-trust.md,
// Pass/Fail Determination).
const SeverityFatal = "fatal"

// OffensesWontParse reports whether any offense is a parse failure (fatal
// severity) — the "won't parse — fix first" signal. It is independent of
// classification: a fatal offense does not make a cookbook Blocked, it flags a
// data-quality problem the operator should fix before trusting the scan.
func OffensesWontParse(offenses []CookstyleOffense) bool {
	for i := range offenses {
		if offenses[i].Severity == SeverityFatal {
			return true
		}
	}
	return false
}

// DeriveCookstyleStatus computes the rollup status for a set of offenses from
// resolved cop classification alone. This is the single derivation every
// surface consumes; the legacy passed boolean is a convenience = status !=
// StatusBlocked.
//
// Rules (per spec):
//   - Blocked: any Blocker offense. Blocked always dominates. Severity is never
//     a source (the rules argument is inert — see DeriveStatusFromFingerprint).
//   - Needs review: no blockers, but ≥1 Review offense.
//   - Ready: otherwise (clean, or only Noise).
//
// An empty offense slice is Ready (a scan that found nothing). Untested is the
// caller's concern when no scan result exists at all.
func DeriveCookstyleStatus(offenses []CookstyleOffense, resolver *CopClassificationResolver) string {
	// LIVE derivation: resolve each offence WITH its message so poly-method cops
	// (one cop_name, several deprecations of differing impact) classify per
	// variant — e.g. Socket.gethostbyname is Review, File.exists? is a Blocker.
	// This is the authoritative current-state status. The message-free
	// fingerprint recompute path (DeriveStatusFromFingerprint) keys on cop name
	// and is the documented poly-method exception for recomputed historical trend
	// points only (see specifications/scan-trust.md).
	hasReview := false
	for i := range offenses {
		switch resolver.ResolveOffense(offenses[i].CopName, offenses[i].Message).Classification {
		case ClassificationBlocker:
			return StatusBlocked
		case ClassificationReview:
			hasReview = true
		case ClassificationNoise:
			// Noise contributes nothing to the rollup.
		}
	}
	if hasReview {
		return StatusNeedsReview
	}
	return StatusReady
}
