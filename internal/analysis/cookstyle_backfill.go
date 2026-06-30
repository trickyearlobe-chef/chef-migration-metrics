// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Precise CookStyle rollup-status backfill.
//
// Migrations 0041/0042 backfill cookstyle_status coarsely from the back-compat
// passed boolean (passed→ready else blocked) because SQL cannot evaluate cop
// classification — it can never recover needs_review, which requires the stored
// offences resolved through the classifier. This one-time Go pass re-derives the
// precise status for every result from its stored offences using the exact
// scan-time derivation (DeriveCookstyleStatus), so first boot shows the same
// value a rescan would, not an approximation.
//
// It is the unscoped sibling of the per-cop reclassification propagator
// (internal/webapi/cookstyle_propagation.go): same derivation, applied to every
// result rather than a single cop's closure. It is idempotent — re-deriving an
// already-precise row yields the same status, so a second run changes nothing.

// CookstyleStatusBackfillStore is the datastore subset the backfill needs.
// *datastore.DB satisfies it.
type CookstyleStatusBackfillStore interface {
	ClassificationOverrideLister
	ListAllServerCookbookCookstyleResultRefs(ctx context.Context) ([]datastore.CookstyleResultRef, error)
	ListAllGitRepoCookstyleResultRefs(ctx context.Context) ([]datastore.CookstyleResultRef, error)
	UpdateServerCookbookCookstyleVerdict(ctx context.Context, organisationName, cookbookName, cookbookVersion, targetChefVersion string, passed bool, status string) error
	UpdateGitRepoCookstyleVerdict(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string, passed bool, status string) error
}

// CookstyleStatusBackfillResult reports what the backfill examined and changed.
type CookstyleStatusBackfillResult struct {
	ServerResultsScanned int
	ServerResultsChanged int
	GitResultsScanned    int
	GitResultsChanged    int
}

// Changed reports the total number of rows whose status was corrected.
func (r CookstyleStatusBackfillResult) Changed() int {
	return r.ServerResultsChanged + r.GitResultsChanged
}

// BackfillCookstyleStatus re-derives the precise rollup status for every server
// and git cookstyle result from its stored offences, updating only the rows
// whose materialised status differs from the re-derivation. Inconclusive scans
// (error_message set) carry no verdict and are skipped. A resolver is built once
// per distinct target version (mirroring the complexity scorer's classifierCache)
// so operator overrides + RemovedIn + curated defaults load once per target, not
// once per row. rules is the live failure-rule fallback for unclassified cops.
func BackfillCookstyleStatus(ctx context.Context, store CookstyleStatusBackfillStore, rules CookstyleFailureRules) (CookstyleStatusBackfillResult, error) {
	var res CookstyleStatusBackfillResult
	if store == nil {
		return res, nil
	}

	serverRefs, err := store.ListAllServerCookbookCookstyleResultRefs(ctx)
	if err != nil {
		return res, fmt.Errorf("backfill: listing server cookstyle results: %w", err)
	}
	gitRefs, err := store.ListAllGitRepoCookstyleResultRefs(ctx)
	if err != nil {
		return res, fmt.Errorf("backfill: listing git cookstyle results: %w", err)
	}

	resolvers := map[string]*CopClassificationResolver{}
	resolverFor := func(target string) *CopClassificationResolver {
		if r, ok := resolvers[target]; ok {
			return r
		}
		r := NewResolverFromStore(ctx, store, target)
		resolvers[target] = r
		return r
	}

	for i := range serverRefs {
		ref := &serverRefs[i]
		res.ServerResultsScanned++
		if ref.ErrorMessage != "" {
			continue
		}
		status := DeriveCookstyleStatus(ParseStoredOffenses(ref.Offences), rules, resolverFor(ref.TargetChefVersion))
		if status == ref.CookstyleStatus {
			continue
		}
		if err := store.UpdateServerCookbookCookstyleVerdict(ctx, ref.OrganisationName, ref.CookbookName,
			ref.CookbookVersion, ref.TargetChefVersion, status != StatusBlocked, status); err != nil {
			return res, fmt.Errorf("backfill: updating server verdict for %s/%s@%s: %w",
				ref.OrganisationName, ref.CookbookName, ref.CookbookVersion, err)
		}
		res.ServerResultsChanged++
	}

	for i := range gitRefs {
		ref := &gitRefs[i]
		res.GitResultsScanned++
		if ref.ErrorMessage != "" {
			continue
		}
		status := DeriveCookstyleStatus(ParseStoredOffenses(ref.Offences), rules, resolverFor(ref.TargetChefVersion))
		if status == ref.CookstyleStatus {
			continue
		}
		if err := store.UpdateGitRepoCookstyleVerdict(ctx, ref.GitRepoName, ref.GitRepoURL,
			ref.TargetChefVersion, status != StatusBlocked, status); err != nil {
			return res, fmt.Errorf("backfill: updating git verdict for %s: %w", ref.GitRepoName, err)
		}
		res.GitResultsChanged++
	}

	return res, nil
}

// ParseStoredOffenses parses the offences JSONB persisted on a cookstyle result
// into the minimal shape the status derivation needs (cop name + severity). It
// accepts both the current flat enriched-offence array and the legacy
// file-grouped RuboCop format, mirroring the propagator's parser so the backfill
// and reclassification paths read stored offences identically. An empty or
// unparseable payload yields no offences (a clean scan → Ready).
func ParseStoredOffenses(data []byte) []CookstyleOffense {
	if len(data) == 0 {
		return nil
	}

	// Legacy file-grouped format: [{"path": ..., "offenses": [...]}, ...].
	type fileOffense struct {
		CopName  string `json:"cop_name"`
		Severity string `json:"severity"`
	}
	type fileEntry struct {
		Path     string        `json:"path"`
		Offenses []fileOffense `json:"offenses"`
	}
	var files []fileEntry
	if err := json.Unmarshal(data, &files); err == nil && len(files) > 0 && files[0].Path != "" {
		var out []CookstyleOffense
		for _, fe := range files {
			for _, o := range fe.Offenses {
				out = append(out, CookstyleOffense{CopName: o.CopName, Severity: o.Severity})
			}
		}
		return out
	}

	// Current flat enriched-offence array: [{"cop_name": ..., "severity": ...}, ...].
	var flat []CookstyleOffense
	if err := json.Unmarshal(data, &flat); err == nil {
		return flat
	}
	return nil
}
