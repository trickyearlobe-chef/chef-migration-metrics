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
// fakeLegacyDB — simulates the legacy credentials and runtime_settings tables
// ---------------------------------------------------------------------------

type fakeLegacyDB struct {
	credentials     []legacyCredentialRow
	runtimeSettings []legacyRuntimeSettingRow
	queryErr        error // if set, all queries fail
}

// fakeRows implements the minimum *sql.Rows behaviour needed by the migration
// code. We implement LegacyDB.QueryContext to return *sql.Rows, but since
// *sql.Rows is a concrete type we can't easily fake it. Instead we'll use a
// different approach: make fakeLegacyDB satisfy LegacyDB by returning real
// *sql.Rows from a registered driver.
//
// Actually, the simpler approach: since LegacyDB only requires QueryContext,
// and the migration code calls rows.Scan/rows.Next/rows.Close, we need real
// *sql.Rows. Let's use a thin wrapper that converts our fake data into the
// query results.

// We'll take a pragmatic approach: test the migration logic by using the
// higher-level MigrateFromLegacy function with a fake that returns proper
// sql.Rows via a registered in-memory driver.

// For unit tests, we'll test the individual pieces that don't need sql.Rows,
// and test the full flow with a fakeLegacyDBAdapter that implements LegacyDB
// using a lightweight approach.

// ---------------------------------------------------------------------------
// fakeSQLRows — minimal sql.Rows replacement via an in-memory sql driver
// ---------------------------------------------------------------------------

// Since constructing *sql.Rows without a real database driver is complex,
// we'll test the migration at a higher level: we pre-populate the config
// store with known state and verify MigrateFromLegacy's idempotency and
// the credential envelope format.

// For the credential decrypt→re-encrypt flow, we test it by:
// 1. Encrypting a value with the old Encryptor (hex format with old AAD)
// 2. Passing it through migrateOneCredential
// 3. Verifying the result in config_store can be decrypted with the new AAD

// ---------------------------------------------------------------------------
// Test: migrateOneCredential — decrypt old format, re-encrypt new format
// ---------------------------------------------------------------------------

func TestMigrateOneCredential_RoundTrip(t *testing.T) {
	// Create a real encryptor for the old-style encryption.
	masterKey := "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE=" // base64 of 32 × 'A'
	enc, err := secrets.NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	// Encrypt a value using the OLD AAD scheme: "credential_type:name"
	credType := "generic"
	credName := "my-test-credential"
	plaintext := []byte("super-secret-value-12345")

	oldAAD, err := secrets.BuildAAD(credType, credName)
	if err != nil {
		t.Fatalf("BuildAAD: %v", err)
	}

	encryptedValue, err := enc.Encrypt(plaintext, oldAAD)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Build a legacy row.
	now := time.Now().UTC()
	row := legacyCredentialRow{
		Name:           credName,
		CredentialType: credType,
		EncryptedValue: encryptedValue,
		Metadata:       []byte(`{"key_format":"test"}`),
		LastRotatedAt:  sql.NullTime{Time: now.Add(-24 * time.Hour), Valid: true},
		CreatedBy:      "alice",
		UpdatedBy:      sql.NullString{String: "bob", Valid: true},
		CreatedAt:      now.Add(-48 * time.Hour),
		UpdatedAt:      now,
	}

	// Create a config store backed by the fake DB.
	db := newFakeDB()
	// The store needs the same derived key as the encryptor to re-encrypt.
	store := NewStore(db, enc)

	ctx := context.Background()

	// Migrate the credential.
	if err := migrateOneCredential(ctx, store, enc, row); err != nil {
		t.Fatalf("migrateOneCredential: %v", err)
	}

	// Verify the entry exists in config_store with the correct key.
	key := "credentials/" + credName
	entry, err := db.GetConfigEntry(ctx, key)
	if err != nil {
		t.Fatalf("GetConfigEntry: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry in config_store after migration")
	}
	if !entry.Secret {
		t.Error("migrated credential should have secret=true")
	}
	if entry.UpdatedBy != "bob" {
		t.Errorf("UpdatedBy = %q, want %q", entry.UpdatedBy, "bob")
	}

	// Decrypt the value using the new AAD scheme (key name).
	decrypted, err := store.GetSecret(ctx, key)
	if err != nil {
		t.Fatalf("GetSecret after migration: %v", err)
	}

	// Parse the envelope and verify the plaintext round-tripped.
	var env credentialEnvelope
	if err := json.Unmarshal(decrypted, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	if string(env.Value) != "super-secret-value-12345" {
		t.Errorf("plaintext = %q, want %q", env.Value, "super-secret-value-12345")
	}
	if env.CredentialType != "generic" {
		t.Errorf("CredentialType = %q, want %q", env.CredentialType, "generic")
	}
	if env.CreatedBy != "alice" {
		t.Errorf("CreatedBy = %q, want %q", env.CreatedBy, "alice")
	}
	if env.UpdatedBy != "bob" {
		t.Errorf("UpdatedBy = %q, want %q", env.UpdatedBy, "bob")
	}
	if env.LastRotatedAt == nil {
		t.Error("LastRotatedAt should be set")
	}
	if env.Metadata == nil {
		t.Error("Metadata should be set")
	} else if env.Metadata["key_format"] != "test" {
		t.Errorf("Metadata[key_format] = %v, want %q", env.Metadata["key_format"], "test")
	}
}

// ---------------------------------------------------------------------------
// Test: migrateOneCredential — nil metadata handled gracefully
// ---------------------------------------------------------------------------

func TestMigrateOneCredential_NilMetadata(t *testing.T) {
	masterKey := "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE="
	enc, err := secrets.NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	oldAAD, _ := secrets.BuildAAD("generic", "nil-meta-cred")
	encVal, _ := enc.Encrypt([]byte("value"), oldAAD)

	row := legacyCredentialRow{
		Name:           "nil-meta-cred",
		CredentialType: "generic",
		EncryptedValue: encVal,
		Metadata:       nil, // no metadata
		CreatedBy:      "admin",
		CreatedAt:      time.Now().UTC(),
	}

	db := newFakeDB()
	store := NewStore(db, enc)
	ctx := context.Background()

	if err := migrateOneCredential(ctx, store, enc, row); err != nil {
		t.Fatalf("migrateOneCredential with nil metadata: %v", err)
	}

	decrypted, err := store.GetSecret(ctx, "credentials/nil-meta-cred")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}

	var env credentialEnvelope
	if err := json.Unmarshal(decrypted, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(env.Value) != "value" {
		t.Errorf("plaintext = %q, want %q", env.Value, "value")
	}
}

// ---------------------------------------------------------------------------
// Test: migrateOneCredential — null updated_by defaults to "migration"
// ---------------------------------------------------------------------------

func TestMigrateOneCredential_NullUpdatedBy(t *testing.T) {
	masterKey := "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE="
	enc, err := secrets.NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	oldAAD, _ := secrets.BuildAAD("generic", "null-updater-cred")
	encVal, _ := enc.Encrypt([]byte("val"), oldAAD)

	row := legacyCredentialRow{
		Name:           "null-updater-cred",
		CredentialType: "generic",
		EncryptedValue: encVal,
		CreatedBy:      "admin",
		UpdatedBy:      sql.NullString{Valid: false}, // NULL updated_by
		CreatedAt:      time.Now().UTC(),
	}

	db := newFakeDB()
	store := NewStore(db, enc)
	ctx := context.Background()

	if err := migrateOneCredential(ctx, store, enc, row); err != nil {
		t.Fatalf("migrateOneCredential: %v", err)
	}

	entry, _ := db.GetConfigEntry(ctx, "credentials/null-updater-cred")
	if entry == nil {
		t.Fatal("expected entry")
	}
	if entry.UpdatedBy != "migration" {
		t.Errorf("UpdatedBy = %q, want %q (default for null)", entry.UpdatedBy, "migration")
	}
}

// ---------------------------------------------------------------------------
// Test: migrateOneCredential — decryption failure propagates error
// ---------------------------------------------------------------------------

func TestMigrateOneCredential_DecryptionFailure(t *testing.T) {
	masterKey := "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE="
	enc, err := secrets.NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	row := legacyCredentialRow{
		Name:           "bad-cred",
		CredentialType: "generic",
		EncryptedValue: "not-valid-hex:data", // malformed
		CreatedBy:      "admin",
		CreatedAt:      time.Now().UTC(),
	}

	db := newFakeDB()
	store := NewStore(db, enc)

	err = migrateOneCredential(context.Background(), store, enc, row)
	if err == nil {
		t.Fatal("expected error for invalid encrypted value")
	}
}

// ---------------------------------------------------------------------------
// Test: MigrateFromLegacy — skips when config_store is not empty
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

	result, err := MigrateFromLegacy(ctx, legDB, store, enc)
	if err != nil {
		t.Fatalf("MigrateFromLegacy: %v", err)
	}
	if !result.Skipped {
		t.Error("expected Skipped=true when config_store is not empty")
	}
	if result.SkipReason == "" {
		t.Error("expected SkipReason to be set")
	}
	if result.CredentialsMigrated != 0 {
		t.Errorf("CredentialsMigrated = %d, want 0", result.CredentialsMigrated)
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

	// Simulate runtime_settings rows via direct call to migrateRuntimeSettings
	// using a fake legacy DB.
	legDB := &inMemoryLegacyDB{
		runtimeSettings: []legacyRuntimeSettingRow{
			{
				Key:       "test_kitchen",
				Value:     json.RawMessage(`{"driver":"dokken","enabled":true}`),
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
	if string(val1) != `{"driver":"dokken","enabled":true}` {
		t.Errorf("test_kitchen value = %s, want %s", val1, `{"driver":"dokken","enabled":true}`)
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
// Test: migrateCredentials — nil encryptor with empty credentials table is OK
// ---------------------------------------------------------------------------

func TestMigrateCredentials_NilEncryptorEmptyTable(t *testing.T) {
	db := newFakeDB()
	store, err := NewStoreWithKey(db, testKey())
	if err != nil {
		t.Fatalf("NewStoreWithKey: %v", err)
	}

	legDB := &inMemoryLegacyDB{
		credentialCount: 0,
	}

	count, err := migrateCredentials(context.Background(), legDB, store, nil)
	if err != nil {
		t.Fatalf("migrateCredentials with nil encryptor and empty table: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

// ---------------------------------------------------------------------------
// Test: migrateCredentials — nil encryptor with non-empty credentials table fails
// ---------------------------------------------------------------------------

func TestMigrateCredentials_NilEncryptorNonEmptyTable(t *testing.T) {
	db := newFakeDB()
	store, err := NewStoreWithKey(db, testKey())
	if err != nil {
		t.Fatalf("NewStoreWithKey: %v", err)
	}

	legDB := &inMemoryLegacyDB{
		credentialCount: 3,
	}

	_, err = migrateCredentials(context.Background(), legDB, store, nil)
	if err == nil {
		t.Fatal("expected error when credentials exist but no encryption key")
	}
	if !containsString(err.Error(), "no encryption key") {
		t.Errorf("expected 'no encryption key' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test: MigrateResult — zero value
// ---------------------------------------------------------------------------

func TestMigrateResult_ZeroValue(t *testing.T) {
	var r MigrateResult
	if r.CredentialsMigrated != 0 {
		t.Errorf("CredentialsMigrated = %d, want 0", r.CredentialsMigrated)
	}
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

	// Verify optional fields are omitted.
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
	// updated_by is omitempty, so empty string should be omitted.
	if _, ok := m["updated_by"]; ok {
		t.Error("updated_by should be omitted when empty")
	}
}

// ---------------------------------------------------------------------------
// Test: multiple credentials migration preserves order and all data
// ---------------------------------------------------------------------------

func TestMigrateMultipleCredentials(t *testing.T) {
	masterKey := "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE="
	enc, err := secrets.NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	// Encrypt three credentials with the old scheme.
	creds := []struct {
		name     string
		credType string
		value    string
	}{
		{"alpha-cred", "generic", "alpha-secret"},
		{"beta-cred", "webhook_url", "https://hooks.example.com/test"},
		{"gamma-cred", "smtp_password", "smtp-pass-123"},
	}

	var rows []legacyCredentialRow
	now := time.Now().UTC()
	for _, c := range creds {
		oldAAD, _ := secrets.BuildAAD(c.credType, c.name)
		encVal, encErr := enc.Encrypt([]byte(c.value), oldAAD)
		if encErr != nil {
			t.Fatalf("Encrypt %s: %v", c.name, encErr)
		}
		rows = append(rows, legacyCredentialRow{
			Name:           c.name,
			CredentialType: c.credType,
			EncryptedValue: encVal,
			CreatedBy:      "admin",
			CreatedAt:      now,
		})
	}

	db := newFakeDB()
	store := NewStore(db, enc)
	ctx := context.Background()

	legDB := &inMemoryLegacyDB{
		credentials: rows,
	}

	count, err := migrateCredentials(ctx, legDB, store, enc)
	if err != nil {
		t.Fatalf("migrateCredentials: %v", err)
	}
	if count != 3 {
		t.Errorf("migrated count = %d, want 3", count)
	}

	// Verify each credential can be decrypted.
	for _, c := range creds {
		key := "credentials/" + c.name
		raw, getErr := store.GetSecret(ctx, key)
		if getErr != nil {
			t.Errorf("GetSecret(%q): %v", key, getErr)
			continue
		}

		var env credentialEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Errorf("unmarshal %q: %v", key, err)
			continue
		}

		if string(env.Value) != c.value {
			t.Errorf("%s plaintext = %q, want %q", c.name, env.Value, c.value)
		}
		if env.CredentialType != c.credType {
			t.Errorf("%s type = %q, want %q", c.name, env.CredentialType, c.credType)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: MigrateFromLegacy — full flow with credentials and runtime_settings
// ---------------------------------------------------------------------------

func TestMigrateFromLegacy_FullFlow(t *testing.T) {
	masterKey := "QUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUFBQUE="
	enc, err := secrets.NewEncryptor(masterKey)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	defer enc.Close()

	now := time.Now().UTC()

	// Create legacy credential.
	oldAAD, _ := secrets.BuildAAD("generic", "test-cred")
	encVal, _ := enc.Encrypt([]byte("secret-value"), oldAAD)

	legDB := &inMemoryLegacyDB{
		credentials: []legacyCredentialRow{
			{
				Name:           "test-cred",
				CredentialType: "generic",
				EncryptedValue: encVal,
				CreatedBy:      "admin",
				CreatedAt:      now,
			},
		},
		runtimeSettings: []legacyRuntimeSettingRow{
			{
				Key:       "test_kitchen",
				Value:     json.RawMessage(`{"driver":"dokken"}`),
				UpdatedAt: now,
				UpdatedBy: "admin",
			},
		},
	}

	db := newFakeDB()
	store := NewStore(db, enc)
	ctx := context.Background()

	result, err := MigrateFromLegacy(ctx, legDB, store, enc)
	if err != nil {
		t.Fatalf("MigrateFromLegacy: %v", err)
	}

	if result.Skipped {
		t.Error("should not be skipped on empty config_store")
	}
	if result.CredentialsMigrated != 1 {
		t.Errorf("CredentialsMigrated = %d, want 1", result.CredentialsMigrated)
	}
	if result.RuntimeSettingsMigrated != 1 {
		t.Errorf("RuntimeSettingsMigrated = %d, want 1", result.RuntimeSettingsMigrated)
	}

	// Verify credential.
	credRaw, err := store.GetSecret(ctx, "credentials/test-cred")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	var env credentialEnvelope
	if err := json.Unmarshal(credRaw, &env); err != nil {
		t.Fatalf("unmarshal credential: %v", err)
	}
	if string(env.Value) != "secret-value" {
		t.Errorf("credential value = %q, want %q", env.Value, "secret-value")
	}

	// Verify runtime setting.
	settingVal, err := store.Get(ctx, "test_kitchen")
	if err != nil {
		t.Fatalf("Get test_kitchen: %v", err)
	}
	if string(settingVal) != `{"driver":"dokken"}` {
		t.Errorf("test_kitchen = %s, want %s", settingVal, `{"driver":"dokken"}`)
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

	oldAAD, _ := secrets.BuildAAD("generic", "idem-cred")
	encVal, _ := enc.Encrypt([]byte("val"), oldAAD)

	legDB := &inMemoryLegacyDB{
		credentials: []legacyCredentialRow{
			{
				Name:           "idem-cred",
				CredentialType: "generic",
				EncryptedValue: encVal,
				CreatedBy:      "admin",
				CreatedAt:      time.Now().UTC(),
			},
		},
	}

	db := newFakeDB()
	store := NewStore(db, enc)
	ctx := context.Background()

	// First migration.
	result1, err := MigrateFromLegacy(ctx, legDB, store, enc)
	if err != nil {
		t.Fatalf("first MigrateFromLegacy: %v", err)
	}
	if result1.Skipped {
		t.Error("first call should not be skipped")
	}
	if result1.CredentialsMigrated != 1 {
		t.Errorf("first CredentialsMigrated = %d, want 1", result1.CredentialsMigrated)
	}

	// Second migration — should be skipped because config_store has entries.
	result2, err := MigrateFromLegacy(ctx, legDB, store, enc)
	if err != nil {
		t.Fatalf("second MigrateFromLegacy: %v", err)
	}
	if !result2.Skipped {
		t.Error("second call should be skipped")
	}
	if result2.CredentialsMigrated != 0 {
		t.Errorf("second CredentialsMigrated = %d, want 0", result2.CredentialsMigrated)
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

	_, err = MigrateFromLegacy(context.Background(), legDB, store, enc)
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
	credentials     []legacyCredentialRow
	runtimeSettings []legacyRuntimeSettingRow
	credentialCount int // used when credentials slice is nil
}

func (m *inMemoryLegacyDB) QueryContext(_ context.Context, query string, _ ...any) (*sql.Rows, error) {
	// We can't easily create *sql.Rows without a real driver, so we use
	// a registered in-memory driver. But that's heavyweight for unit tests.
	//
	// Instead, we use a pragmatic workaround: register a minimal driver
	// that serves our canned data based on the query string.
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

	// Determine which data to serve based on the query.
	var driverRows *fakeDriverRows

	switch {
	case containsString(query, "FROM credentials") && containsString(query, "COUNT"):
		count := m.credentialCount
		if m.credentials != nil {
			count = len(m.credentials)
		}
		driverRows = &fakeDriverRows{
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(count)}},
		}

	case containsString(query, "FROM credentials"):
		cols := []string{"name", "credential_type", "encrypted_value", "metadata",
			"last_rotated_at", "created_by", "updated_by", "created_at", "updated_at"}
		var rows [][]driver.Value
		for _, c := range m.credentials {
			var metadata driver.Value
			if c.Metadata != nil {
				metadata = c.Metadata
			}
			var lastRotated driver.Value
			if c.LastRotatedAt.Valid {
				lastRotated = c.LastRotatedAt.Time
			}
			var updatedBy driver.Value
			if c.UpdatedBy.Valid {
				updatedBy = c.UpdatedBy.String
			}
			rows = append(rows, []driver.Value{
				c.Name, c.CredentialType, c.EncryptedValue, metadata,
				lastRotated, c.CreatedBy, updatedBy, c.CreatedAt, c.UpdatedAt,
			})
		}
		driverRows = &fakeDriverRows{columns: cols, rows: rows}

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
