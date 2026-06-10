// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// readTrustedProxy decodes the stored server.trusted_proxy scalar.
func readTrustedProxy(t *testing.T, store *configstore.Store) bool {
	t.Helper()
	raw, err := store.Get(context.Background(), configstore.KeyServerTrustedProxy)
	if err != nil {
		t.Fatalf("get server.trusted_proxy: %v", err)
	}
	var v bool
	if err := configstore.DeserializeValue(raw, &v); err != nil {
		t.Fatalf("DeserializeValue: %v", err)
	}
	return v
}

// tls mode <m> sets server.tls.mode to each valid value, preserving other
// fields so switching deployment modes never loses cert paths/source.
func TestTLSSetMode_SetsEachModePreservingFields(t *testing.T) {
	for _, mode := range []string{"off", "static", "acme"} {
		store := newMemStore(t)
		ctx := context.Background()
		seedTLSSection(t, store, config.TLSConfig{
			Mode:       "static",
			CertSource: "db",
			CertPath:   "/etc/cmm/cert.pem",
			CAPath:     "/etc/cmm/ca.pem",
			MinVersion: "1.2",
		})

		res, err := tlsSetMode(ctx, store, mode, "repair-cli")
		if err != nil {
			t.Fatalf("tlsSetMode(%q): %v", mode, err)
		}
		// static→static is a no-op; the others are changes.
		wantRes := repairChanged
		if mode == "static" {
			wantRes = repairNoChange
		}
		if res != wantRes {
			t.Fatalf("mode %q: result = %v, want %v", mode, res, wantRes)
		}

		got := readTLSSection(t, store)
		if got.Mode != mode {
			t.Errorf("mode = %q, want %q", got.Mode, mode)
		}
		if got.CertSource != "db" || got.CertPath != "/etc/cmm/cert.pem" || got.CAPath != "/etc/cmm/ca.pem" || got.MinVersion != "1.2" {
			t.Errorf("non-mode fields not preserved: %+v", got)
		}
	}
}

// tls mode with no server.tls section reports nothing to do and writes nothing
// (does not create a config-shadowing section).
func TestTLSSetMode_NoSection(t *testing.T) {
	store := newMemStore(t)
	ctx := context.Background()

	res, err := tlsSetMode(ctx, store, "off", "repair-cli")
	if err != nil {
		t.Fatalf("tlsSetMode: %v", err)
	}
	if res != repairNoSection {
		t.Errorf("result = %v, want repairNoSection", res)
	}
	if n, _ := store.Count(ctx); n != 0 {
		t.Errorf("store wrote %d entries, want 0", n)
	}
}

// tlsResetMode (the recovery alias) still sets mode off via the generalised path.
func TestTLSResetMode_DelegatesToSetMode(t *testing.T) {
	store := newMemStore(t)
	ctx := context.Background()
	seedTLSSection(t, store, config.TLSConfig{Mode: "acme"})

	res, err := tlsResetMode(ctx, store, "repair-cli")
	if err != nil {
		t.Fatalf("tlsResetMode: %v", err)
	}
	if res != repairChanged {
		t.Fatalf("result = %v, want repairChanged", res)
	}
	if got := readTLSSection(t, store); got.Mode != "off" {
		t.Errorf("mode = %q, want off", got.Mode)
	}
}

// --trusted-proxy sets server.trusted_proxy true and false; idempotent.
func TestTLSSetTrustedProxy(t *testing.T) {
	store := newMemStore(t)
	ctx := context.Background()

	res, err := tlsSetTrustedProxy(ctx, store, true, "repair-cli")
	if err != nil {
		t.Fatalf("set true: %v", err)
	}
	if res != repairChanged {
		t.Fatalf("set true: result = %v, want repairChanged", res)
	}
	if !readTrustedProxy(t, store) {
		t.Error("trusted_proxy = false, want true")
	}

	// Idempotent: setting true again is a no-op.
	res, err = tlsSetTrustedProxy(ctx, store, true, "repair-cli")
	if err != nil {
		t.Fatalf("set true again: %v", err)
	}
	if res != repairNoChange {
		t.Errorf("set true again: result = %v, want repairNoChange", res)
	}

	// Flip back to false.
	res, err = tlsSetTrustedProxy(ctx, store, false, "repair-cli")
	if err != nil {
		t.Fatalf("set false: %v", err)
	}
	if res != repairChanged {
		t.Fatalf("set false: result = %v, want repairChanged", res)
	}
	if readTrustedProxy(t, store) {
		t.Error("trusted_proxy = true, want false")
	}
}

// trusted_proxy defaults to false when the key is absent, so setting false on a
// fresh store is a no-op (no orphan key written).
func TestTLSSetTrustedProxy_FalseWhenAbsentIsNoChange(t *testing.T) {
	store := newMemStore(t)
	ctx := context.Background()

	res, err := tlsSetTrustedProxy(ctx, store, false, "repair-cli")
	if err != nil {
		t.Fatalf("tlsSetTrustedProxy: %v", err)
	}
	if res != repairNoChange {
		t.Errorf("result = %v, want repairNoChange", res)
	}
	if n, _ := store.Count(ctx); n != 0 {
		t.Errorf("store wrote %d entries, want 0", n)
	}
}

// parseTLSModeArgs extracts the mode and the optional --trusted-proxy flag in
// any order, defaulting a bare flag to true and parsing an explicit bool value.
func TestParseTLSModeArgs(t *testing.T) {
	bptr := func(b bool) *bool { return &b }
	tests := []struct {
		name    string
		args    []string
		mode    string
		tp      *bool
		wantErr bool
	}{
		{name: "mode only", args: []string{"off"}, mode: "off", tp: nil},
		{name: "static", args: []string{"static"}, mode: "static", tp: nil},
		{name: "acme", args: []string{"acme"}, mode: "acme", tp: nil},
		{name: "bare flag", args: []string{"off", "--trusted-proxy"}, mode: "off", tp: bptr(true)},
		{name: "flag true", args: []string{"off", "--trusted-proxy=true"}, mode: "off", tp: bptr(true)},
		{name: "flag false", args: []string{"off", "--trusted-proxy=false"}, mode: "off", tp: bptr(false)},
		{name: "flag before mode", args: []string{"--trusted-proxy", "off"}, mode: "off", tp: bptr(true)},
		{name: "no mode", args: []string{}, wantErr: true},
		{name: "bad mode", args: []string{"bogus"}, wantErr: true},
		{name: "bad flag value", args: []string{"off", "--trusted-proxy=maybe"}, wantErr: true},
		{name: "two modes", args: []string{"off", "static"}, wantErr: true},
		{name: "unknown flag", args: []string{"off", "--wat"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mode, tp, err := parseTLSModeArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got mode=%q tp=%v", mode, tp)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode != tc.mode {
				t.Errorf("mode = %q, want %q", mode, tc.mode)
			}
			switch {
			case tc.tp == nil && tp != nil:
				t.Errorf("tp = %v, want nil", *tp)
			case tc.tp != nil && tp == nil:
				t.Errorf("tp = nil, want %v", *tc.tp)
			case tc.tp != nil && tp != nil && *tp != *tc.tp:
				t.Errorf("tp = %v, want %v", *tp, *tc.tp)
			}
		})
	}
}
