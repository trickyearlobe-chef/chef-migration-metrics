// Package secrets provides credential encryption, storage, resolution, and
// rotation for the Chef Migration Metrics application.
//
// All database-stored credentials are encrypted at the application layer using
// AES-256-GCM with HKDF-SHA256 key derivation. This package is the only
// package that performs encryption/decryption operations — other packages
// obtain plaintext through the CredentialStore interface.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const (
	// masterKeyMinBytes is the minimum length of the master key after
	// Base64 decoding. AES-256 requires a 256-bit (32-byte) key.
	masterKeyMinBytes = 32

	// nonceSize is the byte length of the GCM nonce (IV). The Go
	// crypto/cipher GCM implementation uses a 12-byte nonce by default.
	nonceSize = 12

	// hkdfInfo is the context string used in HKDF expansion. It binds the
	// derived key to this application's credential encryption use case.
	hkdfInfo = "chef-migration-metrics-credential-encryption"

	// atRestSeparator is the delimiter between the hex-encoded nonce and
	// the hex-encoded ciphertext in the at-rest storage format.
	atRestSeparator = ":"
)

// Sentinel errors returned by encryption operations.
var (
	// ErrMasterKeyRequired is returned when an encryption or decryption
	// operation is attempted but no master key has been configured.
	ErrMasterKeyRequired = errors.New("secrets: master encryption key is required")

	// ErrMasterKeyTooShort is returned when the decoded master key is
	// shorter than masterKeyMinBytes.
	ErrMasterKeyTooShort = errors.New("secrets: master encryption key must be at least 32 bytes (256 bits)")

	// ErrMasterKeyInvalidBase64 is returned when the master key cannot be
	// decoded from Base64.
	ErrMasterKeyInvalidBase64 = errors.New("secrets: master encryption key is not valid Base64")

	// ErrInvalidCiphertext is returned when the stored ciphertext cannot
	// be parsed (missing separator, invalid hex, etc.).
	ErrInvalidCiphertext = errors.New("secrets: invalid ciphertext format")

	// ErrDecryptionFailed is returned when GCM decryption fails — either
	// the master key is wrong or the ciphertext has been tampered with.
	ErrDecryptionFailed = errors.New("secrets: decryption failed (wrong key or tampered data)")

	// ErrAADRequired is returned when the associated data (credential_type
	// and name) required for encryption/decryption is empty.
	ErrAADRequired = errors.New("secrets: associated data (credential type and name) is required")
)

// Encryptor performs AES-256-GCM encryption and decryption of credential
// values. It holds the derived encryption key in memory. Create one via
// NewEncryptor and discard it when no longer needed.
type Encryptor struct {
	derivedKey []byte
}

// NewEncryptor creates an Encryptor from a Base64-encoded master key string.
// The master key is decoded, validated for minimum length, and then used to
// derive a 256-bit encryption key via HKDF-SHA256.
//
// The caller should call Encryptor.Close when the encryptor is no longer
// needed to zero the derived key material from memory.
func NewEncryptor(masterKeyBase64 string) (*Encryptor, error) {
	if masterKeyBase64 == "" {
		return nil, ErrMasterKeyRequired
	}

	masterKey, err := base64.StdEncoding.DecodeString(masterKeyBase64)
	if err != nil {
		// Also try URL-safe and raw encodings for operator convenience.
		masterKey, err = base64.RawStdEncoding.DecodeString(masterKeyBase64)
		if err != nil {
			return nil, ErrMasterKeyInvalidBase64
		}
	}

	if len(masterKey) < masterKeyMinBytes {
		ZeroBytes(masterKey)
		return nil, ErrMasterKeyTooShort
	}

	// Derive a 32-byte encryption key using HKDF-SHA256.
	// Salt is nil — the master key is expected to be high-entropy random
	// material, so an HKDF extract step without salt is acceptable.
	hkdfReader := hkdf.New(sha256.New, masterKey, nil, []byte(hkdfInfo))
	derivedKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, derivedKey); err != nil {
		ZeroBytes(masterKey)
		return nil, fmt.Errorf("secrets: HKDF key derivation failed: %w", err)
	}

	// Zero the raw master key now that we have the derived key.
	ZeroBytes(masterKey)

	return &Encryptor{derivedKey: derivedKey}, nil
}

// Close zeros the derived key material held in memory. The Encryptor must
// not be used after Close is called.
func (e *Encryptor) Close() {
	if e != nil && e.derivedKey != nil {
		ZeroBytes(e.derivedKey)
		e.derivedKey = nil
	}
}

// BuildAAD constructs the associated authenticated data string from a
// credential type and name. The AAD is used during both encryption and
// decryption to bind the ciphertext to a specific credential identity,
// preventing row-swap attacks.
//
// Format: "<credential_type>:<name>"
func BuildAAD(credentialType, name string) ([]byte, error) {
	if credentialType == "" || name == "" {
		return nil, ErrAADRequired
	}
	return []byte(credentialType + ":" + name), nil
}

// Encrypt encrypts a plaintext credential value using AES-256-GCM with a
// random nonce and the provided associated data (AAD). The AAD should be
// constructed using BuildAAD.
//
// Returns the ciphertext in the at-rest format: "<nonce_hex>:<ciphertext_hex>".
// The ciphertext includes the GCM authentication tag appended by the Go
// cipher.AEAD.Seal method.
//
// The caller is responsible for zeroing the plaintext byte slice after this
// call returns.
func (e *Encryptor) Encrypt(plaintext, aad []byte) (string, error) {
	if e == nil || e.derivedKey == nil {
		return "", ErrMasterKeyRequired
	}

	block, err := aes.NewCipher(e.derivedKey)
	if err != nil {
		return "", fmt.Errorf("secrets: failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("secrets: failed to create GCM: %w", err)
	}

	// Generate a random 12-byte nonce.
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secrets: failed to generate nonce: %w", err)
	}

	// Seal encrypts and authenticates the plaintext, appending the GCM
	// tag to the ciphertext. The AAD is authenticated but not encrypted.
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)

	// Encode to the at-rest format: <nonce_hex>:<ciphertext_hex>
	encoded := hex.EncodeToString(nonce) + atRestSeparator + hex.EncodeToString(ciphertext)
	return encoded, nil
}

// Decrypt decrypts a credential value from the at-rest format using the
// provided associated data (AAD). The AAD must match what was used during
// encryption — if it doesn't, decryption will fail due to the GCM
// authentication tag mismatch.
//
// Returns the plaintext credential value. The caller is responsible for
// zeroing the returned byte slice when it is no longer needed.
func (e *Encryptor) Decrypt(encoded string, aad []byte) ([]byte, error) {
	if e == nil || e.derivedKey == nil {
		return nil, ErrMasterKeyRequired
	}

	// Parse the at-rest format: <nonce_hex>:<ciphertext_hex>
	parts := strings.SplitN(encoded, atRestSeparator, 2)
	if len(parts) != 2 {
		return nil, ErrInvalidCiphertext
	}

	nonce, err := hex.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid nonce hex: %v", ErrInvalidCiphertext, err)
	}

	if len(nonce) != nonceSize {
		return nil, fmt.Errorf("%w: nonce must be %d bytes, got %d", ErrInvalidCiphertext, nonceSize, len(nonce))
	}

	ciphertext, err := hex.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid ciphertext hex: %v", ErrInvalidCiphertext, err)
	}

	block, err := aes.NewCipher(e.derivedKey)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// DecodeMasterKey validates and decodes a Base64-encoded master key string
// without creating an Encryptor. This is useful for startup validation.
// Returns the decoded key bytes. The caller must zero the returned slice
// when done.
func DecodeMasterKey(masterKeyBase64 string) ([]byte, error) {
	if masterKeyBase64 == "" {
		return nil, ErrMasterKeyRequired
	}

	masterKey, err := base64.StdEncoding.DecodeString(masterKeyBase64)
	if err != nil {
		masterKey, err = base64.RawStdEncoding.DecodeString(masterKeyBase64)
		if err != nil {
			return nil, ErrMasterKeyInvalidBase64
		}
	}

	if len(masterKey) < masterKeyMinBytes {
		ZeroBytes(masterKey)
		return nil, ErrMasterKeyTooShort
	}

	return masterKey, nil
}
