// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package acme implements the core ACME (RFC 8555) certificate management
// engine for mode: acme — account registration, the order/challenge flow via
// golang.org/x/crypto/acme, DB-backed (encrypted) state storage, and the
// renewal scheduler. Challenge solvers (HTTP-01, Route 53 DNS-01) and the
// listener/mode wiring live in sibling files/chunks behind the Solver seam.
//
// No higher-level ACME library is used: the engine owns account/order/challenge/
// renewal orchestration directly to keep the dependency/lockfile surface minimal
// (tls-acme.md § 3.1).
package acme

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// ErrNotStored is returned by Storage accessors when the requested ACME
// material (account key or issued certificate) is not yet present in the
// config store. Callers treat this as "needs issuance", not a hard error.
var ErrNotStored = errors.New("acme: not stored")

// SecretStore is the subset of the encrypted config store that the ACME engine
// needs. *configstore.Store satisfies it; tests supply an in-memory fake. Keys
// flagged secret are encrypted at rest and never returned by any API.
type SecretStore interface {
	Get(ctx context.Context, key string) (json.RawMessage, error)
	GetSecret(ctx context.Context, key string) (json.RawMessage, error)
	Set(ctx context.Context, key string, value json.RawMessage, secret bool, updatedBy string) error
	Delete(ctx context.Context, key string) error
}

// Storage persists ACME state — the account key, issued certificate, and issued
// private key — to the encrypted config store (tls-acme.md § 3.5). PEM material
// is stored as a JSON-encoded string, matching the static cert_source: db path.
// The account key and issued private key are secret (secret: true); the issued
// certificate is public (secret: false). Private keys are never returned by any
// API.
type Storage struct {
	store SecretStore
}

// NewStorage wraps a SecretStore (the encrypted config store) for ACME state.
func NewStorage(store SecretStore) *Storage {
	return &Storage{store: store}
}

// updatedBy is the audit actor recorded for config-store writes made by the
// engine itself (as opposed to an operator via the admin API).
const updatedBy = "acme"

// AccountKey returns the persisted ACME account private key, or ErrNotStored if
// no account has been registered yet.
func (s *Storage) AccountKey(ctx context.Context) (crypto.Signer, error) {
	keyPEM, err := s.getPEMSecret(ctx, configstore.KeyServerTLSACMEAccountKey)
	if err != nil {
		return nil, err
	}
	return parsePrivateKey(keyPEM)
}

// SetAccountKey persists the ACME account private key as an encrypted secret.
func (s *Storage) SetAccountKey(ctx context.Context, key crypto.Signer) error {
	keyPEM, err := marshalPrivateKey(key)
	if err != nil {
		return err
	}
	return s.setPEM(ctx, configstore.KeyServerTLSACMEAccountKey, keyPEM, true)
}

// Certificate returns the persisted issued certificate (leaf + chain) PEM and
// its private key PEM. It returns ErrNotStored unless BOTH are present — a half
// pair is treated as no usable certificate so the listener falls open
// (tls-acme.md § 3.11) rather than serving a cert without its key.
func (s *Storage) Certificate(ctx context.Context) (certPEM, keyPEM []byte, err error) {
	certPEM, err = s.getPEM(ctx, configstore.KeyServerTLSACMECert)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err = s.getPEMSecret(ctx, configstore.KeyServerTLSACMEKey)
	if err != nil {
		return nil, nil, err
	}
	return certPEM, keyPEM, nil
}

// SetCertificate persists an issued certificate (public) and its private key
// (secret) atomically from the engine's point of view — the cert is written
// first, then the key, both as JSON-encoded PEM strings.
func (s *Storage) SetCertificate(ctx context.Context, certPEM, keyPEM []byte) error {
	if err := s.setPEM(ctx, configstore.KeyServerTLSACMECert, certPEM, false); err != nil {
		return err
	}
	return s.setPEM(ctx, configstore.KeyServerTLSACMEKey, keyPEM, true)
}

// getPEM reads a non-secret JSON-encoded PEM string, translating a missing key
// into ErrNotStored.
func (s *Storage) getPEM(ctx context.Context, key string) ([]byte, error) {
	raw, err := s.store.Get(ctx, key)
	return decodePEMValue(raw, err)
}

// getPEMSecret reads a secret JSON-encoded PEM string, translating a missing
// key into ErrNotStored.
func (s *Storage) getPEMSecret(ctx context.Context, key string) ([]byte, error) {
	raw, err := s.store.GetSecret(ctx, key)
	return decodePEMValue(raw, err)
}

// setPEM writes PEM bytes as a JSON-encoded string under key.
func (s *Storage) setPEM(ctx context.Context, key string, pemBytes []byte, secret bool) error {
	value, err := json.Marshal(string(pemBytes))
	if err != nil {
		return fmt.Errorf("acme: encode %s: %w", key, err)
	}
	if err := s.store.Set(ctx, key, value, secret, updatedBy); err != nil {
		return fmt.Errorf("acme: store %s: %w", key, err)
	}
	return nil
}

// decodePEMValue unwraps a config-store read: a not-found error becomes
// ErrNotStored, and the JSON string value is decoded back to PEM bytes.
func decodePEMValue(raw json.RawMessage, err error) ([]byte, error) {
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			return nil, ErrNotStored
		}
		return nil, err
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("acme: decode stored PEM: %w", err)
	}
	return []byte(s), nil
}

// marshalPrivateKey encodes a private key as PKCS#8 PEM ("PRIVATE KEY").
func marshalPrivateKey(key crypto.Signer) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("acme: marshal private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// parsePrivateKey decodes a PKCS#8 PEM private key into a crypto.Signer.
func parsePrivateKey(keyPEM []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("acme: no PEM private key block found")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("acme: parse private key: %w", err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, errors.New("acme: stored key is not a signer")
	}
	return signer, nil
}
