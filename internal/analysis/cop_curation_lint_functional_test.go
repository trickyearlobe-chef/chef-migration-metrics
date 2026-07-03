// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package analysis_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// TestCuratedRemovalsAgainstRealBinary is the curation linter's CI gate: it runs
// the shipped RemovedIn mapping against the real `cookstyle --show-cops` and
// fails if any curated verified-removal entry has gone stale (cop removed or
// renamed upstream) or disagrees with the shipped cop description. It is the
// durability mechanism for the curated Blocker set — catching rot at the source.
func TestCuratedRemovalsAgainstRealBinary(t *testing.T) {
	path, err := exec.LookPath("cookstyle")
	if err != nil {
		t.Skip("cookstyle not on PATH; skipping functional curation-linter test")
	}

	p := analysis.NewCopRegistryProvider(analysis.NewCookstyleExecutor(path), "functional")
	reg, err := p.Registry(context.Background())
	if err != nil {
		t.Fatalf("loading cop registry from real binary: %v", err)
	}

	issues := analysis.ValidateCuratedRemovals(remediation.AllCopMappings(), reg)
	for _, i := range issues {
		t.Errorf("curation drift: %s [%s] — %s", i.CopName, i.Kind, i.Detail)
	}
	if len(issues) > 0 {
		t.Logf("%d curated verified-removal entrie(s) need manual resolution against cookstyle %s",
			len(issues), reg.Version())
	}
}

// TestCurationLinterDetectsInjectedDrift proves the linter FAILS on injected
// drift over the real registry: a curated RemovedIn for a cop the binary does
// not emit must be reported stale.
func TestCurationLinterDetectsInjectedDrift(t *testing.T) {
	path, err := exec.LookPath("cookstyle")
	if err != nil {
		t.Skip("cookstyle not on PATH; skipping functional curation-linter test")
	}

	p := analysis.NewCopRegistryProvider(analysis.NewCookstyleExecutor(path), "functional")
	reg, err := p.Registry(context.Background())
	if err != nil {
		t.Fatalf("loading cop registry from real binary: %v", err)
	}

	injected := []remediation.CopMapping{
		{CopName: "Chef/Deprecations/ThisCopDoesNotExistUpstream", RemovedIn: "14.0"},
	}
	issues := analysis.ValidateCuratedRemovals(injected, reg)
	if len(issues) != 1 || issues[0].Kind != analysis.CurationStale {
		t.Fatalf("expected one stale issue for the injected cop, got %+v", issues)
	}
}
