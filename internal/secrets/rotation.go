package secrets

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RotationResult summarises the outcome of a master encryption key rotation.
type RotationResult struct {
	// TotalCredentials is the number of credential rows that were examined.
	TotalCredentials int

	// AlreadyRotated is the number of rows that were already encrypted
	// with the new key (decrypted successfully on first attempt).
	AlreadyRotated int

	// ReEncrypted is the number of rows that were decrypted with the
	// previous key and re-encrypted with the new key.
	ReEncrypted int

	// Failed is the number of rows that could not be decrypted with
	// either key. These credentials are marked unusable.
	Failed int

	// Errors contains one entry per failed credential, keyed by credential
	// name. Each value describes why the credential could not be rotated.
	Errors map[string]error

	// Duration is the wall-clock time the rotation took.
	Duration time.Duration
}

// RotationRow represents the minimal credential row data needed to perform
// key rotation. This is read from the database by the caller (typically the
// datastore package) and passed into the rotation logic so that the secrets
// package does not issue SQL directly during rotation.
type RotationRow struct {
	// Name is the credential's unique human-readable identifier.
	Name string

	// CredentialType is the credential's type (e.g. "chef_client_key").
	CredentialType string

	// EncryptedValue is the current at-rest ciphertext in
	// <nonce_hex>:<ciphertext_hex> format.
	EncryptedValue string
}

// RotatedRow is the output of a successful single-row rotation. It contains
// the new ciphertext that the caller must write back to the database (in its
// own transaction for crash-safety).
type RotatedRow struct {
	// Name identifies the credential that was re-encrypted.
	Name string

	// NewEncryptedValue is the ciphertext produced by the new key.
	NewEncryptedValue string

	// WasReEncrypted is true if the row was decrypted with the previous
	// key and re-encrypted with the new key. False if the row was already
	// encrypted with the new key (no write needed).
	WasReEncrypted bool
}

// Sentinel errors for rotation operations.
var (
	// ErrPreviousKeyRequired is returned when RotateMasterKey is called
	// without a previous-key encryptor.
	ErrPreviousKeyRequired = errors.New("secrets: previous encryption key is required for rotation")

	// ErrRotationDecryptFailed is returned when a credential row cannot
	// be decrypted with either the new or the previous master key.
	ErrRotationDecryptFailed = errors.New("secrets: credential could not be decrypted with either key")
)

// RotateCredentialRow attempts to re-encrypt a single credential row from
// the old master key to the new master key.
//
// The algorithm:
//  1. Build AAD from the row's credential_type and name.
//  2. Try to decrypt with newEncryptor (the row may already be rotated).
//  3. If that fails, try to decrypt with prevEncryptor (the old key).
//  4. If both fail, return ErrRotationDecryptFailed.
//  5. Re-encrypt the plaintext with newEncryptor and return the result.
//  6. Zero the plaintext before returning.
//
// The caller is responsible for persisting RotatedRow.NewEncryptedValue to
// the database in its own transaction.
func RotateCredentialRow(
	row RotationRow,
	newEncryptor *Encryptor,
	prevEncryptor *Encryptor,
) (*RotatedRow, error) {
	if newEncryptor == nil {
		return nil, ErrMasterKeyRequired
	}
	if prevEncryptor == nil {
		return nil, ErrPreviousKeyRequired
	}

	aad, err := BuildAAD(row.CredentialType, row.Name)
	if err != nil {
		return nil, fmt.Errorf("secrets: rotation failed for %q: %w", row.Name, err)
	}

	// Attempt 1: decrypt with the new key. If this succeeds the row is
	// already rotated and no write is needed.
	plaintext, errNew := newEncryptor.Decrypt(row.EncryptedValue, aad)
	if errNew == nil {
		// Already using the new key — no re-encryption required.
		ZeroBytes(plaintext)
		return &RotatedRow{
			Name:           row.Name,
			WasReEncrypted: false,
		}, nil
	}

	// Attempt 2: decrypt with the previous key.
	plaintext, errPrev := prevEncryptor.Decrypt(row.EncryptedValue, aad)
	if errPrev != nil {
		return nil, fmt.Errorf(
			"%w: %q (new key: %v; previous key: %v)",
			ErrRotationDecryptFailed, row.Name, errNew, errPrev,
		)
	}

	// Re-encrypt with the new key. Ensure plaintext is zeroed regardless
	// of whether encryption succeeds.
	newCiphertext, err := newEncryptor.Encrypt(plaintext, aad)
	ZeroBytes(plaintext)
	if err != nil {
		return nil, fmt.Errorf("secrets: re-encryption failed for %q: %w", row.Name, err)
	}

	return &RotatedRow{
		Name:              row.Name,
		NewEncryptedValue: newCiphertext,
		WasReEncrypted:    true,
	}, nil
}

// RotationRowWriter is a callback that the caller provides to
// RotateMasterKey. It is invoked once for each credential row that was
// re-encrypted with the new key. The implementation must persist the new
// encrypted value to the database in its own transaction.
//
// Returning an error from the writer causes that credential to be recorded
// as a failure in the RotationResult, but rotation continues for the
// remaining rows.
type RotationRowWriter func(ctx context.Context, row RotatedRow) error

// RotateMasterKey iterates over all credential rows and re-encrypts any
// that are still encrypted with the previous key.
//
//   - rows: all credential rows from the database (the caller reads these
//     before invoking rotation).
//   - newEncryptor: encryptor initialised with the new master key.
//   - prevEncryptor: encryptor initialised with the previous master key.
//   - writer: callback to persist each re-encrypted row (one transaction
//     per row for crash-safety).
//
// The function processes every row even if some fail, collecting failures
// into RotationResult.Errors. This allows the application to log per-
// credential errors and continue startup.
func RotateMasterKey(
	ctx context.Context,
	rows []RotationRow,
	newEncryptor *Encryptor,
	prevEncryptor *Encryptor,
	writer RotationRowWriter,
) (*RotationResult, error) {
	if newEncryptor == nil {
		return nil, ErrMasterKeyRequired
	}
	if prevEncryptor == nil {
		return nil, ErrPreviousKeyRequired
	}
	if writer == nil {
		return nil, fmt.Errorf("secrets: rotation row writer must not be nil")
	}

	start := time.Now()
	result := &RotationResult{
		TotalCredentials: len(rows),
		Errors:           make(map[string]error),
	}

	for _, row := range rows {
		// Check for context cancellation between rows.
		if err := ctx.Err(); err != nil {
			// Record remaining rows as failed.
			result.Failed += result.TotalCredentials - result.AlreadyRotated - result.ReEncrypted - result.Failed
			result.Errors["_context"] = err
			break
		}

		rotated, err := RotateCredentialRow(row, newEncryptor, prevEncryptor)
		if err != nil {
			result.Failed++
			result.Errors[row.Name] = err
			continue
		}

		if !rotated.WasReEncrypted {
			// Row already uses the new key — nothing to persist.
			result.AlreadyRotated++
			continue
		}

		// Persist the re-encrypted value via the caller-provided writer.
		if err := writer(ctx, *rotated); err != nil {
			result.Failed++
			result.Errors[row.Name] = fmt.Errorf("secrets: failed to persist re-encrypted credential %q: %w", row.Name, err)
			continue
		}

		result.ReEncrypted++
	}

	result.Duration = time.Since(start)
	return result, nil
}

// NeedsRotation returns true if the previous encryption key environment
// variable is set, indicating that a key rotation should be performed on
// startup.
func NeedsRotation(envLookup EnvLookupFunc) bool {
	if envLookup == nil {
		return false
	}
	val, ok := envLookup("CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS")
	return ok && val != ""
}
