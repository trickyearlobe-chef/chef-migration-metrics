// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// ---------------------------------------------------------------------------
// Mock credential store
// ---------------------------------------------------------------------------

type mockCredentialStore struct {
	creds map[string][]byte
}

func (m *mockCredentialStore) Get(_ context.Context, name string) (*secrets.Credential, error) {
	if p, ok := m.creds[name]; ok {
		cp := make([]byte, len(p))
		copy(cp, p)
		return &secrets.Credential{
			Name:           name,
			CredentialType: "generic",
			Plaintext:      cp,
			CreatedBy:      "test",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}, nil
	}
	return nil, secrets.ErrCredentialNotFound
}

func (m *mockCredentialStore) Create(context.Context, secrets.CreateCredentialInput) (*secrets.CredentialMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCredentialStore) GetMetadata(context.Context, string) (*secrets.CredentialMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCredentialStore) Update(context.Context, secrets.UpdateCredentialInput) (*secrets.CredentialMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCredentialStore) Delete(context.Context, string) error {
	return fmt.Errorf("not implemented")
}

func (m *mockCredentialStore) List(context.Context) ([]secrets.CredentialMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCredentialStore) ListByType(context.Context, string) ([]secrets.CredentialMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCredentialStore) Test(context.Context, string) (*secrets.ValidationResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCredentialStore) ReferencedBy(context.Context, string) ([]secrets.CredentialReference, error) {
	return nil, fmt.Errorf("not implemented")
}

// newMockResolver creates a CredentialResolver backed by a mock store with
// the given credential name → plaintext mappings.
func newMockResolver(creds map[string][]byte) *secrets.CredentialResolver {
	return secrets.NewCredentialResolver(&mockCredentialStore{creds: creds})
}

// ---------------------------------------------------------------------------
// ResolveKitchenCredentials tests
// ---------------------------------------------------------------------------

func TestResolveKitchenCredentials_NilResolver(t *testing.T) {
	kc, err := ResolveKitchenCredentials(context.Background(), nil, config.TestKitchenConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kc == nil {
		t.Fatal("expected non-nil KitchenCredentials")
	}
	if len(kc.EnvVars) != 0 {
		t.Errorf("expected empty EnvVars, got %d entries", len(kc.EnvVars))
	}
}

func TestResolveKitchenCredentials_DriverSecrets(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{
		"vcenter_password": []byte("secret-value"),
	})
	tkConfig := config.TestKitchenConfig{
		DriverSecrets: map[string]string{
			"vcenter_password": "vcenter_password",
		},
	}

	kc, err := ResolveKitchenCredentials(context.Background(), resolver, tkConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	envName := "CMM_TK_SECRET_VCENTER_PASSWORD"
	val, ok := kc.EnvVars[envName]
	if !ok {
		t.Fatalf("missing env var %s, got keys: %v", envName, envVarKeys(kc.EnvVars))
	}
	if string(val) != "secret-value" {
		t.Errorf("env var %s = %q, want %q", envName, string(val), "secret-value")
	}
}

func TestResolveKitchenCredentials_TransportPassword(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{
		"ubuntu_pass": []byte("pass-123"),
	})
	tkConfig := config.TestKitchenConfig{
		PlatformMap: []config.PlatformMapEntry{
			{
				KitchenName: "ubuntu-22.04",
				Transport: &config.PlatformMapTransport{
					Username:           "root",
					PasswordCredential: "ubuntu_pass",
				},
			},
		},
	}

	kc, err := ResolveKitchenCredentials(context.Background(), resolver, tkConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	envName := "CMM_TK_TRANSPORT_UBUNTU_22_04"
	val, ok := kc.EnvVars[envName]
	if !ok {
		t.Fatalf("missing env var %s, got keys: %v", envName, envVarKeys(kc.EnvVars))
	}
	if string(val) != "pass-123" {
		t.Errorf("env var %s = %q, want %q", envName, string(val), "pass-123")
	}
}

func TestResolveKitchenCredentials_TransportSSHKey(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{
		"ubuntu_key": []byte("-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----"),
	})
	tkConfig := config.TestKitchenConfig{
		PlatformMap: []config.PlatformMapEntry{
			{
				KitchenName: "ubuntu-22.04",
				Transport: &config.PlatformMapTransport{
					Username:         "root",
					SSHKeyCredential: "ubuntu_key",
				},
			},
		},
	}

	kc, err := ResolveKitchenCredentials(context.Background(), resolver, tkConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	envName := "CMM_TK_KEY_UBUNTU_22_04"
	val, ok := kc.EnvVars[envName]
	if !ok {
		t.Fatalf("missing env var %s, got keys: %v", envName, envVarKeys(kc.EnvVars))
	}
	if !strings.Contains(string(val), "RSA PRIVATE KEY") {
		t.Errorf("env var %s does not contain expected key material", envName)
	}
}

func TestResolveKitchenCredentials_MixedSecrets(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{
		"vc_pass":  []byte("driver-secret"),
		"ssh_pass": []byte("transport-pass"),
		"ssh_key":  []byte("transport-key"),
	})
	tkConfig := config.TestKitchenConfig{
		DriverSecrets: map[string]string{
			"password": "vc_pass",
		},
		PlatformMap: []config.PlatformMapEntry{
			{
				KitchenName: "centos-7",
				Transport: &config.PlatformMapTransport{
					Username:           "root",
					PasswordCredential: "ssh_pass",
					SSHKeyCredential:   "ssh_key",
				},
			},
		},
	}

	kc, err := ResolveKitchenCredentials(context.Background(), resolver, tkConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"CMM_TK_SECRET_PASSWORD":    "driver-secret",
		"CMM_TK_TRANSPORT_CENTOS_7": "transport-pass",
		"CMM_TK_KEY_CENTOS_7":       "transport-key",
	}
	for envName, wantVal := range expected {
		got, ok := kc.EnvVars[envName]
		if !ok {
			t.Errorf("missing env var %s", envName)
			continue
		}
		if string(got) != wantVal {
			t.Errorf("env var %s = %q, want %q", envName, string(got), wantVal)
		}
	}
	if len(kc.EnvVars) != len(expected) {
		t.Errorf("got %d env vars, want %d", len(kc.EnvVars), len(expected))
	}
}

func TestResolveKitchenCredentials_ResolutionFailure(t *testing.T) {
	// Resolver that only knows about "known_cred".
	resolver := newMockResolver(map[string][]byte{
		"known_cred": []byte("known-value"),
	})
	tkConfig := config.TestKitchenConfig{
		DriverSecrets: map[string]string{
			"good_key":    "known_cred",
			"missing_key": "does_not_exist",
		},
	}

	kc, err := ResolveKitchenCredentials(context.Background(), resolver, tkConfig)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "does_not_exist") {
		t.Errorf("error should mention missing credential name, got: %v", err)
	}
	// Partial results: the known credential should still be present.
	found := false
	for _, v := range kc.EnvVars {
		if string(v) == "known-value" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected partial results to contain the known credential value")
	}
}

func TestResolveKitchenCredentials_NoTransport(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{})
	tkConfig := config.TestKitchenConfig{
		PlatformMap: []config.PlatformMapEntry{
			{
				KitchenName: "ubuntu-22.04",
				Transport:   nil,
			},
		},
	}

	kc, err := ResolveKitchenCredentials(context.Background(), resolver, tkConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kc.EnvVars) != 0 {
		t.Errorf("expected no env vars for nil transport, got %d", len(kc.EnvVars))
	}
}

// ---------------------------------------------------------------------------
// Cleanup test
// ---------------------------------------------------------------------------

func TestKitchenCredentials_Cleanup(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{
		"cred_a": []byte("secret-aaa"),
		"cred_b": []byte("secret-bbb"),
	})
	tkConfig := config.TestKitchenConfig{
		DriverSecrets: map[string]string{
			"key_a": "cred_a",
		},
		PlatformMap: []config.PlatformMapEntry{
			{
				KitchenName: "ubuntu-22.04",
				Transport: &config.PlatformMapTransport{
					Username:           "root",
					PasswordCredential: "cred_b",
				},
			},
		},
	}

	kc, err := ResolveKitchenCredentials(context.Background(), resolver, tkConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kc.EnvVars) == 0 {
		t.Fatal("expected EnvVars to be populated")
	}

	// Capture references to the []byte values before cleanup so we can
	// verify they were zeroed.
	var plaintextRefs [][]byte
	for _, v := range kc.EnvVars {
		plaintextRefs = append(plaintextRefs, v)
	}

	kc.Cleanup()

	for i, p := range plaintextRefs {
		if !secrets.IsZeroed(p) {
			t.Errorf("plaintext[%d] was not zeroed after Cleanup", i)
		}
	}
	if kc.EnvVars != nil {
		t.Error("EnvVars should be nil after Cleanup")
	}
}

// ---------------------------------------------------------------------------
// InjectCredentialEnvVars tests
// ---------------------------------------------------------------------------

func TestInjectCredentialEnvVars_Empty(t *testing.T) {
	base := []string{"HOME=/home/user", "PATH=/usr/bin"}
	creds := &KitchenCredentials{EnvVars: map[string][]byte{}}

	result := InjectCredentialEnvVars(base, creds)
	if len(result) != len(base) {
		t.Errorf("expected %d env vars, got %d", len(base), len(result))
	}
	for i, v := range base {
		if result[i] != v {
			t.Errorf("result[%d] = %q, want %q", i, result[i], v)
		}
	}
}

func TestInjectCredentialEnvVars_AppendsVars(t *testing.T) {
	base := []string{"HOME=/home/user"}
	creds := &KitchenCredentials{
		EnvVars: map[string][]byte{
			"CMM_TK_SECRET_FOO": []byte("bar"),
		},
	}

	result := InjectCredentialEnvVars(base, creds)
	if len(result) != 2 {
		t.Fatalf("expected 2 env vars, got %d: %v", len(result), result)
	}
	if result[0] != "HOME=/home/user" {
		t.Errorf("result[0] = %q, want HOME=/home/user", result[0])
	}
	found := false
	for _, kv := range result {
		if kv == "CMM_TK_SECRET_FOO=bar" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected CMM_TK_SECRET_FOO=bar in result, got: %v", result)
	}
}

func TestInjectCredentialEnvVars_StripsExistingCMMTK(t *testing.T) {
	base := []string{
		"HOME=/home/user",
		"CMM_TK_OLD=stale",
		"CMM_TK_SECRET_LEAKED=old-secret",
		"PATH=/usr/bin",
	}
	creds := &KitchenCredentials{
		EnvVars: map[string][]byte{
			"CMM_TK_SECRET_NEW": []byte("fresh"),
		},
	}

	result := InjectCredentialEnvVars(base, creds)

	for _, kv := range result {
		if idx := strings.IndexByte(kv, '='); idx > 0 {
			key := kv[:idx]
			if key == "CMM_TK_OLD" || key == "CMM_TK_SECRET_LEAKED" {
				t.Errorf("stale env var %q should have been stripped", key)
			}
		}
	}

	foundHome := false
	foundPath := false
	foundNew := false
	for _, kv := range result {
		switch {
		case strings.HasPrefix(kv, "HOME="):
			foundHome = true
		case strings.HasPrefix(kv, "PATH="):
			foundPath = true
		case kv == "CMM_TK_SECRET_NEW=fresh":
			foundNew = true
		}
	}
	if !foundHome {
		t.Error("HOME should be preserved")
	}
	if !foundPath {
		t.Error("PATH should be preserved")
	}
	if !foundNew {
		t.Error("CMM_TK_SECRET_NEW=fresh should be injected")
	}
}

func TestInjectCredentialEnvVars_NilCreds(t *testing.T) {
	base := []string{"HOME=/home/user", "PATH=/usr/bin"}
	result := InjectCredentialEnvVars(base, nil)
	if len(result) != len(base) {
		t.Errorf("expected %d env vars, got %d", len(base), len(result))
	}
}

func TestInjectCredentialEnvVars_NilCreds_StripsStaleCMMTK(t *testing.T) {
	base := []string{
		"HOME=/home/user",
		"CMM_TK_OLD=stale",
		"CMM_TK_SECRET_LEAKED=old-secret",
		"PATH=/usr/bin",
	}
	result := InjectCredentialEnvVars(base, nil)

	for _, kv := range result {
		if idx := strings.IndexByte(kv, '='); idx > 0 {
			key := kv[:idx]
			if strings.HasPrefix(strings.ToUpper(key), "CMM_TK_") {
				t.Errorf("stale env var %q should have been stripped even with nil creds", key)
			}
		}
	}
	if len(result) != 2 {
		t.Errorf("expected 2 env vars (HOME, PATH), got %d: %v", len(result), result)
	}
}

func TestInjectCredentialEnvVars_EmptyCreds_StripsStaleCMMTK(t *testing.T) {
	base := []string{
		"HOME=/home/user",
		"CMM_TK_OLD=stale",
		"PATH=/usr/bin",
	}
	creds := &KitchenCredentials{EnvVars: map[string][]byte{}}

	result := InjectCredentialEnvVars(base, creds)

	for _, kv := range result {
		if idx := strings.IndexByte(kv, '='); idx > 0 {
			key := kv[:idx]
			if strings.HasPrefix(strings.ToUpper(key), "CMM_TK_") {
				t.Errorf("stale env var %q should have been stripped even with empty creds", key)
			}
		}
	}
	if len(result) != 2 {
		t.Errorf("expected 2 env vars (HOME, PATH), got %d: %v", len(result), result)
	}
}

// ---------------------------------------------------------------------------
// ValidateDriverCredentials tests
// ---------------------------------------------------------------------------

func TestValidateDriverCredentials_NilResolver(t *testing.T) {
	errs := ValidateDriverCredentials(context.Background(), nil, config.TestKitchenConfig{
		DriverSecrets: map[string]string{"key": "cred"},
	})
	if errs != nil {
		t.Errorf("expected nil errors for nil resolver, got: %v", errs)
	}
}

func TestValidateDriverCredentials_AllValid(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{
		"cred_a": []byte("aaa"),
		"cred_b": []byte("bbb"),
	})
	tkConfig := config.TestKitchenConfig{
		DriverSecrets: map[string]string{
			"key_a": "cred_a",
		},
		PlatformMap: []config.PlatformMapEntry{
			{
				KitchenName: "ubuntu-22.04",
				Transport: &config.PlatformMapTransport{
					Username:           "root",
					PasswordCredential: "cred_b",
				},
			},
		},
	}

	errs := ValidateDriverCredentials(context.Background(), resolver, tkConfig)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidateDriverCredentials_ZeroesResolvedPlaintext(t *testing.T) {
	// The mock store returns a copy each time, so we use a custom store
	// that lets us inspect the returned plaintext after validation.
	plaintexts := make([][]byte, 0)
	store := &trackingCredentialStore{
		inner:      &mockCredentialStore{creds: map[string][]byte{"drv": []byte("secret"), "pwd": []byte("pass"), "key": []byte("sshkey")}},
		plaintexts: &plaintexts,
	}
	resolver := secrets.NewCredentialResolver(store)

	tkConfig := config.TestKitchenConfig{
		DriverSecrets: map[string]string{
			"password": "drv",
		},
		PlatformMap: []config.PlatformMapEntry{
			{
				KitchenName: "centos-7",
				Transport: &config.PlatformMapTransport{
					Username:           "root",
					PasswordCredential: "pwd",
					SSHKeyCredential:   "key",
				},
			},
		},
	}

	errs := ValidateDriverCredentials(context.Background(), resolver, tkConfig)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if len(plaintexts) != 3 {
		t.Fatalf("expected 3 resolved plaintexts, got %d", len(plaintexts))
	}
	for i, p := range plaintexts {
		if !secrets.IsZeroed(p) {
			t.Errorf("plaintext[%d] was not zeroed after ValidateDriverCredentials", i)
		}
	}
}

func TestValidateDriverCredentials_MissingCredential(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{
		"existing": []byte("value"),
	})
	tkConfig := config.TestKitchenConfig{
		DriverSecrets: map[string]string{
			"good":    "existing",
			"missing": "nonexistent",
		},
	}

	errs := ValidateDriverCredentials(context.Background(), resolver, tkConfig)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}

	found := false
	for _, e := range errs {
		if strings.Contains(e, "nonexistent") {
			found = true
		}
	}
	if !found {
		t.Errorf("error list should mention 'nonexistent', got: %v", errs)
	}
}

// ---------------------------------------------------------------------------
// trackingCredentialStore wraps a store and records returned Plaintext slices
// so tests can verify they were zeroed after use.
// ---------------------------------------------------------------------------

type trackingCredentialStore struct {
	inner      secrets.CredentialStore
	plaintexts *[][]byte
}

func (t *trackingCredentialStore) Get(ctx context.Context, name string) (*secrets.Credential, error) {
	cred, err := t.inner.Get(ctx, name)
	if err == nil && cred != nil {
		*t.plaintexts = append(*t.plaintexts, cred.Plaintext)
	}
	return cred, err
}

func (t *trackingCredentialStore) Create(ctx context.Context, in secrets.CreateCredentialInput) (*secrets.CredentialMetadata, error) {
	return t.inner.Create(ctx, in)
}

func (t *trackingCredentialStore) GetMetadata(ctx context.Context, name string) (*secrets.CredentialMetadata, error) {
	return t.inner.GetMetadata(ctx, name)
}

func (t *trackingCredentialStore) Update(ctx context.Context, in secrets.UpdateCredentialInput) (*secrets.CredentialMetadata, error) {
	return t.inner.Update(ctx, in)
}

func (t *trackingCredentialStore) Delete(ctx context.Context, name string) error {
	return t.inner.Delete(ctx, name)
}

func (t *trackingCredentialStore) List(ctx context.Context) ([]secrets.CredentialMetadata, error) {
	return t.inner.List(ctx)
}

func (t *trackingCredentialStore) ListByType(ctx context.Context, credType string) ([]secrets.CredentialMetadata, error) {
	return t.inner.ListByType(ctx, credType)
}

func (t *trackingCredentialStore) Test(ctx context.Context, name string) (*secrets.ValidationResult, error) {
	return t.inner.Test(ctx, name)
}

func (t *trackingCredentialStore) ReferencedBy(ctx context.Context, name string) ([]secrets.CredentialReference, error) {
	return t.inner.ReferencedBy(ctx, name)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func envVarKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---------------------------------------------------------------------------
// Tests: Chef license key credential resolution
// ---------------------------------------------------------------------------

func TestResolveKitchenCredentials_ChefLicenseKey(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{
		"my-chef-license": []byte("ABCD-1234-EFGH-5678"),
	})
	tkConfig := config.TestKitchenConfig{
		ChefLicenseKeyCredential: "my-chef-license",
	}
	kc, err := ResolveKitchenCredentials(context.Background(), resolver, tkConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	val, ok := kc.EnvVars["CMM_TK_CHEF_LICENSE_KEY"]
	if !ok {
		t.Fatalf("expected CMM_TK_CHEF_LICENSE_KEY in env vars, got keys: %v", envVarKeys(kc.EnvVars))
	}
	if string(val) != "ABCD-1234-EFGH-5678" {
		t.Errorf("expected license key value, got %q", string(val))
	}
}

func TestResolveKitchenCredentials_NoLicenseKey_WhenNotConfigured(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{})
	tkConfig := config.TestKitchenConfig{}
	kc, err := ResolveKitchenCredentials(context.Background(), resolver, tkConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := kc.EnvVars["CMM_TK_CHEF_LICENSE_KEY"]; ok {
		t.Error("should not inject CMM_TK_CHEF_LICENSE_KEY when credential not configured")
	}
}

func TestResolveKitchenCredentials_ChefLicenseKey_MissingCredential(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{}) // credential not in store
	tkConfig := config.TestKitchenConfig{
		ChefLicenseKeyCredential: "missing-license",
	}
	_, err := ResolveKitchenCredentials(context.Background(), resolver, tkConfig)
	if err == nil {
		t.Fatal("expected error when license credential is missing, got nil")
	}
}

func TestValidateDriverCredentials_ChefLicenseKey(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{
		"my-chef-license": []byte("ABCD-1234"),
	})
	tkConfig := config.TestKitchenConfig{
		ChefLicenseKeyCredential: "my-chef-license",
	}
	errs := ValidateDriverCredentials(context.Background(), resolver, tkConfig)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidateDriverCredentials_ChefLicenseKey_Missing(t *testing.T) {
	resolver := newMockResolver(map[string][]byte{})
	tkConfig := config.TestKitchenConfig{
		ChefLicenseKeyCredential: "missing-license",
	}
	errs := ValidateDriverCredentials(context.Background(), resolver, tkConfig)
	if len(errs) == 0 {
		t.Error("expected validation error for missing license credential")
	}
}
