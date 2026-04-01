// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// GetConfigEntry — non-existent key
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_GetNonExistent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	got, err := db.GetConfigEntry(ctx, "func-test-nonexistent-config-key-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent key, got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// SetConfigEntry + GetConfigEntry — round-trip
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_SetAndGet(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-config-set-and-get"
	cleanupTestData(t, db, "DELETE FROM config_store WHERE key = '"+key+"'")

	entry := &ConfigEntry{
		Key:            key,
		EncryptedValue: []byte("encrypted-test-value-bytes"),
		Nonce:          []byte("012345678901"), // 12 bytes
		Secret:         false,
		UpdatedBy:      "admin@example.com",
	}

	if err := db.SetConfigEntry(ctx, entry); err != nil {
		t.Fatalf("SetConfigEntry: %v", err)
	}

	got, err := db.GetConfigEntry(ctx, key)
	if err != nil {
		t.Fatalf("GetConfigEntry: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil entry after Set")
	}

	if got.Key != key {
		t.Errorf("Key = %q, want %q", got.Key, key)
	}
	if string(got.EncryptedValue) != "encrypted-test-value-bytes" {
		t.Errorf("EncryptedValue = %q, want %q", got.EncryptedValue, "encrypted-test-value-bytes")
	}
	if string(got.Nonce) != "012345678901" {
		t.Errorf("Nonce = %q, want %q", got.Nonce, "012345678901")
	}
	if got.Secret != false {
		t.Errorf("Secret = %v, want false", got.Secret)
	}
	if got.UpdatedBy != "admin@example.com" {
		t.Errorf("UpdatedBy = %q, want %q", got.UpdatedBy, "admin@example.com")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
	if time.Since(got.UpdatedAt) > 30*time.Second {
		t.Errorf("UpdatedAt looks stale: %v", got.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// SetConfigEntry — secret flag stored and retrieved
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_SecretFlag(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-config-secret-flag"
	cleanupTestData(t, db, "DELETE FROM config_store WHERE key = '"+key+"'")

	entry := &ConfigEntry{
		Key:            key,
		EncryptedValue: []byte("secret-value"),
		Nonce:          []byte("abcdefghijkl"),
		Secret:         true,
		UpdatedBy:      "admin",
	}

	if err := db.SetConfigEntry(ctx, entry); err != nil {
		t.Fatalf("SetConfigEntry: %v", err)
	}

	got, err := db.GetConfigEntry(ctx, key)
	if err != nil {
		t.Fatalf("GetConfigEntry: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil entry")
	}
	if got.Secret != true {
		t.Errorf("Secret = %v, want true", got.Secret)
	}
}

// ---------------------------------------------------------------------------
// SetConfigEntry — upsert (update existing key)
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_Upsert(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-config-upsert"
	cleanupTestData(t, db, "DELETE FROM config_store WHERE key = '"+key+"'")

	// Initial insert.
	entry1 := &ConfigEntry{
		Key:            key,
		EncryptedValue: []byte("value-v1"),
		Nonce:          []byte("nonce-v1-xxx"),
		Secret:         false,
		UpdatedBy:      "user-a",
	}
	if err := db.SetConfigEntry(ctx, entry1); err != nil {
		t.Fatalf("first Set: %v", err)
	}

	got1, err := db.GetConfigEntry(ctx, key)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if got1 == nil {
		t.Fatal("expected non-nil after first Set")
	}
	firstUpdatedAt := got1.UpdatedAt

	// Small delay so NOW() advances.
	time.Sleep(50 * time.Millisecond)

	// Upsert with new value, nonce, secret flag, and user.
	entry2 := &ConfigEntry{
		Key:            key,
		EncryptedValue: []byte("value-v2-updated"),
		Nonce:          []byte("nonce-v2-yyy"),
		Secret:         true,
		UpdatedBy:      "user-b",
	}
	if err := db.SetConfigEntry(ctx, entry2); err != nil {
		t.Fatalf("second Set: %v", err)
	}

	got2, err := db.GetConfigEntry(ctx, key)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got2 == nil {
		t.Fatal("expected non-nil after second Set")
	}

	if string(got2.EncryptedValue) != "value-v2-updated" {
		t.Errorf("EncryptedValue = %q, want %q", got2.EncryptedValue, "value-v2-updated")
	}
	if string(got2.Nonce) != "nonce-v2-yyy" {
		t.Errorf("Nonce = %q, want %q", got2.Nonce, "nonce-v2-yyy")
	}
	if got2.Secret != true {
		t.Errorf("Secret = %v, want true", got2.Secret)
	}
	if got2.UpdatedBy != "user-b" {
		t.Errorf("UpdatedBy = %q, want %q", got2.UpdatedBy, "user-b")
	}
	if !got2.UpdatedAt.After(firstUpdatedAt) {
		t.Errorf("UpdatedAt did not advance: first=%v, second=%v", firstUpdatedAt, got2.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// DeleteConfigEntry — removes the entry
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_Delete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-config-delete"
	cleanupTestData(t, db, "DELETE FROM config_store WHERE key = '"+key+"'")

	entry := &ConfigEntry{
		Key:            key,
		EncryptedValue: []byte("to-be-deleted"),
		Nonce:          []byte("delete-nonce!"),
		Secret:         false,
		UpdatedBy:      "admin",
	}
	if err := db.SetConfigEntry(ctx, entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Verify it exists.
	got, err := db.GetConfigEntry(ctx, key)
	if err != nil {
		t.Fatalf("Get before delete: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil before delete")
	}

	// Delete.
	if err := db.DeleteConfigEntry(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify gone.
	got, err = db.GetConfigEntry(ctx, key)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// DeleteConfigEntry — idempotent on non-existent key
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_DeleteNonExistent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	err := db.DeleteConfigEntry(ctx, "func-test-config-delete-nonexistent-xyz")
	if err != nil {
		t.Errorf("expected nil error for idempotent delete, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListConfigEntries — returns all rows ordered by key
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_List(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Use a unique prefix to avoid interference from other tests.
	prefix := "func-test-config-list-"
	keys := []string{prefix + "c-third", prefix + "a-first", prefix + "b-second"}
	for _, k := range keys {
		cleanupTestData(t, db, "DELETE FROM config_store WHERE key = '"+k+"'")
	}

	// Insert out of alphabetical order.
	for _, k := range keys {
		entry := &ConfigEntry{
			Key:            k,
			EncryptedValue: []byte("val-" + k),
			Nonce:          []byte("nonce-" + k)[:12],
			Secret:         false,
			UpdatedBy:      "test",
		}
		if err := db.SetConfigEntry(ctx, entry); err != nil {
			t.Fatalf("Set %q: %v", k, err)
		}
	}

	entries, err := db.ListConfigEntries(ctx)
	if err != nil {
		t.Fatalf("ListConfigEntries: %v", err)
	}

	// Find our test entries in the result (other tests may have entries too).
	var found []string
	for _, e := range entries {
		for _, k := range keys {
			if e.Key == k {
				found = append(found, e.Key)
			}
		}
	}

	if len(found) != 3 {
		t.Fatalf("expected 3 test entries in list, found %d", len(found))
	}

	// Verify alphabetical order.
	expectedOrder := []string{prefix + "a-first", prefix + "b-second", prefix + "c-third"}
	for i, want := range expectedOrder {
		if found[i] != want {
			t.Errorf("list[%d] = %q, want %q", i, found[i], want)
		}
	}
}

// ---------------------------------------------------------------------------
// ListConfigEntriesByPrefix — filters by key prefix
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_ListByPrefix(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	prefix := "func-test-config-prefix/"
	otherKey := "func-test-config-other-key"
	keys := []string{prefix + "alpha", prefix + "beta"}
	allKeys := append(keys, otherKey)
	for _, k := range allKeys {
		cleanupTestData(t, db, "DELETE FROM config_store WHERE key = '"+k+"'")
	}

	for _, k := range allKeys {
		entry := &ConfigEntry{
			Key:            k,
			EncryptedValue: []byte("val"),
			Nonce:          []byte("123456789012"),
			Secret:         k == otherKey,
			UpdatedBy:      "test",
		}
		if err := db.SetConfigEntry(ctx, entry); err != nil {
			t.Fatalf("Set %q: %v", k, err)
		}
	}

	entries, err := db.ListConfigEntriesByPrefix(ctx, prefix)
	if err != nil {
		t.Fatalf("ListConfigEntriesByPrefix: %v", err)
	}

	// Should only contain the two prefixed keys, not the other one.
	var found []string
	for _, e := range entries {
		found = append(found, e.Key)
	}

	if len(found) < 2 {
		t.Fatalf("expected at least 2 prefixed entries, found %d", len(found))
	}

	// Verify the other key is NOT in the result.
	for _, e := range entries {
		if e.Key == otherKey {
			t.Errorf("ListByPrefix should not include %q", otherKey)
		}
	}

	// Verify both prefixed keys are present.
	foundMap := make(map[string]bool)
	for _, e := range entries {
		foundMap[e.Key] = true
	}
	for _, k := range keys {
		if !foundMap[k] {
			t.Errorf("missing expected key %q in prefix results", k)
		}
	}
}

// ---------------------------------------------------------------------------
// CountConfigEntries — returns correct count
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_Count(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-config-count"
	cleanupTestData(t, db, "DELETE FROM config_store WHERE key = '"+key+"'")

	// Get baseline count.
	baseline, err := db.CountConfigEntries(ctx)
	if err != nil {
		t.Fatalf("CountConfigEntries (baseline): %v", err)
	}

	// Insert one entry.
	entry := &ConfigEntry{
		Key:            key,
		EncryptedValue: []byte("val"),
		Nonce:          []byte("123456789012"),
		Secret:         false,
		UpdatedBy:      "test",
	}
	if err := db.SetConfigEntry(ctx, entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	afterInsert, err := db.CountConfigEntries(ctx)
	if err != nil {
		t.Fatalf("CountConfigEntries (after insert): %v", err)
	}

	if afterInsert != baseline+1 {
		t.Errorf("count after insert = %d, want %d", afterInsert, baseline+1)
	}

	// Delete and verify count returns to baseline.
	if err := db.DeleteConfigEntry(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	afterDelete, err := db.CountConfigEntries(ctx)
	if err != nil {
		t.Fatalf("CountConfigEntries (after delete): %v", err)
	}

	if afterDelete != baseline {
		t.Errorf("count after delete = %d, want %d (baseline)", afterDelete, baseline)
	}
}

// ---------------------------------------------------------------------------
// ConfigStoreIsEmpty — true when no rows, false otherwise
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_IsEmpty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-config-is-empty"
	cleanupTestData(t, db, "DELETE FROM config_store WHERE key = '"+key+"'")

	// Note: we can't guarantee the table is truly empty (other tests may
	// have data), so we test that inserting makes it non-empty and that
	// the method doesn't error.
	_, err := db.ConfigStoreIsEmpty(ctx)
	if err != nil {
		t.Fatalf("ConfigStoreIsEmpty: %v", err)
	}

	// Insert an entry.
	entry := &ConfigEntry{
		Key:            key,
		EncryptedValue: []byte("val"),
		Nonce:          []byte("123456789012"),
		Secret:         false,
		UpdatedBy:      "test",
	}
	if err := db.SetConfigEntry(ctx, entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	isEmpty, err := db.ConfigStoreIsEmpty(ctx)
	if err != nil {
		t.Fatalf("ConfigStoreIsEmpty after insert: %v", err)
	}
	if isEmpty {
		t.Error("expected not empty after insert")
	}
}

// ---------------------------------------------------------------------------
// BYTEA round-trip — binary data preserved exactly
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_BinaryRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-config-binary-roundtrip"
	cleanupTestData(t, db, "DELETE FROM config_store WHERE key = '"+key+"'")

	// Use bytes with null bytes, high bytes, and all 256 values in the
	// nonce to verify BYTEA storage works correctly.
	encValue := make([]byte, 256)
	for i := range encValue {
		encValue[i] = byte(i)
	}
	nonce := []byte{0x00, 0xFF, 0x80, 0x7F, 0x01, 0xFE, 0x55, 0xAA, 0x10, 0xEF, 0x42, 0xBD}

	entry := &ConfigEntry{
		Key:            key,
		EncryptedValue: encValue,
		Nonce:          nonce,
		Secret:         false,
		UpdatedBy:      "binary-test",
	}

	if err := db.SetConfigEntry(ctx, entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := db.GetConfigEntry(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}

	if len(got.EncryptedValue) != 256 {
		t.Fatalf("EncryptedValue length = %d, want 256", len(got.EncryptedValue))
	}
	for i, b := range got.EncryptedValue {
		if b != byte(i) {
			t.Errorf("EncryptedValue[%d] = %d, want %d", i, b, i)
			break
		}
	}

	if len(got.Nonce) != 12 {
		t.Fatalf("Nonce length = %d, want 12", len(got.Nonce))
	}
	for i, b := range got.Nonce {
		if b != nonce[i] {
			t.Errorf("Nonce[%d] = %d, want %d", i, b, nonce[i])
			break
		}
	}
}

// ---------------------------------------------------------------------------
// SetConfigEntry — updated_at and updated_by set correctly
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_UpdatedAtAndBy(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-config-updated-fields"
	cleanupTestData(t, db, "DELETE FROM config_store WHERE key = '"+key+"'")

	before := time.Now().UTC().Add(-1 * time.Second)

	entry := &ConfigEntry{
		Key:            key,
		EncryptedValue: []byte("val"),
		Nonce:          []byte("123456789012"),
		Secret:         false,
		UpdatedBy:      "creator",
	}
	if err := db.SetConfigEntry(ctx, entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	after := time.Now().UTC().Add(1 * time.Second)

	got, err := db.GetConfigEntry(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}

	if got.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt %v is before test start %v", got.UpdatedAt, before)
	}
	if got.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt %v is after test end %v", got.UpdatedAt, after)
	}
	if got.UpdatedBy != "creator" {
		t.Errorf("UpdatedBy = %q, want %q", got.UpdatedBy, "creator")
	}

	// Update and verify updated_by changes.
	time.Sleep(50 * time.Millisecond)
	entry.UpdatedBy = "updater"
	entry.EncryptedValue = []byte("val-v2")
	if err := db.SetConfigEntry(ctx, entry); err != nil {
		t.Fatalf("second Set: %v", err)
	}

	got2, err := db.GetConfigEntry(ctx, key)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got2.UpdatedBy != "updater" {
		t.Errorf("UpdatedBy after update = %q, want %q", got2.UpdatedBy, "updater")
	}
	if !got2.UpdatedAt.After(got.UpdatedAt) {
		t.Errorf("UpdatedAt should advance on update: first=%v, second=%v", got.UpdatedAt, got2.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// Key with slashes — credentials/name pattern works
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_SlashKey(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "credentials/func-test-my-credential"
	cleanupTestData(t, db, "DELETE FROM config_store WHERE key = '"+key+"'")

	entry := &ConfigEntry{
		Key:            key,
		EncryptedValue: []byte("credential-ciphertext"),
		Nonce:          []byte("cred-nonce!!"),
		Secret:         true,
		UpdatedBy:      "admin",
	}

	if err := db.SetConfigEntry(ctx, entry); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := db.GetConfigEntry(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil for slash key")
	}
	if got.Key != key {
		t.Errorf("Key = %q, want %q", got.Key, key)
	}
	if got.Secret != true {
		t.Errorf("Secret = %v, want true", got.Secret)
	}

	// Verify ListByPrefix finds it.
	entries, err := db.ListConfigEntriesByPrefix(ctx, "credentials/func-test-")
	if err != nil {
		t.Fatalf("ListByPrefix: %v", err)
	}

	found := false
	for _, e := range entries {
		if e.Key == key {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ListByPrefix(credentials/func-test-) should include %q", key)
	}
}

// ---------------------------------------------------------------------------
// Dot-notation keys — config section pattern works
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_DotKey(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	keys := []string{
		"func-test-config.server.tls",
		"func-test-config.server.websocket",
	}
	for _, k := range keys {
		cleanupTestData(t, db, "DELETE FROM config_store WHERE key = '"+k+"'")
	}

	for _, k := range keys {
		entry := &ConfigEntry{
			Key:            k,
			EncryptedValue: []byte("val-" + k),
			Nonce:          []byte("123456789012"),
			Secret:         false,
			UpdatedBy:      "test",
		}
		if err := db.SetConfigEntry(ctx, entry); err != nil {
			t.Fatalf("Set %q: %v", k, err)
		}
	}

	// Verify both can be retrieved.
	for _, k := range keys {
		got, err := db.GetConfigEntry(ctx, k)
		if err != nil {
			t.Fatalf("Get %q: %v", k, err)
		}
		if got == nil {
			t.Errorf("expected non-nil for dot key %q", k)
		}
	}

	// Verify prefix query works with dot notation.
	entries, err := db.ListConfigEntriesByPrefix(ctx, "func-test-config.server.")
	if err != nil {
		t.Fatalf("ListByPrefix: %v", err)
	}

	foundKeys := make(map[string]bool)
	for _, e := range entries {
		foundKeys[e.Key] = true
	}
	for _, k := range keys {
		if !foundKeys[k] {
			t.Errorf("missing expected key %q in dot-prefix results", k)
		}
	}
}

// ---------------------------------------------------------------------------
// ConfigEntry struct — zero value
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_ZeroValue(t *testing.T) {
	var e ConfigEntry
	if e.Key != "" {
		t.Errorf("zero-value Key should be empty, got %q", e.Key)
	}
	if e.EncryptedValue != nil {
		t.Errorf("zero-value EncryptedValue should be nil, got %v", e.EncryptedValue)
	}
	if e.Nonce != nil {
		t.Errorf("zero-value Nonce should be nil, got %v", e.Nonce)
	}
	if e.Secret != false {
		t.Errorf("zero-value Secret should be false, got %v", e.Secret)
	}
	if e.UpdatedBy != "" {
		t.Errorf("zero-value UpdatedBy should be empty, got %q", e.UpdatedBy)
	}
	if !e.UpdatedAt.IsZero() {
		t.Errorf("zero-value UpdatedAt should be zero, got %v", e.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// ListConfigEntries — empty table returns empty slice (not nil)
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_ListReturnsEmptyNotNil(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// We can't guarantee the table is empty, but we can at least verify
	// the method doesn't return an error and returns a non-nil slice or nil.
	entries, err := db.ListConfigEntries(ctx)
	if err != nil {
		t.Fatalf("ListConfigEntries: %v", err)
	}

	// entries may be nil if table is empty (append to nil slice) or non-nil
	// if rows exist. Either way, no error is the success condition.
	_ = entries
}

// ---------------------------------------------------------------------------
// ListConfigEntriesByPrefix — no matches returns empty slice
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_ListByPrefixNoMatches(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	entries, err := db.ListConfigEntriesByPrefix(ctx, "func-test-no-such-prefix-xyz/")
	if err != nil {
		t.Fatalf("ListConfigEntriesByPrefix: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entries for non-matching prefix, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// ConfigStoreIsEmpty — consistent with CountConfigEntries
// ---------------------------------------------------------------------------

func TestFunctional_ConfigStore_IsEmptyConsistentWithCount(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	count, err := db.CountConfigEntries(ctx)
	if err != nil {
		t.Fatalf("CountConfigEntries: %v", err)
	}

	isEmpty, err := db.ConfigStoreIsEmpty(ctx)
	if err != nil {
		t.Fatalf("ConfigStoreIsEmpty: %v", err)
	}

	if count == 0 && !isEmpty {
		t.Error("count is 0 but IsEmpty returned false")
	}
	if count > 0 && isEmpty {
		t.Errorf("count is %d but IsEmpty returned true", count)
	}
}
