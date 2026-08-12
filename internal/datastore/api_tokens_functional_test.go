//go:build functional

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The in-memory stand-in used elsewhere proves the shape of the calls. Only
// this proves the SQL: the unique constraints, the scoping of a delete, and
// that the last-used timestamp is written at all.

func TestFunctional_APIToken_RoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() { _, _ = db.DeleteAPITokensByUsername(ctx, "credentials-test-user") })

	tok, err := db.InsertAPIToken(ctx, InsertAPITokenParams{
		Username:  "credentials-test-user",
		Name:      "editor",
		TokenHash: "hash-round-trip",
		CanWrite:  true,
	})
	if err != nil {
		t.Fatalf("inserting: %v", err)
	}
	if tok.ID == "" || tok.Name != "editor" || !tok.CanWrite {
		t.Fatalf("inserted credential came back as %+v", tok)
	}
	if tok.LastUsedAt != nil {
		t.Errorf("a credential that has never been used reports a last use of %v", tok.LastUsedAt)
	}

	found, err := db.GetAPITokenByHash(ctx, "hash-round-trip")
	if err != nil {
		t.Fatalf("looking up by hash: %v", err)
	}
	if found.ID != tok.ID {
		t.Errorf("lookup by hash returned %q, want %q", found.ID, tok.ID)
	}

	listed, err := db.ListAPITokensByUsername(ctx, "credentials-test-user")
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != tok.ID {
		t.Fatalf("listing returned %+v", listed)
	}
}

// Read-only is the default in the column as well as in the handler, so a row
// inserted by anything that never heard of the flag cannot write.
func TestFunctional_APIToken_DefaultsToReadOnly(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() { _, _ = db.DeleteAPITokensByUsername(ctx, "credentials-default-user") })

	tok, err := db.InsertAPIToken(ctx, InsertAPITokenParams{
		Username:  "credentials-default-user",
		Name:      "editor",
		TokenHash: "hash-default",
	})
	if err != nil {
		t.Fatalf("inserting: %v", err)
	}
	if tok.CanWrite {
		t.Error("a credential inserted without asking for write access can write")
	}
}

// Destroying is scoped to the owner. A guessed id from somebody else's record
// must be refused exactly as a missing one is.
func TestFunctional_APIToken_DeleteIsScopedToItsOwner(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = db.DeleteAPITokensByUsername(ctx, "credentials-owner")
	})

	tok, err := db.InsertAPIToken(ctx, InsertAPITokenParams{
		Username:  "credentials-owner",
		Name:      "editor",
		TokenHash: "hash-scoped",
	})
	if err != nil {
		t.Fatalf("inserting: %v", err)
	}

	if err := db.DeleteAPIToken(ctx, "credentials-someone-else", tok.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting another account's credential returned %v, want ErrNotFound", err)
	}
	if _, err := db.GetAPITokenByHash(ctx, "hash-scoped"); err != nil {
		t.Errorf("the credential was destroyed by somebody who does not own it: %v", err)
	}

	if err := db.DeleteAPIToken(ctx, "credentials-owner", tok.ID); err != nil {
		t.Fatalf("its owner could not destroy it: %v", err)
	}
	if _, err := db.GetAPITokenByHash(ctx, "hash-scoped"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a destroyed credential still resolves: %v", err)
	}
}

// One secret, one credential. Two rows sharing a hash would mean destroying
// one leaves the other answering to the same secret.
func TestFunctional_APIToken_HashIsUnique(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = db.DeleteAPITokensByUsername(ctx, "credentials-uniq-a")
		_, _ = db.DeleteAPITokensByUsername(ctx, "credentials-uniq-b")
	})

	if _, err := db.InsertAPIToken(ctx, InsertAPITokenParams{
		Username: "credentials-uniq-a", Name: "editor", TokenHash: "hash-collision",
	}); err != nil {
		t.Fatalf("inserting the first: %v", err)
	}
	if _, err := db.InsertAPIToken(ctx, InsertAPITokenParams{
		Username: "credentials-uniq-b", Name: "editor", TokenHash: "hash-collision",
	}); err == nil {
		t.Error("two credentials were stored under one secret")
	}
}

// A blank name would give somebody a list of identical rows and no way to
// choose which to destroy.
func TestFunctional_APIToken_NameCannotBeBlank(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() { _, _ = db.DeleteAPITokensByUsername(ctx, "credentials-blank") })

	if _, err := db.InsertAPIToken(ctx, InsertAPITokenParams{
		Username: "credentials-blank", Name: "   ", TokenHash: "hash-blank",
	}); err == nil {
		t.Error("a credential was stored with a blank name")
	}
}

// "I want to see ... roughly when it was last used."
func TestFunctional_APIToken_LastUsedIsRecorded(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() { _, _ = db.DeleteAPITokensByUsername(ctx, "credentials-touch") })

	tok, err := db.InsertAPIToken(ctx, InsertAPITokenParams{
		Username: "credentials-touch", Name: "editor", TokenHash: "hash-touch",
	})
	if err != nil {
		t.Fatalf("inserting: %v", err)
	}

	before := time.Now().Add(-time.Minute)
	if err := db.TouchAPITokenLastUsed(ctx, tok.ID); err != nil {
		t.Fatalf("recording use: %v", err)
	}

	found, err := db.GetAPITokenByHash(ctx, "hash-touch")
	if err != nil {
		t.Fatalf("re-reading: %v", err)
	}
	if found.LastUsedAt == nil {
		t.Fatal("using a credential did not record that it was used, so its owner cannot " +
			"tell one still in use from one to destroy")
	}
	if found.LastUsedAt.Before(before) {
		t.Errorf("last used = %v, which is before this test started", found.LastUsedAt)
	}
}

// The register must be able to say what made an entry, not only whose it is.
func TestFunctional_FailureRegister_RecordsWhatMadeTheEntry(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	entry, err := db.RecordFailureVerdict(ctx, RecordFailureVerdictParams{
		SubjectName:      "example-cookbook-origin",
		SubjectType:      SubjectTypeGitRepo,
		CookbookName:     "example-cookbook-origin",
		Verdict:          VerdictBroken,
		Reason:           "it fails to converge",
		RaisedBy:         "engineer",
		RaisedOrigin:     OriginCredential,
		RaisedOriginName: "editor on my laptop",
	})
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	if entry.RaisedOrigin != OriginCredential || entry.RaisedOriginName != "editor on my laptop" {
		t.Errorf("entry recorded origin %q/%q, so a note a tool wrote reads as its owner's "+
			"own judgement", entry.RaisedOrigin, entry.RaisedOriginName)
	}

	// A caller that does not say is a screen, which is what every entry was
	// before credentials existed.
	plain, err := db.RecordFailureVerdict(ctx, RecordFailureVerdictParams{
		SubjectName:  "example-cookbook-origin-two",
		SubjectType:  SubjectTypeGitRepo,
		CookbookName: "example-cookbook-origin-two",
		Verdict:      VerdictBroken,
		Reason:       "it fails to converge",
		RaisedBy:     "engineer",
	})
	if err != nil {
		t.Fatalf("recording without an origin: %v", err)
	}
	if plain.RaisedOrigin != OriginScreen {
		t.Errorf("an entry recorded with no origin came back as %q, want %q",
			plain.RaisedOrigin, OriginScreen)
	}
}
