// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// ---------------------------------------------------------------------------
// Operator addon cop files — config-driven require injection + load failure
// isolation.
// ---------------------------------------------------------------------------

// seqExecutor returns a different canned result per call, cycling through the
// configured results (last one repeats). It records every invocation's args.
type seqExecutor struct {
	results []fakeCookstyleResult
	calls   [][]string
}

func (e *seqExecutor) Run(_ context.Context, _ string, args ...string) (string, string, int, error) {
	idx := len(e.calls)
	e.calls = append(e.calls, append([]string{}, args...))
	if idx >= len(e.results) {
		idx = len(e.results) - 1
	}
	r := e.results[idx]
	return r.stdout, r.stderr, r.exitCode, r.err
}

// writeAddonCop creates a minimal but well-formed addon cop .rb file (so its cop
// name parses) and returns its path.
func writeAddonCop(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	src := `module RuboCop
  module Cop
    module Cmm
      class Sample < RuboCop::Cop::Base
      end
    end
  end
end
`
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestScan_AddonRequireInjectedIntoSidecar proves a configured addon cop path is
// resolved and require:'d into the scan sidecar.
func TestScan_AddonRequireInjectedIntoSidecar(t *testing.T) {
	cookbookDir := t.TempDir()
	addonDir := t.TempDir()
	addon := writeAddonCop(t, addonDir, "no_eval.rb")

	fe := &fakeCookstyleExecutor{stdout: makeCleanJSON()}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe),
		WithCookstyleAddonCopPathsFn(func() []string { return []string{addon} }),
	)

	s.scanOneNoDB(context.Background(), "cb", "1.0.0", "18.0", cookbookDir)

	sidecar, err := os.ReadFile(filepath.Join(cookbookDir, remediation.CmmConfigName))
	if err != nil {
		t.Fatalf("sidecar not written: %v", err)
	}
	if !strings.Contains(string(sidecar), addon) {
		t.Errorf("sidecar must require the addon cop %q:\n%s", addon, sidecar)
	}
	if !strings.Contains(string(sidecar), "Cmm/Sample:\n  Enabled: true") {
		t.Errorf("sidecar must enable the addon cop explicitly:\n%s", sidecar)
	}
}

// TestScan_NoAddonProviderLeavesScanUnchanged proves that with no addon provider
// the sidecar contains no extra requires (baseline behaviour preserved).
func TestScan_NoAddonProviderLeavesScanUnchanged(t *testing.T) {
	cookbookDir := t.TempDir()
	fe := &fakeCookstyleExecutor{stdout: makeCleanJSON()}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	s.scanOneNoDB(context.Background(), "cb", "1.0.0", "18.0", cookbookDir)

	sidecar, _ := os.ReadFile(filepath.Join(cookbookDir, remediation.CmmConfigName))
	if strings.Contains(string(sidecar), "Enabled: true") {
		t.Errorf("no addons configured — no cop-enable block must appear:\n%s", sidecar)
	}
}

// TestScan_BrokenAddonIsolated_NotErrored proves the load-failure isolation: a
// broken addon cop makes cookstyle exit 2, but the scan retries WITHOUT the
// addon, succeeds, and the cookbook is NOT recorded as errored.
func TestScan_BrokenAddonIsolated_NotErrored(t *testing.T) {
	cookbookDir := t.TempDir()
	addonDir := t.TempDir()
	addon := writeAddonCop(t, addonDir, "broken.rb")

	fe := &seqExecutor{results: []fakeCookstyleResult{
		// 1st call (addons loaded): cookstyle errors loading the broken cop.
		{stderr: "cannot load such file -- broken", exitCode: 2},
		// 2nd call (addons removed): clean scan with one cosmetic offence.
		{stdout: makeOffenseJSON("Style/StringLiterals", "convention", "Prefer single quotes", false), exitCode: 1},
	}}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe),
		WithCookstyleAddonCopPathsFn(func() []string { return []string{addon} }),
	)

	sr := s.scanOneNoDB(context.Background(), "cb", "1.0.0", "18.0", cookbookDir)

	if len(fe.calls) != 2 {
		t.Fatalf("expected an isolation retry (2 calls), got %d", len(fe.calls))
	}
	if sr.ErrorMessage != "" || sr.Error != nil {
		t.Errorf("a broken addon must NOT error the cookbook; got ErrorMessage=%q Error=%v", sr.ErrorMessage, sr.Error)
	}
	if sr.OffenseCount != 1 {
		t.Errorf("expected the clean-retry offences to be counted, got %d", sr.OffenseCount)
	}
}

// TestScan_GenuineErrorWithAddons_StillErrors proves isolation does not mask a
// real cookbook error: when cookstyle errors both WITH and WITHOUT addons, the
// scan is recorded as errored.
func TestScan_GenuineErrorWithAddons_StillErrors(t *testing.T) {
	cookbookDir := t.TempDir()
	addonDir := t.TempDir()
	addon := writeAddonCop(t, addonDir, "ok.rb")

	fe := &seqExecutor{results: []fakeCookstyleResult{
		{stderr: "bad .rubocop.yml", exitCode: 2}, // with addons
		{stderr: "bad .rubocop.yml", exitCode: 2}, // without addons — still broken
	}}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe),
		WithCookstyleAddonCopPathsFn(func() []string { return []string{addon} }),
	)

	sr := s.scanOneNoDB(context.Background(), "cb", "1.0.0", "18.0", cookbookDir)

	if sr.ErrorMessage == "" {
		t.Errorf("a genuine error (broken with AND without addons) must still be recorded as errored")
	}
}

// TestScan_AddonPathProblem_Surfaced proves a missing addon path is surfaced as
// a problem via addonScanInfo and does not abort the scan.
func TestScan_AddonPathProblem_Surfaced(t *testing.T) {
	cookbookDir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "nope.rb")

	fe := &fakeCookstyleExecutor{stdout: makeCleanJSON()}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe),
		WithCookstyleAddonCopPathsFn(func() []string { return []string{missing} }),
	)

	scanCtx := context.Background()
	_, _, _, info, _ := s.runScanWithAddonIsolation(scanCtx, cookbookDir, "18.0")
	if len(info.problems) != 1 {
		t.Fatalf("expected the missing addon path surfaced as one problem, got %v", info.problems)
	}
	if len(info.requires) != 0 {
		t.Errorf("a missing path resolves to no requires, got %v", info.requires)
	}
}
