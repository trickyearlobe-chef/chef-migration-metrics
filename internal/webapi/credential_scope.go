// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// What a credential may reach.
//
// A credential is another way into the same account, so what it can READ is
// settled by the account's role and nothing here narrows it: an administrator's
// credential sees what that administrator sees on screen, which is the whole
// point of there being no second permissions model.
//
// Writing is different, and is narrowed on purpose. A tool that only reads is
// one somebody can point at production without thinking hard. A tool that can
// write is aimed at one thing — recording a diagnosis where diagnoses go — so
// that is the only place it can reach, and everything else is refused whether
// or not the account could do it at a screen.
//
// The narrowing lives here rather than in the auth package because it is a
// statement about which addresses this service serves, and the auth package
// knows nothing about addresses.

// failureRegisterPath is the register's own address — where a new entry is
// recorded, and the only write a credential can make.
//
// NOT everything beneath it. Revising an entry and resolving one are further
// acts of judgement about somebody else's finding, and neither records what
// made the change: a revision stores no author at all, and a resolution stores
// a username with no way to say a tool typed it. So a credential doing either
// would be exactly the thing this journey refuses — a note that reads as its
// owner's own decision. Recording a NEW entry is safe because that path does
// carry the origin.
//
// If revising ever needs to be reachable by a tool, the attribution has to come
// first, in that order.
const failureRegisterPath = "/api/v1/failure-register"

// credentialMayProceed reports whether a request is allowed past the scope
// rule. Requests that did not arrive on a credential are unaffected.
//
// Reads are anything that does not change state. Everything else is a write,
// including the handful of POSTs this service uses to preview something: a
// preview that a read-only credential cannot run is a small loss, and guessing
// which POSTs are harmless is how a scope rule quietly stops being one.
func credentialMayProceed(info *auth.SessionInfo, req *http.Request) bool {
	if info == nil || !info.IsCredential() {
		return true
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	// The assistant surface is a transport, not an act. Every message to it is
	// a POST, including listing what there is and every read — so refusing the
	// envelope would refuse a read-only credential everything, and the journey
	// says most of them will be read-only.
	//
	// Nothing is given away by allowing it. What the caller actually asked for
	// is dispatched inward as its own request, and that request arrives back
	// here and meets this rule the same as any other. So the write below is
	// still the only write, whichever door it came through.
	if req.URL.Path == mcpPath {
		return true
	}

	if !info.CredentialCanWrite {
		return false
	}
	return req.Method == http.MethodPost && req.URL.Path == failureRegisterPath
}

// entryOrigin says what made a register entry: a person at a screen, or a tool
// holding a credential.
//
// Taken from the session and nowhere else. There is deliberately no way for a
// request body or a header to influence this — an entry a caller could sign as
// somebody's own judgement is worse than no attribution at all, because it
// reads as fact.
func entryOrigin(req *http.Request) string {
	if info := auth.SessionFromContext(req.Context()); info != nil && info.IsCredential() {
		return datastore.OriginCredential
	}
	return datastore.OriginScreen
}

// entryOriginName is the credential's name when one made the entry, and empty
// for a screen.
func entryOriginName(req *http.Request) string {
	if info := auth.SessionFromContext(req.Context()); info != nil && info.IsCredential() {
		return info.CredentialName
	}
	return ""
}

// enforceCredentialScope wraps a handler with the scope rule. It must sit
// INSIDE the authentication middleware, because it reads the session that
// middleware attaches.
func enforceCredentialScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		info := auth.SessionFromContext(req.Context())
		if credentialMayProceed(info, req) {
			next.ServeHTTP(w, req)
			return
		}

		// Say which of the two refusals this is, because they call for
		// different actions: make a new credential that can write, or stop
		// trying to use this one for the thing it is not for.
		if !info.CredentialCanWrite {
			WriteError(w, http.StatusForbidden, ErrCodeForbidden,
				"This credential can only read. Whoever created it chose read-only; "+
					"recording a finding needs one created with write access.")
			return
		}
		WriteError(w, http.StatusForbidden, ErrCodeForbidden,
			"A credential that can write may record a new entry in the failure register and "+
				"nothing else. Revising or resolving an entry is a person's act, at the "+
				"screen, because nothing records that a tool did it.")
	})
}
