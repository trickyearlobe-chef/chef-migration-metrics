// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/hypervisor"
)

// DiscoveredPlatformStatus describes a single discovered platform with its
// mapping status relative to the current platform map config.
type DiscoveredPlatformStatus struct {
	PlatformName      string `json:"platform_name"`
	NormalisedName    string `json:"normalised_name"`
	OSFamily          string `json:"os_family"`
	CookbookCount     int    `json:"cookbook_count"`
	TransportType     string `json:"transport_type,omitempty"`
	MappingStatus     string `json:"mapping_status"`      // "mapped", "skipped", or "unmapped"
	MatchedEntryIndex int    `json:"matched_entry_index"` // -1 if unmapped
	MatchedImage      string `json:"matched_image"`       // empty if unmapped or skipped
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

	// Resolve effective Test Kitchen config: database override first, then file.
	var tkCfg config.TestKitchenConfig
	setting, err := r.db.GetRuntimeSetting(ctx, "test_kitchen")
	if err != nil {
		r.logf("ERROR", "platform-mapping-status: load runtime setting: %v", err)
		WriteInternalError(w, "Failed to load Test Kitchen configuration.")
		return
	}
	if setting != nil {
		if unmarshalErr := json.Unmarshal(setting.Value, &tkCfg); unmarshalErr != nil {
			r.logf("ERROR", "platform-mapping-status: parse stored config: %v", unmarshalErr)
			WriteInternalError(w, "Failed to parse stored Test Kitchen configuration.")
			return
		}
	} else {
		tkCfg = r.liveConfig().AnalysisTools.TestKitchen
	}

	// Fetch discovered platforms.
	platforms, err := r.db.ListDiscoveredPlatforms(ctx)
	if err != nil {
		r.logf("ERROR", "platform-mapping-status: list discovered platforms: %v", err)
		WriteInternalError(w, "Failed to list discovered platforms.")
		return
	}

	// Fetch hypervisor templates (best-effort).
	var templates []hypervisor.Template
	if r.hypervisor != nil {
		t, tErr := r.hypervisor.ListTemplates(ctx)
		if tErr != nil {
			r.logf("WARN", "platform-mapping-status: list templates: %v", tErr)
		} else {
			templates = t
		}
	}

	// Build per-platform status and counters.
	var mapped, skipped, unmapped int
	statuses := make([]DiscoveredPlatformStatus, 0, len(platforms))

	for _, p := range platforms {
		match := config.MatchPlatform(p.PlatformName, tkCfg.PlatformMap)

		s := DiscoveredPlatformStatus{
			PlatformName:   p.PlatformName,
			NormalisedName: p.NormalisedName,
			OSFamily:       p.OSFamily,
			CookbookCount:  p.CookbookCount,
			TransportType:  p.TransportType,
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
