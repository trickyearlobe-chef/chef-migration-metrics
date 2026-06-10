// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"bytes"
	"encoding/pem"
	"testing"
	"time"
)

// subjectsOf parses a reordered bundle and returns the per-cert subject CNs in
// stored order, so a test can assert the leaf → intermediate(s) → root layout.
func subjectsOf(t *testing.T, bundle []byte) []string {
	t.Helper()
	chain, err := ChainMetadataFromPEM(bundle)
	if err != nil {
		t.Fatalf("ChainMetadataFromPEM(reordered) = %v, want nil", err)
	}
	subs := make([]string, len(chain))
	for i, c := range chain {
		subs[i] = c.Subject
	}
	return subs
}

func TestReorderChainPEM_OutOfOrder(t *testing.T) {
	leafPEM, interPEM, rootPEM := generateTestChainPEM(t)
	// Operator pasted root → leaf → intermediate.
	bundle := bytes.Join([][]byte{rootPEM, leafPEM, interPEM}, nil)

	out, warn, err := ReorderChainPEM(bundle)
	if err != nil {
		t.Fatalf("ReorderChainPEM = %v, want nil", err)
	}
	if warn != "" {
		t.Errorf("warn = %q, want empty for a complete chain", warn)
	}
	got := subjectsOf(t, out)
	want := []string{"leaf.example.com", "intermediate.example.com", "root.example.com"}
	if !equalStrings(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestReorderChainPEM_ReverseOrder(t *testing.T) {
	leafPEM, interPEM, rootPEM := generateTestChainPEM(t)
	bundle := bytes.Join([][]byte{rootPEM, interPEM, leafPEM}, nil)

	out, warn, err := ReorderChainPEM(bundle)
	if err != nil {
		t.Fatalf("ReorderChainPEM = %v, want nil", err)
	}
	if warn != "" {
		t.Errorf("warn = %q, want empty", warn)
	}
	want := []string{"leaf.example.com", "intermediate.example.com", "root.example.com"}
	if got := subjectsOf(t, out); !equalStrings(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestReorderChainPEM_AlreadyOrdered(t *testing.T) {
	leafPEM, interPEM, rootPEM := generateTestChainPEM(t)
	bundle := bytes.Join([][]byte{leafPEM, interPEM, rootPEM}, nil)

	out, warn, err := ReorderChainPEM(bundle)
	if err != nil {
		t.Fatalf("ReorderChainPEM = %v, want nil", err)
	}
	if warn != "" {
		t.Errorf("warn = %q, want empty", warn)
	}
	want := []string{"leaf.example.com", "intermediate.example.com", "root.example.com"}
	if got := subjectsOf(t, out); !equalStrings(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestReorderChainPEM_NoRootIsComplete(t *testing.T) {
	// Root is optional (clients hold it). A leaf + intermediate, intermediate
	// first, must reorder to leaf → intermediate with NO warning.
	leafPEM, interPEM, _ := generateTestChainPEM(t)
	bundle := bytes.Join([][]byte{interPEM, leafPEM}, nil)

	out, warn, err := ReorderChainPEM(bundle)
	if err != nil {
		t.Fatalf("ReorderChainPEM = %v, want nil", err)
	}
	if warn != "" {
		t.Errorf("warn = %q, want empty (missing root is not a warning)", warn)
	}
	want := []string{"leaf.example.com", "intermediate.example.com"}
	if got := subjectsOf(t, out); !equalStrings(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestReorderChainPEM_MissingIntermediateWarnsLeafFirst(t *testing.T) {
	// Gap in the middle: leaf + root, no intermediate. The bundle cannot form a
	// single complete chain, so it is stored best-effort WITH a warning — but the
	// true (non-self-signed) leaf must still land first so the key still matches
	// cert[0] at preflight (fail-open: never reject a merely-incomplete chain).
	leafPEM, _, rootPEM := generateTestChainPEM(t)
	bundle := bytes.Join([][]byte{rootPEM, leafPEM}, nil)

	out, warn, err := ReorderChainPEM(bundle)
	if err != nil {
		t.Fatalf("ReorderChainPEM = %v, want nil", err)
	}
	if warn == "" {
		t.Error("warn = empty, want a non-fatal warning for the incomplete chain")
	}
	got := subjectsOf(t, out)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (both certs preserved)", len(got))
	}
	if got[0] != "leaf.example.com" {
		t.Errorf("first cert = %q, want leaf.example.com (key must match cert[0])", got[0])
	}
}

func TestReorderChainPEM_UnrelatedCertsWarn(t *testing.T) {
	// Two unrelated self-signed certs: not a chain. Both preserved, warning set.
	aPEM, _, _ := generateTestCertPEM(t, "a.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	bPEM, _, _ := generateTestCertPEM(t, "b.example.com",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	bundle := bytes.Join([][]byte{aPEM, bPEM}, nil)

	out, warn, err := ReorderChainPEM(bundle)
	if err != nil {
		t.Fatalf("ReorderChainPEM = %v, want nil", err)
	}
	if warn == "" {
		t.Error("warn = empty, want a warning for unrelated certs")
	}
	if got := subjectsOf(t, out); len(got) != 2 {
		t.Errorf("len = %d, want 2 (both preserved)", len(got))
	}
}

func TestReorderChainPEM_SingleCert(t *testing.T) {
	leafPEM, _, _ := generateTestChainPEM(t)

	out, warn, err := ReorderChainPEM(leafPEM)
	if err != nil {
		t.Fatalf("ReorderChainPEM = %v, want nil", err)
	}
	if warn != "" {
		t.Errorf("warn = %q, want empty for a single cert", warn)
	}
	if got := subjectsOf(t, out); len(got) != 1 || got[0] != "leaf.example.com" {
		t.Errorf("subjects = %v, want [leaf.example.com]", got)
	}
}

func TestReorderChainPEM_SkipsNonCertBlocks(t *testing.T) {
	leafPEM, interPEM, rootPEM := generateTestChainPEM(t)
	keyBlock := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("not-a-real-key")})
	bundle := bytes.Join([][]byte{rootPEM, keyBlock, interPEM, leafPEM}, nil)

	out, warn, err := ReorderChainPEM(bundle)
	if err != nil {
		t.Fatalf("ReorderChainPEM = %v, want nil", err)
	}
	if warn != "" {
		t.Errorf("warn = %q, want empty", warn)
	}
	// The key block must be dropped — only certs are re-encoded.
	if bytes.Contains(out, []byte("EC PRIVATE KEY")) {
		t.Error("output retains a non-certificate PEM block")
	}
	want := []string{"leaf.example.com", "intermediate.example.com", "root.example.com"}
	if got := subjectsOf(t, out); !equalStrings(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestReorderChainPEM_Unparseable(t *testing.T) {
	if _, _, err := ReorderChainPEM([]byte("garbage")); err == nil {
		t.Fatal("ReorderChainPEM(garbage) = nil, want error")
	}
}

func TestReorderChainPEM_Empty(t *testing.T) {
	if _, _, err := ReorderChainPEM(nil); err == nil {
		t.Fatal("ReorderChainPEM(nil) = nil, want error")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
