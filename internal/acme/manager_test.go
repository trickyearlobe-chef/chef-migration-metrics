// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	xacme "golang.org/x/crypto/acme"
)

// --- test doubles -----------------------------------------------------------

type fakeSolver struct {
	presented  []Challenge
	cleaned    []Challenge
	presentErr error
}

func (f *fakeSolver) Present(_ context.Context, ch Challenge) error {
	f.presented = append(f.presented, ch)
	return f.presentErr
}

func (f *fakeSolver) CleanUp(_ context.Context, ch Challenge) error {
	f.cleaned = append(f.cleaned, ch)
	return nil
}

// fakeClient is a scripted acmeClient: it returns one pending authorization per
// domain (challenge type = challengeType), records what it was asked to do, and
// hands back certDER on finalize. No network.
type fakeClient struct {
	challengeType string
	certDER       [][]byte
	waitErr       error

	registerCalls int
	accepted      []*xacme.Challenge
	finalizeCSR   []byte

	authzByURL map[string]*xacme.Authorization
}

func newFakeClient(domains []string, challengeType string, certDER [][]byte) *fakeClient {
	fc := &fakeClient{
		challengeType: challengeType,
		certDER:       certDER,
		authzByURL:    map[string]*xacme.Authorization{},
	}
	for i, d := range domains {
		url := "https://ca.test/authz/" + d
		fc.authzByURL[url] = &xacme.Authorization{
			Status:     xacme.StatusPending,
			Identifier: xacme.AuthzID{Type: "dns", Value: d},
			Challenges: []*xacme.Challenge{
				{Type: "http-01", Token: "http-tok-" + d, URI: "https://ca.test/chal/h/" + d},
				{Type: "dns-01", Token: "dns-tok-" + d, URI: "https://ca.test/chal/d/" + d},
			},
		}
		_ = i
	}
	return fc
}

func (f *fakeClient) authzURLs() []string {
	urls := make([]string, 0, len(f.authzByURL))
	for u := range f.authzByURL {
		urls = append(urls, u)
	}
	return urls
}

func (f *fakeClient) Register(_ context.Context, _ *xacme.Account, _ func(string) bool) (*xacme.Account, error) {
	f.registerCalls++
	return &xacme.Account{URI: "https://ca.test/acct/1"}, nil
}

func (f *fakeClient) AuthorizeOrder(_ context.Context, _ []xacme.AuthzID, _ ...xacme.OrderOption) (*xacme.Order, error) {
	return &xacme.Order{
		URI:         "https://ca.test/order/1",
		Status:      xacme.StatusPending,
		AuthzURLs:   f.authzURLs(),
		FinalizeURL: "https://ca.test/order/1/finalize",
	}, nil
}

func (f *fakeClient) GetAuthorization(_ context.Context, url string) (*xacme.Authorization, error) {
	a, ok := f.authzByURL[url]
	if !ok {
		return nil, errors.New("unknown authz url")
	}
	return a, nil
}

func (f *fakeClient) Accept(_ context.Context, chal *xacme.Challenge) (*xacme.Challenge, error) {
	f.accepted = append(f.accepted, chal)
	return chal, nil
}

func (f *fakeClient) WaitAuthorization(_ context.Context, _ string) (*xacme.Authorization, error) {
	if f.waitErr != nil {
		return nil, f.waitErr
	}
	return &xacme.Authorization{Status: xacme.StatusValid}, nil
}

func (f *fakeClient) CreateOrderCert(_ context.Context, _ string, csr []byte, _ bool) ([][]byte, string, error) {
	f.finalizeCSR = csr
	return f.certDER, "https://ca.test/cert/1", nil
}

func (f *fakeClient) HTTP01ChallengeResponse(token string) (string, error) {
	return "keyauth-" + token, nil
}

func (f *fakeClient) DNS01ChallengeRecord(token string) (string, error) {
	return "dnsvalue-" + token, nil
}

// selfSignedDER produces a parseable DER certificate for use as the CA's
// "issued" cert in tests.
func selfSignedDER(t *testing.T, notAfter time.Time) []byte {
	t.Helper()
	key := newTestKey(t)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "metrics.example.com"},
		NotBefore:    notAfter.Add(-90 * 24 * time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{"metrics.example.com"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return der
}

// newTestManager wires a Manager around the fake client/solver and storage.
func newTestManager(t *testing.T, cfg Config, fc *fakeClient, solver Solver) (*Manager, *Storage) {
	t.Helper()
	st := NewStorage(newFakeStore())
	m := NewManager(st, solver, cfg)
	m.newClient = func(_ crypto.Signer) acmeClient { return fc }
	return m, st
}

// --- tests ------------------------------------------------------------------

func baseConfig(domains []string, challenge string) Config {
	return Config{
		Domains:         domains,
		Email:           "admin@example.com",
		CAURL:           "https://ca.test/directory",
		Challenge:       challenge,
		RenewBeforeDays: 30,
		AgreeToTOS:      true,
	}
}

func TestObtainHTTP01NewAccount(t *testing.T) {
	domains := []string{"a.example.com", "b.example.com"}
	fc := newFakeClient(domains, "http-01", [][]byte{selfSignedDER(t, time.Now().Add(90*24*time.Hour))})
	solver := &fakeSolver{}
	m, st := newTestManager(t, baseConfig(domains, "http-01"), fc, solver)
	ctx := context.Background()

	certPEM, keyPEM, err := m.Obtain(ctx)
	if err != nil {
		t.Fatalf("Obtain: %v", err)
	}
	if fc.registerCalls != 1 {
		t.Errorf("Register calls = %d, want 1 (new account)", fc.registerCalls)
	}
	if len(solver.presented) != 2 || len(solver.cleaned) != 2 {
		t.Errorf("solver presented=%d cleaned=%d, want 2/2", len(solver.presented), len(solver.cleaned))
	}
	for _, ch := range solver.presented {
		if ch.Type != "http-01" {
			t.Errorf("challenge type = %q, want http-01", ch.Type)
		}
		if ch.KeyAuth != "keyauth-http-tok-"+ch.Domain {
			t.Errorf("KeyAuth = %q for %s", ch.KeyAuth, ch.Domain)
		}
		if ch.DNSValue != "" {
			t.Errorf("http-01 challenge has DNSValue %q", ch.DNSValue)
		}
	}
	if len(fc.accepted) != 2 {
		t.Errorf("accepted %d challenges, want 2", len(fc.accepted))
	}

	// Issued material persisted and round-trips.
	gotCert, gotKey, err := st.Certificate(ctx)
	if err != nil {
		t.Fatalf("stored Certificate: %v", err)
	}
	if string(gotCert) != string(certPEM) || string(gotKey) != string(keyPEM) {
		t.Error("stored cert/key does not match returned material")
	}
	if block, _ := pem.Decode(gotCert); block == nil {
		t.Error("stored cert is not valid PEM")
	}
	// Account key persisted.
	if _, err := st.AccountKey(ctx); err != nil {
		t.Errorf("account key not persisted: %v", err)
	}
}

func TestObtainExistingAccountSkipsRegister(t *testing.T) {
	domains := []string{"a.example.com"}
	fc := newFakeClient(domains, "http-01", [][]byte{selfSignedDER(t, time.Now().Add(90*24*time.Hour))})
	m, st := newTestManager(t, baseConfig(domains, "http-01"), fc, &fakeSolver{})
	ctx := context.Background()

	// Pre-register an account key.
	if err := st.SetAccountKey(ctx, newTestKey(t)); err != nil {
		t.Fatalf("seed account key: %v", err)
	}
	if _, _, err := m.Obtain(ctx); err != nil {
		t.Fatalf("Obtain: %v", err)
	}
	if fc.registerCalls != 0 {
		t.Errorf("Register calls = %d, want 0 (existing account)", fc.registerCalls)
	}
}

func TestObtainDNS01PopulatesDNSValue(t *testing.T) {
	domains := []string{"a.example.com"}
	fc := newFakeClient(domains, "dns-01", [][]byte{selfSignedDER(t, time.Now().Add(90*24*time.Hour))})
	solver := &fakeSolver{}
	m, _ := newTestManager(t, baseConfig(domains, "dns-01"), fc, solver)

	if _, _, err := m.Obtain(context.Background()); err != nil {
		t.Fatalf("Obtain: %v", err)
	}
	if len(solver.presented) != 1 {
		t.Fatalf("presented %d, want 1", len(solver.presented))
	}
	ch := solver.presented[0]
	if ch.Type != "dns-01" || ch.KeyAuth != "" || ch.DNSValue != "dnsvalue-dns-tok-a.example.com" {
		t.Errorf("dns-01 challenge wrong: %+v", ch)
	}
}

func TestObtainToSNotAccepted(t *testing.T) {
	domains := []string{"a.example.com"}
	fc := newFakeClient(domains, "http-01", nil)
	cfg := baseConfig(domains, "http-01")
	cfg.AgreeToTOS = false
	m, _ := newTestManager(t, cfg, fc, &fakeSolver{})

	_, _, err := m.Obtain(context.Background())
	if !errors.Is(err, ErrTOSNotAccepted) {
		t.Fatalf("want ErrTOSNotAccepted, got %v", err)
	}
	if fc.registerCalls != 0 {
		t.Error("no ACME calls should be made when ToS not accepted")
	}
}

func TestObtainNoMatchingChallenge(t *testing.T) {
	domains := []string{"a.example.com"}
	fc := newFakeClient(domains, "http-01", nil)
	// Strip all challenges so none match.
	for _, a := range fc.authzByURL {
		a.Challenges = nil
	}
	m, _ := newTestManager(t, baseConfig(domains, "http-01"), fc, &fakeSolver{})

	if _, _, err := m.Obtain(context.Background()); err == nil {
		t.Fatal("want error when no matching challenge offered")
	}
}

func TestObtainCleanupRunsOnWaitFailure(t *testing.T) {
	domains := []string{"a.example.com"}
	fc := newFakeClient(domains, "http-01", nil)
	fc.waitErr = errors.New("authz failed")
	solver := &fakeSolver{}
	m, _ := newTestManager(t, baseConfig(domains, "http-01"), fc, solver)

	if _, _, err := m.Obtain(context.Background()); err == nil {
		t.Fatal("want error when authorization fails")
	}
	if len(solver.cleaned) != 1 {
		t.Errorf("cleanup ran %d times, want 1 even on failure", len(solver.cleaned))
	}
}
