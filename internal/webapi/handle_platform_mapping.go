// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"sort"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/hypervisor"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/platform"
)

// DiscoveredPlatformStatus describes a single discovered platform with its
// mapping status relative to the current platform map config.
type DiscoveredPlatformStatus struct {
	PlatformName      string  `json:"platform_name"`
	DisplayName       *string `json:"display_name"`
	NormalisedName    string  `json:"normalised_name"`
	OSFamily          string  `json:"os_family"`
	CookbookCount     int     `json:"cookbook_count"`
	NodeCount         int     `json:"node_count"`
	Source            string  `json:"source"` // "kitchen", "nodes", or "both"
	TransportType     string  `json:"transport_type,omitempty"`
	MappingStatus     string  `json:"mapping_status"`      // "mapped", "skipped", or "unmapped"
	MatchedEntryIndex int     `json:"matched_entry_index"` // -1 if unmapped
	MatchedImage      string  `json:"matched_image"`       // empty if unmapped or skipped
}

// PlatformMappingStatusResponse is the response for
// GET /api/v1/admin/platform-mapping/status.
type PlatformMappingStatusResponse struct {
	DiscoveredPlatforms []DiscoveredPlatformStatus `json:"discovered_platforms"`
	Templates           []hypervisor.Template      `json:"templates"`
	UnmappedCount       int                        `json:"unmapped_count"`
	SkippedCount        int                        `json:"skipped_count"`
	MappedCount         int                        `json:"mapped_count"`
}

// handlePlatformMappingStatus returns the mapping status for all discovered
// platforms against the current platform map configuration.
//
//	GET /api/v1/admin/platform-mapping/status
func (r *Router) handlePlatformMappingStatus(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Resolve effective Test Kitchen config from live config.
	tkCfg := r.liveConfig().AnalysisTools.TestKitchen

	// Fetch discovered platforms from kitchen analysis.
	platforms, err := r.db.ListDiscoveredPlatforms(ctx)
	if err != nil {
		r.logf("ERROR", "platform-mapping-status: list discovered platforms: %v", err)
		WriteInternalError(w, "Failed to list discovered platforms.")
		return
	}

	// Fetch node platform distribution (best-effort).
	nodePlatforms, _, nodeErr := r.db.CountNodePlatformDistribution(ctx, datastore.NodeSnapshotFilter{})
	if nodeErr != nil {
		r.logf("WARN", "platform-mapping-status: count node platforms: %v", nodeErr)
		nodePlatforms = map[string]int{}
	}

	// Fetch hypervisor templates (best-effort).
	var templates []hypervisor.Template
	hyp, hypErr := r.buildHypervisor(ctx)
	if hypErr != nil {
		r.logf("WARN", "platform-mapping-status: build hypervisor: %v", hypErr)
	} else if hyp != nil {
		t, tErr := hyp.ListTemplates(ctx)
		if tErr != nil {
			r.logf("WARN", "platform-mapping-status: list templates: %v", tErr)
		} else {
			templates = t
		}
	}

	// Load platform display name mappings for friendly name resolution.
	displayMappings, _ := r.loadPlatformDisplayNames(ctx)

	// Build per-platform status and counters.
	// Start with kitchen-discovered platforms, then merge node platforms.
	var mapped, skipped, unmapped int
	statuses := make([]DiscoveredPlatformStatus, 0, len(platforms)+len(nodePlatforms))

	for _, p := range platforms {
		match := config.MatchPlatform(p.PlatformName, tkCfg.PlatformMap)

		s := DiscoveredPlatformStatus{
			PlatformName:   p.PlatformName,
			NormalisedName: p.NormalisedName,
			OSFamily:       p.OSFamily,
			CookbookCount:  p.CookbookCount,
			TransportType:  p.TransportType,
			Source:         "kitchen",
		}

		if match.Entry != nil {
			s.MatchedEntryIndex = match.Index
			if match.Entry.Skip {
				s.MappingStatus = "skipped"
				skipped++
			} else {
				s.MappingStatus = "mapped"
				s.MatchedImage = match.Entry.Image
				mapped++
			}
		} else {
			s.MappingStatus = "unmapped"
			s.MatchedEntryIndex = -1
			unmapped++
		}

		// Check if a node platform matches this kitchen platform.
		if nc, ok := nodePlatforms[p.PlatformName]; ok {
			s.NodeCount = nc
			s.Source = "both"
			delete(nodePlatforms, p.PlatformName)
		}

		statuses = append(statuses, s)
	}

	// Add remaining node-only platforms.
	nodePlatformNames := make([]string, 0, len(nodePlatforms))
	for name := range nodePlatforms {
		nodePlatformNames = append(nodePlatformNames, name)
	}
	sort.Strings(nodePlatformNames)

	for _, name := range nodePlatformNames {
		count := nodePlatforms[name]
		match := config.MatchPlatform(name, tkCfg.PlatformMap)

		s := DiscoveredPlatformStatus{
			PlatformName:   name,
			NormalisedName: name,
			NodeCount:      count,
			Source:         "nodes",
		}

		// Resolve OS family and display name from the "platform version" label.
		if idx := strings.IndexByte(name, ' '); idx > 0 {
			plat := name[:idx]
			ver := name[idx+1:]
			s.OSFamily = platform.DetectOSFamilyFromPlatform(plat)
			if dn := resolvePlatformDisplayName(plat, ver, displayMappings); dn != nil {
				s.DisplayName = dn
			}
		} else if name != "unknown" {
			s.OSFamily = platform.DetectOSFamilyFromPlatform(name)
		}

		if match.Entry != nil {
			s.MatchedEntryIndex = match.Index
			if match.Entry.Skip {
				s.MappingStatus = "skipped"
				skipped++
			} else {
				s.MappingStatus = "mapped"
				s.MatchedImage = match.Entry.Image
				mapped++
			}
		} else {
			s.MappingStatus = "unmapped"
			s.MatchedEntryIndex = -1
			unmapped++
		}

		statuses = append(statuses, s)
	}

	// Ensure non-nil slices for JSON serialisation.
	if templates == nil {
		templates = []hypervisor.Template{}
	}
	if statuses == nil {
		statuses = []DiscoveredPlatformStatus{}
	}

	WriteJSON(w, http.StatusOK, PlatformMappingStatusResponse{
		DiscoveredPlatforms: statuses,
		Templates:           templates,
		UnmappedCount:       unmapped,
		SkippedCount:        skipped,
		MappedCount:         mapped,
	})
}
