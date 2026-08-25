// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// Neither settings screen can reach the other's settings.
//
// Test Kitchen and the analysis tools are two stored records. The Analysis Tools
// screen replaces its record wholesale and does not carry the Test Kitchen part,
// so nesting one inside the other loses the driver, the images, the credential
// references and the rate limits while reporting a successful save.
//
// In the gating suite rather than held as debt: silently losing somebody's
// settings must not ship.

// Saving the Analysis Tools screen leaves the Test Kitchen settings alone.
func TestSettings_SavingAnalysisToolsKeepsTheTestKitchenSettings(t *testing.T) {
	ctx := context.Background()
	store := newTestConfigStore(t)
	cfg := testConfig()
	router := newTestRouterForAdminConfig(cfg, store, configstore.NewConfigHolder(cfg, store))

	// Configured the way an operator does, through its own screen.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/config/test-kitchen",
		strings.NewReader(`{"driver":"vcenter","timeout_minutes":45}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("configuring Test Kitchen was refused (%d): %s", w.Code, w.Body.String())
	}
	// The baseline: it really is stored before the save below. Without this the
	// test goes green the day the fixture stops storing anything.
	if got := storedKitchenDriver(t, ctx, store); got != "vcenter" {
		t.Fatalf("the driver was not stored to begin with (%q), so this test cannot show "+
			"one surviving", got)
	}

	// What the Analysis Tools screen sends: its own fields, nothing about Test
	// Kitchen, because its own type does not have them.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/config/analysis-tools", strings.NewReader(
			`{"embedded_bin_dir":"","cookstyle_timeout_minutes":10,"cookstyle_addon_cop_paths":[]}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("saving the analysis tools was refused (%d): %s", w.Code, w.Body.String())
	}

	if got := storedKitchenDriver(t, ctx, store); got != "vcenter" {
		t.Errorf("saving the Analysis Tools screen left the Test Kitchen driver as %q — an "+
			"operator changed a timeout and lost the driver, the images and the credential "+
			"references, and was told the save succeeded", got)
	}
}

// And the other way round — the half that a merge-based fix would break.
func TestSettings_SavingTestKitchenKeepsTheAnalysisToolsSettings(t *testing.T) {
	ctx := context.Background()
	store := newTestConfigStore(t)
	cfg := testConfig()
	router := newTestRouterForAdminConfig(cfg, store, configstore.NewConfigHolder(cfg, store))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/config/analysis-tools", strings.NewReader(
			`{"embedded_bin_dir":"/opt/chef-workstation/embedded/bin",`+
				`"cookstyle_timeout_minutes":10,"cookstyle_addon_cop_paths":[]}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("saving the analysis tools was refused (%d): %s", w.Code, w.Body.String())
	}
	if got := storedBinDir(t, ctx, store); got != "/opt/chef-workstation/embedded/bin" {
		t.Fatalf("the Chef tools directory was not stored to begin with (%q)", got)
	}

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/config/test-kitchen",
		strings.NewReader(`{"driver":"vcenter","timeout_minutes":45}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("configuring Test Kitchen was refused (%d): %s", w.Code, w.Body.String())
	}

	if got := storedBinDir(t, ctx, store); got != "/opt/chef-workstation/embedded/bin" {
		t.Errorf("saving the Test Kitchen screen left the Chef tools directory as %q", got)
	}
}

// A setting can still be cleared. This is what merging what-was-not-sent would
// have cost: with a merge, absent and cleared are the same thing, and a setting
// can never be emptied again.
func TestSettings_ATestKitchenSettingCanStillBeCleared(t *testing.T) {
	ctx := context.Background()
	store := newTestConfigStore(t)
	cfg := testConfig()
	router := newTestRouterForAdminConfig(cfg, store, configstore.NewConfigHolder(cfg, store))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/config/test-kitchen",
		strings.NewReader(`{"driver":"vcenter","vm_name_prefix":"example-"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("configuring it was refused (%d): %s", w.Code, w.Body.String())
	}
	if got := storedKitchenField(t, ctx, store, "vm_name_prefix"); got != "example-" {
		t.Fatalf("the prefix was not stored to begin with (%q)", got)
	}

	// The same screen, with the prefix emptied.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/config/test-kitchen",
		strings.NewReader(`{"driver":"vcenter","vm_name_prefix":""}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("clearing it was refused (%d): %s", w.Code, w.Body.String())
	}

	if got := storedKitchenField(t, ctx, store, "vm_name_prefix"); got != "" {
		t.Errorf("a setting could not be emptied (%q) — absent and cleared have become the "+
			"same thing, which is what keeping the two records apart was for", got)
	}
}

// storedKitchenDriver reads the driver out of the Test Kitchen record.
func storedKitchenDriver(t *testing.T, ctx context.Context, store *configstore.Store) string {
	t.Helper()
	return storedKitchenField(t, ctx, store, "driver")
}

// storedKitchenField reads one field out of the Test Kitchen record.
func storedKitchenField(t *testing.T, ctx context.Context, store *configstore.Store,
	field string) string {
	t.Helper()
	raw, err := store.Get(ctx, configstore.KeyTestKitchen)
	if err != nil {
		t.Fatalf("reading the Test Kitchen record: %v", err)
	}
	var section map[string]any
	if err := json.Unmarshal(raw, &section); err != nil {
		t.Fatalf("reading it: %v", err)
	}
	value, _ := section[field].(string)
	return value
}

// storedBinDir reads the Chef tools directory out of the analysis tools record.
func storedBinDir(t *testing.T, ctx context.Context, store *configstore.Store) string {
	t.Helper()
	raw, err := store.Get(ctx, configstore.KeyAnalysisTools)
	if err != nil {
		t.Fatalf("reading the analysis tools record: %v", err)
	}
	var section map[string]any
	if err := json.Unmarshal(raw, &section); err != nil {
		t.Fatalf("reading it: %v", err)
	}
	value, _ := section["embedded_bin_dir"].(string)
	return value
}
