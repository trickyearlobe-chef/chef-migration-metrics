// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import "strings"

// This file holds the *structural* Noise rules and the custom-cop test. Under
// the trustworthy-reds model there are no curated Review/Noise *guesses* for
// whole namespaces: a cop is Noise only for a positive structural reason, and
// everything else defaults to Review. Verified-removal Blockers live in the
// compiled RemovedIn mapping (internal/remediation/copmapping.go), not here.

// customCopPrefix marks cops hand-defined in a migration tool. Their offences
// are stored with a cop_name prefixed "Custom/",
// and they resolve as Blocker by intent.
const customCopPrefix = "Custom/"

// isCustomCop reports whether a cop is a hand-defined custom/manual cop.
func isCustomCop(copName string) bool {
	return strings.HasPrefix(copName, customCopPrefix)
}

// noiseDepartmentPrefixes are cosmetic RuboCop departments — non-functional
// *by RuboCop's own taxonomy*, so they can never affect production convergence.
// The Chef/Style/ prefix is listed separately because it does not start with
// the bare "Style/".
var noiseDepartmentPrefixes = []string{
	"Style/",
	"Layout/",
	"Chef/Style/",
}

// noiseToolingMarkers name test-/CI-tooling-only cop families (ChefSpec,
// Foodcritic, Delivery, Librarian/Berkshelf). A cop whose name contains one of
// these cannot affect a production Chef converge, so it is structural Noise.
// Kept deliberately narrow: anything not provably tooling-only stays Review.
var noiseToolingMarkers = []string{
	"ChefSpec",
	"Foodcritic",
	"Delivery",
	"Librarian",
	"Berks",
}

// isStructuralNoise reports whether a cop is Noise for a positive structural
// reason: a cosmetic RuboCop department, or a test/CI-tooling-only cop. It is
// never a fallback — an unmatched cop defaults to Review, not Noise.
func isStructuralNoise(copName string) bool {
	for _, p := range noiseDepartmentPrefixes {
		if strings.HasPrefix(copName, p) {
			return true
		}
	}
	for _, tool := range noiseToolingMarkers {
		if strings.Contains(copName, tool) {
			return true
		}
	}
	return false
}
