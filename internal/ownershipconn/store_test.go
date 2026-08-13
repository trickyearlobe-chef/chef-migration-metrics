// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipconn

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipsql"
)

// journeys/ownership-connection.md — "Only the password out of sight. The
// address, the database, the account and the domain in plain view, and
// editable."
//
// So a connection is configuration and the password beside it is a secret. The
// two are held apart here: nothing in this package ever sees a password, which
// is why none of these tests has one to hide.

// fakeConfig is the config store, minus the encryption and the database. It
// records the secret flag because that flag is the whole point: a connection
// written as a secret cannot be read back, which is the state this replaces.
type fakeConfig struct {
	values    map[string]json.RawMessage
	secret    map[string]bool
	updatedBy map[string]string
	setCalls  int
}

func newFakeConfig() *fakeConfig {
	return &fakeConfig{
		values:    map[string]json.RawMessage{},
		secret:    map[string]bool{},
		updatedBy: map[string]string{},
	}
}

func (f *fakeConfig) Get(_ context.Context, key string) (json.RawMessage, error) {
	v, ok := f.values[key]
	if !ok {
		return nil, configstore.ErrNotFound
	}
	return v, nil
}

func (f *fakeConfig) Set(_ context.Context, key string, value json.RawMessage, secret bool, updatedBy string) error {
	f.setCalls++
	f.values[key] = value
	f.secret[key] = secret
	f.updatedBy[key] = updatedBy
	return nil
}

// A connection as an administrator would be handed one on a ticket: an account
// with a domain in front of it, and the password marked rather than written.
const (
	sqlServerConnection = `sqlserver://EXAMPLECORP\svcaccount:` + ownershipsql.PasswordMarker +
		`@dbhost.example.com:1433?database=cmdb`
	postgresConnection = `postgres://svcaccount:` + ownershipsql.PasswordMarker +
		`@dbhost.example.com:5432/cmdb?sslmode=require`
)

func aConnection() Connection {
	return Connection{
		Name:               "asset-database",
		Driver:             ownershipsql.DriverSQLServer,
		Connection:         sqlServerConnection,
		PasswordCredential: "asset-database-password",
	}
}

func store(t *testing.T) (*Store, *fakeConfig) {
	t.Helper()
	cfg := newFakeConfig()
	return NewStore(cfg), cfg
}

// The requirement, and the reason this package exists: what was stored can be
// read back and checked, because everything in it except the password is
// configuration.
func TestAConnectionReadsBackWithoutItsPassword(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	if err := s.Save(ctx, aConnection(), "admin"); err != nil {
		t.Fatalf("saving the connection: %v", err)
	}

	got, err := s.Get(ctx, "asset-database")
	if err != nil {
		t.Fatalf("reading the connection back: %v", err)
	}
	// Every part the administrator has to be able to check when it fails.
	for _, visible := range []string{
		"dbhost.example.com", "1433", "database=cmdb", "EXAMPLECORP", "svcaccount",
	} {
		if !strings.Contains(got.Connection, visible) {
			t.Errorf("%q is not readable in the stored connection, so it cannot be checked: %s",
				visible, got.Connection)
		}
	}
	// And the position of the password survives storage: a connection that
	// came back without it would refuse to compose the next time it was used.
	if !strings.Contains(got.Connection, ownershipsql.PasswordMarker) {
		t.Errorf("the marker did not survive being stored: %s", got.Connection)
	}
	if got.PasswordCredential != "asset-database-password" {
		t.Errorf("password credential = %q, want the name of the credential holding it",
			got.PasswordCredential)
	}
	if got.UpdatedBy != "admin" || got.UpdatedAt.IsZero() {
		t.Errorf("nothing records who last changed the connection: %+v", got)
	}
}

// The flag that decides whether any of the above is true. A secret entry can
// only be read back by a caller that asks for the secret, and no screen does —
// which is exactly how the whole connection came to be invisible.
func TestTheConnectionIsStoredAsConfigurationRatherThanAsASecret(t *testing.T) {
	ctx := context.Background()
	s, cfg := store(t)

	if err := s.Save(ctx, aConnection(), "admin"); err != nil {
		t.Fatalf("saving the connection: %v", err)
	}
	if cfg.setCalls == 0 {
		t.Fatal("the fixture proves nothing: nothing was written to the config store at all")
	}
	if cfg.secret[ConfigKey] {
		t.Error("the connection was stored as a secret, so it cannot be read back and the " +
			"administrator is back to guessing at what was sent")
	}
	if cfg.updatedBy[ConfigKey] != "admin" {
		t.Errorf("the config entry does not record who wrote it: %q", cfg.updatedBy[ConfigKey])
	}
}

// "If I leave it out, refuse me and say so. Do not decide for me and send
// something I did not write."
//
// It is also what stops a password being stored here: a connection with a real
// password written into it carries no marker, so it is refused rather than
// filed away as readable configuration.
func TestAConnectionThatDoesNotSayWhereThePasswordGoesIsRefused(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	// Baseline: with the marker this saves, so the refusal below is about the
	// marker and not about something else in the connection.
	if err := s.Save(ctx, aConnection(), "admin"); err != nil {
		t.Fatalf("the fixture proves nothing: even with the marker this will not save: %v", err)
	}

	inline := aConnection()
	inline.Name = "password-written-in"
	inline.Connection = `sqlserver://EXAMPLECORP\svcaccount:hunter2@dbhost.example.com:1433?database=cmdb`
	err := s.Save(ctx, inline, "admin")
	if err == nil {
		t.Fatal("a connection with the password written into it was stored as readable " +
			"configuration, which puts a password where anybody can read it")
	}
	if !strings.Contains(err.Error(), ownershipsql.PasswordMarker) {
		t.Errorf("the refusal does not say how to mark the position: %v", err)
	}
}

func TestAConnectionThatNamesNoDatabaseIsRefused(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	noDatabase := aConnection()
	noDatabase.Connection = `sqlserver://svcaccount:` + ownershipsql.PasswordMarker +
		`@dbhost.example.com:1433`
	if err := s.Save(ctx, noDatabase, "admin"); err == nil {
		t.Error("stored a connection that never says which database to read")
	}
}

// Measured, and the reason it is refused rather than passed on: given a
// postgres:// connection the SQL Server driver reads it as keyword pairs, finds
// no account, and the server answers "Login failed for user ''" — a refusal
// that names the wrong team.
func TestAConnectionWhoseSchemeNamesAnotherDatabaseIsRefused(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	mismatched := aConnection()
	mismatched.Connection = postgresConnection
	err := s.Save(ctx, mismatched, "admin")
	if err == nil {
		t.Fatal("stored a postgres connection under SQL Server, which fails as a refused login")
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Errorf("the refusal does not say which database the connection is actually for: %v", err)
	}
}

// A URL-shaped connection already says which database it is for, so the
// administrator is not asked a second time. See journeys/ownership-connection.md
// and ownershipsql.DriverNamedByScheme.
func TestTheDatabaseIsTakenFromTheSchemeWhenTheConnectionNamesIt(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	derived := aConnection()
	derived.Name = "reporting-database"
	derived.Driver = ""
	derived.Connection = postgresConnection
	if err := s.Save(ctx, derived, "admin"); err != nil {
		t.Fatalf("saving a connection whose scheme names its database: %v", err)
	}

	got, err := s.Get(ctx, "reporting-database")
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	if got.Driver != ownershipsql.DriverPostgres {
		t.Errorf("driver = %q, want it read from the scheme the connection already carries",
			got.Driver)
	}
}

// A keyword-shaped connection carries no scheme, so something still has to say
// which database it is — the derivation above cannot answer for this shape.
func TestAKeywordConnectionMustStillSayWhichDatabaseItIs(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	keyword := Connection{
		Name: "keyword-shaped",
		// Deliberately not named: a keyword connection carries no scheme.
		Driver: "",
		Connection: `server=dbhost.example.com;database=cmdb;user id=svcaccount;password=` +
			ownershipsql.PasswordMarker,
		PasswordCredential: "asset-database-password",
	}
	if err := s.Save(ctx, keyword, "admin"); err == nil {
		t.Fatal("a connection with no scheme and no database chosen was accepted, so which " +
			"driver reads it is a guess")
	}

	keyword.Driver = ownershipsql.DriverSQLServer
	if err := s.Save(ctx, keyword, "admin"); err != nil {
		t.Errorf("refused a keyword connection that does say which database it is: %v", err)
	}
}

func TestAConnectionWithNoNameIsRefused(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	unnamed := aConnection()
	unnamed.Name = "   "
	if err := s.Save(ctx, unnamed, "admin"); err == nil {
		t.Error("stored a connection nothing can refer to")
	}
}

// The password is somewhere, and this says where. Without it the connection
// composes to nothing usable and the failure arrives at connection time.
func TestAConnectionMustNameTheCredentialHoldingItsPassword(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	noCredential := aConnection()
	noCredential.PasswordCredential = ""
	if err := s.Save(ctx, noCredential, "admin"); err == nil {
		t.Error("stored a connection with nothing holding its password")
	}
}

func TestSavingTheSameNameReplacesItRatherThanAddingASecond(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	if err := s.Save(ctx, aConnection(), "admin"); err != nil {
		t.Fatalf("saving: %v", err)
	}
	edited := aConnection()
	edited.Connection = strings.Replace(sqlServerConnection, "dbhost", "dbhost2", 1)
	if err := s.Save(ctx, edited, "someone-else"); err != nil {
		t.Fatalf("saving the edit: %v", err)
	}

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("editing a connection left %d of them", len(all))
	}
	if !strings.Contains(all[0].Connection, "dbhost2") {
		t.Errorf("the edit was not kept: %s", all[0].Connection)
	}
	if all[0].UpdatedBy != "someone-else" {
		t.Errorf("updated by = %q, want whoever made the edit", all[0].UpdatedBy)
	}
}

func TestConnectionsAreListedInAStableOrder(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	for _, name := range []string{"warehouse", "asset-database", "people"} {
		c := aConnection()
		c.Name = name
		if err := s.Save(ctx, c, "admin"); err != nil {
			t.Fatalf("saving %q: %v", name, err)
		}
	}

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	var got []string
	for _, c := range all {
		got = append(got, c.Name)
	}
	want := []string{"asset-database", "people", "warehouse"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("listed %v, want %v — a list that reorders itself is one nobody can scan", got, want)
	}
}

func TestNothingStoredIsAnEmptyListRatherThanAFailure(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("listing before anything is stored: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("listed %d connections when none were stored", len(all))
	}
}

func TestAConnectionThatWasNeverStoredIsNotFound(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	if _, err := s.Get(ctx, "no-such-connection"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound so a screen can say so rather than report a failure", err)
	}
	if err := s.Delete(ctx, "no-such-connection"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting one that is not there = %v, want ErrNotFound", err)
	}
}

func TestDeletingOneLeavesTheOthers(t *testing.T) {
	ctx := context.Background()
	s, _ := store(t)

	for _, name := range []string{"asset-database", "people"} {
		c := aConnection()
		c.Name = name
		if err := s.Save(ctx, c, "admin"); err != nil {
			t.Fatalf("saving %q: %v", name, err)
		}
	}
	if err := s.Delete(ctx, "asset-database"); err != nil {
		t.Fatalf("deleting: %v", err)
	}

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(all) != 1 || all[0].Name != "people" {
		t.Errorf("after deleting one connection the rest are %+v", all)
	}
}

// The stored document is read by whatever comes next — an endpoint, a screen, a
// scheduled import — so its shape is part of the contract rather than an
// implementation detail.
func TestTheStoredDocumentIsPlainReadableJSON(t *testing.T) {
	ctx := context.Background()
	s, cfg := store(t)

	if err := s.Save(ctx, aConnection(), "admin"); err != nil {
		t.Fatalf("saving: %v", err)
	}

	var doc struct {
		Connections []map[string]any `json:"connections"`
	}
	if err := json.Unmarshal(cfg.values[ConfigKey], &doc); err != nil {
		t.Fatalf("the stored value is not readable JSON: %v", err)
	}
	if len(doc.Connections) != 1 {
		t.Fatalf("stored %d connections", len(doc.Connections))
	}
	for _, field := range []string{"name", "driver", "connection", "password_credential"} {
		if _, ok := doc.Connections[0][field]; !ok {
			t.Errorf("the stored connection has no %q: %v", field, doc.Connections[0])
		}
	}
}
