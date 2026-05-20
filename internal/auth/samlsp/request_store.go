// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package samlsp

import (
	"sync"
	"time"
)

// requestStore is a thread-safe, time-limited store for SAML AuthnRequest IDs.
// It is used for InResponseTo validation to prevent replay attacks.
type requestStore struct {
	mu      sync.Mutex
	entries map[string]time.Time
	ttl     time.Duration
}

// newRequestStore creates a new request store with the given TTL for entries.
func newRequestStore(ttl time.Duration) *requestStore {
	return &requestStore{
		entries: make(map[string]time.Time),
		ttl:     ttl,
	}
}

// Store adds a request ID to the store with the current timestamp.
func (s *requestStore) Store(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[id] = time.Now()
	s.cleanup()
}

// Delete removes a request ID from the store (called after successful validation).
func (s *requestStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, id)
}

// possibleRequestIDs returns all non-expired request IDs. This is passed to
// the crewjam/saml ParseResponse for InResponseTo matching.
func (s *requestStore) possibleRequestIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanup()

	ids := make([]string, 0, len(s.entries))
	for id := range s.entries {
		ids = append(ids, id)
	}
	return ids
}

// cleanup removes expired entries. Must be called with mu held.
func (s *requestStore) cleanup() {
	cutoff := time.Now().Add(-s.ttl)
	for id, ts := range s.entries {
		if ts.Before(cutoff) {
			delete(s.entries, id)
		}
	}
}
