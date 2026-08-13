// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package ownershipconn holds the database connections an ownership import
// reads from.
//
// See journeys/ownership-connection.md. The address, the database, the account
// and the domain are configuration: they appear in tickets, in runbooks and in
// the logs of everything else that talks to that server, and an administrator
// who cannot read them cannot check them. Only the password is a secret, so
// only the password is a credential — named here, held encrypted in
// internal/secrets, and put into the connection at the last moment by
// internal/ownershipsql.
//
// Nothing in this package ever handles a password. That is not an oversight to
// be corrected later: the moment a password passes through here, the connection
// stops being something that can be shown on a screen.
package ownershipconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipsql"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// ConfigKey is where the connections live in the config store. One document
// rather than a key each: the list is read whole every time a screen shows it,
// and administrators edit it one at a time by hand.
const ConfigKey = "ownership_import_connections"

// ErrNotFound is returned when no connection has the given name.
var ErrNotFound = errors.New("ownershipconn: no connection with that name")

// Connection is one database an ownership import can read from.
//
// Everything here is readable. The password is not here — PasswordCredential
// names the credential that holds it, and Connection says where it goes with
// ownershipsql.PasswordMarker.
type Connection struct {
	// Name identifies the connection to a saved import and to a person.
	Name string `json:"name"`
	// Driver is which database reads this connection, one of
	// ownershipsql.SupportedDrivers. It is taken from the scheme when the
	// connection is URL-shaped, because the connection already says.
	Driver string `json:"driver"`
	// Connection is the string as the administrator wrote it, with the
	// password's position marked rather than written out. It is shown, edited
	// and stored exactly as typed: rewriting anything visible is the same
	// unreadable failure in a new place.
	Connection string `json:"connection"`
	// PasswordCredential is the name of the stored credential holding the
	// password. The value never appears here or in anything this returns.
	PasswordCredential string `json:"password_credential"`

	// Who last changed it and when, so an import that stopped working can be
	// lined up against the change that stopped it.
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

// document is what is written to the config store. It is an object rather than
// a bare array so a later field can be added beside the list without rewriting
// what is already stored.
type document struct {
	Connections []Connection `json:"connections"`
}

// ConfigStore is the part of the config store this needs. Narrow because the
// only thing it does is read and write one key, and a narrow interface is one a
// test can stand in for without a database.
type ConfigStore interface {
	Get(ctx context.Context, key string) (json.RawMessage, error)
	Set(ctx context.Context, key string, value json.RawMessage, secret bool, updatedBy string) error
}

// Store reads and writes the named connections.
type Store struct {
	cfg ConfigStore
	// now is the clock, so a test can assert what was stamped rather than that
	// something was.
	now func() time.Time
}

// NewStore returns a Store over the given config store.
func NewStore(cfg ConfigStore) *Store {
	return &Store{cfg: cfg, now: func() time.Time { return time.Now().UTC() }}
}

// List returns every stored connection, ordered by name.
//
// Nothing stored is an empty list rather than a failure: before the first
// connection is added there is nothing wrong, there is nothing there.
func (s *Store) List(ctx context.Context) ([]Connection, error) {
	doc, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	return doc.Connections, nil
}

// Get returns one connection by name, or ErrNotFound.
func (s *Store) Get(ctx context.Context, name string) (Connection, error) {
	doc, err := s.read(ctx)
	if err != nil {
		return Connection{}, err
	}
	for _, c := range doc.Connections {
		if c.Name == strings.TrimSpace(name) {
			return c, nil
		}
	}
	return Connection{}, fmt.Errorf("%w: %q", ErrNotFound, name)
}

// Save validates a connection and stores it, replacing any connection of the
// same name.
//
// It is validated here rather than when somebody tries to use it, for the
// reason the credential type gives: the refusal belongs in front of whoever
// composed the connection, while they still have it open.
func (s *Store) Save(ctx context.Context, conn Connection, updatedBy string) error {
	prepared, err := Validate(conn)
	if err != nil {
		return err
	}
	prepared.UpdatedAt = s.now()
	prepared.UpdatedBy = updatedBy

	doc, err := s.read(ctx)
	if err != nil {
		return err
	}
	replaced := false
	for i, existing := range doc.Connections {
		if existing.Name == prepared.Name {
			doc.Connections[i] = prepared
			replaced = true
			break
		}
	}
	if !replaced {
		doc.Connections = append(doc.Connections, prepared)
	}
	return s.write(ctx, doc, updatedBy)
}

// Delete removes one connection by name, or returns ErrNotFound.
//
// What refers to it is not checked here: whether a saved import still names
// this connection is a question for the layer that knows about saved imports.
func (s *Store) Delete(ctx context.Context, name string) error {
	doc, err := s.read(ctx)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	kept := make([]Connection, 0, len(doc.Connections))
	for _, c := range doc.Connections {
		if c.Name != name {
			kept = append(kept, c)
		}
	}
	if len(kept) == len(doc.Connections) {
		return fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	doc.Connections = kept
	return s.write(ctx, doc, "")
}

// Validate checks a connection and returns it as it will be stored, with the
// driver filled in from the scheme where the connection names one.
//
// Every refusal names what to change. These are strings composed by somebody
// else and handed over on a ticket, and "invalid connection" sends that person
// back to guessing — which is the failure the whole journey is about.
func Validate(conn Connection) (Connection, error) {
	conn.Name = strings.TrimSpace(conn.Name)
	conn.Connection = strings.TrimSpace(conn.Connection)
	conn.PasswordCredential = strings.TrimSpace(conn.PasswordCredential)
	conn.Driver = strings.TrimSpace(conn.Driver)

	if conn.Name == "" {
		return Connection{}, errors.New("ownershipconn: the connection needs a name, so a saved import can refer to it")
	}
	if conn.Connection == "" {
		return Connection{}, errors.New("ownershipconn: the connection is empty")
	}
	if conn.PasswordCredential == "" {
		return Connection{}, errors.New(
			"ownershipconn: the connection must name the stored credential holding its password")
	}

	driver, err := ResolveDriver(conn.Driver, conn.Connection)
	if err != nil {
		return Connection{}, err
	}
	conn.Driver = driver

	// A connection has to name the database it reads. Checked by the same code
	// that checks a stored credential, so the two cannot drift apart — they
	// did once, and a customer connection was accepted when it was stored and
	// refused when it was used.
	if err := secrets.ValidateDatabaseURL(conn.Connection); err != nil {
		return Connection{}, fmt.Errorf("ownershipconn: %w", err)
	}

	// Composed with a stand-in password rather than checked by inspection: this
	// runs the same code the import will, so a connection that is stored is one
	// that composes. It is what refuses a connection with no marker — which is
	// also what stops a password being stored here as readable configuration —
	// and one whose scheme names a database other than the one chosen.
	if _, err := ownershipsql.Compose(conn.Driver, conn.Connection, ownershipsql.PasswordMask); err != nil {
		return Connection{}, err
	}

	return conn, nil
}

// ResolveDriver says which database reads a connection: the one chosen, or the
// one the connection's own scheme names when nothing was chosen.
//
// A URL-shaped connection already says which database it is for, so asking a
// second time is not merely redundant — the two can disagree, and neither
// driver says so when they do. A keyword-shaped connection carries no scheme,
// so for that shape somebody still has to say.
func ResolveDriver(driver, connection string) (string, error) {
	driver = strings.TrimSpace(driver)
	if scheme, _, isURL := strings.Cut(strings.TrimSpace(connection), "://"); isURL && driver == "" {
		if named, known := ownershipsql.DriverNamedByScheme(scheme); known {
			driver = named
		}
	}
	if !ownershipsql.IsSupportedDriver(driver) {
		return "", fmt.Errorf(
			"ownershipconn: say which database this connection is for (%s) — its own spelling does not",
			strings.Join(ownershipsql.SupportedDrivers, " or "))
	}
	return driver, nil
}

// read returns the stored document, or an empty one when nothing is stored.
func (s *Store) read(ctx context.Context) (document, error) {
	raw, err := s.cfg.Get(ctx, ConfigKey)
	if errors.Is(err, configstore.ErrNotFound) {
		return document{}, nil
	}
	if err != nil {
		return document{}, fmt.Errorf("ownershipconn: reading the stored connections: %w", err)
	}
	var doc document
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return document{}, fmt.Errorf("ownershipconn: the stored connections could not be read: %w", err)
		}
	}
	sortByName(doc.Connections)
	return doc, nil
}

func (s *Store) write(ctx context.Context, doc document, updatedBy string) error {
	sortByName(doc.Connections)
	value, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("ownershipconn: writing the connections: %w", err)
	}
	// secret=false is the whole point. A secret entry can only be read back by
	// a caller that asks for the secret, and that is how the address, the
	// account and the database came to be as invisible as the password.
	if err := s.cfg.Set(ctx, ConfigKey, value, false, updatedBy); err != nil {
		return fmt.Errorf("ownershipconn: storing the connections: %w", err)
	}
	return nil
}

func sortByName(connections []Connection) {
	sort.Slice(connections, func(i, j int) bool {
		return connections[i].Name < connections[j].Name
	})
}
