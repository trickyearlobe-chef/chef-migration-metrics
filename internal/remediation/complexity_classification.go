// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import "encoding/json"

// CopClassifier resolves a cop's migration classification (one of the
// classification level values below). It is implemented by
// analysis.CopClassificationResolver; the interface is declared here so that
// classification-aware complexity scoring stays free of an import cycle on the
// analysis package (analysis already imports remediation).
type CopClassifier interface {
	// Classify returns the resolved classification level for a cop name. Used by
	// the message-free fingerprint recompute path (a fingerprint entry has no
	// message).
	Classify(copName string) string
	// ClassifyOffense returns the resolved classification level for a specific
	// offence, discriminating poly-method cops by their message (see
	// journeys/scan-trust.md). Live derivations —
	// which have the offence message — use this. ClassifyOffense(cop, "") ==
	// Classify(cop).
	ClassifyOffense(copName, message string) string
}

// Classification level values — mirror the constants in
// internal/analysis/cop_classification.go. Duplicated here only to avoid the
// import cycle; the analysis package remains authoritative.
const (
	classBlocker      = "blocker"
	classReview       = "review"
	classNoise        = "noise"
	classUnclassified = "unclassified"
)

// Classification complexity weights. Each offense contributes exactly once via
// its resolved classification, so an advisory-only repo does not score "high".
const (
	// WeightBlocker is applied per Blocker-classified offense. Blockers
	// dominate — this is the highest single-offense weight.
	WeightBlocker = 8

	// WeightReview is applied per Review-classified offense (advisory, low).
	WeightReview = 1

	// Noise offenses contribute 0.
)

// ComputeCookstyleComplexity computes the classification-weighted CookStyle
// portion of a complexity score from resolved per-offense classifications.
// Each offense contributes exactly once:
//
//   - Blocker:      WeightBlocker (dominant)
//   - Review:       WeightReview (advisory, low)
//   - Noise:        0
//   - Unclassified: the single highest applicable legacy category weight
//     (error/fatal, deprecation, correctness, modernize) — keeping the
//     dominant category signal as the fallback while removing the
//     deprecation+manual-fix double-count that inflated today's scores.
//
// This is the CookStyle contribution only; the Test Kitchen weight is added by
// the caller (see tkWeight). The classifier resolves each offense's cop.
func ComputeCookstyleComplexity(offenses []ClassifiedOffense) int {
	score := 0
	for i := range offenses {
		score += offenses[i].weight()
	}
	return score
}

// ClassifiedOffense is a single CookStyle offense paired with its resolved
// classification, ready for classification-weighted scoring.
type ClassifiedOffense struct {
	CopName        string
	Severity       string
	Classification string
}

// weight returns this offense's single complexity contribution.
func (o ClassifiedOffense) weight() int {
	switch o.Classification {
	case classBlocker:
		return WeightBlocker
	case classReview:
		return WeightReview
	case classNoise:
		return 0
	default: // unclassified — single highest applicable legacy category weight
		return o.unclassifiedWeight()
	}
}

// unclassifiedWeight is the legacy-category fallback for an unclassified
// offense: the highest applicable category weight, so the offense contributes
// once rather than being double-counted across overlapping categories.
func (o ClassifiedOffense) unclassifiedWeight() int {
	w := 0
	if isErrorOrFatal(o.Severity) && WeightErrorFatal > w {
		w = WeightErrorFatal
	}
	if isDeprecation(o.CopName) && WeightDeprecation > w {
		w = WeightDeprecation
	}
	if isCorrectness(o.CopName) && WeightCorrectness > w {
		w = WeightCorrectness
	}
	if isModernize(o.CopName) && WeightModernize > w {
		w = WeightModernize
	}
	return w
}

// tkWeight returns the flat complexity weight for an aggregate Test Kitchen
// status. Shared by the legacy and classification-weighted scoring paths.
func tkWeight(status string) int {
	switch status {
	case "failed":
		return WeightTKFail
	case "partial":
		return WeightTKPartial
	default:
		return 0
	}
}

// classifyOffensesForComplexity parses the JSONB offense array and resolves
// each offense's classification via the supplied classifier, producing the
// per-offense input for ComputeCookstyleComplexity. Offenses that fail to parse
// yield an empty slice (the caller falls back to the legacy aggregate path).
func classifyOffensesForComplexity(offencesJSON []byte, classifier CopClassifier) []ClassifiedOffense {
	if len(offencesJSON) == 0 || classifier == nil {
		return nil
	}
	var offenses []storedOffense
	if err := json.Unmarshal(offencesJSON, &offenses); err != nil {
		return nil
	}
	out := make([]ClassifiedOffense, 0, len(offenses))
	for _, off := range offenses {
		out = append(out, ClassifiedOffense{
			CopName:  off.CopName,
			Severity: off.Severity,
			// Message-aware: a poly-method cop's deprecation-only variant scores as
			// Review, not Blocker (see Poly-method cops in the spec).
			Classification: classifier.ClassifyOffense(off.CopName, off.Message),
		})
	}
	return out
}
