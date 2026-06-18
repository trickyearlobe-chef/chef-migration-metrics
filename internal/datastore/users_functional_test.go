// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"errors"
	"testing"
)

// TestFunctional_UpsertSAMLUser_TransientNameID_AnchorsOnUsername reproduces the
// production failure where an IdP sends an unstable (transient) NameID. The
// federated saml_subject ("{idp_entity_id}:{NameID}") then changes on every
// login, while the username attribute stays stable. Identity must anchor on the
// stable username so re-login updates the same row (refreshing saml_subject)
// rather than attempting a second INSERT and colliding on the username unique
// constraint ("already exists").
func TestFunctional_UpsertSAMLUser_TransientNameID_AnchorsOnUsername(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const username = "func-saml-drift-user"
	cleanupTestData(t, db,
		"DELETE FROM users WHERE username = '"+username+"'",
	)

	// Login 1: transient NameID #1.
	u1, isNew1, err := db.UpsertSAMLUser(ctx, InsertUserParams{
		Username:     username,
		DisplayName:  "Drift User",
		Email:        "drift@example.com",
		Role:         "admin",
		AuthProvider: "saml",
		SAMLSubject:  "https://idp.example.com/saml:transient-aaaaaaaa",
	})
	if err != nil {
		t.Fatalf("login 1 upsert: %v", err)
	}
	if !isNew1 {
		t.Errorf("login 1: expected isNew=true, got false")
	}

	// Login 2: DIFFERENT transient NameID, SAME stable username.
	u2, isNew2, err := db.UpsertSAMLUser(ctx, InsertUserParams{
		Username:     username,
		DisplayName:  "Drift User",
		Email:        "drift@example.com",
		Role:         "admin",
		AuthProvider: "saml",
		SAMLSubject:  "https://idp.example.com/saml:transient-bbbbbbbb",
	})
	if err != nil {
		t.Fatalf("login 2 upsert (must not fail with 'already exists'): %v", err)
	}
	if isNew2 {
		t.Errorf("login 2: expected isNew=false (same human), got true")
	}
	if u2.Username != u1.Username {
		t.Errorf("login 2: username changed %q -> %q", u1.Username, u2.Username)
	}
	if u2.SAMLSubject != "https://idp.example.com/saml:transient-bbbbbbbb" {
		t.Errorf("login 2: saml_subject not refreshed to new NameID, got %q", u2.SAMLSubject)
	}

	// Exactly one row must exist for this username.
	var count int
	if err := db.pool.QueryRowContext(ctx,
		"SELECT count(*) FROM users WHERE username = $1", username).Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 row for %q, got %d", username, count)
	}
}

// TestFunctional_UpsertSAMLUser_StableSubject_Updates confirms the normal
// stable-NameID path still updates the matched row in place (role re-evaluated
// from the assertion on every login — SAML remains authoritative).
func TestFunctional_UpsertSAMLUser_StableSubject_Updates(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const username = "func-saml-stable-user"
	const subject = "https://idp.example.com/saml:stable-nameid"
	cleanupTestData(t, db,
		"DELETE FROM users WHERE username = '"+username+"'",
	)

	if _, isNew, err := db.UpsertSAMLUser(ctx, InsertUserParams{
		Username: username, Email: "stable@example.com", Role: "viewer",
		AuthProvider: "saml", SAMLSubject: subject,
	}); err != nil || !isNew {
		t.Fatalf("first upsert: isNew=%v err=%v", isNew, err)
	}

	u, isNew, err := db.UpsertSAMLUser(ctx, InsertUserParams{
		Username: username, Email: "stable@example.com", Role: "admin",
		AuthProvider: "saml", SAMLSubject: subject,
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if isNew {
		t.Errorf("second upsert: expected isNew=false")
	}
	if u.Role != "admin" {
		t.Errorf("expected role re-evaluated to admin, got %q", u.Role)
	}
}

// TestFunctional_UpsertSAMLUser_DoesNotHijackLocalUser ensures the stable-username
// fallback never takes over a non-SAML (local-password) account that happens to
// share the username — the spec keeps SAML and local identities separate.
func TestFunctional_UpsertSAMLUser_DoesNotHijackLocalUser(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	const username = "func-collision-user"
	cleanupTestData(t, db,
		"DELETE FROM users WHERE username = '"+username+"'",
	)

	// Pre-existing local-password user.
	if _, err := db.InsertUser(ctx, InsertUserParams{
		Username: username, PasswordHash: "x", Role: "admin", AuthProvider: "local",
	}); err != nil {
		t.Fatalf("seeding local user: %v", err)
	}

	_, _, err := db.UpsertSAMLUser(ctx, InsertUserParams{
		Username: username, Email: "saml@example.com", Role: "viewer",
		AuthProvider: "saml", SAMLSubject: "https://idp.example.com/saml:some-nameid",
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists when username belongs to a local user, got %v", err)
	}

	// The local user must be untouched (still local, no saml_subject).
	lu, err := db.GetUserByUsername(ctx, username)
	if err != nil {
		t.Fatalf("re-reading local user: %v", err)
	}
	if lu.AuthProvider != "local" || lu.SAMLSubject != "" {
		t.Errorf("local user hijacked: provider=%q saml_subject=%q", lu.AuthProvider, lu.SAMLSubject)
	}
}
