//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The journey suite for journeys/service-secrets.md. Run it with
// `make journey`.
//
// One test per thing the journey says has to be in place. Green means built,
// red means still to do, so running this recomputes the todo list rather than
// asking anybody to keep one true. Outside the gating suite on purpose: a red
// here is a gap, never a broken build.
//
// Most of this journey is built, and the contracts that hold it are named in
// the journey itself. What is asked here is asked in the person's terms — the
// question a security review puts, rather than the shape of the store — because
// that is the promise being made and it is the one that has to survive a change
// nobody thought was about secrets.

// "To enter a credential once, through the interface, and have it stored
// encrypted."
//
// The stored bytes must not contain the secret. Anything weaker — obfuscated,
// encoded, encrypted somewhere else and cached here — fails this the moment
// somebody reads the table.
func TestJourney_AStoredSecretIsNotReadableInTheStore(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	const secret = "correct-horse-battery-staple"
	if err := store.Set(ctx, "credentials/chef-server", json.RawMessage(`{"value":"`+secret+`"}`), true, "admin"); err != nil {
		t.Fatalf("storing a credential: %v", err)
	}

	entry, err := store.GetEntry(ctx, "credentials/chef-server")
	if err != nil || entry == nil {
		t.Fatalf("reading back what was stored: %v", err)
	}
	if bytes.Contains(entry.EncryptedValue, []byte(secret)) {
		t.Error("the password is sitting in the stored bytes, so anybody who can read the " +
			"database can read every credential this service holds")
	}
}

// "To be able to see what credentials exist, what they are for and when they
// were last changed, without any path that returns the secret."
//
// Listing and reading are different operations with different results. This is
// the property the journey calls most likely to be broken by a well-meaning
// change, so it is asked directly: the path the rest of the product uses to
// read configuration must not return a secret at all.
func TestJourney_TheOrdinaryReadPathCannotReturnASecret(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	const secret = "correct-horse-battery-staple"
	if err := store.Set(ctx, "credentials/chef-server", json.RawMessage(`{"value":"`+secret+`"}`), true, "admin"); err != nil {
		t.Fatalf("storing a credential: %v", err)
	}
	if err := store.Set(ctx, "logging", json.RawMessage(`{"level":"info"}`), false, "admin"); err != nil {
		t.Fatalf("storing an ordinary setting: %v", err)
	}

	all, err := store.GetAll(ctx)
	if err != nil {
		t.Fatalf("reading the configuration the way the rest of the product does: %v", err)
	}

	if _, present := all["credentials/chef-server"]; present {
		t.Error("the general read path returned a credential, so every caller that reads " +
			"configuration is now handling a secret whether it meant to or not")
	}
	if _, present := all["logging"]; !present {
		t.Error("the general read path no longer returns ordinary settings, so excluding " +
			"secrets has been implemented by excluding everything")
	}

	for key, value := range all {
		if strings.Contains(string(value), secret) {
			t.Errorf("the secret appears in the general read path under %q, so relying on "+
				"the secret flag alone is not enough", key)
		}
	}
}

// "what they are for and when they were last changed"
func TestJourney_ICanSeeWhatExistsWithoutSeeingWhatItIs(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	const secret = "correct-horse-battery-staple"
	if err := store.Set(ctx, "credentials/chef-server", json.RawMessage(`{"value":"`+secret+`"}`), true, "admin"); err != nil {
		t.Fatalf("storing a credential: %v", err)
	}

	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("listing what the store holds: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("nothing is listed, so an administrator cannot see which credentials " +
			"exist at all")
	}

	for _, entry := range listed {
		if entry.UpdatedAt.IsZero() {
			t.Errorf("%q does not say when it was last changed, so nobody can tell a "+
				"credential that was rotated from one nobody has touched in two years",
				entry.Key)
		}
		if entry.Key == "credentials/chef-server" && !entry.Secret {
			t.Error("a credential is not marked as a secret, so nothing downstream knows " +
				"to keep it out of an ordinary read")
		}
	}
}

// "The same value encrypted twice does not produce the same stored bytes.
// Otherwise anybody who can see the store can tell which credentials are
// identical, which is information they should not have."
func TestJourney_TwoIdenticalPasswordsDoNotLookIdenticalInTheStore(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	const same = `{"value":"the-same-password"}`
	if err := store.Set(ctx, "credentials/one", json.RawMessage(same), true, "admin"); err != nil {
		t.Fatalf("storing the first credential: %v", err)
	}
	if err := store.Set(ctx, "credentials/two", json.RawMessage(same), true, "admin"); err != nil {
		t.Fatalf("storing the second credential: %v", err)
	}

	one, _ := store.GetEntry(ctx, "credentials/one")
	two, _ := store.GetEntry(ctx, "credentials/two")
	if one == nil || two == nil {
		t.Fatal("one of the two credentials was not stored")
	}
	if bytes.Equal(one.EncryptedValue, two.EncryptedValue) {
		t.Error("two systems sharing a password are visibly sharing it in the store, so " +
			"anybody who can read it learns which credentials are reused")
	}
}

// "Encryption is bound to what the value is for, not just to the key. A stored
// value that could be lifted from one place and made to decrypt in another has
// not really been protected — it has been obfuscated."
func TestJourney_ASecretMovedSomewhereItDoesNotBelongDoesNotDecrypt(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	if err := store.Set(ctx, "credentials/production", json.RawMessage(`{"value":"prod-password"}`), true, "admin"); err != nil {
		t.Fatalf("storing a credential: %v", err)
	}

	entry, err := store.GetEntry(ctx, "credentials/production")
	if err != nil || entry == nil {
		t.Fatalf("reading back what was stored: %v", err)
	}

	// The same bytes, filed under a different name. If they decrypt, the
	// binding is decorative and a credential can be promoted by moving it.
	moved := *entry
	moved.Key = "credentials/staging"
	if err := db.SetConfigEntry(ctx, &moved); err != nil {
		t.Fatalf("moving the stored bytes: %v", err)
	}

	if _, err := store.GetSecret(ctx, "credentials/staging"); err == nil {
		t.Error("a credential's stored bytes decrypt under a different name, so anybody " +
			"who can write the store can make a production password read as a test one")
	}
}

// "To replace one when it rotates, and delete one when the system it reaches is
// gone."
func TestJourney_ICanRotateAndRemoveACredential(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	if err := store.Set(ctx, "credentials/rotating", json.RawMessage(`{"value":"old"}`), true, "admin"); err != nil {
		t.Fatalf("storing a credential: %v", err)
	}
	if err := store.Set(ctx, "credentials/rotating", json.RawMessage(`{"value":"new"}`), true, "admin"); err != nil {
		t.Fatalf("rotating a credential: %v", err)
	}

	got, err := store.GetSecret(ctx, "credentials/rotating")
	if err != nil {
		t.Fatalf("reading the rotated credential: %v", err)
	}
	if !strings.Contains(string(got), "new") {
		t.Error("rotating a credential did not replace it, so the old password is still " +
			"what the service presents after somebody changed it")
	}

	if err := store.Delete(ctx, "credentials/rotating"); err != nil {
		t.Fatalf("deleting a credential: %v", err)
	}
	if entry, _ := store.GetEntry(ctx, "credentials/rotating"); entry != nil {
		t.Error("a deleted credential is still in the store, so decommissioning a system " +
			"leaves its password behind")
	}
}

// "To use a stored credential by naming it, so the thing that needs it never
// handles the secret itself."
//
// Naming rather than passing is what keeps a secret out of the screens, the
// logs and the request bodies of everything that needs one.
func TestJourney_AThingThatNeedsACredentialNamesItRatherThanHoldingIt(t *testing.T) {
	adapter := NewCredentialStoreAdapter(mustNewStore(t, newFakeDB()), nil)
	if adapter == nil {
		t.Fatal("there is no way to reach a credential by name, so everything that needs " +
			"one has to be handed the secret itself")
	}

	// Reaching one by name must be a distinct act from reading configuration.
	// If the same call served both, naming a credential would be no protection.
	if _, err := adapter.Get(context.Background(), "does-not-exist"); err == nil {
		t.Error("asking for a credential nobody has stored succeeded, so a caller cannot " +
			"tell a missing credential from an empty one")
	}
}

// "The key that decrypts all of this cannot live in the thing it decrypts."
func TestJourney_TheKeyIsNotStoredInWhatItDecrypts(t *testing.T) {
	db := newFakeDB()
	store := mustNewStore(t, db)
	ctx := context.Background()

	if err := store.Set(ctx, "logging", json.RawMessage(`{"level":"info"}`), false, "admin"); err != nil {
		t.Fatalf("storing an ordinary setting: %v", err)
	}

	all, err := store.GetAll(ctx)
	if err != nil {
		t.Fatalf("reading the configuration: %v", err)
	}
	for key, value := range all {
		if bytes.Contains(value, testKey()) {
			t.Errorf("the encryption key itself is stored under %q, inside the thing it "+
				"decrypts — there is no way to open the store without already having it",
				key)
		}
	}
}

// "Nothing proves a secret never reaches a log. The store will not hand one
// back, but a value in memory can be printed by any code holding it."
func TestJourney_NoSecretReachesALog(t *testing.T) {
	t.Skip("whether a secret is ever written to a log is a property of every package that " +
		"holds one in memory, not of the store; the store refusing to hand one back does " +
		"not constrain code that already has it")
}

// "Nothing proves what happens when the key changes. Rotating the key that
// decrypts everything is not covered here."
func TestJourney_TheEncryptionKeyCanBeRotated(t *testing.T) {
	t.Skip("rotating the key that decrypts every stored credential is not implemented and " +
		"has no rehearsal behind it; assume it is a manual operation")
}

// "Nothing proves a backup is safe to hand over. Backups are asserted to be
// complete and verifiable, not to be free of credential material."
func TestJourney_ABackupIsSafeToHandOver(t *testing.T) {
	t.Skip("whether a backup carries credential material is a property of internal/backup, " +
		"which this package cannot reach; treat a backup as sensitive")
}
