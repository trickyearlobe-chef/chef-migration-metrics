// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package webapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// The database ingest through the API, against a real SQL Server. The unit
// tests cover the reading; this covers the thing an administrator actually
// does: point the importer at their system of record and see what it would do.
//
// Start the database and seed it first:
//
//	make mssql-up && make seed-mssql
//
// then set CMM_TEST_MSSQL_DSN.
func mssqlDSNForAPI(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("CMM_TEST_MSSQL_DSN")
	if dsn == "" {
		t.Skip("CMM_TEST_MSSQL_DSN is not set; run: make mssql-up && make seed-mssql")
	}
	return dsn
}

const apiOwnerQuery = `
	SELECT s.email AS owner_name, a.asset_kind AS entity_type,
	       a.asset_name AS entity_key, s.full_name AS display_name
	FROM asset_owner a
	LEFT JOIN staff s ON s.staff_id = a.staff_id
	WHERE s.left_company = 0 OR s.left_company IS NULL
	ORDER BY a.asset_id`

// databaseIntakeForm builds the multipart body naming a database source.
func databaseIntakeForm(t *testing.T, credential, query string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	for k, v := range map[string]string{
		"source_type":   "database",
		"db_driver":     "sqlserver",
		"db_credential": credential,
		"db_query":      query,
	} {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("writing field %s: %v", k, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing the form: %v", err)
	}
	return body, w.FormDataContentType()
}

func TestFunctional_IntakeProfile_ReadsFromSQLServer(t *testing.T) {
	dsn := mssqlDSNForAPI(t)

	credStore := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, credStore, "cmdb-connection", "generic", dsn)
	r := newTestRouterWithCredentials(credStore)

	body, contentType := databaseIntakeForm(t, "cmdb-connection", apiOwnerQuery)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/import/profile", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var profile struct {
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
		RowCount int `json:"row_count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &profile); err != nil {
		t.Fatalf("decoding the profile: %v; body: %s", err, w.Body.String())
	}
	if len(profile.Columns) != 4 {
		t.Errorf("profiled %d columns, want 4: %+v", len(profile.Columns), profile.Columns)
	}
	// The profile is what the mapping screen offers, so the query's own column
	// names have to survive to it.
	names := map[string]bool{}
	for _, c := range profile.Columns {
		names[c.Name] = true
	}
	for _, want := range []string{"owner_name", "entity_type", "entity_key", "display_name"} {
		if !names[want] {
			t.Errorf("column %q missing from the profile: %v", want, names)
		}
	}
}

// The "Browse tables" button, end to end against a real SQL Server. It exists
// because whoever sets an import up usually cannot inspect the database, so
// the alternative is writing a query blind against a schema they cannot see.
// Nothing had ever run this query: the endpoint was unreachable, so the button
// answered an error and the SQL underneath it was never executed.
func TestFunctional_IntakeListTables_ReadsFromSQLServer(t *testing.T) {
	dsn := mssqlDSNForAPI(t)

	credStore := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, credStore, "cmdb-connection", "generic", dsn)
	r := newTestRouterWithCredentials(credStore)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	for k, v := range map[string]string{
		"db_driver":     "sqlserver",
		"db_credential": "cmdb-connection",
	} {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("writing field %s: %v", k, err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("closing the form: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/import/tables", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var out struct {
		Data []struct {
			Schema        string `json:"schema"`
			Name          string `json:"name"`
			QualifiedName string `json:"qualified_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the table list: %v; body: %s", err, w.Body.String())
	}

	// The seeded ownership tables have to be offered, or the button lists
	// something other than what the reader came to find.
	byName := map[string]string{}
	for _, tbl := range out.Data {
		byName[tbl.Name] = tbl.QualifiedName
	}
	for _, want := range []string{"staff", "asset_owner"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("table %q missing from the list: %v", want, byName)
		}
	}
	// The name is quoted here rather than in the browser, because how an
	// identifier is quoted is a property of the database. It has to arrive
	// ready to drop into a query.
	if got := byName["asset_owner"]; got != "[dbo].[asset_owner]" {
		t.Errorf("qualified name = %q, want %q", got, "[dbo].[asset_owner]")
	}
}

// The connection string is never taken from the request. Without a stored
// credential the request must be refused, so a password cannot be pasted into
// a URL, a log or a browser's history.
func TestFunctional_IntakeProfile_RefusesWithoutAStoredCredential(t *testing.T) {
	mssqlDSNForAPI(t)

	credStore := newMockCredentialStore(testCredentialEncryptor(t))
	r := newTestRouterWithCredentials(credStore)

	body, contentType := databaseIntakeForm(t, "", apiOwnerQuery)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/import/profile", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	assertBodyContains(t, w, "db_credential is required")
}

// A query the server rejects must come back as a bad request naming the
// problem, not as an empty profile that reads like an empty database.
func TestFunctional_IntakeProfile_ReportsABadQuery(t *testing.T) {
	dsn := mssqlDSNForAPI(t)

	credStore := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, credStore, "cmdb-connection", "generic", dsn)
	r := newTestRouterWithCredentials(credStore)

	body, contentType := databaseIntakeForm(t, "cmdb-connection", "SELECT * FROM no_such_table")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/import/profile", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	assertBodyContains(t, w, "Could not read from the database")
}
