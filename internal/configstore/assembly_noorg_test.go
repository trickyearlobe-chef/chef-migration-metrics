// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Reproduces the startup failure seen when the last organisation is removed but
// other config sections remain in the store (e.g. leftover TLS/ACME): config
// assembly must NOT treat zero organisations as fatal — the server has to boot
// into the setup wizard so the operator can add an org via the UI. The previous
// behaviour aborted startup with "at least one organisation must be configured".
func TestAssembleConfig_NoOrganisations_BootsWithWarning(t *testing.T) {
	ctx := context.Background()
	store := mustNewStore(t, newFakeDB())

	// A non-org section present in the store (stands in for leftover TLS/ACME),
	// but no organisations section at all.
	tv, err := json.Marshal("18.5.0")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := store.Set(ctx, KeyTargetChefVersion, tv, false, "test"); err != nil {
		t.Fatalf("seed section: %v", err)
	}

	cfg, warnings, err := AssembleConfig(ctx, store)
	if err != nil {
		t.Fatalf("AssembleConfig with zero organisations must not be fatal, got: %v", err)
	}
	if cfg == nil || len(cfg.Organisations) != 0 {
		t.Fatalf("expected an assembled config with zero organisations")
	}
	found := false
	if warnings != nil {
		for _, m := range warnings.Messages {
			if strings.Contains(m, "organisation") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected a no-organisations setup warning, got %v", warnings)
	}
}

// Back-compat: a store still holding the pre-single-target legacy key
// (target_chef_versions as a JSON array) must assemble into the scalar
// TargetChefVersion, picking the highest version.
func TestAssembleConfig_LegacyTargetChefVersions_MigratesToHighest(t *testing.T) {
	ctx := context.Background()
	store := mustNewStore(t, newFakeDB())

	legacy, err := json.Marshal([]string{"17.0.0", "18.0.0"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := store.Set(ctx, KeyTargetChefVersionsLegacy, legacy, false, "test"); err != nil {
		t.Fatalf("seed legacy section: %v", err)
	}

	cfg, _, err := AssembleConfig(ctx, store)
	if err != nil {
		t.Fatalf("AssembleConfig: %v", err)
	}
	if cfg.TargetChefVersion != "18.0.0" {
		t.Errorf("TargetChefVersion: got %q, want %q", cfg.TargetChefVersion, "18.0.0")
	}
}
