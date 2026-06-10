// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"errors"
	"time"
)

// Renewal timing constants (tls-acme.md § 3.6).
const (
	backoffInitial = time.Hour
	backoffCap     = 24 * time.Hour
	// expiryWarnWindow is how close to expiry, without a successful renewal,
	// triggers a WARN and a certificate_expiry_warning event.
	expiryWarnWindow = 7 * 24 * time.Hour
	// defaultCheckInterval is the steady-state poll between renewal checks when
	// the certificate is healthy.
	defaultCheckInterval = 12 * time.Hour
)

// CertObtainer obtains and persists a fresh certificate. *Manager implements it;
// tests supply a fake so renewal timing is exercised without ACME calls.
type CertObtainer interface {
	Obtain(ctx context.Context) (certPEM, keyPEM []byte, err error)
}

// ExpiryWarning is the payload of a certificate_expiry_warning notification: the
// affected domains and the imminent expiry. It carries no key material.
type ExpiryWarning struct {
	Domains  []string
	NotAfter time.Time
}

// WarnFunc receives a certificate_expiry_warning when one fires (if notifications
// are configured). The acme package does not depend on the notification system
// directly — the caller wires this seam.
type WarnFunc func(ExpiryWarning)

// Renewer drives certificate renewal: it polls the stored certificate's expiry,
// renews before the configured window, applies exponential backoff on failure,
// and emits expiry warnings (tls-acme.md § 3.6). It holds no certificate state
// itself — the issued material lives in Storage.
type Renewer struct {
	storage       *Storage
	issuer        CertObtainer
	cfg           Config
	log           LogFunc
	warn          WarnFunc
	now           func() time.Time
	checkInterval time.Duration
}

// RenewerOption configures a Renewer.
type RenewerOption func(*Renewer)

// WithClock overrides the time source (tests inject a fixed clock).
func WithClock(fn func() time.Time) RenewerOption {
	return func(r *Renewer) { r.now = fn }
}

// WithExpiryWarning sets the notification callback for imminent-expiry warnings.
func WithExpiryWarning(fn WarnFunc) RenewerOption {
	return func(r *Renewer) { r.warn = fn }
}

// WithCheckInterval overrides the steady-state poll interval (mainly for tests).
func WithCheckInterval(d time.Duration) RenewerOption {
	return func(r *Renewer) { r.checkInterval = d }
}

// NewRenewer builds a Renewer around the storage, issuer, and config.
func NewRenewer(storage *Storage, issuer CertObtainer, cfg Config, log LogFunc, opts ...RenewerOption) *Renewer {
	r := &Renewer{
		storage:       storage,
		issuer:        issuer,
		cfg:           cfg,
		log:           log,
		now:           time.Now,
		checkInterval: defaultCheckInterval,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run blocks, periodically checking and renewing the certificate until ctx is
// cancelled. On a healthy certificate it sleeps checkInterval; on a renewal
// failure it backs off exponentially (1h → 24h cap) before retrying.
func (r *Renewer) Run(ctx context.Context) {
	backoff := time.Duration(0)
	for {
		_, err := r.checkOnce(ctx)
		sleep := r.checkInterval
		if err != nil {
			backoff = nextBackoff(backoff)
			sleep = backoff
		} else {
			backoff = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(sleep):
		}
	}
}

// checkOnce performs a single renewal evaluation: issue if no cert exists yet,
// renew if within the renewal window, and warn if within the expiry window
// without a successful renewal. It returns whether a (re)issue happened and any
// error from the attempt (nil when the certificate is healthy and not due).
func (r *Renewer) checkOnce(ctx context.Context) (renewed bool, err error) {
	now := r.now()

	certPEM, _, cerr := r.storage.Certificate(ctx)
	if errors.Is(cerr, ErrNotStored) {
		if _, _, oerr := r.issuer.Obtain(ctx); oerr != nil {
			logf(r.log, "ERROR", "ACME initial issuance failed: %v", oerr)
			return false, oerr
		}
		return true, nil
	}
	if cerr != nil {
		logf(r.log, "ERROR", "ACME renewal: cannot read stored certificate: %v", cerr)
		return false, cerr
	}

	notAfter, perr := leafNotAfter(certPEM)
	if perr != nil {
		logf(r.log, "ERROR", "ACME renewal: cannot parse stored certificate: %v", perr)
		return false, perr
	}

	var renewErr error
	if renewalDue(notAfter, now, r.cfg.RenewBeforeDays) {
		if _, _, oerr := r.issuer.Obtain(ctx); oerr == nil {
			return true, nil
		} else {
			renewErr = oerr
			logf(r.log, "ERROR", "ACME renewal failed (current certificate expires %s): %v",
				notAfter.Format(time.RFC3339), oerr)
		}
	}

	if expiryWarning(notAfter, now) {
		logf(r.log, "WARN", "ACME certificate for %v expires %s and has not been renewed",
			r.cfg.Domains, notAfter.Format(time.RFC3339))
		if r.warn != nil {
			r.warn(ExpiryWarning{Domains: r.cfg.Domains, NotAfter: notAfter})
		}
	}
	return false, renewErr
}

// renewalDue reports whether now is at or past (notAfter - renewBeforeDays).
func renewalDue(notAfter, now time.Time, renewBeforeDays int) bool {
	threshold := notAfter.Add(-time.Duration(renewBeforeDays) * 24 * time.Hour)
	return !now.Before(threshold)
}

// expiryWarning reports whether now is within expiryWarnWindow of notAfter.
func expiryWarning(notAfter, now time.Time) bool {
	return !now.Before(notAfter.Add(-expiryWarnWindow))
}

// nextBackoff doubles the current backoff, starting at backoffInitial and
// capping at backoffCap.
func nextBackoff(cur time.Duration) time.Duration {
	if cur <= 0 {
		return backoffInitial
	}
	next := cur * 2
	if next > backoffCap {
		return backoffCap
	}
	return next
}
