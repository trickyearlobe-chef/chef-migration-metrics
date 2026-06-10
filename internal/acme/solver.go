// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"fmt"
)

// LogFunc is a logging callback. The level is one of "DEBUG", "INFO", "WARN",
// "ERROR". The acme package does not import the logging package directly (to
// avoid a dependency cycle) — the caller provides a callback, matching the
// internal/tls package's LogFunc convention. ERROR/WARN messages emitted by the
// engine never include key material (tls-acme.md § 3.11).
type LogFunc func(level, msg string)

// Challenge is the engine-computed view of a single ACME challenge that a
// Solver must publish so the CA can validate domain control. The engine derives
// KeyAuth/DNSValue from the account key, so a Solver never touches key material
// — it only needs to publish a value at a location (HTTP path or DNS record).
type Challenge struct {
	// Type is "http-01" or "dns-01".
	Type string
	// Domain is the identifier being authorized (e.g. "metrics.example.com").
	Domain string
	// Token is the ACME challenge token.
	Token string
	// KeyAuth is the HTTP-01 key authorization — the body served at
	// /.well-known/acme-challenge/<Token>. Empty for dns-01.
	KeyAuth string
	// DNSValue is the DNS-01 TXT record value published at
	// _acme-challenge.<Domain>. Empty for http-01.
	DNSValue string
}

// Solver publishes and tears down the proof for a challenge. Implementations
// (HTTP-01 file/handler, Route 53 DNS-01 TXT) are provided by later chunks and
// wired in by the caller; the core engine only depends on this seam.
//
// Present must make the proof observable to the CA (and, for dns-01, should
// block until the record is propagated/INSYNC). CleanUp removes whatever
// Present published and is always called after the authorization settles,
// regardless of outcome; CleanUp errors are best-effort (logged, not fatal).
type Solver interface {
	Present(ctx context.Context, ch Challenge) error
	CleanUp(ctx context.Context, ch Challenge) error
}

// logf invokes the optional log callback.
func logf(log LogFunc, level, format string, args ...any) {
	if log == nil {
		return
	}
	log(level, fmt.Sprintf(format, args...))
}
