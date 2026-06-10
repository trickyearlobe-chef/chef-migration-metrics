// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// putConfigResponse is the standard JSON body for PUT admin config responses.
// Value contains the stored section data; RestartRequired is true when the
// change requires an application restart to take effect.
type putConfigResponse struct {
	Value           json.RawMessage `json:"value"`
	RestartRequired bool            `json:"restart_required"`
	// Warnings carries non-fatal advisories about a save that was accepted and
	// stored anyway (e.g. an incomplete TLS certificate chain — tls-static.md
	// § 2.2). Omitted when empty.
	Warnings []string `json:"warnings,omitempty"`
}

// adminConfigCronRe matches a basic 5-field cron expression.
var adminConfigCronRe = regexp.MustCompile(`^(\S+\s+){4}\S+$`)

// adminConfigSemverRe matches MAJOR.MINOR.PATCH version strings.
var adminConfigSemverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/collection
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigCollection(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		r.writeAdminConfigSection(w, &config.Config{Collection: cfg.Collection}, configstore.KeyCollection)
	case http.MethodPut:
		r.putAdminConfigCollection(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigCollection(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input config.CollectionConfig
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	if !adminConfigCronRe.MatchString(input.Schedule) {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"collection.schedule: must be a valid cron expression with 5 space-separated fields.")
		return
	}
	if input.StaleNodeThresholdDays < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"collection.stale_node_threshold_days: must be >= 1.")
		return
	}
	if input.StaleCookbookThresholdDays < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"collection.stale_cookbook_threshold_days: must be >= 1.")
		return
	}

	r.storeAdminConfigSection(w, req, &config.Config{Collection: input}, configstore.KeyCollection, false)
}

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/target-versions
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigTargetVersions(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		r.writeAdminConfigSection(w, &config.Config{TargetChefVersions: cfg.TargetChefVersions}, configstore.KeyTargetChefVersions)
	case http.MethodPut:
		r.putAdminConfigTargetVersions(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigTargetVersions(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input []string
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	for i, v := range input {
		if !adminConfigSemverRe.MatchString(v) {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("target_chef_versions[%d]: %q is not a valid semver (expected MAJOR.MINOR.PATCH).", i, v))
			return
		}
	}

	// Reset materialised status columns — results for old target are invalid.
	if err := r.db.ResetAllGitRepoStatuses(req.Context()); err != nil {
		r.logf("ERROR", "admin/config/target-chef-versions: reset git repo statuses: %v", err)
		// Non-fatal: statuses will be recomputed on next scan cycle.
	}

	r.storeAdminConfigSection(w, req, &config.Config{TargetChefVersions: input}, configstore.KeyTargetChefVersions, false)
}

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/git-urls
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigGitURLs(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		r.writeAdminConfigSection(w, &config.Config{GitBaseURLs: cfg.GitBaseURLs}, configstore.KeyGitBaseURLs)
	case http.MethodPut:
		r.putAdminConfigGitURLs(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigGitURLs(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input []string
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	r.storeAdminConfigSection(w, req, &config.Config{GitBaseURLs: input}, configstore.KeyGitBaseURLs, false)
}

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/concurrency
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigConcurrency(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		r.writeAdminConfigSection(w, &config.Config{Concurrency: cfg.Concurrency}, configstore.KeyConcurrency)
	case http.MethodPut:
		r.putAdminConfigConcurrency(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigConcurrency(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input config.ConcurrencyConfig
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	type concurrencyField struct {
		name string
		val  int
	}
	fields := []concurrencyField{
		{"organisation_collection", input.OrganisationCollection},
		{"node_page_fetching", input.NodePageFetching},
		{"git_pull", input.GitPull},
		{"cookbook_download", input.CookbookDownload},
		{"cookstyle_scan", input.CookstyleScan},
		{"readiness_evaluation", input.ReadinessEvaluation},
	}
	for _, f := range fields {
		if f.val < 1 {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("concurrency.%s: must be >= 1.", f.name))
			return
		}
	}

	r.storeAdminConfigSection(w, req, &config.Config{Concurrency: input}, configstore.KeyConcurrency, false)
}

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/logging
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigLogging(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		r.writeAdminConfigSection(w, &config.Config{Logging: cfg.Logging}, configstore.KeyLogging)
	case http.MethodPut:
		r.putAdminConfigLogging(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigLogging(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input config.LoggingConfig
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	switch strings.ToUpper(input.Level) {
	case "DEBUG", "INFO", "WARN", "ERROR":
		// valid
	default:
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			fmt.Sprintf("logging.level: must be one of DEBUG, INFO, WARN, ERROR, got %q.", input.Level))
		return
	}

	r.storeAdminConfigSection(w, req, &config.Config{Logging: input}, configstore.KeyLogging, false)
}

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/organisations
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigOrganisations(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		r.writeAdminConfigSection(w, &config.Config{Organisations: cfg.Organisations}, configstore.KeyOrganisations)
	case http.MethodPut:
		r.putAdminConfigOrganisations(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigOrganisations(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input []config.Organisation
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	if len(input) == 0 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"at least one organisation must be configured.")
		return
	}

	seen := make(map[string]bool)
	for i, org := range input {
		prefix := fmt.Sprintf("organisations[%d]", i)
		if org.Name == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("%s: name is required", prefix))
			return
		}
		if seen[org.Name] {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("%s: duplicate organisation name %q", prefix, org.Name))
			return
		}
		seen[org.Name] = true
		if org.ChefServerURL == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("%s: chef_server_url is required", prefix))
			return
		}
		// org_name is not entered in the UI — it is derived from the full org
		// URL's "/organizations/<org>" segment (it labels the User-Agent; the
		// URL is authoritative). An explicit value is honoured. If absent and
		// underivable, reject — the org table requires a non-empty org name.
		if org.OrgName == "" {
			derived := deriveOrgNameFromURL(org.ChefServerURL)
			if derived == "" {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("%s: could not derive org name from chef_server_url %q — expected a full org URL like https://chef.example.com/organizations/<org>", prefix, org.ChefServerURL))
				return
			}
			input[i].OrgName = derived
			org.OrgName = derived
		}
		if org.ClientName == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("%s: client_name is required", prefix))
			return
		}
		if org.ClientKeyPath == "" && org.ClientKeyCredential == "" {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				fmt.Sprintf("%s: one of client_key_path or client_key_credential is required", prefix))
			return
		}
		if org.ClientKeyPath != "" {
			if _, err := os.Stat(org.ClientKeyPath); err != nil {
				WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
					fmt.Sprintf("%s: client_key_path %q: %v", prefix, org.ClientKeyPath, err))
				return
			}
		}
	}

	var postReload []func(context.Context) error
	if r.onOrganisationsChanged != nil {
		postReload = append(postReload, r.onOrganisationsChanged)
	}
	r.storeAdminConfigSection(w, req, &config.Config{Organisations: input}, configstore.KeyOrganisations, false, postReload...)
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// deriveOrgNameFromURL extracts the Chef organisation name from a full org
// URL's "/organizations/<org>" path segment, e.g.
// "https://chef.example.com/organizations/myorg" → "myorg". Returns "" when
// the segment is absent. Used to populate the User-Agent label without asking
// the operator to repeat the org name they already typed in the URL.
func deriveOrgNameFromURL(u string) string {
	_, rest, found := strings.Cut(u, "/organizations/")
	if !found {
		return ""
	}
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

// decodeAdminConfigBody reads the request body and unmarshals it via the YAML
// decoder (which honours yaml struct tags and accepts JSON as valid YAML).
// Returns true on success; on failure a 400 response has already been written.
func decodeAdminConfigBody(w http.ResponseWriter, req *http.Request, target any) bool {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		WriteBadRequest(w, "Failed to read request body.")
		return false
	}
	if err := yaml.Unmarshal(body, target); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return false
	}
	return true
}

// storeAdminConfigSection serialises the named key from partial via
// ConfigToSections, writes it to the config store, optionally triggers a
// ConfigHolder reload, and responds with the stored JSON on success.
// postReload hooks run after the section is stored and the live config has
// reloaded, before the success response is written. A hook error fails the
// request with 500 (the section is already persisted, but the running app
// could not be brought into sync — surfacing the error is better than a silent
// drift). Used by the organisations PUT to reconcile the operational org table.
func (r *Router) storeAdminConfigSection(w http.ResponseWriter, req *http.Request, partial *config.Config, key string, restartRequired bool, postReload ...func(context.Context) error) {
	sections, err := configstore.ConfigToSections(partial)
	if err != nil {
		r.logf("ERROR", "admin/config/%s: serialise: %v", key, err)
		WriteInternalError(w, "Failed to serialise config section.")
		return
	}

	value := sections[key]
	if err := r.configStore.Set(req.Context(), key, value, false, "admin"); err != nil {
		r.logf("ERROR", "admin/config/%s: store: %v", key, err)
		WriteInternalError(w, "Failed to store config section.")
		return
	}

	if r.configHolder != nil {
		if err := r.configHolder.Reload(req.Context()); err != nil {
			r.logf("ERROR", "admin/config/%s: reload: %v", key, err)
			WriteInternalError(w, "Failed to reload config after update.")
			return
		}
	}

	for _, hook := range postReload {
		if err := hook(req.Context()); err != nil {
			r.logf("ERROR", "admin/config/%s: post-update hook: %v", key, err)
			WriteInternalError(w, "Failed to apply config change.")
			return
		}
	}

	WriteJSON(w, http.StatusOK, putConfigResponse{Value: value, RestartRequired: restartRequired})
}

// writeAdminConfigSection serialises the named key from partial and writes
// it as a 200 JSON response.
func (r *Router) writeAdminConfigSection(w http.ResponseWriter, partial *config.Config, key string) {
	sections, err := configstore.ConfigToSections(partial)
	if err != nil {
		r.logf("ERROR", "admin/config/%s: serialise: %v", key, err)
		WriteInternalError(w, "Failed to serialise config section.")
		return
	}
	WriteJSON(w, http.StatusOK, sections[key])
}
