// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// ---------------------------------------------------------------------------
// mockCredentialStore — in-process test double for secrets.CredentialStore
// ---------------------------------------------------------------------------

type mockCredEntry struct {
	name           string
	credentialType string
	encryptedValue string
	metadata       map[string]any
	lastRotatedAt  *time.Time
	createdBy      string
	updatedBy      string
	createdAt      time.Time
	updatedAt      time.Time
}

type mockCredentialStore struct {
	mu          sync.Mutex
	encryptor   *secrets.Encryptor
	credentials map[string]*mockCredEntry
	orgRefs     map[string][]string
}

func newMockCredentialStore(enc *secrets.Encryptor) *mockCredentialStore {
	return &mockCredentialStore{
		encryptor:   enc,
		credentials: make(map[string]*mockCredEntry),
		orgRefs:     make(map[string][]string),
	}
}

func (s *mockCredentialStore) AddOrgReference(credentialName, orgName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orgRefs[credentialName] = append(s.orgRefs[credentialName], orgName)
}

func (s *mockCredentialStore) Create(_ context.Context, input secrets.CreateCredentialInput) (*secrets.CredentialMetadata, error) {
	if s.encryptor == nil {
		return nil, secrets.ErrEncryptionKeyNotConfigured
	}

	result := secrets.ValidateCredentialValue(input.CredentialType, input.Plaintext)
	if !result.Valid {
		return nil, result.Error
	}

	aad, err := secrets.BuildAAD(input.CredentialType, input.Name)
	if err != nil {
		return nil, err
	}
	encrypted, err := s.encryptor.Encrypt(input.Plaintext, aad)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.credentials[input.Name]; exists {
		return nil, secrets.ErrCredentialAlreadyExists
	}

	now := time.Now().UTC()
	s.credentials[input.Name] = &mockCredEntry{
		name:           input.Name,
		credentialType: input.CredentialType,
		encryptedValue: encrypted,
		metadata:       result.Metadata,
		createdBy:      input.CreatedBy,
		createdAt:      now,
		updatedAt:      now,
	}

	return &secrets.CredentialMetadata{
		Name:           input.Name,
		CredentialType: input.CredentialType,
		Metadata:       result.Metadata,
		CreatedBy:      input.CreatedBy,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func (s *mockCredentialStore) Get(_ context.Context, name string) (*secrets.Credential, error) {
	if s.encryptor == nil {
		return nil, secrets.ErrEncryptionKeyNotConfigured
	}

	s.mu.Lock()
	entry, exists := s.credentials[name]
	if !exists {
		s.mu.Unlock()
		return nil, secrets.ErrCredentialNotFound
	}
	cpy := *entry
	s.mu.Unlock()

	aad, err := secrets.BuildAAD(cpy.credentialType, cpy.name)
	if err != nil {
		return nil, err
	}
	plain, err := s.encryptor.Decrypt(cpy.encryptedValue, aad)
	if err != nil {
		return nil, err
	}

	return &secrets.Credential{
		Name:           cpy.name,
		CredentialType: cpy.credentialType,
		Plaintext:      plain,
		Metadata:       cpy.metadata,
		LastRotatedAt:  cpy.lastRotatedAt,
		CreatedBy:      cpy.createdBy,
		UpdatedBy:      cpy.updatedBy,
		CreatedAt:      cpy.createdAt,
		UpdatedAt:      cpy.updatedAt,
	}, nil
}

func (s *mockCredentialStore) GetMetadata(_ context.Context, name string) (*secrets.CredentialMetadata, error) {
	s.mu.Lock()
	entry, exists := s.credentials[name]
	if !exists {
		s.mu.Unlock()
		return nil, secrets.ErrCredentialNotFound
	}
	cpy := *entry
	s.mu.Unlock()

	return &secrets.CredentialMetadata{
		Name:           cpy.name,
		CredentialType: cpy.credentialType,
		Metadata:       cpy.metadata,
		LastRotatedAt:  cpy.lastRotatedAt,
		CreatedBy:      cpy.createdBy,
		UpdatedBy:      cpy.updatedBy,
		CreatedAt:      cpy.createdAt,
		UpdatedAt:      cpy.updatedAt,
	}, nil
}

func (s *mockCredentialStore) Update(_ context.Context, input secrets.UpdateCredentialInput) (*secrets.CredentialMetadata, error) {
	if s.encryptor == nil {
		return nil, secrets.ErrEncryptionKeyNotConfigured
	}

	s.mu.Lock()
	entry, exists := s.credentials[input.Name]
	if !exists {
		s.mu.Unlock()
		return nil, secrets.ErrCredentialNotFound
	}
	credType := entry.credentialType
	s.mu.Unlock()

	result := secrets.ValidateCredentialValue(credType, input.Plaintext)
	if !result.Valid {
		return nil, result.Error
	}

	aad, err := secrets.BuildAAD(credType, input.Name)
	if err != nil {
		return nil, err
	}
	encrypted, err := s.encryptor.Encrypt(input.Plaintext, aad)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists = s.credentials[input.Name]
	if !exists {
		return nil, secrets.ErrCredentialNotFound
	}

	now := time.Now().UTC()
	entry.encryptedValue = encrypted
	entry.metadata = result.Metadata
	entry.updatedBy = input.UpdatedBy
	entry.updatedAt = now
	entry.lastRotatedAt = &now

	return &secrets.CredentialMetadata{
		Name:           entry.name,
		CredentialType: entry.credentialType,
		Metadata:       entry.metadata,
		LastRotatedAt:  entry.lastRotatedAt,
		CreatedBy:      entry.createdBy,
		UpdatedBy:      entry.updatedBy,
		CreatedAt:      entry.createdAt,
		UpdatedAt:      entry.updatedAt,
	}, nil
}

func (s *mockCredentialStore) Delete(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.credentials[name]; !exists {
		return secrets.ErrCredentialNotFound
	}
	if refs, ok := s.orgRefs[name]; ok && len(refs) > 0 {
		return secrets.ErrCredentialInUse
	}
	delete(s.credentials, name)
	delete(s.orgRefs, name)
	return nil
}

func (s *mockCredentialStore) List(_ context.Context) ([]secrets.CredentialMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]secrets.CredentialMetadata, 0, len(s.credentials))
	for _, e := range s.credentials {
		out = append(out, secrets.CredentialMetadata{
			Name:           e.name,
			CredentialType: e.credentialType,
			Metadata:       e.metadata,
			LastRotatedAt:  e.lastRotatedAt,
			CreatedBy:      e.createdBy,
			UpdatedBy:      e.updatedBy,
			CreatedAt:      e.createdAt,
			UpdatedAt:      e.updatedAt,
		})
	}
	return out, nil
}

func (s *mockCredentialStore) ListByType(_ context.Context, credentialType string) ([]secrets.CredentialMetadata, error) {
	if !secrets.IsValidCredentialType(credentialType) {
		return nil, fmt.Errorf("%w: %q", secrets.ErrInvalidCredentialType, credentialType)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var out []secrets.CredentialMetadata
	for _, e := range s.credentials {
		if e.credentialType != credentialType {
			continue
		}
		out = append(out, secrets.CredentialMetadata{
			Name:           e.name,
			CredentialType: e.credentialType,
			Metadata:       e.metadata,
			LastRotatedAt:  e.lastRotatedAt,
			CreatedBy:      e.createdBy,
			UpdatedBy:      e.updatedBy,
			CreatedAt:      e.createdAt,
			UpdatedAt:      e.updatedAt,
		})
	}
	return out, nil
}

func (s *mockCredentialStore) Test(ctx context.Context, name string) (*secrets.ValidationResult, error) {
	cred, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	defer secrets.ZeroBytes(cred.Plaintext)
	result := secrets.ValidateCredentialValue(cred.CredentialType, cred.Plaintext)
	return &result, nil
}

func (s *mockCredentialStore) ReferencedBy(_ context.Context, name string) ([]secrets.CredentialReference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.credentials[name]; !exists {
		return nil, secrets.ErrCredentialNotFound
	}

	refs, ok := s.orgRefs[name]
	if !ok || len(refs) == 0 {
		return []secrets.CredentialReference{}, nil
	}
	out := make([]secrets.CredentialReference, 0, len(refs))
	for _, orgName := range refs {
		out = append(out, secrets.CredentialReference{
			EntityType: "organisation",
			EntityName: orgName,
		})
	}
	return out, nil
}

// compile-time check
var _ secrets.CredentialStore = (*mockCredentialStore)(nil)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func testCredentialEncryptor(t *testing.T) *secrets.Encryptor {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	enc, err := secrets.NewEncryptor(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func newTestRouterWithCredentials(credStore secrets.CredentialStore) *Router {
	store := &mockStore{}
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	return NewRouter(store, cfg, hub, WithCredentialStore(credStore))
}

func mustCreateCredential(t *testing.T, cs *mockCredentialStore, name, credType, value string) {
	t.Helper()
	_, err := cs.Create(context.Background(), secrets.CreateCredentialInput{
		Name:           name,
		CredentialType: credType,
		Plaintext:      []byte(value),
		CreatedBy:      "test-admin",
	})
	if err != nil {
		t.Fatalf("mustCreateCredential(%q): %v", name, err)
	}
}

// ---------------------------------------------------------------------------
// 503 when credential store is nil
// ---------------------------------------------------------------------------

func TestCredentials_503_WhenStoreNotConfigured(t *testing.T) {
	store := &mockStore{}
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(store, cfg, hub) // no WithCredentialStore

	methods := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/admin/credentials"},
		{http.MethodPost, "/api/v1/admin/credentials"},
		{http.MethodPut, "/api/v1/admin/credentials/some-cred"},
		{http.MethodDelete, "/api/v1/admin/credentials/some-cred?confirm=true"},
		{http.MethodPost, "/api/v1/admin/credentials/some-cred/test"},
	}

	for _, tc := range methods {
		t.Run(tc.method+"_"+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			r.ServeHTTP(w, req)
			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
			}
			var body map[string]any
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if body["error"] != ErrCodeServiceUnavailable {
				t.Errorf("error code = %q, want %q", body["error"], ErrCodeServiceUnavailable)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/credentials — list
// ---------------------------------------------------------------------------

func TestCredentials_List_Empty(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/credentials", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data       []any `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 0 {
		t.Errorf("data length = %d, want 0", len(body.Data))
	}
	if body.Pagination.TotalItems != 0 {
		t.Errorf("total_items = %d, want 0", body.Pagination.TotalItems)
	}
}

func TestCredentials_List_WithCredentials(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "cred-a", secrets.CredentialTypeGeneric, "secret-a")
	mustCreateCredential(t, cs, "cred-b", secrets.CredentialTypeGeneric, "secret-b")
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/credentials", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data []struct {
			Name           string `json:"name"`
			CredentialType string `json:"credential_type"`
			CreatedBy      string `json:"created_by"`
		} `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Pagination.TotalItems != 2 {
		t.Errorf("total_items = %d, want 2", body.Pagination.TotalItems)
	}
	if len(body.Data) != 2 {
		t.Fatalf("data length = %d, want 2", len(body.Data))
	}

	names := map[string]bool{}
	for _, d := range body.Data {
		names[d.Name] = true
	}
	if !names["cred-a"] {
		t.Error("missing cred-a in list response")
	}
	if !names["cred-b"] {
		t.Error("missing cred-b in list response")
	}
}

func TestCredentials_List_FilterByType(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "gen-1", secrets.CredentialTypeGeneric, "val1")
	mustCreateCredential(t, cs, "gen-2", secrets.CredentialTypeGeneric, "val2")
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/credentials?type=generic", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data []struct {
			Name           string `json:"name"`
			CredentialType string `json:"credential_type"`
		} `json:"data"`
		Pagination struct {
			TotalItems int `json:"total_items"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Pagination.TotalItems != 2 {
		t.Errorf("total_items = %d, want 2", body.Pagination.TotalItems)
	}
	if len(body.Data) != 2 {
		t.Fatalf("data length = %d, want 2", len(body.Data))
	}
	for _, d := range body.Data {
		if d.CredentialType != secrets.CredentialTypeGeneric {
			t.Errorf("credential_type = %q, want %q", d.CredentialType, secrets.CredentialTypeGeneric)
		}
	}
}

func TestCredentials_List_FilterByType_NoMatch(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "gen-only", secrets.CredentialTypeGeneric, "val")
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/credentials?type=chef_client_key", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Data []any `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data) != 0 {
		t.Errorf("data length = %d, want 0", len(body.Data))
	}
}

func TestCredentials_List_InvalidType(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/credentials?type=bogus_type", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}

	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != ErrCodeValidationError {
		t.Errorf("error = %q, want %q", body.Error, ErrCodeValidationError)
	}
	if !strings.Contains(body.Message, "bogus_type") {
		t.Errorf("message should mention the invalid type, got %q", body.Message)
	}
}

func TestCredentials_List_NeverReturnsEncryptedValue(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "secret-cred", secrets.CredentialTypeGeneric, "super-secret-value")
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/credentials", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var raw map[string]any
	if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rawJSON, _ := json.Marshal(raw)
	jsonStr := string(rawJSON)

	if strings.Contains(jsonStr, "encrypted_value") {
		t.Error("response contains 'encrypted_value' key — credential values must never be returned")
	}
	if strings.Contains(jsonStr, "plaintext") {
		t.Error("response contains 'plaintext' key — credential values must never be returned")
	}
	if strings.Contains(jsonStr, "super-secret-value") {
		t.Error("response contains the plaintext secret value")
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/admin/credentials — create
// ---------------------------------------------------------------------------

func TestCredentials_Create_Success_Generic(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	payload := `{"name":"test-cred","credential_type":"generic","value":"my-secret"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/credentials", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var body struct {
		Name           string `json:"name"`
		CredentialType string `json:"credential_type"`
		CreatedAt      string `json:"created_at"`
		UpdatedAt      string `json:"updated_at"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "test-cred" {
		t.Errorf("name = %q, want %q", body.Name, "test-cred")
	}
	if body.CredentialType != secrets.CredentialTypeGeneric {
		t.Errorf("credential_type = %q, want %q", body.CredentialType, secrets.CredentialTypeGeneric)
	}
	if body.CreatedAt == "" {
		t.Error("created_at should not be empty")
	}
	if body.UpdatedAt == "" {
		t.Error("updated_at should not be empty")
	}
}

// A refused database connection has to say what shape it saw, on screen.
//
// The value is encrypted at rest and never logged, so when somebody was refused,
// neither they nor we could tell which rule had fired without decrypting the
// credential — and this customer is reachable through a VDI and a screenshot.
// The shape is the diagnosis, so it has to survive all the way into the
// response body, and it must carry nothing from the value itself.
func TestCredentials_Create_RefusedDatabaseURLSaysWhatShapeItSaw(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	const (
		user = "svcaccount"
		pass = "hunter2"
		host = "dbserver01"
	)
	// A real shape: a DBA appending options with semicolons, and no database.
	dsn := "sqlserver://" + user + ":" + pass + "@" + host + ":1433;ApplicationIntent=ReadOnly"

	payload, err := json.Marshal(map[string]string{
		"name":            "cmdb-connection",
		"credential_type": secrets.CredentialTypeDatabaseURL,
		"value":           dsn,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/credentials", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	body := w.Body.String()
	for _, want := range []string{"shape:", "semicolons", "no database named"} {
		if !strings.Contains(body, want) {
			t.Errorf("the refusal reaching the screen does not say %q\n  body: %s", want, body)
		}
	}
	// The same response must not carry any part of the connection string.
	for _, secret := range []string{user, pass, host} {
		if strings.Contains(body, secret) {
			t.Errorf("the refusal reaching the screen carries %q from the value\n  body: %s", secret, body)
		}
	}
}

func TestCredentials_Create_DuplicateName(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "dup-cred", secrets.CredentialTypeGeneric, "first")
	r := newTestRouterWithCredentials(cs)

	payload := `{"name":"dup-cred","credential_type":"generic","value":"second"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/credentials", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
}

func TestCredentials_Create_MissingName(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	payload := `{"credential_type":"generic","value":"val"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/credentials", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCredentials_Create_MissingType(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	payload := `{"name":"no-type","value":"val"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/credentials", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCredentials_Create_MissingValue(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	payload := `{"name":"no-val","credential_type":"generic"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/credentials", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCredentials_Create_InvalidType(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	payload := `{"name":"bad-type","credential_type":"not_a_real_type","value":"val"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/credentials", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != ErrCodeValidationError {
		t.Errorf("error = %q, want %q", body.Error, ErrCodeValidationError)
	}
}

func TestCredentials_Create_InvalidJSON(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/credentials", bytes.NewBufferString(`{bad json`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCredentials_Create_NeverReturnsPlaintext(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	secretValue := "ultra-secret-password-12345"
	payload := `{"name":"leak-check","credential_type":"generic","value":"` + secretValue + `"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/credentials", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var raw map[string]any
	if err := json.NewDecoder(w.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rawJSON, _ := json.Marshal(raw)
	jsonStr := string(rawJSON)

	if strings.Contains(jsonStr, secretValue) {
		t.Error("response contains the plaintext secret value")
	}
	if _, ok := raw["value"]; ok {
		t.Error("response contains 'value' key — plaintext must never be returned")
	}
	if _, ok := raw["encrypted_value"]; ok {
		t.Error("response contains 'encrypted_value' key")
	}
	if _, ok := raw["plaintext"]; ok {
		t.Error("response contains 'plaintext' key")
	}
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/credentials/:name — update/rotate
// ---------------------------------------------------------------------------

func TestCredentials_Update_Success(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "rotate-me", secrets.CredentialTypeGeneric, "old-value")
	r := newTestRouterWithCredentials(cs)

	payload := `{"value":"new-value"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/credentials/rotate-me", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		Name           string  `json:"name"`
		CredentialType string  `json:"credential_type"`
		UpdatedAt      string  `json:"updated_at"`
		LastRotatedAt  *string `json:"last_rotated_at"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Name != "rotate-me" {
		t.Errorf("name = %q, want %q", body.Name, "rotate-me")
	}
	if body.CredentialType != secrets.CredentialTypeGeneric {
		t.Errorf("credential_type = %q, want %q", body.CredentialType, secrets.CredentialTypeGeneric)
	}
	if body.UpdatedAt == "" {
		t.Error("updated_at should not be empty after rotation")
	}
	if body.LastRotatedAt == nil || *body.LastRotatedAt == "" {
		t.Error("last_rotated_at should be set after rotation")
	}
}

func TestCredentials_Update_NotFound(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	payload := `{"value":"new-value"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/credentials/nonexistent", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestCredentials_Update_MissingValue(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "need-value", secrets.CredentialTypeGeneric, "original")
	r := newTestRouterWithCredentials(cs)

	payload := `{}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/credentials/need-value", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/admin/credentials/:name
// ---------------------------------------------------------------------------

func TestCredentials_Delete_Success(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "delete-me", secrets.CredentialTypeGeneric, "doomed")
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/credentials/delete-me?confirm=true", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}

	// Verify credential is actually gone.
	_, err := cs.GetMetadata(context.Background(), "delete-me")
	if err == nil {
		t.Error("expected credential to be deleted, but GetMetadata succeeded")
	}
}

func TestCredentials_Delete_NotFound(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/credentials/ghost?confirm=true", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestCredentials_Delete_MissingConfirm(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "keep-me", secrets.CredentialTypeGeneric, "safe")
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/credentials/keep-me", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(body.Message, "confirm=true") {
		t.Errorf("message should mention confirm=true, got %q", body.Message)
	}

	// Verify credential still exists.
	_, err := cs.GetMetadata(context.Background(), "keep-me")
	if err != nil {
		t.Errorf("credential should still exist: %v", err)
	}
}

func TestCredentials_Delete_InUse(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "in-use-cred", secrets.CredentialTypeGeneric, "referenced")
	cs.AddOrgReference("in-use-cred", "my-org")
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/credentials/in-use-cred?confirm=true", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}

	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "conflict" {
		t.Errorf("error = %q, want %q", body.Error, "conflict")
	}
	if !strings.Contains(body.Message, "in-use-cred") {
		t.Errorf("message should mention credential name, got %q", body.Message)
	}

	// Verify credential still exists.
	_, err := cs.GetMetadata(context.Background(), "in-use-cred")
	if err != nil {
		t.Errorf("credential should still exist after failed delete: %v", err)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/admin/credentials/:name/test
// ---------------------------------------------------------------------------

func TestCredentials_Test_Success_Generic(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "test-generic", secrets.CredentialTypeGeneric, "test-value")
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/credentials/test-generic/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		Valid bool `json:"valid"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Valid {
		t.Error("expected valid=true for generic credential test")
	}
}

func TestCredentials_Test_NotFound(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/credentials/no-such-cred/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestCredentials_Test_MethodNotAllowed(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, cs, "test-method", secrets.CredentialTypeGeneric, "val")
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/credentials/test-method/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusMethodNotAllowed, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Method not allowed
// ---------------------------------------------------------------------------

func TestCredentials_MethodNotAllowed_Collection(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/credentials", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusMethodNotAllowed, w.Body.String())
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != ErrCodeMethodNotAllowed {
		t.Errorf("error = %q, want %q", body.Error, ErrCodeMethodNotAllowed)
	}
}

func TestCredentials_MethodNotAllowed_Item(t *testing.T) {
	cs := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(cs)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/credentials/some-cred", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusMethodNotAllowed, w.Body.String())
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != ErrCodeMethodNotAllowed {
		t.Errorf("error = %q, want %q", body.Error, ErrCodeMethodNotAllowed)
	}
}
