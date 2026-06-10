// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTP01SolverServesPresentedChallenge(t *testing.T) {
	s := NewHTTP01Solver(nil)
	ch := Challenge{Type: "http-01", Domain: "metrics.example.com", Token: "tok-123", KeyAuth: "tok-123.keyauth"}
	if err := s.Present(context.Background(), ch); err != nil {
		t.Fatalf("Present: %v", err)
	}

	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/.well-known/acme-challenge/tok-123")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "tok-123.keyauth" {
		t.Errorf("body = %q, want the key authorization", string(body))
	}
}

func TestHTTP01SolverUnknownTokenIs404(t *testing.T) {
	s := NewHTTP01Solver(nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/.well-known/acme-challenge/nope")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for unknown token", resp.StatusCode)
	}
}

func TestHTTP01SolverCleanUpRemovesToken(t *testing.T) {
	s := NewHTTP01Solver(nil)
	ch := Challenge{Type: "http-01", Domain: "metrics.example.com", Token: "tok-abc", KeyAuth: "tok-abc.keyauth"}
	ctx := context.Background()
	if err := s.Present(ctx, ch); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if err := s.CleanUp(ctx, ch); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}

	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/.well-known/acme-challenge/tok-abc")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 after cleanup", resp.StatusCode)
	}
}

// Concurrent challenges (multiple domains) are served independently.
func TestHTTP01SolverMultipleTokens(t *testing.T) {
	s := NewHTTP01Solver(nil)
	ctx := context.Background()
	for _, ch := range []Challenge{
		{Token: "t1", KeyAuth: "t1.auth"},
		{Token: "t2", KeyAuth: "t2.auth"},
	} {
		if err := s.Present(ctx, ch); err != nil {
			t.Fatalf("Present: %v", err)
		}
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for tok, want := range map[string]string{"t1": "t1.auth", "t2": "t2.auth"} {
		resp, err := http.Get(srv.URL + "/.well-known/acme-challenge/" + tok)
		if err != nil {
			t.Fatalf("GET %s: %v", tok, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) != want {
			t.Errorf("token %s body = %q, want %q", tok, string(body), want)
		}
	}
}

// Non-challenge paths are not served by the solver handler (404), so the
// caller can compose it with a redirect fallback (challenge > redirect).
func TestHTTP01SolverIgnoresNonChallengePaths(t *testing.T) {
	s := NewHTTP01Solver(nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for non-challenge path", resp.StatusCode)
	}
}
