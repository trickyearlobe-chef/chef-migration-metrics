// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// ClassificationOverrideLister loads operator classification overrides.
// Overrides are keyed by cop_name only (single active target). *datastore.DB
// satisfies it; declared as an interface so callers (scanner, scorer wiring)
// can build a resolver without a concrete DB.
type ClassificationOverrideLister interface {
	ListCopClassifications(ctx context.Context) ([]datastore.CopClassification, error)
}

// NewResolverFromStore builds a classification resolver for the active target
// version, loading operator overrides from the store. A nil store (or a load
// error) yields a resolver with no overrides — RemovedIn auto-seed and curated
// defaults still apply, so classification works without operator input. The
// target version is still carried for the RemovedIn ≤ target comparison; it is
// no longer a key for the overrides.
func NewResolverFromStore(ctx context.Context, store ClassificationOverrideLister, targetChefVersion string) *CopClassificationResolver {
	overrides := map[string]string{}
	if store != nil {
		if rows, err := store.ListCopClassifications(ctx); err == nil {
			for _, r := range rows {
				overrides[r.CopName] = r.Classification
			}
		}
	}
	return &CopClassificationResolver{OperatorOverrides: overrides, TargetChefVersion: targetChefVersion}
}

// Classification levels represent the migration impact of a cop. There is no
// "unclassified" level: an unresolved cop *is* a Review item (the honest
// default). See journeys/scan-trust.md.
const (
	ClassificationBlocker = "blocker"
	ClassificationReview  = "review"
	ClassificationNoise   = "noise"
)

// ClassificationSource describes how a classification was determined — every
// source is a positive statement of knowledge; the default is Review, never a
// severity-derived red.
const (
	SourceOperatorOverride = "operator_override" // operator's confirmed verdict (DB)
	SourceCustomCop        = "custom_cop"        // hand-defined migration cop → Blocker by intent
	SourceVerifiedRemoval  = "verified_removal"  // curated RemovedIn ≤ target → Blocker
	SourceStructuralNoise  = "structural_noise"  // cosmetic department or test/CI tooling → Noise
	SourceReviewDefault    = "review_default"    // unproven — operator decides (Review)
)

// ResolvedClassification holds the result of classification resolution for a cop.
type ResolvedClassification struct {
	Classification string // blocker, review, noise
	Source         string // operator_override, custom_cop, verified_removal, structural_noise, review_default
}

// CopClassificationResolver resolves the effective classification for a cop
// against the single active target version. Sources are checked in priority
// order (see Resolve). The TargetChefVersion is used only for the
// verified-removal (RemovedIn ≤ target) comparison; it is not a key for the
// operator overrides (there is one active target).
type CopClassificationResolver struct {
	// OperatorOverrides maps cop_name → classification (operator's verdict).
	OperatorOverrides map[string]string
	// TargetChefVersion is the version being migrated to.
	TargetChefVersion string
}

// Resolve determines the classification for a given cop name in priority order:
//  1. Operator override (the operator's confirmed verdict).
//  2. Custom/manual cop → Blocker (a hand-defined migration cop is a blocker by intent).
//  3. Verified removal → Blocker (a curated RemovedIn ≤ target).
//  4. Structural Noise → Noise (cosmetic department or test/CI tooling only).
//  5. Review (default) — everything else; honest "unproven — operator decides".
//
// No severity fallback: severity is the signal this feature exists to distrust,
// so it never produces a red.
func (r *CopClassificationResolver) Resolve(copName string) ResolvedClassification {
	return r.ResolveOffense(copName, "")
}

// ResolveOffense is the message-aware form of Resolve. It is identical except
// that the verified-removal step consults the message-discriminated variant of a
// poly-method cop (see journeys/scan-trust.md):
// e.g. a Lint/DeprecatedClassMethods offence for Socket.gethostbyname (no
// RemovedIn) resolves to Review, while one for File.exists? (RemovedIn 18.0)
// resolves to Blocker. An empty message falls back to the cop-name mapping, so
// ResolveOffense(cop, "") == Resolve(cop) for every cop. Operator overrides,
// custom cops, and structural noise are cop-name concepts and ignore the message.
func (r *CopClassificationResolver) ResolveOffense(copName, message string) ResolvedClassification {
	// 1. Operator override (highest priority; keyed by cop_name).
	if class, ok := r.OperatorOverrides[copName]; ok {
		return ResolvedClassification{Classification: class, Source: SourceOperatorOverride}
	}

	// 2. Custom/manual cop → Blocker by intent.
	if isCustomCop(copName) {
		return ResolvedClassification{Classification: ClassificationBlocker, Source: SourceCustomCop}
	}

	// 3. Verified removal → Blocker (curated RemovedIn ≤ target). Message-aware
	// for poly-method cops; cop-name mapping otherwise.
	if mapping := remediation.LookupCopForOffense(copName, message); mapping != nil && mapping.RemovedIn != "" {
		if versionLessOrEqual(mapping.RemovedIn, r.TargetChefVersion) {
			return ResolvedClassification{Classification: ClassificationBlocker, Source: SourceVerifiedRemoval}
		}
	}

	// 4. Structural Noise (positive structural reason only).
	if isStructuralNoise(copName) {
		return ResolvedClassification{Classification: ClassificationNoise, Source: SourceStructuralNoise}
	}

	// 5. Review (default) — unproven; operator decides.
	return ResolvedClassification{Classification: ClassificationReview, Source: SourceReviewDefault}
}

// IsBlocker returns true if the given cop is classified as a blocker at the target version.
func (r *CopClassificationResolver) IsBlocker(copName string) bool {
	return r.Resolve(copName).Classification == ClassificationBlocker
}

// Classify returns the resolved classification level (blocker / review / noise /
// unclassified) for a cop, discarding the source. It satisfies the
// remediation.CopClassifier interface used by classification-aware complexity
// scoring.
func (r *CopClassificationResolver) Classify(copName string) string {
	return r.Resolve(copName).Classification
}

// ClassifyOffense is the message-aware form of Classify, used by
// classification-weighted complexity scoring over live offences (which carry a
// message). It satisfies remediation.CopClassifier. ClassifyOffense(cop, "")
// == Classify(cop).
func (r *CopClassificationResolver) ClassifyOffense(copName, message string) string {
	return r.ResolveOffense(copName, message).Classification
}

// EvaluatePassFailWithClassification evaluates whether a set of offenses passes,
// using cop classification alone. The boolean is a convenience derived from the
// single source of truth: passed = status != Blocked.
func EvaluatePassFailWithClassification(offenses []CookstyleOffense, resolver *CopClassificationResolver) bool {
	return DeriveCookstyleStatus(offenses, resolver) != StatusBlocked
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
