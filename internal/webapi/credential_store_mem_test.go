// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"sync"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// memCredentialStore is an in-memory stand-in for the credential tables,
// keyed by hash exactly as the database is.
//
// Real rather than a stub returning canned answers, because the questions
// asked of it are things like "does a destroyed credential still work" and
// "does the listing carry the secret" — which a stub would answer by
// construction rather than by behaving.
type memCredentialStore struct {
	mu     sync.Mutex
	byHash map[string]datastore.APIToken
	users  map[string]datastore.User
	nextID int
}

func newMemCredentialStore() *memCredentialStore {
	return &memCredentialStore{
		byHash: map[string]datastore.APIToken{},
		users:  map[string]datastore.User{},
	}
}

// withUser seeds an account, so a credential has something to belong to.
func (s *memCredentialStore) withUser(username, role string) *memCredentialStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[username] = datastore.User{
		Username:     username,
		Role:         role,
		AuthProvider: "local",
	}
	return s
}

func (s *memCredentialStore) InsertAPIToken(_ context.Context, p datastore.InsertAPITokenParams) (
	datastore.APIToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.byHash {
		if t.Username == p.Username && t.Name == p.Name {
			return datastore.APIToken{}, datastore.ErrAlreadyExists
		}
	}
	s.nextID++
	tok := datastore.APIToken{
		ID:       "tok-" + string(rune('a'+s.nextID-1)),
		Username: p.Username,
		Name:     p.Name,
		CanWrite: p.CanWrite,
	}
	s.byHash[p.TokenHash] = tok
	return tok, nil
}

func (s *memCredentialStore) ListAPITokensByUsername(_ context.Context, username string) (
	[]datastore.APIToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []datastore.APIToken{}
	for _, t := range s.byHash {
		if t.Username == username {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *memCredentialStore) GetAPITokenByHash(_ context.Context, hash string) (
	datastore.APIToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.byHash[hash]
	if !ok {
		return datastore.APIToken{}, datastore.ErrNotFound
	}
	return t, nil
}

func (s *memCredentialStore) DeleteAPIToken(_ context.Context, username, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, t := range s.byHash {
		if t.ID == id && t.Username == username {
			delete(s.byHash, hash)
			return nil
		}
	}
	return datastore.ErrNotFound
}

func (s *memCredentialStore) DeleteAPITokensByUsername(_ context.Context, username string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for hash, t := range s.byHash {
		if t.Username == username {
			delete(s.byHash, hash)
			n++
		}
	}
	return n, nil
}

func (s *memCredentialStore) TouchAPITokenLastUsed(context.Context, string) error {
	return nil
}

func (s *memCredentialStore) GetUserByUsername(_ context.Context, username string) (
	datastore.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[username]
	if !ok {
		return datastore.User{}, datastore.ErrNotFound
	}
	return u, nil
}
