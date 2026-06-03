// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Disk status constants.
const (
	DiskStatusSufficient   = "sufficient"
	DiskStatusInsufficient = "insufficient"
	DiskStatusUnknown      = "unknown"
)

// CookStyle status constants.
const (
	CookstyleStatusPassed  = "passed"
	CookstyleStatusFailed  = "failed"
	CookstyleStatusUnknown = "unknown"
)

// Test Kitchen status constants.
const (
	KitchenStatusPassed  = "passed"
	KitchenStatusFailed  = "failed"
	KitchenStatusPartial = "partial"
	KitchenStatusUnknown = "unknown"
)

// checkStatusResult holds the derived per-check status and detail string
// for a single readiness evaluation.
type checkStatusResult struct {
	DiskStatus      string  `json:"disk_status"`
	CookstyleStatus string  `json:"cookstyle_status"`
	KitchenStatus   string  `json:"kitchen_status"`
	DiskDetail      *string `json:"disk_detail"`
	CookstyleDetail *string `json:"cookstyle_detail"`
	KitchenDetail   *string `json:"kitchen_detail"`
}

// blockingEntry is a minimal struct for parsing the blocking_cookbooks JSON.
type blockingEntry struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	Reason          string `json:"reason"`
	Source          string `json:"source"`
	ComplexityScore int    `json:"complexity_score"`
	ComplexityLabel string `json:"complexity_label"`
	Verdicts        []struct {
		Source string `json:"source"`
		Status string `json:"status"`
	} `json:"verdicts"`
}

// deriveCheckStatus computes per-check status from a NodeReadiness record.
// When persisted status values are available (non-empty), they take precedence
// over re-derivation from blocking cookbooks. This ensures the node detail and
// list views agree with the analysis write-time computation.
func deriveCheckStatus(nr datastore.NodeReadiness, installPath string) checkStatusResult {
	result := checkStatusResult{
		DiskStatus: deriveDiskStatus(nr),
	}

	// Prefer persisted statuses (computed at analysis time with full context).
	// Fall back to re-derivation only for legacy data with empty strings.
	if nr.CookstyleStatus != "" {
		result.CookstyleStatus = nr.CookstyleStatus
	} else {
		result.CookstyleStatus = deriveCookstyleStatus(nr)
	}

	if nr.KitchenStatus != "" {
		result.KitchenStatus = nr.KitchenStatus
	} else {
		result.KitchenStatus = deriveKitchenStatus(nr)
	}

	result.DiskDetail = diskDetail(nr, installPath)
	result.CookstyleDetail = cookstyleDetail(nr, result.CookstyleStatus)
	result.KitchenDetail = kitchenDetail(nr, result.KitchenStatus)
	return result
}

// deriveDiskStatus returns the disk check status.
func deriveDiskStatus(nr datastore.NodeReadiness) string {
	if nr.SufficientDiskSpace == nil {
		return DiskStatusUnknown
	}
	if *nr.SufficientDiskSpace {
		return DiskStatusSufficient
	}
	return DiskStatusInsufficient
}

// deriveCookstyleStatus returns the CookStyle check status.
func deriveCookstyleStatus(nr datastore.NodeReadiness) string {
	if nr.StaleData {
		return CookstyleStatusUnknown
	}

	blocking := parseBlockingCookbooks(nr.BlockingCookbooks)

	if nr.AllCookbooksCompatible && len(blocking) == 0 {
		return CookstyleStatusPassed
	}

	csFailCount := 0
	hasCookstyleVerdict := false
	for _, b := range blocking {
		for _, v := range b.Verdicts {
			if isCookstyleSource(v.Source) {
				hasCookstyleVerdict = true
				if v.Status == "incompatible" {
					csFailCount++
					break // one incompatible verdict per cookbook is enough
				}
			}
		}
	}

	if csFailCount > 0 {
		return CookstyleStatusFailed
	}

	// If blocking entries exist but none have cookstyle verdicts at all,
	// we cannot determine cookstyle status.
	if len(blocking) > 0 && !hasCookstyleVerdict {
		// Check if all blocking entries only have TK failures — cookstyle
		// itself passed for these cookbooks.
		allTKOnly := true
		for _, b := range blocking {
			if !isOnlyTKFailure(b) {
				allTKOnly = false
				break
			}
		}
		if allTKOnly {
			return CookstyleStatusPassed
		}
		return CookstyleStatusUnknown
	}

	// Some blocking entries have cookstyle verdicts but all are compatible —
	// the blocking must be from TK only.
	if hasCookstyleVerdict && csFailCount == 0 {
		return CookstyleStatusPassed
	}

	return CookstyleStatusUnknown
}

// deriveKitchenStatus returns the Test Kitchen check status.
func deriveKitchenStatus(nr datastore.NodeReadiness) string {
	if nr.StaleData {
		return KitchenStatusUnknown
	}

	blocking := parseBlockingCookbooks(nr.BlockingCookbooks)

	if nr.AllCookbooksCompatible && len(blocking) == 0 {
		return KitchenStatusPassed
	}

	tkFailCount := 0
	tkTestedCount := 0
	noTKVerdictCount := 0

	for _, b := range blocking {
		hasTK := false
		for _, v := range b.Verdicts {
			if v.Source == "git_test_kitchen" {
				hasTK = true
				tkTestedCount++
				if v.Status == "incompatible" {
					tkFailCount++
				}
				break
			}
		}
		if !hasTK {
			noTKVerdictCount++
		}
	}

	if tkFailCount > 0 {
		return KitchenStatusFailed
	}

	// Some blocking entries tested, some not.
	if tkTestedCount > 0 && noTKVerdictCount > 0 {
		return KitchenStatusPartial
	}

	// All blocking entries lack TK results entirely.
	if len(blocking) > 0 && tkTestedCount == 0 {
		return KitchenStatusUnknown
	}

	return KitchenStatusUnknown
}

// diskDetail returns a human-readable detail string for the disk check.
func diskDetail(nr datastore.NodeReadiness, installPath string) *string {
	if nr.SufficientDiskSpace == nil {
		return strPtr("Disk: unknown")
	}
	if *nr.SufficientDiskSpace {
		if nr.AvailableDiskMB != nil {
			s := fmt.Sprintf("Disk: sufficient (%.1f GB free on %s)", float64(*nr.AvailableDiskMB)/1024.0, installPath)
			return &s
		}
		return strPtr("Disk: sufficient")
	}
	// Insufficient.
	switch {
	case nr.AvailableDiskMB != nil && nr.RequiredDiskMB != nil:
		s := fmt.Sprintf("Disk: insufficient (%.1f GB free on %s, need %.1f GB)",
			float64(*nr.AvailableDiskMB)/1024.0, installPath, float64(*nr.RequiredDiskMB)/1024.0)
		return &s
	case nr.AvailableDiskMB != nil:
		s := fmt.Sprintf("Disk: insufficient (%.1f GB free on %s)", float64(*nr.AvailableDiskMB)/1024.0, installPath)
		return &s
	default:
		return strPtr("Disk: insufficient")
	}
}

// cookstyleDetail returns a human-readable detail string for the CookStyle check.
func cookstyleDetail(nr datastore.NodeReadiness, status string) *string {
	switch status {
	case CookstyleStatusPassed:
		return strPtr("CookStyle: all cookbooks passed")
	case CookstyleStatusFailed:
		n := countCookstyleFailures(nr.BlockingCookbooks)
		if n == 1 {
			return strPtr("CookStyle: 1 cookbook incompatible")
		}
		return strPtr(fmt.Sprintf("CookStyle: %d cookbooks incompatible", n))
	default:
		return strPtr("CookStyle: not scanned")
	}
}

// kitchenDetail returns a human-readable detail string for the Kitchen check.
func kitchenDetail(nr datastore.NodeReadiness, status string) *string {
	switch status {
	case KitchenStatusPassed:
		return strPtr("Test Kitchen: all passed")
	case KitchenStatusFailed:
		n := countKitchenFailures(nr.BlockingCookbooks)
		if n == 1 {
			return strPtr("Test Kitchen: 1 cookbook failed")
		}
		return strPtr(fmt.Sprintf("Test Kitchen: %d cookbooks failed", n))
	case KitchenStatusPartial:
		return strPtr("Test Kitchen: partially tested")
	default:
		return strPtr("Test Kitchen: not tested")
	}
}

// parseBlockingCookbooks unmarshals the blocking_cookbooks JSON into a slice
// of blockingEntry. Returns nil on empty, null, or invalid JSON.
func parseBlockingCookbooks(raw json.RawMessage) []blockingEntry {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var entries []blockingEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil
	}
	return entries
}

// isCookstyleSource returns true if the verdict source is a cookstyle check.
func isCookstyleSource(source string) bool {
	return strings.Contains(source, "cookstyle")
}

// isOnlyTKFailure returns true if the blocking entry has only a
// git_test_kitchen incompatible verdict and no cookstyle failures.
func isOnlyTKFailure(b blockingEntry) bool {
	hasTKFail := false
	for _, v := range b.Verdicts {
		if isCookstyleSource(v.Source) && v.Status == "incompatible" {
			return false
		}
		if v.Source == "git_test_kitchen" && v.Status == "incompatible" {
			hasTKFail = true
		}
	}
	return hasTKFail
}

// countCookstyleFailures counts blocking cookbooks with incompatible
// cookstyle verdicts.
func countCookstyleFailures(raw json.RawMessage) int {
	blocking := parseBlockingCookbooks(raw)
	count := 0
	for _, b := range blocking {
		for _, v := range b.Verdicts {
			if isCookstyleSource(v.Source) && v.Status == "incompatible" {
				count++
				break
			}
		}
	}
	return count
}

// countKitchenFailures counts blocking cookbooks with incompatible
// git_test_kitchen verdicts.
func countKitchenFailures(raw json.RawMessage) int {
	blocking := parseBlockingCookbooks(raw)
	count := 0
	for _, b := range blocking {
		for _, v := range b.Verdicts {
			if v.Source == "git_test_kitchen" && v.Status == "incompatible" {
				count++
				break
			}
		}
	}
	return count
}

// strPtr returns a pointer to s.
func strPtr(s string) *string {
	return &s
}
