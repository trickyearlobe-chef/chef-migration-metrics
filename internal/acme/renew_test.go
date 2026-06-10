// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeIssuer struct {
	calls int
	err   error
}

func (f *fakeIssuer) Obtain(_ context.Context) ([]byte, []byte, error) {
	f.calls++
	if f.err != nil {
		return nil, nil, f.err
	}
	return []byte("cert"), []byte("key"), nil
}

func TestNextBackoff(t *testing.T) {
	cases := []struct {
		in, want time.Duration
	}{
		{0, time.Hour},
		{time.Hour, 2 * time.Hour},
		{2 * time.Hour, 4 * time.Hour},
		{12 * time.Hour, 24 * time.Hour},
		{16 * time.Hour, 24 * time.Hour},
		{24 * time.Hour, 24 * time.Hour},
	}
	for _, c := range cases {
		if got := nextBackoff(c.in); got != c.want {
			t.Errorf("nextBackoff(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestRenewalDue(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	cases := []struct {
		name      string
		notAfter  time.Time
		renewDays int
		want      bool
	}{
		{"40 days out, renew 30", now.Add(40 * day), 30, false},
		{"30 days out, renew 30 (boundary)", now.Add(30 * day), 30, true},
		{"20 days out, renew 30", now.Add(20 * day), 30, true},
		{"already expired", now.Add(-day), 30, true},
	}
	for _, c := range cases {
		if got := renewalDue(c.notAfter, now, c.renewDays); got != c.want {
			t.Errorf("%s: renewalDue = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestExpiryWarning(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	cases := []struct {
		name     string
		notAfter time.Time
		want     bool
	}{
		{"10 days out", now.Add(10 * day), false},
		{"7 days out (boundary)", now.Add(7 * day), true},
		{"3 days out", now.Add(3 * day), true},
		{"expired", now.Add(-day), true},
	}
	for _, c := range cases {
		if got := expiryWarning(c.notAfter, now); got != c.want {
			t.Errorf("%s: expiryWarning = %v, want %v", c.name, got, c.want)
		}
	}
}

// seedCert stores a certificate with the given expiry (and a dummy key) so the
// renewer can read its NotAfter.
func seedCert(t *testing.T, st *Storage, notAfter time.Time) {
	t.Helper()
	certPEM := encodeCertChain([][]byte{selfSignedDER(t, notAfter)})
	if err := st.SetCertificate(context.Background(), certPEM, []byte("-----BEGIN PRIVATE KEY-----\nx\n-----END PRIVATE KEY-----\n")); err != nil {
		t.Fatalf("seed cert: %v", err)
	}
}

func renewerFor(st *Storage, iss CertObtainer, now time.Time, warn WarnFunc) *Renewer {
	cfg := Config{Domains: []string{"a.example.com"}, RenewBeforeDays: 30}
	opts := []RenewerOption{WithClock(func() time.Time { return now })}
	if warn != nil {
		opts = append(opts, WithExpiryWarning(warn))
	}
	return NewRenewer(st, iss, cfg, nil, opts...)
}

func TestCheckOnceNoCertIssues(t *testing.T) {
	st := NewStorage(newFakeStore())
	iss := &fakeIssuer{}
	r := renewerFor(st, iss, time.Now(), nil)

	renewed, err := r.checkOnce(context.Background())
	if err != nil || !renewed {
		t.Fatalf("checkOnce = (%v, %v), want (true, nil)", renewed, err)
	}
	if iss.calls != 1 {
		t.Errorf("issuer called %d times, want 1", iss.calls)
	}
}

func TestCheckOnceHealthyCertDoesNothing(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	st := NewStorage(newFakeStore())
	seedCert(t, st, now.Add(60*24*time.Hour)) // 60 days out, not due
	iss := &fakeIssuer{}
	r := renewerFor(st, iss, now, nil)

	renewed, err := r.checkOnce(context.Background())
	if err != nil || renewed {
		t.Fatalf("checkOnce = (%v, %v), want (false, nil)", renewed, err)
	}
	if iss.calls != 0 {
		t.Errorf("issuer called %d times, want 0 for healthy cert", iss.calls)
	}
}

func TestCheckOnceDueRenews(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	st := NewStorage(newFakeStore())
	seedCert(t, st, now.Add(20*24*time.Hour)) // 20 days out, due (renew 30)
	iss := &fakeIssuer{}
	r := renewerFor(st, iss, now, nil)

	renewed, err := r.checkOnce(context.Background())
	if err != nil || !renewed {
		t.Fatalf("checkOnce = (%v, %v), want (true, nil)", renewed, err)
	}
	if iss.calls != 1 {
		t.Errorf("issuer called %d times, want 1", iss.calls)
	}
}

func TestCheckOnceFailureWithinWarnWindowWarns(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	st := NewStorage(newFakeStore())
	notAfter := now.Add(3 * 24 * time.Hour) // 3 days out: due AND within warn window
	seedCert(t, st, notAfter)
	iss := &fakeIssuer{err: errors.New("CA unreachable")}

	var warned *ExpiryWarning
	r := renewerFor(st, iss, now, func(w ExpiryWarning) { warned = &w })

	renewed, err := r.checkOnce(context.Background())
	if renewed || err == nil {
		t.Fatalf("checkOnce = (%v, %v), want (false, err)", renewed, err)
	}
	if warned == nil {
		t.Fatal("expected a certificate_expiry_warning")
	}
	if !warned.NotAfter.Equal(notAfter) || len(warned.Domains) != 1 {
		t.Errorf("warning payload wrong: %+v", warned)
	}
}

func TestCheckOnceFailureOutsideWarnWindowNoWarn(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	st := NewStorage(newFakeStore())
	seedCert(t, st, now.Add(20*24*time.Hour)) // due but 20 days out: no warn yet
	iss := &fakeIssuer{err: errors.New("CA unreachable")}

	var warned bool
	r := renewerFor(st, iss, now, func(ExpiryWarning) { warned = true })

	renewed, err := r.checkOnce(context.Background())
	if renewed || err == nil {
		t.Fatalf("checkOnce = (%v, %v), want (false, err)", renewed, err)
	}
	if warned {
		t.Error("should not warn outside the 7-day window")
	}
}
