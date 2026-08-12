// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// knownTKDrivers is the set of recognised Test Kitchen driver profile names.
var knownTKDrivers = map[string]bool{
	"vcenter": true,
	"vra":     true,
	"ec2":     true,
	"vagrant": true,
	"proxmox": true,
}

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/analysis-tools
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigAnalysisTools(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.getAdminConfigAnalysisTools(w)
	case http.MethodPut:
		r.putAdminConfigAnalysisTools(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

// analysisToolsGetResponse wraps the standard config section JSON.
type analysisToolsGetResponse struct {
	Value json.RawMessage `json:"value"`
}

// analysisToolsResponse says what that wrapper carries.
//
// The section is written already serialised — the settings are read and written
// as YAML, so their names on the wire come from yaml tags rather than json ones
// — and a raw message describes nothing. This names the type inside it, so the
// fields stay reflected off the real settings and only the wrapper is written
// down. If the two ever part company, probe.py reports the address as sending
// something the description does not mention.
//
// The section rather than the whole analysis tools struct, because Test Kitchen
// keeps a record of its own. Naming the whole struct here would advertise a
// field this call neither answers with nor reads.
type analysisToolsResponse struct {
	Value configstore.AnalysisToolsSection `json:"value"`
}

func (r *Router) getAdminConfigAnalysisTools(w http.ResponseWriter) {
	cfg := r.liveConfig()
	sections, err := configstore.ConfigToSections(&config.Config{AnalysisTools: cfg.AnalysisTools})
	if err != nil {
		r.logf("ERROR", "admin/config/analysis_tools: serialise: %v", err)
		WriteInternalError(w, "Failed to serialise config section.")
		return
	}
	WriteJSON(w, http.StatusOK, analysisToolsGetResponse{
		Value: sections[configstore.KeyAnalysisTools],
	})
}

func (r *Router) putAdminConfigAnalysisTools(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input configstore.AnalysisToolsSection
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
	// Test Kitchen is not validated here: this call no longer carries it, and a
	// caller that sends it is refused rather than quietly ignored.

	// Store, reload, and apply — inlined (not storeAdminConfigSection) so we
	// can capture the rescore verdicts_changed count for the response.
	//
	// Put back together with what Test Kitchen already has, so the prospective
	// config this is validated against is the whole one. Validating one with
	// Test Kitchen missing would refuse the save for something the caller
	// neither sent nor could fix from this screen.
	tools := config.AnalysisToolsConfig{
		EmbeddedBinDir:            input.EmbeddedBinDir,
		CookstyleEnabled:          input.CookstyleEnabled,
		CookstyleTimeoutMinutes:   input.CookstyleTimeoutMinutes,
		CookstyleAddonCopPaths:    input.CookstyleAddonCopPaths,
		TestKitchenTimeoutMinutes: input.TestKitchenTimeoutMinutes,
		TestKitchen:               r.liveConfig().AnalysisTools.TestKitchen,
	}
	partial := &config.Config{AnalysisTools: tools}
	sections, err := configstore.ConfigToSections(partial)
	if err != nil {
		r.logf("ERROR", "admin/config/analysis_tools: serialise: %v", err)
		WriteInternalError(w, "Failed to serialise config section.")
		return
	}
	value := sections[configstore.KeyAnalysisTools]

	// Pre-persist validation against the assembled config.
	if r.configHolder != nil {
		if current := r.configHolder.Get(); current != nil {
			prospectiveSections, err := configstore.ConfigToSections(current)
			if err != nil {
				r.logf("ERROR", "admin/config/analysis_tools: build prospective config: %v", err)
				WriteInternalError(w, "Failed to validate config section.")
				return
			}
			prospectiveSections[configstore.KeyAnalysisTools] = value
			prospective, err := configstore.AssembleConfigRaw(prospectiveSections)
			if err != nil {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("%s: %v", configstore.KeyAnalysisTools, err))
				return
			}
			prospective.ApplyDefaults()
			if _, valErr := prospective.Validate(); valErr != nil {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError, valErr.Error())
				return
			}
		}
	}

	if err := r.configStore.Set(req.Context(), configstore.KeyAnalysisTools, value, false, "admin"); err != nil {
		r.logf("ERROR", "admin/config/analysis_tools: store: %v", err)
		WriteInternalError(w, "Failed to store config section.")
		return
	}

	if r.configHolder != nil {
		if err := r.configHolder.Reload(req.Context()); err != nil {
			r.logf("ERROR", "admin/config/analysis_tools: reload: %v", err)
			WriteInternalError(w, "Failed to reload config after update.")
			return
		}
	}

	// Apply kitchen worker pool resize.
	kitchenRes, err := r.applyKitchenWorkerCount(req.Context())
	if err != nil {
		r.logf("ERROR", "admin/config/analysis_tools: apply kitchen: %v", err)
		WriteInternalError(w, "Failed to apply config change.")
		return
	}

	// Apply cookstyle re-score and capture result. The active target version can
	// change here, which moves the verified-removal (RemovedIn ≤ target) verdict,
	// so stored results are re-derived against the current classification.
	var rescoreResult RescoreResult
	if r.db != nil {
		res, err := RescoreCookstyleResults(req.Context(), r.db, r.logger)
		if err != nil {
			// Non-fatal: log the error but don't block the config save.
			r.logf("ERROR", "admin/config/analysis_tools: apply rescore: %v", err)
		} else {
			rescoreResult = res
		}
	}

	// Where the Chef tools are applies on the next run: both executors resolve
	// their binary when they run, reading this setting through the config
	// holder. Nothing here is resolved at startup any more, so there is nothing
	// to restart for — and telling an operator to restart for a change that has
	// already taken effect teaches them to distrust the notice on the screens
	// that do need one.
	toolPathMoved := ApplyResult{Reload: ReloadSubsystem}

	reload := worstGranularity([]ApplyResult{kitchenRes, toolPathMoved})
	WriteJSON(w, http.StatusOK, putConfigResponse{
		Value:           value,
		RestartRequired: reload == ReloadProcess,
		Reload:          reload.String(),
		VerdictsChanged: rescoreResult.Changed,
	})
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

	// Its own record. It used to be written back inside the analysis tools
	// section, which the Analysis Tools screen replaces wholesale — so
	// whichever of the two screens was saved last won, and that screen has
	// never carried these settings.
	value, err := configstore.SerializeValue(input)
	if err != nil {
		r.logf("ERROR", "admin/config/test-kitchen: serialise: %v", err)
		WriteInternalError(w, "Failed to serialise test kitchen config.")
		return
	}
	if err := r.configStore.Set(req.Context(), configstore.KeyTestKitchen, value, false, "admin"); err != nil {
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
	// Dynamically adjust kitchen queue worker pool to match new MaxConcurrentVMs.
	if r.kitchenQueue != nil {
		r.kitchenQueue.SetWorkerCount(r.liveConfig().AnalysisTools.TestKitchen.EffectiveMaxConcurrentVMs())
	}
	tkJSON, err := configstore.SerializeValue(input)
	if err != nil {
		r.logf("ERROR", "admin/config/test-kitchen: serialise response: %v", err)
		WriteInternalError(w, "Failed to serialise response.")
		return
	}
	// Worker pool resized in place above (subsystem) — no restart required.
	WriteJSON(w, http.StatusOK, putConfigResponse{Value: tkJSON, RestartRequired: false, Reload: ReloadSubsystem.String()})
}

func (r *Router) deleteAdminConfigTestKitchen(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	// Cleared back to nothing in its own record, rather than by rewriting the
	// analysis tools section around it.
	empty, err := configstore.SerializeValue(config.TestKitchenConfig{})
	if err != nil {
		r.logf("ERROR", "admin/config/test-kitchen: serialise: %v", err)
		WriteInternalError(w, "Failed to serialise test kitchen config.")
		return
	}
	if err := r.configStore.Set(req.Context(), configstore.KeyTestKitchen, empty, false, "admin"); err != nil {
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
	// Dynamically adjust kitchen queue worker pool (reset reverts to default).
	if r.kitchenQueue != nil {
		r.kitchenQueue.SetWorkerCount(r.liveConfig().AnalysisTools.TestKitchen.EffectiveMaxConcurrentVMs())
	}
	// What is there now, read back rather than assumed: the reset above cleared
	// the record and the defaults have since been applied over it.
	tkJSON, err := configstore.SerializeValue(r.liveConfig().AnalysisTools.TestKitchen)
	if err != nil {
		r.logf("ERROR", "admin/config/test-kitchen: serialise response: %v", err)
		WriteInternalError(w, "Failed to serialise response.")
		return
	}
	// Worker pool resized in place above (subsystem) — no restart required.
	WriteJSON(w, http.StatusOK, putConfigResponse{Value: tkJSON, RestartRequired: false, Reload: ReloadSubsystem.String()})
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

	if tk.StartRateWindowMinutes < 0 {
		return "analysis_tools.test_kitchen.start_rate_window_minutes must be >= 0."
	}
	if tk.StartRateMaxPerWindow < 0 {
		return "analysis_tools.test_kitchen.start_rate_max_per_window must be >= 0."
	}

	driver := tk.EffectiveDriver()
	if driver != "" && !knownTKDrivers[driver] {
		return fmt.Sprintf("analysis_tools.test_kitchen.driver %q is not a recognised driver profile.", driver)
	}
	if driver == "" && (len(tk.DriverSettings) > 0 || len(tk.DriverSecrets) > 0) {
		return "analysis_tools.test_kitchen.driver is required when driver_settings or driver_secrets are configured."
	}

	// Validate images registry: unique names, non-empty ID.
	seenImages := make(map[string]int)
	for i, img := range tk.Images {
		if img.Name == "" {
			return fmt.Sprintf("analysis_tools.test_kitchen.images[%d].name is required.", i)
		}
		if img.ID == "" {
			return fmt.Sprintf("analysis_tools.test_kitchen.images[%d].id is required.", i)
		}
		if prev, dup := seenImages[img.Name]; dup {
			return fmt.Sprintf("analysis_tools.test_kitchen.images[%d].name %q duplicates images[%d].", i, img.Name, prev)
		}
		seenImages[img.Name] = i
	}

	// Validate platform map: unique kitchen names, image refs must exist.
	seen := make(map[string]int)
	for i, entry := range tk.PlatformMap {
		if entry.KitchenName == "" {
			return fmt.Sprintf("analysis_tools.test_kitchen.platform_map[%d].kitchen_name is required.", i)
		}
		if prev, dup := seen[entry.KitchenName]; dup {
			return fmt.Sprintf("analysis_tools.test_kitchen.platform_map[%d].kitchen_name %q duplicates entry [%d].", i, entry.KitchenName, prev)
		}
		seen[entry.KitchenName] = i
		if entry.Image != "" {
			if _, ok := seenImages[entry.Image]; !ok {
				return fmt.Sprintf("analysis_tools.test_kitchen.platform_map[%d].image %q does not match any defined image.", i, entry.Image)
			}
		}
	}

	// Validate setup-script glob patterns: non-empty and syntactically valid.
	if tk.SetupScripts != nil {
		for _, fam := range []struct {
			name     string
			patterns []string
		}{
			{"linux", tk.SetupScripts.Linux},
			{"windows", tk.SetupScripts.Windows},
		} {
			for i, p := range fam.patterns {
				if p == "" {
					return fmt.Sprintf("analysis_tools.test_kitchen.setup_scripts.%s[%d] is empty.", fam.name, i)
				}
				if _, err := filepath.Match(p, ""); err != nil {
					return fmt.Sprintf("analysis_tools.test_kitchen.setup_scripts.%s[%d] %q is not a valid glob pattern.", fam.name, i, p)
				}
			}
		}
	}

	return ""
}
