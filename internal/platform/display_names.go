// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package platform

import (
	"fmt"
	"strings"
)

// DetectOSFamilyFromPlatform returns the OS family for a Chef platform name.
// This uses the Ohai platform value (e.g. "windows", "ubuntu", "centos").
func DetectOSFamilyFromPlatform(plat string) string {
	p := strings.ToLower(plat)
	switch {
	case p == "windows":
		return "windows"
	case p == "ubuntu" || p == "debian" || p == "linuxmint" || p == "pop_os":
		return "debian"
	case p == "centos" || p == "redhat" || p == "rhel" || p == "oracle" ||
		p == "scientific" || p == "rocky" || p == "almalinux" || p == "amazon" ||
		p == "fedora":
		return "rhel"
	case p == "suse" || p == "opensuse" || p == "sles" || p == "opensuseleap":
		return "suse"
	case p == "mac_os_x" || p == "macos":
		return "macos"
	default:
		return "other"
	}
}

// DisplayNameMapping represents a single platform → friendly name mapping entry.
type DisplayNameMapping struct {
	Platform      string `json:"platform"`
	VersionPrefix string `json:"version_prefix"`
	DisplayName   string `json:"display_name"`
}

// DefaultMappings contains all built-in platform display name mappings.
var DefaultMappings = []DisplayNameMapping{
	// Windows Desktop
	{Platform: "windows", VersionPrefix: "10.0.26200", DisplayName: "Win11 25H2"},
	{Platform: "windows", VersionPrefix: "10.0.26100", DisplayName: "Win11 24H2 / Server 2025"},
	{Platform: "windows", VersionPrefix: "10.0.22631", DisplayName: "Win11 23H2"},
	{Platform: "windows", VersionPrefix: "10.0.22621", DisplayName: "Win11 22H2"},
	{Platform: "windows", VersionPrefix: "10.0.22000", DisplayName: "Win11 21H2"},
	{Platform: "windows", VersionPrefix: "10.0.19045", DisplayName: "Win10 22H2"},
	{Platform: "windows", VersionPrefix: "10.0.19044", DisplayName: "Win10 21H2"},
	{Platform: "windows", VersionPrefix: "10.0.19043", DisplayName: "Win10 21H1"},
	{Platform: "windows", VersionPrefix: "10.0.19042", DisplayName: "Win10 20H2"},
	{Platform: "windows", VersionPrefix: "10.0.18363", DisplayName: "Win10 1909"},
	{Platform: "windows", VersionPrefix: "10.0.17763", DisplayName: "Win10 1809 / Server 2019"},
	{Platform: "windows", VersionPrefix: "10.0.14393", DisplayName: "Win10 1607 / Server 2016"},
	{Platform: "windows", VersionPrefix: "6.3.9600", DisplayName: "Win8.1 / Server 2012 R2"},
	{Platform: "windows", VersionPrefix: "6.2.9200", DisplayName: "Win8 / Server 2012"},
	{Platform: "windows", VersionPrefix: "6.1.7601", DisplayName: "Win7 SP1 / Server 2008 R2"},

	// Windows Server (more-specific prefixes where distinguishable)
	{Platform: "windows", VersionPrefix: "10.0.20348", DisplayName: "Win Server 2022"},
	{Platform: "windows", VersionPrefix: "10.0.26334", DisplayName: "Win Server 2025"},

	// Ubuntu LTS releases
	{Platform: "ubuntu", VersionPrefix: "26.04", DisplayName: "Ubuntu 26.04 LTS (Plucky)"},
	{Platform: "ubuntu", VersionPrefix: "24.04", DisplayName: "Ubuntu 24.04 LTS (Noble)"},
	{Platform: "ubuntu", VersionPrefix: "22.04", DisplayName: "Ubuntu 22.04 LTS (Jammy)"},
	{Platform: "ubuntu", VersionPrefix: "20.04", DisplayName: "Ubuntu 20.04 LTS (Focal)"},
	{Platform: "ubuntu", VersionPrefix: "18.04", DisplayName: "Ubuntu 18.04 LTS (Bionic) — EOL"},
}

// ResolveName finds the best matching display name for the given platform
// and version. It uses prefix matching with longest-match-wins semantics.
// Platform comparison is case-insensitive.
// Returns ("", false) if no mapping matches.
func ResolveName(platform, version string, mappings []DisplayNameMapping) (string, bool) {
	bestName := ""
	bestLen := -1

	for _, m := range mappings {
		if !strings.EqualFold(m.Platform, platform) {
			continue
		}
		if !strings.HasPrefix(version, m.VersionPrefix) {
			continue
		}
		if len(m.VersionPrefix) > bestLen {
			bestLen = len(m.VersionPrefix)
			bestName = m.DisplayName
		}
	}

	if bestLen < 0 {
		return "", false
	}
	return bestName, true
}

// ResolveNameOrFallback returns the display name if found, otherwise returns
// "platform version" as a fallback string.
func ResolveNameOrFallback(platform, version string, mappings []DisplayNameMapping) string {
	if name, ok := ResolveName(platform, version, mappings); ok {
		return name
	}
	return fmt.Sprintf("%s %s", platform, version)
}

// IsDefault returns true if the given mappings slice is equivalent to
// DefaultMappings (same entries, order-independent).
func IsDefault(mappings []DisplayNameMapping) bool {
	if len(mappings) != len(DefaultMappings) {
		return false
	}

	type key struct {
		platform      string
		versionPrefix string
		displayName   string
	}

	counts := make(map[key]int, len(DefaultMappings))
	for _, m := range DefaultMappings {
		counts[key{
			platform:      strings.ToLower(m.Platform),
			versionPrefix: m.VersionPrefix,
			displayName:   m.DisplayName,
		}]++
	}

	for _, m := range mappings {
		k := key{
			platform:      strings.ToLower(m.Platform),
			versionPrefix: m.VersionPrefix,
			displayName:   m.DisplayName,
		}
		counts[k]--
		if counts[k] < 0 {
			return false
		}
	}

	return true
}
