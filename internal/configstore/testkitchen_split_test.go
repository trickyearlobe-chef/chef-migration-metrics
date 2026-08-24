// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// Test Kitchen keeps its own stored section.
//
// Nested inside the analysis tools section it would share a section with a
// second screen, and a screen replaces the whole section with what it was sent —
// so changing a CookStyle timeout would drop the driver, the images, the
// credential references and the rate limits, and report a successful save.
//
// Two records instead, the way the server settings are already split, so
// neither screen can reach the other's. What each screen sends is still exactly
// what gets stored, so clearing a setting keeps working.

// analysisToolsWithKitchen is a configuration with both parts filled in, so a
// test can tell which of the two a section carries.
func analysisToolsWithKitchen() *config.Config {
	cfg := &config.Config{}
	cfg.AnalysisTools = config.AnalysisToolsConfig{
		CookstyleTimeoutMinutes: 10,
		EmbeddedBinDir:          "/opt/chef-workstation/embedded/bin",
		TestKitchen: config.TestKitchenConfig{
			Driver:         "vcenter",
			TimeoutMinutes: 45,
			VMNamePrefix:   "example-",
		},
	}
	return cfg
}

// The two are stored apart. Nothing that writes one can reach the other.
func TestSections_TestKitchenIsStoredApartFromTheAnalysisTools(t *testing.T) {
	sections, err := ConfigToSections(analysisToolsWithKitchen())
	if err != nil {
		t.Fatalf("splitting the configuration into sections: %v", err)
	}

	tools, ok := sections[KeyAnalysisTools]
	if !ok {
		t.Fatal("there is no analysis tools section at all")
	}
	var toolsFields map[string]any
	if err := json.Unmarshal(tools, &toolsFields); err != nil {
		t.Fatalf("reading the analysis tools section: %v", err)
	}
	// The baseline: it still carries its own settings. Without this, a section
	// that had become empty would pass the check below while losing everything.
	if toolsFields["cookstyle_timeout_minutes"] == nil {
		t.Errorf("the analysis tools section no longer carries its own settings: %v", toolsFields)
	}
	if _, nested := toolsFields["test_kitchen"]; nested {
		t.Error("the analysis tools section still carries Test Kitchen inside it, so saving " +
			"one screen still overwrites the other's settings")
	}

	kitchen, ok := sections[KeyTestKitchen]
	if !ok {
		t.Fatalf("Test Kitchen has no section of its own; the keys are %v", sectionKeys(sections))
	}
	var kitchenFields map[string]any
	if err := json.Unmarshal(kitchen, &kitchenFields); err != nil {
		t.Fatalf("reading the Test Kitchen section: %v", err)
	}
	if kitchenFields["driver"] != "vcenter" {
		t.Errorf("the Test Kitchen section does not carry the driver: %v", kitchenFields)
	}
}

// Split apart and put back together is the configuration that went in. A split
// that could not be reassembled would lose the settings on the next read
// instead of on the next save.
func TestSections_TheTwoComeBackTogetherAsOneConfiguration(t *testing.T) {
	sections, err := ConfigToSections(analysisToolsWithKitchen())
	if err != nil {
		t.Fatalf("splitting: %v", err)
	}

	assembled, err := AssembleConfigRaw(sections)
	if err != nil {
		t.Fatalf("reassembling: %v", err)
	}

	if got := assembled.AnalysisTools.TestKitchen.Driver; got != "vcenter" {
		t.Errorf("the Test Kitchen driver did not survive the round trip (%q)", got)
	}
	if got := assembled.AnalysisTools.TestKitchen.VMNamePrefix; got != "example-" {
		t.Errorf("the Test Kitchen VM name prefix did not survive the round trip (%q)", got)
	}
	if got := assembled.AnalysisTools.CookstyleTimeoutMinutes; got != 10 {
		t.Errorf("the CookStyle timeout did not survive the round trip (%d)", got)
	}
	if got := assembled.AnalysisTools.EmbeddedBinDir; got != "/opt/chef-workstation/embedded/bin" {
		t.Errorf("the Chef tools directory did not survive the round trip (%q)", got)
	}
}

// A deployment upgrading has Test Kitchen nested in the stored analysis tools
// section, because that is where it has always been. Reading it must still
// work, or upgrading loses the settings before anything has a chance to move
// them.
func TestSections_TheOldNestedShapeIsStillRead(t *testing.T) {
	sections := map[string]json.RawMessage{
		KeyAnalysisTools: json.RawMessage(
			`{"cookstyle_timeout_minutes":10,"test_kitchen":{"driver":"vcenter"}}`),
	}

	assembled, err := AssembleConfigRaw(sections)
	if err != nil {
		t.Fatalf("reading a store written before the split: %v", err)
	}
	if got := assembled.AnalysisTools.TestKitchen.Driver; got != "vcenter" {
		t.Errorf("a deployment that has not been moved over reads no Test Kitchen driver "+
			"(%q), so upgrading loses it", got)
	}
}

// Where both shapes are present, the section of its own wins. It is the one
// the screens now write; the nested copy is what was there before.
func TestSections_TheSectionOfItsOwnWinsOverTheNestedCopy(t *testing.T) {
	sections := map[string]json.RawMessage{
		KeyAnalysisTools: json.RawMessage(
			`{"cookstyle_timeout_minutes":10,"test_kitchen":{"driver":"stale"}}`),
		KeyTestKitchen: json.RawMessage(`{"driver":"vcenter"}`),
	}

	assembled, err := AssembleConfigRaw(sections)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if got := assembled.AnalysisTools.TestKitchen.Driver; got != "vcenter" {
		t.Errorf("the stale nested copy won (%q), so the screens write somewhere nothing "+
			"reads", got)
	}
}

// ---------------------------------------------------------------------------
// Moving an existing deployment over
// ---------------------------------------------------------------------------

// The nested settings are lifted into their own record, once.
func TestMoveTestKitchen_LiftsTheNestedSettingsIntoTheirOwnRecord(t *testing.T) {
	ctx := context.Background()
	store := mustNewStore(t, newFakeDB())

	// A deployment as it was before the split.
	if err := store.Set(ctx, KeyAnalysisTools, json.RawMessage(
		`{"cookstyle_timeout_minutes":10,"test_kitchen":{"driver":"vcenter","timeout_minutes":45}}`),
		false, "test"); err != nil {
		t.Fatalf("setting up the old shape: %v", err)
	}

	moved, err := MoveTestKitchenToItsOwnSection(ctx, store)
	if err != nil {
		t.Fatalf("moving it: %v", err)
	}
	if !moved {
		t.Fatal("nothing was moved, so a deployment upgrading keeps the shape that loses " +
			"its settings on the next save")
	}

	raw, err := store.Get(ctx, KeyTestKitchen)
	if err != nil {
		t.Fatalf("reading the new record: %v", err)
	}
	var kitchen map[string]any
	if err := json.Unmarshal(raw, &kitchen); err != nil {
		t.Fatalf("reading it: %v", err)
	}
	if kitchen["driver"] != "vcenter" {
		t.Errorf("the moved record does not carry the driver: %v", kitchen)
	}

	// And the nested copy is gone, so nothing can read the stale one later.
	toolsRaw, err := store.Get(ctx, KeyAnalysisTools)
	if err != nil {
		t.Fatalf("reading the analysis tools record: %v", err)
	}
	var tools map[string]any
	if err := json.Unmarshal(toolsRaw, &tools); err != nil {
		t.Fatalf("reading it: %v", err)
	}
	if _, nested := tools["test_kitchen"]; nested {
		t.Error("the nested copy was left behind, so there are two answers to where the " +
			"Test Kitchen settings are")
	}
	if tools["cookstyle_timeout_minutes"] == nil {
		t.Error("moving Test Kitchen out took the analysis tools settings with it")
	}
}

// Running it twice does nothing the second time, and does not overwrite what
// the screens have since written.
func TestMoveTestKitchen_RunningItAgainChangesNothing(t *testing.T) {
	ctx := context.Background()
	store := mustNewStore(t, newFakeDB())

	if err := store.Set(ctx, KeyAnalysisTools, json.RawMessage(
		`{"cookstyle_timeout_minutes":10,"test_kitchen":{"driver":"vcenter"}}`),
		false, "test"); err != nil {
		t.Fatalf("setting up: %v", err)
	}
	if _, err := MoveTestKitchenToItsOwnSection(ctx, store); err != nil {
		t.Fatalf("first move: %v", err)
	}

	// Somebody changes the driver through the Test Kitchen screen.
	if err := store.Set(ctx, KeyTestKitchen,
		json.RawMessage(`{"driver":"proxmox"}`), false, "test"); err != nil {
		t.Fatalf("changing it afterwards: %v", err)
	}

	moved, err := MoveTestKitchenToItsOwnSection(ctx, store)
	if err != nil {
		t.Fatalf("second move: %v", err)
	}
	if moved {
		t.Error("it moved something on the second run, having already moved it once")
	}

	raw, err := store.Get(ctx, KeyTestKitchen)
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	var kitchen map[string]any
	if err := json.Unmarshal(raw, &kitchen); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if kitchen["driver"] != "proxmox" {
		t.Errorf("running it again overwrote what somebody had since set (%v)", kitchen["driver"])
	}
}

// A deployment that never had Test Kitchen configured has nothing to move, and
// must not end up with an empty record that then reads as deliberate.
func TestMoveTestKitchen_WithNothingToMoveWritesNothing(t *testing.T) {
	ctx := context.Background()
	store := mustNewStore(t, newFakeDB())

	if err := store.Set(ctx, KeyAnalysisTools,
		json.RawMessage(`{"cookstyle_timeout_minutes":10}`), false, "test"); err != nil {
		t.Fatalf("setting up: %v", err)
	}

	moved, err := MoveTestKitchenToItsOwnSection(ctx, store)
	if err != nil {
		t.Fatalf("moving: %v", err)
	}
	if moved {
		t.Error("it reported moving Test Kitchen settings that were never there")
	}
	if _, err := store.Get(ctx, KeyTestKitchen); err == nil {
		t.Error("a record was written for a deployment that never configured Test Kitchen")
	}
}

// A store with no analysis tools record at all — a fresh install — is not an
// error.
func TestMoveTestKitchen_AFreshInstallIsNotAFailure(t *testing.T) {
	moved, err := MoveTestKitchenToItsOwnSection(
		context.Background(), mustNewStore(t, newFakeDB()))
	if err != nil {
		t.Fatalf("a fresh install failed to start: %v", err)
	}
	if moved {
		t.Error("something was moved on a store with nothing in it")
	}
}

// sectionKeys lists what a split produced, for a failure message.
func sectionKeys(sections map[string]json.RawMessage) []string {
	out := make([]string, 0, len(sections))
	for k := range sections {
		out = append(out, k)
	}
	return out
}
