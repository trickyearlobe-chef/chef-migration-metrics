// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package configstore provides an encrypted configuration store backed by the
// config_store database table. All values are AES-256-GCM encrypted with a
// per-row nonce and AAD bound to the key name.
//
// This package sits above the raw datastore CRUD layer and below the API
// handlers. It handles encryption/decryption, JSON serialisation, and the
// secret flag semantics. The datastore layer handles only raw byte storage.
package configstore

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

const (
	// nonceSize is the byte length of the GCM nonce (IV). The Go
	// crypto/cipher GCM implementation uses a 12-byte nonce by default.
	nonceSize = 12
)

// Sentinel errors returned by Store operations.
var (
	// ErrNotFound is returned when a config store key does not exist.
	ErrNotFound = errors.New("configstore: key not found")

	// ErrNotSecret is returned when GetSecret is called for a key that
	// exists but does not have the secret flag set.
	ErrNotSecret = errors.New("configstore: key is not marked as secret")

	// ErrEncryptionKeyRequired is returned when an operation that requires
	// encryption or decryption is attempted but no Encryptor is available.
	ErrEncryptionKeyRequired = errors.New("configstore: encryption key is required")

	// ErrDecryptionFailed is returned when GCM decryption fails — either
	// the master key is wrong or the ciphertext has been tampered with.
	ErrDecryptionFailed = errors.New("configstore: decryption failed (wrong key or tampered data)")
)

// EntryMetadata contains non-sensitive metadata about a config store entry.
// The decrypted value is never included.
type EntryMetadata struct {
	Key       string    `json:"key"`
	Secret    bool      `json:"secret"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

// DatastoreDB abstracts the datastore methods required by Store. This
// interface is satisfied by *datastore.DB and allows testing with fakes.
type DatastoreDB interface {
	GetConfigEntry(ctx context.Context, key string) (*datastore.ConfigEntry, error)
	SetConfigEntry(ctx context.Context, e *datastore.ConfigEntry) error
	DeleteConfigEntry(ctx context.Context, key string) error
	ListConfigEntries(ctx context.Context) ([]datastore.ConfigEntry, error)
	ListConfigEntriesByPrefix(ctx context.Context, prefix string) ([]datastore.ConfigEntry, error)
	CountConfigEntries(ctx context.Context) (int, error)
	ConfigStoreIsEmpty(ctx context.Context) (bool, error)
}

// Store provides encrypted read/write access to the config_store table. All
// values are AES-256-GCM encrypted using a key derived from the master
// encryption key via HKDF (handled by the secrets.Encryptor).
//
// Store is safe for concurrent use — it holds no mutable state. Concurrency
// control is handled by the underlying database connection pool.
type Store struct {
	db         DatastoreDB
	derivedKey []byte
}

// NewStore creates a Store. The encryptor is used to obtain the derived
// encryption key. The encryptor may be nil if the caller only needs
// metadata operations (List, IsEmpty) — operations requiring encryption
// or decryption will return ErrEncryptionKeyRequired.
func NewStore(db DatastoreDB, encryptor *secrets.Encryptor) *Store {
	s := &Store{db: db}
	if encryptor != nil {
		// We need the raw derived key for direct AES-256-GCM operations
		// with separate nonce storage (not the hex-encoded string format
		// used by Encryptor.Encrypt). Extract it via the DerivedKey accessor.
		s.derivedKey = encryptor.DerivedKey()
	}
	return s
}

// NewStoreWithKey creates a Store using a raw 32-byte derived encryption key
// directly. This is primarily for testing — production code should use
// NewStore with an Encryptor. The key must be exactly 32 bytes (AES-256).
func NewStoreWithKey(db DatastoreDB, derivedKey []byte) (*Store, error) {
	if len(derivedKey) != 32 {
		return nil, fmt.Errorf("configstore: derived key must be 32 bytes, got %d", len(derivedKey))
	}
	return &Store{db: db, derivedKey: derivedKey}, nil
}

// Get retrieves a config entry by key, decrypts it, and returns the JSON
// value. Returns ErrNotFound if the key does not exist.
func (s *Store) Get(ctx context.Context, key string) (json.RawMessage, error) {
	entry, err := s.db.GetConfigEntry(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("configstore: get %q: %w", key, err)
	}
	if entry == nil {
		return nil, ErrNotFound
	}

	plaintext, err := s.decrypt(entry.EncryptedValue, entry.Nonce, key)
	if err != nil {
		return nil, fmt.Errorf("configstore: decrypt %q: %w", key, err)
	}

	return json.RawMessage(plaintext), nil
}

// GetSecret retrieves a config entry that has the secret flag set. Returns
// ErrNotFound if the key does not exist and ErrNotSecret if the key exists
// but is not marked as secret.
func (s *Store) GetSecret(ctx context.Context, key string) (json.RawMessage, error) {
	entry, err := s.db.GetConfigEntry(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("configstore: get secret %q: %w", key, err)
	}
	if entry == nil {
		return nil, ErrNotFound
	}
	if !entry.Secret {
		return nil, ErrNotSecret
	}

	plaintext, err := s.decrypt(entry.EncryptedValue, entry.Nonce, key)
	if err != nil {
		return nil, fmt.Errorf("configstore: decrypt secret %q: %w", key, err)
	}

	return json.RawMessage(plaintext), nil
}

// Set encrypts a JSON value and stores it in the config store. A fresh
// nonce is generated for each write. If the key already exists, it is
// overwritten (upsert).
func (s *Store) Set(ctx context.Context, key string, value json.RawMessage, secret bool, updatedBy string) error {
	ciphertext, nonce, err := s.encrypt([]byte(value), key)
	if err != nil {
		return fmt.Errorf("configstore: encrypt %q: %w", key, err)
	}

	entry := &datastore.ConfigEntry{
		Key:            key,
		EncryptedValue: ciphertext,
		Nonce:          nonce,
		Secret:         secret,
		UpdatedBy:      updatedBy,
	}

	if err := s.db.SetConfigEntry(ctx, entry); err != nil {
		return fmt.Errorf("configstore: set %q: %w", key, err)
	}
	return nil
}

// Delete removes a config entry by key. Returns nil even if the key did
// not exist (idempotent).
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.db.DeleteConfigEntry(ctx, key); err != nil {
		return fmt.Errorf("configstore: delete %q: %w", key, err)
	}
	return nil
}

// List returns metadata for all config store entries, ordered by key.
// Decrypted values are never included.
func (s *Store) List(ctx context.Context) ([]EntryMetadata, error) {
	entries, err := s.db.ListConfigEntries(ctx)
	if err != nil {
		return nil, fmt.Errorf("configstore: list: %w", err)
	}
	return toMetadata(entries), nil
}

// ListByPrefix returns metadata for config store entries whose key starts
// with the given prefix, ordered by key. Decrypted values are never included.
func (s *Store) ListByPrefix(ctx context.Context, prefix string) ([]EntryMetadata, error) {
	entries, err := s.db.ListConfigEntriesByPrefix(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("configstore: list by prefix %q: %w", prefix, err)
	}
	return toMetadata(entries), nil
}

// GetAll decrypts all non-secret config entries and returns them as a map
// of key → JSON value. Secret entries are excluded. This is used during
// config assembly to build the in-memory Config struct from the database.
func (s *Store) GetAll(ctx context.Context) (map[string]json.RawMessage, error) {
	entries, err := s.db.ListConfigEntries(ctx)
	if err != nil {
		return nil, fmt.Errorf("configstore: get all: %w", err)
	}

	result := make(map[string]json.RawMessage, len(entries))
	for _, e := range entries {
		if e.Secret {
			continue
		}
		plaintext, decErr := s.decrypt(e.EncryptedValue, e.Nonce, e.Key)
		if decErr != nil {
			return nil, fmt.Errorf("configstore: decrypt %q during get all: %w", e.Key, decErr)
		}
		result[e.Key] = json.RawMessage(plaintext)
	}
	return result, nil
}

// GetEntry retrieves the raw datastore entry for a key. This is a lower-level
// method used by the credential adapter when it needs access to the secret
// flag and timestamps without decrypting. Returns nil, nil when the key does
// not exist.
func (s *Store) GetEntry(ctx context.Context, key string) (*datastore.ConfigEntry, error) {
	return s.db.GetConfigEntry(ctx, key)
}

// IsEmpty returns true when the config_store table contains zero rows.
func (s *Store) IsEmpty(ctx context.Context) (bool, error) {
	return s.db.ConfigStoreIsEmpty(ctx)
}

// Count returns the total number of entries in the config store.
func (s *Store) Count(ctx context.Context) (int, error) {
	return s.db.CountConfigEntries(ctx)
}

// encrypt performs AES-256-GCM encryption with a randomly generated nonce.
// The AAD is the config store key name, binding the ciphertext to its row.
// Returns the ciphertext (including GCM tag) and the nonce.
func (s *Store) encrypt(plaintext []byte, key string) (ciphertext, nonce []byte, err error) {
	if s.derivedKey == nil {
		return nil, nil, ErrEncryptionKeyRequired
	}

	block, err := aes.NewCipher(s.derivedKey)
	if err != nil {
		return nil, nil, fmt.Errorf("aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("gcm: %w", err)
	}

	nonce = make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}

	aad := []byte(key)
	ciphertext = gcm.Seal(nil, nonce, plaintext, aad)

	return ciphertext, nonce, nil
}

// decrypt performs AES-256-GCM decryption. The AAD is reconstructed from the
// config store key name. If the key name doesn't match what was used during
// encryption, decryption fails (GCM tag mismatch).
func (s *Store) decrypt(ciphertext, nonce []byte, key string) ([]byte, error) {
	if s.derivedKey == nil {
		return nil, ErrEncryptionKeyRequired
	}

	block, err := aes.NewCipher(s.derivedKey)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}

	aad := []byte(key)
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// toMetadata converts a slice of datastore.ConfigEntry to EntryMetadata,
// stripping encrypted values.
func toMetadata(entries []datastore.ConfigEntry) []EntryMetadata {
	result := make([]EntryMetadata, len(entries))
	for i, e := range entries {
		result[i] = EntryMetadata{
			Key:       e.Key,
			Secret:    e.Secret,
			UpdatedAt: e.UpdatedAt,
			UpdatedBy: e.UpdatedBy,
		}
	}
	return result
}
