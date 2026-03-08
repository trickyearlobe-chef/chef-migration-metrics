package secrets

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors returned by CredentialStore operations.
var (
	// ErrCredentialNotFound is returned when a credential lookup by name
	// finds no matching row in the database.
	ErrCredentialNotFound = errors.New("secrets: credential not found")

	// ErrCredentialAlreadyExists is returned when a Create operation is
	// attempted with a name that already exists in the database.
	ErrCredentialAlreadyExists = errors.New("secrets: credential with this name already exists")

	// ErrCredentialInUse is returned when a Delete operation is attempted
	// on a credential that is still referenced by one or more entities
	// (e.g. organisations, auth providers, notification channels).
	ErrCredentialInUse = errors.New("secrets: credential is still referenced by other entities")

	// ErrEncryptionKeyNotConfigured is returned when an operation that
	// requires the master encryption key is attempted but no Encryptor
	// has been provided.
	ErrEncryptionKeyNotConfigured = errors.New("secrets: master encryption key is not configured")
)

// Credential represents a fully-decrypted credential retrieved from the
// store. The Plaintext field contains sensitive material and MUST be zeroed
// by the caller using ZeroBytes after use.
type Credential struct {
	// Name is the unique human-readable identifier for this credential.
	Name string

	// CredentialType is one of the recognised credential types:
	// chef_client_key, ldap_bind_password, smtp_password, webhook_url, generic.
	CredentialType string

	// Plaintext is the decrypted credential value. The caller MUST call
	// ZeroBytes(Plaintext) when the value is no longer needed.
	Plaintext []byte

	// Metadata contains non-sensitive metadata extracted during validation
	// (e.g. {"key_format": "pkcs1", "bits": 2048} for chef_client_key).
	Metadata map[string]any

	// LastRotatedAt is the time the credential value was last updated.
	// Nil if the credential has never been rotated after initial creation.
	LastRotatedAt *time.Time

	// CreatedBy is the username of the admin who created this credential.
	CreatedBy string

	// UpdatedBy is the username of the admin who last updated this credential.
	// Empty if the credential has never been updated after creation.
	UpdatedBy string

	// CreatedAt is the time the credential was created.
	CreatedAt time.Time

	// UpdatedAt is the time the credential was last modified.
	UpdatedAt time.Time
}

// CredentialMetadata represents the non-sensitive metadata for a credential.
// It is returned by list and detail operations that do not need to decrypt
// the stored value. The encrypted_value is never included.
type CredentialMetadata struct {
	// Name is the unique human-readable identifier for this credential.
	Name string

	// CredentialType is one of the recognised credential types.
	CredentialType string

	// Metadata contains non-sensitive metadata extracted during validation.
	Metadata map[string]any

	// LastRotatedAt is the time the credential value was last updated.
	// Nil if the credential has never been rotated after initial creation.
	LastRotatedAt *time.Time

	// CreatedBy is the username of the admin who created this credential.
	CreatedBy string

	// UpdatedBy is the username of the admin who last updated this credential.
	UpdatedBy string

	// CreatedAt is the time the credential was created.
	CreatedAt time.Time

	// UpdatedAt is the time the credential was last modified.
	UpdatedAt time.Time
}

// CredentialReference describes an entity that references a credential.
// It is used by ReferencedBy to report which organisations or config
// entries depend on a credential, and by Delete to block deletion of
// in-use credentials.
type CredentialReference struct {
	// EntityType describes the kind of entity holding the reference
	// (e.g. "organisation", "auth_provider", "notification_channel").
	EntityType string

	// EntityName is the human-readable name of the referencing entity
	// (e.g. the organisation name).
	EntityName string
}

// CreateCredentialInput holds the parameters for creating a new credential.
type CreateCredentialInput struct {
	// Name is the unique human-readable identifier. Required.
	Name string

	// CredentialType is the type of credential. Must be one of the
	// recognised credential types. Required.
	CredentialType string

	// Plaintext is the raw credential value to be validated and encrypted.
	// Required. The caller should zero this slice after Create returns.
	Plaintext []byte

	// CreatedBy is the username of the admin creating this credential.
	// Required.
	CreatedBy string
}

// UpdateCredentialInput holds the parameters for rotating a credential's
// value.
type UpdateCredentialInput struct {
	// Name is the unique identifier of the credential to update. Required.
	Name string

	// Plaintext is the new credential value to be validated and encrypted.
	// Required. The caller should zero this slice after Update returns.
	Plaintext []byte

	// UpdatedBy is the username of the admin performing the rotation.
	// Required.
	UpdatedBy string
}

// CredentialStore defines the interface for credential storage operations.
// All methods accept a context.Context for cancellation and deadline
// propagation.
//
// Implementations must:
//   - Validate credential values before storing (using ValidateCredentialValue)
//   - Encrypt plaintext before writing to the database
//   - Decrypt ciphertext when returning Credential (with Plaintext populated)
//   - Never return encrypted_value or plaintext in CredentialMetadata
//   - Check for references before allowing deletion
//   - Update last_rotated_at on value changes
type CredentialStore interface {
	// Create validates, encrypts, and stores a new credential. Returns the
	// metadata of the created credential (without plaintext).
	//
	// Returns ErrCredentialAlreadyExists if a credential with the same name
	// already exists.
	// Returns ErrEncryptionKeyNotConfigured if no Encryptor is available.
	// Returns validation errors if the plaintext fails type-specific checks.
	Create(ctx context.Context, input CreateCredentialInput) (*CredentialMetadata, error)

	// Get retrieves a credential by name and decrypts its value. The
	// returned Credential contains the Plaintext field, which the caller
	// MUST zero after use.
	//
	// Returns ErrCredentialNotFound if no credential with the given name
	// exists.
	// Returns ErrEncryptionKeyNotConfigured if no Encryptor is available.
	// Returns ErrDecryptionFailed if the stored value cannot be decrypted.
	Get(ctx context.Context, name string) (*Credential, error)

	// GetMetadata retrieves a credential's metadata by name without
	// decrypting the stored value. This is suitable for detail views
	// where the plaintext is not needed.
	//
	// Returns ErrCredentialNotFound if no credential with the given name
	// exists.
	GetMetadata(ctx context.Context, name string) (*CredentialMetadata, error)

	// Update validates and re-encrypts a credential's value. This is used
	// for credential value rotation. The last_rotated_at and updated_at
	// timestamps are updated.
	//
	// Returns ErrCredentialNotFound if no credential with the given name
	// exists.
	// Returns ErrEncryptionKeyNotConfigured if no Encryptor is available.
	// Returns validation errors if the new plaintext fails type-specific
	// checks.
	Update(ctx context.Context, input UpdateCredentialInput) (*CredentialMetadata, error)

	// Delete removes a credential by name. The row is hard-deleted.
	//
	// Returns ErrCredentialNotFound if no credential with the given name
	// exists.
	// Returns ErrCredentialInUse if the credential is still referenced by
	// one or more entities. The caller should use ReferencedBy to discover
	// what is holding the reference.
	Delete(ctx context.Context, name string) error

	// List returns metadata for all credentials, ordered by name. The
	// result never includes plaintext or encrypted values.
	List(ctx context.Context) ([]CredentialMetadata, error)

	// ListByType returns metadata for all credentials of the given type,
	// ordered by name. The result never includes plaintext or encrypted
	// values.
	//
	// Returns ErrInvalidCredentialType if the type is not recognised.
	ListByType(ctx context.Context, credentialType string) ([]CredentialMetadata, error)

	// Test decrypts a credential and performs type-specific validation to
	// verify the credential is still functional. The plaintext is zeroed
	// before returning.
	//
	// Returns ErrCredentialNotFound if no credential with the given name
	// exists.
	// Returns ErrEncryptionKeyNotConfigured if no Encryptor is available.
	// Returns a ValidationResult describing the outcome. If the credential
	// can be decrypted and passes type-specific validation, Valid is true.
	Test(ctx context.Context, name string) (*ValidationResult, error)

	// ReferencedBy returns a list of entities that reference the named
	// credential. This is used to determine whether a credential can be
	// safely deleted and to inform the operator what needs to be unlinked.
	//
	// Returns ErrCredentialNotFound if no credential with the given name
	// exists.
	// Returns an empty slice if no entities reference the credential.
	ReferencedBy(ctx context.Context, name string) ([]CredentialReference, error)
}
