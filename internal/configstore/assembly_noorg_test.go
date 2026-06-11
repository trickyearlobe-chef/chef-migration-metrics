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
	tv, err := json.Marshal([]string{"18.5.0"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := store.Set(ctx, KeyTargetChefVersions, tv, false, "test"); err != nil {
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
