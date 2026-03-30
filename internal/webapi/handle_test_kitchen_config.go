// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// testKitchenConfigResponse is the API response for the Test Kitchen
// configuration endpoint.
type testKitchenConfigResponse struct {
	Config    config.TestKitchenConfig `json:"config"`
	Source    string                   `json:"source"`
	UpdatedAt *time.Time               `json:"updated_at,omitempty"`
	UpdatedBy string                   `json:"updated_by,omitempty"`
}

// handleTestKitchenConfig dispatches GET/PUT/DELETE for
// /api/v1/admin/test-kitchen/config.
func (r *Router) handleTestKitchenConfig(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handleGetTestKitchenConfig(w, req)
	case http.MethodPut:
		r.handlePutTestKitchenConfig(w, req)
	case http.MethodDelete:
		r.handleDeleteTestKitchenConfig(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET, PUT, and DELETE.")
	}
}

func (r *Router) handleGetTestKitchenConfig(w http.ResponseWriter, req *http.Request) {
	setting, err := r.db.GetRuntimeSetting(req.Context(), "test_kitchen")
	if err != nil {
		r.logf("ERROR", "test-kitchen-config: load: %v", err)
		WriteInternalError(w, "Failed to load Test Kitchen configuration.")
		return
	}

	if setting != nil {
		var tkCfg config.TestKitchenConfig
		if unmarshalErr := json.Unmarshal(setting.Value, &tkCfg); unmarshalErr != nil {
			r.logf("ERROR", "test-kitchen-config: parse stored config: %v", unmarshalErr)
			WriteInternalError(w, "Failed to parse stored Test Kitchen configuration.")
			return
		}
		WriteJSON(w, http.StatusOK, testKitchenConfigResponse{
			Config:    tkCfg,
			Source:    "database",
			UpdatedAt: &setting.UpdatedAt,
			UpdatedBy: setting.UpdatedBy,
		})
		return
	}

	// Fall back to file config.
	WriteJSON(w, http.StatusOK, testKitchenConfigResponse{
		Config: r.cfg.AnalysisTools.TestKitchen,
		Source: "file",
	})
}

func (r *Router) handlePutTestKitchenConfig(w http.ResponseWriter, req *http.Request) {
	var tkCfg config.TestKitchenConfig
	if err := json.NewDecoder(req.Body).Decode(&tkCfg); err != nil {
		WriteError(w, http.StatusBadRequest, ErrCodeBadRequest,
			fmt.Sprintf("Invalid JSON body: %v", err))
		return
	}

	// Validate the config.
	if problems := validateTestKitchenConfig(tkCfg); len(problems) > 0 {
		WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "validation_failed",
			"details": problems,
		})
		return
	}

	// Marshal and save.
	value, marshalErr := json.Marshal(tkCfg)
	if marshalErr != nil {
		r.logf("ERROR", "test-kitchen-config: marshal: %v", marshalErr)
		WriteInternalError(w, "Failed to marshal Test Kitchen configuration.")
		return
	}

	username := adminUsername(req)
	if err := r.db.SetRuntimeSetting(req.Context(), "test_kitchen", value, username); err != nil {
		r.logf("ERROR", "test-kitchen-config: save: %v", err)
		WriteInternalError(w, "Failed to save Test Kitchen configuration.")
		return
	}

	r.logf("INFO", "admin %q saved Test Kitchen configuration (driver=%s)", username, tkCfg.EffectiveDriver())

	// Re-read to get the server-set updated_at.
	saved, _ := r.db.GetRuntimeSetting(req.Context(), "test_kitchen")
	resp := map[string]any{
		"config":     tkCfg,
		"source":     "database",
		"updated_by": username,
	}
	if saved != nil {
		resp["updated_at"] = saved.UpdatedAt
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (r *Router) handleDeleteTestKitchenConfig(w http.ResponseWriter, req *http.Request) {
	if req.URL.Query().Get("confirm") != "true" {
		WriteError(w, http.StatusBadRequest, ErrCodeBadRequest,
			"Add ?confirm=true to confirm deletion of the database config override.")
		return
	}

	username := adminUsername(req)
	if err := r.db.DeleteRuntimeSetting(req.Context(), "test_kitchen"); err != nil {
		r.logf("ERROR", "test-kitchen-config: delete: %v", err)
		WriteInternalError(w, "Failed to delete Test Kitchen configuration override.")
		return
	}

	r.logf("INFO", "admin %q reverted Test Kitchen configuration to file defaults", username)
	w.WriteHeader(http.StatusNoContent)
}

// validateTestKitchenConfig checks the config and returns a list of problems.
// An empty list means valid.
func validateTestKitchenConfig(cfg config.TestKitchenConfig) []string {
	var problems []string

	driver := cfg.EffectiveDriver()

	// Non-dokken drivers need a platform map.
	if !analysis.IsDokken(driver) && len(cfg.PlatformMap) == 0 {
		problems = append(problems, "platform_map must have at least one entry when driver is not dokken")
	}

	// Custom driver needs image_field_name.
	if driver == "custom" && cfg.ImageFieldName == "" {
		problems = append(problems, "image_field_name is required when driver is 'custom'")
	}

	// Validate platform map entries.
	seen := make(map[string]bool)
	for i, entry := range cfg.PlatformMap {
		if entry.KitchenName == "" {
			problems = append(problems, fmt.Sprintf("platform_map[%d]: kitchen_name is required", i))
		}
		if entry.Image == "" {
			problems = append(problems, fmt.Sprintf("platform_map[%d]: image is required", i))
		}
		if entry.KitchenName != "" {
			if seen[entry.KitchenName] {
				problems = append(problems, fmt.Sprintf("platform_map[%d]: duplicate kitchen_name %q", i, entry.KitchenName))
			}
			seen[entry.KitchenName] = true
		}
	}

	return problems
}
