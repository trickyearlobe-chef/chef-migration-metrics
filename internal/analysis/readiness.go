// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// ---------------------------------------------------------------------------
// Cookbook compatibility status constants
// ---------------------------------------------------------------------------

const (
	// StatusCompatible means Test Kitchen passed for this cookbook × target.
	StatusCompatible = "compatible"

	// StatusCompatibleCookstyleOnly means CookStyle passed but no Test
	// Kitchen result exists. The cookbook has no detected errors but has
	// not been integration-tested.
	StatusCompatibleCookstyleOnly = "compatible_cookstyle_only"

	// StatusIncompatible means Test Kitchen failed or CookStyle reported
	// error/fatal offenses.
	StatusIncompatible = "incompatible"

	// StatusUntested means no test or scan results exist for this cookbook
	// × target version.
	StatusUntested = "untested"
)

// Compatibility source constants — record how the verdict was determined.
const (
	SourceTestKitchen = "test_kitchen"
	SourceCookstyle   = "cookstyle"
	SourceNone        = "none"
)

// Multi-source verdict constants — identify which specific source produced a verdict.
const (
	SourceServerCookstyle = "server_cookstyle"
	SourceGitCookstyle    = "git_cookstyle"
	SourceGitTestKitchen  = "git_test_kitchen"
)

// CookbookSourceVerdict records the compatibility result from one source.
type CookbookSourceVerdict struct {
	Source          string `json:"source"`               // "server_cookstyle", "git_cookstyle", "git_test_kitchen"
	Status          string `json:"status"`               // "compatible", "incompatible", "untested"
	Version         string `json:"version,omitempty"`    // server version or "HEAD" for git
	CommitSHA       string `json:"commit_sha,omitempty"` // git HEAD SHA (git sources only)
	ComplexityScore int    `json:"complexity_score,omitempty"`
	ComplexityLabel string `json:"complexity_label,omitempty"`
}

// ---------------------------------------------------------------------------
// BlockingCookbook — one entry in the blocking_cookbooks JSONB array
// ---------------------------------------------------------------------------

// BlockingCookbook describes a single cookbook that is preventing a node from
// being ready for upgrade.
type BlockingCookbook struct {
	Name            string                  `json:"name"`
	Version         string                  `json:"version"`
	Reason          string                  `json:"reason"`           // StatusIncompatible or StatusUntested
	Source          string                  `json:"source"`           // SourceTestKitchen, SourceCookstyle, or SourceNone
	ComplexityScore int                     `json:"complexity_score"` // 0 if no complexity data
	ComplexityLabel string                  `json:"complexity_label"` // "" if no complexity data
	Verdicts        []CookbookSourceVerdict `json:"verdicts,omitempty"`
}

// ---------------------------------------------------------------------------
// ReadinessResult — the output for one node × target version
// ---------------------------------------------------------------------------

// ReadinessResult holds the evaluation outcome for a single node × target
// Chef Client version pair.
type ReadinessResult struct {
	NodeSnapshotID         string
	OrganisationID         string
	NodeName               string
	TargetChefVersion      string
	IsReady                bool
	AllCookbooksCompatible bool
	SufficientDiskSpace    *bool // nil = unknown
	BlockingCookbooks      []BlockingCookbook
	AvailableDiskMB        *int // nil = unknown
	RequiredDiskMB         int
	StaleData              bool
	EvaluatedAt            time.Time
}

// ---------------------------------------------------------------------------
// ReadinessDataStore — interface for testability
// ---------------------------------------------------------------------------

// ReadinessDataStore is the subset of datastore.DB methods needed by the
// readiness evaluator. Accepting an interface allows tests to inject fakes
// without a live PostgreSQL database.
type ReadinessDataStore interface {
	// Node snapshots
	ListNodeSnapshotsByOrganisation(ctx context.Context, organisationID string) ([]datastore.NodeSnapshot, error)

	// Server cookbooks — used to resolve cookbook name+version to server cookbook ID
	GetServerCookbookIDMap(ctx context.Context, organisationID string) (map[string]map[string]string, error)

	// Git repos — used to resolve cookbook name to git repo for TK cross-lookup
	GetGitRepoByName(ctx context.Context, name string) (datastore.GitRepo, error)

	// Test Kitchen results (git repo)
	GetLatestGitRepoTestKitchenResult(ctx context.Context, gitRepoID, targetChefVersion string) (*datastore.GitRepoTestKitchenResult, error)

	// CookStyle results (server cookbook)
	GetServerCookbookCookstyleResult(ctx context.Context, serverCookbookID, targetChefVersion string) (*datastore.ServerCookbookCookstyleResult, error)

	// CookStyle results (git repo)
	GetGitRepoCookstyleResult(ctx context.Context, gitRepoID, targetChefVersion string) (*datastore.GitRepoCookstyleResult, error)

	// Server cookbook complexity
	GetServerCookbookComplexity(ctx context.Context, serverCookbookID, targetChefVersion string) (*datastore.ServerCookbookComplexity, error)

	// Git repo complexity
	GetGitRepoComplexity(ctx context.Context, gitRepoID, targetChefVersion string) (*datastore.GitRepoComplexity, error)

	// Persistence
	UpsertNodeReadiness(ctx context.Context, p datastore.UpsertNodeReadinessParams) (*datastore.NodeReadiness, error)
}

// ---------------------------------------------------------------------------
// ReadinessEvaluator
// ---------------------------------------------------------------------------

// ReadinessEvaluator computes per-node per-target-version upgrade readiness.
type ReadinessEvaluator struct {
	db            ReadinessDataStore
	logger        *logging.Logger
	concurrency   int
	minFreeDiskMB int
}

// ReadinessEvaluatorOption configures a ReadinessEvaluator.
type ReadinessEvaluatorOption func(*ReadinessEvaluator)

// WithReadinessDataStore overrides the datastore (for testing).
func WithReadinessDataStore(ds ReadinessDataStore) ReadinessEvaluatorOption {
	return func(e *ReadinessEvaluator) { e.db = ds }
}

// NewReadinessEvaluator creates an evaluator.
//
// Parameters:
//   - db: datastore for reading test results and persisting readiness
//   - logger: structured logger (may be nil for silent operation)
//   - concurrency: max parallel node evaluations (worker pool size)
//   - minFreeDiskMB: minimum free disk in MB required for Habitat bundle
//   - opts: optional overrides
func NewReadinessEvaluator(
	db ReadinessDataStore,
	logger *logging.Logger,
	concurrency int,
	minFreeDiskMB int,
	opts ...ReadinessEvaluatorOption,
) *ReadinessEvaluator {
	if concurrency <= 0 {
		concurrency = 1
	}
	if minFreeDiskMB <= 0 {
		minFreeDiskMB = 2048
	}

	e := &ReadinessEvaluator{
		db:            db,
		logger:        logger,
		concurrency:   concurrency,
		minFreeDiskMB: minFreeDiskMB,
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// ---------------------------------------------------------------------------
// Batch evaluation
// ---------------------------------------------------------------------------

// workItem pairs a node snapshot with a target Chef version for fan-out.
type workItem struct {
	snapshot          datastore.NodeSnapshot
	targetChefVersion string
}

// EvaluateOrganisation evaluates readiness for all nodes in the given
// organisation across all specified target Chef Client versions.
//
// The method:
//  1. Loads the latest node snapshots for the organisation
//  2. Pre-loads the server cookbook ID map for efficient lookups
//  3. Fans out work items (node × target version) across a bounded worker pool
//  4. Persists each result to the node_readiness table
//
// Returns the collected results and any error that prevented evaluation from
// starting. Individual node evaluation errors are logged but do not abort the
// batch.
func (e *ReadinessEvaluator) EvaluateOrganisation(
	ctx context.Context,
	organisationID string,
	orgName string,
	targetChefVersions []string,
) ([]ReadinessResult, error) {
	if len(targetChefVersions) == 0 {
		return nil, nil
	}

	// Step 1: Load latest node snapshots for the organisation.
	snapshots, err := e.db.ListNodeSnapshotsByOrganisation(ctx, organisationID)
	if err != nil {
		return nil, fmt.Errorf("readiness: listing node snapshots: %w", err)
	}
	if len(snapshots) == 0 {
		e.logInfo(orgName, "no node snapshots found — skipping readiness evaluation")
		return nil, nil
	}

	// Step 2: Pre-load the cookbook ID map.
	cookbookIDMap, err := e.db.GetServerCookbookIDMap(ctx, organisationID)
	if err != nil {
		return nil, fmt.Errorf("readiness: loading cookbook ID map: %w", err)
	}

	// Step 3: Build work items.
	items := make([]workItem, 0, len(snapshots)*len(targetChefVersions))
	for _, snap := range snapshots {
		for _, tv := range targetChefVersions {
			items = append(items, workItem{
				snapshot:          snap,
				targetChefVersion: tv,
			})
		}
	}

	e.logInfo(orgName, fmt.Sprintf("evaluating %d nodes × %d target versions = %d work items",
		len(snapshots), len(targetChefVersions), len(items)))

	// Step 4: Fan out.
	sem := make(chan struct{}, e.concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make([]ReadinessResult, 0, len(items))

	for _, item := range items {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{} // acquire
		go func(wi workItem) {
			defer wg.Done()
			defer func() { <-sem }() // release

			result := e.evaluateOne(ctx, wi.snapshot, wi.targetChefVersion, cookbookIDMap)

			// Persist.
			if persistErr := e.persistResult(ctx, result); persistErr != nil {
				e.logError(orgName,
					fmt.Sprintf("failed to persist readiness for node %s target %s: %v",
						wi.snapshot.NodeName, wi.targetChefVersion, persistErr))
			}

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(item)
	}

	wg.Wait()

	e.logInfo(orgName, fmt.Sprintf("readiness evaluation complete: %d results", len(results)))
	return results, nil
}

// ---------------------------------------------------------------------------
// Single node evaluation
// ---------------------------------------------------------------------------

// evaluateOne computes readiness for one node × target version.
func (e *ReadinessEvaluator) evaluateOne(
	ctx context.Context,
	snapshot datastore.NodeSnapshot,
	targetChefVersion string,
	cookbookIDMap map[string]map[string]string,
) ReadinessResult {
	now := time.Now().UTC()

	result := ReadinessResult{
		NodeSnapshotID:    snapshot.ID,
		OrganisationID:    snapshot.OrganisationID,
		NodeName:          snapshot.NodeName,
		TargetChefVersion: targetChefVersion,
		StaleData:         snapshot.IsStale,
		RequiredDiskMB:    e.minFreeDiskMB,
		EvaluatedAt:       now,
	}

	// --- Cookbook compatibility ---
	blockingCookbooks := e.evaluateCookbooks(ctx, snapshot, targetChefVersion, cookbookIDMap)
	result.BlockingCookbooks = blockingCookbooks
	result.AllCookbooksCompatible = len(blockingCookbooks) == 0

	// --- Disk space ---
	if snapshot.IsStale {
		// Stale nodes: disk space treated as unknown.
		result.SufficientDiskSpace = nil
		result.AvailableDiskMB = nil
	} else {
		availMB, known := e.evaluateDiskSpace(snapshot)
		if known {
			result.AvailableDiskMB = &availMB
			sufficient := availMB >= e.minFreeDiskMB
			result.SufficientDiskSpace = &sufficient
		}
		// If not known: SufficientDiskSpace and AvailableDiskMB remain nil.
	}

	// --- Overall readiness ---
	// Ready only if ALL cookbooks compatible AND disk space is sufficient.
	// Unknown disk space blocks readiness (erring on the side of caution).
	diskOK := result.SufficientDiskSpace != nil && *result.SufficientDiskSpace
	result.IsReady = result.AllCookbooksCompatible && diskOK

	return result
}

// ---------------------------------------------------------------------------
// Cookbook compatibility evaluation
// ---------------------------------------------------------------------------

// nodeCookbookEntry represents one entry from automatic.cookbooks JSON.
// The JSON format is: {"name": {"version": "1.2.3", ...}, ...}
// We only need the version field.
type nodeCookbookEntry struct {
	Version string `json:"version"`
}

// evaluateCookbooks checks all cookbooks on the node against the target
// Chef Client version. Returns the list of blocking cookbooks.
func (e *ReadinessEvaluator) evaluateCookbooks(
	ctx context.Context,
	snapshot datastore.NodeSnapshot,
	targetChefVersion string,
	cookbookIDMap map[string]map[string]string,
) []BlockingCookbook {
	if len(snapshot.Cookbooks) == 0 {
		return nil
	}

	// Parse the automatic.cookbooks attribute.
	cookbooks := parseCookbooksAttribute(snapshot.Cookbooks)
	if len(cookbooks) == 0 {
		return nil
	}

	var blocking []BlockingCookbook

	for cbName, cbVersion := range cookbooks {
		status, source, verdicts := e.checkCookbookCompatibility(ctx, cbName, cbVersion, targetChefVersion, cookbookIDMap)

		switch status {
		case StatusCompatible, StatusCompatibleCookstyleOnly:
			// Not blocking.
			continue
		case StatusIncompatible, StatusUntested:
			bc := BlockingCookbook{
				Name:     cbName,
				Version:  cbVersion,
				Reason:   status,
				Source:   source,
				Verdicts: verdicts,
			}

			// Try to enrich with server cookbook complexity data.
			cookbookID := lookupCookbookID(cookbookIDMap, cbName, cbVersion)
			if cookbookID != "" {
				if cc, err := e.db.GetServerCookbookComplexity(ctx, cookbookID, targetChefVersion); err == nil && cc != nil {
					bc.ComplexityScore = cc.ComplexityScore
					bc.ComplexityLabel = cc.ComplexityLabel
				}
			}

			// Enrich verdicts with complexity data.
			for i := range bc.Verdicts {
				switch bc.Verdicts[i].Source {
				case SourceServerCookstyle:
					if cookbookID != "" {
						if cc, err := e.db.GetServerCookbookComplexity(ctx, cookbookID, targetChefVersion); err == nil && cc != nil {
							bc.Verdicts[i].ComplexityScore = cc.ComplexityScore
							bc.Verdicts[i].ComplexityLabel = cc.ComplexityLabel
						}
					}
				case SourceGitCookstyle, SourceGitTestKitchen:
					gitRepo, grErr := e.db.GetGitRepoByName(ctx, cbName)
					if grErr == nil && gitRepo.ID != "" {
						if gc, gcErr := e.db.GetGitRepoComplexity(ctx, gitRepo.ID, targetChefVersion); gcErr == nil && gc != nil {
							bc.Verdicts[i].ComplexityScore = gc.ComplexityScore
							bc.Verdicts[i].ComplexityLabel = gc.ComplexityLabel
						}
					}
				}
			}

			blocking = append(blocking, bc)
		}
	}

	return blocking
}

// parseCookbooksAttribute parses the automatic.cookbooks JSONB into a map
// of cookbook_name → version. It handles both the standard Ohai format
// (map of name → object with "version" key) and simpler formats.
func parseCookbooksAttribute(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}

	// Standard format: {"cb_name": {"version": "1.2.3", ...}, ...}
	var structured map[string]nodeCookbookEntry
	if err := json.Unmarshal(raw, &structured); err == nil && len(structured) > 0 {
		result := make(map[string]string, len(structured))
		for name, entry := range structured {
			if entry.Version != "" {
				result[name] = entry.Version
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	// Fallback: {"cb_name": "version_string", ...}
	var simple map[string]string
	if err := json.Unmarshal(raw, &simple); err == nil && len(simple) > 0 {
		return simple
	}

	return nil
}

// checkCookbookCompatibility determines the compatibility status of a single
// cookbook × version against the target Chef Client version using multi-source
// evaluation.
//
// Algorithm:
//  1. Check Git repo Test Kitchen result → verdict
//  2. Check Git repo CookStyle result → verdict
//  3. Check Server cookbook CookStyle result → verdict
//  4. Aggregate: any compatible → compatible; all tested incompatible →
//     incompatible; no results → untested
//
// Returns the overall status, primary source (for backward compat), and
// per-source verdicts.
func (e *ReadinessEvaluator) checkCookbookCompatibility(
	ctx context.Context,
	cookbookName string,
	cookbookVersion string,
	targetChefVersion string,
	cookbookIDMap map[string]map[string]string,
) (status, source string, verdicts []CookbookSourceVerdict) {
	cookbookID := lookupCookbookID(cookbookIDMap, cookbookName, cookbookVersion)

	var anyCompatible bool
	var anyTested bool

	// --- Source 1: Git repo Test Kitchen ---
	gitRepo, grErr := e.db.GetGitRepoByName(ctx, cookbookName)
	if grErr == nil && gitRepo.ID != "" {
		tkResult, tkErr := e.db.GetLatestGitRepoTestKitchenResult(ctx, gitRepo.ID, targetChefVersion)
		if tkErr == nil && tkResult != nil {
			anyTested = true
			v := CookbookSourceVerdict{
				Source:    SourceGitTestKitchen,
				Version:   "HEAD",
				CommitSHA: gitRepo.HeadCommitSHA,
			}
			if tkResult.Compatible {
				v.Status = StatusCompatible
				anyCompatible = true
			} else {
				v.Status = StatusIncompatible
			}
			verdicts = append(verdicts, v)
		}

		// --- Source 2: Git repo CookStyle ---
		gitCSResult, gitCSErr := e.db.GetGitRepoCookstyleResult(ctx, gitRepo.ID, targetChefVersion)
		if gitCSErr == nil && gitCSResult != nil {
			anyTested = true
			v := CookbookSourceVerdict{
				Source:    SourceGitCookstyle,
				Version:   "HEAD",
				CommitSHA: gitRepo.HeadCommitSHA,
			}
			if gitCSResult.Passed {
				v.Status = StatusCompatible
				anyCompatible = true
			} else {
				v.Status = StatusIncompatible
			}
			verdicts = append(verdicts, v)
		}
	}

	// --- Source 3: Server cookbook CookStyle ---
	if cookbookID != "" {
		csResult, err := e.db.GetServerCookbookCookstyleResult(ctx, cookbookID, targetChefVersion)
		if err == nil && csResult != nil {
			anyTested = true
			v := CookbookSourceVerdict{
				Source:  SourceServerCookstyle,
				Version: cookbookVersion,
			}
			if csResult.Passed {
				v.Status = StatusCompatible
				anyCompatible = true
			} else {
				v.Status = StatusIncompatible
			}
			verdicts = append(verdicts, v)
		} else {
			// Also check CookStyle without a target version — server-sourced
			// cookbooks may have been scanned without a target version profile.
			csResult, err = e.db.GetServerCookbookCookstyleResult(ctx, cookbookID, "")
			if err == nil && csResult != nil {
				anyTested = true
				v := CookbookSourceVerdict{
					Source:  SourceServerCookstyle,
					Version: cookbookVersion,
				}
				if csResult.Passed {
					v.Status = StatusCompatible
					anyCompatible = true
				} else {
					v.Status = StatusIncompatible
				}
				verdicts = append(verdicts, v)
			}
		}
	}

	// --- Determine overall status ---
	if anyCompatible {
		// Determine primary source for backward compat
		primarySource := SourceNone
		for _, v := range verdicts {
			if v.Status == StatusCompatible || v.Status == StatusCompatibleCookstyleOnly {
				if v.Source == SourceGitTestKitchen {
					primarySource = SourceTestKitchen
					break // TK is highest confidence
				}
				if primarySource == SourceNone {
					primarySource = SourceCookstyle
				}
			}
		}
		// If TK is the compatible source, return StatusCompatible
		// If only cookstyle sources are compatible, return StatusCompatibleCookstyleOnly
		for _, v := range verdicts {
			if v.Status == StatusCompatible && v.Source == SourceGitTestKitchen {
				return StatusCompatible, primarySource, verdicts
			}
		}
		return StatusCompatibleCookstyleOnly, primarySource, verdicts
	}

	if anyTested {
		// All tested sources say incompatible
		primarySource := SourceNone
		for _, v := range verdicts {
			if v.Source == SourceGitTestKitchen {
				primarySource = SourceTestKitchen
				break
			}
			if primarySource == SourceNone {
				primarySource = SourceCookstyle
			}
		}
		return StatusIncompatible, primarySource, verdicts
	}

	// No results at all
	return StatusUntested, SourceNone, verdicts
}

// lookupCookbookID resolves a cookbook name + version to its database ID
// using the pre-loaded map.
func lookupCookbookID(idMap map[string]map[string]string, name, version string) string {
	if idMap == nil {
		return ""
	}
	versions, ok := idMap[name]
	if !ok {
		return ""
	}
	return versions[version]
}

// ---------------------------------------------------------------------------
// Disk space evaluation
// ---------------------------------------------------------------------------

// filesystemEntry represents one entry in the automatic.filesystem JSON.
// Values may be strings or integers depending on the Chef Client version.
type filesystemEntry struct {
	KBSize      interface{} `json:"kb_size"`
	KBUsed      interface{} `json:"kb_used"`
	KBAvailable interface{} `json:"kb_available"`
	PercentUsed interface{} `json:"percent_used"`
	Mount       interface{} `json:"mount"`
}

// evaluateDiskSpace determines the available disk space on the installation
// target mount point and returns it in MB along with whether the data is
// known.
func (e *ReadinessEvaluator) evaluateDiskSpace(snapshot datastore.NodeSnapshot) (availableMB int, known bool) {
	if len(snapshot.Filesystem) == 0 {
		return 0, false
	}

	fsMap := parseFilesystemAttribute(snapshot.Filesystem)
	if len(fsMap) == 0 {
		return 0, false
	}

	// Determine the installation target path based on platform.
	installPath := determineInstallPath(snapshot.Platform)

	// Find the filesystem entry whose mount is the longest prefix match.
	matchedMount, entry := findBestMount(fsMap, installPath, snapshot.Platform)
	if matchedMount == "" && entry == nil {
		return 0, false
	}

	// Extract kb_available.
	kbAvail := toInt64(entry.KBAvailable)
	if kbAvail < 0 {
		// kb_available missing or unparseable — treat as 0.
		kbAvail = 0
	}

	// Convert KB to MB.
	availableMB = int(kbAvail / 1024)
	return availableMB, true
}

// parseFilesystemAttribute parses the automatic.filesystem JSONB into a map
// of device/mount-name → filesystemEntry.
func parseFilesystemAttribute(raw json.RawMessage) map[string]filesystemEntry {
	if len(raw) == 0 {
		return nil
	}

	var fsMap map[string]filesystemEntry
	if err := json.Unmarshal(raw, &fsMap); err != nil {
		return nil
	}
	return fsMap
}

// determineInstallPath returns the installation target path for the Habitat
// bundle based on the platform.
func determineInstallPath(platform string) string {
	p := strings.ToLower(platform)
	if p == "windows" {
		return `C:\hab`
	}
	return "/hab"
}

// findBestMount finds the filesystem entry whose mount is the longest prefix
// match for the given installation path. Returns the mount path and entry.
//
// For Windows, we match on the drive letter (e.g. "C:").
// For Linux, we do longest prefix match on the mount path.
func findBestMount(
	fsMap map[string]filesystemEntry,
	installPath string,
	platform string,
) (string, *filesystemEntry) {
	isWindows := strings.ToLower(platform) == "windows"

	if isWindows {
		return findBestMountWindows(fsMap, installPath)
	}
	return findBestMountLinux(fsMap, installPath)
}

// findBestMountLinux finds the filesystem entry whose mount field is the
// longest prefix of the install path.
func findBestMountLinux(
	fsMap map[string]filesystemEntry,
	installPath string,
) (string, *filesystemEntry) {
	var bestMount string
	var bestEntry *filesystemEntry
	bestLen := -1

	for key, entry := range fsMap {
		mount := toString(entry.Mount)
		if mount == "" {
			// Some entries might use the key as the device name (e.g. "/dev/sda1")
			// but have no mount field — skip those.
			continue
		}

		// Check if the mount is a prefix of the install path.
		if isPathPrefix(mount, installPath) {
			if len(mount) > bestLen {
				bestLen = len(mount)
				bestMount = key
				e := entry // copy
				bestEntry = &e
			}
		}
	}

	return bestMount, bestEntry
}

// findBestMountWindows finds the filesystem entry matching the drive letter
// of the install path.
func findBestMountWindows(
	fsMap map[string]filesystemEntry,
	installPath string,
) (string, *filesystemEntry) {
	// Extract drive letter from installPath (e.g. "C" from "C:\hab").
	targetDrive := ""
	if len(installPath) >= 2 && installPath[1] == ':' {
		targetDrive = strings.ToUpper(installPath[:1])
	}
	if targetDrive == "" {
		// Can't determine drive letter — try C: as default.
		targetDrive = "C"
	}

	// First try to find exact drive match by key (e.g. "C:" or "C:\").
	for key, entry := range fsMap {
		keyUpper := strings.ToUpper(strings.TrimRight(key, `\/`))
		if keyUpper == targetDrive+":" {
			e := entry
			return key, &e
		}
	}

	// Fallback: check mount fields.
	for key, entry := range fsMap {
		mount := toString(entry.Mount)
		mountUpper := strings.ToUpper(strings.TrimRight(mount, `\/`))
		if mountUpper == targetDrive+":" {
			e := entry
			return key, &e
		}
	}

	return "", nil
}

// isPathPrefix returns true if prefix is a filesystem path prefix of path.
// This handles the subtlety that "/opt" is a prefix of "/opt/hab" but NOT
// of "/optional".
func isPathPrefix(prefix, path string) bool {
	if prefix == "/" {
		return true // root is always a prefix
	}
	if prefix == path {
		return true
	}
	// prefix must end at a path separator boundary.
	if strings.HasPrefix(path, prefix) {
		if len(path) > len(prefix) && path[len(prefix)] == '/' {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Value conversion helpers
// ---------------------------------------------------------------------------

// toInt64 converts an interface{} (string or numeric) to int64.
// Returns -1 if the value cannot be parsed.
func toInt64(v interface{}) int64 {
	if v == nil {
		return -1
	}
	switch val := v.(type) {
	case string:
		// Strip surrounding quotes and whitespace.
		s := strings.TrimSpace(val)
		if s == "" {
			return -1
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			// Try parsing as float (some systems report "12345.0").
			f, fErr := strconv.ParseFloat(s, 64)
			if fErr != nil {
				return -1
			}
			return int64(math.Floor(f))
		}
		return n
	case float64:
		return int64(math.Floor(val))
	case float32:
		return int64(math.Floor(float64(val)))
	case int:
		return int64(val)
	case int64:
		return val
	case int32:
		return int64(val)
	case json.Number:
		n, err := val.Int64()
		if err != nil {
			f, fErr := val.Float64()
			if fErr != nil {
				return -1
			}
			return int64(math.Floor(f))
		}
		return n
	default:
		return -1
	}
}

// toString converts an interface{} to a string. Returns "" for nil.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

// persistResult writes a ReadinessResult to the node_readiness table.
func (e *ReadinessEvaluator) persistResult(ctx context.Context, result ReadinessResult) error {
	var blockingJSON json.RawMessage
	if len(result.BlockingCookbooks) > 0 {
		b, err := json.Marshal(result.BlockingCookbooks)
		if err != nil {
			return fmt.Errorf("readiness: marshalling blocking cookbooks: %w", err)
		}
		blockingJSON = b
	}

	requiredDiskMB := result.RequiredDiskMB
	_, err := e.db.UpsertNodeReadiness(ctx, datastore.UpsertNodeReadinessParams{
		NodeSnapshotID:         result.NodeSnapshotID,
		OrganisationID:         result.OrganisationID,
		NodeName:               result.NodeName,
		TargetChefVersion:      result.TargetChefVersion,
		IsReady:                result.IsReady,
		AllCookbooksCompatible: result.AllCookbooksCompatible,
		SufficientDiskSpace:    result.SufficientDiskSpace,
		BlockingCookbooks:      blockingJSON,
		AvailableDiskMB:        result.AvailableDiskMB,
		RequiredDiskMB:         &requiredDiskMB,
		StaleData:              result.StaleData,
		EvaluatedAt:            result.EvaluatedAt,
	})
	return err
}

// ---------------------------------------------------------------------------
// Logging helpers
// ---------------------------------------------------------------------------

func (e *ReadinessEvaluator) logInfo(orgName, msg string) {
	if e.logger == nil {
		return
	}
	e.logger.Info(logging.ScopeReadinessEvaluation, msg, logging.WithOrganisation(orgName))
}

func (e *ReadinessEvaluator) logError(orgName, msg string) {
	if e.logger == nil {
		return
	}
	e.logger.Error(logging.ScopeReadinessEvaluation, msg, logging.WithOrganisation(orgName))
}
