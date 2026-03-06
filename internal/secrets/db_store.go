package secrets

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// DB abstracts the database operations needed by DBCredentialStore. This
// interface is satisfied by *sql.DB, *sql.Tx, and test fakes. Keeping
// database/sql types out of the CredentialStore interface allows the
// secrets package to be tested without a live database.
type DB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// DBCredentialStore is a database-backed implementation of CredentialStore.
// It encrypts credential values before writing them to the credentials table
// and decrypts them when reading. All encryption operations use the
// Encryptor provided at construction time.
type DBCredentialStore struct {
	db        DB
	encryptor *Encryptor
}

// Compile-time check that DBCredentialStore implements CredentialStore.
var _ CredentialStore = (*DBCredentialStore)(nil)

// NewDBCredentialStore creates a new DBCredentialStore. The encryptor may be
// nil if no master encryption key is configured — operations that require
// encryption or decryption will return ErrEncryptionKeyNotConfigured.
func NewDBCredentialStore(db DB, encryptor *Encryptor) *DBCredentialStore {
	return &DBCredentialStore{
		db:        db,
		encryptor: encryptor,
	}
}

// Create validates, encrypts, and inserts a new credential row. Returns the
// metadata of the created credential (without plaintext).
func (s *DBCredentialStore) Create(ctx context.Context, input CreateCredentialInput) (*CredentialMetadata, error) {
	if s.encryptor == nil {
		return nil, ErrEncryptionKeyNotConfigured
	}

	if input.Name == "" {
		return nil, fmt.Errorf("secrets: credential name is required")
	}
	if input.CreatedBy == "" {
		return nil, fmt.Errorf("secrets: created_by is required")
	}

	// Validate the credential value according to its type.
	result := ValidateCredentialValue(input.CredentialType, input.Plaintext)
	if !result.Valid {
		return nil, fmt.Errorf("secrets: validation failed: %w", result.Error)
	}

	// Build AAD from credential type and name.
	aad, err := BuildAAD(input.CredentialType, input.Name)
	if err != nil {
		return nil, err
	}

	// Encrypt the plaintext.
	encrypted, err := s.encryptor.Encrypt(input.Plaintext, aad)
	if err != nil {
		return nil, fmt.Errorf("secrets: encryption failed: %w", err)
	}

	// Serialise metadata to JSONB (may be nil).
	var metadataJSON []byte
	if result.Metadata != nil {
		metadataJSON, err = json.Marshal(result.Metadata)
		if err != nil {
			return nil, fmt.Errorf("secrets: failed to serialise metadata: %w", err)
		}
	}

	var (
		name           string
		credentialType string
		metadata       []byte
		createdBy      string
		createdAt      time.Time
		updatedAt      time.Time
	)

	query := `
		INSERT INTO credentials (name, credential_type, encrypted_value, metadata, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING name, credential_type, metadata, created_by, created_at, updated_at`

	row := s.db.QueryRowContext(ctx, query,
		input.Name,
		input.CredentialType,
		encrypted,
		nullableJSONB(metadataJSON),
		input.CreatedBy,
	)

	err = row.Scan(&name, &credentialType, &metadata, &createdBy, &createdAt, &updatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrCredentialAlreadyExists
		}
		return nil, fmt.Errorf("secrets: failed to insert credential: %w", err)
	}

	meta := &CredentialMetadata{
		Name:           name,
		CredentialType: credentialType,
		Metadata:       parseJSONBMetadata(metadata),
		CreatedBy:      createdBy,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}

	return meta, nil
}

// Get retrieves a credential by name and decrypts its value. The caller
// MUST zero the returned Credential.Plaintext after use.
func (s *DBCredentialStore) Get(ctx context.Context, name string) (*Credential, error) {
	if s.encryptor == nil {
		return nil, ErrEncryptionKeyNotConfigured
	}

	query := `
		SELECT name, credential_type, encrypted_value, metadata,
		       last_rotated_at, created_by, updated_by, created_at, updated_at
		FROM credentials
		WHERE name = $1`

	var (
		credName       string
		credType       string
		encryptedValue string
		metadata       []byte
		lastRotatedAt  sql.NullTime
		createdBy      string
		updatedBy      sql.NullString
		createdAt      time.Time
		updatedAt      time.Time
	)

	err := s.db.QueryRowContext(ctx, query, name).Scan(
		&credName, &credType, &encryptedValue, &metadata,
		&lastRotatedAt, &createdBy, &updatedBy, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCredentialNotFound
		}
		return nil, fmt.Errorf("secrets: failed to query credential %q: %w", name, err)
	}

	// Build AAD and decrypt.
	aad, err := BuildAAD(credType, credName)
	if err != nil {
		return nil, err
	}

	plaintext, err := s.encryptor.Decrypt(encryptedValue, aad)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to decrypt credential %q: %w", name, err)
	}

	cred := &Credential{
		Name:           credName,
		CredentialType: credType,
		Plaintext:      plaintext,
		Metadata:       parseJSONBMetadata(metadata),
		CreatedBy:      createdBy,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}

	if lastRotatedAt.Valid {
		t := lastRotatedAt.Time
		cred.LastRotatedAt = &t
	}
	if updatedBy.Valid {
		cred.UpdatedBy = updatedBy.String
	}

	return cred, nil
}

// GetMetadata retrieves a credential's metadata by name without decrypting
// the stored value.
func (s *DBCredentialStore) GetMetadata(ctx context.Context, name string) (*CredentialMetadata, error) {
	query := `
		SELECT name, credential_type, metadata,
		       last_rotated_at, created_by, updated_by, created_at, updated_at
		FROM credentials
		WHERE name = $1`

	var (
		credName      string
		credType      string
		metadata      []byte
		lastRotatedAt sql.NullTime
		createdBy     string
		updatedBy     sql.NullString
		createdAt     time.Time
		updatedAt     time.Time
	)

	err := s.db.QueryRowContext(ctx, query, name).Scan(
		&credName, &credType, &metadata,
		&lastRotatedAt, &createdBy, &updatedBy, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCredentialNotFound
		}
		return nil, fmt.Errorf("secrets: failed to query credential metadata %q: %w", name, err)
	}

	meta := &CredentialMetadata{
		Name:           credName,
		CredentialType: credType,
		Metadata:       parseJSONBMetadata(metadata),
		CreatedBy:      createdBy,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}

	if lastRotatedAt.Valid {
		t := lastRotatedAt.Time
		meta.LastRotatedAt = &t
	}
	if updatedBy.Valid {
		meta.UpdatedBy = updatedBy.String
	}

	return meta, nil
}

// Update validates a new plaintext value, re-encrypts it, and overwrites
// the existing credential row. The last_rotated_at timestamp is set to now.
func (s *DBCredentialStore) Update(ctx context.Context, input UpdateCredentialInput) (*CredentialMetadata, error) {
	if s.encryptor == nil {
		return nil, ErrEncryptionKeyNotConfigured
	}

	if input.Name == "" {
		return nil, fmt.Errorf("secrets: credential name is required")
	}
	if input.UpdatedBy == "" {
		return nil, fmt.Errorf("secrets: updated_by is required")
	}

	// Read the existing credential type so we can validate and build AAD.
	var credType string
	err := s.db.QueryRowContext(ctx,
		`SELECT credential_type FROM credentials WHERE name = $1`,
		input.Name,
	).Scan(&credType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCredentialNotFound
		}
		return nil, fmt.Errorf("secrets: failed to look up credential %q for update: %w", input.Name, err)
	}

	// Validate the new value according to the credential's type.
	result := ValidateCredentialValue(credType, input.Plaintext)
	if !result.Valid {
		return nil, fmt.Errorf("secrets: validation failed: %w", result.Error)
	}

	// Build AAD and encrypt the new value.
	aad, err := BuildAAD(credType, input.Name)
	if err != nil {
		return nil, err
	}

	encrypted, err := s.encryptor.Encrypt(input.Plaintext, aad)
	if err != nil {
		return nil, fmt.Errorf("secrets: encryption failed: %w", err)
	}

	// Serialise metadata to JSONB.
	var metadataJSON []byte
	if result.Metadata != nil {
		metadataJSON, err = json.Marshal(result.Metadata)
		if err != nil {
			return nil, fmt.Errorf("secrets: failed to serialise metadata: %w", err)
		}
	}

	query := `
		UPDATE credentials
		SET encrypted_value = $1,
		    metadata = $2,
		    last_rotated_at = now(),
		    updated_by = $3,
		    updated_at = now()
		WHERE name = $4
		RETURNING name, credential_type, metadata,
		          last_rotated_at, created_by, updated_by, created_at, updated_at`

	var (
		retName       string
		retType       string
		retMeta       []byte
		lastRotatedAt sql.NullTime
		createdBy     string
		retUpdatedBy  sql.NullString
		createdAt     time.Time
		updatedAt     time.Time
	)

	err = s.db.QueryRowContext(ctx, query,
		encrypted,
		nullableJSONB(metadataJSON),
		input.UpdatedBy,
		input.Name,
	).Scan(
		&retName, &retType, &retMeta,
		&lastRotatedAt, &createdBy, &retUpdatedBy, &createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCredentialNotFound
		}
		return nil, fmt.Errorf("secrets: failed to update credential %q: %w", input.Name, err)
	}

	meta := &CredentialMetadata{
		Name:           retName,
		CredentialType: retType,
		Metadata:       parseJSONBMetadata(retMeta),
		CreatedBy:      createdBy,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}

	if lastRotatedAt.Valid {
		t := lastRotatedAt.Time
		meta.LastRotatedAt = &t
	}
	if retUpdatedBy.Valid {
		meta.UpdatedBy = retUpdatedBy.String
	}

	return meta, nil
}

// Delete removes a credential by name. Returns ErrCredentialInUse if the
// credential is still referenced by one or more entities.
func (s *DBCredentialStore) Delete(ctx context.Context, name string) error {
	// First verify the credential exists.
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM credentials WHERE name = $1)`,
		name,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("secrets: failed to check credential existence %q: %w", name, err)
	}
	if !exists {
		return ErrCredentialNotFound
	}

	// Check for references before deleting.
	refs, err := s.referencedByInternal(ctx, name)
	if err != nil {
		return fmt.Errorf("secrets: failed to check references for credential %q: %w", name, err)
	}
	if len(refs) > 0 {
		return ErrCredentialInUse
	}

	result, err := s.db.ExecContext(ctx,
		`DELETE FROM credentials WHERE name = $1`,
		name,
	)
	if err != nil {
		return fmt.Errorf("secrets: failed to delete credential %q: %w", name, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("secrets: failed to check delete result for %q: %w", name, err)
	}
	if rowsAffected == 0 {
		return ErrCredentialNotFound
	}

	return nil
}

// List returns metadata for all credentials, ordered by name.
func (s *DBCredentialStore) List(ctx context.Context) ([]CredentialMetadata, error) {
	query := `
		SELECT name, credential_type, metadata,
		       last_rotated_at, created_by, updated_by, created_at, updated_at
		FROM credentials
		ORDER BY name`

	return s.queryMetadataRows(ctx, query)
}

// ListByType returns metadata for all credentials of the given type, ordered
// by name.
func (s *DBCredentialStore) ListByType(ctx context.Context, credentialType string) ([]CredentialMetadata, error) {
	if !IsValidCredentialType(credentialType) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidCredentialType, credentialType)
	}

	query := `
		SELECT name, credential_type, metadata,
		       last_rotated_at, created_by, updated_by, created_at, updated_at
		FROM credentials
		WHERE credential_type = $1
		ORDER BY name`

	return s.queryMetadataRows(ctx, query, credentialType)
}

// Test decrypts a credential and performs type-specific validation to verify
// the credential is still functional. The decrypted plaintext is zeroed
// before returning.
func (s *DBCredentialStore) Test(ctx context.Context, name string) (*ValidationResult, error) {
	cred, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(cred.Plaintext)

	result := ValidateCredentialValue(cred.CredentialType, cred.Plaintext)
	return &result, nil
}

// ReferencedBy returns a list of entities that reference the named
// credential. Returns ErrCredentialNotFound if the credential does not exist.
func (s *DBCredentialStore) ReferencedBy(ctx context.Context, name string) ([]CredentialReference, error) {
	// Verify the credential exists.
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM credentials WHERE name = $1)`,
		name,
	).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to check credential existence %q: %w", name, err)
	}
	if !exists {
		return nil, ErrCredentialNotFound
	}

	return s.referencedByInternal(ctx, name)
}

// referencedByInternal returns references without checking credential
// existence. Used internally by both ReferencedBy and Delete.
func (s *DBCredentialStore) referencedByInternal(ctx context.Context, name string) ([]CredentialReference, error) {
	// Check organisations that reference this credential via FK.
	query := `
		SELECT o.name
		FROM organisations o
		JOIN credentials c ON o.client_key_credential_id = c.id
		WHERE c.name = $1
		ORDER BY o.name`

	rows, err := s.db.QueryContext(ctx, query, name)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to query organisation references for %q: %w", name, err)
	}
	defer rows.Close()

	var refs []CredentialReference
	for rows.Next() {
		var orgName string
		if err := rows.Scan(&orgName); err != nil {
			return nil, fmt.Errorf("secrets: failed to scan organisation reference: %w", err)
		}
		refs = append(refs, CredentialReference{
			EntityType: "organisation",
			EntityName: orgName,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("secrets: failed to iterate organisation references: %w", err)
	}

	return refs, nil
}

// queryMetadataRows is a helper that executes a SELECT query returning
// credential metadata rows and scans them into a slice.
func (s *DBCredentialStore) queryMetadataRows(ctx context.Context, query string, args ...any) ([]CredentialMetadata, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to query credentials: %w", err)
	}
	defer rows.Close()

	var results []CredentialMetadata
	for rows.Next() {
		var (
			name          string
			credType      string
			metadata      []byte
			lastRotatedAt sql.NullTime
			createdBy     string
			updatedBy     sql.NullString
			createdAt     time.Time
			updatedAt     time.Time
		)

		if err := rows.Scan(
			&name, &credType, &metadata,
			&lastRotatedAt, &createdBy, &updatedBy, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("secrets: failed to scan credential row: %w", err)
		}

		meta := CredentialMetadata{
			Name:           name,
			CredentialType: credType,
			Metadata:       parseJSONBMetadata(metadata),
			CreatedBy:      createdBy,
			CreatedAt:      createdAt,
			UpdatedAt:      updatedAt,
		}

		if lastRotatedAt.Valid {
			t := lastRotatedAt.Time
			meta.LastRotatedAt = &t
		}
		if updatedBy.Valid {
			meta.UpdatedBy = updatedBy.String
		}

		results = append(results, meta)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("secrets: failed to iterate credential rows: %w", err)
	}

	// Return empty slice rather than nil so JSON serialisation produces [].
	if results == nil {
		results = []CredentialMetadata{}
	}

	return results, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// nullableJSONB returns nil if the input is nil or empty, otherwise returns
// the raw bytes. This maps Go nil to SQL NULL for the JSONB column.
func nullableJSONB(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// parseJSONBMetadata unmarshals a JSONB byte slice into a map. Returns nil
// if the input is nil or empty.
func parseJSONBMetadata(b []byte) map[string]any {
	if len(b) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

// isUniqueViolation checks whether a database error represents a unique
// constraint violation. It inspects the error string for the PostgreSQL
// SQLSTATE code 23505 (unique_violation). This avoids importing the pgx
// or lib/pq driver packages.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL drivers typically surface SQLSTATE in the error message or
	// via a Code field. We check the string representation for the code
	// and the common error text patterns.
	errStr := err.Error()
	// lib/pq and pgx both include "23505" or "unique" in their messages.
	for _, pattern := range []string{
		"23505",
		"unique constraint",
		"duplicate key value violates unique constraint",
	} {
		if containsCI(errStr, pattern) {
			return true
		}
	}
	return false
}

// containsCI performs a case-insensitive contains check.
func containsCI(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	// Simple byte-level lower for ASCII patterns (sufficient for PG errors).
	sl := toLowerASCII(s)
	subl := toLowerASCII(substr)
	for i := 0; i <= len(sl)-len(subl); i++ {
		if sl[i:i+len(subl)] == subl {
			return true
		}
	}
	return false
}

// toLowerASCII lowercases ASCII bytes in a string.
func toLowerASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// Rotation helpers
// ---------------------------------------------------------------------------

// ListRotationRows returns all credential rows with their encrypted values,
// suitable for passing to RotateMasterKey. This is the only method that
// exposes the raw encrypted_value column — it exists solely for the key
// rotation flow.
func (s *DBCredentialStore) ListRotationRows(ctx context.Context) ([]RotationRow, error) {
	query := `
		SELECT name, credential_type, encrypted_value
		FROM credentials
		ORDER BY name`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to list rotation rows: %w", err)
	}
	defer rows.Close()

	var result []RotationRow
	for rows.Next() {
		var r RotationRow
		if err := rows.Scan(&r.Name, &r.CredentialType, &r.EncryptedValue); err != nil {
			return nil, fmt.Errorf("secrets: failed to scan rotation row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("secrets: failed to iterate rotation rows: %w", err)
	}

	if result == nil {
		result = []RotationRow{}
	}
	return result, nil
}

// UpdateEncryptedValueRaw updates the encrypted_value column for a single
// credential identified by name. It also sets last_rotated_at and updated_at
// to now(), and updated_by to "key-rotation".
//
// This method is intended exclusively for use by the master key rotation
// flow as the RotationRowWriter callback. It bypasses validation and
// re-encryption since the caller (RotateMasterKey) has already produced the
// new ciphertext.
func (s *DBCredentialStore) UpdateEncryptedValueRaw(ctx context.Context, name, newEncryptedValue string) error {
	query := `
		UPDATE credentials
		SET encrypted_value = $1,
		    last_rotated_at = now(),
		    updated_by      = 'key-rotation',
		    updated_at      = now()
		WHERE name = $2`

	res, err := s.db.ExecContext(ctx, query, newEncryptedValue, name)
	if err != nil {
		return fmt.Errorf("secrets: failed to update encrypted value for %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("secrets: failed to check rows affected for %q: %w", name, err)
	}
	if n == 0 {
		return ErrCredentialNotFound
	}
	return nil
}

// CredentialCount returns the number of credentials in the store. This is
// useful for startup validation to determine whether master key
// configuration is required.
func (s *DBCredentialStore) CredentialCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM credentials`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("secrets: failed to count credentials: %w", err)
	}
	return count, nil
}
