// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/platform"
)

const platformDisplayNamesKey = "platform_display_names"

// platformDisplayNamesResponse is the JSON envelope for GET and PUT responses.
type platformDisplayNamesResponse struct {
	Mappings  []platform.DisplayNameMapping `json:"mappings"`
	IsDefault bool                          `json:"is_default"`
}

// loadPlatformDisplayNames loads the display name mappings from the config
// store, falling back to DefaultMappings if none are stored.
func (r *Router) loadPlatformDisplayNames(ctx context.Context) ([]platform.DisplayNameMapping, error) {
	if r.configStore == nil {
		return platform.DefaultMappings, nil
	}
	raw, err := r.configStore.Get(ctx, platformDisplayNamesKey)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			return platform.DefaultMappings, nil
		}
		return nil, err
	}

	var mappings []platform.DisplayNameMapping
	if err := json.Unmarshal(raw, &mappings); err != nil {
		return nil, fmt.Errorf("unmarshal platform display names: %w", err)
	}
	return mappings, nil
}

// handlePlatformDisplayNames handles GET and PUT on
// /api/v1/admin/platform-display-names.
func (r *Router) handlePlatformDisplayNames(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured.")
		return
	}

	switch req.Method {
	case http.MethodGet:
		r.getPlatformDisplayNames(w, req)
	case http.MethodPut:
		r.putPlatformDisplayNames(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) getPlatformDisplayNames(w http.ResponseWriter, req *http.Request) {
	mappings, err := r.loadPlatformDisplayNames(req.Context())
	if err != nil {
		r.logf("ERROR", "platform-display-names: load: %v", err)
		WriteInternalError(w, "Failed to load platform display names.")
		return
	}

	WriteJSON(w, http.StatusOK, platformDisplayNamesResponse{
		Mappings:  mappings,
		IsDefault: platform.IsDefault(mappings),
	})
}

func (r *Router) putPlatformDisplayNames(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		WriteBadRequest(w, "Failed to read request body.")
		return
	}

	var mappings []platform.DisplayNameMapping
	if err := json.Unmarshal(body, &mappings); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	// Validate and normalise.
	seen := make(map[string]bool, len(mappings))
	for i := range mappings {
		m := &mappings[i]

		if m.Platform == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("mappings[%d]: platform is required.", i))
			return
		}
		if m.VersionPrefix == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("mappings[%d]: version_prefix is required.", i))
			return
		}
		if m.DisplayName == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("mappings[%d]: display_name is required.", i))
			return
		}

		// Normalise platform to lowercase.
		m.Platform = strings.ToLower(m.Platform)

		// Check for duplicate (platform, version_prefix) pairs.
		dupKey := m.Platform + "\x00" + m.VersionPrefix
		if seen[dupKey] {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("mappings[%d]: duplicate entry for platform=%q version_prefix=%q.", i, m.Platform, m.VersionPrefix))
			return
		}
		seen[dupKey] = true
	}

	jsonBytes, err := json.Marshal(mappings)
	if err != nil {
		r.logf("ERROR", "platform-display-names: marshal: %v", err)
		WriteInternalError(w, "Failed to serialise mappings.")
		return
	}

	if err := r.configStore.Set(req.Context(), platformDisplayNamesKey, jsonBytes, false, "admin"); err != nil {
		r.logf("ERROR", "platform-display-names: store: %v", err)
		WriteInternalError(w, "Failed to store platform display names.")
		return
	}

	WriteJSON(w, http.StatusOK, platformDisplayNamesResponse{
		Mappings:  mappings,
		IsDefault: platform.IsDefault(mappings),
	})
}

// handlePlatformDisplayNamesReset handles POST on
// /api/v1/admin/platform-display-names/reset.
func (r *Router) handlePlatformDisplayNamesReset(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured.")
		return
	}

	if !requirePOST(w, req) {
		return
	}

	if err := r.configStore.Delete(req.Context(), platformDisplayNamesKey); err != nil {
		r.logf("ERROR", "platform-display-names: delete: %v", err)
		WriteInternalError(w, "Failed to reset platform display names.")
		return
	}

	WriteJSON(w, http.StatusOK, platformDisplayNamesResponse{
		Mappings:  platform.DefaultMappings,
		IsDefault: true,
	})
}

// resolvePlatformDisplayName resolves the display name for a node's platform
// and version using the centralized resolver. Always returns a non-nil result
// unless platform is empty.
func resolvePlatformDisplayName(plat, ver string, mappings []platform.DisplayNameMapping) *string {
	if plat == "" {
		return nil
	}
	family := platform.DetectOSFamilyFromPlatform(plat)
	info := platform.ResolveInfo(plat, ver, family, "", mappings)
	return &info.DisplayName
}
