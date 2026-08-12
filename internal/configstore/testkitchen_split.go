// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"fmt"
)

// Moving a deployment onto the split.
//
// Test Kitchen used to live nested inside the analysis tools section. Reading
// the old shape still works — assembleOneField takes the whole struct for that
// key — so nothing is lost by simply upgrading.
//
// What is lost is the first time somebody saves the Analysis Tools screen
// before ever opening the Test Kitchen one: that save writes the section
// without the nested part, and there is no record of its own yet to fall back
// to. So the nested copy is lifted out once, here.
//
// This is not a schema change. The config store is a generic key and value
// table; this reads one row and writes two.

// MoveTestKitchenToItsOwnSection lifts nested Test Kitchen settings into the
// record they now live in, and takes the nested copy out of the old one so
// there are not two answers to where they are.
//
// Reports whether anything moved. Doing it twice moves nothing the second time
// and does not touch what the screens have written since.
func MoveTestKitchenToItsOwnSection(ctx context.Context, store *Store) (bool, error) {
	// Already split. Nothing here may overwrite what the screens have written
	// since — running this again after somebody changed the driver must not
	// put the old one back.
	if _, err := store.Get(ctx, KeyTestKitchen); err == nil {
		return false, nil
	}

	raw, err := store.Get(ctx, KeyAnalysisTools)
	if err != nil {
		// A fresh install has no analysis tools record and nothing to move.
		return false, nil
	}

	var section map[string]json.RawMessage
	if err := json.Unmarshal(raw, &section); err != nil {
		return false, fmt.Errorf("configstore: reading the analysis tools section: %w", err)
	}
	nested, ok := section["test_kitchen"]
	if !ok || len(nested) == 0 || string(nested) == "null" {
		// Configured before Test Kitchen was ever set up. Writing an empty
		// record here would read as a deliberate one later.
		return false, nil
	}

	if err := store.Set(ctx, KeyTestKitchen, nested, false, "config split"); err != nil {
		return false, fmt.Errorf("configstore: writing the Test Kitchen section: %w", err)
	}

	// Only once the new record is safely written. The other order loses the
	// settings if the second write fails.
	delete(section, "test_kitchen")
	rest, err := json.Marshal(section)
	if err != nil {
		return false, fmt.Errorf("configstore: rewriting the analysis tools section: %w", err)
	}
	if err := store.Set(ctx, KeyAnalysisTools, rest, false, "config split"); err != nil {
		return false, fmt.Errorf("configstore: storing the analysis tools section: %w", err)
	}
	return true, nil
}
