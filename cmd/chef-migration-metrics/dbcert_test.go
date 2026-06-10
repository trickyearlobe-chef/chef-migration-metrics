// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// memConfigDB is a minimal in-memory configstore.DatastoreDB for exercising
// loadDBCertKey without a real database.
type memConfigDB struct {
	entries map[string]*datastore.ConfigEntry
}

func newMemConfigDB() *memConfigDB {
	return &memConfigDB{entries: map[string]*datastore.ConfigEntry{}}
}

func (m *memConfigDB) GetConfigEntry(_ context.Context, key string) (*datastore.ConfigEntry, error) {
	return m.entries[key], nil
}

func (m *memConfigDB) SetConfigEntry(_ context.Context, e *datastore.ConfigEntry) error {
	cp := *e
	m.entries[e.Key] = &cp
	return nil
}

func (m *memConfigDB) DeleteConfigEntry(_ context.Context, key string) error {
	delete(m.entries, key)
	return nil
}

func (m *memConfigDB) ListConfigEntries(_ context.Context) ([]datastore.ConfigEntry, error) {
	out := make([]datastore.ConfigEntry, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, *e)
	}
	return out, nil
}

func (m *memConfigDB) ListConfigEntriesByPrefix(_ context.Context, _ string) ([]datastore.ConfigEntry, error) {
	return m.ListConfigEntries(context.Background())
}

func (m *memConfigDB) CountConfigEntries(_ context.Context) (int, error) {
	return len(m.entries), nil
}

func (m *memConfigDB) ConfigStoreIsEmpty(_ context.Context) (bool, error) {
	return len(m.entries) == 0, nil
}

func newMemStore(t *testing.T) *configstore.Store {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	store, err := configstore.NewStoreWithKey(newMemConfigDB(), key)
	if err != nil {
		t.Fatalf("NewStoreWithKey: %v", err)
	}
	return store
}

// loadDBCertKey returns the stored cert/key PEM round-tripped through the
// encrypted config store.
func TestLoadDBCertKey_RoundTrip(t *testing.T) {
	store := newMemStore(t)
	ctx := context.Background()

	certJSON, _ := json.Marshal("CERT-PEM")
	keyJSON, _ := json.Marshal("KEY-PEM")
	if err := store.Set(ctx, configstore.KeyServerTLSCertificate, certJSON, false, "admin"); err != nil {
		t.Fatalf("set cert: %v", err)
	}
	if err := store.Set(ctx, configstore.KeyServerTLSPrivateKey, keyJSON, true, "admin"); err != nil {
		t.Fatalf("set key: %v", err)
	}

	app := &serverApp{cfgStore: store}
	certPEM, keyPEM, err := app.loadDBCertKey(ctx)
	if err != nil {
		t.Fatalf("loadDBCertKey = %v, want nil", err)
	}
	if string(certPEM) != "CERT-PEM" || string(keyPEM) != "KEY-PEM" {
		t.Errorf("got cert=%q key=%q, want CERT-PEM/KEY-PEM", certPEM, keyPEM)
	}
}

// A missing certificate is an error (caller fails open to plain HTTP).
func TestLoadDBCertKey_MissingCert(t *testing.T) {
	app := &serverApp{cfgStore: newMemStore(t)}
	if _, _, err := app.loadDBCertKey(context.Background()); err == nil {
		t.Fatal("loadDBCertKey(missing cert) = nil, want error")
	}
}

// A nil config store is an error (db source needs the encryption key).
func TestLoadDBCertKey_NilStore(t *testing.T) {
	app := &serverApp{}
	if _, _, err := app.loadDBCertKey(context.Background()); err == nil {
		t.Fatal("loadDBCertKey(nil store) = nil, want error")
	}
}
