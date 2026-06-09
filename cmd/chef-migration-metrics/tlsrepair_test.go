// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// seedTLSSection writes a server.tls section into the store the same way the
// admin save path does (snake_case yaml tags).
func seedTLSSection(t *testing.T, store *configstore.Store, tls config.TLSConfig) {
	t.Helper()
	raw, err := configstore.SerializeValue(tls)
	if err != nil {
		t.Fatalf("SerializeValue: %v", err)
	}
	if err := store.Set(context.Background(), configstore.KeyServerTLS, raw, false, "admin"); err != nil {
		t.Fatalf("seed server.tls: %v", err)
	}
}

// readTLSSection decodes the stored server.tls section back into a TLSConfig.
func readTLSSection(t *testing.T, store *configstore.Store) config.TLSConfig {
	t.Helper()
	raw, err := store.Get(context.Background(), configstore.KeyServerTLS)
	if err != nil {
		t.Fatalf("get server.tls: %v", err)
	}
	var tls config.TLSConfig
	if err := configstore.DeserializeValue(raw, &tls); err != nil {
		t.Fatalf("DeserializeValue: %v", err)
	}
	return tls
}

// tls reset sets mode=off and preserves the rest of the section so re-enabling
// TLS later does not lose the operator's cert paths/source.
func TestTLSResetMode_SetsOffPreservingFields(t *testing.T) {
	store := newMemStore(t)
	ctx := context.Background()
	seedTLSSection(t, store, config.TLSConfig{
		Mode:       "static",
		CertSource: "db",
		CertPath:   "/etc/cmm/cert.pem",
		CAPath:     "/etc/cmm/ca.pem",
		MinVersion: "1.2",
	})

	res, err := tlsResetMode(ctx, store, "repair-cli")
	if err != nil {
		t.Fatalf("tlsResetMode: %v", err)
	}
	if res != repairChanged {
		t.Fatalf("result = %v, want repairChanged", res)
	}

	got := readTLSSection(t, store)
	if got.Mode != "off" {
		t.Errorf("mode = %q, want off", got.Mode)
	}
	if got.CertSource != "db" || got.CertPath != "/etc/cmm/cert.pem" || got.CAPath != "/etc/cmm/ca.pem" || got.MinVersion != "1.2" {
		t.Errorf("non-mode fields not preserved: %+v", got)
	}
}

// tls reset is idempotent — running it when already off is a no-op.
func TestTLSResetMode_AlreadyOff(t *testing.T) {
	store := newMemStore(t)
	ctx := context.Background()
	seedTLSSection(t, store, config.TLSConfig{Mode: "off"})

	res, err := tlsResetMode(ctx, store, "repair-cli")
	if err != nil {
		t.Fatalf("tlsResetMode: %v", err)
	}
	if res != repairNoChange {
		t.Errorf("result = %v, want repairNoChange", res)
	}
}

// tls reset with no server.tls section in the DB reports nothing to do and does
// not create a (config-shadowing) section.
func TestTLSResetMode_NoSection(t *testing.T) {
	store := newMemStore(t)
	ctx := context.Background()

	res, err := tlsResetMode(ctx, store, "repair-cli")
	if err != nil {
		t.Fatalf("tlsResetMode: %v", err)
	}
	if res != repairNoSection {
		t.Errorf("result = %v, want repairNoSection", res)
	}
	if n, _ := store.Count(ctx); n != 0 {
		t.Errorf("store wrote %d entries, want 0 (no shadowing section)", n)
	}
}

// tls clear-ca removes ca_path but keeps TLS on (mode and other fields intact).
func TestTLSClearCA_RemovesCAKeepingTLSOn(t *testing.T) {
	store := newMemStore(t)
	ctx := context.Background()
	seedTLSSection(t, store, config.TLSConfig{
		Mode:       "static",
		CertSource: "db",
		CAPath:     "/etc/cmm/ca.pem",
	})

	res, err := tlsClearCA(ctx, store, "repair-cli")
	if err != nil {
		t.Fatalf("tlsClearCA: %v", err)
	}
	if res != repairChanged {
		t.Fatalf("result = %v, want repairChanged", res)
	}

	got := readTLSSection(t, store)
	if got.CAPath != "" {
		t.Errorf("ca_path = %q, want empty", got.CAPath)
	}
	if got.Mode != "static" || got.CertSource != "db" {
		t.Errorf("TLS turned off or fields lost: %+v", got)
	}
}

// tls clear-ca is idempotent — no ca_path set is a no-op.
func TestTLSClearCA_AlreadyClear(t *testing.T) {
	store := newMemStore(t)
	ctx := context.Background()
	seedTLSSection(t, store, config.TLSConfig{Mode: "static"})

	res, err := tlsClearCA(ctx, store, "repair-cli")
	if err != nil {
		t.Fatalf("tlsClearCA: %v", err)
	}
	if res != repairNoChange {
		t.Errorf("result = %v, want repairNoChange", res)
	}
}

// tls clear-ca with no server.tls section reports nothing to do.
func TestTLSClearCA_NoSection(t *testing.T) {
	store := newMemStore(t)
	ctx := context.Background()

	res, err := tlsClearCA(ctx, store, "repair-cli")
	if err != nil {
		t.Fatalf("tlsClearCA: %v", err)
	}
	if res != repairNoSection {
		t.Errorf("result = %v, want repairNoSection", res)
	}
}
