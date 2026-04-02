// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// fakeDB — in-memory implementation of DatastoreDB for unit tests
// ---------------------------------------------------------------------------

type fakeDB struct {
	mu      sync.Mutex
	entries map[string]*datastore.ConfigEntry
}

func newFakeDB() *fakeDB {
	return &fakeDB{entries: make(map[string]*datastore.ConfigEntry)}
}

func (f *fakeDB) GetConfigEntry(_ context.Context, key string) (*datastore.ConfigEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[key]
	if !ok {
		return nil, nil
	}
	// Return a copy to avoid mutation.
	cp := *e
	cp.EncryptedValue = append([]byte(nil), e.EncryptedValue...)
	cp.Nonce = append([]byte(nil), e.Nonce...)
	return &cp, nil
}

func (f *fakeDB) SetConfigEntry(_ context.Context, e *datastore.ConfigEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *e
	cp.EncryptedValue = append([]byte(nil), e.EncryptedValue...)
	cp.Nonce = append([]byte(nil), e.Nonce...)
	cp.UpdatedAt = time.Now().UTC()
	f.entries[cp.Key] = &cp
	return nil
}

func (f *fakeDB) DeleteConfigEntry(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.entries, key)
	return nil
}

func (f *fakeDB) ListConfigEntries(_ context.Context) ([]datastore.ConfigEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]datastore.ConfigEntry, 0, len(f.entries))
	for _, e := range f.entries {
		cp := *e
		cp.EncryptedValue = append([]byte(nil), e.EncryptedValue...)
		cp.Nonce = append([]byte(nil), e.Nonce...)
		result = append(result, cp)
	}
	return result, nil
}

func (f *fakeDB) ListConfigEntriesByPrefix(_ context.Context, prefix string) ([]datastore.ConfigEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []datastore.ConfigEntry
	for k, e := range f.entries {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			cp := *e
			cp.EncryptedValue = append([]byte(nil), e.EncryptedValue...)
			cp.Nonce = append([]byte(nil), e.Nonce...)
			result = append(result, cp)
		}
	}
	return result, nil
}

func (f *fakeDB) CountConfigEntries(_ context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.entries), nil
}

func (f *fakeDB) ConfigStoreIsEmpty(_ context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.entries) == 0, nil
}

// ---------------------------------------------------------------------------
// errorDB — always returns errors, for testing error paths
// ---------------------------------------------------------------------------

type errorDB struct {
	err error
}

func (e *errorDB) GetConfigEntry(context.Context, string) (*datastore.ConfigEntry, error) {
	return nil, e.err
}
func (e *errorDB) SetConfigEntry(context.Context, *datastore.ConfigEntry) error { return e.err }
func (e *errorDB) DeleteConfigEntry(context.Context, string) error              { return e.err }
func (e *errorDB) ListConfigEntries(context.Context) ([]datastore.ConfigEntry, error) {
	return nil, e.err
}
func (e *errorDB) ListConfigEntriesByPrefix(context.Context, string) ([]datastore.ConfigEntry, error) {
	return nil, e.err
}
func (e *errorDB) CountConfigEntries(context.Context) (int, error) { return 0, e.err }
func (e *errorDB) ConfigStoreIsEmpty(context.Context) (bool, error) {
	return false, e.err
}

// ---------------------------------------------------------------------------
// testKey — fixed 32-byte AES-256 key for deterministic tests
// ---------------------------------------------------------------------------

func testKey() []byte {
	return []byte("test-aes-256-key-exactly-32bytes")
}

func mustNewStore(t *testing.T, db DatastoreDB) *Store {
	t.Helper()
	s, err := NewStoreWithKey(db, testKey())
	if err != nil {
		t.Fatalf("NewStoreWithKey: %v", err)
	}
	return s
}

// ---------------------------------------------------------------------------
// encrypt / decrypt round-trip
// ---------------------------------------------------------------------------

func TestEncryptDecryptRoundTrip(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"simple string", "organisations", `["org1","org2"]`},
		{"nested object", "collection", `{"schedule":"0 * * * *","stale_node_threshold_days":30}`},
		{"empty object", "concurrency", `{}`},
		{"empty array", "target_chef_versions", `[]`},
		{"integer value", "server.graceful_shutdown_seconds", `30`},
		{"boolean value", "server.websocket.enabled", `true`},
		{"null value", "test_null", `null`},
		{"unicode", "test_unicode", `{"name":"日本語テスト"}`},
		{"credential slash key", "credentials/my-secret", `{"credential_type":"generic","value":"supersecret"}`},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.Set(ctx, tt.key, json.RawMessage(tt.value), false, "test-user")
			if err != nil {
				t.Fatalf("Set: %v", err)
			}

			got, err := s.Get(ctx, tt.key)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}

			if string(got) != tt.value {
				t.Errorf("round-trip mismatch:\n  got:  %s\n  want: %s", got, tt.value)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// encrypt produces different ciphertext each time (random nonce)
// ---------------------------------------------------------------------------

func TestEncryptProducesDifferentCiphertextPerWrite(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	value := json.RawMessage(`{"test":"data"}`)

	if err := s.Set(ctx, "key1", value, false, "user"); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	entry1, _ := db.GetConfigEntry(ctx, "key1")

	// Overwrite with the same plaintext.
	if err := s.Set(ctx, "key1", value, false, "user"); err != nil {
		t.Fatalf("second Set: %v", err)
	}
	entry2, _ := db.GetConfigEntry(ctx, "key1")

	// Nonces must differ.
	if string(entry1.Nonce) == string(entry2.Nonce) {
		t.Error("nonces should differ between writes")
	}

	// Ciphertexts must differ (because nonce differs).
	if string(entry1.EncryptedValue) == string(entry2.EncryptedValue) {
		t.Error("ciphertexts should differ between writes (different nonce)")
	}
}

// ---------------------------------------------------------------------------
// AAD mismatch — decryption fails if key name changes
// ---------------------------------------------------------------------------

func TestDecryptFailsWithWrongAAD(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	// Encrypt under key "real-key".
	if err := s.Set(ctx, "real-key", json.RawMessage(`"secret"`), false, "user"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Manually move the row to a different key (simulates row-swap attack).
	entry, _ := db.GetConfigEntry(ctx, "real-key")
	swapped := *entry
	swapped.Key = "fake-key"
	if err := db.SetConfigEntry(ctx, &swapped); err != nil {
		t.Fatalf("manual swap: %v", err)
	}

	// Decryption under "fake-key" should fail because AAD won't match.
	_, err := s.Get(ctx, "fake-key")
	if err == nil {
		t.Fatal("expected decryption error for AAD mismatch, got nil")
	}
	if !errors.Is(err, ErrDecryptionFailed) {
		// The error is wrapped, so check the message.
		if !containsString(err.Error(), "decryption failed") {
			t.Errorf("expected decryption failed error, got: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Get — non-existent key returns ErrNotFound
// ---------------------------------------------------------------------------

func TestGetNonExistentKey(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)

	_, err := s.Get(context.Background(), "no-such-key")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetSecret — returns value only for secret-flagged entries
// ---------------------------------------------------------------------------

func TestGetSecretOnlyReturnsSecrets(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	// Set a non-secret entry.
	if err := s.Set(ctx, "config-key", json.RawMessage(`"config"`), false, "user"); err != nil {
		t.Fatalf("Set non-secret: %v", err)
	}

	// Set a secret entry.
	if err := s.Set(ctx, "credentials/my-cred", json.RawMessage(`"password"`), true, "user"); err != nil {
		t.Fatalf("Set secret: %v", err)
	}

	// GetSecret on a non-secret key should fail.
	_, err := s.GetSecret(ctx, "config-key")
	if !errors.Is(err, ErrNotSecret) {
		t.Errorf("expected ErrNotSecret for non-secret key, got: %v", err)
	}

	// GetSecret on a secret key should succeed.
	got, err := s.GetSecret(ctx, "credentials/my-cred")
	if err != nil {
		t.Fatalf("GetSecret on secret key: %v", err)
	}
	if string(got) != `"password"` {
		t.Errorf("GetSecret value = %s, want %q", got, `"password"`)
	}
}

// ---------------------------------------------------------------------------
// GetSecret — non-existent key returns ErrNotFound
// ---------------------------------------------------------------------------

func TestGetSecretNonExistent(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)

	_, err := s.GetSecret(context.Background(), "no-such-secret")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Delete — idempotent
// ---------------------------------------------------------------------------

func TestDeleteIdempotent(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	if err := s.Set(ctx, "to-delete", json.RawMessage(`"val"`), false, "user"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// First delete.
	if err := s.Delete(ctx, "to-delete"); err != nil {
		t.Fatalf("first Delete: %v", err)
	}

	// Second delete (idempotent).
	if err := s.Delete(ctx, "to-delete"); err != nil {
		t.Fatalf("second Delete: %v", err)
	}

	// Confirm gone.
	_, err := s.Get(ctx, "to-delete")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// List — returns metadata without values
// ---------------------------------------------------------------------------

func TestListReturnsMetadata(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	if err := s.Set(ctx, "a-config", json.RawMessage(`"val1"`), false, "alice"); err != nil {
		t.Fatalf("Set a-config: %v", err)
	}
	if err := s.Set(ctx, "credentials/secret", json.RawMessage(`"val2"`), true, "bob"); err != nil {
		t.Fatalf("Set credentials/secret: %v", err)
	}

	entries, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Verify metadata fields are populated.
	for _, e := range entries {
		if e.Key == "" {
			t.Error("entry Key should not be empty")
		}
		if e.UpdatedBy == "" {
			t.Error("entry UpdatedBy should not be empty")
		}
		if e.UpdatedAt.IsZero() {
			t.Error("entry UpdatedAt should not be zero")
		}
	}

	// Find the secret entry and verify its flag.
	var foundSecret bool
	for _, e := range entries {
		if e.Key == "credentials/secret" {
			foundSecret = true
			if !e.Secret {
				t.Error("credentials/secret should have Secret=true")
			}
		}
	}
	if !foundSecret {
		t.Error("credentials/secret not found in list")
	}
}

// ---------------------------------------------------------------------------
// ListByPrefix — filters correctly
// ---------------------------------------------------------------------------

func TestListByPrefix(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	if err := s.Set(ctx, "credentials/a", json.RawMessage(`"a"`), true, "user"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set(ctx, "credentials/b", json.RawMessage(`"b"`), true, "user"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set(ctx, "organisations", json.RawMessage(`[]`), false, "user"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	entries, err := s.ListByPrefix(ctx, "credentials/")
	if err != nil {
		t.Fatalf("ListByPrefix: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 credential entries, got %d", len(entries))
	}

	for _, e := range entries {
		if len(e.Key) < len("credentials/") || e.Key[:len("credentials/")] != "credentials/" {
			t.Errorf("unexpected key %q in prefix results", e.Key)
		}
	}
}

// ---------------------------------------------------------------------------
// GetAll — excludes secrets
// ---------------------------------------------------------------------------

func TestGetAllExcludesSecrets(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	if err := s.Set(ctx, "organisations", json.RawMessage(`["org1"]`), false, "user"); err != nil {
		t.Fatalf("Set config: %v", err)
	}
	if err := s.Set(ctx, "collection", json.RawMessage(`{"schedule":"0 * * * *"}`), false, "user"); err != nil {
		t.Fatalf("Set config: %v", err)
	}
	if err := s.Set(ctx, "credentials/secret", json.RawMessage(`"password"`), true, "user"); err != nil {
		t.Fatalf("Set secret: %v", err)
	}

	all, err := s.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}

	// Should have 2 non-secret entries.
	if len(all) != 2 {
		t.Fatalf("expected 2 entries from GetAll, got %d", len(all))
	}

	// Verify secrets are excluded.
	if _, ok := all["credentials/secret"]; ok {
		t.Error("GetAll should not include secret entries")
	}

	// Verify config entries are present and decrypted.
	if string(all["organisations"]) != `["org1"]` {
		t.Errorf("organisations = %s, want %s", all["organisations"], `["org1"]`)
	}
	if string(all["collection"]) != `{"schedule":"0 * * * *"}` {
		t.Errorf("collection = %s, want %s", all["collection"], `{"schedule":"0 * * * *"}`)
	}
}

// ---------------------------------------------------------------------------
// IsEmpty
// ---------------------------------------------------------------------------

func TestIsEmpty(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	empty, err := s.IsEmpty(ctx)
	if err != nil {
		t.Fatalf("IsEmpty: %v", err)
	}
	if !empty {
		t.Error("expected empty on fresh store")
	}

	if err := s.Set(ctx, "key", json.RawMessage(`"val"`), false, "user"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	empty, err = s.IsEmpty(ctx)
	if err != nil {
		t.Fatalf("IsEmpty after insert: %v", err)
	}
	if empty {
		t.Error("expected not empty after insert")
	}
}

// ---------------------------------------------------------------------------
// Count
// ---------------------------------------------------------------------------

func TestCount(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	count, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	if err := s.Set(ctx, "a", json.RawMessage(`"1"`), false, "u"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set(ctx, "b", json.RawMessage(`"2"`), true, "u"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	count, err = s.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// NewStoreWithKey — rejects bad key lengths
// ---------------------------------------------------------------------------

func TestNewStoreWithKeyRejectsBadLength(t *testing.T) {
	db := newFakeDB()

	tests := []struct {
		name   string
		keyLen int
	}{
		{"too short 16", 16},
		{"too short 0", 0},
		{"too long 64", 64},
		{"too long 33", 33},
		{"too short 31", 31},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keyLen)
			_, err := NewStoreWithKey(db, key)
			if err == nil {
				t.Error("expected error for bad key length")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NewStoreWithKey — accepts exactly 32 bytes
// ---------------------------------------------------------------------------

func TestNewStoreWithKeyAccepts32Bytes(t *testing.T) {
	db := newFakeDB()
	key := make([]byte, 32)
	s, err := NewStoreWithKey(db, key)
	if err != nil {
		t.Fatalf("expected no error for 32-byte key, got: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil Store")
	}
}

// ---------------------------------------------------------------------------
// Operations without encryption key return ErrEncryptionKeyRequired
// ---------------------------------------------------------------------------

func TestOperationsWithoutKeyFail(t *testing.T) {
	db := newFakeDB()
	s := &Store{db: db, derivedKey: nil}
	ctx := context.Background()

	// Set should fail (needs encrypt).
	err := s.Set(ctx, "key", json.RawMessage(`"val"`), false, "user")
	if !errors.Is(err, ErrEncryptionKeyRequired) {
		if !containsString(err.Error(), "encryption key is required") {
			t.Errorf("Set: expected encryption key error, got: %v", err)
		}
	}

	// Manually insert a fake entry to test Get failure.
	fakeEntry := &datastore.ConfigEntry{
		Key:            "fake",
		EncryptedValue: []byte("garbage"),
		Nonce:          []byte("123456789012"),
		Secret:         false,
		UpdatedBy:      "test",
	}
	_ = db.SetConfigEntry(ctx, fakeEntry)

	_, err = s.Get(ctx, "fake")
	if !errors.Is(err, ErrEncryptionKeyRequired) {
		if !containsString(err.Error(), "encryption key is required") {
			t.Errorf("Get: expected encryption key error, got: %v", err)
		}
	}

	// GetAll should fail.
	_, err = s.GetAll(ctx)
	if !errors.Is(err, ErrEncryptionKeyRequired) {
		if !containsString(err.Error(), "encryption key is required") {
			t.Errorf("GetAll: expected encryption key error, got: %v", err)
		}
	}

	// List and IsEmpty should still work (no encryption needed).
	_, err = s.List(ctx)
	if err != nil {
		t.Errorf("List should work without key, got: %v", err)
	}

	_, err = s.IsEmpty(ctx)
	if err != nil {
		t.Errorf("IsEmpty should work without key, got: %v", err)
	}

	// Delete should work (no decryption needed).
	err = s.Delete(ctx, "fake")
	if err != nil {
		t.Errorf("Delete should work without key, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Database errors propagate correctly
// ---------------------------------------------------------------------------

func TestDatabaseErrorsPropagated(t *testing.T) {
	dbErr := errors.New("database is on fire")
	db := &errorDB{err: dbErr}
	s := mustNewStoreWithErrorDB(t, db)
	ctx := context.Background()

	_, err := s.Get(ctx, "key")
	if err == nil || !containsString(err.Error(), "database is on fire") {
		t.Errorf("Get should propagate DB error, got: %v", err)
	}

	err = s.Set(ctx, "key", json.RawMessage(`"val"`), false, "user")
	if err == nil || !containsString(err.Error(), "database is on fire") {
		t.Errorf("Set should propagate DB error, got: %v", err)
	}

	err = s.Delete(ctx, "key")
	if err == nil || !containsString(err.Error(), "database is on fire") {
		t.Errorf("Delete should propagate DB error, got: %v", err)
	}

	_, err = s.List(ctx)
	if err == nil || !containsString(err.Error(), "database is on fire") {
		t.Errorf("List should propagate DB error, got: %v", err)
	}

	_, err = s.ListByPrefix(ctx, "prefix/")
	if err == nil || !containsString(err.Error(), "database is on fire") {
		t.Errorf("ListByPrefix should propagate DB error, got: %v", err)
	}

	_, err = s.GetAll(ctx)
	if err == nil || !containsString(err.Error(), "database is on fire") {
		t.Errorf("GetAll should propagate DB error, got: %v", err)
	}

	_, err = s.IsEmpty(ctx)
	if err == nil || !containsString(err.Error(), "database is on fire") {
		t.Errorf("IsEmpty should propagate DB error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Overwrite — Set replaces existing value
// ---------------------------------------------------------------------------

func TestSetOverwritesExistingValue(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	if err := s.Set(ctx, "key", json.RawMessage(`"v1"`), false, "user"); err != nil {
		t.Fatalf("first Set: %v", err)
	}

	got1, err := s.Get(ctx, "key")
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if string(got1) != `"v1"` {
		t.Errorf("first Get = %s, want %q", got1, `"v1"`)
	}

	if err := s.Set(ctx, "key", json.RawMessage(`"v2"`), false, "user"); err != nil {
		t.Fatalf("second Set: %v", err)
	}

	got2, err := s.Get(ctx, "key")
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if string(got2) != `"v2"` {
		t.Errorf("second Get = %s, want %q", got2, `"v2"`)
	}
}

// ---------------------------------------------------------------------------
// Set changes secret flag on overwrite
// ---------------------------------------------------------------------------

func TestSetChangesSecretFlagOnOverwrite(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	// Initially not secret.
	if err := s.Set(ctx, "key", json.RawMessage(`"val"`), false, "user"); err != nil {
		t.Fatalf("Set (false): %v", err)
	}

	// GetSecret should fail.
	_, err := s.GetSecret(ctx, "key")
	if !errors.Is(err, ErrNotSecret) {
		t.Errorf("expected ErrNotSecret, got: %v", err)
	}

	// Overwrite as secret.
	if err := s.Set(ctx, "key", json.RawMessage(`"val"`), true, "user"); err != nil {
		t.Fatalf("Set (true): %v", err)
	}

	// Now GetSecret should succeed.
	got, err := s.GetSecret(ctx, "key")
	if err != nil {
		t.Fatalf("GetSecret after making secret: %v", err)
	}
	if string(got) != `"val"` {
		t.Errorf("GetSecret = %s, want %q", got, `"val"`)
	}
}

// ---------------------------------------------------------------------------
// GetEntry — lower-level access returns raw entry
// ---------------------------------------------------------------------------

func TestGetEntry(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	if err := s.Set(ctx, "test-key", json.RawMessage(`"data"`), true, "admin"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	entry, err := s.GetEntry(ctx, "test-key")
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Key != "test-key" {
		t.Errorf("Key = %q, want %q", entry.Key, "test-key")
	}
	if !entry.Secret {
		t.Error("Secret should be true")
	}
	if len(entry.EncryptedValue) == 0 {
		t.Error("EncryptedValue should not be empty")
	}
	if len(entry.Nonce) != nonceSize {
		t.Errorf("Nonce length = %d, want %d", len(entry.Nonce), nonceSize)
	}

	// Non-existent key returns nil.
	entry2, err := s.GetEntry(ctx, "no-such-key")
	if err != nil {
		t.Fatalf("GetEntry non-existent: %v", err)
	}
	if entry2 != nil {
		t.Error("expected nil for non-existent key")
	}
}

// ---------------------------------------------------------------------------
// Large values — verify no truncation
// ---------------------------------------------------------------------------

func TestLargeValueRoundTrip(t *testing.T) {
	db := newFakeDB()
	s := mustNewStore(t, db)
	ctx := context.Background()

	// Build a large JSON array (simulating a big organisations list).
	var orgs []map[string]string
	for i := 0; i < 100; i++ {
		orgs = append(orgs, map[string]string{
			"name":            "org-" + string(rune('A'+i%26)),
			"chef_server_url": "https://chef.example.com",
			"org_name":        "org-" + string(rune('A'+i%26)),
		})
	}
	data, err := json.Marshal(orgs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := s.Set(ctx, "large-orgs", json.RawMessage(data), false, "user"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := s.Get(ctx, "large-orgs")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if string(got) != string(data) {
		t.Errorf("large value round-trip failed: got %d bytes, want %d bytes", len(got), len(data))
	}
}

// ---------------------------------------------------------------------------
// NewStore with nil encryptor — metadata operations work, crypto fails
// ---------------------------------------------------------------------------

func TestNewStoreNilEncryptor(t *testing.T) {
	db := newFakeDB()
	s := NewStore(db, nil)
	ctx := context.Background()

	// Metadata operations should work.
	_, err := s.IsEmpty(ctx)
	if err != nil {
		t.Errorf("IsEmpty should work with nil encryptor: %v", err)
	}

	_, err = s.List(ctx)
	if err != nil {
		t.Errorf("List should work with nil encryptor: %v", err)
	}

	// Crypto operations should fail gracefully.
	err = s.Set(ctx, "key", json.RawMessage(`"val"`), false, "user")
	if err == nil {
		t.Error("Set should fail with nil encryptor")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustNewStoreWithErrorDB(t *testing.T, db DatastoreDB) *Store {
	t.Helper()
	s, err := NewStoreWithKey(db, testKey())
	if err != nil {
		t.Fatalf("NewStoreWithKey: %v", err)
	}
	return s
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
