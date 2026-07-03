// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// GET /api/v1/cookstyle/cop-drift
//
// Reports classification-table drift against the live cookstyle binary:
//   - stale: curated/mapping entries for a cop the binary no longer emits;
//   - coverage_gaps: live Chef/* cops that resolve to unclassified.
//
// When the registry is unavailable (cookstyle not wired, or `--show-cops`
// failed) the report degrades to registry_available=false with no findings —
// this is a diagnostic surface, so an unavailable binary is a 200 with a clear
// flag, not a 500.
func (r *Router) handleCookstyleCopDrift(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	ctx := req.Context()

	targetVersion := queryString(req, "target_chef_version", "")
	if targetVersion == "" {
		targetVersion = r.defaultTargetVersion()
	}
	if targetVersion == "" {
		WriteBadRequest(w, "No target_chef_version specified and none configured.")
		return
	}

	resolver, err := r.copResolver(ctx, targetVersion)
	if err != nil {
		r.logf("ERROR", "loading cop classifications for drift: %v", err)
		WriteInternalError(w, "Failed to load cop classifications.")
		return
	}

	report := analysis.ComputeCopDrift(r.copRegistrySnapshot(ctx), resolver, staticCopSources())
	WriteJSON(w, http.StatusOK, report)
}

// copResolver builds a classification resolver for the target version, loading
// operator overrides from the datastore.
func (r *Router) copResolver(ctx context.Context, targetVersion string) (*analysis.CopClassificationResolver, error) {
	overrides, err := r.db.ListCopClassifications(ctx)
	if err != nil {
		return nil, err
	}
	overrideMap := make(map[string]string, len(overrides))
	for _, o := range overrides {
		overrideMap[o.CopName] = o.Classification
	}
	return &analysis.CopClassificationResolver{
		OperatorOverrides: overrideMap,
		TargetChefVersion: targetVersion,
	}, nil
}

// copRegistrySnapshot returns the live cop registry, or nil when no provider is
// wired or the load fails. A load failure is logged and treated as "registry
// unavailable" (non-fatal) so drift degrades gracefully.
func (r *Router) copRegistrySnapshot(ctx context.Context) *analysis.CopRegistry {
	if r.copRegistry == nil {
		return nil
	}
	reg, err := r.copRegistry.Registry(ctx)
	if err != nil {
		r.logf("WARN", "cop registry unavailable: %v", err)
		return nil
	}
	return reg
}

// staticCopSources assembles the names in the compiled static tables for
// stale-drift detection. The verified-removal RemovedIn mapping is the only
// enumerable table; the structural-Noise rules classify whole namespaces and
// have no concrete cop names to go stale.
func staticCopSources() []analysis.StaticCopSource {
	var out []analysis.StaticCopSource
	for _, m := range remediation.AllCopMappings() {
		if m.CopName != "" {
			out = append(out, analysis.StaticCopSource{CopName: m.CopName, Source: analysis.StaticSourceMapping})
		}
	}
	return out
}
