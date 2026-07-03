// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// fakeCopRegistry is a static CopRegistryProvider for handler tests.
type fakeCopRegistry struct {
	reg *analysis.CopRegistry
	err error
}

func (f fakeCopRegistry) Registry(context.Context) (*analysis.CopRegistry, error) {
	return f.reg, f.err
}

// regEntry builds a registry entry with department/top-namespace derived from
// the cop name, matching the parser's behaviour.
func regEntry(name string) analysis.CopRegistryEntry {
	dept := ""
	top := name
	if i := lastSlash(name); i >= 0 {
		dept = name[:i]
	}
	if i := firstSlash(name); i >= 0 {
		top = name[:i]
	}
	return analysis.CopRegistryEntry{CopName: name, Department: dept, TopNamespace: top, Enabled: true}
}

func firstSlash(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// registryCoveringStatics builds a registry containing every static-table cop
// (so nothing is stale) minus the names in omit, plus the extra entries. The
// only enumerable static table is the verified-removal RemovedIn mapping, so
// covering it lets a test control drift precisely against the real mapping table.
func registryCoveringStatics(omit map[string]bool, extra ...analysis.CopRegistryEntry) *analysis.CopRegistry {
	var entries []analysis.CopRegistryEntry
	for _, m := range remediation.AllCopMappings() {
		if m.CopName != "" && !omit[m.CopName] {
			entries = append(entries, regEntry(m.CopName))
		}
	}
	entries = append(entries, extra...)
	return analysis.NewCopRegistry(entries, "test-8.6.10")
}

func getDrift(t *testing.T, r *Router) analysis.CopDriftReport {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookstyle/cop-drift?target_chef_version=18.0", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var report analysis.CopDriftReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal drift report: %v; body: %s", err, w.Body.String())
	}
	return report
}

func TestHandleCookstyleCopDrift_CoverageGaps(t *testing.T) {
	// A live Chef/* cop that resolves to the Review default (no RemovedIn, not
	// structural noise) is a coverage gap. A Chef/Style/* cop is covered — it
	// resolves to structural_noise, not review_default — so it is not a gap.
	reg := registryCoveringStatics(nil,
		regEntry("Chef/Modernize/ZzzGapCop"),
		regEntry("Chef/Style/ZzzCoveredCop"),
		regEntry("Style/ZzzGenericCop"), // generic Ruby — excluded from coverage
	)
	r := newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("18.0"))
	r.copRegistry = fakeCopRegistry{reg: reg}

	report := getDrift(t, r)
	if !report.RegistryAvailable {
		t.Fatal("registry_available = false, want true")
	}

	gaps := map[string]bool{}
	for _, g := range report.CoverageGaps {
		gaps[g.CopName] = true
	}
	if !gaps["Chef/Modernize/ZzzGapCop"] {
		t.Error("expected Chef/Modernize/ZzzGapCop in coverage gaps")
	}
	if gaps["Chef/Style/ZzzCoveredCop"] {
		t.Error("Chef/Style/ZzzCoveredCop resolves to structural_noise; should not be a gap")
	}
	if gaps["Style/ZzzGenericCop"] {
		t.Error("generic Ruby cop should not appear in coverage gaps")
	}
	// With every static cop present in the registry, nothing is stale.
	if len(report.Stale) != 0 {
		t.Errorf("expected no stale entries, got %+v", report.Stale)
	}
}

func TestHandleCookstyleCopDrift_Stale(t *testing.T) {
	// The verified-removal RemovedIn mapping is the only enumerable static table.
	// Drop one of its cops from the live registry: the binary no longer emits it,
	// so it must be reported stale against the mapping source.
	mappings := remediation.AllCopMappings()
	var dropped string
	for _, m := range mappings {
		if m.CopName != "" {
			dropped = m.CopName
			break
		}
	}
	if dropped == "" {
		t.Skip("no mapping cops to omit")
	}
	reg := registryCoveringStatics(map[string]bool{dropped: true})

	r := newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("18.0"))
	r.copRegistry = fakeCopRegistry{reg: reg}

	report := getDrift(t, r)
	var found *analysis.StaleCopEntry
	for i := range report.Stale {
		if report.Stale[i].CopName == dropped {
			found = &report.Stale[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("dropped mapping cop %q not reported stale: %+v", dropped, report.Stale)
	}
	if found.Source != analysis.StaticSourceMapping {
		t.Errorf("stale source = %q, want %q", found.Source, analysis.StaticSourceMapping)
	}
}

func TestHandleCookstyleCopDrift_RegistryUnavailable(t *testing.T) {
	r := newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("18.0"))
	// No copRegistry wired.
	report := getDrift(t, r)
	if report.RegistryAvailable {
		t.Error("registry_available = true with no provider, want false")
	}
	if len(report.Stale) != 0 || len(report.CoverageGaps) != 0 {
		t.Errorf("unavailable registry should yield no findings, got stale=%d gaps=%d", len(report.Stale), len(report.CoverageGaps))
	}
}

func TestHandleCookstyleCopDrift_RegistryError(t *testing.T) {
	r := newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("18.0"))
	r.copRegistry = fakeCopRegistry{err: errors.New("show-cops failed")}
	// A provider error degrades to registry_available=false with a 200, not a 500.
	report := getDrift(t, r)
	if report.RegistryAvailable {
		t.Error("registry_available = true on provider error, want false (graceful degrade)")
	}
}
