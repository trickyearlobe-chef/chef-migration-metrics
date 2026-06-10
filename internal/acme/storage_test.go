// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// fakeStore is an in-memory SecretStore for the acme package tests. It mirrors
// the relevant configstore semantics: Get returns any entry, GetSecret returns
// only secret entries (ErrNotSecret otherwise), and missing keys yield
// configstore.ErrNotFound.
type fakeStore struct {
	values  map[string]json.RawMessage
	secrets map[string]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		values:  map[string]json.RawMessage{},
		secrets: map[string]bool{},
	}
}

func (f *fakeStore) Get(_ context.Context, key string) (json.RawMessage, error) {
	v, ok := f.values[key]
	if !ok {
		return nil, configstore.ErrNotFound
	}
	return v, nil
}

func (f *fakeStore) GetSecret(_ context.Context, key string) (json.RawMessage, error) {
	v, ok := f.values[key]
	if !ok {
		return nil, configstore.ErrNotFound
	}
	if !f.secrets[key] {
		return nil, configstore.ErrNotSecret
	}
	return v, nil
}

func (f *fakeStore) Set(_ context.Context, key string, value json.RawMessage, secret bool, _ string) error {
	f.values[key] = value
	f.secrets[key] = secret
	return nil
}

func (f *fakeStore) Delete(_ context.Context, key string) error {
	delete(f.values, key)
	delete(f.secrets, key)
	return nil
}

func newTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func TestStorageAccountKeyRoundTrip(t *testing.T) {
	st := NewStorage(newFakeStore())
	ctx := context.Background()

	if _, err := st.AccountKey(ctx); !errors.Is(err, ErrNotStored) {
		t.Fatalf("AccountKey before set: want ErrNotStored, got %v", err)
	}

	key := newTestKey(t)
	if err := st.SetAccountKey(ctx, key); err != nil {
		t.Fatalf("SetAccountKey: %v", err)
	}

	got, err := st.AccountKey(ctx)
	if err != nil {
		t.Fatalf("AccountKey: %v", err)
	}
	gotEC, ok := got.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("AccountKey returned %T, want *ecdsa.PrivateKey", got)
	}
	if !gotEC.Equal(key) {
		t.Fatal("round-tripped account key does not match original")
	}
}

func TestStorageAccountKeyIsSecret(t *testing.T) {
	fs := newFakeStore()
	st := NewStorage(fs)
	if err := st.SetAccountKey(context.Background(), newTestKey(t)); err != nil {
		t.Fatalf("SetAccountKey: %v", err)
	}
	if !fs.secrets[configstore.KeyServerTLSACMEAccountKey] {
		t.Fatal("account key must be stored as a secret")
	}
}

func TestStorageCertificateRoundTrip(t *testing.T) {
	fs := newFakeStore()
	st := NewStorage(fs)
	ctx := context.Background()

	if _, _, err := st.Certificate(ctx); !errors.Is(err, ErrNotStored) {
		t.Fatalf("Certificate before set: want ErrNotStored, got %v", err)
	}

	certPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n")
	keyPEM := []byte("-----BEGIN PRIVATE KEY-----\nMIGH\n-----END PRIVATE KEY-----\n")
	if err := st.SetCertificate(ctx, certPEM, keyPEM); err != nil {
		t.Fatalf("SetCertificate: %v", err)
	}

	gotCert, gotKey, err := st.Certificate(ctx)
	if err != nil {
		t.Fatalf("Certificate: %v", err)
	}
	if string(gotCert) != string(certPEM) {
		t.Errorf("cert PEM mismatch: got %q", gotCert)
	}
	if string(gotKey) != string(keyPEM) {
		t.Errorf("key PEM mismatch: got %q", gotKey)
	}

	if fs.secrets[configstore.KeyServerTLSACMECert] {
		t.Error("issued certificate must be non-secret (public)")
	}
	if !fs.secrets[configstore.KeyServerTLSACMEKey] {
		t.Error("issued private key must be secret")
	}
}

func TestStorageCertificateHalfPairIsNotStored(t *testing.T) {
	fs := newFakeStore()
	st := NewStorage(fs)
	ctx := context.Background()

	// Cert present but key absent → ErrNotStored (no usable certificate).
	if err := st.setPEM(ctx, configstore.KeyServerTLSACMECert, []byte("cert"), false); err != nil {
		t.Fatalf("setPEM: %v", err)
	}
	if _, _, err := st.Certificate(ctx); !errors.Is(err, ErrNotStored) {
		t.Fatalf("Certificate with missing key: want ErrNotStored, got %v", err)
	}
}
