// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

const (
	// credentialKeyPrefix is prepended to credential names to form the
	// config store key. Example: credential "my-key" → "credentials/my-key".
	credentialKeyPrefix = "credentials/"
)

// credentialEnvelope is the JSON structure stored in config_store for each
// credential. It captures all the metadata that the old credentials table
// had as separate columns.
type credentialEnvelope struct {
	CredentialType string         `json:"credential_type"`
	Value          []byte         `json:"value"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	LastRotatedAt  *time.Time     `json:"last_rotated_at,omitempty"`
	CreatedBy      string         `json:"created_by"`
	UpdatedBy      string         `json:"updated_by,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

// CredentialReferenceChecker provides the ability to check whether a
// credential is still referenced by other entities (organisations, auth
// providers, etc.). This is typically backed by database queries.
type CredentialReferenceChecker interface {
	// CheckCredentialReferences returns entities that reference the named
	// credential. Returns an empty slice if none reference it.
	CheckCredentialReferences(ctx context.Context, name string) ([]secrets.CredentialReference, error)
}

// CredentialStoreAdapter implements the secrets.CredentialStore interface
// using the encrypted config store as the backing storage. Credentials are
// stored as config entries with key "credentials/<name>" and secret=true.
//
// This adapter consolidates the old credentials table into the unified
// config_store table while preserving the full CredentialStore API contract.
type CredentialStoreAdapter struct {
	store    *Store
	refCheck CredentialReferenceChecker
}

// Compile-time check that CredentialStoreAdapter implements CredentialStore.
var _ secrets.CredentialStore = (*CredentialStoreAdapter)(nil)

// NewCredentialStoreAdapter creates a new adapter. The refCheck parameter
// provides credential reference checking (for delete safety). It may be nil
// if reference checking is not needed (e.g. during migration), in which
// case Delete will skip the reference check.
func NewCredentialStoreAdapter(store *Store, refCheck CredentialReferenceChecker) *CredentialStoreAdapter {
	return &CredentialStoreAdapter{
		store:    store,
		refCheck: refCheck,
	}
}

// credentialKey returns the config store key for a credential name.
func credentialKey(name string) string {
	return credentialKeyPrefix + name
}

// credentialNameFromKey extracts the credential name from a config store key.
// Returns empty string if the key does not have the credential prefix.
func credentialNameFromKey(key string) string {
	if !strings.HasPrefix(key, credentialKeyPrefix) {
		return ""
	}
	return key[len(credentialKeyPrefix):]
}

// Create validates, encrypts, and stores a new credential. Returns the
// metadata of the created credential (without plaintext).
func (a *CredentialStoreAdapter) Create(ctx context.Context, input secrets.CreateCredentialInput) (*secrets.CredentialMetadata, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("secrets: credential name is required")
	}
	if input.CreatedBy == "" {
		return nil, fmt.Errorf("secrets: created_by is required")
	}

	key := credentialKey(input.Name)

	// Check for duplicate.
	existing, err := a.store.GetEntry(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to check credential existence %q: %w", input.Name, err)
	}
	if existing != nil {
		return nil, secrets.ErrCredentialAlreadyExists
	}

	// Validate the credential value according to its type.
	result := secrets.ValidateCredentialValue(input.CredentialType, input.Plaintext)
	if !result.Valid {
		return nil, fmt.Errorf("secrets: validation failed: %w", result.Error)
	}

	now := time.Now().UTC()
	env := credentialEnvelope{
		CredentialType: input.CredentialType,
		Value:          input.Plaintext,
		Metadata:       result.Metadata,
		CreatedBy:      input.CreatedBy,
		CreatedAt:      now,
	}

	data, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to serialise credential envelope: %w", err)
	}

	if err := a.store.Set(ctx, key, json.RawMessage(data), true, input.CreatedBy); err != nil {
		return nil, fmt.Errorf("secrets: failed to store credential %q: %w", input.Name, err)
	}

	meta := &secrets.CredentialMetadata{
		Name:           input.Name,
		CredentialType: input.CredentialType,
		Metadata:       result.Metadata,
		CreatedBy:      input.CreatedBy,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	return meta, nil
}

// Get retrieves a credential by name and decrypts its value. The caller
// MUST zero the returned Credential.Plaintext after use.
func (a *CredentialStoreAdapter) Get(ctx context.Context, name string) (*secrets.Credential, error) {
	key := credentialKey(name)

	raw, err := a.store.GetSecret(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, secrets.ErrCredentialNotFound
		}
		if errors.Is(err, ErrEncryptionKeyRequired) {
			return nil, secrets.ErrEncryptionKeyNotConfigured
		}
		return nil, fmt.Errorf("secrets: failed to get credential %q: %w", name, err)
	}

	var env credentialEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("secrets: failed to deserialise credential %q: %w", name, err)
	}

	// Retrieve entry metadata for timestamps.
	entry, err := a.store.GetEntry(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to get entry metadata for %q: %w", name, err)
	}

	cred := &secrets.Credential{
		Name:           name,
		CredentialType: env.CredentialType,
		Plaintext:      env.Value,
		Metadata:       env.Metadata,
		LastRotatedAt:  env.LastRotatedAt,
		CreatedBy:      env.CreatedBy,
		UpdatedBy:      env.UpdatedBy,
		CreatedAt:      env.CreatedAt,
	}

	if entry != nil {
		cred.UpdatedAt = entry.UpdatedAt
	}

	return cred, nil
}

// GetMetadata retrieves a credential's metadata by name without decrypting
// the stored value. This is suitable for detail views where the plaintext
// is not needed.
func (a *CredentialStoreAdapter) GetMetadata(ctx context.Context, name string) (*secrets.CredentialMetadata, error) {
	key := credentialKey(name)

	// We need to decrypt to get the envelope metadata (credential_type, etc.)
	// but we won't expose the plaintext value.
	raw, err := a.store.GetSecret(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, secrets.ErrCredentialNotFound
		}
		if errors.Is(err, ErrEncryptionKeyRequired) {
			return nil, secrets.ErrEncryptionKeyNotConfigured
		}
		return nil, fmt.Errorf("secrets: failed to get credential metadata %q: %w", name, err)
	}

	var env credentialEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("secrets: failed to deserialise credential %q: %w", name, err)
	}

	// Zero the plaintext that was inside the envelope.
	secrets.ZeroBytes(env.Value)

	entry, err := a.store.GetEntry(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to get entry metadata for %q: %w", name, err)
	}

	meta := &secrets.CredentialMetadata{
		Name:           name,
		CredentialType: env.CredentialType,
		Metadata:       env.Metadata,
		LastRotatedAt:  env.LastRotatedAt,
		CreatedBy:      env.CreatedBy,
		UpdatedBy:      env.UpdatedBy,
		CreatedAt:      env.CreatedAt,
	}

	if entry != nil {
		meta.UpdatedAt = entry.UpdatedAt
	}

	return meta, nil
}

// Update validates and re-encrypts a credential's value. This is used for
// credential value rotation. The last_rotated_at timestamp is updated.
func (a *CredentialStoreAdapter) Update(ctx context.Context, input secrets.UpdateCredentialInput) (*secrets.CredentialMetadata, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("secrets: credential name is required")
	}
	if input.UpdatedBy == "" {
		return nil, fmt.Errorf("secrets: updated_by is required")
	}

	key := credentialKey(input.Name)

	// Read the existing credential to get its type and metadata.
	raw, err := a.store.GetSecret(ctx, key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, secrets.ErrCredentialNotFound
		}
		if errors.Is(err, ErrEncryptionKeyRequired) {
			return nil, secrets.ErrEncryptionKeyNotConfigured
		}
		return nil, fmt.Errorf("secrets: failed to read existing credential %q for update: %w", input.Name, err)
	}

	var env credentialEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("secrets: failed to deserialise existing credential %q: %w", input.Name, err)
	}

	// Zero old plaintext.
	secrets.ZeroBytes(env.Value)

	// Validate the new value against the existing type.
	result := secrets.ValidateCredentialValue(env.CredentialType, input.Plaintext)
	if !result.Valid {
		return nil, fmt.Errorf("secrets: validation failed: %w", result.Error)
	}

	now := time.Now().UTC()
	env.Value = input.Plaintext
	env.Metadata = result.Metadata
	env.LastRotatedAt = &now
	env.UpdatedBy = input.UpdatedBy

	data, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to serialise updated credential envelope: %w", err)
	}

	if err := a.store.Set(ctx, key, json.RawMessage(data), true, input.UpdatedBy); err != nil {
		return nil, fmt.Errorf("secrets: failed to store updated credential %q: %w", input.Name, err)
	}

	meta := &secrets.CredentialMetadata{
		Name:           input.Name,
		CredentialType: env.CredentialType,
		Metadata:       result.Metadata,
		LastRotatedAt:  env.LastRotatedAt,
		CreatedBy:      env.CreatedBy,
		UpdatedBy:      env.UpdatedBy,
		CreatedAt:      env.CreatedAt,
		UpdatedAt:      now,
	}

	return meta, nil
}

// Delete removes a credential by name. Returns ErrCredentialInUse if the
// credential is still referenced by one or more entities.
func (a *CredentialStoreAdapter) Delete(ctx context.Context, name string) error {
	key := credentialKey(name)

	// Verify the credential exists.
	entry, err := a.store.GetEntry(ctx, key)
	if err != nil {
		return fmt.Errorf("secrets: failed to check credential existence %q: %w", name, err)
	}
	if entry == nil {
		return secrets.ErrCredentialNotFound
	}

	// Check for references before deleting (if checker is available).
	if a.refCheck != nil {
		refs, err := a.refCheck.CheckCredentialReferences(ctx, name)
		if err != nil {
			return fmt.Errorf("secrets: failed to check references for credential %q: %w", name, err)
		}
		if len(refs) > 0 {
			return secrets.ErrCredentialInUse
		}
	}

	if err := a.store.Delete(ctx, key); err != nil {
		return fmt.Errorf("secrets: failed to delete credential %q: %w", name, err)
	}

	return nil
}

// List returns metadata for all credentials, ordered by name. The result
// never includes plaintext or encrypted values.
func (a *CredentialStoreAdapter) List(ctx context.Context) ([]secrets.CredentialMetadata, error) {
	return a.listWithFilter(ctx, "")
}

// ListByType returns metadata for all credentials of the given type,
// ordered by name.
func (a *CredentialStoreAdapter) ListByType(ctx context.Context, credentialType string) ([]secrets.CredentialMetadata, error) {
	if !secrets.IsValidCredentialType(credentialType) {
		return nil, fmt.Errorf("%w: %q", secrets.ErrInvalidCredentialType, credentialType)
	}

	return a.listWithFilter(ctx, credentialType)
}

// Test decrypts a credential and performs type-specific validation to verify
// the credential is still functional. The plaintext is zeroed before returning.
func (a *CredentialStoreAdapter) Test(ctx context.Context, name string) (*secrets.ValidationResult, error) {
	cred, err := a.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	defer secrets.ZeroBytes(cred.Plaintext)

	result := secrets.ValidateCredentialValue(cred.CredentialType, cred.Plaintext)
	return &result, nil
}

// ReferencedBy returns a list of entities that reference the named
// credential.
func (a *CredentialStoreAdapter) ReferencedBy(ctx context.Context, name string) ([]secrets.CredentialReference, error) {
	// Verify the credential exists.
	key := credentialKey(name)
	entry, err := a.store.GetEntry(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to check credential existence %q: %w", name, err)
	}
	if entry == nil {
		return nil, secrets.ErrCredentialNotFound
	}

	if a.refCheck == nil {
		return nil, nil
	}

	return a.refCheck.CheckCredentialReferences(ctx, name)
}

// listWithFilter retrieves all credential entries from the config store,
// decrypts their envelopes to extract metadata, and optionally filters by
// credential type. The plaintext values are zeroed after metadata extraction.
func (a *CredentialStoreAdapter) listWithFilter(ctx context.Context, filterType string) ([]secrets.CredentialMetadata, error) {
	entries, err := a.store.ListByPrefix(ctx, credentialKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to list credentials: %w", err)
	}

	var result []secrets.CredentialMetadata
	for _, e := range entries {
		name := credentialNameFromKey(e.Key)
		if name == "" {
			continue
		}

		// Decrypt to read the envelope metadata.
		raw, decErr := a.store.GetSecret(ctx, e.Key)
		if decErr != nil {
			// If we can't decrypt an individual credential during list,
			// include it with minimal metadata rather than failing the
			// entire list.
			result = append(result, secrets.CredentialMetadata{
				Name:      name,
				UpdatedAt: e.UpdatedAt,
				UpdatedBy: e.UpdatedBy,
			})
			continue
		}

		var env credentialEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			result = append(result, secrets.CredentialMetadata{
				Name:      name,
				UpdatedAt: e.UpdatedAt,
				UpdatedBy: e.UpdatedBy,
			})
			continue
		}

		// Zero the plaintext from the envelope.
		secrets.ZeroBytes(env.Value)

		// Apply type filter if specified.
		if filterType != "" && env.CredentialType != filterType {
			continue
		}

		meta := secrets.CredentialMetadata{
			Name:           name,
			CredentialType: env.CredentialType,
			Metadata:       env.Metadata,
			LastRotatedAt:  env.LastRotatedAt,
			CreatedBy:      env.CreatedBy,
			UpdatedBy:      env.UpdatedBy,
			CreatedAt:      env.CreatedAt,
			UpdatedAt:      e.UpdatedAt,
		}

		result = append(result, meta)
	}

	return result, nil
}
