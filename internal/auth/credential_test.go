// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// fakeCredentialStore is an in-memory stand-in for the credential tables. It
// keys tokens by hash exactly as the database does, so a test that destroys one
// and then presents it exercises the real lookup path.
type fakeCredentialStore struct {
	byHash map[string]datastore.APIToken
	users  map[string]datastore.User
	// touched records which tokens were marked as used, so a test can tell
	// "last used" is written without depending on a clock.
	touched []string
}

func newFakeCredentialStore() *fakeCredentialStore {
	return &fakeCredentialStore{
		byHash: map[string]datastore.APIToken{},
		users:  map[string]datastore.User{},
	}
}

func (f *fakeCredentialStore) InsertAPIToken(_ context.Context, p datastore.InsertAPITokenParams) (
	datastore.APIToken, error) {
	tok := datastore.APIToken{
		ID:       "tok-" + p.Name,
		Username: p.Username,
		Name:     p.Name,
		CanWrite: p.CanWrite,
	}
	f.byHash[p.TokenHash] = tok
	return tok, nil
}

func (f *fakeCredentialStore) ListAPITokensByUsername(_ context.Context, username string) (
	[]datastore.APIToken, error) {
	var out []datastore.APIToken
	for _, t := range f.byHash {
		if t.Username == username {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeCredentialStore) GetAPITokenByHash(_ context.Context, hash string) (
	datastore.APIToken, error) {
	t, ok := f.byHash[hash]
	if !ok {
		return datastore.APIToken{}, datastore.ErrNotFound
	}
	return t, nil
}

func (f *fakeCredentialStore) DeleteAPIToken(_ context.Context, username, id string) error {
	for hash, t := range f.byHash {
		if t.ID == id && t.Username == username {
			delete(f.byHash, hash)
			return nil
		}
	}
	return datastore.ErrNotFound
}

func (f *fakeCredentialStore) DeleteAPITokensByUsername(_ context.Context, username string) (int, error) {
	n := 0
	for hash, t := range f.byHash {
		if t.Username == username {
			delete(f.byHash, hash)
			n++
		}
	}
	return n, nil
}

func (f *fakeCredentialStore) TouchAPITokenLastUsed(_ context.Context, id string) error {
	f.touched = append(f.touched, id)
	return nil
}

func (f *fakeCredentialStore) GetUserByUsername(_ context.Context, username string) (
	datastore.User, error) {
	u, ok := f.users[username]
	if !ok {
		return datastore.User{}, datastore.ErrNotFound
	}
	return u, nil
}

// credentialFixture returns a manager whose store already holds one account.
func credentialFixture(t *testing.T, role string) (*CredentialManager, *fakeCredentialStore) {
	t.Helper()
	store := newFakeCredentialStore()
	store.users["engineer"] = datastore.User{
		Username:     "engineer",
		Role:         role,
		AuthProvider: "local",
	}
	return NewCredentialManager(store), store
}

// The secret is generated here and handed back once. Nothing else may be able
// to produce it, so two issues of the same credential name must not collide.
func TestIssueCredentialReturnsAFreshSecretEachTime(t *testing.T) {
	m, _ := credentialFixture(t, "operator")

	_, first, err := m.Issue(context.Background(), "engineer", "editor", false)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}
	_, second, err := m.Issue(context.Background(), "engineer", "laptop", false)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	if first == second {
		t.Fatal("two credentials were issued with the same secret, so destroying one " +
			"would not stop the other")
	}
	for _, secret := range []string{first, second} {
		if !strings.HasPrefix(secret, CredentialPrefix) {
			t.Errorf("secret %q does not carry the credential prefix, so nothing can tell "+
				"it apart from a session id presented in the same header", secret)
		}
		if len(secret) < 40 {
			t.Errorf("secret is %d characters, which is short enough to be worth guessing",
				len(secret))
		}
	}
}

// "Shown once. No recovering an old one." The plaintext must not be anywhere
// the service can read it back from.
func TestTheStoredCredentialIsNotTheSecret(t *testing.T) {
	m, store := credentialFixture(t, "viewer")

	_, secret, err := m.Issue(context.Background(), "engineer", "editor", false)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	for hash, tok := range store.byHash {
		if hash == secret {
			t.Fatal("the secret itself was stored, so anybody who can read the database " +
				"holds every live credential")
		}
		if strings.Contains(hash, strings.TrimPrefix(secret, CredentialPrefix)) {
			t.Fatal("the stored value contains the secret")
		}
		if tok.Name != "editor" {
			t.Errorf("stored credential name = %q, want %q", tok.Name, "editor")
		}
	}
}

// "It acts as me, at my level of access." The role must be read from the
// account at the moment the credential is used, not frozen when it was made —
// otherwise a demotion leaves a credential at the old level.
func TestValidateCredentialCarriesTheAccountsCurrentRole(t *testing.T) {
	m, store := credentialFixture(t, "admin")

	_, secret, err := m.Issue(context.Background(), "engineer", "editor", false)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	info, err := m.Validate(context.Background(), secret)
	if err != nil {
		t.Fatalf("validating a credential just issued: %v", err)
	}
	if !info.IsAdmin() {
		t.Errorf("role = %q, want the account's own role", info.Role)
	}

	// The account is demoted after the credential was made.
	demoted := store.users["engineer"]
	demoted.Role = "viewer"
	store.users["engineer"] = demoted

	info, err = m.Validate(context.Background(), secret)
	if err != nil {
		t.Fatalf("validating after demotion: %v", err)
	}
	if info.IsAdmin() {
		t.Error("the credential still carries admin after the account was demoted, so it is " +
			"a second permissions model rather than another way into the same account")
	}
}

// "How something got in is settled when it signs in ... the service attaches
// that, never the caller."
func TestValidateCredentialRecordsHowTheCallerGotIn(t *testing.T) {
	m, _ := credentialFixture(t, "operator")

	_, secret, err := m.Issue(context.Background(), "engineer", "editor", true)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	info, err := m.Validate(context.Background(), secret)
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	if info.AccessMethod != AccessMethodCredential {
		t.Errorf("access method = %q, want %q — nothing can tell this from a login at a "+
			"screen", info.AccessMethod, AccessMethodCredential)
	}
	if info.CredentialName != "editor" {
		t.Errorf("credential name = %q, want %q — one credential cannot be told from another",
			info.CredentialName, "editor")
	}
	if !info.CredentialCanWrite {
		t.Error("a credential issued with write access does not carry it")
	}
	if !info.IsCredential() {
		t.Error("IsCredential() is false for a caller that got in with a credential")
	}
}

// "read only if they do not choose" — the default is enforced at the layer that
// mints, so a caller that never heard of the flag cannot get a writing one.
func TestACredentialIsReadOnlyUnlessAskedFor(t *testing.T) {
	m, _ := credentialFixture(t, "admin")

	tok, secret, err := m.Issue(context.Background(), "engineer", "editor", false)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}
	if tok.CanWrite {
		t.Error("a credential made without asking for write access can write")
	}

	info, err := m.Validate(context.Background(), secret)
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	if info.CredentialCanWrite {
		t.Error("a read-only credential authenticates as one that can write")
	}
}

// "destroy it the moment I am unsure ... Immediate, because the reason to
// destroy one is believing somebody else has it."
func TestADestroyedCredentialStopsWorkingAtOnce(t *testing.T) {
	m, _ := credentialFixture(t, "operator")

	tok, secret, err := m.Issue(context.Background(), "engineer", "editor", false)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}
	if _, err := m.Validate(context.Background(), secret); err != nil {
		t.Fatalf("the credential did not work before being destroyed: %v", err)
	}

	if err := m.Destroy(context.Background(), "engineer", tok.ID); err != nil {
		t.Fatalf("destroying: %v", err)
	}

	if _, err := m.Validate(context.Background(), secret); err == nil {
		t.Fatal("a destroyed credential still authenticates, so destroying one does not " +
			"take it away from whoever else has it")
	}
}

// Destroying is scoped to the owner: one person's listing cannot reach
// another's credential by guessing an id.
func TestDestroyingSomebodyElsesCredentialIsRefused(t *testing.T) {
	m, store := credentialFixture(t, "admin")
	store.users["colleague"] = datastore.User{Username: "colleague", Role: "admin"}

	tok, secret, err := m.Issue(context.Background(), "engineer", "editor", false)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	if err := m.Destroy(context.Background(), "colleague", tok.ID); err == nil {
		t.Fatal("an administrator destroyed another person's credential through their own " +
			"listing, which is not the same act and is not what this endpoint is for")
	}
	if _, err := m.Validate(context.Background(), secret); err != nil {
		t.Errorf("the credential stopped working anyway: %v", err)
	}
}

// A locked account is locked everywhere. Refusing at the screen and admitting
// at the header would make the credential the way around a lockout.
func TestALockedAccountsCredentialIsRefused(t *testing.T) {
	m, store := credentialFixture(t, "operator")

	_, secret, err := m.Issue(context.Background(), "engineer", "editor", false)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	locked := store.users["engineer"]
	locked.IsLocked = true
	store.users["engineer"] = locked

	if _, err := m.Validate(context.Background(), secret); err == nil {
		t.Fatal("a locked account's credential still works, so locking somebody out of the " +
			"screen leaves every tool they configured still signed in")
	}
}

// An account that has gone leaves nothing behind that authenticates.
func TestACredentialForAnAccountThatIsGoneIsRefused(t *testing.T) {
	m, store := credentialFixture(t, "operator")

	_, secret, err := m.Issue(context.Background(), "engineer", "editor", false)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}
	delete(store.users, "engineer")

	if _, err := m.Validate(context.Background(), secret); err == nil {
		t.Fatal("a credential outlived the account it belongs to")
	}
}

// The header carries both kinds. Anything without the prefix is somebody else's
// business — a session id — and must be handed back as such rather than
// reported as a bad credential.
func TestSomethingThatIsNotACredentialIsNotTreatedAsOne(t *testing.T) {
	m, _ := credentialFixture(t, "viewer")

	for _, presented := range []string{
		"",
		"9f8d7c6b-5a4e-4321-8765-0fedcba98765", // a session id
		"Bearer something",
	} {
		_, err := m.Validate(context.Background(), presented)
		if !errors.Is(err, ErrNotACredential) {
			t.Errorf("Validate(%q) = %v, want ErrNotACredential so the session path still "+
				"gets a chance at it", presented, err)
		}
	}
}

// "I want to see ... roughly when it was last used." Nothing records that
// unless using it writes it.
func TestUsingACredentialRecordsThatItWasUsed(t *testing.T) {
	m, store := credentialFixture(t, "operator")

	tok, secret, err := m.Issue(context.Background(), "engineer", "editor", false)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}
	if _, err := m.Validate(context.Background(), secret); err != nil {
		t.Fatalf("validating: %v", err)
	}

	if len(store.touched) != 1 || store.touched[0] != tok.ID {
		t.Errorf("last used was recorded as %v, want one entry for %q — its owner cannot "+
			"tell a credential still in use from one to destroy", store.touched, tok.ID)
	}
}

// "When I leave, my access leaves with me." Sessions cascade off the account
// row; credentials are keyed by username and would not, so they are collected
// deliberately or not at all.
func TestDeletingAnAccountTakesItsCredentials(t *testing.T) {
	m, store := credentialFixture(t, "operator")

	_, first, err := m.Issue(context.Background(), "engineer", "editor", false)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}
	_, second, err := m.Issue(context.Background(), "engineer", "unattended job", true)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	n, err := m.DestroyAllFor(context.Background(), "engineer")
	if err != nil {
		t.Fatalf("destroying all: %v", err)
	}
	if n != 2 {
		t.Errorf("destroyed %d credentials, want 2", n)
	}
	// Put the account back to prove it is the credentials that are gone and
	// not merely the account lookup failing.
	store.users["engineer"] = datastore.User{Username: "engineer", Role: "operator"}
	for _, secret := range []string{first, second} {
		if _, err := m.Validate(context.Background(), secret); err == nil {
			t.Error("a credential outlived the account it was made from, so somebody who " +
				"left still has a way in")
		}
	}
}

// A credential must be named, so a listing can be read and the right one
// destroyed.
func TestACredentialMustBeNamed(t *testing.T) {
	m, _ := credentialFixture(t, "admin")

	if _, _, err := m.Issue(context.Background(), "engineer", "   ", false); err == nil {
		t.Error("a credential was issued with a blank name, so its owner sees a list of " +
			"identical rows and cannot tell which to destroy")
	}
}
