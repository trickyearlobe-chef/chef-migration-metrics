// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const sampleFieldMap = `{"owner":{"source":{"kind":"column","column":"Owner Email"},` +
	`"transforms":[{"kind":"trim"},{"kind":"strip_domain"}]},` +
	`"entity_type":{"source":{"kind":"constant","value":"git_repo"}},` +
	`"entity_key":{"source":{"kind":"column","column":"Repo"}}}`

func insertTestMapping(t *testing.T, db *DB, name string) ImportMapping {
	t.Helper()
	m, err := db.InsertImportMapping(context.Background(), InsertImportMappingParams{
		Name:      name,
		Delimiter: ";",
		FieldMap:  json.RawMessage(sampleFieldMap),
		CreatedBy: "tester",
	})
	if err != nil {
		t.Fatalf("InsertImportMapping(%q): %v", name, err)
	}
	t.Cleanup(func() { _ = db.DeleteImportMapping(context.Background(), m.ID) })
	return m
}

func TestFunctional_ImportMapping_InsertAndGet(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	created := insertTestMapping(t, db, "acme-export")

	if created.ID == 0 {
		t.Error("InsertImportMapping returned a zero id")
	}
	if created.SourceKind != "csv" {
		t.Errorf("SourceKind = %q, want the default %q", created.SourceKind, "csv")
	}
	if created.Delimiter != ";" {
		t.Errorf("Delimiter = %q, want %q", created.Delimiter, ";")
	}
	if created.CreatedBy != "tester" {
		t.Errorf("CreatedBy = %q", created.CreatedBy)
	}

	got, err := db.GetImportMapping(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetImportMapping: %v", err)
	}
	// The document is an API contract stored in a table and outlives the code,
	// so it must come back out semantically identical to what went in.
	var in, out map[string]any
	if err := json.Unmarshal([]byte(sampleFieldMap), &in); err != nil {
		t.Fatalf("unmarshalling the input document: %v", err)
	}
	if err := json.Unmarshal(got.FieldMap, &out); err != nil {
		t.Fatalf("unmarshalling the stored document: %v — got %s", err, got.FieldMap)
	}
	if len(in) != len(out) {
		t.Fatalf("stored document has %d fields, want %d: %s", len(out), len(in), got.FieldMap)
	}
	owner, ok := out["owner"].(map[string]any)
	if !ok {
		t.Fatalf("stored document lost its owner field: %s", got.FieldMap)
	}
	transforms, ok := owner["transforms"].([]any)
	if !ok || len(transforms) != 2 {
		t.Errorf("stored document lost the owner transform chain: %s", got.FieldMap)
	}
}

func TestFunctional_ImportMapping_DefaultsDelimiterToComma(t *testing.T) {
	db := testDB(t)
	m, err := db.InsertImportMapping(context.Background(), InsertImportMappingParams{
		Name:     "defaults",
		FieldMap: json.RawMessage(sampleFieldMap),
	})
	if err != nil {
		t.Fatalf("InsertImportMapping: %v", err)
	}
	t.Cleanup(func() { _ = db.DeleteImportMapping(context.Background(), m.ID) })

	if m.Delimiter != "," {
		t.Errorf("Delimiter = %q, want %q", m.Delimiter, ",")
	}
	if m.CreatedBy != "" {
		t.Errorf("CreatedBy = %q, want empty", m.CreatedBy)
	}
}

func TestFunctional_ImportMapping_DuplicateNameIsAlreadyExists(t *testing.T) {
	db := testDB(t)
	insertTestMapping(t, db, "collide")

	_, err := db.InsertImportMapping(context.Background(), InsertImportMappingParams{
		Name:     "collide",
		FieldMap: json.RawMessage(sampleFieldMap),
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("second insert = %v, want ErrAlreadyExists", err)
	}
}

func TestFunctional_ImportMapping_RejectsAnUnknownSourceKind(t *testing.T) {
	// The CHECK is what keeps a SQL source from arriving by accident ahead of
	// the code that can read one.
	db := testDB(t)
	_, err := db.InsertImportMapping(context.Background(), InsertImportMappingParams{
		Name:       "mssql-attempt",
		SourceKind: "mssql",
		FieldMap:   json.RawMessage(sampleFieldMap),
	})
	if err == nil {
		t.Fatal("InsertImportMapping with source_kind 'mssql' = nil error, want the CHECK to reject it")
	}
	// Assert on the constraint by name. "any error" would also be satisfied by
	// the table not existing at all, which is how this test passed against a
	// database the migration had never reached.
	if !strings.Contains(err.Error(), "chk_ownership_import_mapping_source_kind") {
		t.Errorf("error = %v, want a violation of chk_ownership_import_mapping_source_kind", err)
	}
}

func TestFunctional_ImportMapping_ListOmitsTheFieldMap(t *testing.T) {
	// A page of mapping documents is a lot of JSON nobody on the list screen
	// reads.
	db := testDB(t)
	insertTestMapping(t, db, "list-me")

	mappings, total, err := db.ListImportMappings(context.Background(), 50, 0)
	if err != nil {
		t.Fatalf("ListImportMappings: %v", err)
	}
	if total < 1 {
		t.Fatalf("total = %d, want at least 1", total)
	}

	var found bool
	for _, m := range mappings {
		if m.Name != "list-me" {
			continue
		}
		found = true
		if len(m.FieldMap) != 0 {
			t.Errorf("list returned a field map: %s", m.FieldMap)
		}
		if m.Delimiter != ";" {
			t.Errorf("Delimiter = %q, want %q", m.Delimiter, ";")
		}
	}
	if !found {
		t.Error("the inserted mapping is missing from the list")
	}
}

func TestFunctional_ImportMapping_Update(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	created := insertTestMapping(t, db, "before")

	replacement := `{"owner":{"source":{"kind":"constant","value":"platform"}},` +
		`"entity_type":{"source":{"kind":"constant","value":"node"}},` +
		`"entity_key":{"source":{"kind":"column","column":"Host"}}}`

	updated, err := db.UpdateImportMapping(ctx, created.ID, UpdateImportMappingParams{
		Name:      "after",
		Delimiter: "|",
		FieldMap:  json.RawMessage(replacement),
	})
	if err != nil {
		t.Fatalf("UpdateImportMapping: %v", err)
	}
	if updated.Name != "after" || updated.Delimiter != "|" {
		t.Errorf("update did not take: %+v", updated)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) && !updated.UpdatedAt.Equal(created.UpdatedAt) {
		t.Errorf("UpdatedAt went backwards: %v then %v", created.UpdatedAt, updated.UpdatedAt)
	}

	got, err := db.GetImportMapping(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetImportMapping: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(got.FieldMap, &doc); err != nil {
		t.Fatalf("unmarshalling the replaced document: %v", err)
	}
	if _, ok := doc["organisation"]; ok {
		t.Error("update merged rather than replaced the document")
	}
}

func TestFunctional_ImportMapping_UpdateAndDeleteReportNotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.UpdateImportMapping(ctx, 999999999, UpdateImportMappingParams{
		Name:     "nope",
		FieldMap: json.RawMessage(sampleFieldMap),
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateImportMapping on a missing id = %v, want ErrNotFound", err)
	}

	if err := db.DeleteImportMapping(ctx, 999999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteImportMapping on a missing id = %v, want ErrNotFound", err)
	}
	if _, err := db.GetImportMapping(ctx, 999999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetImportMapping on a missing id = %v, want ErrNotFound", err)
	}
}

func TestFunctional_ImportMapping_UpdateToATakenNameIsAlreadyExists(t *testing.T) {
	db := testDB(t)
	insertTestMapping(t, db, "taken")
	other := insertTestMapping(t, db, "mine")

	_, err := db.UpdateImportMapping(context.Background(), other.ID, UpdateImportMappingParams{
		Name:     "taken",
		FieldMap: json.RawMessage(sampleFieldMap),
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("rename onto a taken name = %v, want ErrAlreadyExists", err)
	}
}

func TestFunctional_ImportMapping_Delete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	created := insertTestMapping(t, db, "delete-me")

	if err := db.DeleteImportMapping(ctx, created.ID); err != nil {
		t.Fatalf("DeleteImportMapping: %v", err)
	}
	if _, err := db.GetImportMapping(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetImportMapping after delete = %v, want ErrNotFound", err)
	}
}

func TestFunctional_LookupAssignmentOwnersByEntity(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	owners := []string{"intake-alice", "intake-bob"}
	for _, name := range owners {
		if _, err := db.InsertOwner(ctx, InsertOwnerParams{Name: name, OwnerType: "individual"}); err != nil {
			t.Fatalf("InsertOwner(%q): %v", name, err)
		}
		t.Cleanup(func() { _, _ = db.DeleteOwner(context.Background(), name) })
	}

	// Two owners on one repo — the source data carries this routinely, and it
	// is what owned_by_other reports.
	for _, name := range owners {
		if _, err := db.InsertAssignment(ctx, InsertAssignmentParams{
			OwnerName:        name,
			EntityType:       "git_repo",
			EntityKey:        "intake-web-app",
			AssignmentSource: "import",
			Confidence:       "definitive",
		}); err != nil {
			t.Fatalf("InsertAssignment(%q): %v", name, err)
		}
	}

	got, err := db.LookupAssignmentOwnersByEntity(ctx, "git_repo", []string{"intake-web-app", "intake-never-assigned"})
	if err != nil {
		t.Fatalf("LookupAssignmentOwnersByEntity: %v", err)
	}
	if len(got["intake-web-app"]) != 2 {
		t.Errorf("intake-web-app has %d assignments, want 2: %+v", len(got["intake-web-app"]), got["intake-web-app"])
	}
	// A key with no assignment is absent, not present and empty — a caller that
	// distinguishes the two must be able to.
	if _, present := got["intake-never-assigned"]; present {
		t.Error("an unassigned key is present in the result")
	}
	// organisation_name is NULL here and must come back as an empty string, not
	// as a scan failure.
	for _, a := range got["intake-web-app"] {
		if a.OrganisationName != "" {
			t.Errorf("OrganisationName = %q, want empty for a NULL column", a.OrganisationName)
		}
	}
}

func TestFunctional_LookupAssignmentOwnersByEntity_EmptyInputIsNotAQuery(t *testing.T) {
	db := testDB(t)
	got, err := db.LookupAssignmentOwnersByEntity(context.Background(), "git_repo", nil)
	if err != nil {
		t.Fatalf("LookupAssignmentOwnersByEntity with no keys: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want an empty map", got)
	}
}

// Git history routinely carries one person under several domains — a corporate
// address, a noreply address, a personal one — sharing a localpart.
func TestFunctional_SuggestOwnersByEmailLocalpart(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	seed := []struct{ owner, aliasType, aliasValue string }{
		{"intake-localpart-a", "git_email", "sameuser@corp.example"},
		{"intake-localpart-b", "git_email", "sameuser@users.noreply.example"},
		{"intake-localpart-c", "email", "SameUser@Other.Example"},
		{"intake-localpart-d", "git_email", "differentuser@corp.example"},
		// A username alias with the same text must not match: the query is
		// about email localparts, and a username is not an address.
		{"intake-localpart-e", "username", "sameuser"},
	}
	for _, s := range seed {
		if _, err := db.InsertOwner(ctx, InsertOwnerParams{Name: s.owner, OwnerType: "individual"}); err != nil {
			t.Fatalf("InsertOwner(%q): %v", s.owner, err)
		}
		t.Cleanup(func() { _, _ = db.DeleteOwner(context.Background(), s.owner) })

		if _, err := db.InsertOwnerAlias(ctx, InsertOwnerAliasParams{
			OwnerName: s.owner, AliasType: s.aliasType, AliasValue: s.aliasValue, Source: "manual",
		}); err != nil {
			t.Fatalf("InsertOwnerAlias(%q): %v", s.aliasValue, err)
		}
	}

	got, err := db.SuggestOwnersByEmailLocalpart(ctx, "sameuser", 10)
	if err != nil {
		t.Fatalf("SuggestOwnersByEmailLocalpart: %v", err)
	}

	found := map[string]bool{}
	for _, s := range got {
		found[s.OwnerName] = true
	}
	for _, want := range []string{"intake-localpart-a", "intake-localpart-b"} {
		if !found[want] {
			t.Errorf("%s missing from the suggestions: %+v", want, got)
		}
	}
	// Case must not decide it — export casing varies freely.
	if !found["intake-localpart-c"] {
		t.Errorf("a differently-cased address was missed: %+v", got)
	}
	if found["intake-localpart-d"] {
		t.Errorf("a different localpart was suggested: %+v", got)
	}
	if found["intake-localpart-e"] {
		t.Errorf("a username alias was treated as an address: %+v", got)
	}
}

func TestFunctional_SuggestOwnersByEmailLocalpart_EmptyInputIsNotAQuery(t *testing.T) {
	db := testDB(t)
	got, err := db.SuggestOwnersByEmailLocalpart(context.Background(), "", 10)
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v; want no suggestions and no error", got, err)
	}
}

func TestFunctional_EntityKeysExist(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Every entity type the assignment CHECK permits must be queryable without
	// erroring, whether or not anything has been collected. A query against a
	// column that does not exist fails here and nowhere else.
	for _, entityType := range []string{"node", "cookbook", "git_repo", "role", "policy"} {
		got, err := db.EntityKeysExist(ctx, entityType, []string{"definitely-not-collected"})
		if err != nil {
			t.Errorf("EntityKeysExist(%q) = %v, want no error", entityType, err)
			continue
		}
		if got["definitely-not-collected"] {
			t.Errorf("EntityKeysExist(%q) claims an uncollected key exists", entityType)
		}
	}
}

func TestFunctional_EntityKeysExist_FindsACollectedRepo(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	var name string
	err := db.pool.QueryRowContext(ctx, `SELECT name FROM git_repos LIMIT 1`).Scan(&name)
	if err != nil {
		t.Skip("no git repos collected in the test database")
	}

	got, err := db.EntityKeysExist(ctx, "git_repo", []string{name, "definitely-not-collected"})
	if err != nil {
		t.Fatalf("EntityKeysExist: %v", err)
	}
	if !got[name] {
		t.Errorf("EntityKeysExist did not find the collected repo %q", name)
	}
	if got["definitely-not-collected"] {
		t.Error("EntityKeysExist claims an uncollected key exists")
	}
}

// ---------------------------------------------------------------------------
// Scheduled database imports (migration 0064)
//
// The SQL below is hand-written and carries thirteen placeholders, so these
// exercise it against a real database rather than a mock: a mis-ordered
// parameter would store the query in the credential column and nothing in Go
// would notice.
// ---------------------------------------------------------------------------

func insertScheduledImport(t *testing.T, db *DB, name, cron string) ImportMapping {
	t.Helper()
	m, err := db.InsertImportMapping(context.Background(), InsertImportMappingParams{
		Name:       name,
		SourceKind: "database",
		FieldMap:   json.RawMessage(sampleFieldMap),
		CreatedBy:  "tester",
		ImportMappingSource: ImportMappingSource{
			DBConnection:    "cmdb-connection",
			DBQuery:         "SELECT owner, repo FROM asset_owner",
			FilterColumn:    "asset_kind",
			FilterValue:     "git_repo",
			CreateOwners:    true,
			Schedule:        cron,
			ScheduleEnabled: cron != "",
		},
	})
	if err != nil {
		t.Fatalf("InsertImportMapping(%q): %v", name, err)
	}
	t.Cleanup(func() { _ = db.DeleteImportMapping(context.Background(), m.ID) })
	return m
}

func TestFunctional_ScheduledImport_RoundTripsItsSource(t *testing.T) {
	db := testDB(t)

	created := insertScheduledImport(t, db, "cmdb-nightly", "0 2 * * *")

	// Read back rather than trusting the RETURNING clause alone: the columns
	// have to line up in the scanner as well as in the insert.
	got, err := db.GetImportMapping(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetImportMapping: %v", err)
	}
	for _, c := range []struct{ field, got, want string }{
		{"db_connection", got.DBConnection, "cmdb-connection"},
		{"db_query", got.DBQuery, "SELECT owner, repo FROM asset_owner"},
		{"filter_column", got.FilterColumn, "asset_kind"},
		{"filter_value", got.FilterValue, "git_repo"},
		{"schedule", got.Schedule, "0 2 * * *"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
		}
	}
	if !got.CreateOwners || !got.ScheduleEnabled {
		t.Errorf("create_owners = %v, schedule_enabled = %v, want both true", got.CreateOwners, got.ScheduleEnabled)
	}
	if got.LastRunAt != nil {
		t.Errorf("last_run_at = %v on a fresh import, want nil so 'never run' is distinguishable", got.LastRunAt)
	}
}

func TestFunctional_ListScheduledImports_ReturnsOnlyTheEnabledOnes(t *testing.T) {
	db := testDB(t)

	scheduled := insertScheduledImport(t, db, "cmdb-nightly", "0 2 * * *")
	unscheduled := insertScheduledImport(t, db, "cmdb-on-demand", "")

	list, err := db.ListScheduledImports(context.Background())
	if err != nil {
		t.Fatalf("ListScheduledImports: %v", err)
	}

	byID := map[int64]ImportMapping{}
	for _, m := range list {
		byID[m.ID] = m
	}
	if _, ok := byID[scheduled.ID]; !ok {
		t.Error("the scheduled import is missing, so the scheduler would never run it")
	}
	if _, ok := byID[unscheduled.ID]; ok {
		t.Error("an import with its schedule off was returned, so it would run unasked")
	}
	// The scheduler runs the definition it is handed, so the field map has to
	// come with it — the summary columns would leave nothing to map with.
	if len(byID[scheduled.ID].FieldMap) == 0 {
		t.Error("the scheduled import came back without its field map")
	}
}

func TestFunctional_RecordImportRun_StoresTheOutcome(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	m := insertScheduledImport(t, db, "cmdb-nightly", "0 2 * * *")

	if err := db.RecordImportRun(ctx, m.ID, "failed", "could not read the credential"); err != nil {
		t.Fatalf("RecordImportRun: %v", err)
	}

	got, err := db.GetImportMapping(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetImportMapping: %v", err)
	}
	if got.LastRunStatus != "failed" {
		t.Errorf("last_run_status = %q, want %q", got.LastRunStatus, "failed")
	}
	if !strings.Contains(got.LastRunDetail, "credential") {
		t.Errorf("last_run_detail = %q, want the reason", got.LastRunDetail)
	}
	if got.LastRunAt == nil {
		t.Error("last_run_at is still unset, so a run that happened reads as never run")
	}
}

// The constraint that stops an unrunnable schedule being stored. Enforced in
// the database as well as the API because an enabled schedule with nothing to
// connect to is indistinguishable from a broken scheduler when somebody comes
// to ask why nothing happened.
func TestFunctional_ScheduledImport_CannotBeEnabledWithNothingToRun(t *testing.T) {
	db := testDB(t)

	_, err := db.InsertImportMapping(context.Background(), InsertImportMappingParams{
		Name:       "unrunnable",
		SourceKind: "database",
		FieldMap:   json.RawMessage(sampleFieldMap),
		ImportMappingSource: ImportMappingSource{
			Schedule:        "0 2 * * *",
			ScheduleEnabled: true,
			// No credential and no query.
		},
	})
	if err == nil {
		t.Fatal("an enabled schedule with no connection was stored; it would never run and nothing would say why")
	}
}
