// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// CoverageReport is the JSONB structure stored in cookbook_platform_coverage.
type CoverageReport struct {
	KitchenPlatforms     []string              `json:"kitchen_platforms"`
	ProductionPlatforms  []ProductionPlatform  `json:"production_platforms"`
	TestedAndInProd      []TestedPlatformMatch `json:"tested_and_in_production"`
	TestedNotInProd      []string              `json:"tested_not_in_production"`
	InProdNotTested      []ProductionPlatform  `json:"in_production_not_tested"`
	GapCount             int                   `json:"gap_count"`
	TotalProductionNodes int                   `json:"total_production_nodes"`
	CoveredNodeCount     int                   `json:"covered_node_count"`
	CoveragePercentage   float64               `json:"coverage_percentage"`
}

// ProductionPlatform represents a production platform tuple with node count.
type ProductionPlatform struct {
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platform_version"`
	PlatformFamily  string `json:"platform_family"`
	NodeCount       int    `json:"node_count"`
}

// TestedPlatformMatch records a match between a kitchen platform and production.
type TestedPlatformMatch struct {
	KitchenName     string `json:"kitchen_name"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platform_version"`
	NodeCount       int    `json:"node_count"`
}

// ProductionPlatformsFromRows converts datastore ProductionPlatformRow slices
// to the analysis domain type. Returns a non-nil empty slice when the input
// is nil or empty (so JSON marshalling produces [] not null).
func ProductionPlatformsFromRows(rows []datastore.ProductionPlatformRow) []ProductionPlatform {
	result := make([]ProductionPlatform, len(rows))
	for i, r := range rows {
		result[i] = ProductionPlatform{
			Platform:        r.Platform,
			PlatformVersion: r.PlatformVersion,
			PlatformFamily:  r.PlatformFamily,
			NodeCount:       r.NodeCount,
		}
	}
	return result
}

// ComputeAndUpsertCoverageForRepo computes platform coverage for a single
// git repo cookbook and upserts the result into the database. It:
//  1. Parses .kitchen.yml from the repo working directory
//  2. Queries production platforms from node_snapshots
//  3. Runs ComputeCoverage to match kitchen ↔ production platforms
//  4. Upserts the CoverageReport into cookbook_platform_coverage
//
// Returns nil if the repo has no .kitchen.yml (no coverage to compute).
// Errors are logged as warnings and returned to the caller.
func ComputeAndUpsertCoverageForRepo(
	ctx context.Context,
	db *datastore.DB,
	logger *logging.Logger,
	repo datastore.GitRepo,
	repoDir string,
) error {
	log := logger.WithScope(logging.ScopePlatformCoverage,
		logging.WithCookbook(repo.Name, ""))

	// Step 1: Find and parse .kitchen.yml.
	kitchenPath := filepath.Join(repoDir, ".kitchen.yml")
	if _, err := os.Stat(kitchenPath); os.IsNotExist(err) {
		log.Debug(fmt.Sprintf("no .kitchen.yml in %s — skipping coverage", repo.Name))
		return nil
	}

	kitchenPlatforms := ParseKitchenYMLPlatforms(kitchenPath)
	if len(kitchenPlatforms) == 0 {
		log.Debug(fmt.Sprintf("no platforms in .kitchen.yml for %s — skipping coverage", repo.Name))
		return nil
	}

	// Step 2: Query production platforms from node_snapshots.
	rows, err := db.GetProductionPlatformsForCookbook(ctx, repo.Name)
	if err != nil {
		log.Warn(fmt.Sprintf("failed to query production platforms for %s: %v", repo.Name, err))
		return fmt.Errorf("querying production platforms for %s: %w", repo.Name, err)
	}
	prodPlatforms := ProductionPlatformsFromRows(rows)

	// Step 3: Compute coverage.
	report := ComputeCoverage(kitchenPlatforms, prodPlatforms)

	log.Info(fmt.Sprintf(
		"cookbook %s: %d kitchen platform(s), %d production platform(s), coverage %.1f%% (%d gap(s))",
		repo.Name, len(kitchenPlatforms), len(prodPlatforms),
		report.CoveragePercentage, report.GapCount))

	// Step 4: Upsert into database.
	_, err = db.UpsertCookbookPlatformCoverage(ctx, datastore.UpsertCookbookPlatformCoverageParams{
		GitRepoName:  repo.Name,
		GitRepoURL:   repo.GitRepoURL,
		CookbookName: repo.Name,
		CoverageData: report,
	})
	if err != nil {
		log.Warn(fmt.Sprintf("failed to upsert coverage for %s: %v", repo.Name, err))
		return fmt.Errorf("upserting coverage for %s: %w", repo.Name, err)
	}

	return nil
}

// ComputeAllGitRepoCoverage runs coverage analysis for all provided git
// repos. It is intended to be called after the collection + analysis cycle
// completes. Errors for individual repos are logged and do not abort the
// batch. Returns the count of repos that were evaluated and the count that
// had errors.
func ComputeAllGitRepoCoverage(
	ctx context.Context,
	db *datastore.DB,
	logger *logging.Logger,
	repos []datastore.GitRepo,
	repoDirFn func(datastore.GitRepo) string,
) (evaluated int, errCount int) {
	for _, repo := range repos {
		if ctx.Err() != nil {
			break
		}

		dir := repoDirFn(repo)
		if dir == "" {
			continue
		}

		evaluated++
		if err := ComputeAndUpsertCoverageForRepo(ctx, db, logger, repo, dir); err != nil {
			errCount++
		}
	}
	return evaluated, errCount
}

// ParseKitchenPlatformName splits a kitchen platform name on the LAST hyphen
// into (os, version). Returns ("", "", false) if the name cannot be parsed
// (no hyphen found, or hyphen is at the start or end).
func ParseKitchenPlatformName(name string) (os, version string, ok bool) {
	idx := strings.LastIndex(name, "-")
	if idx <= 0 || idx >= len(name)-1 {
		return "", "", false
	}
	return name[:idx], name[idx+1:], true
}

// ComputeCoverage computes the platform coverage report for a cookbook.
// kitchenPlatforms are the platform names from .kitchen.yml.
// productionPlatforms are the aggregated production platform tuples.
func ComputeCoverage(kitchenPlatforms []string, productionPlatforms []ProductionPlatform) CoverageReport {
	report := CoverageReport{
		KitchenPlatforms:    kitchenPlatforms,
		ProductionPlatforms: productionPlatforms,
	}

	if len(kitchenPlatforms) == 0 && len(productionPlatforms) == 0 {
		// Ensure nil slices become empty arrays in JSON.
		ensureNonNilSlices(&report)
		return report
	}

	// Track which production platforms are covered.
	covered := make([]bool, len(productionPlatforms))
	// Track which kitchen platforms matched something.
	kitchenMatched := make([]bool, len(kitchenPlatforms))

	for ki, kName := range kitchenPlatforms {
		parsedOS, version, ok := ParseKitchenPlatformName(kName)
		if !ok {
			// Unparseable — cannot match anything.
			continue
		}
		parsedOS = strings.ToLower(parsedOS)

		for pi, pp := range productionPlatforms {
			if covered[pi] {
				continue // already matched by another kitchen platform
			}

			ppPlatform := strings.ToLower(pp.Platform)
			ppFamily := strings.ToLower(pp.PlatformFamily)

			matched := false

			// Rule 1: Exact match (os == platform AND version == platform_version)
			if parsedOS == ppPlatform && version == pp.PlatformVersion {
				matched = true
			}

			// Rule 2: Major version match (os == platform AND major version matches)
			if !matched && parsedOS == ppPlatform {
				if majorVersionMatch(version, pp.PlatformVersion) {
					matched = true
				}
			}

			// Rule 3: Family grouping (os matches platform_family)
			if !matched && parsedOS == ppFamily {
				matched = true
			}

			if matched {
				covered[pi] = true
				kitchenMatched[ki] = true
				report.TestedAndInProd = append(report.TestedAndInProd, TestedPlatformMatch{
					KitchenName:     kName,
					Platform:        pp.Platform,
					PlatformVersion: pp.PlatformVersion,
					NodeCount:       pp.NodeCount,
				})
				report.CoveredNodeCount += pp.NodeCount
			}
		}
	}

	// Tested but not in production.
	for ki, kName := range kitchenPlatforms {
		if !kitchenMatched[ki] {
			report.TestedNotInProd = append(report.TestedNotInProd, kName)
		}
	}

	// In production but not tested (gaps).
	for pi, pp := range productionPlatforms {
		if !covered[pi] {
			report.InProdNotTested = append(report.InProdNotTested, pp)
		}
	}

	report.GapCount = len(report.InProdNotTested)

	// Totals.
	for _, pp := range productionPlatforms {
		report.TotalProductionNodes += pp.NodeCount
	}

	if report.TotalProductionNodes > 0 {
		report.CoveragePercentage = float64(report.CoveredNodeCount) / float64(report.TotalProductionNodes) * 100.0
		// Round to 1 decimal.
		report.CoveragePercentage = math.Round(report.CoveragePercentage*10) / 10
	}

	// Ensure nil slices become empty arrays in JSON.
	ensureNonNilSlices(&report)

	return report
}

// ensureNonNilSlices replaces nil slices with empty slices so JSON
// marshalling produces [] instead of null.
func ensureNonNilSlices(r *CoverageReport) {
	if r.TestedAndInProd == nil {
		r.TestedAndInProd = []TestedPlatformMatch{}
	}
	if r.TestedNotInProd == nil {
		r.TestedNotInProd = []string{}
	}
	if r.InProdNotTested == nil {
		r.InProdNotTested = []ProductionPlatform{}
	}
	if r.KitchenPlatforms == nil {
		r.KitchenPlatforms = []string{}
	}
	if r.ProductionPlatforms == nil {
		r.ProductionPlatforms = []ProductionPlatform{}
	}
}

// majorVersionMatch returns true if the kitchen version matches the
// production version using prefix semantics. For example:
// "7" matches "7.9.2009", "22.04" matches "22.04.1", "9" matches "9.3".
// Dotted kitchen versions like "22.04" do NOT match "22.10".
func majorVersionMatch(kitchenVersion, productionVersion string) bool {
	if kitchenVersion == "" || productionVersion == "" {
		return false
	}
	if strings.Contains(kitchenVersion, ".") {
		// Dotted kitchen version — use as a prefix match.
		// "22.04" matches "22.04" and "22.04.1" but not "22.045".
		if productionVersion == kitchenVersion {
			return true
		}
		return strings.HasPrefix(productionVersion, kitchenVersion) &&
			productionVersion[len(kitchenVersion)] == '.'
	}
	// Simple major version — extract first component from production.
	prodMajor := productionVersion
	if idx := strings.Index(productionVersion, "."); idx > 0 {
		prodMajor = productionVersion[:idx]
	}
	return kitchenVersion == prodMajor
}

// ParseKitchenYMLPlatforms reads a .kitchen.yml file and extracts the
// platform names. Returns nil if the file cannot be read or has no
// platforms section.
//
// This is a best-effort line-scanner (no full YAML parser dependency).
// It looks for the top-level `platforms:` key and extracts `- name: <value>`
// entries.
func ParseKitchenYMLPlatforms(kitchenYMLPath string) []string {
	data, err := os.ReadFile(kitchenYMLPath)
	if err != nil {
		return nil
	}

	var platforms []string
	lines := strings.Split(string(data), "\n")
	inPlatforms := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Top-level key detection (no leading whitespace).
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if strings.HasPrefix(trimmed, "platforms:") {
				inPlatforms = true
				continue
			}
			if inPlatforms {
				inPlatforms = false
			}
			continue
		}

		if inPlatforms {
			// Look for `- name: <value>` pattern.
			if strings.HasPrefix(trimmed, "- name:") {
				val := strings.TrimPrefix(trimmed, "- name:")
				val = strings.TrimSpace(val)
				if idx := strings.Index(val, " #"); idx >= 0 {
					val = strings.TrimSpace(val[:idx])
				}
				val = strings.Trim(val, `"'`)
				if val != "" {
					platforms = append(platforms, val)
				}
			}
		}
	}

	return platforms
}
