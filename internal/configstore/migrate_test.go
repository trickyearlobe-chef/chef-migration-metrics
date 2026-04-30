// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// ---------------------------------------------------------------------------
// fakeLegacyDB — simulates the legacy runtime_settings table
// ---------------------------------------------------------------------------

type fakeLegacyDB struct {
	runtimeSettings []legacyRuntimeSettingRow
	queryErr        error // if set, all queries fail
}

// ---------------------------------------------------------------------------
// Test: MigrateFromLegacy — skips when config_store already has entries
// ---------------------------------------------------------------------------

func TestMigrateFromLegacy_SkipsWhenNotEmpty(t *testing.T) {
	masterKey := "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE="
	enc, err := secrets.NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	db := newFakeDB()
	store := NewStore(db, enc)
	ctx := context.Background()

	// Pre-populate config_store with an entry.
	if err := store.Set(ctx, "organisations", json.RawMessage(`[]`), false, "admin"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// The legacy DB shouldn't even be queried.
	legDB := &panicLegacyDB{}

	result, err := MigrateFromLegacy(ctx, legDB, store)
	if err != nil {
		t.Fatalf("MigrateFromLegacy: %v", err)
	}
	if !result.Skipped {
		t.Error("expected Skipped=true when config_store is not empty")
	}
	if result.SkipReason == "" {
		t.Error("expected SkipReason to be set")
	}
	if result.RuntimeSettingsMigrated != 0 {
		t.Errorf("RuntimeSettingsMigrated = %d, want 0", result.RuntimeSettingsMigrated)
	}
}

// ---------------------------------------------------------------------------
// Test: migrateRuntimeSettings — encrypts and stores settings
// ---------------------------------------------------------------------------

func TestMigrateRuntimeSettings(t *testing.T) {
	masterKey := "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE="
	enc, err := secrets.NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	db := newFakeDB()
	store := NewStore(db, enc)
	ctx := context.Background()

	legDB := &inMemoryLegacyDB{
		runtimeSettings: []legacyRuntimeSettingRow{
			{
				Key:       "test_kitchen",
				Value:     json.RawMessage(`{"driver":"proxmox","enabled":true}`),
				UpdatedAt: time.Now().UTC(),
				UpdatedBy: "admin@example.com",
			},
			{
				Key:       "custom_setting",
				Value:     json.RawMessage(`{"key":"value"}`),
				UpdatedAt: time.Now().UTC(),
				UpdatedBy: "user",
			},
		},
	}

	count, err := migrateRuntimeSettings(ctx, legDB, store)
	if err != nil {
		t.Fatalf("migrateRuntimeSettings: %v", err)
	}
	if count != 2 {
		t.Errorf("migrated count = %d, want 2", count)
	}

	// Verify the entries are in config_store and can be decrypted.
	val1, err := store.Get(ctx, "test_kitchen")
	if err != nil {
		t.Fatalf("Get test_kitchen: %v", err)
	}
	if string(val1) != `{"driver":"proxmox","enabled":true}` {
		t.Errorf("test_kitchen value = %s, want %s", val1, `{"driver":"proxmox","enabled":true}`)
	}

	val2, err := store.Get(ctx, "custom_setting")
	if err != nil {
		t.Fatalf("Get custom_setting: %v", err)
	}
	if string(val2) != `{"key":"value"}` {
		t.Errorf("custom_setting value = %s, want %s", val2, `{"key":"value"}`)
	}

	// Verify they are not marked as secret.
	entry, _ := db.GetConfigEntry(ctx, "test_kitchen")
	if entry == nil {
		t.Fatal("expected test_kitchen entry")
	}
	if entry.Secret {
		t.Error("runtime settings should not be secret")
	}
}

// ---------------------------------------------------------------------------
// Test: MigrateResult zero value
// ---------------------------------------------------------------------------

func TestMigrateResult_ZeroValue(t *testing.T) {
	var r MigrateResult
	if r.RuntimeSettingsMigrated != 0 {
		t.Errorf("RuntimeSettingsMigrated = %d, want 0", r.RuntimeSettingsMigrated)
	}
	if r.Skipped {
		t.Error("Skipped should be false")
	}
	if r.SkipReason != "" {
		t.Errorf("SkipReason = %q, want empty", r.SkipReason)
	}
}

// ---------------------------------------------------------------------------
// Test: credentialEnvelope JSON round-trip
// ---------------------------------------------------------------------------

func TestCredentialEnvelope_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	rotated := now.Add(-1 * time.Hour)

	env := credentialEnvelope{
		CredentialType: "chef_client_key",
		Value:          []byte("-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----"),
		Metadata:       map[string]any{"key_format": "pkcs1", "bits": float64(2048)},
		LastRotatedAt:  &rotated,
		CreatedBy:      "alice",
		UpdatedBy:      "bob",
		CreatedAt:      now,
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got credentialEnvelope
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.CredentialType != env.CredentialType {
		t.Errorf("CredentialType = %q, want %q", got.CredentialType, env.CredentialType)
	}
	if string(got.Value) != string(env.Value) {
		t.Errorf("Value mismatch")
	}
	if got.CreatedBy != env.CreatedBy {
		t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, env.CreatedBy)
	}
	if got.UpdatedBy != env.UpdatedBy {
		t.Errorf("UpdatedBy = %q, want %q", got.UpdatedBy, env.UpdatedBy)
	}
	if got.LastRotatedAt == nil {
		t.Error("LastRotatedAt should not be nil")
	}
	if got.Metadata == nil {
		t.Error("Metadata should not be nil")
	}
}

// ---------------------------------------------------------------------------
// Test: credentialEnvelope — omitempty fields
// ---------------------------------------------------------------------------

func TestCredentialEnvelope_OmitEmpty(t *testing.T) {
	env := credentialEnvelope{
		CredentialType: "generic",
		Value:          []byte("val"),
		CreatedBy:      "admin",
		CreatedAt:      time.Now().UTC(),
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	if _, ok := m["last_rotated_at"]; ok {
		t.Error("last_rotated_at should be omitted when nil")
	}
	if _, ok := m["metadata"]; ok {
		t.Error("metadata should be omitted when nil")
	}
	if _, ok := m["updated_by"]; ok {
		t.Error("updated_by should be omitted when empty")
	}
}

// ---------------------------------------------------------------------------
// Test: MigrateFromLegacy — runtime settings full flow
// ---------------------------------------------------------------------------

func TestMigrateFromLegacy_FullFlow(t *testing.T) {
	masterKey := "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE="
	enc, err := secrets.NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	now := time.Now().UTC()

	legDB := &inMemoryLegacyDB{
		runtimeSettings: []legacyRuntimeSettingRow{
			{
				Key:       "test_kitchen",
				Value:     json.RawMessage(`{"driver":"proxmox"}`),
				UpdatedAt: now,
				UpdatedBy: "admin",
			},
		},
	}

	db := newFakeDB()
	store := NewStore(db, enc)
	ctx := context.Background()

	result, err := MigrateFromLegacy(ctx, legDB, store)
	if err != nil {
		t.Fatalf("MigrateFromLegacy: %v", err)
	}

	if result.Skipped {
		t.Error("should not be skipped on empty config_store")
	}
	if result.RuntimeSettingsMigrated != 1 {
		t.Errorf("RuntimeSettingsMigrated = %d, want 1", result.RuntimeSettingsMigrated)
	}

	// Verify runtime setting.
	settingVal, err := store.Get(ctx, "test_kitchen")
	if err != nil {
		t.Fatalf("Get test_kitchen: %v", err)
	}
	if string(settingVal) != `{"driver":"proxmox"}` {
		t.Errorf("test_kitchen = %s, want %s", settingVal, `{"driver":"proxmox"}`)
	}
}

// ---------------------------------------------------------------------------
// Test: MigrateFromLegacy — idempotent (second call is a no-op)
// ---------------------------------------------------------------------------

func TestMigrateFromLegacy_Idempotent(t *testing.T) {
	masterKey := "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE="
	enc, err := secrets.NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	legDB := &inMemoryLegacyDB{
		runtimeSettings: []legacyRuntimeSettingRow{
			{Key: "k", Value: json.RawMessage(`"v"`), UpdatedBy: "admin", UpdatedAt: time.Now().UTC()},
		},
	}

	db := newFakeDB()
	store := NewStore(db, enc)
	ctx := context.Background()

	result1, err := MigrateFromLegacy(ctx, legDB, store)
	if err != nil {
		t.Fatalf("first MigrateFromLegacy: %v", err)
	}
	if result1.Skipped {
		t.Error("first call should not be skipped")
	}
	if result1.RuntimeSettingsMigrated != 1 {
		t.Errorf("first RuntimeSettingsMigrated = %d, want 1", result1.RuntimeSettingsMigrated)
	}

	result2, err := MigrateFromLegacy(ctx, legDB, store)
	if err != nil {
		t.Fatalf("second MigrateFromLegacy: %v", err)
	}
	if !result2.Skipped {
		t.Error("second call should be skipped")
	}
	if result2.RuntimeSettingsMigrated != 0 {
		t.Errorf("second RuntimeSettingsMigrated = %d, want 0", result2.RuntimeSettingsMigrated)
	}
}

// ---------------------------------------------------------------------------
// Test: MigrateFromLegacy — config_store IsEmpty error propagated
// ---------------------------------------------------------------------------

func TestMigrateFromLegacy_IsEmptyError(t *testing.T) {
	masterKey := "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE="
	enc, err := secrets.NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	dbErr := errors.New("database unreachable")
	errDB := &errorDB{err: dbErr}
	store, _ := NewStoreWithKey(errDB, testKey())

	legDB := &inMemoryLegacyDB{}

	_, err = MigrateFromLegacy(context.Background(), legDB, store)
	if err == nil {
		t.Fatal("expected error when IsEmpty fails")
	}
	if !containsString(err.Error(), "database unreachable") {
		t.Errorf("expected database error in chain, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// panicLegacyDB — panics if QueryContext is called (verifies skip path)
// ---------------------------------------------------------------------------

type panicLegacyDB struct{}

func (p *panicLegacyDB) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	panic("panicLegacyDB: QueryContext should not be called when migration is skipped")
}

// ---------------------------------------------------------------------------
// inMemoryLegacyDB — provides canned query results for migration tests
// ---------------------------------------------------------------------------

type inMemoryLegacyDB struct {
	runtimeSettings []legacyRuntimeSettingRow
}

func (m *inMemoryLegacyDB) QueryContext(_ context.Context, query string, _ ...any) (*sql.Rows, error) {
	return openFakeRows(query, m)
}

// ---------------------------------------------------------------------------
// Minimal sql driver for test fakes
// ---------------------------------------------------------------------------

func init() {
	sql.Register("configstore_test_fake", &fakeDriver{})
}

var fakeDriverSeq int

type fakeDriver struct{}

func (d *fakeDriver) Open(name string) (driver.Conn, error) {
	return &fakeConn{name: name}, nil
}

type fakeConn struct {
	name string
}

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeStmt{query: query, connName: c.name}, nil
}

func (c *fakeConn) Close() error { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not supported")
}

type fakeStmt struct {
	query    string
	connName string
}

func (s *fakeStmt) Close() error                                    { return nil }
func (s *fakeStmt) NumInput() int                                   { return 0 }
func (s *fakeStmt) Exec(args []driver.Value) (driver.Result, error) { return nil, nil }

func (s *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	data := fakeDataRegistry[s.connName]
	if data == nil {
		return &fakeDriverRows{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}}, nil
	}
	return data, nil
}

type fakeDriverRows struct {
	columns []string
	rows    [][]driver.Value
	pos     int
}

func (r *fakeDriverRows) Columns() []string { return r.columns }
func (r *fakeDriverRows) Close() error      { return nil }

func (r *fakeDriverRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.pos])
	r.pos++
	return nil
}

var fakeDataRegistry = make(map[string]*fakeDriverRows)

func openFakeRows(query string, m *inMemoryLegacyDB) (*sql.Rows, error) {
	fakeDriverSeq++
	dsn := fmt.Sprintf("fake_%d", fakeDriverSeq)

	var driverRows *fakeDriverRows

	switch {
	case containsString(query, "FROM runtime_settings"):
		cols := []string{"key", "value", "updated_at", "updated_by"}
		var rows [][]driver.Value
		for _, s := range m.runtimeSettings {
			rows = append(rows, []driver.Value{
				s.Key, []byte(s.Value), s.UpdatedAt, s.UpdatedBy,
			})
		}
		driverRows = &fakeDriverRows{columns: cols, rows: rows}

	default:
		driverRows = &fakeDriverRows{
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(0)}},
		}
	}

	fakeDataRegistry[dsn] = driverRows

	db, err := sql.Open("configstore_test_fake", dsn)
	if err != nil {
		return nil, err
	}

	return db.Query("SELECT 1")
}
