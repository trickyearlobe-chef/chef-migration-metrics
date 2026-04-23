// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package config

import "path"

// MatchResult holds the outcome of a platform match lookup.
type MatchResult struct {
	Index int               // Index of the matching entry in the slice (-1 if no match)
	Entry *PlatformMapEntry // Pointer to the matching entry (nil if no match)
}

// MatchPlatform finds the best matching PlatformMapEntry for a kitchen
// platform name. Exact (non-pattern) entries are checked first regardless of
// position in the slice; if no exact match is found, pattern entries are
// evaluated in order and the first glob match wins.
func MatchPlatform(name string, entries []PlatformMapEntry) MatchResult {
	if name == "" || len(entries) == 0 {
		return MatchResult{Index: -1}
	}

	// First pass: exact matches only.
	for i := range entries {
		if entries[i].IsPattern {
			continue
		}
		if entries[i].KitchenName == name {
			return MatchResult{Index: i, Entry: &entries[i]}
		}
	}

	// Second pass: pattern (glob) matches in order.
	for i := range entries {
		if !entries[i].IsPattern {
			continue
		}
		if matched, _ := path.Match(entries[i].KitchenName, name); matched {
			return MatchResult{Index: i, Entry: &entries[i]}
		}
	}

	return MatchResult{Index: -1}
}
