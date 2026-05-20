// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package jit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth/samlsp"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// mockStore implements UserStore for testing.
type mockStore struct {
	upsertFn func(ctx context.Context, p datastore.InsertUserParams) (datastore.User, bool, error)
	loginFn  func(ctx context.Context, username string) error
}

func (m *mockStore) UpsertSAMLUser(ctx context.Context, p datastore.InsertUserParams) (datastore.User, bool, error) {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, p)
	}
	return datastore.User{
		Username:     p.Username,
		DisplayName:  p.DisplayName,
		Email:        p.Email,
		Role:         p.Role,
		AuthProvider: p.AuthProvider,
		SAMLSubject:  p.SAMLSubject,
	}, true, nil
}

func (m *mockStore) RecordLoginSuccess(ctx context.Context, username string) error {
	if m.loginFn != nil {
		return m.loginFn(ctx, username)
	}
	return nil
}

func TestProvision_NewUser(t *testing.T) {
	var capturedParams datastore.InsertUserParams
	store := &mockStore{
		upsertFn: func(_ context.Context, p datastore.InsertUserParams) (datastore.User, bool, error) {
			capturedParams = p
			return datastore.User{
				Username:     p.Username,
				DisplayName:  p.DisplayName,
				Email:        p.Email,
				Role:         p.Role,
				AuthProvider: "saml",
				SAMLSubject:  p.SAMLSubject,
			}, true, nil
		},
	}

	prov := New(store, nil)
	info := &samlsp.UserInfo{
		SAMLSubject: "https://idp.example.com:uid-123",
		Username:    "jdoe",
		Email:       "jdoe@example.com",
		DisplayName: "Jane Doe",
		Groups:      []string{"ops"},
		Role:        "operator",
	}

	user, isNew, err := prov.Provision(context.Background(), info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isNew {
		t.Error("expected isNew=true for new user")
	}
	if user.Username != "jdoe" {
		t.Errorf("Username = %q, want %q", user.Username, "jdoe")
	}
	if user.Role != "operator" {
		t.Errorf("Role = %q, want %q", user.Role, "operator")
	}
	if capturedParams.SAMLSubject != "https://idp.example.com:uid-123" {
		t.Errorf("SAMLSubject = %q, want %q", capturedParams.SAMLSubject, "https://idp.example.com:uid-123")
	}
	if capturedParams.AuthProvider != "saml" {
		t.Errorf("AuthProvider = %q, want %q", capturedParams.AuthProvider, "saml")
	}
}

func TestProvision_ExistingUser(t *testing.T) {
	store := &mockStore{
		upsertFn: func(_ context.Context, p datastore.InsertUserParams) (datastore.User, bool, error) {
			return datastore.User{
				Username:    p.Username,
				Role:        p.Role,
				SAMLSubject: p.SAMLSubject,
			}, false, nil
		},
	}

	prov := New(store, nil)
	info := &samlsp.UserInfo{
		SAMLSubject: "https://idp.example.com:uid-123",
		Username:    "jdoe",
		Role:        "admin",
	}

	_, isNew, err := prov.Provision(context.Background(), info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isNew {
		t.Error("expected isNew=false for existing user")
	}
}

func TestProvision_NilUserInfo(t *testing.T) {
	prov := New(&mockStore{}, nil)
	_, _, err := prov.Provision(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil info")
	}
}

func TestProvision_EmptySAMLSubject(t *testing.T) {
	prov := New(&mockStore{}, nil)
	_, _, err := prov.Provision(context.Background(), &samlsp.UserInfo{})
	if err == nil {
		t.Fatal("expected error for empty SAML subject")
	}
}

func TestProvision_DefaultsRoleToViewer(t *testing.T) {
	var capturedRole string
	store := &mockStore{
		upsertFn: func(_ context.Context, p datastore.InsertUserParams) (datastore.User, bool, error) {
			capturedRole = p.Role
			return datastore.User{Username: p.Username, Role: p.Role}, true, nil
		},
	}

	prov := New(store, nil)
	info := &samlsp.UserInfo{
		SAMLSubject: "https://idp.example.com:uid-456",
		Username:    "viewer-user",
		Role:        "", // empty role
	}

	_, _, err := prov.Provision(context.Background(), info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedRole != "viewer" {
		t.Errorf("expected default role 'viewer', got %q", capturedRole)
	}
}

func TestProvision_FallbackUsername(t *testing.T) {
	var capturedUsername string
	store := &mockStore{
		upsertFn: func(_ context.Context, p datastore.InsertUserParams) (datastore.User, bool, error) {
			capturedUsername = p.Username
			return datastore.User{Username: p.Username}, true, nil
		},
	}

	prov := New(store, nil)
	info := &samlsp.UserInfo{
		SAMLSubject: "https://idp.example.com:User@Example.COM",
		Username:    "", // empty triggers fallback
		Role:        "viewer",
	}

	_, _, err := prov.Provision(context.Background(), info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedUsername != "user_at_example.com" {
		t.Errorf("fallback username = %q, want %q", capturedUsername, "user_at_example.com")
	}
}

func TestProvision_StoreError(t *testing.T) {
	store := &mockStore{
		upsertFn: func(_ context.Context, _ datastore.InsertUserParams) (datastore.User, bool, error) {
			return datastore.User{}, false, errors.New("db connection failed")
		},
	}

	prov := New(store, nil)
	info := &samlsp.UserInfo{
		SAMLSubject: "https://idp.example.com:uid-789",
		Username:    "fail-user",
		Role:        "viewer",
	}

	_, _, err := prov.Provision(context.Background(), info)
	if err == nil {
		t.Fatal("expected error from store failure")
	}
}

func TestSanitiseUsername(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://idp.example.com:user@example.com", "user_at_example.com"},
		{"urn:idp:John Doe", "john_doe"},
		{"simple", "simple"},
		{"idp:user/name\\here", "user_name_here"},
		{"idp:" + strings.Repeat("a", 100), strings.Repeat("a", 64)},
	}

	for _, tt := range tests {
		got := sanitiseUsername(tt.input)
		if got != tt.want {
			t.Errorf("sanitiseUsername(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
