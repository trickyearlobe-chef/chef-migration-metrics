package secrets

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// InMemoryCredentialStore — test double for CredentialStore
// ---------------------------------------------------------------------------
//
// InMemoryCredentialStore is a pure Go implementation of CredentialStore
// that uses the same Encryptor, the same ValidateCredentialValue, and the
// same error contract as DBCredentialStore — minus the SQL layer.
//
// This lets us:
//   1. Validate the CredentialStore interface contract thoroughly
//   2. Test encryption round-trips
//   3. Test validation integration
//   4. Test reference checking logic
//   5. Test all error paths
//
// The DBCredentialStore's SQL correctness is verified by functional tests
// (build-tagged) that run against a real PostgreSQL instance.
// ---------------------------------------------------------------------------

// InMemoryCredentialStore implements CredentialStore backed by in-memory
// maps. It uses the real Encryptor for encryption/decryption and the real
// ValidateCredentialValue for validation, ensuring the same behaviour as
// DBCredentialStore minus the SQL layer.
type InMemoryCredentialStore struct {
	mu          sync.Mutex
	encryptor   *Encryptor
	credentials map[string]*inMemCredential
	// orgRefs maps credential name → list of organisation names that
	// reference it.
	orgRefs map[string][]string
}

type inMemCredential struct {
	name           string
	credentialType string
	encryptedValue string // <nonce_hex>:<ciphertext_hex>
	metadata       map[string]any
	lastRotatedAt  *time.Time
	createdBy      string
	updatedBy      string
	createdAt      time.Time
	updatedAt      time.Time
}

func NewInMemoryCredentialStore(encryptor *Encryptor) *InMemoryCredentialStore {
	return &InMemoryCredentialStore{
		encryptor:   encryptor,
		credentials: make(map[string]*inMemCredential),
		orgRefs:     make(map[string][]string),
	}
}

// AddOrgReference simulates an organisation referencing a credential (for
// testing reference checks and delete blocking).
func (s *InMemoryCredentialStore) AddOrgReference(credentialName, orgName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orgRefs[credentialName] = append(s.orgRefs[credentialName], orgName)
}

func (s *InMemoryCredentialStore) Create(ctx context.Context, input CreateCredentialInput) (*CredentialMetadata, error) {
	if s.encryptor == nil {
		return nil, ErrEncryptionKeyNotConfigured
	}
	if input.Name == "" {
		return nil, fmt.Errorf("secrets: credential name is required")
	}
	if input.CreatedBy == "" {
		return nil, fmt.Errorf("secrets: created_by is required")
	}

	result := ValidateCredentialValue(input.CredentialType, input.Plaintext)
	if !result.Valid {
		return nil, fmt.Errorf("secrets: validation failed: %w", result.Error)
	}

	aad, err := BuildAAD(input.CredentialType, input.Name)
	if err != nil {
		return nil, err
	}

	encrypted, err := s.encryptor.Encrypt(input.Plaintext, aad)
	if err != nil {
		return nil, fmt.Errorf("secrets: encryption failed: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.credentials[input.Name]; exists {
		return nil, ErrCredentialAlreadyExists
	}

	now := time.Now().UTC()
	cred := &inMemCredential{
		name:           input.Name,
		credentialType: input.CredentialType,
		encryptedValue: encrypted,
		metadata:       result.Metadata,
		createdBy:      input.CreatedBy,
		createdAt:      now,
		updatedAt:      now,
	}
	s.credentials[input.Name] = cred

	return &CredentialMetadata{
		Name:           cred.name,
		CredentialType: cred.credentialType,
		Metadata:       cred.metadata,
		CreatedBy:      cred.createdBy,
		CreatedAt:      cred.createdAt,
		UpdatedAt:      cred.updatedAt,
	}, nil
}

func (s *InMemoryCredentialStore) Get(ctx context.Context, name string) (*Credential, error) {
	if s.encryptor == nil {
		return nil, ErrEncryptionKeyNotConfigured
	}

	s.mu.Lock()
	cred, exists := s.credentials[name]
	s.mu.Unlock()

	if !exists {
		return nil, ErrCredentialNotFound
	}

	aad, err := BuildAAD(cred.credentialType, cred.name)
	if err != nil {
		return nil, err
	}

	plaintext, err := s.encryptor.Decrypt(cred.encryptedValue, aad)
	if err != nil {
		return nil, fmt.Errorf("secrets: failed to decrypt credential %q: %w", name, err)
	}

	return &Credential{
		Name:           cred.name,
		CredentialType: cred.credentialType,
		Plaintext:      plaintext,
		Metadata:       cred.metadata,
		LastRotatedAt:  cred.lastRotatedAt,
		CreatedBy:      cred.createdBy,
		UpdatedBy:      cred.updatedBy,
		CreatedAt:      cred.createdAt,
		UpdatedAt:      cred.updatedAt,
	}, nil
}

func (s *InMemoryCredentialStore) GetMetadata(ctx context.Context, name string) (*CredentialMetadata, error) {
	s.mu.Lock()
	cred, exists := s.credentials[name]
	s.mu.Unlock()

	if !exists {
		return nil, ErrCredentialNotFound
	}

	return &CredentialMetadata{
		Name:           cred.name,
		CredentialType: cred.credentialType,
		Metadata:       cred.metadata,
		LastRotatedAt:  cred.lastRotatedAt,
		CreatedBy:      cred.createdBy,
		UpdatedBy:      cred.updatedBy,
		CreatedAt:      cred.createdAt,
		UpdatedAt:      cred.updatedAt,
	}, nil
}

func (s *InMemoryCredentialStore) Update(ctx context.Context, input UpdateCredentialInput) (*CredentialMetadata, error) {
	if s.encryptor == nil {
		return nil, ErrEncryptionKeyNotConfigured
	}
	if input.Name == "" {
		return nil, fmt.Errorf("secrets: credential name is required")
	}
	if input.UpdatedBy == "" {
		return nil, fmt.Errorf("secrets: updated_by is required")
	}

	s.mu.Lock()
	cred, exists := s.credentials[input.Name]
	if !exists {
		s.mu.Unlock()
		return nil, ErrCredentialNotFound
	}
	credType := cred.credentialType
	s.mu.Unlock()

	result := ValidateCredentialValue(credType, input.Plaintext)
	if !result.Valid {
		return nil, fmt.Errorf("secrets: validation failed: %w", result.Error)
	}

	aad, err := BuildAAD(credType, input.Name)
	if err != nil {
		return nil, err
	}

	encrypted, err := s.encryptor.Encrypt(input.Plaintext, aad)
	if err != nil {
		return nil, fmt.Errorf("secrets: encryption failed: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-check existence under lock.
	cred, exists = s.credentials[input.Name]
	if !exists {
		return nil, ErrCredentialNotFound
	}

	now := time.Now().UTC()
	cred.encryptedValue = encrypted
	cred.metadata = result.Metadata
	cred.lastRotatedAt = &now
	cred.updatedBy = input.UpdatedBy
	cred.updatedAt = now

	return &CredentialMetadata{
		Name:           cred.name,
		CredentialType: cred.credentialType,
		Metadata:       cred.metadata,
		LastRotatedAt:  cred.lastRotatedAt,
		CreatedBy:      cred.createdBy,
		UpdatedBy:      cred.updatedBy,
		CreatedAt:      cred.createdAt,
		UpdatedAt:      cred.updatedAt,
	}, nil
}

func (s *InMemoryCredentialStore) Delete(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.credentials[name]; !exists {
		return ErrCredentialNotFound
	}

	if refs, ok := s.orgRefs[name]; ok && len(refs) > 0 {
		return ErrCredentialInUse
	}

	delete(s.credentials, name)
	return nil
}

func (s *InMemoryCredentialStore) List(ctx context.Context) ([]CredentialMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := make([]CredentialMetadata, 0, len(s.credentials))
	for _, cred := range s.credentials {
		meta := CredentialMetadata{
			Name:           cred.name,
			CredentialType: cred.credentialType,
			Metadata:       cred.metadata,
			LastRotatedAt:  cred.lastRotatedAt,
			CreatedBy:      cred.createdBy,
			UpdatedBy:      cred.updatedBy,
			CreatedAt:      cred.createdAt,
			UpdatedAt:      cred.updatedAt,
		}
		results = append(results, meta)
	}

	// Sort by name to match DB ORDER BY.
	sortMetadataByName(results)
	return results, nil
}

func (s *InMemoryCredentialStore) ListByType(ctx context.Context, credentialType string) ([]CredentialMetadata, error) {
	if !IsValidCredentialType(credentialType) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidCredentialType, credentialType)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var results []CredentialMetadata
	for _, cred := range s.credentials {
		if cred.credentialType != credentialType {
			continue
		}
		meta := CredentialMetadata{
			Name:           cred.name,
			CredentialType: cred.credentialType,
			Metadata:       cred.metadata,
			LastRotatedAt:  cred.lastRotatedAt,
			CreatedBy:      cred.createdBy,
			UpdatedBy:      cred.updatedBy,
			CreatedAt:      cred.createdAt,
			UpdatedAt:      cred.updatedAt,
		}
		results = append(results, meta)
	}

	if results == nil {
		results = []CredentialMetadata{}
	}
	sortMetadataByName(results)
	return results, nil
}

func (s *InMemoryCredentialStore) Test(ctx context.Context, name string) (*ValidationResult, error) {
	cred, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(cred.Plaintext)

	result := ValidateCredentialValue(cred.CredentialType, cred.Plaintext)
	return &result, nil
}

func (s *InMemoryCredentialStore) ReferencedBy(ctx context.Context, name string) ([]CredentialReference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.credentials[name]; !exists {
		return nil, ErrCredentialNotFound
	}

	refs, ok := s.orgRefs[name]
	if !ok || len(refs) == 0 {
		return []CredentialReference{}, nil
	}

	var result []CredentialReference
	for _, orgName := range refs {
		result = append(result, CredentialReference{
			EntityType: "organisation",
			EntityName: orgName,
		})
	}
	return result, nil
}

// sortMetadataByName sorts a slice of CredentialMetadata by Name.
func sortMetadataByName(s []CredentialMetadata) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].Name < s[j-1].Name; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// Compile-time check that InMemoryCredentialStore implements CredentialStore.
var _ CredentialStore = (*InMemoryCredentialStore)(nil)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func testEncryptor(t *testing.T) *Encryptor {
	t.Helper()
	// Generate a valid 32-byte key, base64-encoded.
	key := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("failed to create test encryptor: %v", err)
	}
	t.Cleanup(func() { enc.Close() })
	return enc
}

func testEncryptorAlt(t *testing.T) *Encryptor {
	t.Helper()
	key := base64.StdEncoding.EncodeToString([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ012345"))
	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("failed to create alt test encryptor: %v", err)
	}
	t.Cleanup(func() { enc.Close() })
	return enc
}

func testStore(t *testing.T) *InMemoryCredentialStore {
	t.Helper()
	return NewInMemoryCredentialStore(testEncryptor(t))
}

func testStoreNoEncryptor(t *testing.T) *InMemoryCredentialStore {
	t.Helper()
	return NewInMemoryCredentialStore(nil)
}

func mustCreateGeneric(t *testing.T, store CredentialStore, name, value, createdBy string) *CredentialMetadata {
	t.Helper()
	meta, err := store.Create(context.Background(), CreateCredentialInput{
		Name:           name,
		CredentialType: CredentialTypeGeneric,
		Plaintext:      []byte(value),
		CreatedBy:      createdBy,
	})
	if err != nil {
		t.Fatalf("failed to create credential %q: %v", name, err)
	}
	return meta
}

func mustCreateWebhook(t *testing.T, store CredentialStore, name, url, createdBy string) *CredentialMetadata {
	t.Helper()
	meta, err := store.Create(context.Background(), CreateCredentialInput{
		Name:           name,
		CredentialType: CredentialTypeWebhookURL,
		Plaintext:      []byte(url),
		CreatedBy:      createdBy,
	})
	if err != nil {
		t.Fatalf("failed to create webhook credential %q: %v", name, err)
	}
	return meta
}

// ---------------------------------------------------------------------------
// Create tests
// ---------------------------------------------------------------------------

func TestStore_Create_Success_Generic(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	meta, err := store.Create(ctx, CreateCredentialInput{
		Name:           "my-secret",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      []byte("super-secret-value"),
		CreatedBy:      "admin",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if meta.Name != "my-secret" {
		t.Errorf("Name = %q, want %q", meta.Name, "my-secret")
	}
	if meta.CredentialType != CredentialTypeGeneric {
		t.Errorf("CredentialType = %q, want %q", meta.CredentialType, CredentialTypeGeneric)
	}
	if meta.CreatedBy != "admin" {
		t.Errorf("CreatedBy = %q, want %q", meta.CreatedBy, "admin")
	}
	if meta.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if meta.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
	// Generic credentials have no metadata.
	if meta.Metadata != nil {
		t.Errorf("Metadata should be nil for generic, got %v", meta.Metadata)
	}
	// Not rotated yet.
	if meta.LastRotatedAt != nil {
		t.Error("LastRotatedAt should be nil on initial creation")
	}
}

func TestStore_Create_Success_WebhookURL(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	meta, err := store.Create(ctx, CreateCredentialInput{
		Name:           "webhook-prod",
		CredentialType: CredentialTypeWebhookURL,
		Plaintext:      []byte("https://hooks.example.com/notify"),
		CreatedBy:      "admin",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if meta.Name != "webhook-prod" {
		t.Errorf("Name = %q, want %q", meta.Name, "webhook-prod")
	}
	if meta.CredentialType != CredentialTypeWebhookURL {
		t.Errorf("CredentialType = %q, want %q", meta.CredentialType, CredentialTypeWebhookURL)
	}
}

func TestStore_Create_Success_LDAPBindPassword(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	meta, err := store.Create(ctx, CreateCredentialInput{
		Name:           "ldap-bind",
		CredentialType: CredentialTypeLDAPBindPassword,
		Plaintext:      []byte("ldap-pass-123"),
		CreatedBy:      "admin",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if meta.CredentialType != CredentialTypeLDAPBindPassword {
		t.Errorf("CredentialType = %q, want %q", meta.CredentialType, CredentialTypeLDAPBindPassword)
	}
}

func TestStore_Create_Success_SMTPPassword(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	meta, err := store.Create(ctx, CreateCredentialInput{
		Name:           "smtp-pass",
		CredentialType: CredentialTypeSMTPPassword,
		Plaintext:      []byte("smtp-secret"),
		CreatedBy:      "admin",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if meta.CredentialType != CredentialTypeSMTPPassword {
		t.Errorf("CredentialType = %q, want %q", meta.CredentialType, CredentialTypeSMTPPassword)
	}
}

func TestStore_Create_DuplicateName(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "dup-test", "value1", "admin")

	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "dup-test",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      []byte("value2"),
		CreatedBy:      "admin",
	})
	if !errors.Is(err, ErrCredentialAlreadyExists) {
		t.Fatalf("expected ErrCredentialAlreadyExists, got %v", err)
	}
}

func TestStore_Create_EmptyName(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      []byte("value"),
		CreatedBy:      "admin",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStore_Create_EmptyCreatedBy(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "test",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      []byte("value"),
		CreatedBy:      "",
	})
	if err == nil {
		t.Fatal("expected error for empty created_by")
	}
	if !strings.Contains(err.Error(), "created_by is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStore_Create_InvalidCredentialType(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "test",
		CredentialType: "invalid_type",
		Plaintext:      []byte("value"),
		CreatedBy:      "admin",
	})
	if err == nil {
		t.Fatal("expected error for invalid credential type")
	}
	if !errors.Is(err, ErrInvalidCredentialType) {
		t.Errorf("expected ErrInvalidCredentialType in chain, got: %v", err)
	}
}

func TestStore_Create_EmptyPlaintext(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "test",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      []byte{},
		CreatedBy:      "admin",
	})
	if err == nil {
		t.Fatal("expected error for empty plaintext")
	}
	if !errors.Is(err, ErrEmptyValue) {
		t.Errorf("expected ErrEmptyValue in chain, got: %v", err)
	}
}

func TestStore_Create_NilPlaintext(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "test",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      nil,
		CreatedBy:      "admin",
	})
	if err == nil {
		t.Fatal("expected error for nil plaintext")
	}
	if !errors.Is(err, ErrEmptyValue) {
		t.Errorf("expected ErrEmptyValue in chain, got: %v", err)
	}
}

func TestStore_Create_WebhookInvalidURL(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "bad-webhook",
		CredentialType: CredentialTypeWebhookURL,
		Plaintext:      []byte("ftp://not-http.example.com"),
		CreatedBy:      "admin",
	})
	if err == nil {
		t.Fatal("expected error for FTP webhook URL")
	}
	if !errors.Is(err, ErrInvalidWebhookURL) {
		t.Errorf("expected ErrInvalidWebhookURL in chain, got: %v", err)
	}
}

func TestStore_Create_NoEncryptor(t *testing.T) {
	store := testStoreNoEncryptor(t)
	ctx := context.Background()

	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "test",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      []byte("value"),
		CreatedBy:      "admin",
	})
	if !errors.Is(err, ErrEncryptionKeyNotConfigured) {
		t.Fatalf("expected ErrEncryptionKeyNotConfigured, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Get tests
// ---------------------------------------------------------------------------

func TestStore_Get_Success(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "get-test", "my-secret-value", "admin")

	cred, err := store.Get(ctx, "get-test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer ZeroBytes(cred.Plaintext)

	if cred.Name != "get-test" {
		t.Errorf("Name = %q, want %q", cred.Name, "get-test")
	}
	if cred.CredentialType != CredentialTypeGeneric {
		t.Errorf("CredentialType = %q, want %q", cred.CredentialType, CredentialTypeGeneric)
	}
	if string(cred.Plaintext) != "my-secret-value" {
		t.Errorf("Plaintext = %q, want %q", string(cred.Plaintext), "my-secret-value")
	}
	if cred.CreatedBy != "admin" {
		t.Errorf("CreatedBy = %q, want %q", cred.CreatedBy, "admin")
	}
}

func TestStore_Get_DecryptionRoundTrip_AllTypes(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	tests := []struct {
		name           string
		credentialType string
		value          string
	}{
		{"generic-cred", CredentialTypeGeneric, "generic-secret"},
		{"ldap-cred", CredentialTypeLDAPBindPassword, "ldap-password"},
		{"smtp-cred", CredentialTypeSMTPPassword, "smtp-password"},
		{"webhook-cred", CredentialTypeWebhookURL, "https://hooks.example.com/test"},
	}

	for _, tt := range tests {
		t.Run(tt.credentialType, func(t *testing.T) {
			_, err := store.Create(ctx, CreateCredentialInput{
				Name:           tt.name,
				CredentialType: tt.credentialType,
				Plaintext:      []byte(tt.value),
				CreatedBy:      "admin",
			})
			if err != nil {
				t.Fatalf("Create failed: %v", err)
			}

			cred, err := store.Get(ctx, tt.name)
			if err != nil {
				t.Fatalf("Get failed: %v", err)
			}
			defer ZeroBytes(cred.Plaintext)

			if string(cred.Plaintext) != tt.value {
				t.Errorf("Plaintext = %q, want %q", string(cred.Plaintext), tt.value)
			}
		})
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("expected ErrCredentialNotFound, got %v", err)
	}
}

func TestStore_Get_NoEncryptor(t *testing.T) {
	store := testStoreNoEncryptor(t)
	ctx := context.Background()

	_, err := store.Get(ctx, "anything")
	if !errors.Is(err, ErrEncryptionKeyNotConfigured) {
		t.Fatalf("expected ErrEncryptionKeyNotConfigured, got %v", err)
	}
}

func TestStore_Get_SpecialCharactersInValue(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	specialValue := "p@$$w0rd!#%^&*()_+-=[]{}|;':\",./<>?\n\ttab"
	mustCreateGeneric(t, store, "special-chars", specialValue, "admin")

	cred, err := store.Get(ctx, "special-chars")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer ZeroBytes(cred.Plaintext)

	if string(cred.Plaintext) != specialValue {
		t.Errorf("Plaintext does not match special characters")
	}
}

func TestStore_Get_BinaryValue(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	binaryValue := make([]byte, 256)
	for i := range binaryValue {
		binaryValue[i] = byte(i)
	}

	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "binary-cred",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      binaryValue,
		CreatedBy:      "admin",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	cred, err := store.Get(ctx, "binary-cred")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer ZeroBytes(cred.Plaintext)

	if len(cred.Plaintext) != len(binaryValue) {
		t.Fatalf("Plaintext length = %d, want %d", len(cred.Plaintext), len(binaryValue))
	}
	for i := range binaryValue {
		if cred.Plaintext[i] != binaryValue[i] {
			t.Fatalf("Plaintext[%d] = %d, want %d", i, cred.Plaintext[i], binaryValue[i])
		}
	}
}

func TestStore_Get_LargeValue(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	// 64 KB value
	largeValue := make([]byte, 64*1024)
	for i := range largeValue {
		largeValue[i] = byte('A' + (i % 26))
	}

	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "large-cred",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      largeValue,
		CreatedBy:      "admin",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	cred, err := store.Get(ctx, "large-cred")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer ZeroBytes(cred.Plaintext)

	if len(cred.Plaintext) != len(largeValue) {
		t.Fatalf("Plaintext length = %d, want %d", len(cred.Plaintext), len(largeValue))
	}
}

// ---------------------------------------------------------------------------
// GetMetadata tests
// ---------------------------------------------------------------------------

func TestStore_GetMetadata_Success(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "meta-test", "secret-value", "admin")

	meta, err := store.GetMetadata(ctx, "meta-test")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}

	if meta.Name != "meta-test" {
		t.Errorf("Name = %q, want %q", meta.Name, "meta-test")
	}
	if meta.CredentialType != CredentialTypeGeneric {
		t.Errorf("CredentialType = %q, want %q", meta.CredentialType, CredentialTypeGeneric)
	}
	if meta.CreatedBy != "admin" {
		t.Errorf("CreatedBy = %q, want %q", meta.CreatedBy, "admin")
	}
}

func TestStore_GetMetadata_NotFound(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.GetMetadata(ctx, "nonexistent")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("expected ErrCredentialNotFound, got %v", err)
	}
}

func TestStore_GetMetadata_DoesNotRequireEncryptor(t *testing.T) {
	// GetMetadata should work even without an encryptor since it doesn't
	// decrypt. However, our InMemoryCredentialStore needs an encryptor
	// to Create. So we create with one store and query metadata with another.
	// In the in-memory store, GetMetadata doesn't use the encryptor, so
	// we test it indirectly: create first, then verify GetMetadata works.
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "no-enc-meta", "value", "admin")

	meta, err := store.GetMetadata(ctx, "no-enc-meta")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if meta.Name != "no-enc-meta" {
		t.Errorf("Name = %q, want %q", meta.Name, "no-enc-meta")
	}
}

// ---------------------------------------------------------------------------
// Update tests
// ---------------------------------------------------------------------------

func TestStore_Update_Success(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "update-test", "old-value", "admin")

	meta, err := store.Update(ctx, UpdateCredentialInput{
		Name:      "update-test",
		Plaintext: []byte("new-value"),
		UpdatedBy: "operator",
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if meta.Name != "update-test" {
		t.Errorf("Name = %q, want %q", meta.Name, "update-test")
	}
	if meta.UpdatedBy != "operator" {
		t.Errorf("UpdatedBy = %q, want %q", meta.UpdatedBy, "operator")
	}
	if meta.LastRotatedAt == nil {
		t.Error("LastRotatedAt should be set after update")
	}

	// Verify the new value can be retrieved.
	cred, err := store.Get(ctx, "update-test")
	if err != nil {
		t.Fatalf("Get after update failed: %v", err)
	}
	defer ZeroBytes(cred.Plaintext)

	if string(cred.Plaintext) != "new-value" {
		t.Errorf("Plaintext = %q, want %q", string(cred.Plaintext), "new-value")
	}
}

func TestStore_Update_PreservesCredentialType(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateWebhook(t, store, "webhook-update", "https://old.example.com", "admin")

	_, err := store.Update(ctx, UpdateCredentialInput{
		Name:      "webhook-update",
		Plaintext: []byte("https://new.example.com/hook"),
		UpdatedBy: "operator",
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	cred, err := store.Get(ctx, "webhook-update")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer ZeroBytes(cred.Plaintext)

	if cred.CredentialType != CredentialTypeWebhookURL {
		t.Errorf("CredentialType changed to %q after update", cred.CredentialType)
	}
	if string(cred.Plaintext) != "https://new.example.com/hook" {
		t.Errorf("Plaintext = %q", string(cred.Plaintext))
	}
}

func TestStore_Update_ValidationFailure_WebhookInvalidURL(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateWebhook(t, store, "webhook-bad-update", "https://valid.example.com", "admin")

	_, err := store.Update(ctx, UpdateCredentialInput{
		Name:      "webhook-bad-update",
		Plaintext: []byte("ftp://not-http.example.com"),
		UpdatedBy: "operator",
	})
	if err == nil {
		t.Fatal("expected validation error for FTP URL")
	}
	if !errors.Is(err, ErrInvalidWebhookURL) {
		t.Errorf("expected ErrInvalidWebhookURL in chain, got: %v", err)
	}

	// Original value should be unchanged.
	cred, err := store.Get(ctx, "webhook-bad-update")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer ZeroBytes(cred.Plaintext)

	if string(cred.Plaintext) != "https://valid.example.com" {
		t.Errorf("Value should be unchanged after failed update, got %q", string(cred.Plaintext))
	}
}

func TestStore_Update_ValidationFailure_EmptyValue(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "empty-update", "original", "admin")

	_, err := store.Update(ctx, UpdateCredentialInput{
		Name:      "empty-update",
		Plaintext: []byte{},
		UpdatedBy: "operator",
	})
	if err == nil {
		t.Fatal("expected error for empty plaintext")
	}
	if !errors.Is(err, ErrEmptyValue) {
		t.Errorf("expected ErrEmptyValue in chain, got: %v", err)
	}
}

func TestStore_Update_NotFound(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.Update(ctx, UpdateCredentialInput{
		Name:      "nonexistent",
		Plaintext: []byte("value"),
		UpdatedBy: "admin",
	})
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("expected ErrCredentialNotFound, got %v", err)
	}
}

func TestStore_Update_EmptyName(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.Update(ctx, UpdateCredentialInput{
		Name:      "",
		Plaintext: []byte("value"),
		UpdatedBy: "admin",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestStore_Update_EmptyUpdatedBy(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "no-updater", "value", "admin")

	_, err := store.Update(ctx, UpdateCredentialInput{
		Name:      "no-updater",
		Plaintext: []byte("new-value"),
		UpdatedBy: "",
	})
	if err == nil {
		t.Fatal("expected error for empty updated_by")
	}
}

func TestStore_Update_NoEncryptor(t *testing.T) {
	store := testStoreNoEncryptor(t)
	ctx := context.Background()

	_, err := store.Update(ctx, UpdateCredentialInput{
		Name:      "anything",
		Plaintext: []byte("value"),
		UpdatedBy: "admin",
	})
	if !errors.Is(err, ErrEncryptionKeyNotConfigured) {
		t.Fatalf("expected ErrEncryptionKeyNotConfigured, got %v", err)
	}
}

func TestStore_Update_MultipleRotations(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "multi-rotate", "v1", "admin")

	for i := 2; i <= 5; i++ {
		newVal := fmt.Sprintf("v%d", i)
		meta, err := store.Update(ctx, UpdateCredentialInput{
			Name:      "multi-rotate",
			Plaintext: []byte(newVal),
			UpdatedBy: fmt.Sprintf("operator-%d", i),
		})
		if err != nil {
			t.Fatalf("Update %d failed: %v", i, err)
		}
		if meta.LastRotatedAt == nil {
			t.Fatalf("LastRotatedAt should be set after update %d", i)
		}
	}

	cred, err := store.Get(ctx, "multi-rotate")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer ZeroBytes(cred.Plaintext)

	if string(cred.Plaintext) != "v5" {
		t.Errorf("Plaintext = %q, want %q", string(cred.Plaintext), "v5")
	}
	if cred.UpdatedBy != "operator-5" {
		t.Errorf("UpdatedBy = %q, want %q", cred.UpdatedBy, "operator-5")
	}
}

// ---------------------------------------------------------------------------
// Delete tests
// ---------------------------------------------------------------------------

func TestStore_Delete_Success(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "delete-me", "value", "admin")

	err := store.Delete(ctx, "delete-me")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone.
	_, err = store.Get(ctx, "delete-me")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("expected ErrCredentialNotFound after delete, got %v", err)
	}
}

func TestStore_Delete_NotFound(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	err := store.Delete(ctx, "nonexistent")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("expected ErrCredentialNotFound, got %v", err)
	}
}

func TestStore_Delete_InUse(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "in-use-cred", "value", "admin")
	store.AddOrgReference("in-use-cred", "production-org")

	err := store.Delete(ctx, "in-use-cred")
	if !errors.Is(err, ErrCredentialInUse) {
		t.Fatalf("expected ErrCredentialInUse, got %v", err)
	}

	// Verify it still exists.
	_, err = store.Get(ctx, "in-use-cred")
	if err != nil {
		t.Fatalf("credential should still exist after blocked delete: %v", err)
	}
}

func TestStore_Delete_InUse_MultipleReferences(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "multi-ref-cred", "value", "admin")
	store.AddOrgReference("multi-ref-cred", "org-1")
	store.AddOrgReference("multi-ref-cred", "org-2")
	store.AddOrgReference("multi-ref-cred", "org-3")

	err := store.Delete(ctx, "multi-ref-cred")
	if !errors.Is(err, ErrCredentialInUse) {
		t.Fatalf("expected ErrCredentialInUse, got %v", err)
	}
}

func TestStore_Delete_DoubleDelete(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "double-delete", "value", "admin")

	if err := store.Delete(ctx, "double-delete"); err != nil {
		t.Fatalf("first Delete failed: %v", err)
	}

	err := store.Delete(ctx, "double-delete")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("second Delete should return ErrCredentialNotFound, got %v", err)
	}
}

func TestStore_Delete_ThenRecreate(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "recreate-me", "original", "admin")

	if err := store.Delete(ctx, "recreate-me"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Recreate with same name.
	mustCreateGeneric(t, store, "recreate-me", "new-value", "admin2")

	cred, err := store.Get(ctx, "recreate-me")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer ZeroBytes(cred.Plaintext)

	if string(cred.Plaintext) != "new-value" {
		t.Errorf("Plaintext = %q, want %q", string(cred.Plaintext), "new-value")
	}
	if cred.CreatedBy != "admin2" {
		t.Errorf("CreatedBy = %q, want %q", cred.CreatedBy, "admin2")
	}
}

// ---------------------------------------------------------------------------
// List tests
// ---------------------------------------------------------------------------

func TestStore_List_Empty(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if list == nil {
		t.Fatal("List should return empty slice, not nil")
	}
	if len(list) != 0 {
		t.Errorf("List length = %d, want 0", len(list))
	}
}

func TestStore_List_Multiple(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "charlie", "v1", "admin")
	mustCreateGeneric(t, store, "alpha", "v2", "admin")
	mustCreateGeneric(t, store, "bravo", "v3", "admin")

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("List length = %d, want 3", len(list))
	}

	// Should be sorted by name.
	if list[0].Name != "alpha" {
		t.Errorf("list[0].Name = %q, want %q", list[0].Name, "alpha")
	}
	if list[1].Name != "bravo" {
		t.Errorf("list[1].Name = %q, want %q", list[1].Name, "bravo")
	}
	if list[2].Name != "charlie" {
		t.Errorf("list[2].Name = %q, want %q", list[2].Name, "charlie")
	}
}

func TestStore_List_ReflectsCreatesAndDeletes(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "one", "v1", "admin")
	mustCreateGeneric(t, store, "two", "v2", "admin")

	list, _ := store.List(ctx)
	if len(list) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(list))
	}

	_ = store.Delete(ctx, "one")

	list, _ = store.List(ctx)
	if len(list) != 1 {
		t.Fatalf("expected 1 credential after delete, got %d", len(list))
	}
	if list[0].Name != "two" {
		t.Errorf("remaining credential = %q, want %q", list[0].Name, "two")
	}
}

func TestStore_List_MixedTypes(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "generic-1", "v1", "admin")
	mustCreateWebhook(t, store, "webhook-1", "https://example.com", "admin")

	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "ldap-1",
		CredentialType: CredentialTypeLDAPBindPassword,
		Plaintext:      []byte("ldap-pass"),
		CreatedBy:      "admin",
	})
	if err != nil {
		t.Fatalf("Create LDAP failed: %v", err)
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("List length = %d, want 3", len(list))
	}

	types := make(map[string]bool)
	for _, m := range list {
		types[m.CredentialType] = true
	}
	if !types[CredentialTypeGeneric] {
		t.Error("missing generic type in list")
	}
	if !types[CredentialTypeWebhookURL] {
		t.Error("missing webhook type in list")
	}
	if !types[CredentialTypeLDAPBindPassword] {
		t.Error("missing ldap type in list")
	}
}

// ---------------------------------------------------------------------------
// ListByType tests
// ---------------------------------------------------------------------------

func TestStore_ListByType_Success(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "gen-1", "v1", "admin")
	mustCreateGeneric(t, store, "gen-2", "v2", "admin")
	mustCreateWebhook(t, store, "hook-1", "https://example.com", "admin")

	list, err := store.ListByType(ctx, CredentialTypeGeneric)
	if err != nil {
		t.Fatalf("ListByType failed: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("ListByType length = %d, want 2", len(list))
	}

	for _, m := range list {
		if m.CredentialType != CredentialTypeGeneric {
			t.Errorf("unexpected type %q in generic list", m.CredentialType)
		}
	}
}

func TestStore_ListByType_Empty(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "gen-only", "v1", "admin")

	list, err := store.ListByType(ctx, CredentialTypeWebhookURL)
	if err != nil {
		t.Fatalf("ListByType failed: %v", err)
	}

	if list == nil {
		t.Fatal("ListByType should return empty slice, not nil")
	}
	if len(list) != 0 {
		t.Errorf("ListByType length = %d, want 0", len(list))
	}
}

func TestStore_ListByType_InvalidType(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.ListByType(ctx, "bogus_type")
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !errors.Is(err, ErrInvalidCredentialType) {
		t.Errorf("expected ErrInvalidCredentialType, got: %v", err)
	}
}

func TestStore_ListByType_SortedByName(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "zulu", "v1", "admin")
	mustCreateGeneric(t, store, "alpha", "v2", "admin")
	mustCreateGeneric(t, store, "mike", "v3", "admin")

	list, err := store.ListByType(ctx, CredentialTypeGeneric)
	if err != nil {
		t.Fatalf("ListByType failed: %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("length = %d, want 3", len(list))
	}
	if list[0].Name != "alpha" || list[1].Name != "mike" || list[2].Name != "zulu" {
		t.Errorf("not sorted: %q, %q, %q", list[0].Name, list[1].Name, list[2].Name)
	}
}

func TestStore_ListByType_AllTypes(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "gen", "v", "admin")
	mustCreateWebhook(t, store, "hook", "https://x.com", "admin")

	store.Create(ctx, CreateCredentialInput{
		Name: "ldap", CredentialType: CredentialTypeLDAPBindPassword,
		Plaintext: []byte("p"), CreatedBy: "admin",
	})
	store.Create(ctx, CreateCredentialInput{
		Name: "smtp", CredentialType: CredentialTypeSMTPPassword,
		Plaintext: []byte("p"), CreatedBy: "admin",
	})

	for _, ct := range []string{
		CredentialTypeGeneric,
		CredentialTypeWebhookURL,
		CredentialTypeLDAPBindPassword,
		CredentialTypeSMTPPassword,
	} {
		list, err := store.ListByType(ctx, ct)
		if err != nil {
			t.Fatalf("ListByType(%q) failed: %v", ct, err)
		}
		if len(list) != 1 {
			t.Errorf("ListByType(%q) length = %d, want 1", ct, len(list))
		}
	}
}

// ---------------------------------------------------------------------------
// Test (credential self-test) tests
// ---------------------------------------------------------------------------

func TestStore_Test_Generic_Valid(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "test-generic", "some-value", "admin")

	result, err := store.Test(ctx, "test-generic")
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !result.Valid {
		t.Errorf("expected Valid=true, got Valid=false, Error=%v", result.Error)
	}
}

func TestStore_Test_Webhook_Valid(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateWebhook(t, store, "test-webhook", "https://hooks.example.com/test", "admin")

	result, err := store.Test(ctx, "test-webhook")
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !result.Valid {
		t.Errorf("expected Valid=true, got Valid=false, Error=%v", result.Error)
	}
}

func TestStore_Test_NotFound(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.Test(ctx, "nonexistent")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("expected ErrCredentialNotFound, got %v", err)
	}
}

func TestStore_Test_NoEncryptor(t *testing.T) {
	store := testStoreNoEncryptor(t)
	ctx := context.Background()

	_, err := store.Test(ctx, "anything")
	if !errors.Is(err, ErrEncryptionKeyNotConfigured) {
		t.Fatalf("expected ErrEncryptionKeyNotConfigured, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ReferencedBy tests
// ---------------------------------------------------------------------------

func TestStore_ReferencedBy_NoReferences(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "orphan-cred", "value", "admin")

	refs, err := store.ReferencedBy(ctx, "orphan-cred")
	if err != nil {
		t.Fatalf("ReferencedBy failed: %v", err)
	}

	if len(refs) != 0 {
		t.Errorf("expected 0 references, got %d", len(refs))
	}
}

func TestStore_ReferencedBy_SingleReference(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "ref-cred", "value", "admin")
	store.AddOrgReference("ref-cred", "production")

	refs, err := store.ReferencedBy(ctx, "ref-cred")
	if err != nil {
		t.Fatalf("ReferencedBy failed: %v", err)
	}

	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(refs))
	}
	if refs[0].EntityType != "organisation" {
		t.Errorf("EntityType = %q, want %q", refs[0].EntityType, "organisation")
	}
	if refs[0].EntityName != "production" {
		t.Errorf("EntityName = %q, want %q", refs[0].EntityName, "production")
	}
}

func TestStore_ReferencedBy_MultipleReferences(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "multi-ref", "value", "admin")
	store.AddOrgReference("multi-ref", "org-a")
	store.AddOrgReference("multi-ref", "org-b")
	store.AddOrgReference("multi-ref", "org-c")

	refs, err := store.ReferencedBy(ctx, "multi-ref")
	if err != nil {
		t.Fatalf("ReferencedBy failed: %v", err)
	}

	if len(refs) != 3 {
		t.Fatalf("expected 3 references, got %d", len(refs))
	}
}

func TestStore_ReferencedBy_NotFound(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.ReferencedBy(ctx, "nonexistent")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("expected ErrCredentialNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Encryption isolation tests
// ---------------------------------------------------------------------------

func TestStore_Create_EncryptedValueDiffersFromPlaintext(t *testing.T) {
	store := testStore(t)

	plaintext := "this-is-my-secret"
	mustCreateGeneric(t, store, "enc-test", plaintext, "admin")

	// Access the internal encrypted value to verify it's not plaintext.
	store.mu.Lock()
	cred := store.credentials["enc-test"]
	encrypted := cred.encryptedValue
	store.mu.Unlock()

	if encrypted == plaintext {
		t.Error("encrypted value should differ from plaintext")
	}
	if strings.Contains(encrypted, plaintext) {
		t.Error("encrypted value should not contain plaintext")
	}
	// Should be in <nonce_hex>:<ciphertext_hex> format.
	if !strings.Contains(encrypted, ":") {
		t.Error("encrypted value should contain ':' separator")
	}
}

func TestStore_Create_SamePlaintextDifferentCiphertext(t *testing.T) {
	store := testStore(t)

	// Create two credentials with the same value but different names.
	mustCreateGeneric(t, store, "same-val-1", "identical-secret", "admin")
	mustCreateGeneric(t, store, "same-val-2", "identical-secret", "admin")

	store.mu.Lock()
	enc1 := store.credentials["same-val-1"].encryptedValue
	enc2 := store.credentials["same-val-2"].encryptedValue
	store.mu.Unlock()

	if enc1 == enc2 {
		t.Error("identical plaintext should produce different ciphertext (unique nonce)")
	}
}

func TestStore_Get_WrongEncryptorFails(t *testing.T) {
	// Create with one key, try to Get with another — should fail decryption.
	enc1 := testEncryptor(t)
	enc2 := testEncryptorAlt(t)

	store1 := NewInMemoryCredentialStore(enc1)
	mustCreateGeneric(t, store1, "cross-key", "secret", "admin")

	// Transplant the credential into a store using a different encryptor.
	store2 := NewInMemoryCredentialStore(enc2)
	store1.mu.Lock()
	store2.mu.Lock()
	store2.credentials["cross-key"] = store1.credentials["cross-key"]
	store2.mu.Unlock()
	store1.mu.Unlock()

	_, err := store2.Get(context.Background(), "cross-key")
	if err == nil {
		t.Fatal("expected decryption error with wrong key")
	}
	if !strings.Contains(err.Error(), "decrypt") {
		t.Errorf("error should mention decryption, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Metadata preservation tests
// ---------------------------------------------------------------------------

func TestStore_Create_MetadataPreservedOnGet(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "meta-get", "value", "admin-user")

	cred, err := store.Get(ctx, "meta-get")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer ZeroBytes(cred.Plaintext)

	if cred.CreatedBy != "admin-user" {
		t.Errorf("CreatedBy = %q, want %q", cred.CreatedBy, "admin-user")
	}
	if cred.LastRotatedAt != nil {
		t.Error("LastRotatedAt should be nil before rotation")
	}
	if cred.UpdatedBy != "" {
		t.Errorf("UpdatedBy should be empty before rotation, got %q", cred.UpdatedBy)
	}
}

func TestStore_Update_MetadataUpdatedCorrectly(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	createMeta := mustCreateGeneric(t, store, "meta-update", "v1", "creator")
	originalCreatedAt := createMeta.CreatedAt

	// Small sleep to ensure timestamps differ.
	time.Sleep(time.Millisecond)

	updateMeta, err := store.Update(ctx, UpdateCredentialInput{
		Name:      "meta-update",
		Plaintext: []byte("v2"),
		UpdatedBy: "updater",
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if updateMeta.CreatedBy != "creator" {
		t.Errorf("CreatedBy should not change, got %q", updateMeta.CreatedBy)
	}
	if updateMeta.UpdatedBy != "updater" {
		t.Errorf("UpdatedBy = %q, want %q", updateMeta.UpdatedBy, "updater")
	}
	if updateMeta.LastRotatedAt == nil {
		t.Error("LastRotatedAt should be set after update")
	}
	if updateMeta.CreatedAt.Before(originalCreatedAt) {
		t.Error("CreatedAt should not change")
	}
	if !updateMeta.UpdatedAt.After(originalCreatedAt) && !updateMeta.UpdatedAt.Equal(originalCreatedAt) {
		t.Error("UpdatedAt should be >= original CreatedAt")
	}
}

// ---------------------------------------------------------------------------
// Context cancellation tests
// ---------------------------------------------------------------------------

func TestStore_Create_CancelledContext(t *testing.T) {
	store := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	// The in-memory store doesn't check context, but this verifies the
	// interface accepts a context. Real DB implementation would fail.
	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "ctx-test",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      []byte("value"),
		CreatedBy:      "admin",
	})
	// In-memory store doesn't check ctx, so this should succeed.
	// This test documents the interface contract.
	if err != nil {
		t.Logf("Create with cancelled context: %v (may be expected for DB-backed stores)", err)
	}
}

// ---------------------------------------------------------------------------
// Concurrent access tests
// ---------------------------------------------------------------------------

func TestStore_ConcurrentCreates(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := store.Create(ctx, CreateCredentialInput{
				Name:           fmt.Sprintf("concurrent-%d", idx),
				CredentialType: CredentialTypeGeneric,
				Plaintext:      []byte(fmt.Sprintf("value-%d", idx)),
				CreatedBy:      "admin",
			})
			errs[idx] = err
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent Create %d failed: %v", i, err)
		}
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 50 {
		t.Errorf("List length = %d, want 50", len(list))
	}
}

func TestStore_ConcurrentReadsAndWrites(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "rw-target", "initial", "admin")

	var wg sync.WaitGroup

	// 20 readers.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cred, err := store.Get(ctx, "rw-target")
			if err != nil {
				t.Errorf("concurrent Get failed: %v", err)
				return
			}
			ZeroBytes(cred.Plaintext)
		}()
	}

	// 10 writers.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := store.Update(ctx, UpdateCredentialInput{
				Name:      "rw-target",
				Plaintext: []byte(fmt.Sprintf("updated-%d", idx)),
				UpdatedBy: fmt.Sprintf("writer-%d", idx),
			})
			if err != nil {
				t.Errorf("concurrent Update %d failed: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	// Final value should be one of the written values.
	cred, err := store.Get(ctx, "rw-target")
	if err != nil {
		t.Fatalf("final Get failed: %v", err)
	}
	defer ZeroBytes(cred.Plaintext)

	if !strings.HasPrefix(string(cred.Plaintext), "updated-") {
		t.Errorf("unexpected final value: %q", string(cred.Plaintext))
	}
}

// ---------------------------------------------------------------------------
// Interface contract tests — verify compile-time interface compliance
// ---------------------------------------------------------------------------

func TestCredentialStore_InterfaceSatisfied_InMemory(t *testing.T) {
	var _ CredentialStore = (*InMemoryCredentialStore)(nil)
}

func TestCredentialStore_InterfaceSatisfied_DB(t *testing.T) {
	var _ CredentialStore = (*DBCredentialStore)(nil)
}

// ---------------------------------------------------------------------------
// Edge case: create → update → get → delete → list full lifecycle
// ---------------------------------------------------------------------------

func TestStore_FullLifecycle(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	// 1. Create
	createMeta, err := store.Create(ctx, CreateCredentialInput{
		Name:           "lifecycle-cred",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      []byte("version-1"),
		CreatedBy:      "creator",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if createMeta.Name != "lifecycle-cred" {
		t.Errorf("Create name = %q", createMeta.Name)
	}

	// 2. List — should see it
	list, _ := store.List(ctx)
	if len(list) != 1 {
		t.Fatalf("List after create: length = %d, want 1", len(list))
	}

	// 3. Get — verify plaintext
	cred, err := store.Get(ctx, "lifecycle-cred")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(cred.Plaintext) != "version-1" {
		t.Errorf("Get plaintext = %q, want %q", string(cred.Plaintext), "version-1")
	}
	ZeroBytes(cred.Plaintext)

	// 4. GetMetadata — verify no decryption needed
	meta, err := store.GetMetadata(ctx, "lifecycle-cred")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if meta.CreatedBy != "creator" {
		t.Errorf("GetMetadata CreatedBy = %q", meta.CreatedBy)
	}

	// 5. Update
	updateMeta, err := store.Update(ctx, UpdateCredentialInput{
		Name:      "lifecycle-cred",
		Plaintext: []byte("version-2"),
		UpdatedBy: "rotator",
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updateMeta.UpdatedBy != "rotator" {
		t.Errorf("Update UpdatedBy = %q", updateMeta.UpdatedBy)
	}
	if updateMeta.LastRotatedAt == nil {
		t.Error("Update should set LastRotatedAt")
	}

	// 6. Get — verify updated plaintext
	cred, err = store.Get(ctx, "lifecycle-cred")
	if err != nil {
		t.Fatalf("Get after update failed: %v", err)
	}
	if string(cred.Plaintext) != "version-2" {
		t.Errorf("Get after update = %q, want %q", string(cred.Plaintext), "version-2")
	}
	ZeroBytes(cred.Plaintext)

	// 7. Test
	result, err := store.Test(ctx, "lifecycle-cred")
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !result.Valid {
		t.Error("Test should be valid for generic credential")
	}

	// 8. ReferencedBy — no references
	refs, err := store.ReferencedBy(ctx, "lifecycle-cred")
	if err != nil {
		t.Fatalf("ReferencedBy failed: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected 0 references, got %d", len(refs))
	}

	// 9. Delete
	if err := store.Delete(ctx, "lifecycle-cred"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 10. List — should be empty
	list, _ = store.List(ctx)
	if len(list) != 0 {
		t.Fatalf("List after delete: length = %d, want 0", len(list))
	}

	// 11. Get — should be not found
	_, err = store.Get(ctx, "lifecycle-cred")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("Get after delete: expected ErrCredentialNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Sentinel error identity tests
// ---------------------------------------------------------------------------

func TestSentinelErrors_AreDistinct(t *testing.T) {
	sentinels := []error{
		ErrCredentialNotFound,
		ErrCredentialAlreadyExists,
		ErrCredentialInUse,
		ErrEncryptionKeyNotConfigured,
		ErrMasterKeyRequired,
		ErrMasterKeyTooShort,
		ErrMasterKeyInvalidBase64,
		ErrInvalidCiphertext,
		ErrDecryptionFailed,
		ErrAADRequired,
		ErrInvalidCredentialType,
		ErrEmptyValue,
		ErrInvalidPEMKey,
		ErrInvalidWebhookURL,
	}

	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			if errors.Is(sentinels[i], sentinels[j]) {
				t.Errorf("sentinel errors should be distinct: %v == %v", sentinels[i], sentinels[j])
			}
		}
	}
}

func TestSentinelErrors_HaveDescriptiveMessages(t *testing.T) {
	sentinels := map[string]error{
		"ErrCredentialNotFound":         ErrCredentialNotFound,
		"ErrCredentialAlreadyExists":    ErrCredentialAlreadyExists,
		"ErrCredentialInUse":            ErrCredentialInUse,
		"ErrEncryptionKeyNotConfigured": ErrEncryptionKeyNotConfigured,
	}

	for name, err := range sentinels {
		msg := err.Error()
		if msg == "" {
			t.Errorf("%s has empty error message", name)
		}
		if !strings.HasPrefix(msg, "secrets:") {
			t.Errorf("%s message should start with 'secrets:', got %q", name, msg)
		}
	}
}

// ---------------------------------------------------------------------------
// DBCredentialStore constructor tests (no real DB needed)
// ---------------------------------------------------------------------------

func TestNewDBCredentialStore_NilEncryptor(t *testing.T) {
	// Should not panic — nil encryptor is valid (operations fail gracefully).
	store := NewDBCredentialStore(nil, nil)
	if store == nil {
		t.Fatal("NewDBCredentialStore should not return nil")
	}
}

func TestNewDBCredentialStore_WithEncryptor(t *testing.T) {
	enc := testEncryptor(t)
	store := NewDBCredentialStore(nil, enc)
	if store == nil {
		t.Fatal("NewDBCredentialStore should not return nil")
	}
	if store.encryptor != enc {
		t.Error("encryptor should be stored")
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestNullableJSONB_Nil(t *testing.T) {
	result := nullableJSONB(nil)
	if result != nil {
		t.Errorf("nullableJSONB(nil) = %v, want nil", result)
	}
}

func TestNullableJSONB_Empty(t *testing.T) {
	result := nullableJSONB([]byte{})
	if result != nil {
		t.Errorf("nullableJSONB(empty) = %v, want nil", result)
	}
}

func TestNullableJSONB_NonEmpty(t *testing.T) {
	data := []byte(`{"key": "value"}`)
	result := nullableJSONB(data)
	if result == nil {
		t.Fatal("nullableJSONB(non-empty) should not be nil")
	}
	if b, ok := result.([]byte); !ok || string(b) != `{"key": "value"}` {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestParseJSONBMetadata_Nil(t *testing.T) {
	m := parseJSONBMetadata(nil)
	if m != nil {
		t.Errorf("parseJSONBMetadata(nil) = %v, want nil", m)
	}
}

func TestParseJSONBMetadata_Empty(t *testing.T) {
	m := parseJSONBMetadata([]byte{})
	if m != nil {
		t.Errorf("parseJSONBMetadata(empty) = %v, want nil", m)
	}
}

func TestParseJSONBMetadata_ValidJSON(t *testing.T) {
	data := []byte(`{"key_format": "pkcs1", "bits": 2048}`)
	m := parseJSONBMetadata(data)
	if m == nil {
		t.Fatal("parseJSONBMetadata should return a map")
	}
	if m["key_format"] != "pkcs1" {
		t.Errorf("key_format = %v, want %q", m["key_format"], "pkcs1")
	}
	// JSON numbers decode as float64.
	if bits, ok := m["bits"].(float64); !ok || bits != 2048 {
		t.Errorf("bits = %v, want 2048", m["bits"])
	}
}

func TestParseJSONBMetadata_InvalidJSON(t *testing.T) {
	data := []byte(`{not valid json}`)
	m := parseJSONBMetadata(data)
	if m != nil {
		t.Errorf("parseJSONBMetadata(invalid) = %v, want nil", m)
	}
}

func TestIsUniqueViolation_Nil(t *testing.T) {
	if isUniqueViolation(nil) {
		t.Error("nil error should not be a unique violation")
	}
}

func TestIsUniqueViolation_PostgresCode(t *testing.T) {
	err := fmt.Errorf("ERROR: duplicate key value violates unique constraint \"uq_credentials_name\" (SQLSTATE 23505)")
	if !isUniqueViolation(err) {
		t.Error("should detect 23505 as unique violation")
	}
}

func TestIsUniqueViolation_UniqueConstraintText(t *testing.T) {
	err := fmt.Errorf("pq: duplicate key value violates unique constraint")
	if !isUniqueViolation(err) {
		t.Error("should detect 'unique constraint' text")
	}
}

func TestIsUniqueViolation_UnrelatedError(t *testing.T) {
	err := fmt.Errorf("connection refused")
	if isUniqueViolation(err) {
		t.Error("unrelated error should not be a unique violation")
	}
}

func TestContainsCI(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"Hello World", "hello", true},
		{"Hello World", "WORLD", true},
		{"Hello World", "xyz", false},
		{"", "a", false},
		{"a", "", true},
		{"23505", "23505", true},
		{"UNIQUE CONSTRAINT", "unique constraint", true},
	}

	for _, tt := range tests {
		got := containsCI(tt.s, tt.substr)
		if got != tt.want {
			t.Errorf("containsCI(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}

func TestToLowerASCII(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ABC", "abc"},
		{"abc", "abc"},
		{"Hello World 123!", "hello world 123!"},
		{"", ""},
		{"MiXeD", "mixed"},
	}

	for _, tt := range tests {
		got := toLowerASCII(tt.input)
		if got != tt.want {
			t.Errorf("toLowerASCII(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// sortMetadataByName tests
// ---------------------------------------------------------------------------

func TestSortMetadataByName_Empty(t *testing.T) {
	var s []CredentialMetadata
	sortMetadataByName(s) // should not panic
}

func TestSortMetadataByName_Single(t *testing.T) {
	s := []CredentialMetadata{{Name: "only"}}
	sortMetadataByName(s)
	if s[0].Name != "only" {
		t.Errorf("single element changed")
	}
}

func TestSortMetadataByName_AlreadySorted(t *testing.T) {
	s := []CredentialMetadata{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	sortMetadataByName(s)
	if s[0].Name != "a" || s[1].Name != "b" || s[2].Name != "c" {
		t.Error("already sorted list was reordered")
	}
}

func TestSortMetadataByName_Reversed(t *testing.T) {
	s := []CredentialMetadata{{Name: "c"}, {Name: "b"}, {Name: "a"}}
	sortMetadataByName(s)
	if s[0].Name != "a" || s[1].Name != "b" || s[2].Name != "c" {
		t.Errorf("got %q %q %q, want a b c", s[0].Name, s[1].Name, s[2].Name)
	}
}

func TestSortMetadataByName_Duplicates(t *testing.T) {
	s := []CredentialMetadata{{Name: "b"}, {Name: "a"}, {Name: "b"}}
	sortMetadataByName(s)
	if s[0].Name != "a" || s[1].Name != "b" || s[2].Name != "b" {
		t.Errorf("got %q %q %q", s[0].Name, s[1].Name, s[2].Name)
	}
}

// ---------------------------------------------------------------------------
// Type assertion tests for CredentialMetadata / Credential
// ---------------------------------------------------------------------------

func TestCredential_ZeroPlaintextAfterUse(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "zero-test", "sensitive-data", "admin")

	cred, err := store.Get(ctx, "zero-test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Verify plaintext is present.
	if len(cred.Plaintext) == 0 {
		t.Fatal("Plaintext should not be empty")
	}

	// Zero it.
	ZeroBytes(cred.Plaintext)

	// Verify it's zeroed.
	if !IsZeroed(cred.Plaintext) {
		t.Error("Plaintext should be zeroed after ZeroBytes")
	}
}

func TestCredentialMetadata_NeverContainsPlaintext(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mustCreateGeneric(t, store, "no-plain-meta", "secret-value-123", "admin")

	meta, err := store.GetMetadata(ctx, "no-plain-meta")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}

	// CredentialMetadata struct has no Plaintext field (compile-time check).
	// Verify the struct doesn't accidentally expose the secret via Metadata.
	if meta.Metadata != nil {
		for k, v := range meta.Metadata {
			if str, ok := v.(string); ok && str == "secret-value-123" {
				t.Errorf("Metadata key %q contains plaintext!", k)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// CreateCredentialInput / UpdateCredentialInput struct tests
// ---------------------------------------------------------------------------

func TestCreateCredentialInput_AllFieldsUsed(t *testing.T) {
	input := CreateCredentialInput{
		Name:           "test",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      []byte("value"),
		CreatedBy:      "admin",
	}

	// This is a compile-time contract test — if fields are added to the
	// struct, this test must be updated.
	if input.Name == "" || input.CredentialType == "" || input.Plaintext == nil || input.CreatedBy == "" {
		t.Error("all fields should be populated")
	}
}

func TestUpdateCredentialInput_AllFieldsUsed(t *testing.T) {
	input := UpdateCredentialInput{
		Name:      "test",
		Plaintext: []byte("value"),
		UpdatedBy: "admin",
	}

	if input.Name == "" || input.Plaintext == nil || input.UpdatedBy == "" {
		t.Error("all fields should be populated")
	}
}

// ---------------------------------------------------------------------------
// CredentialReference struct tests
// ---------------------------------------------------------------------------

func TestCredentialReference_Fields(t *testing.T) {
	ref := CredentialReference{
		EntityType: "organisation",
		EntityName: "prod-org",
	}

	if ref.EntityType != "organisation" {
		t.Errorf("EntityType = %q", ref.EntityType)
	}
	if ref.EntityName != "prod-org" {
		t.Errorf("EntityName = %q", ref.EntityName)
	}
}

// ---------------------------------------------------------------------------
// Multiple credential types in same store
// ---------------------------------------------------------------------------

func TestStore_MultipleTypes_IndependentStorage(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	// Create one of each type (except chef_client_key which needs a real PEM).
	mustCreateGeneric(t, store, "generic-1", "gen-value", "admin")
	mustCreateWebhook(t, store, "webhook-1", "https://example.com/hook", "admin")

	store.Create(ctx, CreateCredentialInput{
		Name: "ldap-1", CredentialType: CredentialTypeLDAPBindPassword,
		Plaintext: []byte("ldap-pass"), CreatedBy: "admin",
	})
	store.Create(ctx, CreateCredentialInput{
		Name: "smtp-1", CredentialType: CredentialTypeSMTPPassword,
		Plaintext: []byte("smtp-pass"), CreatedBy: "admin",
	})

	// Verify each can be retrieved independently.
	cred, _ := store.Get(ctx, "generic-1")
	if string(cred.Plaintext) != "gen-value" {
		t.Errorf("generic value = %q", string(cred.Plaintext))
	}
	ZeroBytes(cred.Plaintext)

	cred, _ = store.Get(ctx, "webhook-1")
	if string(cred.Plaintext) != "https://example.com/hook" {
		t.Errorf("webhook value = %q", string(cred.Plaintext))
	}
	ZeroBytes(cred.Plaintext)

	cred, _ = store.Get(ctx, "ldap-1")
	if string(cred.Plaintext) != "ldap-pass" {
		t.Errorf("ldap value = %q", string(cred.Plaintext))
	}
	ZeroBytes(cred.Plaintext)

	cred, _ = store.Get(ctx, "smtp-1")
	if string(cred.Plaintext) != "smtp-pass" {
		t.Errorf("smtp value = %q", string(cred.Plaintext))
	}
	ZeroBytes(cred.Plaintext)

	// Verify list shows all 4.
	list, _ := store.List(ctx)
	if len(list) != 4 {
		t.Errorf("List length = %d, want 4", len(list))
	}
}

// ---------------------------------------------------------------------------
// Unicode credential names and values
// ---------------------------------------------------------------------------

func TestStore_UnicodeCredentialName(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	_, err := store.Create(ctx, CreateCredentialInput{
		Name:           "日本語-credential",
		CredentialType: CredentialTypeGeneric,
		Plaintext:      []byte("unicode-value-🔑"),
		CreatedBy:      "admin",
	})
	if err != nil {
		t.Fatalf("Create with unicode name failed: %v", err)
	}

	cred, err := store.Get(ctx, "日本語-credential")
	if err != nil {
		t.Fatalf("Get with unicode name failed: %v", err)
	}
	defer ZeroBytes(cred.Plaintext)

	if string(cred.Plaintext) != "unicode-value-🔑" {
		t.Errorf("Plaintext = %q", string(cred.Plaintext))
	}
}
