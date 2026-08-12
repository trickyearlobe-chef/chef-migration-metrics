// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// How a caller got in. Settled when they sign in and attached by the service,
// never read from anything the caller sends: a tool can say it is anything, so
// nothing it says about itself is worth recording.
const (
	// AccessMethodScreen is a person who signed in at the web interface.
	AccessMethodScreen = "screen"
	// AccessMethodCredential is a tool holding a credential somebody made and
	// named. Which credential is recorded too, so one can be told from another
	// — but not two tools sharing one, which is honest: it says what somebody
	// did, not what they meant.
	AccessMethodCredential = "credential"
)

// CredentialPrefix marks a secret as a credential rather than a session id.
// Both arrive in the same header, and the prefix is what lets the two be told
// apart without guessing at the shape of a UUID. It is also what makes a leaked
// one recognisable in a scan of somebody's configuration directory.
const CredentialPrefix = "cmm_"

// credentialSecretBytes is the entropy behind a secret, before encoding.
const credentialSecretBytes = 32

// ErrNotACredential means the presented string is not one of ours to judge —
// it carries no credential prefix. Returned so the caller can fall through to
// the session path rather than reporting a bad credential for what is very
// likely a perfectly good session id.
var ErrNotACredential = errors.New("auth: not a credential")

// ErrCredentialRejected means the string was a credential and did not work:
// unknown, or belonging to an account that is locked or gone. Deliberately one
// error for all three, because telling them apart tells whoever is guessing
// which usernames exist.
var ErrCredentialRejected = errors.New("auth: credential is not valid")

// CredentialStore is the persistence a CredentialManager needs. Satisfied by
// *datastore.DB.
type CredentialStore interface {
	InsertAPIToken(ctx context.Context, p datastore.InsertAPITokenParams) (datastore.APIToken, error)
	ListAPITokensByUsername(ctx context.Context, username string) ([]datastore.APIToken, error)
	GetAPITokenByHash(ctx context.Context, tokenHash string) (datastore.APIToken, error)
	DeleteAPIToken(ctx context.Context, username, id string) error
	DeleteAPITokensByUsername(ctx context.Context, username string) (int, error)
	TouchAPITokenLastUsed(ctx context.Context, id string) error

	// GetUserByUsername is here because a credential carries its account's
	// level of access and no more, which means reading the account rather than
	// anything stored beside the credential. Every account has a row: a person
	// authenticated by an identity provider is provisioned one on first login.
	GetUserByUsername(ctx context.Context, username string) (datastore.User, error)
}

// CredentialManager issues, lists, destroys and validates the credentials a
// person makes for a tool they are holding.
//
// A credential is another way into the same account. There is no service
// account and no second permissions model here — Validate returns the same
// SessionInfo shape the login screen produces, differing only in recording how
// the caller got in.
type CredentialManager struct {
	store  CredentialStore
	logger func(level, msg string)
}

// CredentialManagerOption is a functional option for NewCredentialManager.
type CredentialManagerOption func(*CredentialManager)

// WithCredentialLogger sets a logging callback for credential lifecycle events.
func WithCredentialLogger(fn func(level, msg string)) CredentialManagerOption {
	return func(m *CredentialManager) { m.logger = fn }
}

// NewCredentialManager creates a manager over the given store.
func NewCredentialManager(store CredentialStore, opts ...CredentialManagerOption) *CredentialManager {
	m := &CredentialManager{store: store}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// HashCredential returns the stored form of a secret: hex SHA-256.
//
// A plain hash rather than a password hash on purpose. This is 256 bits of
// randomness we generated, not something a person chose, so there is no
// dictionary to run against it and nothing for a slow hash to buy — and it is
// checked on every request an assistant makes, where a deliberately slow hash
// would be a denial of service anybody could trigger with a wrong guess.
func HashCredential(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// Issue mints a credential for an account and returns it alongside the secret.
//
// The secret is returned here and nowhere else, ever. Nothing stores it, so
// there is no second chance to show it and no copy for anyone to take.
func (m *CredentialManager) Issue(ctx context.Context, username, name string, canWrite bool) (
	datastore.APIToken, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return datastore.APIToken{}, "", errors.New(
			"auth: a credential needs a name, so its owner can tell one from another")
	}
	if strings.TrimSpace(username) == "" {
		return datastore.APIToken{}, "", errors.New("auth: a credential needs an account")
	}

	secret, err := newCredentialSecret()
	if err != nil {
		return datastore.APIToken{}, "", err
	}

	tok, err := m.store.InsertAPIToken(ctx, datastore.InsertAPITokenParams{
		Username:  username,
		Name:      name,
		TokenHash: HashCredential(secret),
		CanWrite:  canWrite,
	})
	if err != nil {
		return datastore.APIToken{}, "", fmt.Errorf("auth: issuing credential: %w", err)
	}

	m.logf("INFO", "credential %q issued for user %q (can_write=%t)", name, username, canWrite)
	return tok, secret, nil
}

// List returns the credentials belonging to an account. None of them can be
// used as one — there is no secret in the record to return.
func (m *CredentialManager) List(ctx context.Context, username string) ([]datastore.APIToken, error) {
	tokens, err := m.store.ListAPITokensByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("auth: listing credentials: %w", err)
	}
	return tokens, nil
}

// Destroy removes one credential belonging to the named account. It stops
// working at once, because the row is gone rather than marked.
//
// Scoped by account: destroying one is something its owner does from their own
// record, and an id from somebody else's is refused exactly as a missing one is.
func (m *CredentialManager) Destroy(ctx context.Context, username, id string) error {
	if err := m.store.DeleteAPIToken(ctx, username, id); err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			return datastore.ErrNotFound
		}
		return fmt.Errorf("auth: destroying credential: %w", err)
	}
	m.logf("INFO", "credential %s destroyed by user %q", id, username)
	return nil
}

// DestroyAllFor removes every credential belonging to an account, and returns
// how many. Called when the account goes: "when I leave, my access leaves with
// me" is only true if the credentials go too, and they are not reachable
// through the account's own row — nothing else would collect them.
func (m *CredentialManager) DestroyAllFor(ctx context.Context, username string) (int, error) {
	n, err := m.store.DeleteAPITokensByUsername(ctx, username)
	if err != nil {
		return 0, fmt.Errorf("auth: destroying credentials for %q: %w", username, err)
	}
	if n > 0 {
		m.logf("INFO", "destroyed %d credential(s) belonging to %q", n, username)
	}
	return n, nil
}

// Validate turns a presented secret into a session.
//
// Returns ErrNotACredential when the string carries no credential prefix, so
// the caller can try it as a session id instead. Returns ErrCredentialRejected
// when it is a credential that does not work, for any reason.
//
// The role comes from the account, read now — so a demotion, a lock or a
// deletion applies to every credential the moment it applies to the person.
func (m *CredentialManager) Validate(ctx context.Context, secret string) (*SessionInfo, error) {
	if !IsCredentialSecret(secret) {
		return nil, ErrNotACredential
	}

	tok, err := m.store.GetAPITokenByHash(ctx, HashCredential(secret))
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			return nil, ErrCredentialRejected
		}
		return nil, fmt.Errorf("auth: validating credential: %w", err)
	}

	user, err := m.store.GetUserByUsername(ctx, tok.Username)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			// The account is gone. Its credentials should have gone with it,
			// so this is worth saying out loud rather than swallowing.
			m.logf("WARN", "credential %q names user %q, which no longer exists",
				tok.Name, tok.Username)
			return nil, ErrCredentialRejected
		}
		return nil, fmt.Errorf("auth: reading the account behind a credential: %w", err)
	}
	if user.IsLocked {
		m.logf("WARN", "credential %q refused: account %q is locked", tok.Name, tok.Username)
		return nil, ErrCredentialRejected
	}

	// Best effort: a failure here costs a stale "last used" and must not cost
	// the caller their request.
	if err := m.store.TouchAPITokenLastUsed(ctx, tok.ID); err != nil {
		m.logf("DEBUG", "recording last use of credential %q: %v", tok.Name, err)
	}

	return &SessionInfo{
		Username:           user.Username,
		AuthProvider:       user.AuthProvider,
		Role:               user.Role,
		AccessMethod:       AccessMethodCredential,
		CredentialID:       tok.ID,
		CredentialName:     tok.Name,
		CredentialCanWrite: tok.CanWrite,
	}, nil
}

// IsCredentialSecret reports whether a presented string is one of ours rather
// than a session id.
func IsCredentialSecret(s string) bool {
	return strings.HasPrefix(s, CredentialPrefix)
}

// newCredentialSecret generates a secret: the prefix, then 32 random bytes in
// unpadded URL-safe base64 so the whole thing survives a shell, a YAML file and
// an editor's settings dialog without quoting.
func newCredentialSecret() (string, error) {
	buf := make([]byte, credentialSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: generating a credential secret: %w", err)
	}
	return CredentialPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func (m *CredentialManager) logf(level, format string, args ...any) {
	if m.logger != nil {
		m.logger(level, fmt.Sprintf(format, args...))
	}
}
