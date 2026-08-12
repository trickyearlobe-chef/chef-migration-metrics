//go:build debt

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

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// Tech debt that can be held by a test, from plans/todo-tech-debt.md. Run it
// with `make debt`. Red means still outstanding; nothing here gates a release.

// Saving the Analysis Tools screen wipes the Test Kitchen configuration.
//
// Two screens write the same stored section. The Test Kitchen one reads what is
// there and puts its part back; the Analysis Tools one replaces the whole
// section with what it was sent — and the screen has never sent the Test
// Kitchen part, because its own type does not have it. So the driver, the
// images, the credential references, the timeouts and the rate limits all go,
// and the screen reports a successful save.
//
// Found by comparing what the frontend sends against what the handlers read, in
// the other direction from the unknown-field work: that one was about fields a
// caller sends and nothing reads, this is about fields nothing sends and the
// service overwrites.
//
// The fix is a decision rather than an edit — merge what was not sent, or make
// the screen send the section whole — and merging silently is how a setting
// becomes impossible to clear. So it is recorded rather than done quietly.
func TestDebt_SavingTheAnalysisToolsScreenKeepsTheTestKitchenSettings(t *testing.T) {
	store := newTestConfigStore(t)

	// A deployment with Test Kitchen configured, exactly as its own screen
	// would have left it.
	cfg := testConfig()
	cfg.AnalysisTools = config.AnalysisToolsConfig{
		CookstyleTimeoutMinutes: 10,
		TestKitchen: config.TestKitchenConfig{
			Driver:         "vcenter",
			TimeoutMinutes: 45,
		},
	}
	holder := configstore.NewConfigHolder(cfg, store)
	router := newTestRouterForAdminConfig(cfg, store, holder)

	// Put it there the way an operator does — through the Test Kitchen screen —
	// rather than reaching into the store, so what is lost below is something
	// that really got there by the supported route.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/config/test-kitchen",
		strings.NewReader(`{"driver":"vcenter","timeout_minutes":45}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("configuring Test Kitchen was refused (%d): %s", w.Code, w.Body.String())
	}

	// The baseline: it is really there before the save. Without this the test
	// would go green the day the fixture stopped setting it, and the item
	// would be silently lost.
	if before := storedTestKitchenDriver(t, store); before != "vcenter" {
		t.Fatalf("the fixture did not store a Test Kitchen driver to begin with (%q), so "+
			"this test cannot show one being lost", before)
	}

	// What the Analysis Tools screen sends: its own fields, and nothing about
	// Test Kitchen. Measured off the frontend's own type, not invented here.
	body := `{"embedded_bin_dir":"","cookstyle_timeout_minutes":10,` +
		`"cookstyle_addon_cop_paths":[]}`
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/config/analysis-tools", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("the save this test is about was refused (%d): %s", w.Code, w.Body.String())
	}

	if after := storedTestKitchenDriver(t, store); after != "vcenter" {
		t.Errorf("saving the Analysis Tools screen left the Test Kitchen driver as %q — "+
			"an operator changed a timeout and lost the driver, the images and the "+
			"credential references, and was told the save succeeded", after)
	}
}

// storedTestKitchenDriver reads the driver out of the stored section.
func storedTestKitchenDriver(t *testing.T, store *configstore.Store) string {
	t.Helper()
	raw, err := store.Get(context.Background(), configstore.KeyAnalysisTools)
	if err != nil {
		// Nothing stored yet is not the same as a wiped driver, and saying so
		// keeps this from reading as the failure it names.
		t.Fatalf("reading the stored analysis tools section: %v", err)
	}
	var section struct {
		TestKitchen struct {
			Driver string `json:"driver"`
		} `json:"test_kitchen"`
	}
	if err := json.Unmarshal(raw, &section); err != nil {
		t.Fatalf("reading the stored section: %v", err)
	}
	return section.TestKitchen.Driver
}
