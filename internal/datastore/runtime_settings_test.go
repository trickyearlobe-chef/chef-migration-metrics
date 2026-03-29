// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// GetRuntimeSetting — non-existent key
// ---------------------------------------------------------------------------

func TestFunctional_RuntimeSetting_GetNonExistent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	got, err := db.GetRuntimeSetting(ctx, "func-test-nonexistent-key-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent key, got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// SetRuntimeSetting + GetRuntimeSetting — round-trip
// ---------------------------------------------------------------------------

func TestFunctional_RuntimeSetting_SetAndGet(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-set-and-get"
	cleanupTestData(t, db, "DELETE FROM runtime_settings WHERE key = '"+key+"'")

	value := json.RawMessage(`{"enabled":true,"timeout":30}`)

	if err := db.SetRuntimeSetting(ctx, key, value, "admin@example.com"); err != nil {
		t.Fatalf("SetRuntimeSetting: %v", err)
	}

	got, err := db.GetRuntimeSetting(ctx, key)
	if err != nil {
		t.Fatalf("GetRuntimeSetting: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil setting after Set")
	}

	if got.Key != key {
		t.Errorf("Key = %q, want %q", got.Key, key)
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

	// Verify the stored JSON value.
	var parsed map[string]any
	if err := json.Unmarshal(got.Value, &parsed); err != nil {
		t.Fatalf("unmarshalling Value: %v", err)
	}
	if enabled, ok := parsed["enabled"].(bool); !ok || !enabled {
		t.Errorf("enabled = %v, want true", parsed["enabled"])
	}
	if timeout, ok := parsed["timeout"].(float64); !ok || int(timeout) != 30 {
		t.Errorf("timeout = %v, want 30", parsed["timeout"])
	}
}

// ---------------------------------------------------------------------------
// SetRuntimeSetting — upsert (update existing key)
// ---------------------------------------------------------------------------

func TestFunctional_RuntimeSetting_Upsert(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-upsert"
	cleanupTestData(t, db, "DELETE FROM runtime_settings WHERE key = '"+key+"'")

	// Initial insert.
	v1 := json.RawMessage(`{"version":1}`)
	if err := db.SetRuntimeSetting(ctx, key, v1, "user-a"); err != nil {
		t.Fatalf("first Set: %v", err)
	}

	got1, err := db.GetRuntimeSetting(ctx, key)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if got1 == nil {
		t.Fatal("expected non-nil after first Set")
	}
	firstUpdatedAt := got1.UpdatedAt

	// Small delay so NOW() advances.
	time.Sleep(50 * time.Millisecond)

	// Upsert with new value and different user.
	v2 := json.RawMessage(`{"version":2,"extra":"field"}`)
	if err := db.SetRuntimeSetting(ctx, key, v2, "user-b"); err != nil {
		t.Fatalf("second Set: %v", err)
	}

	got2, err := db.GetRuntimeSetting(ctx, key)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got2 == nil {
		t.Fatal("expected non-nil after second Set")
	}

	// Value should be updated.
	var parsed map[string]any
	if err := json.Unmarshal(got2.Value, &parsed); err != nil {
		t.Fatalf("unmarshalling Value: %v", err)
	}
	if version, ok := parsed["version"].(float64); !ok || int(version) != 2 {
		t.Errorf("version = %v, want 2", parsed["version"])
	}
	if extra, ok := parsed["extra"].(string); !ok || extra != "field" {
		t.Errorf("extra = %v, want %q", parsed["extra"], "field")
	}

	// updated_by should reflect the second caller.
	if got2.UpdatedBy != "user-b" {
		t.Errorf("UpdatedBy = %q, want %q", got2.UpdatedBy, "user-b")
	}

	// updated_at should have advanced.
	if !got2.UpdatedAt.After(firstUpdatedAt) {
		t.Errorf("UpdatedAt did not advance: first=%v, second=%v", firstUpdatedAt, got2.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// DeleteRuntimeSetting — removes the setting
// ---------------------------------------------------------------------------

func TestFunctional_RuntimeSetting_Delete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-delete"
	cleanupTestData(t, db, "DELETE FROM runtime_settings WHERE key = '"+key+"'")

	// Insert a setting.
	if err := db.SetRuntimeSetting(ctx, key, json.RawMessage(`{"x":1}`), "admin"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Verify it exists.
	got, err := db.GetRuntimeSetting(ctx, key)
	if err != nil {
		t.Fatalf("Get before delete: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil before delete")
	}

	// Delete.
	if err := db.DeleteRuntimeSetting(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify gone.
	got, err = db.GetRuntimeSetting(ctx, key)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// DeleteRuntimeSetting — idempotent on non-existent key
// ---------------------------------------------------------------------------

func TestFunctional_RuntimeSetting_DeleteNonExistent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	err := db.DeleteRuntimeSetting(ctx, "func-test-delete-nonexistent-xyz")
	if err != nil {
		t.Errorf("expected nil error for idempotent delete, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// JSONB round-trip — nested objects, arrays, mixed types
// ---------------------------------------------------------------------------

func TestFunctional_RuntimeSetting_JSONBRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-jsonb-roundtrip"
	cleanupTestData(t, db, "DELETE FROM runtime_settings WHERE key = '"+key+"'")

	complex := json.RawMessage(`{
		"string_field": "hello",
		"int_field": 42,
		"float_field": 3.14,
		"bool_field": true,
		"null_field": null,
		"nested": {
			"inner_key": "inner_value",
			"deep": {"level": 3}
		},
		"array_of_strings": ["a", "b", "c"],
		"array_of_objects": [
			{"name": "first", "value": 1},
			{"name": "second", "value": 2}
		],
		"empty_object": {},
		"empty_array": []
	}`)

	if err := db.SetRuntimeSetting(ctx, key, complex, "test"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := db.GetRuntimeSetting(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}

	var parsed map[string]any
	if err := json.Unmarshal(got.Value, &parsed); err != nil {
		t.Fatalf("unmarshalling Value: %v", err)
	}

	// String field.
	if s, ok := parsed["string_field"].(string); !ok || s != "hello" {
		t.Errorf("string_field = %v, want %q", parsed["string_field"], "hello")
	}

	// Int field (JSON numbers are float64 in Go).
	if n, ok := parsed["int_field"].(float64); !ok || int(n) != 42 {
		t.Errorf("int_field = %v, want 42", parsed["int_field"])
	}

	// Float field.
	if f, ok := parsed["float_field"].(float64); !ok || f != 3.14 {
		t.Errorf("float_field = %v, want 3.14", parsed["float_field"])
	}

	// Bool field.
	if b, ok := parsed["bool_field"].(bool); !ok || !b {
		t.Errorf("bool_field = %v, want true", parsed["bool_field"])
	}

	// Null field.
	if parsed["null_field"] != nil {
		t.Errorf("null_field = %v, want nil", parsed["null_field"])
	}

	// Nested object.
	nested, ok := parsed["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested: expected object, got %T", parsed["nested"])
	}
	if nested["inner_key"] != "inner_value" {
		t.Errorf("nested.inner_key = %v, want %q", nested["inner_key"], "inner_value")
	}
	deep, ok := nested["deep"].(map[string]any)
	if !ok {
		t.Fatalf("nested.deep: expected object, got %T", nested["deep"])
	}
	if level, ok := deep["level"].(float64); !ok || int(level) != 3 {
		t.Errorf("nested.deep.level = %v, want 3", deep["level"])
	}

	// Array of strings.
	arr, ok := parsed["array_of_strings"].([]any)
	if !ok {
		t.Fatalf("array_of_strings: expected array, got %T", parsed["array_of_strings"])
	}
	if len(arr) != 3 {
		t.Errorf("array_of_strings length = %d, want 3", len(arr))
	}

	// Array of objects.
	objArr, ok := parsed["array_of_objects"].([]any)
	if !ok {
		t.Fatalf("array_of_objects: expected array, got %T", parsed["array_of_objects"])
	}
	if len(objArr) != 2 {
		t.Errorf("array_of_objects length = %d, want 2", len(objArr))
	}
	first, ok := objArr[0].(map[string]any)
	if !ok {
		t.Fatalf("array_of_objects[0]: expected object, got %T", objArr[0])
	}
	if first["name"] != "first" {
		t.Errorf("array_of_objects[0].name = %v, want %q", first["name"], "first")
	}

	// Empty object.
	emptyObj, ok := parsed["empty_object"].(map[string]any)
	if !ok {
		t.Fatalf("empty_object: expected object, got %T", parsed["empty_object"])
	}
	if len(emptyObj) != 0 {
		t.Errorf("empty_object length = %d, want 0", len(emptyObj))
	}

	// Empty array.
	emptyArr, ok := parsed["empty_array"].([]any)
	if !ok {
		t.Fatalf("empty_array: expected array, got %T", parsed["empty_array"])
	}
	if len(emptyArr) != 0 {
		t.Errorf("empty_array length = %d, want 0", len(emptyArr))
	}
}

// ---------------------------------------------------------------------------
// updated_at and updated_by — set correctly on insert and update
// ---------------------------------------------------------------------------

func TestFunctional_RuntimeSetting_UpdatedAtAndBy(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const key = "func-test-updated-fields"
	cleanupTestData(t, db, "DELETE FROM runtime_settings WHERE key = '"+key+"'")

	before := time.Now().UTC().Add(-1 * time.Second)

	if err := db.SetRuntimeSetting(ctx, key, json.RawMessage(`{}`), "creator"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	after := time.Now().UTC().Add(1 * time.Second)

	got, err := db.GetRuntimeSetting(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}

	// updated_at should be between before and after.
	if got.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt %v is before test start %v", got.UpdatedAt, before)
	}
	if got.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt %v is after test end %v", got.UpdatedAt, after)
	}

	// updated_by should match.
	if got.UpdatedBy != "creator" {
		t.Errorf("UpdatedBy = %q, want %q", got.UpdatedBy, "creator")
	}

	// Update and verify updated_by changes.
	time.Sleep(50 * time.Millisecond)
	if err := db.SetRuntimeSetting(ctx, key, json.RawMessage(`{"v":2}`), "updater"); err != nil {
		t.Fatalf("second Set: %v", err)
	}

	got2, err := db.GetRuntimeSetting(ctx, key)
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
// RuntimeSetting struct — zero value
// ---------------------------------------------------------------------------

func TestFunctional_RuntimeSetting_ZeroValue(t *testing.T) {
	var s RuntimeSetting
	if s.Key != "" {
		t.Errorf("zero-value Key should be empty, got %q", s.Key)
	}
	if s.Value != nil {
		t.Errorf("zero-value Value should be nil, got %v", s.Value)
	}
	if s.UpdatedBy != "" {
		t.Errorf("zero-value UpdatedBy should be empty, got %q", s.UpdatedBy)
	}
	if !s.UpdatedAt.IsZero() {
		t.Errorf("zero-value UpdatedAt should be zero, got %v", s.UpdatedAt)
	}
}

// ---------------------------------------------------------------------------
// RuntimeSetting struct — JSON marshalling
// ---------------------------------------------------------------------------

func TestFunctional_RuntimeSetting_MarshalJSON(t *testing.T) {
	s := RuntimeSetting{
		Key:       "test_kitchen",
		Value:     json.RawMessage(`{"driver":"dokken","concurrency":4}`),
		UpdatedAt: time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC),
		UpdatedBy: "admin@example.com",
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if m["key"] != "test_kitchen" {
		t.Errorf("key = %v, want %q", m["key"], "test_kitchen")
	}
	if m["updated_by"] != "admin@example.com" {
		t.Errorf("updated_by = %v, want %q", m["updated_by"], "admin@example.com")
	}

	// Value should be the nested object, not a string.
	valueMap, ok := m["value"].(map[string]any)
	if !ok {
		t.Fatalf("value should be an object, got %T", m["value"])
	}
	if valueMap["driver"] != "dokken" {
		t.Errorf("value.driver = %v, want %q", valueMap["driver"], "dokken")
	}
	if conc, ok := valueMap["concurrency"].(float64); !ok || int(conc) != 4 {
		t.Errorf("value.concurrency = %v, want 4", valueMap["concurrency"])
	}
}
