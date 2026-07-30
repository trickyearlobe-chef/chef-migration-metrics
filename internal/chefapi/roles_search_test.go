package chefapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// RoleSearchAttributes
// ---------------------------------------------------------------------------

func TestRoleSearchAttributes_RequestsDependencyGraphFields(t *testing.T) {
	attrs := RoleSearchAttributes()

	for _, key := range []string{"name", "run_list", "env_run_lists"} {
		path, ok := attrs[key]
		if !ok {
			t.Fatalf("expected attribute %q to be requested", key)
		}
		if len(path) != 1 || path[0] != key {
			t.Errorf("attribute %q: expected path [%q], got %v", key, key, path)
		}
	}
}

// Roles are a top-level Chef object, not a node — their fields are not under
// the merged attribute namespace, so the paths must stay unprefixed. This
// pins the shape against the kind of narrowing that emptied Windows
// filesystem data in v2.18.6.
func TestRoleSearchAttributes_PathsAreNotPrefixed(t *testing.T) {
	for key, path := range RoleSearchAttributes() {
		for _, seg := range path {
			if seg == "automatic" || seg == "default" || seg == "override" {
				t.Errorf("attribute %q path %v must not be prefixed with an attribute level", key, path)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// CollectAllRoles — decoding
// ---------------------------------------------------------------------------

func roleRow(name string, runList []string, envRunLists map[string][]string) SearchResultRow {
	data := map[string]interface{}{"name": name}
	if runList != nil {
		entries := make([]interface{}, len(runList))
		for i, e := range runList {
			entries[i] = e
		}
		data["run_list"] = entries
	}
	if envRunLists != nil {
		envs := make(map[string]interface{}, len(envRunLists))
		for env, entries := range envRunLists {
			converted := make([]interface{}, len(entries))
			for i, e := range entries {
				converted[i] = e
			}
			envs[env] = converted
		}
		data["env_run_lists"] = envs
	}
	return SearchResultRow{Data: data}
}

func serveRolePages(t *testing.T, pages map[string]SearchResult) (*Client, *int32Counter) {
	t.Helper()
	counter := &int32Counter{}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		counter.inc()
		start := r.URL.Query().Get("start")
		page, ok := pages[start]
		if !ok {
			w.WriteHeader(500)
			w.Write([]byte(fmt.Sprintf(`{"error":"unexpected start=%s"}`, start)))
			return
		}
		json.NewEncoder(w).Encode(page)
	})
	return client, counter
}

type int32Counter struct {
	mu sync.Mutex
	n  int
}

func (c *int32Counter) inc() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

func (c *int32Counter) value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

func TestCollectAllRoles_DecodesRunListAndEnvRunLists(t *testing.T) {
	client, _ := serveRolePages(t, map[string]SearchResult{
		"0": {Total: 1, Start: 0, Rows: []SearchResultRow{
			roleRow("base",
				[]string{"recipe[ntp::default]", "role[common]"},
				map[string][]string{"production": {"recipe[ntp::prod]"}},
			),
		}},
	})

	roles, err := client.CollectAllRoles(ctx(), 1000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(roles))
	}

	got := roles[0]
	if got.Name != "base" {
		t.Errorf("expected name %q, got %q", "base", got.Name)
	}
	if len(got.RunList) != 2 || got.RunList[0] != "recipe[ntp::default]" || got.RunList[1] != "role[common]" {
		t.Errorf("run_list not decoded: %v", got.RunList)
	}
	prod, ok := got.EnvRunLists["production"]
	if !ok {
		t.Fatalf("env_run_lists missing 'production': %v", got.EnvRunLists)
	}
	if len(prod) != 1 || prod[0] != "recipe[ntp::prod]" {
		t.Errorf("env_run_lists['production'] not decoded: %v", prod)
	}
}

// Ohai and Chef object shapes vary; a malformed row must not panic or poison
// the whole collection.
func TestCollectAllRoles_MalformedRowsAreSkippedNotFatal(t *testing.T) {
	client, _ := serveRolePages(t, map[string]SearchResult{
		"0": {Total: 6, Start: 0, Rows: []SearchResultRow{
			{Data: nil},                                    // no data at all
			{Data: map[string]interface{}{}},               // no name
			{Data: map[string]interface{}{"name": 42}},     // name wrong type
			{Data: map[string]interface{}{"name": ""}},     // empty name
			{Data: map[string]interface{}{                  // wrong-typed members
				"name":          "odd",
				"run_list":      "recipe[not-a-list]",
				"env_run_lists": []interface{}{"not-a-map"},
			}},
			roleRow("good", []string{"recipe[ok]"}, nil),
		}},
	})

	roles, err := client.CollectAllRoles(ctx(), 1000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byName := map[string]*RoleDetail{}
	for _, r := range roles {
		byName[r.Name] = r
	}
	if len(roles) != 2 {
		t.Fatalf("expected the 2 named roles, got %d: %v", len(roles), byName)
	}
	odd, ok := byName["odd"]
	if !ok {
		t.Fatal("expected the wrong-typed role to survive with empty members")
	}
	if len(odd.RunList) != 0 {
		t.Errorf("expected empty run_list for wrong-typed value, got %v", odd.RunList)
	}
	if len(odd.EnvRunLists) != 0 {
		t.Errorf("expected empty env_run_lists for wrong-typed value, got %v", odd.EnvRunLists)
	}
	if byName["good"] == nil || len(byName["good"].RunList) != 1 {
		t.Errorf("well-formed role was not decoded: %v", byName["good"])
	}
}

// Individual non-string entries inside an otherwise valid run_list are dropped
// rather than discarding the whole role.
func TestCollectAllRoles_NonStringRunListEntriesDropped(t *testing.T) {
	client, _ := serveRolePages(t, map[string]SearchResult{
		"0": {Total: 1, Start: 0, Rows: []SearchResultRow{
			{Data: map[string]interface{}{
				"name":     "mixed",
				"run_list": []interface{}{"recipe[a]", 7, nil, "role[b]"},
				"env_run_lists": map[string]interface{}{
					"prod":   []interface{}{"recipe[c]", false},
					"broken": "not-a-list",
				},
			}},
		}},
	})

	roles, err := client.CollectAllRoles(ctx(), 1000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(roles))
	}
	if len(roles[0].RunList) != 2 {
		t.Errorf("expected 2 usable run_list entries, got %v", roles[0].RunList)
	}
	if len(roles[0].EnvRunLists["prod"]) != 1 {
		t.Errorf("expected 1 usable prod entry, got %v", roles[0].EnvRunLists["prod"])
	}
	if _, present := roles[0].EnvRunLists["broken"]; present {
		t.Errorf("wrong-typed env run list should be dropped, got %v", roles[0].EnvRunLists)
	}
}

// ---------------------------------------------------------------------------
// CollectAllRoles — pagination
// ---------------------------------------------------------------------------

func TestCollectAllRoles_MultiplePagesAssembledInOrder(t *testing.T) {
	client, _ := serveRolePages(t, map[string]SearchResult{
		"0": {Total: 6, Start: 0, Rows: []SearchResultRow{roleRow("r1", nil, nil), roleRow("r2", nil, nil)}},
		"2": {Total: 6, Start: 2, Rows: []SearchResultRow{roleRow("r3", nil, nil), roleRow("r4", nil, nil)}},
		"4": {Total: 6, Start: 4, Rows: []SearchResultRow{roleRow("r5", nil, nil), roleRow("r6", nil, nil)}},
	})

	roles, err := client.CollectAllRoles(ctx(), 2, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 6 {
		t.Fatalf("expected 6 roles, got %d", len(roles))
	}
	for i, want := range []string{"r1", "r2", "r3", "r4", "r5", "r6"} {
		if roles[i].Name != want {
			t.Errorf("roles[%d]: expected %q, got %q", i, want, roles[i].Name)
		}
	}
}

// The role index may enforce a lower `rows` cap than the node index. If
// pagination advanced by the *requested* page size rather than the number of
// rows actually returned, every role beyond the cap in each page would be
// silently skipped.
func TestCollectAllRoles_ServerCapsPageSizeBelowRequest(t *testing.T) {
	const cap = 2
	client, _ := serveRolePages(t, map[string]SearchResult{
		"0": {Total: 5, Start: 0, Rows: []SearchResultRow{roleRow("r1", nil, nil), roleRow("r2", nil, nil)}},
		"2": {Total: 5, Start: 2, Rows: []SearchResultRow{roleRow("r3", nil, nil), roleRow("r4", nil, nil)}},
		"4": {Total: 5, Start: 4, Rows: []SearchResultRow{roleRow("r5", nil, nil)}},
	})

	// Ask for 1000 per page; the server only ever returns `cap`.
	roles, err := client.CollectAllRoles(ctx(), 1000, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 5 {
		t.Fatalf("expected all 5 roles despite the server cap of %d, got %d", cap, len(roles))
	}
}

// Pagination-boundary duplication is known to occur on the node index
// (cf. deduplicateSnapshotParams). Assume the role index can do the same.
func TestCollectAllRoles_DeduplicatesByName(t *testing.T) {
	client, _ := serveRolePages(t, map[string]SearchResult{
		"0": {Total: 4, Start: 0, Rows: []SearchResultRow{roleRow("r1", nil, nil), roleRow("r2", nil, nil)}},
		"2": {Total: 4, Start: 2, Rows: []SearchResultRow{roleRow("r2", nil, nil), roleRow("r3", nil, nil)}},
	})

	roles, err := client.CollectAllRoles(ctx(), 2, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 3 {
		t.Fatalf("expected 3 unique roles, got %d", len(roles))
	}
	seen := map[string]int{}
	for _, r := range roles {
		seen[r.Name]++
	}
	for name, n := range seen {
		if n != 1 {
			t.Errorf("role %q appears %d times", name, n)
		}
	}
}

// A page that returns no rows must not spin forever, and must not be treated
// as a hard failure — the caller fills gaps via the per-role fallback.
func TestCollectAllRoles_EmptyPageTerminates(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		client, counter := serveRolePages(t, map[string]SearchResult{
			"0": {Total: 100, Start: 0, Rows: []SearchResultRow{roleRow("r1", nil, nil)}},
			"1": {Total: 100, Start: 1, Rows: []SearchResultRow{}},
		})
		roles, err := client.CollectAllRoles(ctx(), 1, 1)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(roles) == 0 {
			t.Errorf("expected the roles fetched before the empty page to be returned")
		}
		if counter.value() > 100 {
			t.Errorf("made %d requests — pagination did not terminate", counter.value())
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("CollectAllRoles did not terminate on an empty page")
	}
}

func TestCollectAllRoles_EmptyIndex(t *testing.T) {
	client, _ := serveRolePages(t, map[string]SearchResult{
		"0": {Total: 0, Start: 0, Rows: []SearchResultRow{}},
	})

	roles, err := client.CollectAllRoles(ctx(), 1000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 0 {
		t.Errorf("expected no roles, got %d", len(roles))
	}
}

func TestCollectAllRoles_SinglePageMakesOneRequest(t *testing.T) {
	client, counter := serveRolePages(t, map[string]SearchResult{
		"0": {Total: 2, Start: 0, Rows: []SearchResultRow{roleRow("r1", nil, nil), roleRow("r2", nil, nil)}},
	})

	if _, err := client.CollectAllRoles(ctx(), 1000, 8); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counter.value() != 1 {
		t.Errorf("expected 1 request for a single page, got %d", counter.value())
	}
}

// ---------------------------------------------------------------------------
// CollectAllRoles — failure modes
// ---------------------------------------------------------------------------

func TestCollectAllRoles_FirstPageErrorIsReturned(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"boom"}`))
	})

	if _, err := client.CollectAllRoles(ctx(), 1000, 4); err == nil {
		t.Fatal("expected an error when the first page fails")
	}
}

func TestCollectAllRoles_SubsequentPageErrorIsReturned(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("start") == "0" {
			json.NewEncoder(w).Encode(SearchResult{Total: 4, Start: 0, Rows: []SearchResultRow{
				roleRow("r1", nil, nil), roleRow("r2", nil, nil),
			}})
			return
		}
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"boom"}`))
	})

	if _, err := client.CollectAllRoles(ctx(), 2, 2); err == nil {
		t.Fatal("expected an error when a subsequent page fails")
	}
}

func TestCollectAllRoles_ContextCancelled(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("start") == "0" {
			json.NewEncoder(w).Encode(SearchResult{Total: 100, Start: 0, Rows: []SearchResultRow{
				roleRow("r1", nil, nil),
			}})
			return
		}
		time.Sleep(5 * time.Second)
		w.WriteHeader(200)
	})

	cancelCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if _, err := client.CollectAllRoles(cancelCtx, 1, 5); err == nil {
		t.Fatal("expected an error when the context is cancelled")
	}
}

func TestCollectAllRoles_QueriesTheRoleIndex(t *testing.T) {
	var gotPath string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(SearchResult{Total: 0, Start: 0, Rows: []SearchResultRow{}})
	})

	if _, err := client.CollectAllRoles(ctx(), 1000, 4); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "/organizations/testorg/search/role"; gotPath != want {
		t.Errorf("expected the role index at %q, got %q", want, gotPath)
	}
}
