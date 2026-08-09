// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
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
	// CookbooksAffected counts cookbooks carrying this cop in code that runs on
	// a converging node — the cookbooks it can actually block.
	//
	// CookbooksExcludedOnly counts cookbooks that carry it ONLY in files the
	// converge never executes: a helper task, a pipeline, a test suite. That
	// work is real and will break on the new Ruby exactly as predicted, but it
	// belongs to whoever owns the pipeline. It is reported alongside rather than
	// folded in or dropped, because how widespread it is the most useful thing
	// about it: one fix repeated across four hundred repositories is a different
	// conversation from four hundred separate problems.
	CookbooksAffected     int     `json:"cookbooks_affected"`
	CookbooksExcludedOnly int     `json:"cookbooks_excluded_only"`
	TotalOffences         int     `json:"total_offences"`
	ExcludedOffences      int     `json:"excluded_offences"`
	AutoCorrectablePct    float64 `json:"auto_correctable_pct"`
	Unblocks              int     `json:"unblocks"`
	IsCustom              bool    `json:"is_custom"`
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
//
// The two cookbook sets are the whole point of the scan-scope correction:
// `cookbooks` holds those where this cop sits in code Chef executes, and
// `excludedCookbooks` those where it appears in a file Chef never executes.
// A cookbook can be in both — the same finding in a recipe and in a Rakefile —
// and is then reported only as affected, because the recipe copy is what
// decides its verdict.
type copAccum struct {
	severity          string
	offences          int
	excludedOffences  int
	correctable       int
	cookbooks         map[cookbookKey]bool
	excludedCookbooks map[cookbookKey]bool
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
	triggeredOnly := queryString(req, "triggered_only", "") == "true"
	pg := ParsePagination(req)
	sp := ParseSort(req, "cookbooks_affected", []string{"cookbooks_affected", "cookbooks_excluded_only", "total_offences", "cop_name", "unblocks"})

	// Load operator overrides for the target version and build the resolver.
	resolver, err := r.copResolver(ctx, targetVersion)
	if err != nil {
		r.logf("ERROR", "listing cop classifications: %v", err)
		WriteInternalError(w, "Failed to load cop classifications.")
		return
	}

	// Load all cookstyle results for aggregation.
	accum := make(map[string]*copAccum)

	// Track per-cookbook cop sets (for "unblocks" calculation).
	type cbCops struct {
		key  cookbookKey
		cops map[string]bool
	}
	var allCookbooks []cbCops

	// The repository is not the cookbook: an offence in a file the converge never
	// executes is counted, but separately, and it never contributes to a
	// cookbook being blocked. See journeys/scan-trust.md.
	scanScope := r.scanScope(ctx)

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
					severity:          o.Severity,
					cookbooks:         make(map[cookbookKey]bool),
					excludedCookbooks: make(map[cookbookKey]bool),
				}
				accum[o.CopName] = a
			}
			a.offences++
			if o.Correctable {
				a.correctable++
			}
			if scanScope.ExcludesPath(o.Location.File) {
				a.excludedOffences++
				a.excludedCookbooks[cbKey] = true
				// Deliberately NOT added to cbCopSet: this occurrence cannot
				// block the cookbook, so it must not count towards "unblocks"
				// either, or fixing it would be credited with a release it does
				// not deliver.
				continue
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

	// Build the known-cop universe. The spec's Cop Classifications surface lists
	// ALL known cops — curated defaults + RemovedIn mappings + custom definitions
	// + scanned — not only cops already seen in a scan, so operators can classify
	// proactively. Cops that have never triggered appear with zero stats.
	known := make(map[string]bool, len(accum))
	for name := range accum {
		known[name] = true
	}
	// The verified-removal mapping is the enumerable static source of known cops
	// (the structural-Noise rules classify whole namespaces, not named cops).
	for _, m := range remediation.AllCopMappings() {
		if m.CopName != "" {
			known[m.CopName] = true
		}
	}
	// Custom cop definitions are not in the mapping table; pull their metadata
	// from the DB. A load failure is non-fatal — the rest of the universe stands.
	customByName := make(map[string]datastore.CustomCopDefinition)
	if defs, err := r.db.ListCustomCopDefinitions(ctx); err != nil {
		r.logf("ERROR", "listing custom cop definitions for cops: %v", err)
	} else {
		for _, d := range defs {
			known[d.CopName] = true
			customByName[d.CopName] = d
		}
	}
	// Union the live registry's Chef/* cops so unclassified Chef cops are
	// listable before they ever trigger. Generic-Ruby cops (Style/Layout/Lint/…)
	// stay out of the default list — they are still auto-added on trigger via
	// accum. A missing/failed registry is non-fatal (static universe stands).
	reg := r.copRegistrySnapshot(ctx)
	if reg != nil {
		for _, e := range reg.ChefCops() {
			known[e.CopName] = true
		}
	}

	// Build the full item set (unfiltered). The summary is computed from this so
	// it reflects the whole picture regardless of the data-page filters.
	allItems := make([]copAggregateItem, 0, len(known))
	for copName := range known {
		resolved := resolver.Resolve(copName)

		var offences, excludedOffences, correctable, cbAffected, cbExcludedOnly int
		var severity string
		if a := accum[copName]; a != nil {
			offences = a.offences
			excludedOffences = a.excludedOffences
			correctable = a.correctable
			cbAffected = len(a.cookbooks)
			severity = a.severity
			// "Excluded only" — a cookbook carrying this cop in both a recipe
			// and a Rakefile is already counted as affected, and counting it
			// twice would make the two columns stop summing to anything.
			for cb := range a.excludedCookbooks {
				if !a.cookbooks[cb] {
					cbExcludedOnly++
				}
			}
		}

		var autoPct float64
		if offences > 0 {
			autoPct = float64(correctable) / float64(offences) * 100
		}

		item := copAggregateItem{
			CopName:               copName,
			Category:              copNamespace(copName),
			Severity:              severity,
			Classification:        resolved.Classification,
			ClassificationSource:  resolved.Source,
			CookbooksAffected:     cbAffected,
			CookbooksExcludedOnly: cbExcludedOnly,
			TotalOffences:         offences,
			ExcludedOffences:      excludedOffences,
			AutoCorrectablePct:    autoPct,
			Unblocks:              unblocksCounts[copName],
			IsCustom:              strings.HasPrefix(copName, "Custom/"),
		}

		// Enrich from cop mapping.
		if mapping := remediation.LookupCop(copName); mapping != nil {
			item.Description = mapping.Description
			item.RemovedIn = mapping.RemovedIn
			item.IntroducedIn = mapping.IntroducedIn
			item.MigrationURL = mapping.MigrationURL
		}
		// Custom cops carry their own description/removed_in in the DB definition.
		if d, ok := customByName[copName]; ok {
			if item.Description == "" {
				item.Description = d.Description
			}
			if item.RemovedIn == "" {
				item.RemovedIn = d.RemovedIn
			}
		}
		// Fall back to the live registry's description so a Chef/* cop listed
		// from the registry (never triggered, no mapping) is not blank.
		if item.Description == "" && reg != nil {
			if e, ok := reg.Lookup(copName); ok {
				item.Description = e.Description
			}
		}

		allItems = append(allItems, item)
	}

	// Summary population: the universe, narrowed by the opt-in triggered-only
	// toggle (so the Cop Analysis view's cards reflect what actually triggered)
	// but independent of the classification filter (so it always shows the bigger
	// picture across classifications).
	summaryPop := make([]copAggregateItem, 0, len(allItems))
	for _, it := range allItems {
		if triggeredOnly && it.TotalOffences == 0 {
			continue
		}
		summaryPop = append(summaryPop, it)
	}
	summary := computeCopSummary(summaryPop, accum)

	// Data page narrows the summary population further by classification.
	items := make([]copAggregateItem, 0, len(summaryPop))
	for _, it := range summaryPop {
		if classFilter != "" && it.Classification != classFilter {
			continue
		}
		items = append(items, it)
	}

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

// computeCopSummary computes the headline summary counts across the full cop
// universe (all known cops, unaffected by the data-page filters) so the user
// always sees the bigger picture. Cop counts come from every known cop; the
// affected-cookbook counts come from scan offences (accum) only.
func computeCopSummary(allItems []copAggregateItem, accum map[string]*copAccum) copAggregationSummary {
	var s copAggregationSummary

	blockerCBs := make(map[string]bool)
	reviewCBs := make(map[string]bool)

	for _, item := range allItems {
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
		case "cookbooks_excluded_only":
			less = items[i].CookbooksExcludedOnly < items[j].CookbooksExcludedOnly
		default: // "cookbooks_affected"
			less = items[i].CookbooksAffected < items[j].CookbooksAffected
		}
		if sp.Order == "desc" {
			return !less
		}
		return less
	})
}
