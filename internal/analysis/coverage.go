// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"math"
	"os"
	"strings"
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

// majorVersionMatch returns true if the kitchen version's major component
// matches the prefix of the production version. For example:
// "7" matches "7.9.2009", "22.04" matches "22.04.1", "9" matches "9.3".
func majorVersionMatch(kitchenVersion, productionVersion string) bool {
	if kitchenVersion == "" || productionVersion == "" {
		return false
	}
	// Kitchen version major = everything before the first "."
	kitchenMajor := kitchenVersion
	if idx := strings.Index(kitchenVersion, "."); idx > 0 {
		kitchenMajor = kitchenVersion[:idx]
	}
	// Production version major = everything before the first "."
	prodMajor := productionVersion
	if idx := strings.Index(productionVersion, "."); idx > 0 {
		prodMajor = productionVersion[:idx]
	}
	return kitchenMajor == prodMajor
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
				val = strings.Trim(val, `"'`)
				if val != "" {
					platforms = append(platforms, val)
				}
			}
		}
	}

	return platforms
}
