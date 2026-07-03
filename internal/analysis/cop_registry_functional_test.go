// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package analysis_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
)

// TestCopRegistryProviderRealBinary runs the real `cookstyle --show-cops` and
// checks the parser copes with the full ruleset: hundreds of cops, a healthy
// slice of Chef/* cops, and no obviously mangled names. Guards against parser
// drift when cookstyle upgrades change the output shape.
func TestCopRegistryProviderRealBinary(t *testing.T) {
	path, err := exec.LookPath("cookstyle")
	if err != nil {
		t.Skip("cookstyle not on PATH; skipping functional registry test")
	}

	p := analysis.NewCopRegistryProvider(analysis.NewCookstyleExecutor(path), "functional")
	reg, err := p.Registry(context.Background())
	if err != nil {
		t.Fatalf("Registry() over real binary: %v", err)
	}

	if len(reg.Entries) < 500 {
		t.Errorf("parsed %d cops, expected the full ruleset (>500)", len(reg.Entries))
	}
	chef := reg.ChefCops()
	if len(chef) < 50 {
		t.Errorf("parsed %d Chef/* cops, expected >50", len(chef))
	}
	for _, e := range chef {
		if e.TopNamespace != "Chef" {
			t.Errorf("ChefCops returned non-Chef cop %q", e.CopName)
		}
		if e.CopName == "" || e.Department == "" {
			t.Errorf("cop entry has empty name/department: %+v", e)
		}
	}

	// A well-known Chef deprecation cop should be present and classified as a
	// department default (sanity check the name survives parsing verbatim).
	if !reg.Has("Chef/Deprecations/LogResourceNotifications") {
		t.Error("expected Chef/Deprecations/LogResourceNotifications in the live registry")
	}
}
