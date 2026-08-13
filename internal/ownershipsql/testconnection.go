// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Testing a connection is its own act.
//
// See journeys/ownership-connection.md. Asking for the list of tables is not a
// connection test: when that comes back empty or angry it cannot say what
// failed, because it was not trying to find out. And there are five different
// answers, each belonging to a different person to go and talk to.

// Outcome says which of the five things happened, so a screen can send somebody
// to the right place rather than printing "could not connect".
type Outcome string

const (
	// OutcomeConnected means the server answered and the connection is usable.
	OutcomeConnected Outcome = "connected"
	// OutcomeMalformed means it never got as far as the network: the connection
	// could not be read, or does not say where the password goes, or names a
	// database it is not for. This one is ours to report, not a driver's.
	OutcomeMalformed Outcome = "malformed"
	// OutcomeUnreachable means nothing answered: a wrong address, a closed port,
	// a firewall. Somebody in networking.
	OutcomeUnreachable Outcome = "unreachable"
	// OutcomeRefused means the server answered and rejected the account or its
	// password. Somebody who owns the account.
	OutcomeRefused Outcome = "refused"
	// OutcomeNoDatabase means the login worked and the named database did not.
	OutcomeNoDatabase Outcome = "no-database"
	// OutcomeUntrustedDomain means the account is not the database's to check.
	// Anything before a backslash — a domain, a machine name, a workgroup, or a
	// dot — hands the login to a directory instead, and this server is not in
	// it or cannot reach it. Somebody who runs that directory.
	OutcomeUntrustedDomain Outcome = "untrusted-domain"
	// OutcomeUnknown means the server refused in words nothing here recognises.
	// It exists so that an unrecognised failure is not quietly filed under
	// whichever outcome happens to be the fallback: naming the wrong team is
	// worse than admitting we cannot tell, and the detail underneath is still
	// the server's own words.
	OutcomeUnknown Outcome = "unknown"
)

// Result is what a connection test found.
type Result struct {
	// Outcome is which of the five it was.
	Outcome Outcome `json:"outcome"`
	// Connection is the composed connection with the password masked — what
	// was actually sent, which is the thing worth reading when it failed.
	Connection string `json:"connection"`
	// Form is the spelling that was recognised.
	Form Form `json:"form"`
	// Detail is what refused us, in its own words, with the password taken
	// out. Empty when the connection worked.
	//
	// Never tidied into a sentence of ours: a message rewritten as "could not
	// connect" has thrown away the only thing in it worth having.
	Detail string `json:"detail,omitempty"`
}

// Succeeded reports whether the connection is usable.
func (r Result) Succeeded() bool { return r.Outcome == OutcomeConnected }

// TestConnection opens the connection, asks the server to answer, and says what
// happened. It runs no query and reads no rows — the point is that when it
// fails, the failure is about connecting and nothing else.
func TestConnection(ctx context.Context, cfg Config) Result {
	if !IsSupportedDriver(cfg.Driver) {
		return Result{Outcome: OutcomeMalformed, Detail: "unsupported driver " + cfg.Driver}
	}

	// Everything this can say before dialling. Reported as malformed because
	// none of it is the network's fault or the server's.
	if err := validateDSNNamesDatabase(cfg.Driver, cfg.connectionForChecking()); err != nil {
		return Result{Outcome: OutcomeMalformed, Detail: redactErr(err, cfg.Password).Error()}
	}

	composed, err := cfg.composeForTest()
	if err != nil {
		return Result{Outcome: OutcomeMalformed, Detail: redactErr(err, cfg.Password).Error()}
	}
	result := Result{Connection: composed.Masked, Form: composed.Form}

	db, err := sql.Open(cfg.Driver, composed.DSN)
	if err != nil {
		result.Outcome = OutcomeMalformed
		result.Detail = redactErr(err, cfg.Password).Error()
		return result
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	timeout := cfg.ConnectTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		clean := redactErr(err, cfg.Password)
		result.Outcome = classifyFailure(clean, pingCtx.Err())
		result.Detail = clean.Error()
		return result
	}

	result.Outcome = OutcomeConnected
	return result
}

// composeForTest builds the connection the test will actually send, by the same
// path a real import takes — otherwise a test could pass against a string
// nobody uses.
func (c Config) composeForTest() (Composed, error) {
	if c.Connection == "" {
		// The older shape, where the whole connection is one secret. There is
		// nothing to compose and nothing readable to show, so the masked view
		// is deliberately empty rather than a connection carrying a password.
		dsn, err := c.resolveDSN()
		if err != nil {
			return Composed{}, err
		}
		return Composed{DSN: dsn, Masked: "", Form: DetectForm(c.DSN)}, nil
	}
	visible, err := applyTLSMode(c.Driver, c.Connection, c.TLSMode)
	if err != nil {
		return Composed{}, err
	}
	return Compose(c.Driver, visible, c.Password)
}

// classifyFailure works out which of the five a refusal was, from what the
// driver said.
//
// Matching on text is not something to be pleased about, but the drivers offer
// nothing better: neither returns a typed error for "wrong password" as
// distinct from "no such host". Every phrase below was MEASURED against running
// servers rather than taken from documentation, and the exact messages are
// quoted beside the rules that match them. testconnection_functional_test.go
// re-measures them, so this fails if a driver changes its mind.
func classifyFailure(err error, ctxErr error) Outcome {
	if errors.Is(ctxErr, context.DeadlineExceeded) {
		// Nothing answered in time, which is a firewall far more often than it
		// is a slow server.
		return OutcomeUnreachable
	}
	text := strings.ToLower(err.Error())

	switch {
	// First, because it IS a login failure and says so — but the account was
	// never the database's to check, and that is a different person again.
	case contains(text, "untrusted domain", "integrated authentication",
		"cannot generate sspi context"):
		return OutcomeUntrustedDomain

	// Before the refusal checks: SQL Server phrases this as a login error, and
	// PostgreSQL only reaches it once the login has already succeeded.
	//
	//   mssql: login error: Cannot open database "nosuchdb" that was requested
	//          by the login. Using the user default database "master" instead.
	//   pq: database "nosuchdb" does not exist (3D000)
	case contains(text, "cannot open database"),
		strings.Contains(text, `database "`) && strings.Contains(text, "does not exist"),
		strings.Contains(text, "3d000"):
		return OutcomeNoDatabase

	// Measured: neither database distinguishes a wrong password from an account
	// that does not exist, and both say the same thing for each. That is fine —
	// it is the same person to go and talk to either way.
	//
	//   mssql: login error: Login failed for user 'cmmnasty'.
	//   pq: password authentication failed for user "cmm_probe" (28P01)
	case contains(text, "login failed", "password authentication failed",
		"authentication failed", "permission denied", "28p01"):
		return OutcomeRefused

	//   unable to open tcp connection with host 'localhost:14333': ... connection refused
	//   lookup nosuchhost.invalid: no such host
	case contains(text, "no such host", "connection refused", "i/o timeout",
		"no route to host", "network is unreachable", "connection reset",
		"unable to open tcp connection", "timeout"):
		return OutcomeUnreachable

	// Refused before anything was dialled: the string could not be read.
	case contains(text, "unable to parse", "invalid url", `missing "="`,
		"not recognized", "does not say where the password goes"):
		return OutcomeMalformed
	}

	return OutcomeUnknown
}

func contains(text string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(text, n) {
			return true
		}
	}
	return false
}
