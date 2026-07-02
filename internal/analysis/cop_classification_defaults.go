// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import "strings"

// curatedDefault represents a pre-shipped classification for a cop that is
// known to be problematic for migration but cannot be auto-detected from
// RemovedIn metadata alone.
type curatedDefault struct {
	Classification   string // blocker, review, noise
	MinTargetVersion string // classification applies when target >= this version (empty = all)
}

// curatedDefaults is the built-in classification table for cops whose impact
// cannot be derived from severity or RemovedIn metadata. These cover:
// - Generic RuboCop cops that detect Ruby-level removals (File.exists?)
// - Chef cops with silent behaviour changes (log notifications stop firing)
// - Tooling-only cops that are pure noise for runtime migration
//
// Operators can override any of these via the UI.
var curatedDefaults = map[string]curatedDefault{
	// -------------------------------------------------------------------------
	// Blockers: will crash or silently break on the target version
	// -------------------------------------------------------------------------

	// File.exists? removed in Ruby 3 (used by Chef 18+). Auto-correctable.
	"Lint/DeprecatedClassMethods": {Classification: ClassificationBlocker, MinTargetVersion: "18.0"},

	// Log resource notifications silently stop firing in Chef 16.
	"Chef/Deprecations/LogResourceNotifications": {Classification: ClassificationBlocker, MinTargetVersion: "16.0"},

	// :servermanagercmd install method silently ignored in newer Chef.
	"Chef/Deprecations/WindowsFeatureServermanagercmd": {Classification: ClassificationBlocker, MinTargetVersion: "14.0"},

	// Deprecated mixins removed in Chef 14.
	"Chef/Deprecations/DeprecatedMixins": {Classification: ClassificationBlocker, MinTargetVersion: "14.0"},

	// run_command helpers removed — use shell_out! instead.
	"Chef/Deprecations/RunCommandHelper": {Classification: ClassificationBlocker, MinTargetVersion: "14.0"},

	// -------------------------------------------------------------------------
	// Review: likely problematic, operator should investigate
	// -------------------------------------------------------------------------

	// Unified mode recommended for Chef 18+ resources. Not a crash but
	// behaviour differs significantly.
	"Chef/Deprecations/HWRPWithoutUnifiedTrue":     {Classification: ClassificationReview, MinTargetVersion: "18.0"},
	"Chef/Deprecations/ResourceWithoutUnifiedTrue": {Classification: ClassificationReview, MinTargetVersion: "18.0"},

	// node.normal persists unexpectedly — not a crash but a data hygiene issue.
	"Chef/Correctness/NodeNormal":       {Classification: ClassificationReview, MinTargetVersion: ""},
	"Chef/Correctness/NodeNormalUnless": {Classification: ClassificationReview, MinTargetVersion: ""},

	// -------------------------------------------------------------------------
	// Noise: tooling-only, style, or harmless for runtime migration
	// -------------------------------------------------------------------------

	// ChefSpec / test tooling — never affects production runtime.
	"Chef/Deprecations/ChefSpecLegacyRunner":       {Classification: ClassificationNoise, MinTargetVersion: ""},
	"Chef/Deprecations/ChefSpecCoverageReport":     {Classification: ClassificationNoise, MinTargetVersion: ""},
	"Chef/Deprecations/DeprecatedChefSpecPlatform": {Classification: ClassificationNoise, MinTargetVersion: ""},
	"Chef/Deprecations/LibrarianChefspec":          {Classification: ClassificationNoise, MinTargetVersion: ""},

	// Foodcritic — defunct linter, no runtime impact.
	"Chef/Deprecations/FoodcriticTesting": {Classification: ClassificationNoise, MinTargetVersion: ""},
	"Chef/Deprecations/FoodcriticFile":    {Classification: ClassificationNoise, MinTargetVersion: ""},

	// Delivery/Workflow — CI tooling, no runtime impact.
	"Chef/Deprecations/Delivery": {Classification: ClassificationNoise, MinTargetVersion: ""},

	// Poise framework — still works, just unmaintained.
	"Chef/Deprecations/CookbookDependsOnPoise": {Classification: ClassificationNoise, MinTargetVersion: ""},

	// Librarian/Cheffile — dependency tooling, no runtime impact.
	"Chef/Deprecations/Cheffile": {Classification: ClassificationNoise, MinTargetVersion: ""},

	// use_inline_resources — no-op in Chef 13+, harmless dead code.
	"Chef/Deprecations/UseInlineResources": {Classification: ClassificationNoise, MinTargetVersion: ""},
}

// CuratedDefaultCopNames returns the exact cop names that ship with a curated
// classification default. It lets the cop-aggregation surface seed these as
// known cops even before any scan has produced their offence. Department-prefix
// defaults (Style/, Layout/, …) are deliberately excluded — they classify whole
// namespaces and have no concrete cop names to enumerate.
func CuratedDefaultCopNames() []string {
	names := make([]string, 0, len(curatedDefaults))
	for name := range curatedDefaults {
		names = append(names, name)
	}
	return names
}

// curatedPrefixDefault is a department-level curated default: every cop whose
// name starts with Prefix inherits this classification unless a more specific
// source (operator override, RemovedIn, or an exact curatedDefaults entry)
// applies first. It exists so whole cosmetic departments seed to Noise without
// enumerating every cop.
type curatedPrefixDefault struct {
	Prefix         string
	curatedDefault // embeds Classification + MinTargetVersion
}

// curatedPrefixDefaults seeds whole departments to a sensible default so that
// an unmapped cop is never silently Unclassified. Two tiers:
//   - Chef deprecation/correctness departments → Review: an unknown cop here
//     is likely migration-relevant, so it must be visible and advisory. Review
//     is non-blocking (only Blocker blocks) and carries low complexity weight.
//   - Cosmetic style/layout departments → Noise: pure style, never affects
//     runtime migration, pre-sorts into the collapsed Noise section, 0 weight.
//
// The longest matching prefix wins (see lookupCuratedPrefixDefault), so the two
// tiers are disjoint and cannot cross-contaminate. More specific sources —
// operator override, RemovedIn, and exact curatedDefaults entries — are all
// consulted before this table, so they still take precedence. Keep entries
// sorted by descending prefix length for readability.
var curatedPrefixDefaults = []curatedPrefixDefault{
	{Prefix: "Chef/Deprecations/", curatedDefault: curatedDefault{Classification: ClassificationReview}},
	{Prefix: "Chef/Correctness/", curatedDefault: curatedDefault{Classification: ClassificationReview}},
	{Prefix: "Chef/Style/", curatedDefault: curatedDefault{Classification: ClassificationNoise}},
	{Prefix: "Style/", curatedDefault: curatedDefault{Classification: ClassificationNoise}},
	{Prefix: "Layout/", curatedDefault: curatedDefault{Classification: ClassificationNoise}},
}

// lookupCuratedPrefixDefault returns the curated default for the longest
// matching department prefix, if any.
func lookupCuratedPrefixDefault(copName string) (curatedDefault, bool) {
	best := -1
	var found curatedDefault
	for _, p := range curatedPrefixDefaults {
		if len(p.Prefix) > best && strings.HasPrefix(copName, p.Prefix) {
			best = len(p.Prefix)
			found = p.curatedDefault
		}
	}
	return found, best >= 0
}
