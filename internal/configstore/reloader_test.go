// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// ---------------------------------------------------------------------------
// Get / Set — basic functionality
// ---------------------------------------------------------------------------

func TestConfigHolder_GetReturnsInitialConfig(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{Level: "DEBUG"},
	}
	holder := NewConfigHolder(cfg, nil)

	got := holder.Get()
	if got == nil {
		t.Fatal("expected non-nil config")
	}
	if got.Logging.Level != "DEBUG" {
		t.Errorf("Logging.Level = %q, want %q", got.Logging.Level, "DEBUG")
	}
}

func TestConfigHolder_SetReplacesConfig(t *testing.T) {
	cfg1 := &config.Config{
		Logging: config.LoggingConfig{Level: "INFO"},
	}
	cfg2 := &config.Config{
		Logging: config.LoggingConfig{Level: "ERROR"},
	}

	holder := NewConfigHolder(cfg1, nil)

	if got := holder.Get(); got.Logging.Level != "INFO" {
		t.Fatalf("initial config: Logging.Level = %q, want %q", got.Logging.Level, "INFO")
	}

	holder.Set(cfg2)

	if got := holder.Get(); got.Logging.Level != "ERROR" {
		t.Errorf("after Set: Logging.Level = %q, want %q", got.Logging.Level, "ERROR")
	}
}

func TestConfigHolder_SetToNil(t *testing.T) {
	cfg := &config.Config{}
	holder := NewConfigHolder(cfg, nil)

	holder.Set(nil)

	got := holder.Get()
	if got != nil {
		t.Errorf("expected nil after Set(nil), got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// Get returns the same pointer until Set/Reload is called
// ---------------------------------------------------------------------------

func TestConfigHolder_GetReturnsSamePointer(t *testing.T) {
	cfg := &config.Config{}
	holder := NewConfigHolder(cfg, nil)

	a := holder.Get()
	b := holder.Get()
	if a != b {
		t.Error("consecutive Get() calls should return the same pointer")
	}
}

// ---------------------------------------------------------------------------
// Concurrent reads are safe
// ---------------------------------------------------------------------------

func TestConfigHolder_ConcurrentReads(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{Level: "WARN"},
	}
	holder := NewConfigHolder(cfg, nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := holder.Get()
			if got == nil {
				t.Error("Get returned nil during concurrent read")
			}
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Concurrent reads and writes are safe
// ---------------------------------------------------------------------------

func TestConfigHolder_ConcurrentReadWrite(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{Level: "INFO"},
	}
	holder := NewConfigHolder(cfg, nil)

	var wg sync.WaitGroup

	// Writers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			newCfg := &config.Config{
				Logging: config.LoggingConfig{Level: "LEVEL"},
			}
			holder.Set(newCfg)
		}(i)
	}

	// Readers.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := holder.Get()
			// The config should always be non-nil since we never Set(nil).
			if got == nil {
				t.Error("Get returned nil during concurrent read/write")
			}
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Reload — no store returns error
// ---------------------------------------------------------------------------

func TestConfigHolder_Reload_NoStore(t *testing.T) {
	cfg := &config.Config{}
	holder := NewConfigHolder(cfg, nil)

	err := holder.Reload(context.Background())
	if err == nil {
		t.Fatal("expected error when store is nil")
	}
	if !containsString(err.Error(), "no store configured") {
		t.Errorf("expected 'no store configured' error, got: %v", err)
	}

	// Config should be unchanged.
	if holder.Get() != cfg {
		t.Error("config should be unchanged after failed reload")
	}
}

// ---------------------------------------------------------------------------
// Reload — assembles from store and swaps config
// ---------------------------------------------------------------------------

func TestConfigHolder_Reload_SwapsConfig(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	// Seed the config store with enough data for a valid config.
	orgs := `[{"name":"org1","chef_server_url":"https://chef.example.com","org_name":"org1","client_name":"client","client_key_credential":"key1"}]`
	if err := store.Set(ctx, KeyOrganisations, json.RawMessage(orgs), false, "test"); err != nil {
		t.Fatalf("Set organisations: %v", err)
	}
	if err := store.Set(ctx, KeyLogging, json.RawMessage(`{"level":"DEBUG","retention_days":30}`), false, "test"); err != nil {
		t.Fatalf("Set logging: %v", err)
	}
	if err := store.Set(ctx, KeyAuth, json.RawMessage(`{"providers":[{"type":"local"}]}`), false, "test"); err != nil {
		t.Fatalf("Set auth: %v", err)
	}

	initialCfg := &config.Config{
		Logging: config.LoggingConfig{Level: "INFO"},
		Datastore: config.DatastoreConfig{
			URL: "postgres://localhost:5432/test",
		},
		Server: config.ServerConfig{
			ListenAddress: "127.0.0.1",
			Port:          9090,
		},
	}

	holder := NewConfigHolder(initialCfg, store)

	if err := holder.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	got := holder.Get()
	if got == nil {
		t.Fatal("expected non-nil config after reload")
	}

	// Logging should come from the store.
	if got.Logging.Level != "DEBUG" {
		t.Errorf("Logging.Level = %q, want %q (from store)", got.Logging.Level, "DEBUG")
	}

	// Bootstrap values should be carried over from the initial config.
	if got.Datastore.URL != "postgres://localhost:5432/test" {
		t.Errorf("Datastore.URL = %q, want %q (carried over)", got.Datastore.URL, "postgres://localhost:5432/test")
	}
	if got.Server.ListenAddress != "127.0.0.1" {
		t.Errorf("Server.ListenAddress = %q, want %q (carried over)", got.Server.ListenAddress, "127.0.0.1")
	}
	if got.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want %d (carried over)", got.Server.Port, 9090)
	}

	// Organisations should come from the store.
	if len(got.Organisations) != 1 {
		t.Fatalf("expected 1 organisation, got %d", len(got.Organisations))
	}
	if got.Organisations[0].Name != "org1" {
		t.Errorf("Organisations[0].Name = %q, want %q", got.Organisations[0].Name, "org1")
	}
}

// Reload — when the server.listen section is present in the DB, it is the
// source of truth and overrides the current (bootstrap) listen target.
func TestConfigHolder_Reload_SourcesListenFromDB(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	orgs := `[{"name":"org1","chef_server_url":"https://chef.example.com","org_name":"org1","client_name":"client","client_key_credential":"key1"}]`
	if err := store.Set(ctx, KeyOrganisations, json.RawMessage(orgs), false, "test"); err != nil {
		t.Fatalf("Set organisations: %v", err)
	}
	if err := store.Set(ctx, KeyServerListen, json.RawMessage(`{"listen_address":"10.0.0.1","port":9443}`), false, "test"); err != nil {
		t.Fatalf("Set server.listen: %v", err)
	}

	// The current config carries a different (bootstrap) listen target.
	initialCfg := &config.Config{
		Server: config.ServerConfig{ListenAddress: "127.0.0.1", Port: 8080},
	}
	holder := NewConfigHolder(initialCfg, store)

	if err := holder.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	got := holder.Get()
	if got.Server.ListenAddress != "10.0.0.1" {
		t.Errorf("ListenAddress = %q, want 10.0.0.1 (from DB)", got.Server.ListenAddress)
	}
	if got.Server.Port != 9443 {
		t.Errorf("Port = %d, want 9443 (from DB)", got.Server.Port)
	}
}

// ---------------------------------------------------------------------------
// Reload — validation failure preserves existing config
// ---------------------------------------------------------------------------

func TestConfigHolder_Reload_ValidationFailurePreservesConfig(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	// Seed the store with invalid data. An organisation with no name is a fatal
	// validation error (empty organisations is now a non-fatal setup warning, so
	// it can no longer stand in for "invalid config" here).
	if err := store.Set(ctx, KeyOrganisations, json.RawMessage(`[{"name":""}]`), false, "test"); err != nil {
		t.Fatalf("Set organisations: %v", err)
	}

	originalCfg := &config.Config{
		Logging: config.LoggingConfig{Level: "ORIGINAL"},
	}
	holder := NewConfigHolder(originalCfg, store)

	err := holder.Reload(ctx)
	if err == nil {
		t.Fatal("expected validation error from an organisation with no name")
	}

	// The original config should be preserved.
	got := holder.Get()
	if got != originalCfg {
		t.Error("config pointer should be unchanged after failed reload")
	}
	if got.Logging.Level != "ORIGINAL" {
		t.Errorf("Logging.Level = %q, want %q (preserved)", got.Logging.Level, "ORIGINAL")
	}
}

// ---------------------------------------------------------------------------
// Reload — database error propagates
// ---------------------------------------------------------------------------

func TestConfigHolder_Reload_DatabaseError(t *testing.T) {
	dbErr := errors.New("connection refused")
	errDB := &errorDB{err: dbErr}
	store, err := NewStoreWithKey(errDB, testKey())
	if err != nil {
		t.Fatalf("NewStoreWithKey: %v", err)
	}

	cfg := &config.Config{}
	holder := NewConfigHolder(cfg, store)

	err = holder.Reload(context.Background())
	if err == nil {
		t.Fatal("expected error from database failure")
	}
	if !containsString(err.Error(), "connection refused") {
		t.Errorf("expected 'connection refused' in error, got: %v", err)
	}

	// Config should be unchanged.
	if holder.Get() != cfg {
		t.Error("config should be unchanged after database error")
	}
}

// ---------------------------------------------------------------------------
// Store accessor
// ---------------------------------------------------------------------------

func TestConfigHolder_Store(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	holder := NewConfigHolder(&config.Config{}, store)

	if holder.Store() != store {
		t.Error("Store() should return the store passed to NewConfigHolder")
	}
}

func TestConfigHolder_Store_Nil(t *testing.T) {
	holder := NewConfigHolder(&config.Config{}, nil)

	if holder.Store() != nil {
		t.Error("Store() should return nil when no store was provided")
	}
}

// ---------------------------------------------------------------------------
// Reload — defaults are applied to assembled config
// ---------------------------------------------------------------------------

func TestConfigHolder_Reload_AppliesDefaults(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	// Set organisations (required for validation) but leave concurrency
	// empty so defaults should be applied.
	orgs := `[{"name":"org1","chef_server_url":"https://chef.example.com","org_name":"org1","client_name":"client","client_key_credential":"key1"}]`
	if err := store.Set(ctx, KeyOrganisations, json.RawMessage(orgs), false, "test"); err != nil {
		t.Fatalf("Set organisations: %v", err)
	}
	if err := store.Set(ctx, KeyAuth, json.RawMessage(`{"providers":[{"type":"local"}]}`), false, "test"); err != nil {
		t.Fatalf("Set auth: %v", err)
	}

	initialCfg := &config.Config{
		Datastore: config.DatastoreConfig{URL: "postgres://localhost/test"},
		Server:    config.ServerConfig{ListenAddress: "0.0.0.0", Port: 8080},
	}
	holder := NewConfigHolder(initialCfg, store)

	if err := holder.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	got := holder.Get()

	// Concurrency defaults should have been applied.
	if got.Concurrency.OrganisationCollection != 5 {
		t.Errorf("Concurrency.OrganisationCollection = %d, want 5 (default)", got.Concurrency.OrganisationCollection)
	}
	if got.Concurrency.NodePageFetching != 10 {
		t.Errorf("Concurrency.NodePageFetching = %d, want 10 (default)", got.Concurrency.NodePageFetching)
	}

	// Collection defaults should have been applied.
	if got.Collection.Schedule != "0 * * * *" {
		t.Errorf("Collection.Schedule = %q, want %q (default)", got.Collection.Schedule, "0 * * * *")
	}
}

// ---------------------------------------------------------------------------
// Concurrent Reload and Get — no race
// ---------------------------------------------------------------------------

func TestConfigHolder_ConcurrentReloadAndGet(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	orgs := `[{"name":"org1","chef_server_url":"https://chef.example.com","org_name":"org1","client_name":"client","client_key_credential":"key1"}]`
	_ = store.Set(ctx, KeyOrganisations, json.RawMessage(orgs), false, "test")
	_ = store.Set(ctx, KeyAuth, json.RawMessage(`{"providers":[{"type":"local"}]}`), false, "test")

	initialCfg := &config.Config{
		Datastore: config.DatastoreConfig{URL: "postgres://localhost/test"},
		Server:    config.ServerConfig{ListenAddress: "0.0.0.0", Port: 8080},
	}
	holder := NewConfigHolder(initialCfg, store)

	var wg sync.WaitGroup

	// Concurrent reloaders.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = holder.Reload(ctx)
		}()
	}

	// Concurrent readers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := holder.Get()
			if got == nil {
				t.Error("Get returned nil during concurrent reload")
			}
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// NewConfigHolder with nil initial config
// ---------------------------------------------------------------------------

func TestConfigHolder_NilInitialConfig(t *testing.T) {
	holder := NewConfigHolder(nil, nil)

	got := holder.Get()
	if got != nil {
		t.Errorf("expected nil config for nil initial, got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// Multiple reloads update to latest store state
// ---------------------------------------------------------------------------

func TestConfigHolder_MultipleReloads(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	orgs := `[{"name":"org1","chef_server_url":"https://chef.example.com","org_name":"org1","client_name":"client","client_key_credential":"key1"}]`
	_ = store.Set(ctx, KeyOrganisations, json.RawMessage(orgs), false, "test")
	_ = store.Set(ctx, KeyAuth, json.RawMessage(`{"providers":[{"type":"local"}]}`), false, "test")

	initialCfg := &config.Config{
		Datastore: config.DatastoreConfig{URL: "postgres://localhost/test"},
		Server:    config.ServerConfig{ListenAddress: "0.0.0.0", Port: 8080},
	}
	holder := NewConfigHolder(initialCfg, store)

	// First reload — logging defaults to INFO.
	if err := holder.Reload(ctx); err != nil {
		t.Fatalf("first Reload: %v", err)
	}
	got1 := holder.Get()
	if got1.Logging.Level != "INFO" {
		t.Errorf("first reload: Logging.Level = %q, want %q", got1.Logging.Level, "INFO")
	}

	// Update the store with a different logging level.
	_ = store.Set(ctx, KeyLogging, json.RawMessage(`{"level":"ERROR","retention_days":7}`), false, "test")

	// Second reload — picks up new logging level.
	if err := holder.Reload(ctx); err != nil {
		t.Fatalf("second Reload: %v", err)
	}
	got2 := holder.Get()
	if got2.Logging.Level != "ERROR" {
		t.Errorf("second reload: Logging.Level = %q, want %q", got2.Logging.Level, "ERROR")
	}

	// Pointers should differ.
	if got1 == got2 {
		t.Error("successive reloads should produce different config pointers")
	}
}
