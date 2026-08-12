//go:build functional

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// The screens save, against a real database.
//
// Everything else about these paths is exercised against a fake store, which
// answers whatever it was handed. This drives the same handlers into real
// PostgreSQL, through the real encrypted config store, with the bodies the web
// interface actually sends — measured with the TypeScript compiler and kept in
// testdata/frontend_request_fields.json, not invented here.
//
// It exists because "the interface tests are green" never meant the interface
// worked: 31 of the 45 page test files mock the API module, and nothing there
// drives a real body into a real handler. Three screens were sending fields no
// handler read, and one of them silently wiped another screen's settings.
//
//	CMM_TEST_DATABASE_URL=... go test -tags functional ./internal/webapi/ \
//	  -run TestFunctional_Settings

// settingsTestStore opens the test database and returns a real config store on
// it, cleared of the sections these tests write.
func settingsTestStore(t *testing.T) (*configstore.Store, *datastore.DB) {
	t.Helper()
	url := os.Getenv("CMM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("CMM_TEST_DATABASE_URL not set — skipping functional test")
	}
	db, err := datastore.Open(url)
	if err != nil {
		t.Fatalf("opening the test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.MigrateUp(context.Background(), "../../migrations"); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("making an encryption key: %v", err)
	}
	enc, err := secrets.NewEncryptor(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("making an encryptor: %v", err)
	}
	store := configstore.NewStore(db, enc)

	// A settled starting point. These tests are about what one save does to
	// another's settings, so anything left behind by a previous run would make
	// the answer depend on the order they happened to be run in.
	ctx := context.Background()
	for _, key := range []string{
		configstore.KeyAnalysisTools, configstore.KeyTestKitchen,
	} {
		_ = store.Delete(ctx, key)
	}
	t.Cleanup(func() {
		for _, key := range []string{
			configstore.KeyAnalysisTools, configstore.KeyTestKitchen,
		} {
			_ = store.Delete(context.Background(), key)
		}
	})
	return store, db
}

// The bodies the web interface really sends. Field for field what
// testdata/frontend_request_fields.json records, with plausible values.
const (
	// The Analysis Tools screen. No Test Kitchen part — its own type does not
	// have one, which is what used to wipe those settings.
	frontendAnalysisToolsSave = `{
		"embedded_bin_dir": "/opt/chef-workstation/embedded/bin",
		"cookstyle_enabled": true,
		"cookstyle_timeout_minutes": 12,
		"cookstyle_addon_cop_paths": []
	}`

	// The Test Kitchen screen.
	frontendTestKitchenSave = `{
		"enabled": true,
		"driver": "vcenter",
		"timeout_minutes": 45,
		"image_field_name": "",
		"chef_license_key_credential": "",
		"driver_settings": {},
		"driver_secrets": {},
		"images": [],
		"platform_map": [],
		"setup_scripts": {"linux": [], "windows": []},
		"start_rate_window_minutes": 60,
		"start_rate_max_per_window": 10
	}`
)

// Saving the Analysis Tools screen keeps the Test Kitchen settings, against a
// real store rather than one that answers whatever it was handed.
func TestFunctional_SettingsAnalysisToolsSaveKeepsTestKitchen(t *testing.T) {
	store, _ := settingsTestStore(t)
	ctx := context.Background()
	cfg := testConfig()
	router := newTestRouterForAdminConfig(cfg, store, configstore.NewConfigHolder(cfg, store))

	settingsPut(t, router, "/api/v1/admin/config/test-kitchen", frontendTestKitchenSave)
	// The baseline: it really is in PostgreSQL before the save below.
	if got := storedDriver(t, ctx, store); got != "vcenter" {
		t.Fatalf("the driver was not stored to begin with (%q), so this proves nothing", got)
	}

	settingsPut(t, router, "/api/v1/admin/config/analysis-tools", frontendAnalysisToolsSave)

	if got := storedDriver(t, ctx, store); got != "vcenter" {
		t.Errorf("saving the Analysis Tools screen left the Test Kitchen driver as %q in the "+
			"database — an operator changed a timeout and lost the driver, the images and "+
			"the credential references, and was told the save succeeded", got)
	}
}

// And the analysis tools settings themselves are what came back out.
func TestFunctional_SettingsAnalysisToolsSaveIsWhatComesBack(t *testing.T) {
	store, _ := settingsTestStore(t)
	cfg := testConfig()
	holder := configstore.NewConfigHolder(cfg, store)
	router := newTestRouterForAdminConfig(cfg, store, holder)

	settingsPut(t, router, "/api/v1/admin/config/analysis-tools", frontendAnalysisToolsSave)

	// Read back through the screen's own GET, which is what an operator sees.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, withAdminSession(httptest.NewRequest(
		http.MethodGet, "/api/v1/admin/config/analysis-tools", nil)))
	if w.Code != http.StatusOK {
		t.Fatalf("reading the screen back answered %d: %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Value map[string]any `json:"value"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("reading what it answered: %v", err)
	}
	if got := envelope.Value["embedded_bin_dir"]; got != "/opt/chef-workstation/embedded/bin" {
		t.Errorf("the Chef tools directory an operator typed in did not come back (%v)", got)
	}
	if got := envelope.Value["cookstyle_timeout_minutes"]; got != float64(12) {
		t.Errorf("the CookStyle timeout did not come back (%v)", got)
	}
	if _, present := envelope.Value["test_kitchen"]; present {
		t.Error("the analysis tools screen answers with Test Kitchen, which it does not " +
			"carry — so it would send it back on save and be refused")
	}
}

// The other direction, against a real store.
func TestFunctional_SettingsTestKitchenSaveKeepsTheAnalysisTools(t *testing.T) {
	store, _ := settingsTestStore(t)
	ctx := context.Background()
	cfg := testConfig()
	router := newTestRouterForAdminConfig(cfg, store, configstore.NewConfigHolder(cfg, store))

	settingsPut(t, router, "/api/v1/admin/config/analysis-tools", frontendAnalysisToolsSave)
	if got := storedBinDirFunctional(t, ctx, store); got != "/opt/chef-workstation/embedded/bin" {
		t.Fatalf("the Chef tools directory was not stored to begin with (%q)", got)
	}

	settingsPut(t, router, "/api/v1/admin/config/test-kitchen", frontendTestKitchenSave)

	if got := storedBinDirFunctional(t, ctx, store); got != "/opt/chef-workstation/embedded/bin" {
		t.Errorf("saving Test Kitchen left the Chef tools directory as %q", got)
	}
}

// A deployment that still has the old nested shape is moved over, and keeps its
// settings — the thing that happens once, on upgrade, against real PostgreSQL.
func TestFunctional_SettingsAnUpgradeKeepsItsTestKitchenSettings(t *testing.T) {
	store, _ := settingsTestStore(t)
	ctx := context.Background()

	// Written the way a release before the split wrote it.
	if err := store.Set(ctx, configstore.KeyAnalysisTools, json.RawMessage(
		`{"cookstyle_timeout_minutes":12,"test_kitchen":{"driver":"proxmox","timeout_minutes":45}}`),
		false, "test"); err != nil {
		t.Fatalf("setting up the old shape: %v", err)
	}

	moved, err := configstore.MoveTestKitchenToItsOwnSection(ctx, store)
	if err != nil {
		t.Fatalf("moving it over: %v", err)
	}
	if !moved {
		t.Fatal("nothing moved, so a deployment upgrading keeps the shape that loses its " +
			"settings on the next save")
	}
	if got := storedDriver(t, ctx, store); got != "proxmox" {
		t.Errorf("the driver did not survive the move (%q)", got)
	}

	// And the whole configuration still assembles with it in place.
	cfg := testConfig()
	holder := configstore.NewConfigHolder(cfg, store)
	if err := holder.Reload(ctx); err != nil {
		t.Fatalf("reloading the configuration after the move: %v", err)
	}
	if got := holder.Get().AnalysisTools.TestKitchen.Driver; got != "proxmox" {
		t.Errorf("the running configuration reads no driver after the move (%q)", got)
	}
	if got := holder.Get().AnalysisTools.CookstyleTimeoutMinutes; got != 12 {
		t.Errorf("the move took the analysis tools settings with it (timeout %d)", got)
	}
}

// settingsPut saves one screen and fails loudly if it was refused — including
// for carrying a field no handler reads, which is what a screen that has
// stopped working looks like now.
func settingsPut(t *testing.T, router *Router, path, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, withAdminSession(req))
	if w.Code != http.StatusOK {
		t.Fatalf("saving %s answered %d: %s", path, w.Code, w.Body.String())
	}
}

// storedDriver reads the driver out of the Test Kitchen record in PostgreSQL.
func storedDriver(t *testing.T, ctx context.Context, store *configstore.Store) string {
	t.Helper()
	raw, err := store.Get(ctx, configstore.KeyTestKitchen)
	if err != nil {
		t.Fatalf("reading the Test Kitchen record: %v", err)
	}
	var kitchen config.TestKitchenConfig
	if err := configstore.DeserializeValue(raw, &kitchen); err != nil {
		t.Fatalf("reading it: %v", err)
	}
	return kitchen.Driver
}

// storedBinDirFunctional reads the Chef tools directory out of PostgreSQL.
func storedBinDirFunctional(t *testing.T, ctx context.Context, store *configstore.Store) string {
	t.Helper()
	raw, err := store.Get(ctx, configstore.KeyAnalysisTools)
	if err != nil {
		t.Fatalf("reading the analysis tools record: %v", err)
	}
	var section configstore.AnalysisToolsSection
	if err := configstore.DeserializeValue(raw, &section); err != nil {
		t.Fatalf("reading it: %v", err)
	}
	return section.EmbeddedBinDir
}
