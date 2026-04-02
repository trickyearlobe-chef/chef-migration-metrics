// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"fmt"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// knownTKDrivers is the set of recognised Test Kitchen driver profile names.
var knownTKDrivers = map[string]bool{
	"dokken":    true,
	"vcenter":   true,
	"vra":       true,
	"ec2":       true,
	"azurerm":   true,
	"google":    true,
	"vagrant":   true,
	"openstack": true,
	"proxmox":   true,
	"custom":    true,
}

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/analysis-tools
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigAnalysisTools(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		r.writeAdminConfigSection(w, &config.Config{AnalysisTools: cfg.AnalysisTools}, configstore.KeyAnalysisTools)
	case http.MethodPut:
		r.putAdminConfigAnalysisTools(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigAnalysisTools(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input config.AnalysisToolsConfig
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	if input.CookstyleTimeoutMinutes < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"analysis_tools.cookstyle_timeout_minutes must be >= 1.")
		return
	}
	if input.TestKitchenTimeoutMinutes < 0 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"analysis_tools.test_kitchen_timeout_minutes must be >= 0.")
		return
	}
	if msg := validateAdminTKConfig(input.TestKitchen); msg != "" {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError, msg)
		return
	}

	r.storeAdminConfigSection(w, req, &config.Config{AnalysisTools: input}, configstore.KeyAnalysisTools, false)
}

// ---------------------------------------------------------------------------
// GET/PUT/DELETE /api/v1/admin/config/test-kitchen
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigTestKitchen(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		data, err := configstore.SerializeValue(cfg.AnalysisTools.TestKitchen)
		if err != nil {
			r.logf("ERROR", "admin/config/test-kitchen: serialise: %v", err)
			WriteInternalError(w, "Failed to serialise test kitchen config.")
			return
		}
		WriteJSON(w, http.StatusOK, data)
	case http.MethodPut:
		r.putAdminConfigTestKitchen(w, req)
	case http.MethodDelete:
		r.deleteAdminConfigTestKitchen(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET, PUT, and DELETE.")
	}
}

func (r *Router) putAdminConfigTestKitchen(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input config.TestKitchenConfig
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	if msg := validateAdminTKConfig(input); msg != "" {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError, msg)
		return
	}

	analysisTools := r.liveConfig().AnalysisTools
	analysisTools.TestKitchen = input
	sections, err := configstore.ConfigToSections(&config.Config{AnalysisTools: analysisTools})
	if err != nil {
		r.logf("ERROR", "admin/config/test-kitchen: serialise: %v", err)
		WriteInternalError(w, "Failed to serialise analysis tools config.")
		return
	}
	if err := r.configStore.Set(req.Context(), configstore.KeyAnalysisTools, sections[configstore.KeyAnalysisTools], false, "admin"); err != nil {
		r.logf("ERROR", "admin/config/test-kitchen: store: %v", err)
		WriteInternalError(w, "Failed to store test kitchen config.")
		return
	}
	if r.configHolder != nil {
		if err := r.configHolder.Reload(req.Context()); err != nil {
			r.logf("ERROR", "admin/config/test-kitchen: reload: %v", err)
			WriteInternalError(w, "Failed to reload config after update.")
			return
		}
	}
	tkJSON, err := configstore.SerializeValue(input)
	if err != nil {
		r.logf("ERROR", "admin/config/test-kitchen: serialise response: %v", err)
		WriteInternalError(w, "Failed to serialise response.")
		return
	}
	WriteJSON(w, http.StatusOK, putConfigResponse{Value: tkJSON, RestartRequired: false})
}

func (r *Router) deleteAdminConfigTestKitchen(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	analysisTools := r.liveConfig().AnalysisTools
	analysisTools.TestKitchen = config.TestKitchenConfig{}
	sections, err := configstore.ConfigToSections(&config.Config{AnalysisTools: analysisTools})
	if err != nil {
		r.logf("ERROR", "admin/config/test-kitchen: serialise: %v", err)
		WriteInternalError(w, "Failed to serialise analysis tools config.")
		return
	}
	if err := r.configStore.Set(req.Context(), configstore.KeyAnalysisTools, sections[configstore.KeyAnalysisTools], false, "admin"); err != nil {
		r.logf("ERROR", "admin/config/test-kitchen: store: %v", err)
		WriteInternalError(w, "Failed to store test kitchen config.")
		return
	}
	if r.configHolder != nil {
		if err := r.configHolder.Reload(req.Context()); err != nil {
			r.logf("ERROR", "admin/config/test-kitchen: reload: %v", err)
			WriteInternalError(w, "Failed to reload config after update.")
			return
		}
	}
	tkJSON, err := configstore.SerializeValue(analysisTools.TestKitchen)
	if err != nil {
		r.logf("ERROR", "admin/config/test-kitchen: serialise response: %v", err)
		WriteInternalError(w, "Failed to serialise response.")
		return
	}
	WriteJSON(w, http.StatusOK, putConfigResponse{Value: tkJSON, RestartRequired: false})
}

// ---------------------------------------------------------------------------
// Shared validation
// ---------------------------------------------------------------------------

// validateAdminTKConfig validates the TestKitchenConfig portion of an admin
// config request. Returns an error message string, or empty string if valid.
// Error messages use the "analysis_tools.test_kitchen.*" prefix, matching
// both the analysis-tools and test-kitchen API endpoints.
func validateAdminTKConfig(tk config.TestKitchenConfig) string {
	if tk.TimeoutMinutes < 0 {
		return "analysis_tools.test_kitchen.timeout_minutes must be >= 0."
	}

	driver := tk.EffectiveDriver()
	if !knownTKDrivers[driver] {
		return fmt.Sprintf("analysis_tools.test_kitchen.driver %q is not a recognised driver profile.", driver)
	}

	if driver == "custom" && tk.ImageFieldName == "" {
		return "analysis_tools.test_kitchen.image_field_name is required when driver is \"custom\"."
	}

	seen := make(map[string]int)
	for i, entry := range tk.PlatformMap {
		if entry.KitchenName == "" {
			return fmt.Sprintf("analysis_tools.test_kitchen.platform_map[%d].kitchen_name is required.", i)
		}
		if prev, dup := seen[entry.KitchenName]; dup {
			return fmt.Sprintf("analysis_tools.test_kitchen.platform_map[%d].kitchen_name %q duplicates entry [%d].", i, entry.KitchenName, prev)
		}
		seen[entry.KitchenName] = i
	}

	return ""
}
