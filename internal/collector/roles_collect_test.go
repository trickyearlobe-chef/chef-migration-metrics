package collector

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
)

// ---------------------------------------------------------------------------
// Test double
// ---------------------------------------------------------------------------

type fakeRoleSource struct {
	names    []string
	namesErr error

	indexRoles []*chefapi.RoleDetail
	indexErr   error

	// perRole is consulted by the fallback; a name absent from the map fails.
	perRole map[string]*chefapi.RoleDetail

	// Recorded calls.
	indexCalls    int
	fallbackNames []string
}

func (f *fakeRoleSource) GetRoles(ctx context.Context) ([]string, error) {
	if f.namesErr != nil {
		return nil, f.namesErr
	}
	return append([]string(nil), f.names...), nil
}

func (f *fakeRoleSource) CollectAllRoles(ctx context.Context, pageSize, concurrency int) ([]*chefapi.RoleDetail, error) {
	f.indexCalls++
	if f.indexErr != nil {
		return nil, f.indexErr
	}
	return f.indexRoles, nil
}

func (f *fakeRoleSource) GetRolesConcurrent(ctx context.Context, names []string, concurrency int) ([]*chefapi.RoleDetail, []error) {
	f.fallbackNames = append(f.fallbackNames, names...)

	var (
		details []*chefapi.RoleDetail
		errs    []error
	)
	for _, name := range names {
		role, ok := f.perRole[name]
		if !ok {
			errs = append(errs, fmt.Errorf("role %q not found", name))
			continue
		}
		details = append(details, role)
	}
	return details, errs
}

func role(name string, runList ...string) *chefapi.RoleDetail {
	return &chefapi.RoleDetail{Name: name, RunList: runList}
}

func roleNames(roles []*chefapi.RoleDetail) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		out = append(out, r.Name)
	}
	return out
}

// ---------------------------------------------------------------------------
// collectRoleDetails
// ---------------------------------------------------------------------------

// When the index answers in full, no per-role request is issued — one round trip
// saved per role.
func TestCollectRoleDetails_CompleteIndexIssuesNoPerRoleFetch(t *testing.T) {
	src := &fakeRoleSource{
		names:      []string{"base", "web", "db"},
		indexRoles: []*chefapi.RoleDetail{role("base"), role("web"), role("db")},
	}

	result, err := collectRoleDetails(context.Background(), src, 1000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(src.fallbackNames) != 0 {
		t.Errorf("expected no per-role fetches, got %v", src.fallbackNames)
	}
	if got := len(result.Roles); got != 3 {
		t.Fatalf("expected 3 roles, got %d", got)
	}
	if result.FromIndex != 3 || result.FromFallback != 0 {
		t.Errorf("expected 3 from index and 0 from fallback, got %d/%d", result.FromIndex, result.FromFallback)
	}
}

// A role in the org's role list but missing from the index must still reach the
// dependency graph — the index is an optimisation, not the authority on which
// roles exist.
func TestCollectRoleDetails_GapsAreFilledPerRole(t *testing.T) {
	src := &fakeRoleSource{
		names:      []string{"base", "web", "db"},
		indexRoles: []*chefapi.RoleDetail{role("base")},
		perRole: map[string]*chefapi.RoleDetail{
			"web": role("web"),
			"db":  role("db"),
		},
	}

	result, err := collectRoleDetails(context.Background(), src, 1000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sort.Strings(src.fallbackNames)
	if len(src.fallbackNames) != 2 || src.fallbackNames[0] != "db" || src.fallbackNames[1] != "web" {
		t.Errorf("expected only the missing roles to be fetched, got %v", src.fallbackNames)
	}

	got := roleNames(result.Roles)
	sort.Strings(got)
	if len(got) != 3 || got[0] != "base" || got[1] != "db" || got[2] != "web" {
		t.Errorf("expected all 3 roles, got %v", got)
	}
	if result.FromIndex != 1 || result.FromFallback != 2 {
		t.Errorf("expected 1 from index and 2 from fallback, got %d/%d", result.FromIndex, result.FromFallback)
	}
}

// The index is new and unproven at scale. If it fails outright the collection
// must degrade to the previous behaviour, not lose the dependency graph.
func TestCollectRoleDetails_IndexFailureFallsBackToPerRole(t *testing.T) {
	src := &fakeRoleSource{
		names:    []string{"base", "web"},
		indexErr: errors.New("search unavailable"),
		perRole: map[string]*chefapi.RoleDetail{
			"base": role("base"),
			"web":  role("web"),
		},
	}

	result, err := collectRoleDetails(context.Background(), src, 1000, 4)
	if err != nil {
		t.Fatalf("expected a degraded success, got error: %v", err)
	}
	if len(result.Roles) != 2 {
		t.Fatalf("expected both roles via fallback, got %v", roleNames(result.Roles))
	}
	if result.FromIndex != 0 || result.FromFallback != 2 {
		t.Errorf("expected 0 from index and 2 from fallback, got %d/%d", result.FromIndex, result.FromFallback)
	}
	if result.IndexErr == nil {
		t.Error("expected the index failure to be reported for logging")
	}
}

// A partial graph is more useful than none — the pre-existing per-role path
// logged and continued, and that must not regress.
func TestCollectRoleDetails_FallbackFailuresAreReportedNotFatal(t *testing.T) {
	src := &fakeRoleSource{
		names:      []string{"base", "web", "gone"},
		indexRoles: []*chefapi.RoleDetail{role("base")},
		perRole:    map[string]*chefapi.RoleDetail{"web": role("web")},
	}

	result, err := collectRoleDetails(context.Background(), src, 1000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Roles) != 2 {
		t.Errorf("expected the 2 fetchable roles, got %v", roleNames(result.Roles))
	}
	if len(result.FetchErrs) != 1 {
		t.Errorf("expected 1 reported fetch error, got %v", result.FetchErrs)
	}
}

// Without the name list there is no way to detect gaps, but the index result is
// still usable — better than skipping the dependency graph entirely, which is
// what the per-role path did when the list call failed.
func TestCollectRoleDetails_ListFailureStillUsesIndex(t *testing.T) {
	src := &fakeRoleSource{
		namesErr:   errors.New("list unavailable"),
		indexRoles: []*chefapi.RoleDetail{role("base"), role("web")},
	}

	result, err := collectRoleDetails(context.Background(), src, 1000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Roles) != 2 {
		t.Errorf("expected the index roles, got %v", roleNames(result.Roles))
	}
	if len(src.fallbackNames) != 0 {
		t.Errorf("no gap detection is possible without the name list, got fetches for %v", src.fallbackNames)
	}
	if result.ListErr == nil {
		t.Error("expected the list failure to be reported for logging")
	}
}

// Both paths down means there is nothing to build a graph from, and the caller
// must be able to tell that apart from an organisation with no roles.
func TestCollectRoleDetails_BothPathsFailingIsAnError(t *testing.T) {
	src := &fakeRoleSource{
		namesErr: errors.New("list unavailable"),
		indexErr: errors.New("search unavailable"),
	}

	if _, err := collectRoleDetails(context.Background(), src, 1000, 4); err == nil {
		t.Fatal("expected an error when neither the list nor the index is available")
	}
}

func TestCollectRoleDetails_NoRolesIsNotAnError(t *testing.T) {
	src := &fakeRoleSource{}

	result, err := collectRoleDetails(context.Background(), src, 1000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Roles) != 0 {
		t.Errorf("expected no roles, got %v", roleNames(result.Roles))
	}
	if len(src.fallbackNames) != 0 {
		t.Errorf("expected no per-role fetches, got %v", src.fallbackNames)
	}
}

// The index may return a role the /roles list does not (a race with a delete,
// or a stale list). Keep it — dropping it would shrink the graph for no reason.
func TestCollectRoleDetails_IndexExtrasAreKept(t *testing.T) {
	src := &fakeRoleSource{
		names:      []string{"base"},
		indexRoles: []*chefapi.RoleDetail{role("base"), role("extra")},
	}

	result, err := collectRoleDetails(context.Background(), src, 1000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Roles) != 2 {
		t.Errorf("expected both roles kept, got %v", roleNames(result.Roles))
	}
}

// Deterministic output keeps the dependency graph stable between runs, which
// matters because it is replaced wholesale each collection.
func TestCollectRoleDetails_GapFillIsDeterministic(t *testing.T) {
	newSrc := func() *fakeRoleSource {
		return &fakeRoleSource{
			names:      []string{"zeta", "alpha", "mid"},
			indexRoles: []*chefapi.RoleDetail{role("mid")},
			perRole: map[string]*chefapi.RoleDetail{
				"zeta":  role("zeta"),
				"alpha": role("alpha"),
			},
		}
	}

	first := newSrc()
	if _, err := collectRoleDetails(context.Background(), first, 1000, 4); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second := newSrc()
	if _, err := collectRoleDetails(context.Background(), second, 1000, 4); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(first.fallbackNames) != 2 {
		t.Fatalf("expected 2 gap fills, got %v", first.fallbackNames)
	}
	for i := range first.fallbackNames {
		if first.fallbackNames[i] != second.fallbackNames[i] {
			t.Fatalf("gap-fill order is not deterministic: %v vs %v", first.fallbackNames, second.fallbackNames)
		}
	}
	if first.fallbackNames[0] != "alpha" {
		t.Errorf("expected gap fills in name order, got %v", first.fallbackNames)
	}
}

// Duplicate names across index and fallback must not double up: the graph is
// keyed per role and BuildRoleDependencies would emit duplicate edges.
func TestCollectRoleDetails_NoDuplicateRoles(t *testing.T) {
	src := &fakeRoleSource{
		names:      []string{"base", "base", "web"},
		indexRoles: []*chefapi.RoleDetail{role("base")},
		perRole:    map[string]*chefapi.RoleDetail{"web": role("web")},
	}

	result, err := collectRoleDetails(context.Background(), src, 1000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	seen := map[string]int{}
	for _, r := range result.Roles {
		seen[r.Name]++
	}
	for name, n := range seen {
		if n != 1 {
			t.Errorf("role %q appears %d times", name, n)
		}
	}
}
