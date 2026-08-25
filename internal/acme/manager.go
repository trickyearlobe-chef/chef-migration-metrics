// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	xacme "golang.org/x/crypto/acme"
)

// ErrTOSNotAccepted is returned (and the issuance skipped) when agree_to_tos is
// not true. The caller treats this as fail-open, not fatal.
var ErrTOSNotAccepted = errors.New("acme: agree_to_tos must be true")

// Manager performs ACME account registration and the order/challenge flow,
// persisting the account key and issued cert/key to the DB-backed Storage. It is
// challenge-agnostic: the configured Solver publishes the proof. Renewal timing
// lives in Renewer (renew.go); Manager only obtains a certificate on demand.
type Manager struct {
	storage   *Storage
	solver    Solver
	cfg       Config
	log       LogFunc
	newClient newClientFunc
}

// ManagerOption configures a Manager.
type ManagerOption func(*Manager)

// WithLogger sets the logging callback.
func WithLogger(fn LogFunc) ManagerOption {
	return func(m *Manager) { m.log = fn }
}

// NewManager builds a Manager that issues certificates for cfg.Domains using the
// given storage and solver. By default it talks to the real CA at cfg.CAURL;
// tests override the client factory.
func NewManager(storage *Storage, solver Solver, cfg Config, opts ...ManagerOption) *Manager {
	m := &Manager{
		storage:   storage,
		solver:    solver,
		cfg:       cfg,
		newClient: realClientFactory(cfg.CAURL),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Obtain runs the full ACME flow for cfg.Domains — ensure an account, authorize
// each domain via the solver, finalize the order, and persist the issued
// cert/key to storage. It returns the issued leaf+chain PEM and key PEM.
//
// agree_to_tos must be true; otherwise it logs ERROR and returns
// ErrTOSNotAccepted so the caller falls open to plain HTTP.
func (m *Manager) Obtain(ctx context.Context) (certPEM, keyPEM []byte, err error) {
	if len(m.cfg.Domains) == 0 {
		return nil, nil, errors.New("acme: no domains configured")
	}
	if !m.cfg.AgreeToTOS {
		logf(m.log, "ERROR", "ACME issuance skipped: agree_to_tos is false — set it true to accept the CA Terms of Service")
		return nil, nil, ErrTOSNotAccepted
	}

	signer, isNew, err := m.account(ctx)
	if err != nil {
		return nil, nil, err
	}
	client := m.newClient(signer)
	if isNew {
		if err := m.register(ctx, client, signer); err != nil {
			return nil, nil, err
		}
	}
	return m.order(ctx, client)
}

// account loads the persisted account key, or generates a fresh one (flagged
// new so the caller registers it). A new key is not persisted until registration
// succeeds, so a failed registration does not strand an unregistered key.
func (m *Manager) account(ctx context.Context) (signer crypto.Signer, isNew bool, err error) {
	signer, err = m.storage.AccountKey(ctx)
	if err == nil {
		return signer, false, nil
	}
	if !errors.Is(err, ErrNotStored) {
		return nil, false, fmt.Errorf("acme: load account key: %w", err)
	}
	key, err := newECKey()
	if err != nil {
		return nil, false, err
	}
	return key, true, nil
}

// register creates the ACME account (accepting the CA ToS) and persists the
// account key on success. An already-registered key is treated as success.
func (m *Manager) register(ctx context.Context, client acmeClient, signer crypto.Signer) error {
	acct := &xacme.Account{}
	if m.cfg.Email != "" {
		acct.Contact = []string{"mailto:" + m.cfg.Email}
	}
	if _, err := client.Register(ctx, acct, xacme.AcceptTOS); err != nil &&
		!errors.Is(err, xacme.ErrAccountAlreadyExists) {
		return fmt.Errorf("acme: register account: %w", err)
	}
	if err := m.storage.SetAccountKey(ctx, signer); err != nil {
		return err
	}
	logf(m.log, "INFO", "ACME account registered")
	return nil
}

// order authorizes every domain, finalizes the order with a freshly generated
// certificate key, and persists the issued cert/key.
func (m *Manager) order(ctx context.Context, client acmeClient) (certPEM, keyPEM []byte, err error) {
	order, err := client.AuthorizeOrder(ctx, xacme.DomainIDs(m.cfg.Domains...))
	if err != nil {
		return nil, nil, fmt.Errorf("acme: authorize order: %w", err)
	}
	for _, authzURL := range order.AuthzURLs {
		if err := m.solveAuthz(ctx, client, authzURL); err != nil {
			return nil, nil, err
		}
	}

	certKey, err := newECKey()
	if err != nil {
		return nil, nil, err
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader,
		&x509.CertificateRequest{DNSNames: m.cfg.Domains}, certKey)
	if err != nil {
		return nil, nil, fmt.Errorf("acme: build CSR: %w", err)
	}
	ders, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csrDER, true)
	if err != nil {
		return nil, nil, fmt.Errorf("acme: finalize order: %w", err)
	}

	certPEM = encodeCertChain(ders)
	keyPEM, err = marshalPrivateKey(certKey)
	if err != nil {
		return nil, nil, err
	}
	if err := m.storage.SetCertificate(ctx, certPEM, keyPEM); err != nil {
		return nil, nil, err
	}

	if notAfter, perr := leafNotAfter(certPEM); perr == nil {
		logf(m.log, "INFO", "ACME certificate issued for %v, expires %s",
			m.cfg.Domains, notAfter.Format(time.RFC3339))
	} else {
		logf(m.log, "INFO", "ACME certificate issued for %v", m.cfg.Domains)
	}
	return certPEM, keyPEM, nil
}

// solveAuthz drives one authorization: pick the configured challenge, compute
// its proof, present it via the solver, accept, and wait for validation. CleanUp
// always runs once the authorization settles.
func (m *Manager) solveAuthz(ctx context.Context, client acmeClient, authzURL string) error {
	authz, err := client.GetAuthorization(ctx, authzURL)
	if err != nil {
		return fmt.Errorf("acme: get authorization: %w", err)
	}
	if authz.Status == xacme.StatusValid {
		return nil
	}

	chal := pickChallenge(authz.Challenges, m.cfg.Challenge)
	if chal == nil {
		return fmt.Errorf("acme: CA offered no %s challenge for %s", m.cfg.Challenge, authz.Identifier.Value)
	}

	ch := Challenge{Type: chal.Type, Domain: authz.Identifier.Value, Token: chal.Token}
	switch m.cfg.Challenge {
	case "http-01":
		ch.KeyAuth, err = client.HTTP01ChallengeResponse(chal.Token)
	case "dns-01":
		ch.DNSValue, err = client.DNS01ChallengeRecord(chal.Token)
	default:
		return fmt.Errorf("acme: unsupported challenge type %q", m.cfg.Challenge)
	}
	if err != nil {
		return fmt.Errorf("acme: compute challenge response: %w", err)
	}

	if err := m.solver.Present(ctx, ch); err != nil {
		return fmt.Errorf("acme: present challenge: %w", err)
	}
	defer func() {
		if cerr := m.solver.CleanUp(ctx, ch); cerr != nil {
			logf(m.log, "WARN", "ACME challenge cleanup for %s failed: %v", ch.Domain, cerr)
		}
	}()

	if _, err := client.Accept(ctx, chal); err != nil {
		return fmt.Errorf("acme: accept challenge: %w", err)
	}
	if _, err := client.WaitAuthorization(ctx, authzURL); err != nil {
		return fmt.Errorf("acme: authorization for %s failed: %w", ch.Domain, err)
	}
	return nil
}

// pickChallenge returns the first challenge matching typ, or nil.
func pickChallenge(chals []*xacme.Challenge, typ string) *xacme.Challenge {
	for _, c := range chals {
		if c.Type == typ {
			return c
		}
	}
	return nil
}

// newECKey generates a fresh ECDSA P-256 private key (the default key algorithm,
// matching the CSR path.4).
func newECKey() (*ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("acme: generate key: %w", err)
	}
	return key, nil
}

// encodeCertChain PEM-encodes a DER certificate chain (leaf first).
func encodeCertChain(ders [][]byte) []byte {
	var buf bytes.Buffer
	for _, der := range ders {
		_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	return buf.Bytes()
}

// leafNotAfter parses the first certificate in a PEM chain and returns its
// NotAfter. It never inspects key material.
func leafNotAfter(certPEM []byte) (time.Time, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}, errors.New("acme: no certificate PEM block found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("acme: parse certificate: %w", err)
	}
	return cert.NotAfter, nil
}
