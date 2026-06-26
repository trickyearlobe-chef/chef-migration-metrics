// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// ---------------------------------------------------------------------------
// GET /api/v1/cookstyle/cops
// ---------------------------------------------------------------------------

// copAggregateItem is one row in the cop aggregation response.
type copAggregateItem struct {
	CopName              string  `json:"cop_name"`
	Description          string  `json:"description"`
	Category             string  `json:"category"`
	Severity             string  `json:"severity"`
	Classification       string  `json:"classification"`
	ClassificationSource string  `json:"classification_source"`
	RemovedIn            string  `json:"removed_in,omitempty"`
	IntroducedIn         string  `json:"introduced_in,omitempty"`
	MigrationURL         string  `json:"migration_url,omitempty"`
	CookbooksAffected    int     `json:"cookbooks_affected"`
	TotalOffences        int     `json:"total_offences"`
	AutoCorrectablePct   float64 `json:"auto_correctable_pct"`
	Unblocks             int     `json:"unblocks"`
	IsCustom             bool    `json:"is_custom"`
}

// copAggregationSummary holds the headline counts returned with the cop list.
type copAggregationSummary struct {
	BlockerCops      int `json:"blocker_cops"`
	BlockerCookbooks int `json:"blocker_cookbooks"`
	ReviewCops       int `json:"review_cops"`
	ReviewCookbooks  int `json:"review_cookbooks"`
	NoiseCops        int `json:"noise_cops"`
	UnclassifiedCops int `json:"unclassified_cops"`
}

// copAggregationResponse wraps the summary, paginated data, and pagination info.
type copAggregationResponse struct {
	Summary    copAggregationSummary `json:"summary"`
	Data       []copAggregateItem    `json:"data"`
	Pagination PaginationResponse    `json:"pagination"`
}

// cookbookKey uniquely identifies a cookbook by source and name.
type cookbookKey struct {
	source string
	name   string
}

// copAccum accumulates per-cop offense data during aggregation.
type copAccum struct {
	severity    string
	offences    int
	correctable int
	cookbooks   map[cookbookKey]bool
}

// handleCookstyleCopSubroute dispatches /api/v1/cookstyle/cops/<cop_name>/...
// to the appropriate handler based on the URL suffix.
func (r *Router) handleCookstyleCopSubroute(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	switch {
	case strings.HasSuffix(path, "/cookbooks"):
		r.handleCookstyleCopCookbooks(w, req)
	case strings.HasSuffix(path, "/classification"):
		r.handleCookstyleCopClassification(w, req)
	default:
		WriteNotFound(w, "Unknown cop sub-resource. Use /cookbooks or /classification.")
	}
}

// handleCookstyleCops handles GET /api/v1/cookstyle/cops.
func (r *Router) handleCookstyleCops(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	targetVersion := queryString(req, "target_chef_version", "")
	if targetVersion == "" {
		targetVersion = r.defaultTargetVersion()
	}
	if targetVersion == "" {
		WriteBadRequest(w, "No target_chef_version specified and none configured.")
		return
	}

	source := queryString(req, "source", "")
	classFilter := queryString(req, "classification", "")
	pg := ParsePagination(req)
	sp := ParseSort(req, "cookbooks_affected", []string{"cookbooks_affected", "total_offences", "cop_name", "unblocks"})

	// Load operator overrides for the target version.
	overrides, err := r.db.ListCopClassifications(ctx, targetVersion)
	if err != nil {
		r.logf("ERROR", "listing cop classifications: %v", err)
		WriteInternalError(w, "Failed to load cop classifications.")
		return
	}
	overrideMap := make(map[string]string, len(overrides))
	for _, o := range overrides {
		overrideMap[o.CopName] = o.Classification
	}

	resolver := &analysis.CopClassificationResolver{
		OperatorOverrides: overrideMap,
		TargetChefVersion: targetVersion,
	}

	// Load all cookstyle results for aggregation.
	accum := make(map[string]*copAccum)

	// Track per-cookbook cop sets (for "unblocks" calculation).
	type cbCops struct {
		key  cookbookKey
		cops map[string]bool
	}
	var allCookbooks []cbCops

	addResults := func(offencesJSON []byte, src, name string) {
		if len(offencesJSON) == 0 {
			return
		}
		offenses := parseFullOffenses(offencesJSON)
		if len(offenses) == 0 {
			return
		}

		cbKey := cookbookKey{source: src, name: name}
		cbCopSet := make(map[string]bool)

		for _, o := range offenses {
			if o.CopName == "" {
				continue
			}
			a, ok := accum[o.CopName]
			if !ok {
				a = &copAccum{
					severity:  o.Severity,
					cookbooks: make(map[cookbookKey]bool),
				}
				accum[o.CopName] = a
			}
			a.offences++
			if o.Correctable {
				a.correctable++
			}
			a.cookbooks[cbKey] = true
			cbCopSet[o.CopName] = true
		}

		if len(cbCopSet) > 0 {
			allCookbooks = append(allCookbooks, cbCops{key: cbKey, cops: cbCopSet})
		}
	}

	// Load results based on source filter.
	if source == "" || source == "server" {
		results, err := r.db.ListAllServerCookbookCookstyleResultsByTargetVersion(ctx, targetVersion)
		if err != nil {
			r.logf("ERROR", "listing server cookstyle results for cops: %v", err)
			WriteInternalError(w, "Failed to load cookstyle results.")
			return
		}
		for _, res := range results {
			addResults(res.Offences, "server", res.CookbookName)
		}
	}
	if source == "" || source == "git" {
		results, err := r.db.ListGitRepoCookstyleResultsByTargetVersion(ctx, targetVersion)
		if err != nil {
			r.logf("ERROR", "listing git repo cookstyle results for cops: %v", err)
			WriteInternalError(w, "Failed to load cookstyle results.")
			return
		}
		for _, res := range results {
			addResults(res.Offences, "git", res.GitRepoName)
		}
	}

	// Compute "unblocks": for each blocker cop, how many cookbooks would pass
	// (have zero remaining blocker cops) if that cop alone were resolved.
	blockerCops := make(map[string]bool)
	for copName := range accum {
		if resolver.IsBlocker(copName) {
			blockerCops[copName] = true
		}
	}

	unblocksCounts := make(map[string]int) // cop_name → count of cookbooks unblocked
	for _, cb := range allCookbooks {
		// Count how many distinct blocker cops affect this cookbook.
		var blockers []string
		for cop := range cb.cops {
			if blockerCops[cop] {
				blockers = append(blockers, cop)
			}
		}
		// If exactly one blocker cop affects this cookbook, resolving it unblocks.
		if len(blockers) == 1 {
			unblocksCounts[blockers[0]]++
		}
	}

	// Build response items.
	items := make([]copAggregateItem, 0, len(accum))
	for copName, a := range accum {
		resolved := resolver.Resolve(copName)

		// Apply classification filter.
		if classFilter != "" && resolved.Classification != classFilter {
			continue
		}

		var autoPct float64
		if a.offences > 0 {
			autoPct = float64(a.correctable) / float64(a.offences) * 100
		}

		item := copAggregateItem{
			CopName:              copName,
			Category:             copNamespace(copName),
			Severity:             a.severity,
			Classification:       resolved.Classification,
			ClassificationSource: resolved.Source,
			CookbooksAffected:    len(a.cookbooks),
			TotalOffences:        a.offences,
			AutoCorrectablePct:   autoPct,
			Unblocks:             unblocksCounts[copName],
			IsCustom:             strings.HasPrefix(copName, "Custom/"),
		}

		// Enrich from cop mapping.
		if mapping := remediation.LookupCop(copName); mapping != nil {
			item.Description = mapping.Description
			item.RemovedIn = mapping.RemovedIn
			item.IntroducedIn = mapping.IntroducedIn
			item.MigrationURL = mapping.MigrationURL
		}

		items = append(items, item)
	}

	// Compute summary (before pagination, after classification filter).
	summary := computeCopSummary(items, accum, resolver, classFilter)

	// Sort.
	sortCopItems(items, sp)

	// Paginate.
	total := len(items)
	page, _ := PaginateSlice(items, pg)

	WriteJSON(w, http.StatusOK, copAggregationResponse{
		Summary:    summary,
		Data:       page,
		Pagination: NewPaginationResponse(pg, total),
	})
}

// computeCopSummary computes the headline summary counts. When a classification
// filter is active, the summary still reports totals across ALL cops (so the
// user can see the bigger picture).
func computeCopSummary(items []copAggregateItem, accum map[string]*copAccum, resolver *analysis.CopClassificationResolver, classFilter string) copAggregationSummary {
	var s copAggregationSummary

	blockerCBs := make(map[string]bool)
	reviewCBs := make(map[string]bool)

	// If there's a classification filter, we need to iterate ALL cops for summary.
	// Otherwise we can use the already-filtered items.
	if classFilter != "" {
		for copName, a := range accum {
			resolved := resolver.Resolve(copName)
			switch resolved.Classification {
			case analysis.ClassificationBlocker:
				s.BlockerCops++
				for cb := range a.cookbooks {
					blockerCBs[cb.name] = true
				}
			case analysis.ClassificationReview:
				s.ReviewCops++
				for cb := range a.cookbooks {
					reviewCBs[cb.name] = true
				}
			case analysis.ClassificationNoise:
				s.NoiseCops++
			default:
				s.UnclassifiedCops++
			}
		}
	} else {
		for _, item := range items {
			switch item.Classification {
			case analysis.ClassificationBlocker:
				s.BlockerCops++
				if a, ok := accum[item.CopName]; ok {
					for cb := range a.cookbooks {
						blockerCBs[cb.name] = true
					}
				}
			case analysis.ClassificationReview:
				s.ReviewCops++
				if a, ok := accum[item.CopName]; ok {
					for cb := range a.cookbooks {
						reviewCBs[cb.name] = true
					}
				}
			case analysis.ClassificationNoise:
				s.NoiseCops++
			default:
				s.UnclassifiedCops++
			}
		}
	}

	s.BlockerCookbooks = len(blockerCBs)
	s.ReviewCookbooks = len(reviewCBs)
	return s
}

// sortCopItems sorts the cop aggregate items.
func sortCopItems(items []copAggregateItem, sp SortParams) {
	sort.SliceStable(items, func(i, j int) bool {
		var less bool
		switch sp.Field {
		case "total_offences":
			less = items[i].TotalOffences < items[j].TotalOffences
		case "cop_name":
			less = items[i].CopName < items[j].CopName
		case "unblocks":
			less = items[i].Unblocks < items[j].Unblocks
		default: // "cookbooks_affected"
			less = items[i].CookbooksAffected < items[j].CookbooksAffected
		}
		if sp.Order == "desc" {
			return !less
		}
		return less
	})
}

// ---------------------------------------------------------------------------
// GET /api/v1/cookstyle/cops/:cop_name/cookbooks
// ---------------------------------------------------------------------------

// copCookbookItem is one row in the cop drill-down response.
type copCookbookItem struct {
	Source           string `json:"source"`
	Name             string `json:"name"`
	Version          string `json:"version"`
	Organisation     string `json:"organisation,omitempty"`
	OffenceCount     int    `json:"offence_count"`
	AutoCorrectable  int    `json:"auto_correctable"`
	WouldPassWithout bool   `json:"would_pass_without"`
}

// copCookbookResponse wraps the cop drill-down.
type copCookbookResponse struct {
	CopName    string             `json:"cop_name"`
	Data       []copCookbookItem  `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
}

// handleCookstyleCopCookbooks handles GET /api/v1/cookstyle/cops/<cop_name>/cookbooks.
func (r *Router) handleCookstyleCopCookbooks(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Extract cop name from URL path: /api/v1/cookstyle/cops/<cop_name>/cookbooks
	copName := extractCopNameFromPath(req.URL.Path)
	if copName == "" {
		WriteBadRequest(w, "Missing cop name in URL path.")
		return
	}

	targetVersion := queryString(req, "target_chef_version", "")
	if targetVersion == "" {
		targetVersion = r.defaultTargetVersion()
	}
	if targetVersion == "" {
		WriteBadRequest(w, "No target_chef_version specified and none configured.")
		return
	}

	source := queryString(req, "source", "")
	pg := ParsePagination(req)

	// Load operator overrides for "would pass without" calculation.
	overrides, err := r.db.ListCopClassifications(ctx, targetVersion)
	if err != nil {
		r.logf("ERROR", "listing cop classifications for cop drill-down: %v", err)
		WriteInternalError(w, "Failed to load cop classifications.")
		return
	}
	overrideMap := make(map[string]string, len(overrides))
	for _, o := range overrides {
		overrideMap[o.CopName] = o.Classification
	}
	resolver := &analysis.CopClassificationResolver{
		OperatorOverrides: overrideMap,
		TargetChefVersion: targetVersion,
	}

	// Load failure rules for "would pass without" evaluation.
	rules := r.cookstyleFailureRules()

	var items []copCookbookItem

	if source == "" || source == "server" {
		results, err := r.db.ListAllServerCookbookCookstyleResultsByTargetVersion(ctx, targetVersion)
		if err != nil {
			r.logf("ERROR", "listing server results for cop drill-down: %v", err)
			WriteInternalError(w, "Failed to load cookstyle results.")
			return
		}
		for _, res := range results {
			offenses := parseFullOffenses(res.Offences)
			item := buildCopCookbookItem(copName, "server", res.CookbookName, res.CookbookVersion, res.OrganisationName, offenses, resolver, rules)
			if item != nil {
				items = append(items, *item)
			}
		}
	}

	if source == "" || source == "git" {
		results, err := r.db.ListGitRepoCookstyleResultsByTargetVersion(ctx, targetVersion)
		if err != nil {
			r.logf("ERROR", "listing git results for cop drill-down: %v", err)
			WriteInternalError(w, "Failed to load cookstyle results.")
			return
		}
		for _, res := range results {
			offenses := parseFullOffenses(res.Offences)
			item := buildCopCookbookItem(copName, "git", res.GitRepoName, res.CommitSHA, "", offenses, resolver, rules)
			if item != nil {
				items = append(items, *item)
			}
		}
	}

	// Sort by offence count descending.
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].OffenceCount > items[j].OffenceCount
	})

	total := len(items)
	page, _ := PaginateSlice(items, pg)

	WriteJSON(w, http.StatusOK, copCookbookResponse{
		CopName:    copName,
		Data:       page,
		Pagination: NewPaginationResponse(pg, total),
	})
}

// buildCopCookbookItem creates a drill-down item if the given cookbook has
// offenses for the specified cop. Returns nil if the cop is not present.
func buildCopCookbookItem(copName, source, name, version, org string, offenses []fullOffense, resolver *analysis.CopClassificationResolver, rules analysis.CookstyleFailureRules) *copCookbookItem {
	var count, correctable int
	for _, o := range offenses {
		if o.CopName == copName {
			count++
			if o.Correctable {
				correctable++
			}
		}
	}
	if count == 0 {
		return nil
	}

	// "Would pass without": remove offenses for this cop, re-evaluate.
	var remaining []analysis.CookstyleOffense
	for _, o := range offenses {
		if o.CopName != copName {
			remaining = append(remaining, analysis.CookstyleOffense{
				Severity: o.Severity,
				CopName:  o.CopName,
			})
		}
	}
	wouldPass := analysis.EvaluatePassFailWithClassification(remaining, rules, resolver)

	return &copCookbookItem{
		Source:           source,
		Name:             name,
		Version:          version,
		Organisation:     org,
		OffenceCount:     count,
		AutoCorrectable:  correctable,
		WouldPassWithout: wouldPass,
	}
}

// ---------------------------------------------------------------------------
// PUT /api/v1/cookstyle/cops/:cop_name/classification
// ---------------------------------------------------------------------------

// classificationPutRequest is the request body for setting a cop classification.
type classificationPutRequest struct {
	TargetChefVersion string `json:"target_chef_version"`
	Classification    string `json:"classification"`
	Reason            string `json:"reason"`
}

// handleCookstyleCopClassification handles PUT/DELETE
// /api/v1/cookstyle/cops/<cop_name>/classification.
//
// Reclassifying a cop is a migration-policy decision (it changes verdicts,
// complexity, and node readiness across the estate), so it is restricted to
// admins even though the Cop Analysis view that hosts it is available to all
// authenticated users. The GET aggregation/drill-down routes stay open.
func (r *Router) handleCookstyleCopClassification(w http.ResponseWriter, req *http.Request) {
	if !requireAdminRole(w, req) {
		return
	}
	switch req.Method {
	case http.MethodPut:
		r.putCookstyleCopClassification(w, req)
	case http.MethodDelete:
		r.deleteCookstyleCopClassification(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports PUT and DELETE.")
	}
}

func (r *Router) putCookstyleCopClassification(w http.ResponseWriter, req *http.Request) {
	copName := extractCopNameFromClassificationPath(req.URL.Path)
	if copName == "" {
		WriteBadRequest(w, "Missing cop name in URL path.")
		return
	}

	var body classificationPutRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid JSON request body.")
		return
	}

	if body.TargetChefVersion == "" {
		body.TargetChefVersion = r.defaultTargetVersion()
	}
	if body.TargetChefVersion == "" {
		WriteBadRequest(w, "No target_chef_version specified and none configured.")
		return
	}

	switch body.Classification {
	case "blocker", "review", "noise":
		// valid
	default:
		WriteBadRequest(w, "classification must be one of: blocker, review, noise")
		return
	}

	ctx := req.Context()
	if err := r.db.UpsertCopClassification(ctx, copName, body.TargetChefVersion, body.Classification, body.Reason, adminUsername(req)); err != nil {
		r.logf("ERROR", "upserting cop classification: %v", err)
		WriteInternalError(w, "Failed to save classification.")
		return
	}

	// Re-evaluation propagation: re-derive verdicts/compat/complexity and
	// recompute dependent-node readiness for this cop's affected closure.
	prop := r.propagateCop(ctx, copName, body.TargetChefVersion)
	r.auditCookstyle(req, "cop_reclassified", copName, body.TargetChefVersion, map[string]any{
		"classification": body.Classification,
		"reason":         body.Reason,
		"propagation":    prop,
	})
	r.emitCookstyleRecomputed(copName, body.TargetChefVersion, prop)

	WriteJSON(w, http.StatusOK, map[string]any{
		"cop_name":            copName,
		"target_chef_version": body.TargetChefVersion,
		"classification":      body.Classification,
		"status":              "saved",
		"propagation":         prop,
	})
}

func (r *Router) deleteCookstyleCopClassification(w http.ResponseWriter, req *http.Request) {
	copName := extractCopNameFromClassificationPath(req.URL.Path)
	if copName == "" {
		WriteBadRequest(w, "Missing cop name in URL path.")
		return
	}

	targetVersion := queryString(req, "target_chef_version", "")
	if targetVersion == "" {
		targetVersion = r.defaultTargetVersion()
	}
	if targetVersion == "" {
		WriteBadRequest(w, "No target_chef_version specified and none configured.")
		return
	}

	ctx := req.Context()
	if err := r.db.DeleteCopClassification(ctx, copName, targetVersion); err != nil {
		r.logf("ERROR", "deleting cop classification: %v", err)
		WriteInternalError(w, "Failed to delete classification.")
		return
	}

	// Removing an override changes the resolved classification (falls back to
	// RemovedIn/curated/unclassified): re-evaluate the affected closure.
	prop := r.propagateCop(ctx, copName, targetVersion)
	r.auditCookstyle(req, "cop_classification_removed", copName, targetVersion, map[string]any{
		"propagation": prop,
	})
	r.emitCookstyleRecomputed(copName, targetVersion, prop)

	WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted", "propagation": prop})
}

// ---------------------------------------------------------------------------
// CRUD /api/v1/cookstyle/custom-cops
// ---------------------------------------------------------------------------

// handleCookstyleCustomCops handles GET/POST for the custom cops collection.
func (r *Router) handleCookstyleCustomCops(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.listCustomCops(w, req)
	case http.MethodPost:
		r.createCustomCop(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and POST.")
	}
}

// handleCookstyleCustomCop handles GET/PUT/DELETE for a single custom cop.
func (r *Router) handleCookstyleCustomCop(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.getCustomCop(w, req)
	case http.MethodPut:
		r.updateCustomCop(w, req)
	case http.MethodDelete:
		r.deleteCustomCop(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET, PUT, and DELETE.")
	}
}

func (r *Router) listCustomCops(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	cops, err := r.db.ListCustomCopDefinitions(ctx)
	if err != nil {
		r.logf("ERROR", "listing custom cops: %v", err)
		WriteInternalError(w, "Failed to list custom cops.")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": cops})
}

func (r *Router) createCustomCop(w http.ResponseWriter, req *http.Request) {
	var body datastore.CustomCopDefinition
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid JSON request body.")
		return
	}
	if err := validateCustomCop(body); err != nil {
		WriteBadRequest(w, err.Error())
		return
	}

	ctx := req.Context()
	id, err := r.db.CreateCustomCopDefinition(ctx, body)
	if err != nil {
		r.logf("ERROR", "creating custom cop: %v", err)
		WriteInternalError(w, "Failed to create custom cop.")
		return
	}

	body.ID = id
	r.propagateCustomCop(ctx, req, "custom_cop_created", body.CopName)
	WriteJSON(w, http.StatusCreated, body)
}

func (r *Router) getCustomCop(w http.ResponseWriter, req *http.Request) {
	copName := extractCustomCopName(req.URL.Path)
	if copName == "" {
		WriteBadRequest(w, "Missing custom cop name in URL path.")
		return
	}

	ctx := req.Context()
	cop, err := r.db.GetCustomCopDefinition(ctx, copName)
	if err != nil {
		r.logf("ERROR", "getting custom cop: %v", err)
		WriteInternalError(w, "Failed to get custom cop.")
		return
	}
	if cop == nil {
		WriteNotFound(w, "Custom cop not found.")
		return
	}
	WriteJSON(w, http.StatusOK, cop)
}

func (r *Router) updateCustomCop(w http.ResponseWriter, req *http.Request) {
	copName := extractCustomCopName(req.URL.Path)
	if copName == "" {
		WriteBadRequest(w, "Missing custom cop name in URL path.")
		return
	}

	var body datastore.CustomCopDefinition
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid JSON request body.")
		return
	}
	body.CopName = copName
	if err := validateCustomCop(body); err != nil {
		WriteBadRequest(w, err.Error())
		return
	}

	ctx := req.Context()
	if err := r.db.UpdateCustomCopDefinition(ctx, &body); err != nil {
		r.logf("ERROR", "updating custom cop: %v", err)
		WriteInternalError(w, "Failed to update custom cop.")
		return
	}

	r.propagateCustomCop(ctx, req, "custom_cop_updated", body.CopName)
	WriteJSON(w, http.StatusOK, body)
}

func (r *Router) deleteCustomCop(w http.ResponseWriter, req *http.Request) {
	copName := extractCustomCopName(req.URL.Path)
	if copName == "" {
		WriteBadRequest(w, "Missing custom cop name in URL path.")
		return
	}

	ctx := req.Context()
	if err := r.db.DeleteCustomCopDefinition(ctx, copName); err != nil {
		r.logf("ERROR", "deleting custom cop: %v", err)
		WriteInternalError(w, "Failed to delete custom cop.")
		return
	}

	r.propagateCustomCop(ctx, req, "custom_cop_deleted", copName)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// fullOffense extends the basic parsed offense with the correctable flag.
type fullOffense struct {
	CopName     string `json:"cop_name"`
	Severity    string `json:"severity"`
	Correctable bool   `json:"corrected"`
}

// parseFullOffenses parses the offences JSONB into a flat list that includes
// the correctable field. Handles both file-based and flat formats.
func parseFullOffenses(data []byte) []fullOffense {
	if len(data) == 0 {
		return nil
	}

	// Try file-based RuboCop format first.
	type fileOffense struct {
		CopName     string `json:"cop_name"`
		Severity    string `json:"severity"`
		Correctable bool   `json:"corrected"`
	}
	type fileEntry struct {
		Path     string        `json:"path"`
		Offenses []fileOffense `json:"offenses"`
	}

	var fileEntries []fileEntry
	if err := json.Unmarshal(data, &fileEntries); err == nil && len(fileEntries) > 0 && fileEntries[0].Path != "" {
		var result []fullOffense
		for _, fe := range fileEntries {
			for _, o := range fe.Offenses {
				result = append(result, fullOffense{CopName: o.CopName, Severity: o.Severity, Correctable: o.Correctable})
			}
		}
		return result
	}

	// Try flat format.
	var flat []fullOffense
	if err := json.Unmarshal(data, &flat); err == nil {
		return flat
	}

	return nil
}

// extractCopNameFromPath extracts the cop name from a path like
// /api/v1/cookstyle/cops/Chef/Deprecations/NodeSet/cookbooks
// The cop name is everything between "/cops/" and "/cookbooks".
func extractCopNameFromPath(path string) string {
	const prefix = "/api/v1/cookstyle/cops/"
	const suffix = "/cookbooks"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}
	start := len(prefix)
	end := len(path) - len(suffix)
	if end <= start {
		return ""
	}
	return path[start:end]
}

// extractCopNameFromClassificationPath extracts the cop name from
// /api/v1/cookstyle/cops/Chef/Deprecations/NodeSet/classification
func extractCopNameFromClassificationPath(path string) string {
	const prefix = "/api/v1/cookstyle/cops/"
	const suffix = "/classification"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}
	start := len(prefix)
	end := len(path) - len(suffix)
	if end <= start {
		return ""
	}
	return path[start:end]
}

// extractCustomCopName extracts the cop name from /api/v1/cookstyle/custom-cops/Custom/...
func extractCustomCopName(path string) string {
	const prefix = "/api/v1/cookstyle/custom-cops/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	name := path[len(prefix):]
	name = strings.TrimSuffix(name, "/")
	return name
}

// validateCustomCop validates a custom cop definition.
func validateCustomCop(c datastore.CustomCopDefinition) error {
	if c.CopName == "" {
		return errMissingField("cop_name")
	}
	if !strings.HasPrefix(c.CopName, "Custom/") {
		return errInvalidField("cop_name", "must start with 'Custom/'")
	}
	if c.Pattern == "" {
		return errMissingField("pattern")
	}
	switch c.PatternType {
	case "regex", "literal":
		// valid
	default:
		return errInvalidField("pattern_type", "must be 'regex' or 'literal'")
	}
	switch c.Classification {
	case "blocker", "review", "noise":
		// valid
	case "":
		return errMissingField("classification")
	default:
		return errInvalidField("classification", "must be one of: blocker, review, noise")
	}
	return nil
}

type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func errMissingField(field string) error {
	return &validationError{msg: field + " is required"}
}

func errInvalidField(field, reason string) error {
	return &validationError{msg: field + ": " + reason}
}

// ---------------------------------------------------------------------------
// Re-evaluation propagation + audit helpers
// ---------------------------------------------------------------------------

// propagateCop runs the scoped recompute closure for a single cop × target
// version. Best-effort: a nil propagator or an error is logged, never fatal —
// the classification write has already succeeded. Returns the result (zero value
// when no propagator is wired) for inclusion in the response and audit details.
func (r *Router) propagateCop(ctx context.Context, copName, targetVersion string) PropagationResult {
	if r.cookstylePropagator == nil {
		return PropagationResult{Target: targetVersion}
	}
	res, err := r.cookstylePropagator.PropagateReclassification(ctx, copName, targetVersion)
	if err != nil {
		r.logf("ERROR", "cookstyle propagation for cop %q target %q: %v", copName, targetVersion, err)
	}
	return res
}

// propagateCustomCop runs the recompute closure for a custom-cop change across
// every configured target version (custom-cop classification is target-agnostic)
// and records a criteria-change audit entry.
func (r *Router) propagateCustomCop(ctx context.Context, req *http.Request, action, copName string) {
	var results []PropagationResult
	if r.cookstylePropagator != nil {
		for _, t := range r.liveConfig().TargetChefVersions {
			results = append(results, r.propagateCop(ctx, copName, t))
		}
	}
	r.auditCookstyle(req, action, copName, "", map[string]any{"propagation": results})
	r.emitCookstyleRecomputed(copName, "", PropagationResult{})
}

// emitCookstyleRecomputed broadcasts a status-changed event so open UI pages
// refresh after a criteria change propagates (spec: Re-evaluation & Propagation
// step 6). Nil-safe when no event hub is wired.
func (r *Router) emitCookstyleRecomputed(copName, targetVersion string, prop PropagationResult) {
	if r.hub == nil {
		return
	}
	r.hub.Broadcast(NewEvent(EventCookbookStatusChanged, map[string]any{
		"cause":               "cookstyle_reclassification",
		"cop_name":            copName,
		"target_chef_version": targetVersion,
		"propagation":         prop,
	}))
}

// auditCookstyle records a CookStyle criteria-change event for explainability.
// Best-effort — a write failure is logged but never blocks the request.
func (r *Router) auditCookstyle(req *http.Request, action, copName, targetVersion string, details map[string]any) {
	var raw json.RawMessage
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			raw = b
		}
	}
	if err := r.db.InsertCookstyleAuditEntry(req.Context(), datastore.InsertCookstyleAuditParams{
		Action:            action,
		Actor:             adminUsername(req),
		CopName:           copName,
		TargetChefVersion: targetVersion,
		Details:           raw,
	}); err != nil {
		r.logf("WARN", "cookstyle: failed to write audit log: %v", err)
	}
}

// cookstyleFailureRules returns the current cookstyle failure rules from config.
func (r *Router) cookstyleFailureRules() analysis.CookstyleFailureRules {
	cfg := r.liveConfig()
	if cfg != nil && cfg.AnalysisTools.CookstyleFailureRules != nil {
		return analysis.NewCookstyleFailureRules(cfg.AnalysisTools.CookstyleFailureRules)
	}
	return analysis.DefaultFailureRules()
}
