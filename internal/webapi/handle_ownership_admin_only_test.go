// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
)

// Importing owners and resolving duplicate people are administrator functions,
// not everyday ones: an import rewrites who is accountable for the estate, and
// folding two people together is a change to the record of who exists. Both
// moved behind the admin role on 2026-08-06 at the product owner's instruction.
//
// The role check has to live in the handler, not only in the route table. The
// test router is built without auth middleware, so a registration-time gate
// would be invisible here — the endpoints would look guarded and prove nothing.

func withOperatorSession(req *http.Request) *http.Request {
	return req.WithContext(auth.ContextWithSession(req.Context(),
		&auth.SessionInfo{Username: "operator", Role: "operator"}))
}

func withAdminSession(req *http.Request) *http.Request {
	return req.WithContext(auth.ContextWithSession(req.Context(),
		&auth.SessionInfo{Username: "admin", Role: "admin"}))
}

// Every endpoint the import screen calls, including the ones that only read.
// Profiling a source and previewing a run reveal the contents of a system of
// record that an operator has no other way to query, so "it writes nothing" is
// not on its own a reason to leave them open.
func ownershipImportEndpoints(t *testing.T) []*http.Request {
	t.Helper()
	return []*http.Request{
		intakeRequest(t, "/api/v1/ownership/import/tables", "", map[string]string{
			"db_driver": "postgres", "db_credential": "cmdb",
		}),
		intakeRequest(t, "/api/v1/ownership/import/profile", "a,b\n1,2\n", nil),
		intakeRequest(t, "/api/v1/ownership/import/preview", twoRowCSV, map[string]string{
			"field_map": repoFieldMap(t),
		}),
		intakeRequest(t, "/api/v1/ownership/import/commit", twoRowCSV, map[string]string{
			"field_map": repoFieldMap(t),
		}),
		jsonRequest(http.MethodGet, "/api/v1/ownership/import/mappings", ""),
		jsonRequest(http.MethodPost, "/api/v1/ownership/import/mappings", mappingBody(t, "x")),
		jsonRequest(http.MethodGet, "/api/v1/ownership/import/mappings/1", ""),
		jsonRequest(http.MethodPut, "/api/v1/ownership/import/mappings/1", mappingBody(t, "x")),
		jsonRequest(http.MethodDelete, "/api/v1/ownership/import/mappings/1", ""),
	}
}

func ownershipDuplicateEndpoints() []*http.Request {
	pair := `{"owner_a":"a","owner_b":"b"}`
	return []*http.Request{
		jsonRequest(http.MethodGet, "/api/v1/ownership/duplicates", ""),
		jsonRequest(http.MethodPost, "/api/v1/ownership/duplicates/rescan", ""),
		jsonRequest(http.MethodPost, "/api/v1/ownership/duplicates/dismiss", pair),
		jsonRequest(http.MethodGet, "/api/v1/ownership/duplicates/dismissed", ""),
		jsonRequest(http.MethodPost, "/api/v1/ownership/duplicates/restore", pair),
	}
}

func TestOwnershipImport_RefusesAnOperator(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	for _, req := range ownershipImportEndpoints(t) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, withOperatorSession(req))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, want 403 for an operator: %s",
				req.Method, req.URL.Path, w.Code, w.Body.String())
		}
	}
}

func TestOwnershipDuplicates_RefuseAnOperator(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	for _, req := range ownershipDuplicateEndpoints() {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, withOperatorSession(req))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, want 403 for an operator: %s",
				req.Method, req.URL.Path, w.Code, w.Body.String())
		}
	}
}

// The gate has to let the administrator through, or the tests above would pass
// on an endpoint that refuses everybody.
func TestOwnershipAdminEndpoints_AdmitAnAdmin(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	reqs := append(ownershipImportEndpoints(t), ownershipDuplicateEndpoints()...)
	for _, req := range reqs {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, withAdminSession(req))
		if w.Code == http.StatusForbidden {
			t.Errorf("%s %s was refused for an admin: %s",
				req.Method, req.URL.Path, w.Body.String())
		}
	}
}
