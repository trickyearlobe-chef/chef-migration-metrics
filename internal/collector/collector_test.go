// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// ---------------------------------------------------------------------------
// mockDB implements the subset of *datastore.DB methods used by the
// checkpoint/resume logic. Since we can't construct a real *datastore.DB
// without a PostgreSQL connection, we test the pure logic functions and
// the estimateCollectionInterval helper directly.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Test helpers — mock client factory
// ---------------------------------------------------------------------------

// mockSearchRow builds a SearchResultRow with the given node data fields.
func mockSearchRow(name, env, chefVersion, platform string, cookbooks map[string]string) chefapi.SearchResultRow {
	data := map[string]interface{}{
		"name":             name,
		"chef_environment": env,
		"chef_version":     chefVersion,
		"platform":         platform,
		"platform_version": "22.04",
		"platform_family":  "debian",
		"ohai_time":        float64(time.Now().Unix()), // recent — not stale
	}

	if cookbooks != nil {
		cbMap := make(map[string]interface{}, len(cookbooks))
		for cbName, ver := range cookbooks {
			cbMap[cbName] = map[string]interface{}{"version": ver}
		}
		data["cookbooks"] = cbMap
	}

	return chefapi.SearchResultRow{
		URL:  fmt.Sprintf("https://chef.example.com/nodes/%s", name),
		Data: data,
	}
}

// mockCookbookList builds a cookbook list response like GetCookbooks returns.
func mockCookbookList(names map[string][]string) map[string]chefapi.CookbookListEntry {
	result := make(map[string]chefapi.CookbookListEntry, len(names))
	for name, versions := range names {
		entry := chefapi.CookbookListEntry{
			URL: fmt.Sprintf("https://chef.example.com/cookbooks/%s", name),
		}
		for _, v := range versions {
			entry.Versions = append(entry.Versions, chefapi.CookbookVersionEntry{
				URL:     fmt.Sprintf("https://chef.example.com/cookbooks/%s/%s", name, v),
				Version: v,
			})
		}
		result[name] = entry
	}
	return result
}

// testCollector creates a Collector with a mock client factory that intercepts
// the collectOrganisation call at the Chef API level. Since we can't easily
// mock chefapi.Client (it requires a real RSA key), we test at the Collector
// level by verifying the orchestration logic with a factory that returns
// errors or by using the lower-level unit tests.
//
// For integration-style tests, we provide a factory that generates a real
// client from a test RSA key — but that's complex. Instead, we test the
// collector's orchestration patterns: single-run enforcement, error isolation,
// parallel execution, etc.

func newTestCollector(t *testing.T, factory ClientFactory) (*Collector, *logging.MemoryWriter) {
	t.Helper()
	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{memWriter},
	})

	cfg := &config.Config{}
	cfg.Collection.StaleNodeThresholdDays = 7
	cfg.Collection.StaleCookbookThresholdDays = 365
	cfg.Concurrency.OrganisationCollection = 5
	cfg.Concurrency.NodePageFetching = 3

	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver, WithClientFactory(factory))
	return c, memWriter
}

// ---------------------------------------------------------------------------
// Collector — construction
// ---------------------------------------------------------------------------

func TestNew_ReturnsNonNil(t *testing.T) {
	c, _ := newTestCollector(t, func(ctx context.Context, org datastore.Organisation) (*chefapi.Client, error) {
		return nil, nil
	})
	if c == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_DefaultClientFactory(t *testing.T) {
	// When no WithClientFactory is provided, the default factory is used.
	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{memWriter},
	})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver)
	if c.clientFactory == nil {
		t.Fatal("clientFactory should not be nil")
	}
}

func TestWithClientFactory_OverridesDefault(t *testing.T) {
	called := false
	factory := func(ctx context.Context, org datastore.Organisation) (*chefapi.Client, error) {
		called = true
		return nil, fmt.Errorf("test")
	}

	c, _ := newTestCollector(t, factory)
	_, _ = c.clientFactory(context.Background(), datastore.Organisation{Name: "test"})
	if !called {
		t.Error("custom factory was not called")
	}
}

func TestWithClientFactory_NilIsIgnored(t *testing.T) {
	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{memWriter},
	})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver, WithClientFactory(nil))
	// Should still have the default factory.
	if c.clientFactory == nil {
		t.Fatal("clientFactory should not be nil when WithClientFactory(nil) is passed")
	}
}

// ---------------------------------------------------------------------------
// Collector — single-run enforcement
// ---------------------------------------------------------------------------

func TestCollector_IsRunning_InitiallyFalse(t *testing.T) {
	c, _ := newTestCollector(t, nil)
	if c.IsRunning() {
		t.Error("expected IsRunning() to be false initially")
	}
}

func TestCollector_TryStartRun_SetsRunning(t *testing.T) {
	c, _ := newTestCollector(t, nil)

	if !c.tryStartRun() {
		t.Fatal("first tryStartRun should succeed")
	}
	defer c.finishRun()

	if !c.IsRunning() {
		t.Error("expected IsRunning() to be true after tryStartRun")
	}
}

func TestCollector_TryStartRun_SecondCallFails(t *testing.T) {
	c, _ := newTestCollector(t, nil)

	if !c.tryStartRun() {
		t.Fatal("first tryStartRun should succeed")
	}
	defer c.finishRun()

	if c.tryStartRun() {
		t.Error("second tryStartRun should fail")
	}
}

func TestCollector_FinishRun_ClearsRunning(t *testing.T) {
	c, _ := newTestCollector(t, nil)

	c.tryStartRun()
	c.finishRun()

	if c.IsRunning() {
		t.Error("expected IsRunning() to be false after finishRun")
	}
}

func TestCollector_TryStartRun_AfterFinish_Succeeds(t *testing.T) {
	c, _ := newTestCollector(t, nil)

	c.tryStartRun()
	c.finishRun()

	if !c.tryStartRun() {
		t.Error("tryStartRun after finishRun should succeed")
	}
	c.finishRun()
}

func TestCollector_ConcurrentTryStartRun(t *testing.T) {
	c, _ := newTestCollector(t, nil)
	defer c.finishRun()

	// Race multiple goroutines trying to start a run.
	const goroutines = 100
	var succeeded int32
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.tryStartRun() {
				atomic.AddInt32(&succeeded, 1)
			}
		}()
	}

	wg.Wait()

	if succeeded != 1 {
		t.Errorf("expected exactly 1 goroutine to start the run, got %d", succeeded)
	}
}

// ---------------------------------------------------------------------------
// Collector.Run — error when already running
// ---------------------------------------------------------------------------

func TestCollector_Run_ErrorWhenAlreadyRunning(t *testing.T) {
	c, _ := newTestCollector(t, nil)

	// Manually set as running.
	c.tryStartRun()

	_, err := c.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when run is already in progress")
	}
	if err.Error() != "collector: a collection run is already in progress" {
		t.Errorf("unexpected error message: %v", err)
	}

	c.finishRun()
}

// ---------------------------------------------------------------------------
// Collector.Run — no datastore (nil db)
// ---------------------------------------------------------------------------

func TestCollector_Run_NilDB_ReturnsError(t *testing.T) {
	c, _ := newTestCollector(t, nil)

	// db is nil, so ListOrganisations will panic or return error.
	// We expect a panic recovery or error.
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected — nil db causes a nil pointer dereference.
				// This confirms the collector attempts to use the db.
				t.Log("recovered from expected nil DB panic")
			}
		}()
		_, _ = c.Run(context.Background())
	}()
}

// ---------------------------------------------------------------------------
// RunResult — zero value
// ---------------------------------------------------------------------------

func TestRunResult_ZeroValue(t *testing.T) {
	r := RunResult{}
	if r.TotalOrgs != 0 {
		t.Errorf("TotalOrgs: got %d, want 0", r.TotalOrgs)
	}
	if r.SucceededOrgs != 0 {
		t.Errorf("SucceededOrgs: got %d, want 0", r.SucceededOrgs)
	}
	if r.FailedOrgs != 0 {
		t.Errorf("FailedOrgs: got %d, want 0", r.FailedOrgs)
	}
	if r.TotalNodes != 0 {
		t.Errorf("TotalNodes: got %d, want 0", r.TotalNodes)
	}
	if r.TotalCookbooks != 0 {
		t.Errorf("TotalCookbooks: got %d, want 0", r.TotalCookbooks)
	}
}

// ---------------------------------------------------------------------------
// ClientFactory — error propagation
// ---------------------------------------------------------------------------

func TestClientFactory_ErrorPropagates(t *testing.T) {
	expectedErr := errors.New("credential resolution failed")
	factory := func(ctx context.Context, org datastore.Organisation) (*chefapi.Client, error) {
		return nil, expectedErr
	}

	c, _ := newTestCollector(t, factory)
	_, err := c.clientFactory(context.Background(), datastore.Organisation{Name: "test-org"})
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestClientFactory_ContextCancellation(t *testing.T) {
	factory := func(ctx context.Context, org datastore.Organisation) (*chefapi.Client, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return nil, nil
		}
	}

	c, _ := newTestCollector(t, factory)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.clientFactory(ctx, datastore.Organisation{Name: "test"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mock search row helpers
// ---------------------------------------------------------------------------

func TestMockSearchRow_BasicFields(t *testing.T) {
	row := mockSearchRow("node1", "production", "18.4.2", "ubuntu", map[string]string{
		"apache2": "5.0.1",
		"nginx":   "3.2.0",
	})

	if row.URL == "" {
		t.Error("URL should not be empty")
	}

	nd := chefapi.NewNodeData(row.Data)
	if nd.Name() != "node1" {
		t.Errorf("Name = %q, want %q", nd.Name(), "node1")
	}
	if nd.ChefEnvironment() != "production" {
		t.Errorf("ChefEnvironment = %q, want %q", nd.ChefEnvironment(), "production")
	}
	if nd.ChefVersion() != "18.4.2" {
		t.Errorf("ChefVersion = %q, want %q", nd.ChefVersion(), "18.4.2")
	}
	if nd.Platform() != "ubuntu" {
		t.Errorf("Platform = %q, want %q", nd.Platform(), "ubuntu")
	}

	cbVersions := nd.CookbookVersions()
	if len(cbVersions) != 2 {
		t.Fatalf("expected 2 cookbooks, got %d", len(cbVersions))
	}
	if cbVersions["apache2"] != "5.0.1" {
		t.Errorf("apache2 version = %q, want %q", cbVersions["apache2"], "5.0.1")
	}
	if cbVersions["nginx"] != "3.2.0" {
		t.Errorf("nginx version = %q, want %q", cbVersions["nginx"], "3.2.0")
	}
}

func TestMockSearchRow_NilCookbooks(t *testing.T) {
	row := mockSearchRow("node1", "dev", "17.0.0", "centos", nil)
	nd := chefapi.NewNodeData(row.Data)

	cbVersions := nd.CookbookVersions()
	if cbVersions != nil {
		t.Errorf("expected nil cookbooks, got %v", cbVersions)
	}
}

func TestMockSearchRow_OhaiTimeIsRecent(t *testing.T) {
	row := mockSearchRow("node1", "prod", "18.0.0", "ubuntu", nil)
	nd := chefapi.NewNodeData(row.Data)

	if nd.IsStale(7 * 24 * time.Hour) {
		t.Error("node should not be stale — ohai_time is set to now")
	}
}

func TestMockSearchRow_EmptyCookbooks(t *testing.T) {
	row := mockSearchRow("node1", "prod", "18.0.0", "ubuntu", map[string]string{})
	nd := chefapi.NewNodeData(row.Data)

	cbVersions := nd.CookbookVersions()
	if len(cbVersions) != 0 {
		t.Errorf("expected 0 cookbooks, got %d", len(cbVersions))
	}
}

// ---------------------------------------------------------------------------
// Mock cookbook list helpers
// ---------------------------------------------------------------------------

func TestMockCookbookList_BasicStructure(t *testing.T) {
	list := mockCookbookList(map[string][]string{
		"apache2": {"5.0.1", "4.3.2"},
		"nginx":   {"3.2.0"},
	})

	if len(list) != 2 {
		t.Fatalf("expected 2 cookbooks, got %d", len(list))
	}

	apache := list["apache2"]
	if len(apache.Versions) != 2 {
		t.Errorf("apache2: expected 2 versions, got %d", len(apache.Versions))
	}
	if apache.Versions[0].Version != "5.0.1" {
		t.Errorf("apache2 version 0 = %q, want %q", apache.Versions[0].Version, "5.0.1")
	}

	nginx := list["nginx"]
	if len(nginx.Versions) != 1 {
		t.Errorf("nginx: expected 1 version, got %d", len(nginx.Versions))
	}
}

func TestMockCookbookList_Empty(t *testing.T) {
	list := mockCookbookList(map[string][]string{})
	if len(list) != 0 {
		t.Errorf("expected 0 cookbooks, got %d", len(list))
	}
}

// ---------------------------------------------------------------------------
// Node data → snapshot params conversion logic
// ---------------------------------------------------------------------------

func TestNodeDataToSnapshotParams_JSONMarshalling(t *testing.T) {
	// Verify that the JSON marshalling of node data fields produces valid
	// JSON that can be stored in the datastore's JSONB columns.
	row := mockSearchRow("web-01", "staging", "18.4.0", "ubuntu", map[string]string{
		"apache2": "5.0.1",
	})
	nd := chefapi.NewNodeData(row.Data)

	fsJSON, err := json.Marshal(nd.Filesystem())
	if err != nil {
		t.Fatalf("marshalling filesystem: %v", err)
	}
	// Filesystem is nil in our mock, so it should marshal to "null".
	if string(fsJSON) != "null" {
		t.Errorf("filesystem JSON = %s, want null", fsJSON)
	}

	cbJSON, err := json.Marshal(nd.Cookbooks())
	if err != nil {
		t.Fatalf("marshalling cookbooks: %v", err)
	}
	// Should be a valid JSON object.
	var cbMap map[string]interface{}
	if err := json.Unmarshal(cbJSON, &cbMap); err != nil {
		t.Fatalf("cookbooks JSON is not valid: %v (raw: %s)", err, cbJSON)
	}
	if len(cbMap) != 1 {
		t.Errorf("expected 1 cookbook in JSON, got %d", len(cbMap))
	}

	rlJSON, err := json.Marshal(nd.RunList())
	if err != nil {
		t.Fatalf("marshalling run_list: %v", err)
	}
	// RunList is nil in our mock.
	if string(rlJSON) != "null" {
		t.Errorf("run_list JSON = %s, want null", rlJSON)
	}

	rolesJSON, err := json.Marshal(nd.Roles())
	if err != nil {
		t.Fatalf("marshalling roles: %v", err)
	}
	if string(rolesJSON) != "null" {
		t.Errorf("roles JSON = %s, want null", rolesJSON)
	}
}

// ---------------------------------------------------------------------------
// Active cookbook tracking logic
// ---------------------------------------------------------------------------

func TestActiveCookbookTracking(t *testing.T) {
	// Simulate the active cookbook tracking logic from collectOrganisation.
	rows := []chefapi.SearchResultRow{
		mockSearchRow("web-01", "prod", "18.0.0", "ubuntu", map[string]string{
			"apache2": "5.0.1",
			"openssl": "8.0.0",
		}),
		mockSearchRow("web-02", "prod", "18.0.0", "ubuntu", map[string]string{
			"apache2": "5.0.1",
			"mysql":   "10.0.0",
		}),
		mockSearchRow("db-01", "prod", "18.0.0", "centos", map[string]string{
			"mysql":   "10.0.0",
			"openssl": "8.0.0",
		}),
	}

	activeCookbookNames := make(map[string]bool)
	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		for cbName := range nd.CookbookVersions() {
			activeCookbookNames[cbName] = true
		}
	}

	// Should have 3 unique cookbook names.
	if len(activeCookbookNames) != 3 {
		t.Errorf("expected 3 active cookbooks, got %d", len(activeCookbookNames))
	}

	for _, name := range []string{"apache2", "openssl", "mysql"} {
		if !activeCookbookNames[name] {
			t.Errorf("expected %q to be active", name)
		}
	}

	// A cookbook on the server but not used by any node should not be active.
	if activeCookbookNames["unused-cookbook"] {
		t.Error("unused-cookbook should not be in the active set")
	}
}

func TestActiveCookbookTracking_NoNodes(t *testing.T) {
	activeCookbookNames := make(map[string]bool)
	// No nodes — no active cookbooks.
	if len(activeCookbookNames) != 0 {
		t.Errorf("expected 0 active cookbooks, got %d", len(activeCookbookNames))
	}
}

// ---------------------------------------------------------------------------
// Stale-node cookbook filtering
// ---------------------------------------------------------------------------

// TestActiveCookbookTracking_StaleNodesExcluded verifies that cookbooks
// referenced only by stale nodes are excluded from activeCookbookNames
// but still appear in allCookbookNames.
func TestActiveCookbookTracking_StaleNodesExcluded(t *testing.T) {
	staleThreshold := 7 * 24 * time.Hour

	// web-01: active node with apache2 + openssl
	activeRow := mockSearchRow("web-01", "prod", "18.0.0", "ubuntu", map[string]string{
		"apache2": "5.0.1",
		"openssl": "8.0.0",
	})

	// db-01: stale node (30 days old) with mysql + openssl
	staleRow := mockSearchRow("db-01", "prod", "18.0.0", "centos", map[string]string{
		"mysql":   "10.0.0",
		"openssl": "8.0.0",
	})
	staleRow.Data["ohai_time"] = float64(time.Now().Add(-30 * 24 * time.Hour).Unix())

	rows := []chefapi.SearchResultRow{activeRow, staleRow}

	allCookbookNames := make(map[string]bool)
	activeCookbookNames := make(map[string]bool)

	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		nodeIsStale := nd.IsStale(staleThreshold)
		for cbName := range nd.CookbookVersions() {
			allCookbookNames[cbName] = true
			if !nodeIsStale {
				activeCookbookNames[cbName] = true
			}
		}
	}

	// allCookbookNames should contain all 3 unique cookbooks.
	if len(allCookbookNames) != 3 {
		t.Errorf("expected 3 total cookbooks, got %d", len(allCookbookNames))
	}
	for _, name := range []string{"apache2", "openssl", "mysql"} {
		if !allCookbookNames[name] {
			t.Errorf("expected %q in allCookbookNames", name)
		}
	}

	// activeCookbookNames should only contain cookbooks from the active node.
	// openssl is used by both active and stale, so it stays active.
	// mysql is only used by the stale node, so it should be excluded.
	if len(activeCookbookNames) != 2 {
		t.Errorf("expected 2 active cookbooks, got %d: %v", len(activeCookbookNames), activeCookbookNames)
	}
	if !activeCookbookNames["apache2"] {
		t.Error("expected apache2 to be active (used by non-stale node)")
	}
	if !activeCookbookNames["openssl"] {
		t.Error("expected openssl to be active (used by non-stale node)")
	}
	if activeCookbookNames["mysql"] {
		t.Error("mysql should NOT be active — only used by stale node")
	}
}

// TestActiveCookbookTracking_AllNodesStale verifies that when every node
// is stale, activeCookbookNames is empty while allCookbookNames still
// records the full inventory.
func TestActiveCookbookTracking_AllNodesStale(t *testing.T) {
	staleThreshold := 7 * 24 * time.Hour
	thirtyDaysAgo := float64(time.Now().Add(-30 * 24 * time.Hour).Unix())

	rows := []chefapi.SearchResultRow{
		mockSearchRow("stale-01", "prod", "18.0.0", "ubuntu", map[string]string{
			"apache2": "5.0.1",
			"openssl": "8.0.0",
		}),
		mockSearchRow("stale-02", "prod", "18.0.0", "centos", map[string]string{
			"mysql":   "10.0.0",
			"openssl": "8.0.0",
		}),
	}
	// Make both nodes stale.
	rows[0].Data["ohai_time"] = thirtyDaysAgo
	rows[1].Data["ohai_time"] = thirtyDaysAgo

	allCookbookNames := make(map[string]bool)
	activeCookbookNames := make(map[string]bool)

	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		nodeIsStale := nd.IsStale(staleThreshold)
		for cbName := range nd.CookbookVersions() {
			allCookbookNames[cbName] = true
			if !nodeIsStale {
				activeCookbookNames[cbName] = true
			}
		}
	}

	if len(allCookbookNames) != 3 {
		t.Errorf("expected 3 total cookbooks, got %d", len(allCookbookNames))
	}
	if len(activeCookbookNames) != 0 {
		t.Errorf("expected 0 active cookbooks when all nodes are stale, got %d: %v",
			len(activeCookbookNames), activeCookbookNames)
	}
}

// TestActiveCookbookTracking_NoStaleNodes verifies that when no nodes are
// stale, allCookbookNames and activeCookbookNames are identical.
func TestActiveCookbookTracking_NoStaleNodes(t *testing.T) {
	staleThreshold := 7 * 24 * time.Hour

	rows := []chefapi.SearchResultRow{
		mockSearchRow("active-01", "prod", "18.0.0", "ubuntu", map[string]string{
			"apache2": "5.0.1",
			"openssl": "8.0.0",
		}),
		mockSearchRow("active-02", "prod", "18.0.0", "centos", map[string]string{
			"mysql":   "10.0.0",
			"openssl": "8.0.0",
		}),
	}

	allCookbookNames := make(map[string]bool)
	activeCookbookNames := make(map[string]bool)

	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		nodeIsStale := nd.IsStale(staleThreshold)
		for cbName := range nd.CookbookVersions() {
			allCookbookNames[cbName] = true
			if !nodeIsStale {
				activeCookbookNames[cbName] = true
			}
		}
	}

	if len(allCookbookNames) != len(activeCookbookNames) {
		t.Errorf("with no stale nodes, allCookbookNames (%d) and activeCookbookNames (%d) should match",
			len(allCookbookNames), len(activeCookbookNames))
	}
	for name := range allCookbookNames {
		if !activeCookbookNames[name] {
			t.Errorf("expected %q to be in activeCookbookNames when no nodes are stale", name)
		}
	}
}

// TestActiveCookbookTracking_SharedCookbookStaysActive verifies that a
// cookbook used by both stale and active nodes remains in the active set.
func TestActiveCookbookTracking_SharedCookbookStaysActive(t *testing.T) {
	staleThreshold := 7 * 24 * time.Hour

	activeRow := mockSearchRow("active-01", "prod", "18.0.0", "ubuntu", map[string]string{
		"shared": "1.0.0",
	})
	staleRow := mockSearchRow("stale-01", "prod", "18.0.0", "centos", map[string]string{
		"shared":     "1.0.0",
		"stale-only": "2.0.0",
	})
	staleRow.Data["ohai_time"] = float64(time.Now().Add(-30 * 24 * time.Hour).Unix())

	allCookbookNames := make(map[string]bool)
	activeCookbookNames := make(map[string]bool)

	for _, row := range []chefapi.SearchResultRow{activeRow, staleRow} {
		nd := chefapi.NewNodeData(row.Data)
		nodeIsStale := nd.IsStale(staleThreshold)
		for cbName := range nd.CookbookVersions() {
			allCookbookNames[cbName] = true
			if !nodeIsStale {
				activeCookbookNames[cbName] = true
			}
		}
	}

	if !activeCookbookNames["shared"] {
		t.Error("shared cookbook should be active — used by at least one non-stale node")
	}
	if activeCookbookNames["stale-only"] {
		t.Error("stale-only cookbook should NOT be active — only used by stale node")
	}
	if len(allCookbookNames) != 2 {
		t.Errorf("expected 2 total cookbooks, got %d", len(allCookbookNames))
	}
}

// TestActiveCookbookTracking_MissingOhaiTimeTreatedAsStale verifies that
// nodes with no ohai_time are treated as stale and their cookbooks are
// excluded from the active set.
func TestActiveCookbookTracking_MissingOhaiTimeTreatedAsStale(t *testing.T) {
	staleThreshold := 7 * 24 * time.Hour

	activeRow := mockSearchRow("active-01", "prod", "18.0.0", "ubuntu", map[string]string{
		"web": "1.0.0",
	})
	noOhaiRow := mockSearchRow("no-ohai", "prod", "18.0.0", "ubuntu", map[string]string{
		"mystery": "1.0.0",
	})
	delete(noOhaiRow.Data, "ohai_time")

	allCookbookNames := make(map[string]bool)
	activeCookbookNames := make(map[string]bool)

	for _, row := range []chefapi.SearchResultRow{activeRow, noOhaiRow} {
		nd := chefapi.NewNodeData(row.Data)
		nodeIsStale := nd.IsStale(staleThreshold)
		for cbName := range nd.CookbookVersions() {
			allCookbookNames[cbName] = true
			if !nodeIsStale {
				activeCookbookNames[cbName] = true
			}
		}
	}

	if !activeCookbookNames["web"] {
		t.Error("web cookbook should be active")
	}
	if activeCookbookNames["mystery"] {
		t.Error("mystery cookbook should NOT be active — node has no ohai_time so is treated as stale")
	}
	if !allCookbookNames["mystery"] {
		t.Error("mystery cookbook should still be in allCookbookNames")
	}
}

// TestActiveCookbookTracking_StaleOnlyCountCalculation verifies the
// staleOnlyCount calculation used for logging in the collector.
func TestActiveCookbookTracking_StaleOnlyCountCalculation(t *testing.T) {
	staleThreshold := 7 * 24 * time.Hour
	thirtyDaysAgo := float64(time.Now().Add(-30 * 24 * time.Hour).Unix())

	rows := []chefapi.SearchResultRow{
		mockSearchRow("active-01", "prod", "18.0.0", "ubuntu", map[string]string{
			"shared":      "1.0.0",
			"active-only": "1.0.0",
		}),
		mockSearchRow("stale-01", "prod", "18.0.0", "centos", map[string]string{
			"shared":  "1.0.0",
			"stale-a": "1.0.0",
			"stale-b": "1.0.0",
		}),
	}
	rows[1].Data["ohai_time"] = thirtyDaysAgo

	allCookbookNames := make(map[string]bool)
	activeCookbookNames := make(map[string]bool)

	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		nodeIsStale := nd.IsStale(staleThreshold)
		for cbName := range nd.CookbookVersions() {
			allCookbookNames[cbName] = true
			if !nodeIsStale {
				activeCookbookNames[cbName] = true
			}
		}
	}

	staleOnlyCount := len(allCookbookNames) - len(activeCookbookNames)

	// Total: shared, active-only, stale-a, stale-b = 4
	// Active: shared, active-only = 2
	// Stale-only: stale-a, stale-b = 2
	if len(allCookbookNames) != 4 {
		t.Errorf("expected 4 total cookbooks, got %d", len(allCookbookNames))
	}
	if len(activeCookbookNames) != 2 {
		t.Errorf("expected 2 active cookbooks, got %d", len(activeCookbookNames))
	}
	if staleOnlyCount != 2 {
		t.Errorf("expected staleOnlyCount = 2, got %d", staleOnlyCount)
	}
}

func TestActiveCookbookTracking_NoCookbooksOnNodes(t *testing.T) {
	rows := []chefapi.SearchResultRow{
		mockSearchRow("bare-01", "prod", "18.0.0", "ubuntu", nil),
		mockSearchRow("bare-02", "prod", "18.0.0", "ubuntu", map[string]string{}),
	}

	activeCookbookNames := make(map[string]bool)
	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		for cbName := range nd.CookbookVersions() {
			activeCookbookNames[cbName] = true
		}
	}

	if len(activeCookbookNames) != 0 {
		t.Errorf("expected 0 active cookbooks, got %d", len(activeCookbookNames))
	}
}

// ---------------------------------------------------------------------------
// Stale node detection in collection context
// ---------------------------------------------------------------------------

func TestStaleNodeDetection_RecentNode(t *testing.T) {
	row := mockSearchRow("recent", "prod", "18.0.0", "ubuntu", nil)
	nd := chefapi.NewNodeData(row.Data)

	threshold := 7 * 24 * time.Hour
	if nd.IsStale(threshold) {
		t.Error("node with recent ohai_time should not be stale")
	}
}

func TestStaleNodeDetection_StaleNode(t *testing.T) {
	row := mockSearchRow("stale", "prod", "18.0.0", "ubuntu", nil)
	// Override ohai_time to 30 days ago.
	row.Data["ohai_time"] = float64(time.Now().Add(-30 * 24 * time.Hour).Unix())

	nd := chefapi.NewNodeData(row.Data)

	threshold := 7 * 24 * time.Hour
	if !nd.IsStale(threshold) {
		t.Error("node with ohai_time 30 days ago should be stale with 7-day threshold")
	}
}

func TestStaleNodeDetection_MissingOhaiTime(t *testing.T) {
	row := mockSearchRow("no-ohai", "prod", "18.0.0", "ubuntu", nil)
	delete(row.Data, "ohai_time")

	nd := chefapi.NewNodeData(row.Data)

	threshold := 7 * 24 * time.Hour
	if !nd.IsStale(threshold) {
		t.Error("node with missing ohai_time should be treated as stale")
	}
}

func TestStaleNodeDetection_ZeroOhaiTime(t *testing.T) {
	row := mockSearchRow("zero-ohai", "prod", "18.0.0", "ubuntu", nil)
	row.Data["ohai_time"] = float64(0)

	nd := chefapi.NewNodeData(row.Data)

	threshold := 7 * 24 * time.Hour
	if !nd.IsStale(threshold) {
		t.Error("node with zero ohai_time should be treated as stale")
	}
}

// ---------------------------------------------------------------------------
// Collector.Option — WithClientFactory
// ---------------------------------------------------------------------------

func TestOption_WithClientFactory(t *testing.T) {
	invoked := false
	factory := func(ctx context.Context, org datastore.Organisation) (*chefapi.Client, error) {
		invoked = true
		return nil, nil
	}

	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{memWriter},
	})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver, WithClientFactory(factory))
	_, _ = c.clientFactory(context.Background(), datastore.Organisation{})

	if !invoked {
		t.Error("custom client factory was not called")
	}
}

// ---------------------------------------------------------------------------
// Parallel collection — verify the semaphore bounds concurrency
// ---------------------------------------------------------------------------

func TestSemaphoreBoundsConcurrency(t *testing.T) {
	// This test verifies the semaphore pattern used in Run() for bounding
	// concurrent organisation collection. We simulate the pattern here.

	const maxConcurrency = 3
	const totalWork = 10

	sem := make(chan struct{}, maxConcurrency)
	var maxObserved int32
	var current int32
	var wg sync.WaitGroup

	for i := 0; i < totalWork; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			c := atomic.AddInt32(&current, 1)
			// Track the max concurrent workers we observe.
			for {
				old := atomic.LoadInt32(&maxObserved)
				if c <= old {
					break
				}
				if atomic.CompareAndSwapInt32(&maxObserved, old, c) {
					break
				}
			}

			// Simulate some work.
			time.Sleep(10 * time.Millisecond)

			atomic.AddInt32(&current, -1)
		}()
	}

	wg.Wait()

	observed := atomic.LoadInt32(&maxObserved)
	if observed > int32(maxConcurrency) {
		t.Errorf("max concurrent workers observed: %d, expected <= %d", observed, maxConcurrency)
	}
	if observed == 0 {
		t.Error("no workers were observed running")
	}
}

// ---------------------------------------------------------------------------
// Context cancellation — verify the pattern used in Run()
// ---------------------------------------------------------------------------

func TestContextCancellation_SemaphoreAcquire(t *testing.T) {
	sem := make(chan struct{}, 1)
	sem <- struct{}{} // Fill the semaphore.

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	select {
	case sem <- struct{}{}:
		t.Error("should not have acquired semaphore")
	case <-ctx.Done():
		// Expected — context cancelled.
	}
}

// ---------------------------------------------------------------------------
// Per-node cookbook version tracking (nodeCookbookVersions)
// ---------------------------------------------------------------------------

func TestNodeCookbookVersionsTracking(t *testing.T) {
	// Simulate the nodeCookbookVersions tracking logic from collectOrganisation.
	rows := []chefapi.SearchResultRow{
		mockSearchRow("web-01", "prod", "18.0.0", "ubuntu", map[string]string{
			"apache2": "5.0.1",
			"openssl": "8.0.0",
		}),
		mockSearchRow("web-02", "prod", "18.0.0", "ubuntu", map[string]string{
			"apache2": "5.0.1",
			"mysql":   "10.0.0",
		}),
		mockSearchRow("db-01", "prod", "18.0.0", "centos", map[string]string{
			"mysql":   "10.0.0",
			"openssl": "8.0.0",
		}),
	}

	activeCookbookNames := make(map[string]bool)
	nodeCookbookVersions := make(map[string]map[string]string, len(rows))

	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		cbVersions := nd.CookbookVersions()
		for cbName := range cbVersions {
			activeCookbookNames[cbName] = true
		}
		if len(cbVersions) > 0 {
			nodeCookbookVersions[nd.Name()] = cbVersions
		}
	}

	// All 3 nodes should have entries.
	if len(nodeCookbookVersions) != 3 {
		t.Fatalf("expected 3 nodes in nodeCookbookVersions, got %d", len(nodeCookbookVersions))
	}

	// Verify web-01's cookbooks.
	web01 := nodeCookbookVersions["web-01"]
	if web01 == nil {
		t.Fatal("web-01 not found in nodeCookbookVersions")
	}
	if len(web01) != 2 {
		t.Errorf("web-01: expected 2 cookbooks, got %d", len(web01))
	}
	if web01["apache2"] != "5.0.1" {
		t.Errorf("web-01 apache2 version = %q, want %q", web01["apache2"], "5.0.1")
	}
	if web01["openssl"] != "8.0.0" {
		t.Errorf("web-01 openssl version = %q, want %q", web01["openssl"], "8.0.0")
	}

	// Verify db-01's cookbooks.
	db01 := nodeCookbookVersions["db-01"]
	if db01 == nil {
		t.Fatal("db-01 not found in nodeCookbookVersions")
	}
	if db01["mysql"] != "10.0.0" {
		t.Errorf("db-01 mysql version = %q, want %q", db01["mysql"], "10.0.0")
	}

	// activeCookbookNames should still be consistent.
	if len(activeCookbookNames) != 3 {
		t.Errorf("expected 3 active cookbook names, got %d", len(activeCookbookNames))
	}
}

func TestNodeCookbookVersionsTracking_NoCookbooks(t *testing.T) {
	// Nodes with nil or empty cookbooks should NOT appear in nodeCookbookVersions.
	rows := []chefapi.SearchResultRow{
		mockSearchRow("bare-01", "prod", "18.0.0", "ubuntu", nil),
		mockSearchRow("bare-02", "prod", "18.0.0", "ubuntu", map[string]string{}),
	}

	nodeCookbookVersions := make(map[string]map[string]string, len(rows))

	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		cbVersions := nd.CookbookVersions()
		if len(cbVersions) > 0 {
			nodeCookbookVersions[nd.Name()] = cbVersions
		}
	}

	if len(nodeCookbookVersions) != 0 {
		t.Errorf("expected 0 nodes in nodeCookbookVersions, got %d", len(nodeCookbookVersions))
	}
}

func TestNodeCookbookVersionsTracking_MixedNodes(t *testing.T) {
	// Some nodes have cookbooks, some don't.
	rows := []chefapi.SearchResultRow{
		mockSearchRow("web-01", "prod", "18.0.0", "ubuntu", map[string]string{
			"apache2": "5.0.1",
		}),
		mockSearchRow("bare-01", "prod", "18.0.0", "ubuntu", nil),
		mockSearchRow("db-01", "prod", "18.0.0", "centos", map[string]string{
			"mysql": "10.0.0",
		}),
	}

	nodeCookbookVersions := make(map[string]map[string]string, len(rows))

	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		cbVersions := nd.CookbookVersions()
		if len(cbVersions) > 0 {
			nodeCookbookVersions[nd.Name()] = cbVersions
		}
	}

	if len(nodeCookbookVersions) != 2 {
		t.Errorf("expected 2 nodes in nodeCookbookVersions, got %d", len(nodeCookbookVersions))
	}
	if _, ok := nodeCookbookVersions["bare-01"]; ok {
		t.Error("bare-01 should not be in nodeCookbookVersions")
	}
}

// ---------------------------------------------------------------------------
// NodeRecord building tests — verify that NodeRecords are built correctly
// from search result rows during Step 4, matching the wiring in
// collectOrganisation.
// ---------------------------------------------------------------------------

// mockSearchRowWithRolesAndPolicy builds a SearchResultRow with roles,
// policy_name, and policy_group in addition to the fields from mockSearchRow.
func mockSearchRowWithRolesAndPolicy(
	name, env, chefVersion, platform, platformVersion, platformFamily string,
	cookbooks map[string]string,
	roles []string,
	policyName, policyGroup string,
) chefapi.SearchResultRow {
	data := map[string]interface{}{
		"name":             name,
		"chef_environment": env,
		"chef_version":     chefVersion,
		"platform":         platform,
		"platform_version": platformVersion,
		"platform_family":  platformFamily,
		"ohai_time":        float64(time.Now().Unix()),
	}

	if cookbooks != nil {
		cbMap := make(map[string]interface{}, len(cookbooks))
		for cbName, ver := range cookbooks {
			cbMap[cbName] = map[string]interface{}{"version": ver}
		}
		data["cookbooks"] = cbMap
	}

	if roles != nil {
		ifaces := make([]interface{}, len(roles))
		for i, r := range roles {
			ifaces[i] = r
		}
		data["roles"] = ifaces
	}

	if policyName != "" {
		data["policy_name"] = policyName
	}
	if policyGroup != "" {
		data["policy_group"] = policyGroup
	}

	return chefapi.SearchResultRow{
		URL:  fmt.Sprintf("https://chef.example.com/nodes/%s", name),
		Data: data,
	}
}

func TestNodeRecordBuilding_BasicFields(t *testing.T) {
	rows := []chefapi.SearchResultRow{
		mockSearchRowWithRolesAndPolicy(
			"web-01", "prod", "18.0.0", "ubuntu", "22.04", "debian",
			map[string]string{"apache2": "5.0.1", "openssl": "8.0.0"},
			[]string{"web_server", "base"},
			"", "",
		),
	}

	var nodeRecords []analysis.NodeRecord
	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		cbVersions := nd.CookbookVersions()
		nodeRecords = append(nodeRecords, analysis.NodeRecordFromCollectedData(
			nd.Name(),
			nd.Platform(),
			nd.PlatformVersion(),
			nd.PlatformFamily(),
			nd.Roles(),
			nd.PolicyName(),
			nd.PolicyGroup(),
			cbVersions,
		))
	}

	if len(nodeRecords) != 1 {
		t.Fatalf("expected 1 node record, got %d", len(nodeRecords))
	}

	nr := nodeRecords[0]
	if nr.NodeName != "web-01" {
		t.Errorf("NodeName = %q, want %q", nr.NodeName, "web-01")
	}
	if nr.Platform != "ubuntu" {
		t.Errorf("Platform = %q, want %q", nr.Platform, "ubuntu")
	}
	if nr.PlatformVersion != "22.04" {
		t.Errorf("PlatformVersion = %q, want %q", nr.PlatformVersion, "22.04")
	}
	if nr.PlatformFamily != "debian" {
		t.Errorf("PlatformFamily = %q, want %q", nr.PlatformFamily, "debian")
	}
	if len(nr.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(nr.Roles))
	}
	sort.Strings(nr.Roles)
	if nr.Roles[0] != "base" || nr.Roles[1] != "web_server" {
		t.Errorf("Roles = %v, want [base, web_server]", nr.Roles)
	}
	if nr.PolicyName != "" {
		t.Errorf("PolicyName = %q, want empty", nr.PolicyName)
	}
	if nr.PolicyGroup != "" {
		t.Errorf("PolicyGroup = %q, want empty", nr.PolicyGroup)
	}
	if len(nr.CookbookVersions) != 2 {
		t.Fatalf("expected 2 cookbooks, got %d", len(nr.CookbookVersions))
	}
	if nr.CookbookVersions["apache2"] != "5.0.1" {
		t.Errorf("apache2 version = %q, want %q", nr.CookbookVersions["apache2"], "5.0.1")
	}
	if nr.CookbookVersions["openssl"] != "8.0.0" {
		t.Errorf("openssl version = %q, want %q", nr.CookbookVersions["openssl"], "8.0.0")
	}
}

func TestNodeRecordBuilding_PolicyfileNode(t *testing.T) {
	rows := []chefapi.SearchResultRow{
		mockSearchRowWithRolesAndPolicy(
			"app-01", "prod", "18.0.0", "centos", "8.0", "rhel",
			map[string]string{"myapp": "1.0.0"},
			nil,
			"myapp_policy", "production",
		),
	}

	var nodeRecords []analysis.NodeRecord
	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		cbVersions := nd.CookbookVersions()
		nodeRecords = append(nodeRecords, analysis.NodeRecordFromCollectedData(
			nd.Name(),
			nd.Platform(),
			nd.PlatformVersion(),
			nd.PlatformFamily(),
			nd.Roles(),
			nd.PolicyName(),
			nd.PolicyGroup(),
			cbVersions,
		))
	}

	if len(nodeRecords) != 1 {
		t.Fatalf("expected 1 node record, got %d", len(nodeRecords))
	}

	nr := nodeRecords[0]
	if nr.PolicyName != "myapp_policy" {
		t.Errorf("PolicyName = %q, want %q", nr.PolicyName, "myapp_policy")
	}
	if nr.PolicyGroup != "production" {
		t.Errorf("PolicyGroup = %q, want %q", nr.PolicyGroup, "production")
	}
	if nr.Roles != nil {
		t.Errorf("Roles = %v, want nil", nr.Roles)
	}
}

func TestNodeRecordBuilding_NoCookbooks(t *testing.T) {
	rows := []chefapi.SearchResultRow{
		mockSearchRowWithRolesAndPolicy(
			"empty-01", "dev", "18.0.0", "ubuntu", "20.04", "debian",
			nil, nil, "", "",
		),
	}

	var nodeRecords []analysis.NodeRecord
	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		cbVersions := nd.CookbookVersions()
		nodeRecords = append(nodeRecords, analysis.NodeRecordFromCollectedData(
			nd.Name(),
			nd.Platform(),
			nd.PlatformVersion(),
			nd.PlatformFamily(),
			nd.Roles(),
			nd.PolicyName(),
			nd.PolicyGroup(),
			cbVersions,
		))
	}

	nr := nodeRecords[0]
	if nr.NodeName != "empty-01" {
		t.Errorf("NodeName = %q, want %q", nr.NodeName, "empty-01")
	}
	if nr.CookbookVersions != nil {
		t.Errorf("CookbookVersions = %v, want nil", nr.CookbookVersions)
	}
}

func TestNodeRecordBuilding_MultipleNodes(t *testing.T) {
	rows := []chefapi.SearchResultRow{
		mockSearchRowWithRolesAndPolicy(
			"web-01", "prod", "18.0.0", "ubuntu", "22.04", "debian",
			map[string]string{"apache2": "5.0.1"},
			[]string{"web_server"}, "", "",
		),
		mockSearchRowWithRolesAndPolicy(
			"db-01", "prod", "18.0.0", "centos", "8.0", "rhel",
			map[string]string{"mysql": "10.0.0"},
			[]string{"db_server"}, "", "",
		),
		mockSearchRowWithRolesAndPolicy(
			"app-01", "prod", "18.0.0", "ubuntu", "22.04", "debian",
			map[string]string{"myapp": "1.0.0"},
			nil, "myapp_policy", "production",
		),
	}

	var nodeRecords []analysis.NodeRecord
	for _, row := range rows {
		nd := chefapi.NewNodeData(row.Data)
		cbVersions := nd.CookbookVersions()
		nodeRecords = append(nodeRecords, analysis.NodeRecordFromCollectedData(
			nd.Name(),
			nd.Platform(),
			nd.PlatformVersion(),
			nd.PlatformFamily(),
			nd.Roles(),
			nd.PolicyName(),
			nd.PolicyGroup(),
			cbVersions,
		))
	}

	if len(nodeRecords) != 3 {
		t.Fatalf("expected 3 node records, got %d", len(nodeRecords))
	}

	// Verify each node is represented correctly.
	byName := make(map[string]analysis.NodeRecord, len(nodeRecords))
	for _, nr := range nodeRecords {
		byName[nr.NodeName] = nr
	}

	web := byName["web-01"]
	if web.Platform != "ubuntu" {
		t.Errorf("web-01 Platform = %q, want %q", web.Platform, "ubuntu")
	}
	if web.CookbookVersions["apache2"] != "5.0.1" {
		t.Errorf("web-01 apache2 = %q, want %q", web.CookbookVersions["apache2"], "5.0.1")
	}

	db := byName["db-01"]
	if db.PlatformFamily != "rhel" {
		t.Errorf("db-01 PlatformFamily = %q, want %q", db.PlatformFamily, "rhel")
	}
	if db.CookbookVersions["mysql"] != "10.0.0" {
		t.Errorf("db-01 mysql = %q, want %q", db.CookbookVersions["mysql"], "10.0.0")
	}

	app := byName["app-01"]
	if app.PolicyName != "myapp_policy" {
		t.Errorf("app-01 PolicyName = %q, want %q", app.PolicyName, "myapp_policy")
	}
	if app.PolicyGroup != "production" {
		t.Errorf("app-01 PolicyGroup = %q, want %q", app.PolicyGroup, "production")
	}
}

// ---------------------------------------------------------------------------
// CookbookInventoryEntry building tests — verify that the inventory entry
// list is built correctly from the serverCookbooks map, matching the wiring
// in collectOrganisation Step 10.
// ---------------------------------------------------------------------------

// buildInventoryEntries replicates the inventory entry building logic from
// collectOrganisation Step 10 for isolated testing.
func buildInventoryEntries(serverCookbooks map[string]chefapi.CookbookListEntry) []analysis.CookbookInventoryEntry {
	entries := make([]analysis.CookbookInventoryEntry, 0)
	for cbName, entry := range serverCookbooks {
		for _, ver := range entry.Versions {
			entries = append(entries, analysis.CookbookInventoryEntry{
				Name:    cbName,
				Version: ver.Version,
			})
		}
	}
	return entries
}

func TestBuildInventoryEntries_Basic(t *testing.T) {
	serverCookbooks := mockCookbookList(map[string][]string{
		"apache2": {"5.0.1", "4.0.0"},
		"mysql":   {"10.0.0"},
	})

	entries := buildInventoryEntries(serverCookbooks)

	if len(entries) != 3 {
		t.Fatalf("expected 3 inventory entries, got %d", len(entries))
	}

	// Build a set of name+version for verification (map iteration is
	// non-deterministic so we can't check order).
	type nv struct{ name, version string }
	seen := make(map[nv]bool)
	for _, e := range entries {
		seen[nv{e.Name, e.Version}] = true
	}

	expected := []nv{
		{"apache2", "5.0.1"},
		{"apache2", "4.0.0"},
		{"mysql", "10.0.0"},
	}
	for _, exp := range expected {
		if !seen[exp] {
			t.Errorf("missing inventory entry: %s@%s", exp.name, exp.version)
		}
	}
}

func TestBuildInventoryEntries_Empty(t *testing.T) {
	entries := buildInventoryEntries(map[string]chefapi.CookbookListEntry{})
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestBuildInventoryEntries_SingleVersion(t *testing.T) {
	serverCookbooks := mockCookbookList(map[string][]string{
		"nginx": {"2.0.0"},
	})

	entries := buildInventoryEntries(serverCookbooks)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "nginx" {
		t.Errorf("Name = %q, want %q", entries[0].Name, "nginx")
	}
	if entries[0].Version != "2.0.0" {
		t.Errorf("Version = %q, want %q", entries[0].Version, "2.0.0")
	}
}

func TestBuildInventoryEntries_ManyVersions(t *testing.T) {
	serverCookbooks := mockCookbookList(map[string][]string{
		"java": {"1.0.0", "2.0.0", "3.0.0", "4.0.0", "5.0.0"},
	})

	entries := buildInventoryEntries(serverCookbooks)

	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	type nv struct{ name, version string }
	seen := make(map[nv]bool)
	for _, e := range entries {
		seen[nv{e.Name, e.Version}] = true
	}

	for _, ver := range []string{"1.0.0", "2.0.0", "3.0.0", "4.0.0", "5.0.0"} {
		if !seen[nv{"java", ver}] {
			t.Errorf("missing java@%s", ver)
		}
	}
}

func TestBuildInventoryEntries_ManyCookbooks(t *testing.T) {
	cbMap := map[string][]string{
		"apache2": {"5.0.1"},
		"mysql":   {"10.0.0"},
		"nginx":   {"2.0.0"},
		"openssl": {"8.0.0"},
		"java":    {"1.0.0"},
	}
	serverCookbooks := mockCookbookList(cbMap)

	entries := buildInventoryEntries(serverCookbooks)

	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	for name := range cbMap {
		if !names[name] {
			t.Errorf("missing cookbook %q in inventory entries", name)
		}
	}
}

// TestCollector_AnalyserCreated verifies that the Collector creates an
// Analyser during construction (non-nil field).
func TestCollector_AnalyserCreated(t *testing.T) {
	factory := func(ctx context.Context, org datastore.Organisation) (*chefapi.Client, error) {
		return nil, nil
	}
	c, _ := newTestCollector(t, factory)
	if c.analyser == nil {
		t.Fatal("expected analyser to be non-nil after New()")
	}
}

// ---------------------------------------------------------------------------
// Pipeline wiring — Option function tests
// ---------------------------------------------------------------------------

func TestCollector_PipelineFields_NilByDefault(t *testing.T) {
	factory := func(ctx context.Context, org datastore.Organisation) (*chefapi.Client, error) {
		return nil, nil
	}
	c, _ := newTestCollector(t, factory)

	if c.cookstyleScanner != nil {
		t.Error("expected cookstyleScanner to be nil by default")
	}
	if c.kitchenScanner != nil {
		t.Error("expected kitchenScanner to be nil by default")
	}
	if c.autocorrectGen != nil {
		t.Error("expected autocorrectGen to be nil by default")
	}
	if c.complexityScorer != nil {
		t.Error("expected complexityScorer to be nil by default")
	}
	if c.readinessEval != nil {
		t.Error("expected readinessEval to be nil by default")
	}
	if c.serverCookbookDirFn != nil {
		t.Error("expected serverCookbookDirFn to be nil by default")
	}
	if c.gitRepoDirFn != nil {
		t.Error("expected gitRepoDirFn to be nil by default")
	}
}

func TestWithCookstyleScanner_SetsField(t *testing.T) {
	scanner := analysis.NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 2, 5)

	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{Level: logging.DEBUG, Writers: []logging.Writer{memWriter}})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver, WithCookstyleScanner(scanner))
	if c.cookstyleScanner != scanner {
		t.Error("expected cookstyleScanner to be set by WithCookstyleScanner")
	}
}

func TestWithKitchenScanner_SetsField(t *testing.T) {
	scanner := analysis.NewKitchenScanner(nil, nil, "/usr/bin/kitchen", 2, 30, config.TestKitchenConfig{})

	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{Level: logging.DEBUG, Writers: []logging.Writer{memWriter}})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver, WithKitchenScanner(scanner))
	if c.kitchenScanner != scanner {
		t.Error("expected kitchenScanner to be set by WithKitchenScanner")
	}
}

func TestWithAutocorrectGenerator_SetsField(t *testing.T) {
	gen := remediation.NewAutocorrectGenerator(nil, nil, "/usr/bin/cookstyle", 10)

	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{Level: logging.DEBUG, Writers: []logging.Writer{memWriter}})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver, WithAutocorrectGenerator(gen))
	if c.autocorrectGen != gen {
		t.Error("expected autocorrectGen to be set by WithAutocorrectGenerator")
	}
}

func TestWithComplexityScorer_SetsField(t *testing.T) {
	scorer := remediation.NewComplexityScorer(nil, nil)

	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{Level: logging.DEBUG, Writers: []logging.Writer{memWriter}})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver, WithComplexityScorer(scorer))
	if c.complexityScorer != scorer {
		t.Error("expected complexityScorer to be set by WithComplexityScorer")
	}
}

func TestWithReadinessEvaluator_SetsField(t *testing.T) {
	eval := analysis.NewReadinessEvaluator(nil, nil, 2, 2048)

	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{Level: logging.DEBUG, Writers: []logging.Writer{memWriter}})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver, WithReadinessEvaluator(eval))
	if c.readinessEval != eval {
		t.Error("expected readinessEval to be set by WithReadinessEvaluator")
	}
}

func TestWithServerCookbookDirFn_SetsField(t *testing.T) {
	called := false
	fn := func(sc datastore.ServerCookbook) string {
		called = true
		return "/some/path/" + sc.Name
	}

	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{Level: logging.DEBUG, Writers: []logging.Writer{memWriter}})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver, WithServerCookbookDirFn(fn))
	if c.serverCookbookDirFn == nil {
		t.Fatal("expected serverCookbookDirFn to be non-nil after WithServerCookbookDirFn")
	}

	result := c.serverCookbookDirFn(datastore.ServerCookbook{Name: "test"})
	if !called {
		t.Error("expected serverCookbookDirFn to be called")
	}
	if result != "/some/path/test" {
		t.Errorf("serverCookbookDirFn returned %q, want %q", result, "/some/path/test")
	}
}

func TestWithGitRepoDirFn_SetsField(t *testing.T) {
	called := false
	fn := func(repo datastore.GitRepo) string {
		called = true
		return "/git/repos/" + repo.Name
	}

	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{Level: logging.DEBUG, Writers: []logging.Writer{memWriter}})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver, WithGitRepoDirFn(fn))
	if c.gitRepoDirFn == nil {
		t.Fatal("expected gitRepoDirFn to be non-nil after WithGitRepoDirFn")
	}

	result := c.gitRepoDirFn(datastore.GitRepo{Name: "test"})
	if !called {
		t.Error("expected gitRepoDirFn to be called")
	}
	if result != "/git/repos/test" {
		t.Errorf("gitRepoDirFn returned %q, want %q", result, "/git/repos/test")
	}
}

func TestWithServerCookbookDirFn_NilIsAccepted(t *testing.T) {
	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{Level: logging.DEBUG, Writers: []logging.Writer{memWriter}})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver, WithServerCookbookDirFn(nil))
	if c.serverCookbookDirFn != nil {
		t.Error("expected serverCookbookDirFn to remain nil when WithServerCookbookDirFn(nil) is passed")
	}
}

func TestWithGitRepoDirFn_NilIsAccepted(t *testing.T) {
	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{Level: logging.DEBUG, Writers: []logging.Writer{memWriter}})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver, WithGitRepoDirFn(nil))
	if c.gitRepoDirFn != nil {
		t.Error("expected gitRepoDirFn to remain nil when WithGitRepoDirFn(nil) is passed")
	}
}

// ---------------------------------------------------------------------------
// Checkpoint/Resume tests
// ---------------------------------------------------------------------------

func TestEstimateCollectionInterval_DefaultHourly(t *testing.T) {
	cfg := &config.Config{}
	cfg.Collection.Schedule = "0 * * * *" // every hour
	cfg.Collection.StaleNodeThresholdDays = 7
	cfg.Collection.StaleCookbookThresholdDays = 365
	cfg.Concurrency.OrganisationCollection = 1

	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{logging.NewMemoryWriter()},
	})
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver)
	interval := c.estimateCollectionInterval()

	if interval != 1*time.Hour {
		t.Errorf("expected 1h interval for hourly schedule, got %v", interval)
	}
}

func TestEstimateCollectionInterval_Every15Minutes(t *testing.T) {
	cfg := &config.Config{}
	cfg.Collection.Schedule = "*/15 * * * *" // every 15 minutes
	cfg.Collection.StaleNodeThresholdDays = 7
	cfg.Collection.StaleCookbookThresholdDays = 365
	cfg.Concurrency.OrganisationCollection = 1

	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{logging.NewMemoryWriter()},
	})
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver)
	interval := c.estimateCollectionInterval()

	if interval != 15*time.Minute {
		t.Errorf("expected 15m interval, got %v", interval)
	}
}

func TestEstimateCollectionInterval_Every5Minutes(t *testing.T) {
	cfg := &config.Config{}
	cfg.Collection.Schedule = "*/5 * * * *"
	cfg.Collection.StaleNodeThresholdDays = 7
	cfg.Collection.StaleCookbookThresholdDays = 365
	cfg.Concurrency.OrganisationCollection = 1

	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{logging.NewMemoryWriter()},
	})
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver)
	interval := c.estimateCollectionInterval()

	if interval != 5*time.Minute {
		t.Errorf("expected 5m interval, got %v", interval)
	}
}

func TestEstimateCollectionInterval_InvalidScheduleFallsBackTo1Hour(t *testing.T) {
	cfg := &config.Config{}
	cfg.Collection.Schedule = "invalid cron expression that won't parse"
	cfg.Collection.StaleNodeThresholdDays = 7
	cfg.Collection.StaleCookbookThresholdDays = 365
	cfg.Concurrency.OrganisationCollection = 1

	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{logging.NewMemoryWriter()},
	})
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver)
	interval := c.estimateCollectionInterval()

	if interval != 1*time.Hour {
		t.Errorf("expected 1h fallback for invalid schedule, got %v", interval)
	}
}

func TestEstimateCollectionInterval_TwiceDaily(t *testing.T) {
	cfg := &config.Config{}
	cfg.Collection.Schedule = "0 0,12 * * *" // midnight and noon
	cfg.Collection.StaleNodeThresholdDays = 7
	cfg.Collection.StaleCookbookThresholdDays = 365
	cfg.Concurrency.OrganisationCollection = 1

	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{logging.NewMemoryWriter()},
	})
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver)
	interval := c.estimateCollectionInterval()

	if interval != 12*time.Hour {
		t.Errorf("expected 12h interval for twice-daily schedule, got %v", interval)
	}
}

func TestResumeResult_ZeroValue(t *testing.T) {
	r := ResumeResult{}
	if r.Evaluated != 0 {
		t.Errorf("expected 0 evaluated, got %d", r.Evaluated)
	}
	if r.Resumed != 0 {
		t.Errorf("expected 0 resumed, got %d", r.Resumed)
	}
	if r.Abandoned != 0 {
		t.Errorf("expected 0 abandoned, got %d", r.Abandoned)
	}
	if r.ResumedRunResult != nil {
		t.Error("expected nil ResumedRunResult")
	}
}

func TestResumeInterruptedRuns_NilDB_ReturnsError(t *testing.T) {
	cfg := &config.Config{}
	cfg.Collection.Schedule = "0 * * * *"
	cfg.Collection.StaleNodeThresholdDays = 7
	cfg.Collection.StaleCookbookThresholdDays = 365
	cfg.Concurrency.OrganisationCollection = 1

	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{logging.NewMemoryWriter()},
	})
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver)

	// With a nil DB, the call to GetInterruptedCollectionRuns will panic
	// (nil pointer dereference). This verifies the collector handles the
	// nil DB scenario — in production, DB is never nil, but we confirm
	// the method doesn't silently succeed.
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic with nil DB, but got none")
			}
		}()
		_, _ = c.ResumeInterruptedRuns(context.Background())
	}()
}

func TestRunForOrganisations_ErrorWhenAlreadyRunning(t *testing.T) {
	cfg := &config.Config{}
	cfg.Collection.Schedule = "0 * * * *"
	cfg.Collection.StaleNodeThresholdDays = 7
	cfg.Collection.StaleCookbookThresholdDays = 365
	cfg.Concurrency.OrganisationCollection = 1

	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{logging.NewMemoryWriter()},
	})
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver)

	// Manually set running to true to simulate a concurrent run.
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	orgs := map[string]datastore.Organisation{
		"test-id": {ID: "test-id", Name: "test-org"},
	}

	_, err := c.runForOrganisations(context.Background(), orgs)
	if err == nil {
		t.Error("expected error when run already in progress, got nil")
	}
	if err != nil && err.Error() != "collector: a collection run is already in progress" {
		t.Errorf("unexpected error: %v", err)
	}

	// Clean up.
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
}

func TestEstimateCollectionInterval_DailySchedule(t *testing.T) {
	cfg := &config.Config{}
	cfg.Collection.Schedule = "0 2 * * *" // daily at 2am
	cfg.Collection.StaleNodeThresholdDays = 7
	cfg.Collection.StaleCookbookThresholdDays = 365
	cfg.Concurrency.OrganisationCollection = 1

	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{logging.NewMemoryWriter()},
	})
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver)
	interval := c.estimateCollectionInterval()

	if interval != 24*time.Hour {
		t.Errorf("expected 24h interval for daily schedule, got %v", interval)
	}
}

func TestEstimateCollectionInterval_Every30Minutes(t *testing.T) {
	cfg := &config.Config{}
	cfg.Collection.Schedule = "*/30 * * * *"
	cfg.Collection.StaleNodeThresholdDays = 7
	cfg.Collection.StaleCookbookThresholdDays = 365
	cfg.Concurrency.OrganisationCollection = 1

	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{logging.NewMemoryWriter()},
	})
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver)
	interval := c.estimateCollectionInterval()

	if interval != 30*time.Minute {
		t.Errorf("expected 30m interval, got %v", interval)
	}
}

func TestResumeResult_ErrorsMap(t *testing.T) {
	r := ResumeResult{
		Evaluated: 3,
		Resumed:   1,
		Abandoned: 1,
		Errors:    map[string]error{"run-1": errors.New("test error")},
	}

	if r.Evaluated != 3 {
		t.Errorf("expected 3 evaluated, got %d", r.Evaluated)
	}
	if r.Resumed != 1 {
		t.Errorf("expected 1 resumed, got %d", r.Resumed)
	}
	if r.Abandoned != 1 {
		t.Errorf("expected 1 abandoned, got %d", r.Abandoned)
	}
	if len(r.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(r.Errors))
	}
	if _, ok := r.Errors["run-1"]; !ok {
		t.Error("expected error for run-1")
	}
}

func TestRunForOrganisations_EmptyOrgMap(t *testing.T) {
	cfg := &config.Config{}
	cfg.Collection.Schedule = "0 * * * *"
	cfg.Collection.StaleNodeThresholdDays = 7
	cfg.Collection.StaleCookbookThresholdDays = 365
	cfg.Concurrency.OrganisationCollection = 1

	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{memWriter},
	})
	resolver := secrets.NewCredentialResolver(nil)

	// Use a nil DB — since there are no orgs to collect, collectOrganisation
	// is never called and the nil DB is safe.
	c := New(nil, cfg, logger, resolver)

	orgs := map[string]datastore.Organisation{}
	result, err := c.runForOrganisations(context.Background(), orgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TotalOrgs != 0 {
		t.Errorf("expected 0 total orgs, got %d", result.TotalOrgs)
	}
	if result.SucceededOrgs != 0 {
		t.Errorf("expected 0 succeeded orgs, got %d", result.SucceededOrgs)
	}
	if result.FailedOrgs != 0 {
		t.Errorf("expected 0 failed orgs, got %d", result.FailedOrgs)
	}
}

func TestEstimateCollectionInterval_EmptyScheduleFallsBack(t *testing.T) {
	cfg := &config.Config{}
	cfg.Collection.Schedule = "" // empty schedule
	cfg.Collection.StaleNodeThresholdDays = 7
	cfg.Collection.StaleCookbookThresholdDays = 365
	cfg.Concurrency.OrganisationCollection = 1

	logger := logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{logging.NewMemoryWriter()},
	})
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver)
	interval := c.estimateCollectionInterval()

	if interval != 1*time.Hour {
		t.Errorf("expected 1h fallback for empty schedule, got %v", interval)
	}
}

func TestMultiplePipelineOptions_AllSet(t *testing.T) {
	csScanner := analysis.NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 2, 5)
	tkScanner := analysis.NewKitchenScanner(nil, nil, "/usr/bin/kitchen", 2, 30, config.TestKitchenConfig{})
	acGen := remediation.NewAutocorrectGenerator(nil, nil, "/usr/bin/cookstyle", 10)
	cxScorer := remediation.NewComplexityScorer(nil, nil)
	readEval := analysis.NewReadinessEvaluator(nil, nil, 2, 2048)
	scDirFn := func(sc datastore.ServerCookbook) string { return "/cookbooks/" + sc.Name }
	grDirFn := func(repo datastore.GitRepo) string { return "/git/" + repo.Name }

	memWriter := logging.NewMemoryWriter()
	logger := logging.New(logging.Options{Level: logging.DEBUG, Writers: []logging.Writer{memWriter}})
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)

	c := New(nil, cfg, logger, resolver,
		WithCookstyleScanner(csScanner),
		WithKitchenScanner(tkScanner),
		WithAutocorrectGenerator(acGen),
		WithComplexityScorer(cxScorer),
		WithReadinessEvaluator(readEval),
		WithServerCookbookDirFn(scDirFn),
		WithGitRepoDirFn(grDirFn),
	)

	if c.cookstyleScanner != csScanner {
		t.Error("cookstyleScanner not set")
	}
	if c.kitchenScanner != tkScanner {
		t.Error("kitchenScanner not set")
	}
	if c.autocorrectGen != acGen {
		t.Error("autocorrectGen not set")
	}
	if c.complexityScorer != cxScorer {
		t.Error("complexityScorer not set")
	}
	if c.readinessEval != readEval {
		t.Error("readinessEval not set")
	}
	if c.serverCookbookDirFn == nil {
		t.Error("serverCookbookDirFn not set")
	}
	if c.gitRepoDirFn == nil {
		t.Error("gitRepoDirFn not set")
	}

	// Also verify original fields still work.
	if c.analyser == nil {
		t.Error("analyser should still be set")
	}
}
