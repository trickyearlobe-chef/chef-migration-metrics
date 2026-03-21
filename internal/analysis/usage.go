// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package analysis implements cookbook usage analysis for Chef Migration
// Metrics. It derives usage statistics from collected node data — determining
// which cookbooks are in active use, which versions are deployed, which nodes
// run them, and the platform/role/policy breakdown for each cookbook version.
//
// The analysis runs after each node collection cycle completes and proceeds
// in three phases:
//
//  1. Per-node extraction (parallel) — fan out across collected nodes to
//     extract cookbook/version/node/platform/role/policy tuples.
//  2. Aggregation (single goroutine) — merge extracted tuples into per-
//     cookbook-version counts and sets.
//  3. Active/unused flagging — compare aggregated cookbook versions against
//     the full server inventory to flag unused versions.
//
// Results are persisted to the cookbook_usage_analysis and
// cookbook_usage_detail tables in a single transaction.
package analysis

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// NodeRecord holds the subset of node snapshot data needed for usage analysis.
// This is extracted from the datastore NodeSnapshot and decoded cookbook JSONB
// rather than depending on the full chefapi.NodeData type, so the analysis
// package can operate purely on persisted data.
type NodeRecord struct {
	NodeName        string
	Platform        string
	PlatformVersion string
	PlatformFamily  string
	Roles           []string
	PolicyName      string
	PolicyGroup     string
	// CookbookVersions maps cookbook name → version string for all cookbooks
	// this node is running (from the automatic.cookbooks attribute).
	CookbookVersions map[string]string
}

// CookbookInventoryEntry represents a single cookbook version known to the
// Chef server. Used in Phase 3 to determine unused versions.
type CookbookInventoryEntry struct {
	Name    string
	Version string
}

// extractedTuple is the per-node-per-cookbook tuple produced in Phase 1.
type extractedTuple struct {
	CookbookName    string
	CookbookVersion string
	NodeName        string
	Platform        string
	PlatformVersion string
	PlatformFamily  string
	Roles           []string
	PolicyName      string
	PolicyGroup     string
}

// cookbookVersionKey is the aggregation key used in Phase 2.
type cookbookVersionKey struct {
	Name    string
	Version string
}

// aggregatedUsage holds the accumulated statistics for a single cookbook
// version after Phase 2.
type aggregatedUsage struct {
	NodeCount    int
	seenNodes    map[string]bool // used for distinct counting only; not persisted
	Roles        map[string]bool
	PolicyNames  map[string]bool
	PolicyGroups map[string]bool
	// PlatformCounts: "platform/platform_version" → count
	PlatformCounts map[string]int
	// PlatformFamilyCounts: "platform_family" → count
	PlatformFamilyCounts map[string]int
}

// UsageResult is the final result of a usage analysis run. It is returned
// to the caller after persistence completes.
type UsageResult struct {
	AnalysisID      string
	OrganisationID  string
	CollectionRunID string
	TotalCookbooks  int
	ActiveCookbooks int
	UnusedCookbooks int
	TotalNodes      int
	DetailCount     int
	Duration        time.Duration
}

// ---------------------------------------------------------------------------
// Analyser
// ---------------------------------------------------------------------------

// Analyser runs cookbook usage analysis. It is safe for concurrent use.
type Analyser struct {
	db     *datastore.DB
	logger *logging.Logger
	// concurrency controls the number of goroutines used for Phase 1
	// per-node extraction. A value <= 0 defaults to 1.
	concurrency int
}

// New creates a new Analyser with the given dependencies.
func New(db *datastore.DB, logger *logging.Logger, concurrency int) *Analyser {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &Analyser{
		db:          db,
		logger:      logger,
		concurrency: concurrency,
	}
}

// RunUsageAnalysis executes the three-phase cookbook usage analysis for the
// given organisation and collection run. It takes the already-collected node
// records and the full cookbook inventory from the Chef server.
//
// The results are persisted to the database in a single transaction and
// returned as a UsageResult.
func (a *Analyser) RunUsageAnalysis(
	ctx context.Context,
	organisationID string,
	collectionRunID string,
	nodes []NodeRecord,
	inventory []CookbookInventoryEntry,
) (*UsageResult, error) {
	start := time.Now()
	log := a.logger.WithScope(logging.ScopeCollectionRun, logging.WithOrganisation(organisationID))

	log.Info(fmt.Sprintf("starting cookbook usage analysis for %d nodes, %d inventory entries",
		len(nodes), len(inventory)))

	// Phase 1: Per-node extraction (parallel).
	tuples := a.extractTuples(ctx, nodes)

	// Phase 2: Aggregation (single goroutine / synchronous).
	aggregated := aggregateTuples(tuples)

	// Phase 3: Active/unused flagging.
	inventorySet := buildInventorySet(inventory)
	activeSet := buildActiveSet(aggregated)

	activeCount := 0
	unusedCount := 0
	for key := range inventorySet {
		if activeSet[key] {
			activeCount++
		} else {
			unusedCount++
		}
	}

	totalCookbooks := len(inventorySet)
	totalNodes := len(nodes)
	now := time.Now().UTC()

	// Persistence: write header + details in a single transaction.
	analysisHeader := datastore.InsertCookbookUsageAnalysisParams{
		OrganisationID:  organisationID,
		CollectionRunID: collectionRunID,
		TotalCookbooks:  totalCookbooks,
		ActiveCookbooks: activeCount,
		UnusedCookbooks: unusedCount,
		TotalNodes:      totalNodes,
		AnalysedAt:      now,
	}

	var analysisID string
	var detailCount int

	err := a.db.Tx(ctx, func(tx *sql.Tx) error {
		// Delete any existing analysis (and its cascade-deleted details)
		// for this organisation before inserting the replacement. This
		// keeps the table at one row per org rather than accumulating
		// a new row per collection run.
		if _, delErr := tx.ExecContext(ctx,
			`DELETE FROM cookbook_usage_analysis WHERE organisation_id = $1`,
			organisationID,
		); delErr != nil {
			return fmt.Errorf("deleting old usage analysis: %w", delErr)
		}

		header, err := a.db.InsertCookbookUsageAnalysisTx(ctx, tx, analysisHeader)
		if err != nil {
			return fmt.Errorf("inserting analysis header: %w", err)
		}
		analysisID = header.ID

		// Build detail rows — one per cookbook version (both active and unused).
		detailParams := buildDetailParams(analysisID, organisationID, aggregated, inventorySet, activeSet)
		if len(detailParams) > 0 {
			inserted, err := a.db.BulkInsertCookbookUsageDetailsTx(ctx, tx, detailParams)
			if err != nil {
				return fmt.Errorf("inserting usage details: %w", err)
			}
			detailCount = inserted
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("analysis: persisting usage analysis: %w", err)
	}

	duration := time.Since(start)
	log.Info(fmt.Sprintf(
		"cookbook usage analysis complete in %s: %d total, %d active, %d unused, %d detail rows",
		duration.Round(time.Millisecond), totalCookbooks, activeCount, unusedCount, detailCount))

	return &UsageResult{
		AnalysisID:      analysisID,
		OrganisationID:  organisationID,
		CollectionRunID: collectionRunID,
		TotalCookbooks:  totalCookbooks,
		ActiveCookbooks: activeCount,
		UnusedCookbooks: unusedCount,
		TotalNodes:      totalNodes,
		DetailCount:     detailCount,
		Duration:        duration,
	}, nil
}

// ---------------------------------------------------------------------------
// Phase 1: Per-node extraction
// ---------------------------------------------------------------------------

// extractTuples fans out over nodes in parallel using the configured
// concurrency limit. Each goroutine sends extracted tuples to a shared
// channel. Returns all tuples once extraction is complete.
func (a *Analyser) extractTuples(ctx context.Context, nodes []NodeRecord) []extractedTuple {
	if len(nodes) == 0 {
		return nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	resultCh := make(chan []extractedTuple, len(nodes))
	sem := make(chan struct{}, a.concurrency)
	var wg sync.WaitGroup

	for i := range nodes {
		wg.Add(1)
		go func(node *NodeRecord) {
			defer wg.Done()

			// Acquire semaphore.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			tuples := extractNodeTuples(node)
			if len(tuples) > 0 {
				resultCh <- tuples
			}
		}(&nodes[i])
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var all []extractedTuple
	for batch := range resultCh {
		all = append(all, batch...)
	}
	return all
}

// extractNodeTuples extracts one tuple per cookbook version from a single node.
func extractNodeTuples(node *NodeRecord) []extractedTuple {
	if len(node.CookbookVersions) == 0 {
		return nil
	}

	tuples := make([]extractedTuple, 0, len(node.CookbookVersions))
	for cbName, cbVersion := range node.CookbookVersions {
		tuples = append(tuples, extractedTuple{
			CookbookName:    cbName,
			CookbookVersion: cbVersion,
			NodeName:        node.NodeName,
			Platform:        node.Platform,
			PlatformVersion: node.PlatformVersion,
			PlatformFamily:  node.PlatformFamily,
			Roles:           node.Roles,
			PolicyName:      node.PolicyName,
			PolicyGroup:     node.PolicyGroup,
		})
	}
	return tuples
}

// ---------------------------------------------------------------------------
// Phase 2: Aggregation
// ---------------------------------------------------------------------------

// aggregateTuples merges all extracted tuples into per-cookbook-version
// aggregated usage maps. This runs synchronously (single goroutine) to avoid
// concurrent map access.
func aggregateTuples(tuples []extractedTuple) map[cookbookVersionKey]*aggregatedUsage {
	agg := make(map[cookbookVersionKey]*aggregatedUsage)

	for i := range tuples {
		t := &tuples[i]
		key := cookbookVersionKey{Name: t.CookbookName, Version: t.CookbookVersion}

		usage, exists := agg[key]
		if !exists {
			usage = &aggregatedUsage{
				seenNodes:            make(map[string]bool),
				Roles:                make(map[string]bool),
				PolicyNames:          make(map[string]bool),
				PolicyGroups:         make(map[string]bool),
				PlatformCounts:       make(map[string]int),
				PlatformFamilyCounts: make(map[string]int),
			}
			agg[key] = usage
		}

		// Distinct node count.
		if !usage.seenNodes[t.NodeName] {
			usage.seenNodes[t.NodeName] = true
			usage.NodeCount++
		}

		// Roles from this node (a node may have multiple roles).
		for _, role := range t.Roles {
			usage.Roles[role] = true
		}

		// Policyfile references.
		if t.PolicyName != "" {
			usage.PolicyNames[t.PolicyName] = true
		}
		if t.PolicyGroup != "" {
			usage.PolicyGroups[t.PolicyGroup] = true
		}

		// Platform counts.
		if t.Platform != "" {
			platformKey := t.Platform
			if t.PlatformVersion != "" {
				platformKey += "/" + t.PlatformVersion
			}
			usage.PlatformCounts[platformKey]++
		}

		// Platform family counts.
		if t.PlatformFamily != "" {
			usage.PlatformFamilyCounts[t.PlatformFamily]++
		}
	}

	return agg
}

// ---------------------------------------------------------------------------
// Phase 3: Active/unused flagging
// ---------------------------------------------------------------------------

// buildInventorySet creates a set of all cookbook versions known to the Chef
// server.
func buildInventorySet(inventory []CookbookInventoryEntry) map[cookbookVersionKey]bool {
	set := make(map[cookbookVersionKey]bool, len(inventory))
	for _, entry := range inventory {
		set[cookbookVersionKey(entry)] = true
	}
	return set
}

// buildActiveSet creates a set of all cookbook versions that have at least
// one node using them.
func buildActiveSet(aggregated map[cookbookVersionKey]*aggregatedUsage) map[cookbookVersionKey]bool {
	set := make(map[cookbookVersionKey]bool, len(aggregated))
	for key, usage := range aggregated {
		if usage.NodeCount > 0 {
			set[key] = true
		}
	}
	return set
}

// ---------------------------------------------------------------------------
// Persistence helpers
// ---------------------------------------------------------------------------

// buildDetailParams builds the detail row parameters for all cookbook versions
// — both active (with aggregated stats) and unused (with zero counts).
func buildDetailParams(
	analysisID string,
	organisationID string,
	aggregated map[cookbookVersionKey]*aggregatedUsage,
	inventorySet map[cookbookVersionKey]bool,
	activeSet map[cookbookVersionKey]bool,
) []datastore.InsertCookbookUsageDetailParams {
	// Collect all cookbook versions from both the inventory and the
	// aggregated data (a node might run a cookbook not in the server
	// inventory — unlikely but possible with cached runs).
	allKeys := make(map[cookbookVersionKey]bool)
	for key := range inventorySet {
		allKeys[key] = true
	}
	for key := range aggregated {
		allKeys[key] = true
	}

	params := make([]datastore.InsertCookbookUsageDetailParams, 0, len(allKeys))

	// Sort keys for deterministic output.
	sortedKeys := make([]cookbookVersionKey, 0, len(allKeys))
	for key := range allKeys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Slice(sortedKeys, func(i, j int) bool {
		if sortedKeys[i].Name != sortedKeys[j].Name {
			return sortedKeys[i].Name < sortedKeys[j].Name
		}
		return sortedKeys[i].Version < sortedKeys[j].Version
	})

	for _, key := range sortedKeys {
		isActive := activeSet[key]
		usage := aggregated[key]

		p := datastore.InsertCookbookUsageDetailParams{
			AnalysisID:      analysisID,
			OrganisationID:  organisationID,
			CookbookName:    key.Name,
			CookbookVersion: key.Version,
			IsActive:        isActive,
		}

		if usage != nil {
			p.NodeCount = usage.NodeCount
			p.Roles = marshalSortedStringSet(usage.Roles)
			p.PolicyNames = marshalSortedStringSet(usage.PolicyNames)
			p.PolicyGroups = marshalSortedStringSet(usage.PolicyGroups)
			p.PlatformCounts = marshalStringIntMap(usage.PlatformCounts)
			p.PlatformFamilyCounts = marshalStringIntMap(usage.PlatformFamilyCounts)
		}

		params = append(params, p)
	}

	return params
}

// marshalSortedStringSet converts a map[string]bool set to a sorted JSON
// array of strings. Returns nil if the set is empty.
func marshalSortedStringSet(set map[string]bool) json.RawMessage {
	if len(set) == 0 {
		return nil
	}
	sorted := make([]string, 0, len(set))
	for k := range set {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	data, _ := json.Marshal(sorted)
	return data
}

// marshalStringIntMap converts a map[string]int to a JSON object. Returns
// nil if the map is empty.
func marshalStringIntMap(m map[string]int) json.RawMessage {
	if len(m) == 0 {
		return nil
	}
	data, _ := json.Marshal(m)
	return data
}

// ---------------------------------------------------------------------------
// NodeRecord builders (convenience functions for callers)
// ---------------------------------------------------------------------------

// NodeRecordFromSnapshot builds a NodeRecord from a datastore NodeSnapshot
// by decoding the JSONB fields. This is a convenience for callers that fetch
// snapshots from the database rather than directly from the collector.
func NodeRecordFromSnapshot(snap datastore.NodeSnapshot) (NodeRecord, error) {
	nr := NodeRecord{
		NodeName:        snap.NodeName,
		Platform:        snap.Platform,
		PlatformVersion: snap.PlatformVersion,
		PlatformFamily:  snap.PlatformFamily,
		PolicyName:      snap.PolicyName,
		PolicyGroup:     snap.PolicyGroup,
	}

	// Decode cookbooks JSONB → map[string]string (name → version).
	if len(snap.Cookbooks) > 0 {
		// The cookbooks JSONB can be in two formats:
		// 1. map[name]{"version": "x.y.z"} (Chef automatic.cookbooks format)
		// 2. map[name]"version" (simplified format used in some tests)
		var cbMap map[string]interface{}
		if err := json.Unmarshal(snap.Cookbooks, &cbMap); err != nil {
			return nr, fmt.Errorf("analysis: decoding cookbooks JSON: %w", err)
		}

		nr.CookbookVersions = make(map[string]string, len(cbMap))
		for name, val := range cbMap {
			switch v := val.(type) {
			case string:
				nr.CookbookVersions[name] = v
			case map[string]interface{}:
				if ver, ok := v["version"]; ok {
					if verStr, ok := ver.(string); ok {
						nr.CookbookVersions[name] = verStr
					}
				}
			}
		}
	}

	// Decode roles JSONB → []string.
	if len(snap.Roles) > 0 {
		if err := json.Unmarshal(snap.Roles, &nr.Roles); err != nil {
			return nr, fmt.Errorf("analysis: decoding roles JSON: %w", err)
		}
	}

	return nr, nil
}

// NodeRecordFromCollectedData builds a NodeRecord directly from in-memory
// collection data, avoiding the need to re-read from the database. This is
// the preferred path when the analysis is triggered immediately after
// collection within the same process.
func NodeRecordFromCollectedData(
	nodeName string,
	platform string,
	platformVersion string,
	platformFamily string,
	roles []string,
	policyName string,
	policyGroup string,
	cookbookVersions map[string]string,
) NodeRecord {
	return NodeRecord{
		NodeName:         nodeName,
		Platform:         platform,
		PlatformVersion:  platformVersion,
		PlatformFamily:   platformFamily,
		Roles:            roles,
		PolicyName:       policyName,
		PolicyGroup:      policyGroup,
		CookbookVersions: cookbookVersions,
	}
}
