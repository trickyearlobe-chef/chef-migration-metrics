// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package platform

import (
	"fmt"
	"strconv"
	"strings"
)

// PlatformInfo holds resolved display and grouping metadata for a node's platform.
type PlatformInfo struct {
	DisplayName      string // "RHEL 8.10", "Windows Server 2022 Datacenter"
	GroupKey         string // Stable machine key: "rhel:redhat:8", "windows:server-2022"
	GroupDisplayName string // "RHEL 8", "Windows Server 2022"
	SortKey          string // Lexicographically sortable with numeric awareness
}

// DefaultAbbreviations maps lowercase Ohai platform names to proper display names.
var DefaultAbbreviations = map[string]string{
	"redhat":    "RHEL",
	"centos":    "CentOS",
	"rocky":     "Rocky",
	"almalinux": "AlmaLinux",
	"oracle":    "Oracle Linux",
	"amazon":    "Amazon Linux",
	"fedora":    "Fedora",
	"aix":       "AIX",
	"sles":      "SLES",
	"suse":      "SUSE",
	"opensuse":  "openSUSE",
	"mac_os_x":  "macOS",
	"macos":     "macOS",
}

// ResolveInfo resolves display name, group key, group display name, and sort key
// for a node's platform data. Resolution precedence:
//  1. Explicit admin mapping (longest prefix wins)
//  2. Caption-derived (parsed from platform_caption)
//  3. Abbreviation fallback
//  4. Raw fallback
func ResolveInfo(platform, version, family, caption string, mappings []DisplayNameMapping) PlatformInfo {
	if platform == "" && version == "" {
		return PlatformInfo{
			DisplayName:      "unknown",
			GroupKey:         "other:unknown:0",
			GroupDisplayName: "unknown",
			SortKey:          "zzz:unknown:00000:00000:00000",
		}
	}

	info := PlatformInfo{}

	// --- Resolve display name (4-tier) ---
	info.DisplayName = resolveDisplayName(platform, version, caption, mappings)

	// --- Resolve group ---
	info.GroupKey, info.GroupDisplayName = resolveGroup(platform, version, family, caption)

	// --- Resolve sort key ---
	info.SortKey = buildSortKey(platform, version, family)

	return info
}

func resolveDisplayName(platform, version, caption string, mappings []DisplayNameMapping) string {
	// Tier 1: explicit admin mapping
	if name, ok := ResolveName(platform, version, mappings); ok {
		return name
	}

	// Tier 2: caption-derived
	if caption != "" {
		if strings.EqualFold(platform, "windows") {
			disp, _ := parseWindowsCaption(caption)
			if disp != "" {
				return disp
			}
		} else {
			return caption
		}
	}

	// Tier 3: abbreviation fallback
	if abbrev, ok := DefaultAbbreviations[strings.ToLower(platform)]; ok {
		if version == "" {
			return abbrev
		}
		return abbrev + " " + version
	}

	// Tier 4: raw fallback
	if version == "" {
		return platform
	}
	return platform + " " + version
}

func resolveGroup(platform, version, family, caption string) (groupKey, groupDisplay string) {
	platLower := strings.ToLower(platform)

	switch family {
	case "rhel":
		return resolveGroupRHEL(platLower, version)
	case "windows":
		return resolveGroupWindows(platLower, version, caption)
	case "debian":
		return resolveGroupDebian(platLower, version)
	case "aix":
		return resolveGroupAIX(version)
	case "suse":
		return resolveGroupSUSE(platLower, version)
	case "mac_os_x":
		major := majorVersion(version)
		return fmt.Sprintf("macos:%s:%s", platLower, major), "macOS " + major
	default:
		major := majorVersion(version)
		abbrev := abbreviateOrTitle(platLower)
		if major == "" {
			return fmt.Sprintf("other:%s:0", platLower), abbrev
		}
		return fmt.Sprintf("other:%s:%s", platLower, major), abbrev + " " + major
	}
}

func resolveGroupRHEL(platform, version string) (string, string) {
	major := majorVersion(version)
	abbrev := abbreviateOrTitle(platform)
	if major == "" {
		return fmt.Sprintf("rhel:%s:0", platform), abbrev
	}
	return fmt.Sprintf("rhel:%s:%s", platform, major), abbrev + " " + major
}

func resolveGroupWindows(platform, version, caption string) (string, string) {
	if caption != "" {
		_, group := parseWindowsCaption(caption)
		if group != "" {
			key := strings.ToLower(strings.ReplaceAll(group, " ", "-"))
			return "windows:" + key, group
		}
	}
	// Fallback: use version prefix to guess generation
	return "windows:" + version, "Windows " + version
}

func resolveGroupDebian(platform, version string) (string, string) {
	if platform == "ubuntu" {
		// Group by YY.MM (first two dot-separated components)
		yyMM := ubuntuMajor(version)
		return fmt.Sprintf("debian:ubuntu:%s", yyMM), "Ubuntu " + yyMM
	}
	major := majorVersion(version)
	abbrev := abbreviateOrTitle(platform)
	return fmt.Sprintf("debian:%s:%s", platform, major), abbrev + " " + major
}

func resolveGroupAIX(version string) (string, string) {
	// Each M.m release is standalone
	v := version
	if v == "" {
		v = "0"
	}
	return fmt.Sprintf("aix:aix:%s", v), "AIX " + v
}

func resolveGroupSUSE(platform, version string) (string, string) {
	major := majorVersion(version)
	abbrev := abbreviateOrTitle(platform)
	if major == "" {
		return fmt.Sprintf("suse:%s:0", platform), abbrev
	}
	return fmt.Sprintf("suse:%s:%s", platform, major), abbrev + " " + major
}

// parseWindowsCaption parses "Microsoft Windows Server 2022 Datacenter" into
// display name ("Windows Server 2022 Datacenter") and group ("Windows Server 2022").
func parseWindowsCaption(caption string) (display, group string) {
	// Strip "Microsoft " prefix
	display = strings.TrimPrefix(caption, "Microsoft ")
	if display == caption {
		// No "Microsoft " prefix — use as-is
		display = caption
	}

	// Derive group: strip edition suffixes
	group = stripWindowsEdition(display)
	return display, group
}

func stripWindowsEdition(name string) string {
	editions := []string{
		" Datacenter",
		" Standard",
		" Enterprise",
		" Professional",
		" Education",
		" Pro",
		" Home",
		" N",
		" S",
	}
	result := name
	for changed := true; changed; {
		changed = false
		for _, ed := range editions {
			if strings.HasSuffix(result, ed) {
				result = strings.TrimSuffix(result, ed)
				changed = true
				break
			}
		}
	}
	return result
}

// buildSortKey builds a lexicographically sortable key with numeric awareness.
// Format: <family>:<platform>:<padded-major>:<padded-minor>:<padded-patch>
func buildSortKey(platform, version, family string) string {
	if family == "" {
		family = "zzz"
	}
	platLower := strings.ToLower(platform)
	parts := splitVersion(version)
	return fmt.Sprintf("%s:%s:%s:%s:%s",
		family, platLower, padNum(parts[0]), padNum(parts[1]), padNum(parts[2]))
}

// splitVersion splits a version into up to 3 numeric components.
func splitVersion(version string) [3]string {
	var result [3]string
	parts := strings.SplitN(version, ".", 4)
	for i := 0; i < 3 && i < len(parts); i++ {
		result[i] = parts[i]
	}
	return result
}

func padNum(s string) string {
	n, err := strconv.Atoi(s)
	if err != nil {
		if s == "" {
			return "00000"
		}
		return fmt.Sprintf("%05s", s)
	}
	return fmt.Sprintf("%05d", n)
}

func majorVersion(version string) string {
	if version == "" {
		return ""
	}
	idx := strings.IndexByte(version, '.')
	if idx < 0 {
		return version
	}
	return version[:idx]
}

func ubuntuMajor(version string) string {
	// Ubuntu versions are YY.MM — take first two components
	parts := strings.SplitN(version, ".", 3)
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}

func abbreviateOrTitle(platform string) string {
	if abbrev, ok := DefaultAbbreviations[platform]; ok {
		return abbrev
	}
	// Capitalize first letter as fallback
	if platform == "" {
		return ""
	}
	return strings.ToUpper(platform[:1]) + platform[1:]
}
