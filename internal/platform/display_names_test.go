// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package platform

import (
	"testing"
)

func TestResolveName_ExactMatch(t *testing.T) {
	name, ok := ResolveName("windows", "10.0.22631", DefaultMappings)
	if !ok {
		t.Fatal("expected match, got none")
	}
	if name != "Win11 23H2" {
		t.Errorf("got %q, want %q", name, "Win11 23H2")
	}
}

func TestResolveName_PrefixMatch(t *testing.T) {
	name, ok := ResolveName("windows", "10.0.22631.2345", DefaultMappings)
	if !ok {
		t.Fatal("expected match, got none")
	}
	if name != "Win11 23H2" {
		t.Errorf("got %q, want %q", name, "Win11 23H2")
	}
}

func TestResolveName_LongestPrefixWins(t *testing.T) {
	mappings := []DisplayNameMapping{
		{Platform: "windows", VersionPrefix: "10.0", DisplayName: "Generic Win10+"},
		{Platform: "windows", VersionPrefix: "10.0.22631", DisplayName: "Win11 23H2"},
	}
	name, ok := ResolveName("windows", "10.0.22631", mappings)
	if !ok {
		t.Fatal("expected match, got none")
	}
	if name != "Win11 23H2" {
		t.Errorf("got %q, want %q", name, "Win11 23H2")
	}

	// A version that only matches the short prefix.
	name, ok = ResolveName("windows", "10.0.99999", mappings)
	if !ok {
		t.Fatal("expected match, got none")
	}
	if name != "Generic Win10+" {
		t.Errorf("got %q, want %q", name, "Generic Win10+")
	}
}

func TestResolveName_CaseInsensitivePlatform(t *testing.T) {
	for _, p := range []string{"Windows", "WINDOWS", "wInDoWs"} {
		name, ok := ResolveName(p, "10.0.22631", DefaultMappings)
		if !ok {
			t.Errorf("platform %q: expected match, got none", p)
			continue
		}
		if name != "Win11 23H2" {
			t.Errorf("platform %q: got %q, want %q", p, name, "Win11 23H2")
		}
	}
}

func TestResolveName_NoMatch(t *testing.T) {
	name, ok := ResolveName("solaris", "11.4", DefaultMappings)
	if ok {
		t.Errorf("expected no match, got %q", name)
	}
	if name != "" {
		t.Errorf("expected empty string, got %q", name)
	}
}

func TestResolveName_EmptyMappings(t *testing.T) {
	name, ok := ResolveName("windows", "10.0.22631", nil)
	if ok {
		t.Errorf("expected no match, got %q", name)
	}
	if name != "" {
		t.Errorf("expected empty string, got %q", name)
	}
}

func TestResolveName_LinuxVersions(t *testing.T) {
	name, ok := ResolveName("centos", "8.5.2111", DefaultMappings)
	if !ok {
		t.Fatal("expected match, got none")
	}
	if name != "CentOS 8 (EOL)" {
		t.Errorf("got %q, want %q", name, "CentOS 8 (EOL)")
	}
}

func TestResolveNameOrFallback_WithMatch(t *testing.T) {
	result := ResolveNameOrFallback("windows", "10.0.22631", DefaultMappings)
	if result != "Win11 23H2" {
		t.Errorf("got %q, want %q", result, "Win11 23H2")
	}
}

func TestResolveNameOrFallback_NoMatch(t *testing.T) {
	result := ResolveNameOrFallback("solaris", "11.4", DefaultMappings)
	want := "solaris 11.4"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestIsDefault_MatchesDefaults(t *testing.T) {
	if !IsDefault(DefaultMappings) {
		t.Error("expected IsDefault(DefaultMappings) to return true")
	}

	// A copy in the same order should also match.
	cp := make([]DisplayNameMapping, len(DefaultMappings))
	copy(cp, DefaultMappings)
	if !IsDefault(cp) {
		t.Error("expected IsDefault on a copy to return true")
	}

	// A copy in reversed order should also match (order-independent).
	reversed := make([]DisplayNameMapping, len(DefaultMappings))
	for i, m := range DefaultMappings {
		reversed[len(DefaultMappings)-1-i] = m
	}
	if !IsDefault(reversed) {
		t.Error("expected IsDefault on reversed copy to return true")
	}
}

func TestIsDefault_ModifiedMappings(t *testing.T) {
	modified := make([]DisplayNameMapping, len(DefaultMappings))
	copy(modified, DefaultMappings)
	modified[0].DisplayName = "Something Else"
	if IsDefault(modified) {
		t.Error("expected IsDefault to return false for modified mappings")
	}

	// Different length.
	shorter := DefaultMappings[:len(DefaultMappings)-1]
	if IsDefault(shorter) {
		t.Error("expected IsDefault to return false for shorter slice")
	}

	// Empty.
	if IsDefault(nil) {
		t.Error("expected IsDefault to return false for nil")
	}
}
