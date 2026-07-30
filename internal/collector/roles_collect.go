package collector

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
)

// roleSource is the subset of the Chef client used to collect role details. It
// exists so the collection strategy can be tested without a Chef server.
type roleSource interface {
	GetRoles(ctx context.Context) ([]string, error)
	CollectAllRoles(ctx context.Context, pageSize, concurrency int) ([]*chefapi.RoleDetail, error)
	GetRolesConcurrent(ctx context.Context, names []string, concurrency int) ([]*chefapi.RoleDetail, []error)
}

// roleCollection is the outcome of gathering role details for one organisation.
// The counts and errors exist so the caller can log what happened without
// re-deriving it; a degraded run is still a successful one.
type roleCollection struct {
	Roles        []*chefapi.RoleDetail
	FromIndex    int
	FromFallback int

	// ListErr is set when /roles could not be listed. Index results are still
	// usable, but gaps cannot be detected.
	ListErr error
	// IndexErr is set when the search index could not be used and the
	// collection fell back to per-role fetches.
	IndexErr error
	// FetchErrs holds per-role fetch failures. Roles that failed are absent
	// from Roles.
	FetchErrs []error
}

// collectRoleDetails gathers every role detail needed by BuildRoleDependencies.
//
// The search index is the primary path: one request per page instead of one per
// role, which at customer scale is ~74 requests rather than 73,910. The per-role
// GET remains as the fallback, used two ways — to fill roles the index did not
// return, and to carry the whole collection if the index fails outright. The
// index is an optimisation; /roles remains the authority on which roles exist.
//
// An error is returned only when neither path yields anything usable. Every
// other degradation is reported through the result so the caller can log it and
// still build a partial graph, which is what the per-role path did before.
func collectRoleDetails(ctx context.Context, src roleSource, pageSize, concurrency int) (roleCollection, error) {
	var result roleCollection

	names, listErr := src.GetRoles(ctx)
	if listErr != nil {
		result.ListErr = listErr
	}

	indexRoles, indexErr := src.CollectAllRoles(ctx, pageSize, concurrency)
	if indexErr != nil {
		result.IndexErr = indexErr

		// No index and no name list means there is nothing to work from.
		if listErr != nil {
			return result, fmt.Errorf("collector: no role source available: index: %w; list: %v", indexErr, listErr)
		}

		details, errs := src.GetRolesConcurrent(ctx, dedupeSorted(names), concurrency)
		result.Roles = details
		result.FromFallback = len(details)
		result.FetchErrs = errs
		return result, nil
	}

	seen := make(map[string]struct{}, len(indexRoles))
	for _, role := range indexRoles {
		if role == nil {
			continue
		}
		if _, dup := seen[role.Name]; dup {
			continue
		}
		seen[role.Name] = struct{}{}
		result.Roles = append(result.Roles, role)
	}
	result.FromIndex = len(result.Roles)

	// Without the name list, gaps cannot be detected — the index result stands
	// on its own.
	if listErr != nil {
		return result, nil
	}

	missing := make([]string, 0)
	for _, name := range dedupeSorted(names) {
		if _, present := seen[name]; !present {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return result, nil
	}

	details, errs := src.GetRolesConcurrent(ctx, missing, concurrency)
	for _, role := range details {
		if role == nil {
			continue
		}
		if _, dup := seen[role.Name]; dup {
			continue
		}
		seen[role.Name] = struct{}{}
		result.Roles = append(result.Roles, role)
		result.FromFallback++
	}
	result.FetchErrs = errs

	// Gap-filled roles are appended after the index results, so sort the whole
	// set: the dependency graph is replaced wholesale each run and should not
	// churn just because a role arrived by a different path.
	sort.Slice(result.Roles, func(i, j int) bool { return result.Roles[i].Name < result.Roles[j].Name })

	return result, nil
}

// dedupeSorted returns the unique names in sorted order so the gap-fill request
// set — and therefore the resulting graph — is identical between runs.
func dedupeSorted(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Warnings renders the non-fatal degradations as log-ready strings. Returns
// nothing when the collection was clean.
func (rc roleCollection) Warnings() []string {
	var out []string
	if rc.ListErr != nil {
		out = append(out, fmt.Sprintf("failed to list roles (gap detection disabled): %v", rc.ListErr))
	}
	if rc.IndexErr != nil {
		out = append(out, fmt.Sprintf("role search index unavailable, fell back to per-role fetch: %v", rc.IndexErr))
	}
	for _, err := range rc.FetchErrs {
		if errors.Is(err, context.Canceled) {
			continue
		}
		out = append(out, fmt.Sprintf("failed to fetch role: %v", err))
	}
	return out
}
