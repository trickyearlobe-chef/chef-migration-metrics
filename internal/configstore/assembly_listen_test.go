// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// listen_address/port round-trip through the server.listen section: a config
// serialised by ConfigToSections must reassemble with the same listen target.
func TestServerListen_RoundTrip(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.ListenAddress = "127.0.0.1"
	cfg.Server.Port = 9443

	sections, err := ConfigToSections(cfg)
	if err != nil {
		t.Fatalf("ConfigToSections: %v", err)
	}

	raw, ok := sections[KeyServerListen]
	if !ok {
		t.Fatalf("ConfigToSections did not produce a %q section", KeyServerListen)
	}

	// The stored value is snake_case JSON.
	var section ServerListenSection
	if err := json.Unmarshal(raw, &section); err != nil {
		t.Fatalf("unmarshal section: %v", err)
	}
	if section.ListenAddress != "127.0.0.1" || section.Port != 9443 {
		t.Errorf("section = %+v, want {127.0.0.1 9443}", section)
	}

	assembled, err := AssembleConfigRaw(sections)
	if err != nil {
		t.Fatalf("AssembleConfigRaw: %v", err)
	}
	if assembled.Server.ListenAddress != "127.0.0.1" {
		t.Errorf("ListenAddress = %q, want 127.0.0.1", assembled.Server.ListenAddress)
	}
	if assembled.Server.Port != 9443 {
		t.Errorf("Port = %d, want 9443", assembled.Server.Port)
	}
}

// HasKey reports presence/absence so callers can distinguish a DB-sourced
// section from an absent one (used for the listen bootstrap fallback).
func TestHasKey(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB()
	store := mustNewStore(t, db)

	present, err := HasKey(ctx, store, KeyServerListen)
	if err != nil {
		t.Fatalf("HasKey (absent): %v", err)
	}
	if present {
		t.Fatal("expected server.listen absent before write")
	}

	if err := store.Set(ctx, KeyServerListen, json.RawMessage(`{"listen_address":"0.0.0.0","port":8080}`), false, "test"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	present, err = HasKey(ctx, store, KeyServerListen)
	if err != nil {
		t.Fatalf("HasKey (present): %v", err)
	}
	if !present {
		t.Fatal("expected server.listen present after write")
	}
}
