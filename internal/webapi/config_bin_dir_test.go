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

// Where the Chef tools are, when they are not on PATH.
//
// The settings screen has had a box for this since the settings screens were
// built, and until now nothing on this side had the field at all: an operator
// typed a path in, saved, was told it worked, and scanning stayed off. That is
// the failure the strict-body work exists to make impossible, and this is the
// half of it that had to be built rather than refused.

const binDirBody = `{
	"embedded_bin_dir": "/opt/chef-workstation/embedded/bin",
	"cookstyle_timeout_minutes": 10,
	"test_kitchen_timeout_minutes": 0
}`

// The path an operator saves is the path that comes back, and the path that is
// stored. Both, because a value the response echoes but nothing persists is
// exactly as useless as one that is dropped.
func TestBinDir_TheSavedPathIsKeptAndAnsweredBack(t *testing.T) {
	store := newTestConfigStore(t)
	router := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/config/analysis-tools", strings.NewReader(binDirBody)))
	assertStatus(t, w, http.StatusOK)

	var answered map[string]any
	decodePutValue(t, w, &answered)
	if answered["embedded_bin_dir"] != "/opt/chef-workstation/embedded/bin" {
		t.Errorf("the saved path is not in what came back (%v), so an operator has no way to "+
			"tell whether it was taken", answered["embedded_bin_dir"])
	}

	stored, err := store.Get(context.Background(), configstore.KeyAnalysisTools)
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	var kept map[string]any
	if err := json.Unmarshal(stored, &kept); err != nil {
		t.Fatalf("reading what was stored: %v", err)
	}
	if kept["embedded_bin_dir"] != "/opt/chef-workstation/embedded/bin" {
		t.Errorf("the path was answered back but not stored (%v), so it is gone at the next "+
			"restart — which is the one moment it is read", kept["embedded_bin_dir"])
	}
}

// Changing where the tools are applies straight away, and must not send
// anybody off to restart the service.
//
// It comes from the database, and both executors work out where their binary is
// when they run — so the next scan uses the corrected directory and there is
// nothing to restart for.
func TestBinDir_ChangingItAppliesWithoutARestart(t *testing.T) {
	// The baseline first: a save that changes nothing resolved at startup
	// reports no restart. Without it, a handler that had stopped reporting
	// restarts altogether would pass this and take the auth and server screens'
	// notices down with it unnoticed.
	store := newTestConfigStore(t)
	router := newTestRouterForAdminConfig(nil, store, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/config/analysis-tools", strings.NewReader(validAnalysisToolsBody)))
	assertStatus(t, w, http.StatusOK)
	if binDirRestartRequired(t, w) {
		t.Fatalf("a change that touches nothing resolved at startup reported a restart, so " +
			"this test cannot tell the two apart")
	}

	// The same section, with a path that differs from the running one.
	cfg := testConfig()
	cfg.AnalysisTools.EmbeddedBinDir = "/somewhere/else"
	holder := configstore.NewConfigHolder(cfg, store)
	changed := newTestRouterForAdminConfig(cfg, store, holder)
	w = httptest.NewRecorder()
	changed.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/config/analysis-tools", strings.NewReader(binDirBody)))
	assertStatus(t, w, http.StatusOK)

	if binDirRestartRequired(t, w) {
		t.Error("moving where the Chef tools are still asks for a restart, but the path is " +
			"resolved per run now — so the operator is sent to restart a service for a change " +
			"that had already taken effect")
	}
}

// Saving the same path again is not a change, and must not send anybody off to
// restart a service for nothing.
func TestBinDir_SavingTheSamePathAgainNeedsNoRestart(t *testing.T) {
	store := newTestConfigStore(t)
	cfg := testConfig()
	cfg.AnalysisTools.EmbeddedBinDir = "/opt/chef-workstation/embedded/bin"
	holder := configstore.NewConfigHolder(cfg, store)
	router := newTestRouterForAdminConfig(cfg, store, holder)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut,
		"/api/v1/admin/config/analysis-tools", strings.NewReader(binDirBody)))
	assertStatus(t, w, http.StatusOK)

	if binDirRestartRequired(t, w) {
		t.Error("re-saving the settings without touching the path asked for a restart")
	}
}

// binDirRestartRequired reads the flag out of a settings response.
func binDirRestartRequired(t *testing.T, w *httptest.ResponseRecorder) bool {
	t.Helper()
	var envelope struct {
		RestartRequired bool `json:"restart_required"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("reading the settings response: %v (%s)", err, w.Body.String())
	}
	return envelope.RestartRequired
}
