// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

func TestStatusEmptyWhenUnset(t *testing.T) {
	st := NewStorage(newFakeStore())
	got, err := st.Status(context.Background())
	if err != nil {
		t.Fatalf("Status on empty store: %v", err)
	}
	if (got != Status{}) {
		t.Errorf("Status on empty store = %+v, want zero", got)
	}
}

func TestUpdateStatusPersists(t *testing.T) {
	st := NewStorage(newFakeStore())
	ctx := context.Background()
	if err := st.UpdateStatus(ctx, func(s *Status) {
		s.LastRenewal = "2026-06-10T00:00:00Z"
	}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	got, err := st.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got.LastRenewal != "2026-06-10T00:00:00Z" {
		t.Errorf("LastRenewal = %q, want the persisted value", got.LastRenewal)
	}
}

// UpdateStatus must read-modify-write so independent writers (renewal outcome vs
// hostname-registration outcome) do not clobber each other's fields.
func TestUpdateStatusPreservesOtherFields(t *testing.T) {
	st := NewStorage(newFakeStore())
	ctx := context.Background()
	if err := st.UpdateStatus(ctx, func(s *Status) { s.HostnameError = "no IPv4 detectable" }); err != nil {
		t.Fatalf("UpdateStatus hostname: %v", err)
	}
	if err := st.UpdateStatus(ctx, func(s *Status) {
		s.LastRenewal = "2026-06-10T00:00:00Z"
		s.LastError = ""
	}); err != nil {
		t.Fatalf("UpdateStatus renewal: %v", err)
	}
	got, _ := st.Status(ctx)
	if got.HostnameError != "no IPv4 detectable" {
		t.Errorf("HostnameError clobbered: %q", got.HostnameError)
	}
	if got.LastRenewal != "2026-06-10T00:00:00Z" {
		t.Errorf("LastRenewal = %q", got.LastRenewal)
	}
}

// The status entry is operator-visible metadata, never a secret: it must be
// stored non-secret so the admin GET can read it via the public Get path.
func TestStatusStoredNonSecret(t *testing.T) {
	fs := newFakeStore()
	st := NewStorage(fs)
	ctx := context.Background()
	if err := st.UpdateStatus(ctx, func(s *Status) { s.LastError = "boom" }); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if fs.secrets[configstore.KeyServerTLSACMEStatus] {
		t.Error("status entry stored as secret; want non-secret")
	}
	if _, err := fs.Get(ctx, configstore.KeyServerTLSACMEStatus); err != nil {
		t.Errorf("status not readable via non-secret Get: %v", err)
	}
}
