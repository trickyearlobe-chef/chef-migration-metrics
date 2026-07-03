// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// showCopsFixture is a trimmed but faithful sample of `cookstyle --show-cops`
// output: the availability header, per-department comment headers, the
// `# Supports --autocorrect` marker lines, multi-line wrapped Descriptions,
// single-quoted VersionAdded, disabled cops, list-valued keys (Include/Exclude)
// that must be ignored, and a mix of Chef/* and generic-Ruby departments.
const showCopsFixture = `# Available cops (5) + config for /home/user/repo:
# Department 'Chef/Correctness' (2):
# Supports --autocorrect
Chef/Correctness/BlockGuardWithOnlyString:
  Enabled: true
  Description: A resource guard (not_if/only_if) that is a string should not be wrapped
    in {}. Wrapping a guard string in {} causes it to be executed as Ruby code which
    will always return true.
  StyleGuide: chef_correctness_blockguardwithonlystring
  VersionAdded: 5.2.0
  Exclude:
  - "**/attributes/*.rb"
  - "**/metadata.rb"

Chef/Correctness/CookbookUsesNodeSave:
  Enabled: true
  Description: Checks for node.save usage.
  VersionAdded: '6.0.0'

# Department 'Chef/Deprecations' (1):
Chef/Deprecations/LogResourceNotifications:
  Enabled: true
  Description: Checks for log resource notifications removed in Chef 16.
  Severity: warning
  VersionAdded: '5.12.0'

# Department 'Style' (1):
Style/FrozenStringLiteralComment:
  Description: Add the frozen_string_literal comment to the top of files.
  Enabled: false
  VersionAdded: '0.36'
  Include:
  - "**/*.rb"

# Department 'Bundler' (1):
Bundler/DuplicatedGem:
  Description: Checks for duplicate gem entries in Gemfile.
  Enabled: true
  VersionAdded: '0.46'
  Include:
  - "**/Gemfile"
`

func TestParseShowCops(t *testing.T) {
	entries := ParseShowCops(showCopsFixture)

	if len(entries) != 5 {
		t.Fatalf("ParseShowCops returned %d entries, want 5: %+v", len(entries), entries)
	}

	byName := make(map[string]CopRegistryEntry, len(entries))
	for _, e := range entries {
		byName[e.CopName] = e
	}

	tests := []struct {
		name         string
		department   string
		topNamespace string
		enabled      bool
		severity     string
		versionAdded string
		descContains string
	}{
		{
			name:         "Chef/Correctness/BlockGuardWithOnlyString",
			department:   "Chef/Correctness",
			topNamespace: "Chef",
			enabled:      true,
			versionAdded: "5.2.0",
			descContains: "should not be wrapped in {}. Wrapping",
		},
		{
			name:         "Chef/Correctness/CookbookUsesNodeSave",
			department:   "Chef/Correctness",
			topNamespace: "Chef",
			enabled:      true,
			versionAdded: "6.0.0",
			descContains: "node.save",
		},
		{
			name:         "Chef/Deprecations/LogResourceNotifications",
			department:   "Chef/Deprecations",
			topNamespace: "Chef",
			enabled:      true,
			severity:     "warning",
			versionAdded: "5.12.0",
		},
		{
			name:         "Style/FrozenStringLiteralComment",
			department:   "Style",
			topNamespace: "Style",
			enabled:      false,
			versionAdded: "0.36",
		},
		{
			name:         "Bundler/DuplicatedGem",
			department:   "Bundler",
			topNamespace: "Bundler",
			enabled:      true,
			versionAdded: "0.46",
		},
	}

	for _, tt := range tests {
		e, ok := byName[tt.name]
		if !ok {
			t.Errorf("cop %q not parsed", tt.name)
			continue
		}
		if e.Department != tt.department {
			t.Errorf("%s Department = %q, want %q", tt.name, e.Department, tt.department)
		}
		if e.TopNamespace != tt.topNamespace {
			t.Errorf("%s TopNamespace = %q, want %q", tt.name, e.TopNamespace, tt.topNamespace)
		}
		if e.Enabled != tt.enabled {
			t.Errorf("%s Enabled = %v, want %v", tt.name, e.Enabled, tt.enabled)
		}
		if e.Severity != tt.severity {
			t.Errorf("%s Severity = %q, want %q", tt.name, e.Severity, tt.severity)
		}
		if e.VersionAdded != tt.versionAdded {
			t.Errorf("%s VersionAdded = %q, want %q", tt.name, e.VersionAdded, tt.versionAdded)
		}
		if tt.descContains != "" && !strings.Contains(e.Description, tt.descContains) {
			t.Errorf("%s Description = %q, want to contain %q", tt.name, e.Description, tt.descContains)
		}
	}
}

func TestParseShowCopsEmpty(t *testing.T) {
	if got := ParseShowCops(""); len(got) != 0 {
		t.Errorf("ParseShowCops(\"\") = %d entries, want 0", len(got))
	}
}

func TestCopRegistryChefCops(t *testing.T) {
	reg := NewCopRegistry(ParseShowCops(showCopsFixture), "8.6.10")

	chef := reg.ChefCops()
	if len(chef) != 3 {
		t.Fatalf("ChefCops returned %d, want 3 (the Chef/* cops): %+v", len(chef), chef)
	}
	for _, e := range chef {
		if e.TopNamespace != "Chef" {
			t.Errorf("ChefCops included non-Chef cop %q", e.CopName)
		}
	}

	if !reg.Has("Style/FrozenStringLiteralComment") {
		t.Error("Has should report true for a parsed generic cop")
	}
	if reg.Has("Chef/Deprecations/DoesNotExist") {
		t.Error("Has should report false for an unknown cop")
	}
	if reg.Version() != "8.6.10" {
		t.Errorf("Version = %q, want 8.6.10", reg.Version())
	}
}

func TestCopRegistryProviderCachesAndParses(t *testing.T) {
	fe := &fakeCookstyleExecutor{stdout: showCopsFixture}
	p := NewCopRegistryProvider(fe, "8.6.10")

	reg, err := p.Registry(context.Background())
	if err != nil {
		t.Fatalf("first Registry() error: %v", err)
	}
	if len(reg.Entries) != 5 {
		t.Fatalf("Registry parsed %d entries, want 5", len(reg.Entries))
	}

	// Second call must be served from cache — the binary runs once.
	reg2, err := p.Registry(context.Background())
	if err != nil {
		t.Fatalf("second Registry() error: %v", err)
	}
	if reg2 != reg {
		t.Error("second Registry() should return the cached instance")
	}
	if len(fe.calls) != 1 {
		t.Errorf("executor called %d times, want 1 (cached)", len(fe.calls))
	}
	if len(fe.calls) == 1 {
		args := fe.calls[0].Args
		if len(args) != 1 || args[0] != "--show-cops" {
			t.Errorf("executor args = %v, want [--show-cops]", args)
		}
	}
}

func TestCopRegistryProviderErrorPropagates(t *testing.T) {
	fe := &fakeCookstyleExecutor{err: errors.New("boom")}
	p := NewCopRegistryProvider(fe, "8.6.10")

	if _, err := p.Registry(context.Background()); err == nil {
		t.Fatal("Registry() should return an error when the executor fails")
	}
	// A failed load must not be cached — a later successful call should retry.
	fe.err = nil
	fe.stdout = showCopsFixture
	reg, err := p.Registry(context.Background())
	if err != nil {
		t.Fatalf("retry Registry() error: %v", err)
	}
	if len(reg.Entries) != 5 {
		t.Errorf("retry parsed %d entries, want 5", len(reg.Entries))
	}
}
