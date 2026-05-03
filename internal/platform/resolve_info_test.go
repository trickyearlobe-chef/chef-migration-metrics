// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package platform

import (
	"testing"
)

// ---------------------------------------------------------------------------
// DefaultAbbreviations tests
// ---------------------------------------------------------------------------

func TestDefaultAbbreviations_ContainsExpectedEntries(t *testing.T) {
	expected := map[string]string{
		"redhat":    "RHEL",
		"aix":       "AIX",
		"centos":    "CentOS",
		"almalinux": "AlmaLinux",
		"rocky":     "Rocky",
		"oracle":    "Oracle Linux",
		"sles":      "SLES",
		"suse":      "SUSE",
		"opensuse":  "openSUSE",
		"fedora":    "Fedora",
		"mac_os_x":  "macOS",
		"macos":     "macOS",
		"amazon":    "Amazon Linux",
	}
	for k, want := range expected {
		got, ok := DefaultAbbreviations[k]
		if !ok {
			t.Errorf("missing abbreviation for %q", k)
			continue
		}
		if got != want {
			t.Errorf("DefaultAbbreviations[%q] = %q, want %q", k, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// ResolveInfo tests
// ---------------------------------------------------------------------------

func TestResolveInfo_AdminMappingWins(t *testing.T) {
	mappings := []DisplayNameMapping{
		{Platform: "redhat", VersionPrefix: "8.10", DisplayName: "Red Hat Enterprise Linux 8.10 (custom)"},
	}
	info := ResolveInfo("redhat", "8.10", "rhel", "", mappings)
	if info.DisplayName != "Red Hat Enterprise Linux 8.10 (custom)" {
		t.Errorf("got %q, want admin mapping to win", info.DisplayName)
	}
}

func TestResolveInfo_CaptionFallback(t *testing.T) {
	info := ResolveInfo("windows", "10.0.20348", "windows", "Microsoft Windows Server 2022 Datacenter", nil)
	if info.DisplayName != "Windows Server 2022 Datacenter" {
		t.Errorf("got DisplayName=%q", info.DisplayName)
	}
	if info.GroupDisplayName != "Windows Server 2022" {
		t.Errorf("got GroupDisplayName=%q", info.GroupDisplayName)
	}
}

func TestResolveInfo_AbbreviationFallback(t *testing.T) {
	tests := []struct {
		platform string
		version  string
		family   string
		wantDisp string
		wantGrp  string
	}{
		{"redhat", "8.10", "rhel", "RHEL 8.10", "RHEL 8"},
		{"redhat", "7.9", "rhel", "RHEL 7.9", "RHEL 7"},
		{"redhat", "9.7", "rhel", "RHEL 9.7", "RHEL 9"},
		{"centos", "7.8.2003", "rhel", "CentOS 7.8.2003", "CentOS 7"},
		{"almalinux", "9.7", "rhel", "AlmaLinux 9.7", "AlmaLinux 9"},
		{"aix", "7.2", "aix", "AIX 7.2", "AIX 7.2"},
		{"aix", "7.1", "aix", "AIX 7.1", "AIX 7.1"},
		{"sles", "15.4", "suse", "SLES 15.4", "SLES 15"},
	}
	for _, tt := range tests {
		t.Run(tt.platform+"_"+tt.version, func(t *testing.T) {
			info := ResolveInfo(tt.platform, tt.version, tt.family, "", nil)
			if info.DisplayName != tt.wantDisp {
				t.Errorf("DisplayName: got %q, want %q", info.DisplayName, tt.wantDisp)
			}
			if info.GroupDisplayName != tt.wantGrp {
				t.Errorf("GroupDisplayName: got %q, want %q", info.GroupDisplayName, tt.wantGrp)
			}
		})
	}
}

func TestResolveInfo_RawFallback(t *testing.T) {
	info := ResolveInfo("solaris", "11.4", "other", "", nil)
	if info.DisplayName != "solaris 11.4" {
		t.Errorf("got %q, want raw fallback", info.DisplayName)
	}
}

func TestResolveInfo_UbuntuGrouping(t *testing.T) {
	info := ResolveInfo("ubuntu", "24.04", "debian", "Ubuntu 24.04.4 LTS", nil)
	if info.GroupDisplayName != "Ubuntu 24.04" {
		t.Errorf("got GroupDisplayName=%q, want %q", info.GroupDisplayName, "Ubuntu 24.04")
	}
	if info.GroupKey != "debian:ubuntu:24.04" {
		t.Errorf("got GroupKey=%q", info.GroupKey)
	}
}

func TestResolveInfo_UbuntuWithoutCaption(t *testing.T) {
	info := ResolveInfo("ubuntu", "22.04", "debian", "", nil)
	if info.DisplayName != "ubuntu 22.04" {
		// No abbreviation for ubuntu, falls to raw
		t.Errorf("got DisplayName=%q", info.DisplayName)
	}
	if info.GroupDisplayName != "Ubuntu 22.04" {
		t.Errorf("got GroupDisplayName=%q", info.GroupDisplayName)
	}
}

func TestResolveInfo_WindowsWithoutCaption_UsesMapping(t *testing.T) {
	info := ResolveInfo("windows", "10.0.20348", "windows", "", DefaultMappings)
	if info.DisplayName != "Win Server 2022" {
		t.Errorf("got DisplayName=%q, want mapping result", info.DisplayName)
	}
	if info.GroupDisplayName != "Windows Server 2022" {
		t.Errorf("got GroupDisplayName=%q, want %q", info.GroupDisplayName, "Windows Server 2022")
	}
	if info.GroupKey != "windows:windows-server-2022" {
		t.Errorf("got GroupKey=%q, want %q", info.GroupKey, "windows:windows-server-2022")
	}
}

func TestResolveInfo_WindowsGroupFromDisplayName(t *testing.T) {
	cases := []struct {
		version   string
		wantGroup string
	}{
		{"10.0.26200", "Windows 11"},
		{"10.0.26100", "Windows 11"},
		{"10.0.22631", "Windows 11"},
		{"10.0.22000", "Windows 11"},
		{"10.0.19045", "Windows 10"},
		{"10.0.17763", "Windows 10"},
		{"10.0.14393", "Windows 10"},
		{"10.0.20348", "Windows Server 2022"},
		{"10.0.26334", "Windows Server 2025"},
		{"6.3.9600", "Windows 8.1"},
		{"6.2.9200", "Windows 8"},
		{"6.1.7601", "Windows 7"},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			info := ResolveInfo("windows", tc.version, "windows", "", DefaultMappings)
			if info.GroupDisplayName != tc.wantGroup {
				t.Errorf("GroupDisplayName=%q, want %q (display=%q)", info.GroupDisplayName, tc.wantGroup, info.DisplayName)
			}
		})
	}
}

func TestResolveInfo_EmptyPlatform(t *testing.T) {
	info := ResolveInfo("", "", "", "", nil)
	if info.DisplayName != "unknown" {
		t.Errorf("got %q, want %q", info.DisplayName, "unknown")
	}
}

func TestResolveInfo_EmptyVersion(t *testing.T) {
	info := ResolveInfo("redhat", "", "rhel", "", nil)
	if info.DisplayName != "RHEL" {
		t.Errorf("got %q, want %q", info.DisplayName, "RHEL")
	}
}

// ---------------------------------------------------------------------------
// GroupKey tests
// ---------------------------------------------------------------------------

func TestResolveInfo_GroupKeys(t *testing.T) {
	tests := []struct {
		platform string
		version  string
		family   string
		wantKey  string
	}{
		{"redhat", "8.10", "rhel", "rhel:redhat:8"},
		{"centos", "7.8.2003", "rhel", "rhel:centos:7"},
		{"aix", "7.2", "aix", "aix:aix:7.2"},
		{"aix", "7.1", "aix", "aix:aix:7.1"},
		{"sles", "15.4", "suse", "suse:sles:15"},
		{"ubuntu", "22.04", "debian", "debian:ubuntu:22.04"},
		{"debian", "12.1", "debian", "debian:debian:12"},
	}
	for _, tt := range tests {
		t.Run(tt.platform+"_"+tt.version, func(t *testing.T) {
			info := ResolveInfo(tt.platform, tt.version, tt.family, "", nil)
			if info.GroupKey != tt.wantKey {
				t.Errorf("GroupKey: got %q, want %q", info.GroupKey, tt.wantKey)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SortKey tests
// ---------------------------------------------------------------------------

func TestResolveInfo_SortKeyOrdering(t *testing.T) {
	// RHEL 8.9 should sort before RHEL 8.10
	info89 := ResolveInfo("redhat", "8.9", "rhel", "", nil)
	info810 := ResolveInfo("redhat", "8.10", "rhel", "", nil)
	if info89.SortKey >= info810.SortKey {
		t.Errorf("RHEL 8.9 sort key %q should be < RHEL 8.10 sort key %q",
			info89.SortKey, info810.SortKey)
	}

	// Same family groups together
	infoCentos7 := ResolveInfo("centos", "7.9", "rhel", "", nil)
	if infoCentos7.SortKey >= info89.SortKey {
		t.Errorf("CentOS 7.9 %q should sort before RHEL 8.9 %q",
			infoCentos7.SortKey, info89.SortKey)
	}
}

func TestResolveInfo_SortKeyWindowsOrdering(t *testing.T) {
	info2019 := ResolveInfo("windows", "10.0.17763", "windows",
		"Microsoft Windows Server 2019 Datacenter", nil)
	info2022 := ResolveInfo("windows", "10.0.20348", "windows",
		"Microsoft Windows Server 2022 Datacenter", nil)
	if info2019.SortKey >= info2022.SortKey {
		t.Errorf("Server 2019 sort %q should be < Server 2022 sort %q",
			info2019.SortKey, info2022.SortKey)
	}
}

// ---------------------------------------------------------------------------
// Windows caption parsing tests
// ---------------------------------------------------------------------------

func TestParseWindowsCaption(t *testing.T) {
	tests := []struct {
		caption   string
		wantDisp  string
		wantGroup string
	}{
		{"Microsoft Windows Server 2022 Datacenter", "Windows Server 2022 Datacenter", "Windows Server 2022"},
		{"Microsoft Windows Server 2019 Standard", "Windows Server 2019 Standard", "Windows Server 2019"},
		{"Microsoft Windows Server 2016 Datacenter", "Windows Server 2016 Datacenter", "Windows Server 2016"},
		{"Microsoft Windows 11 Pro", "Windows 11 Pro", "Windows 11"},
		{"Microsoft Windows 11 Enterprise", "Windows 11 Enterprise", "Windows 11"},
		{"Microsoft Windows 10 Education N", "Windows 10 Education N", "Windows 10"},
		{"Microsoft Windows 10 Pro", "Windows 10 Pro", "Windows 10"},
	}
	for _, tt := range tests {
		t.Run(tt.caption, func(t *testing.T) {
			disp, group := parseWindowsCaption(tt.caption)
			if disp != tt.wantDisp {
				t.Errorf("display: got %q, want %q", disp, tt.wantDisp)
			}
			if group != tt.wantGroup {
				t.Errorf("group: got %q, want %q", group, tt.wantGroup)
			}
		})
	}
}
