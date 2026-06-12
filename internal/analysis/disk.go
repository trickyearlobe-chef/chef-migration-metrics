// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"encoding/json"
	"strings"
)

// DiskConfig is the version-invariant configuration the disk-space check needs:
// the Chef install path and size per platform, plus the minimum free-space
// percentage that must remain after an install. None of these depend on the
// target Chef version.
type DiskConfig struct {
	InstallPathLinux        string
	InstallPathWindows      string
	InstallSizeMBLinux      int
	InstallSizeMBWindows    int
	MinRemainingFreePercent int
}

func (c DiskConfig) installPath(platform string) string {
	if strings.ToLower(platform) == "windows" {
		return c.InstallPathWindows
	}
	return c.InstallPathLinux
}

func (c DiskConfig) installSizeMB(platform string) int {
	if strings.ToLower(platform) == "windows" {
		return c.InstallSizeMBWindows
	}
	return c.InstallSizeMBLinux
}

// DiskVerdict is the version-invariant disk-space evaluation for one node.
// Sufficient and AvailableMB/TotalMB are nil when the node's filesystem data is
// missing or unparseable (indeterminate) — distinct from a determinate false.
// RequiredMB depends only on the platform, so it is always set.
type DiskVerdict struct {
	Sufficient  *bool
	AvailableMB *int
	RequiredMB  int
	TotalMB     *int
}

// EvaluateDisk computes the disk-space verdict for a node from its ohai
// filesystem attribute and platform, using the install path/size and
// min-remaining-free-% from cfg. It does NOT take a target Chef version: the
// verdict is identical across every target. It returns an indeterminate verdict
// (Sufficient nil) when no usable filesystem entry is found for the install path.
//
// The verdict applies a dual threshold (tls/disk spec): the available space must
// cover (1) the absolute install size AND (2) leave at least
// MinRemainingFreePercent of the filesystem free after the install.
func EvaluateDisk(filesystem json.RawMessage, platform string, cfg DiskConfig) DiskVerdict {
	required := cfg.installSizeMB(platform)
	verdict := DiskVerdict{RequiredMB: required}

	availMB, totalMB, known := resolveDiskUsage(filesystem, platform, cfg.installPath(platform))
	if !known {
		return verdict // Sufficient / AvailableMB / TotalMB stay nil — indeterminate
	}
	a, total := availMB, totalMB
	verdict.AvailableMB = &a
	verdict.TotalMB = &total

	absoluteOK := availMB >= required
	percentOK := true
	if totalMB > 0 && cfg.MinRemainingFreePercent > 0 {
		remainingAfterInstallKB := (int64(availMB) - int64(required)) * 1024
		totalKB := int64(totalMB) * 1024
		pctRemaining := float64(remainingAfterInstallKB) / float64(totalKB) * 100
		percentOK = pctRemaining >= float64(cfg.MinRemainingFreePercent)
	}
	sufficient := absoluteOK && percentOK
	verdict.Sufficient = &sufficient
	return verdict
}

// resolveDiskUsage parses the filesystem attribute, finds the entry whose mount
// best matches the install path, and returns its available/total space in MB.
// known is false when there is no usable filesystem data for the install path.
func resolveDiskUsage(filesystem json.RawMessage, platform, installPath string) (availableMB, totalMB int, known bool) {
	if len(filesystem) == 0 {
		return 0, 0, false
	}
	fsMap := parseFilesystemAttribute(filesystem)
	if len(fsMap) == 0 {
		return 0, 0, false
	}
	matchedMount, entry := findBestMount(fsMap, installPath, platform)
	if matchedMount == "" && entry == nil {
		return 0, 0, false
	}
	kbAvail := toInt64(entry.KBAvailable)
	if kbAvail < 0 {
		kbAvail = 0
	}
	kbSize := toInt64(entry.KBSize)
	if kbSize < 0 {
		kbSize = 0
	}
	return int(kbAvail / 1024), int(kbSize / 1024), true
}
