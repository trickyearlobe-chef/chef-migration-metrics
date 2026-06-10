// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"crypto"

	xacme "golang.org/x/crypto/acme"
)

// Config is the engine's view of server.tls.acme (translated from
// config.ACMEConfig by the caller, to keep this package decoupled from the
// large config package). It carries only what the order/renewal flow needs.
type Config struct {
	Domains         []string
	Email           string
	CAURL           string
	Challenge       string // "http-01" | "dns-01"
	RenewBeforeDays int
	AgreeToTOS      bool
}

// acmeClient is the subset of golang.org/x/crypto/acme.Client the engine drives.
// *xacme.Client satisfies it structurally; tests supply a fake so the order flow
// is exercised without any network (tls-acme.md § 3.1 test note).
type acmeClient interface {
	Register(ctx context.Context, acct *xacme.Account, prompt func(tosURL string) bool) (*xacme.Account, error)
	AuthorizeOrder(ctx context.Context, id []xacme.AuthzID, opt ...xacme.OrderOption) (*xacme.Order, error)
	GetAuthorization(ctx context.Context, url string) (*xacme.Authorization, error)
	Accept(ctx context.Context, chal *xacme.Challenge) (*xacme.Challenge, error)
	WaitAuthorization(ctx context.Context, url string) (*xacme.Authorization, error)
	CreateOrderCert(ctx context.Context, url string, csr []byte, bundle bool) (der [][]byte, certURL string, err error)
	HTTP01ChallengeResponse(token string) (string, error)
	DNS01ChallengeRecord(token string) (string, error)
}

// newClientFunc builds an acmeClient bound to the given account key. The
// production implementation returns a real *xacme.Client; tests inject a fake.
type newClientFunc func(accountKey crypto.Signer) acmeClient

// realClientFactory returns a newClientFunc that constructs real ACME clients
// pointed at caURL. It is the production wiring used by the caller (Chunk 8).
func realClientFactory(caURL string) newClientFunc {
	return func(accountKey crypto.Signer) acmeClient {
		return &xacme.Client{Key: accountKey, DirectoryURL: caURL}
	}
}
