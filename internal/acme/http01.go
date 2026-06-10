// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// ChallengePathPrefix is the well-known HTTP-01 challenge path. The CA fetches
// http://<domain>/.well-known/acme-challenge/<token> to validate domain control
// (tls-acme.md § 3.3). The redirect listener checks this prefix to give the
// challenge handler priority over the HTTPS redirect.
const ChallengePathPrefix = "/.well-known/acme-challenge/"

// HTTP01Solver publishes HTTP-01 challenge proofs in memory and serves them over
// plain HTTP via Handler(). It satisfies the Solver seam: Present records the
// token→keyAuth mapping the CA will fetch, CleanUp removes it. The handler is
// installed on the redirect listener (port 80) by the caller (tls-acme.md § 3.3).
// It holds no key material beyond the key authorization the engine computes — it
// never touches the account key.
type HTTP01Solver struct {
	mu     sync.RWMutex
	tokens map[string]string // token → key authorization
	log    LogFunc
}

// NewHTTP01Solver returns a solver with an empty challenge set.
func NewHTTP01Solver(log LogFunc) *HTTP01Solver {
	return &HTTP01Solver{tokens: map[string]string{}, log: log}
}

// Present records the key authorization to serve for ch.Token. It returns no
// error — publishing an HTTP-01 proof is a local map write with nothing to wait
// on (unlike DNS-01 propagation).
func (s *HTTP01Solver) Present(_ context.Context, ch Challenge) error {
	s.mu.Lock()
	s.tokens[ch.Token] = ch.KeyAuth
	s.mu.Unlock()
	logf(s.log, "DEBUG", "ACME http-01 challenge presented for %s", ch.Domain)
	return nil
}

// CleanUp removes the challenge mapping for ch.Token once the authorization has
// settled. It is idempotent — removing an absent token is a no-op.
func (s *HTTP01Solver) CleanUp(_ context.Context, ch Challenge) error {
	s.mu.Lock()
	delete(s.tokens, ch.Token)
	s.mu.Unlock()
	return nil
}

// Handler returns an http.Handler that serves presented challenges at
// /.well-known/acme-challenge/<token>. An unknown token (or a non-challenge
// path) yields 404 so the handler can be composed with a redirect fallback.
func (s *HTTP01Solver) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, ChallengePathPrefix) {
			http.NotFound(w, r)
			return
		}
		token := strings.TrimPrefix(r.URL.Path, ChallengePathPrefix)

		s.mu.RLock()
		keyAuth, ok := s.tokens[token]
		s.mu.RUnlock()
		if !ok {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(keyAuth))
	})
}
