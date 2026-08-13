// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipconn"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipsql"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// ---------------------------------------------------------------------------
// Setting up a connection to somebody else's database —
// /api/v1/ownership/import/connections and the two verbs beside it.
//
// See journeys/ownership-connection.md. The connection is configuration: the
// address, the database and the account are in plain view and editable, and
// only the password is out of sight, named here and held encrypted beside it.
//
// The order these support is the one the work is actually done in: somebody
// stores the password, somebody composes the connection round the marker,
// somebody tests it, and only then is it kept. So testing works on a connection
// that has never been stored, and storing does not require a passing test — a
// server behind a firewall that has not been opened yet could otherwise never
// be recorded at all.
// ---------------------------------------------------------------------------

// ownershipConnectionRequest is a connection as it is set up. The password is
// not in it and never is: password_credential names where it is kept.
type ownershipConnectionRequest struct {
	Name string `json:"name"`
	// Driver may be left out when the connection's own scheme names the
	// database, which is the usual case for a URL.
	Driver             string `json:"driver,omitempty"`
	Connection         string `json:"connection"`
	PasswordCredential string `json:"password_credential"`
}

// showConnectionRequest asks what a connection will look like when it is sent.
// Either a stored one by name, or one being typed.
type showConnectionRequest struct {
	Name       string `json:"name,omitempty"`
	Driver     string `json:"driver,omitempty"`
	Connection string `json:"connection,omitempty"`
}

// ownershipConnectionsResponse is every connection that has been set up. There
// is no paging: this is a handful of connections an administrator maintains by
// hand, and a list somebody has to walk is a list they cannot scan.
type ownershipConnectionsResponse struct {
	Data []ownershipconn.Connection `json:"data"`
}

// deletedConnectionResponse names what was removed, so a caller acting on a
// list can say which row went.
type deletedConnectionResponse struct {
	Deleted string `json:"deleted"`
}

// composedConnectionResponse is the connection as it will be sent, with the
// password masked.
type composedConnectionResponse struct {
	Driver string `json:"driver"`
	// Connection is what will actually go to the database, with the password
	// replaced by a fixed-width mask. It is safe to screenshot and to put in a
	// support bundle.
	Connection string `json:"connection"`
	// Form is the spelling that was recognised, and therefore which escaping
	// rule was applied — the thing that fails silently when it is wrong.
	Form string `json:"form"`
}

// testConnectionRequest asks a server to answer. Either a stored connection by
// name, or one that has not been stored yet.
type testConnectionRequest struct {
	Name               string `json:"name,omitempty"`
	Driver             string `json:"driver,omitempty"`
	Connection         string `json:"connection,omitempty"`
	PasswordCredential string `json:"password_credential,omitempty"`
}

// connections returns the store connections are kept in, or nil when there is
// nowhere to keep them.
func (r *Router) connections() *ownershipconn.Store {
	if r.configStore == nil {
		return nil
	}
	return ownershipconn.NewStore(r.configStore)
}

// requireConnectionStore answers the caller and returns false when there is
// nowhere to keep a connection, which means no encryption key is configured.
func (r *Router) requireConnectionStore(w http.ResponseWriter) (*ownershipconn.Store, bool) {
	store := r.connections()
	if store == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Configuration storage is not available, so a connection cannot be kept. "+
				"Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable it.")
		return nil, false
	}
	return store, true
}

// ---------------------------------------------------------------------------
// GET and POST /api/v1/ownership/import/connections
// ---------------------------------------------------------------------------

func (r *Router) handleOwnershipConnections(w http.ResponseWriter, req *http.Request) {
	store, ok := r.requireConnectionStore(w)
	if !ok {
		return
	}

	switch req.Method {
	case http.MethodGet:
		stored, err := store.List(req.Context())
		if err != nil {
			r.logf("ERROR", "ownership/connections: listing: %v", err)
			WriteInternalError(w, "Could not read the stored connections.")
			return
		}
		if stored == nil {
			stored = []ownershipconn.Connection{}
		}
		WriteJSON(w, http.StatusOK, ownershipConnectionsResponse{Data: stored})

	case http.MethodPost:
		var body ownershipConnectionRequest
		if !decodeJSONBody(w, req, &body) {
			return
		}
		conn := ownershipconn.Connection{
			Name:               body.Name,
			Driver:             body.Driver,
			Connection:         body.Connection,
			PasswordCredential: body.PasswordCredential,
		}
		// The credential is checked for existence here rather than at the
		// moment somebody tries to import: a connection whose password is
		// nowhere fails as a refused login, which sends the search to the
		// account owner instead of to the person who mistyped a name.
		if !r.credentialExists(w, req, conn.PasswordCredential) {
			return
		}
		if err := store.Save(req.Context(), conn, adminUsername(req)); err != nil {
			WriteBadRequest(w, err.Error())
			return
		}
		saved, err := store.Get(req.Context(), conn.Name)
		if err != nil {
			r.logf("ERROR", "ownership/connections: reading back %q: %v", conn.Name, err)
			WriteInternalError(w, "The connection was stored but could not be read back.")
			return
		}
		WriteJSON(w, http.StatusOK, saved)

	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeValidationError,
			"Method not allowed.")
	}
}

// ---------------------------------------------------------------------------
// GET and DELETE /api/v1/ownership/import/connections/{name}
// ---------------------------------------------------------------------------

func (r *Router) handleOwnershipConnectionItem(w http.ResponseWriter, req *http.Request, name string) {
	store, ok := r.requireConnectionStore(w)
	if !ok {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		WriteBadRequest(w, "Which connection? The name is part of the address.")
		return
	}

	switch req.Method {
	case http.MethodGet:
		conn, err := store.Get(req.Context(), name)
		if errors.Is(err, ownershipconn.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("There is no connection called %q.", name))
			return
		}
		if err != nil {
			r.logf("ERROR", "ownership/connections: reading %q: %v", name, err)
			WriteInternalError(w, "Could not read the stored connection.")
			return
		}
		WriteJSON(w, http.StatusOK, conn)

	case http.MethodDelete:
		// What still refers to it is deliberately not checked: a saved import
		// naming a connection that has gone reports it when it runs, and
		// blocking the delete would leave somebody unable to remove a
		// connection they must stop using.
		err := store.Delete(req.Context(), name)
		if errors.Is(err, ownershipconn.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("There is no connection called %q.", name))
			return
		}
		if err != nil {
			r.logf("ERROR", "ownership/connections: deleting %q: %v", name, err)
			WriteInternalError(w, "Could not remove the connection.")
			return
		}
		WriteJSON(w, http.StatusOK, deletedConnectionResponse{Deleted: name})

	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeValidationError,
			"Method not allowed.")
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/import/show-connection
// ---------------------------------------------------------------------------

// handleShowOwnershipConnection answers with the connection as it will be sent,
// masked.
//
// It never reads the password. The mask goes through the same escaping the real
// password goes through, so what comes back is the shape of the real connection
// — which is the whole question — without the secret being fetched to answer
// it.
func (r *Router) handleShowOwnershipConnection(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}
	var body showConnectionRequest
	if !decodeJSONBody(w, req, &body) {
		return
	}

	asked, ok := r.resolveConnectionToSend(w, req, body.Name, body.Driver, body.Connection)
	if !ok {
		return
	}
	driver := asked.Driver

	composed, err := ownershipsql.Compose(driver, asked.Connection, ownershipsql.PasswordMask)
	if err != nil {
		WriteBadRequest(w, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, composedConnectionResponse{
		Driver:     driver,
		Connection: composed.Masked,
		Form:       string(composed.Form),
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/import/test-connection
// ---------------------------------------------------------------------------

// handleTestOwnershipConnection dials the server and says which of the five
// things happened, in the words of whatever refused — with the password taken
// out of them.
//
// A failure here is a 200 carrying an outcome, not an HTTP error: the call did
// what it was asked, and what it found is the answer. The HTTP errors below are
// for the call being unanswerable at all — no such stored connection, no such
// credential — which are the caller's mistakes rather than the server's word.
func (r *Router) handleTestOwnershipConnection(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}
	var body testConnectionRequest
	if !decodeJSONBody(w, req, &body) {
		return
	}

	asked, ok := r.resolveConnectionToSend(w, req, body.Name, body.Driver, body.Connection)
	if !ok {
		return
	}
	driver, connection := asked.Driver, asked.Connection

	// A stored connection carries the credential holding its password, so
	// testing one by name needs nothing else said. For one being typed, the
	// caller says where the password is.
	credentialName := strings.TrimSpace(body.PasswordCredential)
	if asked.PasswordCredential != "" {
		credentialName = asked.PasswordCredential
	}
	if credentialName == "" {
		WriteBadRequest(w, "Which credential holds the password? Name it in password_credential.")
		return
	}

	password, ok := r.readPassword(w, req, credentialName)
	if !ok {
		return
	}
	defer secrets.ZeroBytes(password)

	result := ownershipsql.TestConnection(req.Context(), ownershipsql.Config{
		Driver:     driver,
		Connection: connection,
		Password:   string(password),
	})
	WriteJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// Reading what was asked about
// ---------------------------------------------------------------------------

// resolveConnectionToSend returns the connection to compose: a stored one when
// a name was given, otherwise the one being typed.
//
// A stored one is returned whole, so its password credential travels with it —
// a caller that names a stored connection cannot then be sent somewhere else
// for the password.
func (r *Router) resolveConnectionToSend(w http.ResponseWriter, req *http.Request,
	name, driver, connection string,
) (ownershipconn.Connection, bool) {
	if strings.TrimSpace(name) != "" {
		store, ok := r.requireConnectionStore(w)
		if !ok {
			return ownershipconn.Connection{}, false
		}
		stored, err := store.Get(req.Context(), name)
		if errors.Is(err, ownershipconn.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("There is no connection called %q.", name))
			return ownershipconn.Connection{}, false
		}
		if err != nil {
			r.logf("ERROR", "ownership/connections: reading %q: %v", name, err)
			WriteInternalError(w, "Could not read the stored connection.")
			return ownershipconn.Connection{}, false
		}
		return stored, true
	}

	if strings.TrimSpace(connection) == "" {
		WriteBadRequest(w, "Send a connection to compose, or the name of a stored one.")
		return ownershipconn.Connection{}, false
	}
	resolved, err := ownershipconn.ResolveDriver(driver, connection)
	if err != nil {
		WriteBadRequest(w, err.Error())
		return ownershipconn.Connection{}, false
	}
	return ownershipconn.Connection{Driver: resolved, Connection: connection}, true
}

// credentialExists reports whether a credential of that name is there, without
// decrypting it. It answers the caller and returns false when it is not.
func (r *Router) credentialExists(w http.ResponseWriter, req *http.Request, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		// Left to the store to refuse, so the wording of that requirement lives
		// in one place.
		return true
	}
	if r.credentialStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Credential storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return false
	}
	if _, err := r.credentialStore.GetMetadata(req.Context(), name); err != nil {
		WriteBadRequest(w, fmt.Sprintf(
			"There is no stored credential called %q to hold the password. Add it under "+
				"Credentials first, as a Generic credential.", name))
		return false
	}
	return true
}

// connectionConfig turns the name of a stored connection into the configuration
// that connects with it: the connection as written, the database it names, and
// the password read at that moment.
//
// The caller must call the returned cleanup, which zeroes the password. It is
// the one way an import reaches a database — the interactive path and a
// scheduled run share it, so a connection that works in one works in the other.
func (r *Router) connectionConfig(ctx context.Context, name string) (ownershipsql.Config, func(), error) {
	noop := func() {}
	name = strings.TrimSpace(name)
	if name == "" {
		return ownershipsql.Config{}, noop, errors.New(
			"no connection was named: choose one that has been set up on this screen")
	}
	if r.configStore == nil {
		return ownershipsql.Config{}, noop, errors.New(
			"configuration storage is not available, so there are no connections to read")
	}
	stored, err := ownershipconn.NewStore(r.configStore).Get(ctx, name)
	if err != nil {
		return ownershipsql.Config{}, noop, fmt.Errorf("connection %q: %w", name, err)
	}
	if r.credentialStore == nil {
		return ownershipsql.Config{}, noop, errors.New(
			"credential storage is not configured, so the password cannot be read")
	}
	cred, err := r.credentialStore.Get(ctx, stored.PasswordCredential)
	if err != nil {
		return ownershipsql.Config{}, noop, fmt.Errorf(
			"connection %q names the credential %q for its password, which could not be read: %w",
			name, stored.PasswordCredential, err)
	}
	return ownershipsql.Config{
			Driver:     stored.Driver,
			Connection: stored.Connection,
			Password:   string(cred.Plaintext),
		}, func() {
			secrets.ZeroBytes(cred.Plaintext)
		}, nil
}

// readPassword fetches the password to put into the connection. The caller must
// zero what comes back.
func (r *Router) readPassword(w http.ResponseWriter, req *http.Request, name string) ([]byte, bool) {
	if r.credentialStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Credential storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return nil, false
	}
	cred, err := r.credentialStore.Get(req.Context(), name)
	if err != nil {
		// The name is the caller's own input, so naming it is helpful rather
		// than leaky; the value never appears either way.
		WriteBadRequest(w, fmt.Sprintf(
			"Could not read the credential %q holding the password: %v", name, err))
		return nil, false
	}
	return cred.Plaintext, true
}
