// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// ---------------------------------------------------------------------------
// fakeRefChecker — test double for CredentialReferenceChecker
// ---------------------------------------------------------------------------

type fakeRefChecker struct {
	refs map[string][]secrets.CredentialReference
}

func newFakeRefChecker() *fakeRefChecker {
	return &fakeRefChecker{refs: make(map[string][]secrets.CredentialReference)}
}

func (f *fakeRefChecker) addRef(credName, entityType, entityName string) {
	f.refs[credName] = append(f.refs[credName], secrets.CredentialReference{
		EntityType: entityType,
		EntityName: entityName,
	})
}

func (f *fakeRefChecker) CheckCredentialReferences(_ context.Context, name string) ([]secrets.CredentialReference, error) {
	return f.refs[name], nil
}

// errorRefChecker always returns an error.
type errorRefChecker struct{ err error }

func (e *errorRefChecker) CheckCredentialReferences(context.Context, string) ([]secrets.CredentialReference, error) {
	return nil, e.err
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustNewAdapter(t *testing.T) (*CredentialStoreAdapter, *fakeRefChecker) {
	t.Helper()
	db := newFakeDB()
	store := mustNewStore(t, db)
	rc := newFakeRefChecker()
	adapter := NewCredentialStoreAdapter(store, rc)
	return adapter, rc
}

func mustNewAdapterNoRefCheck(t *testing.T) *CredentialStoreAdapter {
	t.Helper()
	db := newFakeDB()
	store := mustNewStore(t, db)
	return NewCredentialStoreAdapter(store, nil)
}

func createTestCredential(t *testing.T, adapter *CredentialStoreAdapter, name, credType, value, createdBy string) *secrets.CredentialMetadata {
	t.Helper()
	meta, err := adapter.Create(context.Background(), secrets.CreateCredentialInput{
		Name:           name,
		CredentialType: credType,
		Plaintext:      []byte(value),
		CreatedBy:      createdBy,
	})
	if err != nil {
		t.Fatalf("Create credential %q: %v", name, err)
	}
	return meta
}

// ---------------------------------------------------------------------------
// Compile-time check: implements CredentialStore
// ---------------------------------------------------------------------------

func TestCredentialStoreAdapter_ImplementsInterface(t *testing.T) {
	var _ secrets.CredentialStore = (*CredentialStoreAdapter)(nil)
}

// ---------------------------------------------------------------------------
// Create — happy path
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Create(t *testing.T) {
	adapter, _ := mustNewAdapter(t)
	ctx := context.Background()

	meta, err := adapter.Create(ctx, secrets.CreateCredentialInput{
		Name:           "my-cred",
		CredentialType: "generic",
		Plaintext:      []byte("my-secret-value"),
		CreatedBy:      "admin@example.com",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if meta.Name != "my-cred" {
		t.Errorf("Name = %q, want %q", meta.Name, "my-cred")
	}
	if meta.CredentialType != "generic" {
		t.Errorf("CredentialType = %q, want %q", meta.CredentialType, "generic")
	}
	if meta.CreatedBy != "admin@example.com" {
		t.Errorf("CreatedBy = %q, want %q", meta.CreatedBy, "admin@example.com")
	}
	if meta.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if meta.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

// ---------------------------------------------------------------------------
// Create — duplicate name returns ErrCredentialAlreadyExists
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Create_Duplicate(t *testing.T) {
	adapter, _ := mustNewAdapter(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "dup-cred", "generic", "value1", "admin")

	_, err := adapter.Create(ctx, secrets.CreateCredentialInput{
		Name:           "dup-cred",
		CredentialType: "generic",
		Plaintext:      []byte("value2"),
		CreatedBy:      "admin",
	})
	if !errors.Is(err, secrets.ErrCredentialAlreadyExists) {
		t.Errorf("expected ErrCredentialAlreadyExists, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Create — empty name returns error
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Create_EmptyName(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	_, err := adapter.Create(context.Background(), secrets.CreateCredentialInput{
		Name:           "",
		CredentialType: "generic",
		Plaintext:      []byte("val"),
		CreatedBy:      "admin",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

// ---------------------------------------------------------------------------
// Create — empty created_by returns error
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Create_EmptyCreatedBy(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	_, err := adapter.Create(context.Background(), secrets.CreateCredentialInput{
		Name:           "cred",
		CredentialType: "generic",
		Plaintext:      []byte("val"),
		CreatedBy:      "",
	})
	if err == nil {
		t.Fatal("expected error for empty created_by")
	}
}

// ---------------------------------------------------------------------------
// Create — invalid credential type returns validation error
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Create_InvalidType(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	_, err := adapter.Create(context.Background(), secrets.CreateCredentialInput{
		Name:           "cred",
		CredentialType: "invalid_type",
		Plaintext:      []byte("val"),
		CreatedBy:      "admin",
	})
	if err == nil {
		t.Fatal("expected error for invalid credential type")
	}
}

// ---------------------------------------------------------------------------
// Get — happy path round-trip
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Get(t *testing.T) {
	adapter, _ := mustNewAdapter(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "get-cred", "generic", "the-secret", "admin")

	cred, err := adapter.Get(ctx, "get-cred")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if cred.Name != "get-cred" {
		t.Errorf("Name = %q, want %q", cred.Name, "get-cred")
	}
	if cred.CredentialType != "generic" {
		t.Errorf("CredentialType = %q, want %q", cred.CredentialType, "generic")
	}
	if string(cred.Plaintext) != "the-secret" {
		t.Errorf("Plaintext = %q, want %q", cred.Plaintext, "the-secret")
	}
	if cred.CreatedBy != "admin" {
		t.Errorf("CreatedBy = %q, want %q", cred.CreatedBy, "admin")
	}
	if cred.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	// Zero the plaintext as required by the contract.
	secrets.ZeroBytes(cred.Plaintext)
}

// ---------------------------------------------------------------------------
// Get — non-existent credential returns ErrCredentialNotFound
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Get_NotFound(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	_, err := adapter.Get(context.Background(), "no-such-cred")
	if !errors.Is(err, secrets.ErrCredentialNotFound) {
		t.Errorf("expected ErrCredentialNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetMetadata — returns metadata without plaintext
// ---------------------------------------------------------------------------

func TestCredentialAdapter_GetMetadata(t *testing.T) {
	adapter, _ := mustNewAdapter(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "meta-cred", "generic", "secret-val", "alice")

	meta, err := adapter.GetMetadata(ctx, "meta-cred")
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}

	if meta.Name != "meta-cred" {
		t.Errorf("Name = %q, want %q", meta.Name, "meta-cred")
	}
	if meta.CredentialType != "generic" {
		t.Errorf("CredentialType = %q, want %q", meta.CredentialType, "generic")
	}
	if meta.CreatedBy != "alice" {
		t.Errorf("CreatedBy = %q, want %q", meta.CreatedBy, "alice")
	}
}

// ---------------------------------------------------------------------------
// GetMetadata — non-existent credential returns ErrCredentialNotFound
// ---------------------------------------------------------------------------

func TestCredentialAdapter_GetMetadata_NotFound(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	_, err := adapter.GetMetadata(context.Background(), "no-such-cred")
	if !errors.Is(err, secrets.ErrCredentialNotFound) {
		t.Errorf("expected ErrCredentialNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Update — rotates value, preserves type and created_by
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Update(t *testing.T) {
	adapter, _ := mustNewAdapter(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "update-cred", "generic", "old-secret", "alice")

	// Small delay so updated_at differs.
	time.Sleep(10 * time.Millisecond)

	meta, err := adapter.Update(ctx, secrets.UpdateCredentialInput{
		Name:      "update-cred",
		Plaintext: []byte("new-secret"),
		UpdatedBy: "bob",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if meta.Name != "update-cred" {
		t.Errorf("Name = %q, want %q", meta.Name, "update-cred")
	}
	if meta.CredentialType != "generic" {
		t.Errorf("CredentialType = %q, want %q (should be preserved)", meta.CredentialType, "generic")
	}
	if meta.CreatedBy != "alice" {
		t.Errorf("CreatedBy = %q, want %q (should be preserved)", meta.CreatedBy, "alice")
	}
	if meta.UpdatedBy != "bob" {
		t.Errorf("UpdatedBy = %q, want %q", meta.UpdatedBy, "bob")
	}
	if meta.LastRotatedAt == nil {
		t.Error("LastRotatedAt should be set after update")
	}

	// Verify the new value is stored.
	cred, err := adapter.Get(ctx, "update-cred")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if string(cred.Plaintext) != "new-secret" {
		t.Errorf("Plaintext after update = %q, want %q", cred.Plaintext, "new-secret")
	}
	secrets.ZeroBytes(cred.Plaintext)
}

// ---------------------------------------------------------------------------
// Update — non-existent credential returns ErrCredentialNotFound
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Update_NotFound(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	_, err := adapter.Update(context.Background(), secrets.UpdateCredentialInput{
		Name:      "no-such-cred",
		Plaintext: []byte("val"),
		UpdatedBy: "admin",
	})
	if !errors.Is(err, secrets.ErrCredentialNotFound) {
		t.Errorf("expected ErrCredentialNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Update — empty name returns error
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Update_EmptyName(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	_, err := adapter.Update(context.Background(), secrets.UpdateCredentialInput{
		Name:      "",
		Plaintext: []byte("val"),
		UpdatedBy: "admin",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

// ---------------------------------------------------------------------------
// Update — empty updated_by returns error
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Update_EmptyUpdatedBy(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	createTestCredential(t, adapter, "cred-for-update", "generic", "val", "admin")

	_, err := adapter.Update(context.Background(), secrets.UpdateCredentialInput{
		Name:      "cred-for-update",
		Plaintext: []byte("newval"),
		UpdatedBy: "",
	})
	if err == nil {
		t.Fatal("expected error for empty updated_by")
	}
}

// ---------------------------------------------------------------------------
// Delete — happy path
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Delete(t *testing.T) {
	adapter, _ := mustNewAdapter(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "del-cred", "generic", "val", "admin")

	if err := adapter.Delete(ctx, "del-cred"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify gone.
	_, err := adapter.Get(ctx, "del-cred")
	if !errors.Is(err, secrets.ErrCredentialNotFound) {
		t.Errorf("expected ErrCredentialNotFound after delete, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Delete — non-existent credential returns ErrCredentialNotFound
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Delete_NotFound(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	err := adapter.Delete(context.Background(), "no-such-cred")
	if !errors.Is(err, secrets.ErrCredentialNotFound) {
		t.Errorf("expected ErrCredentialNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Delete — credential in use returns ErrCredentialInUse
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Delete_InUse(t *testing.T) {
	adapter, rc := mustNewAdapter(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "in-use-cred", "generic", "val", "admin")
	rc.addRef("in-use-cred", "organisation", "my-org")

	err := adapter.Delete(ctx, "in-use-cred")
	if !errors.Is(err, secrets.ErrCredentialInUse) {
		t.Errorf("expected ErrCredentialInUse, got: %v", err)
	}

	// Verify the credential still exists.
	cred, err := adapter.Get(ctx, "in-use-cred")
	if err != nil {
		t.Fatalf("credential should still exist after failed delete: %v", err)
	}
	secrets.ZeroBytes(cred.Plaintext)
}

// ---------------------------------------------------------------------------
// Delete — nil refCheck skips reference check
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Delete_NilRefCheck(t *testing.T) {
	adapter := mustNewAdapterNoRefCheck(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "no-ref-check-cred", "generic", "val", "admin")

	// Should succeed even without a reference checker.
	if err := adapter.Delete(ctx, "no-ref-check-cred"); err != nil {
		t.Fatalf("Delete with nil refCheck: %v", err)
	}
}

// ---------------------------------------------------------------------------
// List — returns all credentials ordered by name
// ---------------------------------------------------------------------------

func TestCredentialAdapter_List(t *testing.T) {
	adapter, _ := mustNewAdapter(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "cred-b", "generic", "val-b", "admin")
	createTestCredential(t, adapter, "cred-a", "generic", "val-a", "admin")

	// Also set a non-credential config entry to verify it's excluded.
	if err := adapter.store.Set(ctx, "organisations", json.RawMessage(`[]`), false, "admin"); err != nil {
		t.Fatalf("Set non-credential: %v", err)
	}

	list, err := adapter.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(list))
	}

	// Verify no plaintext is exposed.
	for _, m := range list {
		if m.Name == "" {
			t.Error("credential Name should not be empty")
		}
		if m.CredentialType == "" {
			t.Error("credential CredentialType should not be empty")
		}
	}
}

// ---------------------------------------------------------------------------
// List — empty store returns empty slice
// ---------------------------------------------------------------------------

func TestCredentialAdapter_List_Empty(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	list, err := adapter.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if list != nil && len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}
}

// ---------------------------------------------------------------------------
// ListByType — filters by credential type
// ---------------------------------------------------------------------------

func TestCredentialAdapter_ListByType(t *testing.T) {
	adapter, _ := mustNewAdapter(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "generic-cred", "generic", "val", "admin")
	createTestCredential(t, adapter, "webhook-cred", "webhook_url", "https://hooks.example.com/test", "admin")

	genericList, err := adapter.ListByType(ctx, "generic")
	if err != nil {
		t.Fatalf("ListByType(generic): %v", err)
	}

	if len(genericList) != 1 {
		t.Fatalf("expected 1 generic credential, got %d", len(genericList))
	}
	if genericList[0].Name != "generic-cred" {
		t.Errorf("expected generic-cred, got %q", genericList[0].Name)
	}

	webhookList, err := adapter.ListByType(ctx, "webhook_url")
	if err != nil {
		t.Fatalf("ListByType(webhook_url): %v", err)
	}
	if len(webhookList) != 1 {
		t.Fatalf("expected 1 webhook credential, got %d", len(webhookList))
	}
	if webhookList[0].Name != "webhook-cred" {
		t.Errorf("expected webhook-cred, got %q", webhookList[0].Name)
	}
}

// ---------------------------------------------------------------------------
// ListByType — invalid type returns error
// ---------------------------------------------------------------------------

func TestCredentialAdapter_ListByType_InvalidType(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	_, err := adapter.ListByType(context.Background(), "bogus_type")
	if err == nil {
		t.Fatal("expected error for invalid credential type")
	}
	if !errors.Is(err, secrets.ErrInvalidCredentialType) {
		t.Errorf("expected ErrInvalidCredentialType, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test — validates stored credential
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Test(t *testing.T) {
	adapter, _ := mustNewAdapter(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "test-cred", "generic", "some-value", "admin")

	result, err := adapter.Test(ctx, "test-cred")
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.Valid {
		t.Errorf("expected valid result, got invalid: %v", result.Error)
	}
}

// ---------------------------------------------------------------------------
// Test — non-existent credential returns error
// ---------------------------------------------------------------------------

func TestCredentialAdapter_Test_NotFound(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	_, err := adapter.Test(context.Background(), "no-such-cred")
	if !errors.Is(err, secrets.ErrCredentialNotFound) {
		t.Errorf("expected ErrCredentialNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ReferencedBy — returns references
// ---------------------------------------------------------------------------

func TestCredentialAdapter_ReferencedBy(t *testing.T) {
	adapter, rc := mustNewAdapter(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "ref-cred", "generic", "val", "admin")
	rc.addRef("ref-cred", "organisation", "org-a")
	rc.addRef("ref-cred", "organisation", "org-b")

	refs, err := adapter.ReferencedBy(ctx, "ref-cred")
	if err != nil {
		t.Fatalf("ReferencedBy: %v", err)
	}

	if len(refs) != 2 {
		t.Fatalf("expected 2 references, got %d", len(refs))
	}
}

// ---------------------------------------------------------------------------
// ReferencedBy — non-existent credential returns ErrCredentialNotFound
// ---------------------------------------------------------------------------

func TestCredentialAdapter_ReferencedBy_NotFound(t *testing.T) {
	adapter, _ := mustNewAdapter(t)

	_, err := adapter.ReferencedBy(context.Background(), "no-such-cred")
	if !errors.Is(err, secrets.ErrCredentialNotFound) {
		t.Errorf("expected ErrCredentialNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ReferencedBy — nil refCheck returns nil
// ---------------------------------------------------------------------------

func TestCredentialAdapter_ReferencedBy_NilRefCheck(t *testing.T) {
	adapter := mustNewAdapterNoRefCheck(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "no-ref-cred", "generic", "val", "admin")

	refs, err := adapter.ReferencedBy(ctx, "no-ref-cred")
	if err != nil {
		t.Fatalf("ReferencedBy with nil refCheck: %v", err)
	}
	if refs != nil {
		t.Errorf("expected nil refs with nil refCheck, got %v", refs)
	}
}

// ---------------------------------------------------------------------------
// credentialKey / credentialNameFromKey — key mapping helpers
// ---------------------------------------------------------------------------

func TestCredentialKeyMapping(t *testing.T) {
	tests := []struct {
		name        string
		credName    string
		expectedKey string
	}{
		{"simple name", "my-cred", "credentials/my-cred"},
		{"name with dots", "chef.key", "credentials/chef.key"},
		{"name with dashes", "my-org-prod-key", "credentials/my-org-prod-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := credentialKey(tt.credName)
			if key != tt.expectedKey {
				t.Errorf("credentialKey(%q) = %q, want %q", tt.credName, key, tt.expectedKey)
			}

			got := credentialNameFromKey(key)
			if got != tt.credName {
				t.Errorf("credentialNameFromKey(%q) = %q, want %q", key, got, tt.credName)
			}
		})
	}
}

func TestCredentialNameFromKey_NoPrefix(t *testing.T) {
	got := credentialNameFromKey("organisations")
	if got != "" {
		t.Errorf("credentialNameFromKey(organisations) = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// Multiple credential types in same store
// ---------------------------------------------------------------------------

func TestCredentialAdapter_MultipleTypes(t *testing.T) {
	adapter, _ := mustNewAdapter(t)
	ctx := context.Background()

	createTestCredential(t, adapter, "generic-one", "generic", "val1", "admin")
	createTestCredential(t, adapter, "webhook-one", "webhook_url", "https://hooks.example.com/test", "admin")
	createTestCredential(t, adapter, "smtp-one", "smtp_password", "smtp-pass", "admin")

	list, err := adapter.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 credentials, got %d", len(list))
	}

	// Verify each can be individually retrieved.
	for _, name := range []string{"generic-one", "webhook-one", "smtp-one"} {
		cred, err := adapter.Get(ctx, name)
		if err != nil {
			t.Errorf("Get(%q): %v", name, err)
			continue
		}
		secrets.ZeroBytes(cred.Plaintext)
	}
}

// ---------------------------------------------------------------------------
// Create then Update then Get — full lifecycle
// ---------------------------------------------------------------------------

func TestCredentialAdapter_FullLifecycle(t *testing.T) {
	adapter, _ := mustNewAdapter(t)
	ctx := context.Background()

	// Create.
	createMeta := createTestCredential(t, adapter, "lifecycle-cred", "generic", "initial-val", "alice")
	if createMeta.Name != "lifecycle-cred" {
		t.Fatalf("Create name = %q", createMeta.Name)
	}

	// Get — verify initial value.
	cred, err := adapter.Get(ctx, "lifecycle-cred")
	if err != nil {
		t.Fatalf("Get after create: %v", err)
	}
	if string(cred.Plaintext) != "initial-val" {
		t.Errorf("initial Plaintext = %q, want %q", cred.Plaintext, "initial-val")
	}
	secrets.ZeroBytes(cred.Plaintext)

	// Update — rotate value.
	updateMeta, err := adapter.Update(ctx, secrets.UpdateCredentialInput{
		Name:      "lifecycle-cred",
		Plaintext: []byte("rotated-val"),
		UpdatedBy: "bob",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updateMeta.UpdatedBy != "bob" {
		t.Errorf("UpdatedBy = %q, want %q", updateMeta.UpdatedBy, "bob")
	}

	// Get — verify rotated value.
	cred2, err := adapter.Get(ctx, "lifecycle-cred")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if string(cred2.Plaintext) != "rotated-val" {
		t.Errorf("rotated Plaintext = %q, want %q", cred2.Plaintext, "rotated-val")
	}
	secrets.ZeroBytes(cred2.Plaintext)

	// GetMetadata — no plaintext.
	meta, err := adapter.GetMetadata(ctx, "lifecycle-cred")
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}
	if meta.CreatedBy != "alice" {
		t.Errorf("CreatedBy = %q, want %q (should be preserved)", meta.CreatedBy, "alice")
	}
	if meta.UpdatedBy != "bob" {
		t.Errorf("UpdatedBy = %q, want %q", meta.UpdatedBy, "bob")
	}

	// Test — should be valid.
	result, err := adapter.Test(ctx, "lifecycle-cred")
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.Valid {
		t.Errorf("Test result not valid: %v", result.Error)
	}

	// List — should have 1 entry.
	list, err := adapter.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List count = %d, want 1", len(list))
	}

	// Delete.
	if err := adapter.Delete(ctx, "lifecycle-cred"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify gone.
	_, err = adapter.Get(ctx, "lifecycle-cred")
	if !errors.Is(err, secrets.ErrCredentialNotFound) {
		t.Errorf("expected ErrCredentialNotFound after delete, got: %v", err)
	}

	// List should be empty.
	list, err = adapter.List(ctx)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List count after delete = %d, want 0", len(list))
	}
}

// ---------------------------------------------------------------------------
// Credentials are stored with secret=true and isolated from config entries
// ---------------------------------------------------------------------------

func TestCredentialAdapter_SecretIsolation(t *testing.T) {
	adapter, _ := mustNewAdapter(t)
	ctx := context.Background()

	// Create a credential.
	createTestCredential(t, adapter, "isolated-cred", "generic", "secret-val", "admin")

	// Also store a non-secret config entry directly.
	if err := adapter.store.Set(ctx, "organisations", json.RawMessage(`[]`), false, "admin"); err != nil {
		t.Fatalf("Set non-credential: %v", err)
	}

	// GetAll (non-secret) should NOT include the credential.
	all, err := adapter.store.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}

	if _, ok := all["credentials/isolated-cred"]; ok {
		t.Error("GetAll should not include secret credential entries")
	}
	if _, ok := all["organisations"]; !ok {
		t.Error("GetAll should include non-secret config entries")
	}
}

// ---------------------------------------------------------------------------
// Adapter with no encryption key — Get returns appropriate error
// ---------------------------------------------------------------------------

func TestCredentialAdapter_NoEncryptionKey(t *testing.T) {
	db := newFakeDB()
	store := &Store{db: db, derivedKey: nil} // nil key
	adapter := NewCredentialStoreAdapter(store, nil)

	// Create should fail because we can't encrypt.
	_, err := adapter.Create(context.Background(), secrets.CreateCredentialInput{
		Name:           "cred",
		CredentialType: "generic",
		Plaintext:      []byte("val"),
		CreatedBy:      "admin",
	})
	if err == nil {
		t.Fatal("expected error without encryption key")
	}
}
