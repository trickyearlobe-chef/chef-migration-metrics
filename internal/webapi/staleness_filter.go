// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"strings"
)

// stalenessFilter represents the parsed ?stale= query parameter.
// Default (empty or absent) means "all nodes" for trend continuity with legacy data.
// When explicitly set, it filters to the selected tiers.
type stalenessFilter struct {
	fresh    bool
	warning  bool
	critical bool
	explicit bool // true when ?stale= was present in the request
}

// parseStalenessFilter parses the ?stale= query param.
// Accepts comma-separated values: fresh,warning,critical.
// If absent or empty, returns a non-explicit filter (all tiers, legacy mode).
func parseStalenessFilter(req *http.Request) stalenessFilter {
	raw := req.URL.Query().Get("stale")
	if raw == "" {
		return stalenessFilter{fresh: true, warning: true, critical: true, explicit: false}
	}

	sf := stalenessFilter{explicit: true}
	for _, part := range strings.Split(raw, ",") {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case "fresh":
			sf.fresh = true
		case "warning":
			sf.warning = true
		case "critical":
			sf.critical = true
		}
	}

	// If nothing valid was parsed, default to fresh.
	if !sf.fresh && !sf.warning && !sf.critical {
		sf.fresh = true
	}
	return sf
}

// isDefault returns true if no explicit stale filter was provided.
func (sf stalenessFilter) isDefault() bool {
	return !sf.explicit
}

// isFreshOnly returns true if the filter explicitly requests only fresh nodes.
func (sf stalenessFilter) isFreshOnly() bool {
	return sf.explicit && sf.fresh && !sf.warning && !sf.critical
}

// includesFresh returns true if fresh nodes are included.
func (sf stalenessFilter) includesFresh() bool {
	return sf.fresh
}

// includesNonFresh returns true if warning or critical tiers are requested.
func (sf stalenessFilter) includesNonFresh() bool {
	return sf.warning || sf.critical
}
