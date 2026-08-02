// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipimport"
)

// withViewerSession attaches a read-only session. Without a session at all the
// handlers take the dev-mode path and skip the role check, so a test asserting
// a 403 has to supply one.
func withViewerSession(req *http.Request) *http.Request {
	return req.WithContext(auth.ContextWithSession(req.Context(),
		&auth.SessionInfo{Username: "viewer", Role: "viewer"}))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// intakeRequest builds a multipart request the intake endpoints accept.
func intakeRequest(t *testing.T, path, csv string, fields map[string]string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("writing field %q: %v", k, err)
		}
	}
	if csv != "" {
		part, err := mw.CreateFormFile("file", "ownership.csv")
		if err != nil {
			t.Fatalf("creating the file part: %v", err)
		}
		if _, err := part.Write([]byte(csv)); err != nil {
			t.Fatalf("writing the file part: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("closing the multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// repoFieldMap maps an owner column and a repo column onto git_repo
// assignments — the shape the ownership exports actually arrive in.
func repoFieldMap(t *testing.T, ownerTransforms ...ownershipimport.Transform) string {
	t.Helper()
	fm := ownershipimport.FieldMap{
		ownershipimport.FieldOwner: {
			Source:     ownershipimport.Source{Kind: ownershipimport.SourceColumn, Column: "Owner"},
			Transforms: ownerTransforms,
		},
		ownershipimport.FieldEntityType: {
			Source: ownershipimport.Source{Kind: ownershipimport.SourceConstant, Value: "git_repo"},
		},
		ownershipimport.FieldEntityKey: {
			Source: ownershipimport.Source{Kind: ownershipimport.SourceColumn, Column: "Repo"},
		},
	}
	encoded, err := json.Marshal(fm)
	if err != nil {
		t.Fatalf("marshalling the field map: %v", err)
	}
	return string(encoded)
}

func decodeReport(t *testing.T, w *httptest.ResponseRecorder) ownershipimport.Report {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var report ownershipimport.Report
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("decoding the report: %v — body %s", err, w.Body.String())
	}
	return report
}

func rowFor(t *testing.T, report ownershipimport.Report, sourceRow int) ownershipimport.ReportRow {
	t.Helper()
	for _, r := range report.Rows {
		if r.SourceRow == sourceRow {
			return r
		}
	}
	t.Fatalf("no report row numbered %d", sourceRow)
	return ownershipimport.ReportRow{}
}

const twoRowCSV = "Owner,Repo\nAlice Smith,web-app\nBob Jones,db-tools\n"

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/import/profile
// ---------------------------------------------------------------------------

func TestIntakeProfile_DescribesTheSource(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	csv := "Owner Email,Repo,Org\n" +
		"alice@example.com,web-app,acme\n" +
		"bob@example.com,db-tools,acme\n" +
		"alice@example.com,api,\n"
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/profile", csv, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var profile ownershipimport.SourceProfile
	if err := json.Unmarshal(w.Body.Bytes(), &profile); err != nil {
		t.Fatalf("decoding the profile: %v", err)
	}

	if profile.RowCount != 3 {
		t.Errorf("RowCount = %d, want 3", profile.RowCount)
	}
	if len(profile.Columns) != 3 {
		t.Fatalf("got %d columns, want 3", len(profile.Columns))
	}
	// Source order, not alphabetical — it is how the administrator sees the file.
	if profile.Columns[0].Name != "Owner Email" || profile.Columns[2].Name != "Org" {
		t.Errorf("columns out of source order: %+v", profile.Columns)
	}
	if profile.Columns[0].DistinctCount != 2 {
		t.Errorf("Owner Email DistinctCount = %d, want 2", profile.Columns[0].DistinctCount)
	}
}

func TestIntakeProfile_PersistsNothing(t *testing.T) {
	var wrote bool
	store := &mockStore{
		InsertAssignmentFn: func(_ context.Context, _ datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			wrote = true
			return datastore.OwnershipAssignment{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/profile", twoRowCSV, nil))

	if wrote {
		t.Error("profiling wrote an assignment")
	}
}

func TestIntakeProfile_HonoursAnExplicitDelimiter(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	csv := "Owner;Repo\nAlice Smith;web-app\n"
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/profile", csv, map[string]string{"delimiter": ";"}))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var profile ownershipimport.SourceProfile
	_ = json.Unmarshal(w.Body.Bytes(), &profile)
	if len(profile.Columns) != 2 {
		t.Errorf("got %d columns, want 2 — the explicit delimiter was ignored", len(profile.Columns))
	}
}

func TestIntakeProfile_RequiresAFile(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/profile", "", nil))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestIntakeProfile_RejectsGET(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/import/profile", nil))

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/import/preview
// ---------------------------------------------------------------------------

func TestIntakePreview_WritesNothing(t *testing.T) {
	var wrote bool
	store := &mockStore{
		InsertAssignmentFn: func(_ context.Context, _ datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			wrote = true
			return datastore.OwnershipAssignment{}, nil
		},
		InsertOwnerFn: func(_ context.Context, _ datastore.InsertOwnerParams) (datastore.Owner, error) {
			wrote = true
			return datastore.Owner{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", twoRowCSV, map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	if wrote {
		t.Error("preview wrote to the datastore")
	}
	if report.Committed {
		t.Error("preview reported itself as committed")
	}
	if report.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2", report.RowCount)
	}
}

// A person's name is a first-class identifier in these exports — the incoming
// data is as likely to carry "Alice Smith" as an email address.
func TestIntakePreview_ResolvesAPersonNameThroughAnAlias(t *testing.T) {
	var askedFor []string
	store := &mockStore{
		GetOwnerByNameFn: func(_ context.Context, _ string) (datastore.Owner, error) {
			return datastore.Owner{}, datastore.ErrNotFound
		},
		ResolveOwnerByAliasFn: func(_ context.Context, aliasType, aliasValue string) (string, error) {
			askedFor = append(askedFor, aliasType+"="+aliasValue)
			if aliasType == "git_name" && aliasValue == "Alice Smith" {
				return "asmith", nil
			}
			return "", datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", "Owner,Repo\nAlice Smith,web-app\n", map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	row := rowFor(t, report, 1)
	if row.OwnerMatch != ownershipimport.OwnerMatchAlias {
		t.Errorf("OwnerMatch = %q, want %q (tried: %v)", row.OwnerMatch, ownershipimport.OwnerMatchAlias, askedFor)
	}
	if row.Owner != "asmith" {
		t.Errorf("Owner = %q, want the alias's owner %q", row.Owner, "asmith")
	}
	if row.Outcome != ownershipimport.OutcomeWouldCreate {
		t.Errorf("Outcome = %q, want %q", row.Outcome, ownershipimport.OutcomeWouldCreate)
	}
}

func TestIntakePreview_ResolvesAnExactOwnerName(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(_ context.Context, name string) (datastore.Owner, error) {
			if name == "alice-smith" {
				return datastore.Owner{Name: name}, nil
			}
			return datastore.Owner{}, datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", "Owner,Repo\nAlice Smith,web-app\n", map[string]string{
		"field_map": repoFieldMap(t),
	}))

	row := rowFor(t, decodeReport(t, w), 1)
	if row.OwnerMatch != ownershipimport.OwnerMatchExact {
		t.Errorf("OwnerMatch = %q, want %q", row.OwnerMatch, ownershipimport.OwnerMatchExact)
	}
}

// A close match is never *applied* — the row is not attributed to the suggested
// owner. But it does not reject either: the assignment is ingested under a new
// owner and the suggestion travels with it, because stopping a 20,000-row
// import to adjudicate names is the expensive thing, and the recognisable clue
// survives for whoever later asks who this person is.
func TestIntakePreview_FuzzyCandidatesNeitherApplyNorReject(t *testing.T) {
	store := &mockStore{
		SuggestOwnerAliasesFn: func(_ context.Context, input string, _ int) ([]datastore.AliasSuggestion, error) {
			return []datastore.AliasSuggestion{
				{OwnerName: "asmith", AliasValue: "Alice Smyth", Similarity: 0.82},
				{OwnerName: "a-smith", AliasValue: "A. Smith", Similarity: 0.55},
			}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", "Owner,Repo\nAlice Smith,web-app\n", map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	row := rowFor(t, report, 1)
	if row.OwnerMatch != ownershipimport.OwnerMatchFuzzy {
		t.Fatalf("OwnerMatch = %q, want %q", row.OwnerMatch, ownershipimport.OwnerMatchFuzzy)
	}
	if row.Outcome == ownershipimport.OutcomeRejected {
		t.Error("a close match rejected the row — the assignment is lost at ingest")
	}
	// Crucially: NOT attributed to the suggested owner.
	if row.Owner == "asmith" {
		t.Error("the suggestion was applied — a fuzzy match must never be attributed automatically")
	}
	if row.Owner != "alice-smith" {
		t.Errorf("Owner = %q, want a new owner from the raw value", row.Owner)
	}
	if !row.CreatesOwner {
		t.Error("CreatesOwner = false — the new person is invisible in the preview")
	}
	if len(row.OwnerSuggestions) == 0 {
		t.Error("no suggestions carried — the clue was dropped")
	}
	if len(row.OwnerSuggestions) > ownershipimport.MaxOwnerSuggestions {
		t.Errorf("%d suggestions, want at most %d", len(row.OwnerSuggestions), ownershipimport.MaxOwnerSuggestions)
	}
}

// Every person the import would add is listed as a person, not buried in an
// assignment count. This is the section someone scans to recognise a nickname
// that no similarity score could ever match.
func TestIntakePreview_ListsNewPeopleForReview(t *testing.T) {
	store := &mockStore{
		SuggestOwnerAliasesFn: func(_ context.Context, input string, _ int) ([]datastore.AliasSuggestion, error) {
			if input == "Fat Tommy" {
				return nil, nil // no score comes close; only a human would know
			}
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	csv := "Owner,Repo\n" +
		"Fat Tommy,web-app\n" +
		"Fat Tommy,db-tools\n" +
		"Alice Smith,api\n"
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", csv, map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	if len(report.NewOwners) != 2 {
		t.Fatalf("got %d new people, want 2: %+v", len(report.NewOwners), report.NewOwners)
	}

	var tommy *ownershipimport.NewOwner
	for i := range report.NewOwners {
		if report.NewOwners[i].SourceValue == "Fat Tommy" {
			tommy = &report.NewOwners[i]
		}
	}
	if tommy == nil {
		t.Fatalf("Fat Tommy is not listed: %+v", report.NewOwners)
	}
	// The raw string is what a human recognises — the slug is not.
	if tommy.DisplayName != "Fat Tommy" {
		t.Errorf("DisplayName = %q, want the raw value", tommy.DisplayName)
	}
	if tommy.Name != "fat-tommy" {
		t.Errorf("Name = %q, want the slug", tommy.Name)
	}
	// One person, two rows — listed once, so the list stays scannable.
	if tommy.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2", tommy.RowCount)
	}
}

func TestIntakePreview_NewPersonCarriesItsSuggestions(t *testing.T) {
	store := &mockStore{
		SuggestOwnerAliasesFn: func(_ context.Context, _ string, _ int) ([]datastore.AliasSuggestion, error) {
			return []datastore.AliasSuggestion{{OwnerName: "asmith", Similarity: 0.82}}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", "Owner,Repo\nAlice Smith,web-app\n", map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	if len(report.NewOwners) != 1 {
		t.Fatalf("got %d new people, want 1", len(report.NewOwners))
	}
	if len(report.NewOwners[0].Suggestions) == 0 {
		t.Error("the new person does not carry the owner it might already be")
	}
}

// With nobody to confuse them with, an unrecognised person is created. This is
// the first-import path: requiring owners to pre-exist would make the feature
// useless on an empty deployment.
func TestIntakePreview_UnknownOwnerWithNoCandidatesWouldBeCreated(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", "Owner,Repo\nAlice Smith,web-app\n", map[string]string{
		"field_map": repoFieldMap(t),
	}))

	row := rowFor(t, decodeReport(t, w), 1)
	if row.Outcome != ownershipimport.OutcomeWouldCreate {
		t.Errorf("Outcome = %q, want %q", row.Outcome, ownershipimport.OutcomeWouldCreate)
	}
	if !row.CreatesOwner {
		t.Error("CreatesOwner = false, want true")
	}
	if row.Owner != "alice-smith" {
		t.Errorf("Owner = %q, want the slug", row.Owner)
	}
	if row.DisplayName != "Alice Smith" {
		t.Errorf("DisplayName = %q, want the original string preserved", row.DisplayName)
	}
}

func TestIntakePreview_StrictMatchingRejectsRatherThanCreates(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", "Owner,Repo\nAlice Smith,web-app\n", map[string]string{
		"field_map":     repoFieldMap(t),
		"create_owners": "false",
	}))

	row := rowFor(t, decodeReport(t, w), 1)
	if row.Outcome != ownershipimport.OutcomeRejected {
		t.Errorf("Outcome = %q, want %q", row.Outcome, ownershipimport.OutcomeRejected)
	}
	if row.RejectedReason != ownershipimport.ReasonUnknownOwner {
		t.Errorf("RejectedReason = %q, want %q", row.RejectedReason, ownershipimport.ReasonUnknownOwner)
	}
	if row.CreatesOwner {
		t.Error("CreatesOwner = true with owner creation switched off")
	}
}

func TestIntakePreview_DuplicateAndOverlappingAssignments(t *testing.T) {
	store := &mockStore{
		GetOwnerByNameFn: func(_ context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{Name: name}, nil
		},
		LookupAssignmentOwnersByEntityFn: func(_ context.Context, _ string, _ []string) (map[string][]datastore.EntityAssignment, error) {
			return map[string][]datastore.EntityAssignment{
				"web-app":  {{OwnerName: "alice-smith"}},
				"db-tools": {{OwnerName: "carol-jones"}},
			}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", twoRowCSV, map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)

	same := rowFor(t, report, 1) // alice-smith already holds web-app
	if same.Outcome != ownershipimport.OutcomeDuplicateExists {
		t.Errorf("row 1 Outcome = %q, want %q", same.Outcome, ownershipimport.OutcomeDuplicateExists)
	}

	// A different owner already holds db-tools. Overlapping ownership is
	// legitimate, so this reports the overlap and still creates.
	other := rowFor(t, report, 2)
	if other.Outcome != ownershipimport.OutcomeOwnedByOther {
		t.Errorf("row 2 Outcome = %q, want %q", other.Outcome, ownershipimport.OutcomeOwnedByOther)
	}
	if len(other.ExistingOwners) != 1 || other.ExistingOwners[0] != "carol-jones" {
		t.Errorf("ExistingOwners = %v, want [carol-jones]", other.ExistingOwners)
	}
}

// The alias uniqueness constraint is global, so the raw string may already
// belong to someone else. That skips the alias seed and nothing else.
func TestIntakePreview_AliasConflictDoesNotRecolourTheOutcome(t *testing.T) {
	store := &mockStore{
		ResolveOwnerByAliasFn: func(_ context.Context, aliasType, aliasValue string) (string, error) {
			// The raw string is a custom alias of somebody else, but that
			// somebody is not who this row names.
			if aliasType == "custom" && aliasValue == "Alice Smith" {
				return "someone-else", nil
			}
			return "", datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", "Owner,Repo\nAlice Smith,web-app\n", map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	row := rowFor(t, report, 1)

	// Resolving through the custom alias is a legitimate match, so the row is
	// attributed to that owner — and there is no conflict to report.
	if row.OwnerMatch != ownershipimport.OwnerMatchAlias {
		t.Errorf("OwnerMatch = %q, want %q", row.OwnerMatch, ownershipimport.OwnerMatchAlias)
	}
	if row.Outcome == ownershipimport.OutcomeRejected {
		t.Errorf("Outcome = %q — an alias match must not reject", row.Outcome)
	}
	// Aggregates never sum alias conflicts with outcomes.
	if report.Counts[ownershipimport.OutcomeRejected] != 0 {
		t.Errorf("rejected count = %d, want 0", report.Counts[ownershipimport.OutcomeRejected])
	}
}

// An entity CMM has never collected is a legitimate assignment target:
// assigning ownership before collection has run is a primary use case.
func TestIntakePreview_UncollectedEntityIsReportedNotRejected(t *testing.T) {
	store := &mockStore{
		EntityKeysExistFn: func(_ context.Context, _ string, _ []string) (map[string]bool, error) {
			return map[string]bool{"web-app": true}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", twoRowCSV, map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	known := rowFor(t, report, 1)
	if known.EntityMatch != ownershipimport.EntityMatchFound {
		t.Errorf("row 1 EntityMatch = %q, want %q", known.EntityMatch, ownershipimport.EntityMatchFound)
	}

	unknown := rowFor(t, report, 2)
	if unknown.EntityMatch != ownershipimport.EntityMatchNotFound {
		t.Errorf("row 2 EntityMatch = %q, want %q", unknown.EntityMatch, ownershipimport.EntityMatchNotFound)
	}
	if unknown.Outcome == ownershipimport.OutcomeRejected {
		t.Error("an uncollected entity rejected the row")
	}
}

func TestIntakePreview_RejectsRowsThatCannotMap(t *testing.T) {
	csv := "Owner,Repo\n" +
		"Alice Smith,web-app\n" +
		",orphan-repo\n" + // no owner
		"???,punctuation\n" // cannot become a name
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", csv, map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	if got := rowFor(t, report, 2).RejectedReason; got != ownershipimport.ReasonMissingRequiredField {
		t.Errorf("row 2 RejectedReason = %q, want %q", got, ownershipimport.ReasonMissingRequiredField)
	}
	if got := rowFor(t, report, 3).RejectedReason; got != ownershipimport.ReasonInvalidOwnerName {
		t.Errorf("row 3 RejectedReason = %q, want %q", got, ownershipimport.ReasonInvalidOwnerName)
	}
	if report.Counts[ownershipimport.OutcomeRejected] != 2 {
		t.Errorf("rejected count = %d, want 2", report.Counts[ownershipimport.OutcomeRejected])
	}
}

func TestIntakePreview_ReportsMappingFaultsWithTheirFieldPath(t *testing.T) {
	fm := `{"owner":{"source":{"kind":"column","column":"Nope"}},` +
		`"entity_type":{"source":{"kind":"constant","value":"git_repo"}},` +
		`"entity_key":{"source":{"kind":"column","column":"Repo"}}}`
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", twoRowCSV, map[string]string{"field_map": fm}))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("owner")) {
		t.Errorf("the error does not name the offending field: %s", w.Body.String())
	}
}

func TestIntakePreview_FieldMapAndMappingIDAreMutuallyExclusive(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", twoRowCSV, map[string]string{
		"field_map":  repoFieldMap(t),
		"mapping_id": "1",
	}))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestIntakePreview_UnknownMappingIDIs404(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", twoRowCSV, map[string]string{
		"mapping_id": "424242",
	}))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestIntakePreview_UsesASavedMapping(t *testing.T) {
	store := &mockStore{
		GetImportMappingFn: func(_ context.Context, id int64) (datastore.ImportMapping, error) {
			return datastore.ImportMapping{
				ID:        id,
				Name:      "saved",
				Delimiter: ";",
				FieldMap:  json.RawMessage(repoFieldMap(t)),
			}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	// The saved mapping carries its own delimiter, so this semicolon file
	// parses without the caller restating it.
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview", "Owner;Repo\nAlice Smith;web-app\n", map[string]string{
		"mapping_id": "7",
	}))

	report := decodeReport(t, w)
	if report.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1: %s", report.RowCount, w.Body.String())
	}
	if got := rowFor(t, report, 1).EntityKey; got != "web-app" {
		t.Errorf("EntityKey = %q, want %q", got, "web-app")
	}
}

func TestIntakePreview_DoesNotRequireOperatorRole(t *testing.T) {
	// Preview writes nothing, so it needs only standard protected auth. Making
	// it operator-only would stop the people who own the data from checking it.
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := intakeRequest(t, "/api/v1/ownership/import/preview", twoRowCSV, map[string]string{
		"field_map": repoFieldMap(t),
	})
	r.ServeHTTP(w, withViewerSession(req))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for a viewer: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/import/commit
// ---------------------------------------------------------------------------

func TestIntakeCommit_WritesAssignmentsAndCreatesOwners(t *testing.T) {
	var (
		assignments []datastore.InsertAssignmentParams
		owners      []datastore.InsertOwnerParams
		aliases     []datastore.InsertOwnerAliasParams
	)
	store := &mockStore{
		InsertAssignmentFn: func(_ context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			assignments = append(assignments, p)
			return datastore.OwnershipAssignment{ID: int64(len(assignments))}, nil
		},
		InsertOwnerFn: func(_ context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error) {
			owners = append(owners, p)
			return datastore.Owner{Name: p.Name}, nil
		},
		InsertOwnerAliasFn: func(_ context.Context, p datastore.InsertOwnerAliasParams) (datastore.OwnerAlias, error) {
			aliases = append(aliases, p)
			return datastore.OwnerAlias{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/commit", twoRowCSV, map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	if !report.Committed {
		t.Error("commit did not report itself as committed")
	}
	if report.Created != 2 {
		t.Errorf("Created = %d, want 2", report.Created)
	}
	if len(assignments) != 2 {
		t.Fatalf("wrote %d assignments, want 2", len(assignments))
	}
	if assignments[0].AssignmentSource != "import" {
		t.Errorf("AssignmentSource = %q, want %q", assignments[0].AssignmentSource, "import")
	}
	if len(owners) != 2 {
		t.Errorf("created %d owners, want 2", len(owners))
	}
	if len(owners) == 2 && owners[0].DisplayName != "Alice Smith" {
		t.Errorf("DisplayName = %q, want the original string", owners[0].DisplayName)
	}
	// The raw string is seeded as a custom alias so the original is what future
	// imports and fuzzy matching compare against.
	if len(aliases) != 2 {
		t.Fatalf("seeded %d aliases, want 2", len(aliases))
	}
	if aliases[0].AliasType != "custom" || aliases[0].AliasValue != "Alice Smith" {
		t.Errorf("alias = %+v, want a custom alias of the raw string", aliases[0])
	}
}

// An overlap is not a failure: ownership_assignments is many-to-many and
// skipping the write would silently drop assignments the operator asked for.
func TestIntakeCommit_StillWritesWhenAnotherOwnerHoldsTheEntity(t *testing.T) {
	var written int
	store := &mockStore{
		GetOwnerByNameFn: func(_ context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{Name: name}, nil
		},
		LookupAssignmentOwnersByEntityFn: func(_ context.Context, _ string, _ []string) (map[string][]datastore.EntityAssignment, error) {
			return map[string][]datastore.EntityAssignment{
				"web-app": {{OwnerName: "carol-jones"}},
			}, nil
		},
		InsertAssignmentFn: func(_ context.Context, _ datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			written++
			return datastore.OwnershipAssignment{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/commit", "Owner,Repo\nAlice Smith,web-app\n", map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	if rowFor(t, report, 1).Outcome != ownershipimport.OutcomeOwnedByOther {
		t.Errorf("Outcome = %q, want %q", rowFor(t, report, 1).Outcome, ownershipimport.OutcomeOwnedByOther)
	}
	if written != 1 {
		t.Errorf("wrote %d assignments, want 1 — the overlap must not suppress the write", written)
	}
}

func TestIntakeCommit_DoesNotWriteRejectedRows(t *testing.T) {
	var written int
	store := &mockStore{
		InsertAssignmentFn: func(_ context.Context, _ datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			written++
			return datastore.OwnershipAssignment{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	// Both rows fail to map: no owner, and an owner that cannot become a name.
	csv := "Owner,Repo\n,web-app\n???,db-tools\n"
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/commit", csv, map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	if written != 0 {
		t.Errorf("wrote %d assignments, want 0 — neither row could be mapped", written)
	}
	if report.Counts[ownershipimport.OutcomeRejected] != 2 {
		t.Errorf("rejected count = %d, want 2", report.Counts[ownershipimport.OutcomeRejected])
	}
}

// Strict matching is still available for an administrator importing against an
// established owner catalogue, where a new person really is a mistake.
func TestIntakeCommit_StrictMatchingWritesNothingUnresolved(t *testing.T) {
	var written int
	store := &mockStore{
		InsertAssignmentFn: func(_ context.Context, _ datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			written++
			return datastore.OwnershipAssignment{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/commit", twoRowCSV, map[string]string{
		"field_map":     repoFieldMap(t),
		"create_owners": "false",
	}))

	report := decodeReport(t, w)
	if written != 0 {
		t.Errorf("wrote %d assignments, want 0", written)
	}
	// With creation off, the unmatched strings are what the administrator acts on.
	if len(report.UnmatchedOwners) == 0 {
		t.Error("the unmatched owner strings were not recorded")
	}
	if len(report.NewOwners) != 0 {
		t.Errorf("new people listed with creation switched off: %+v", report.NewOwners)
	}
}

func TestIntakeCommit_RequiresOperatorOrAdmin(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	req := intakeRequest(t, "/api/v1/ownership/import/commit", twoRowCSV, map[string]string{
		"field_map": repoFieldMap(t),
	})
	r.ServeHTTP(w, withViewerSession(req))

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a viewer: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Re-ingesting the same source
//
// These matter for a scheduled import: the same file arrives again after
// somebody has corrected what the last one got wrong.
// ---------------------------------------------------------------------------

// The durable correction is to move the alias. Once the raw string resolves to
// the right person, every future ingest of the same file lands on them.
func TestIntakeReingest_CorrectionSurvivesWhenTheAliasIsMoved(t *testing.T) {
	var written int
	store := &mockStore{
		GetOwnerByNameFn: func(_ context.Context, _ string) (datastore.Owner, error) {
			return datastore.Owner{}, datastore.ErrNotFound
		},
		// Somebody recognised the nickname and re-pointed the alias.
		ResolveOwnerByAliasFn: func(_ context.Context, aliasType, aliasValue string) (string, error) {
			if aliasType == "custom" && aliasValue == "Fat Tommy" {
				return "thomas-smith", nil
			}
			return "", datastore.ErrNotFound
		},
		LookupAssignmentOwnersByEntityFn: func(_ context.Context, _ string, _ []string) (map[string][]datastore.EntityAssignment, error) {
			return map[string][]datastore.EntityAssignment{
				"web-app": {{OwnerName: "thomas-smith"}},
			}, nil
		},
		InsertAssignmentFn: func(_ context.Context, _ datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			written++
			return datastore.OwnershipAssignment{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/commit", "Owner,Repo\nFat Tommy,web-app\n", map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	row := rowFor(t, report, 1)
	if row.Owner != "thomas-smith" {
		t.Errorf("Owner = %q, want the corrected owner", row.Owner)
	}
	if row.Outcome != ownershipimport.OutcomeDuplicateExists {
		t.Errorf("Outcome = %q, want %q", row.Outcome, ownershipimport.OutcomeDuplicateExists)
	}
	if written != 0 {
		t.Errorf("wrote %d assignments, want 0 — re-ingest must be a no-op once corrected", written)
	}
	if len(report.NewOwners) != 0 {
		t.Errorf("re-created a person that was already merged away: %+v", report.NewOwners)
	}
}

// Reassignment alone is NOT durable. ReassignOwnership moves assignments and
// leaves both the owner and its aliases in place
// (internal/datastore/ownership_assignments.go:199), so the raw string still
// resolves to the owner the work was moved off, and the next ingest puts it
// straight back.
//
// This test documents that, so the behaviour cannot change silently. The
// durable correction is POST /api/v1/ownership/merge, which moves the aliases
// as well as the work; reassignment on its own is still what is described here.
func TestIntakeReingest_ReassignmentAloneIsUndoneByTheNextIngest(t *testing.T) {
	var writtenTo []string
	store := &mockStore{
		GetOwnerByNameFn: func(_ context.Context, _ string) (datastore.Owner, error) {
			return datastore.Owner{}, datastore.ErrNotFound
		},
		// The alias was never moved, so it still points at the emptied owner.
		ResolveOwnerByAliasFn: func(_ context.Context, aliasType, aliasValue string) (string, error) {
			if aliasType == "custom" && aliasValue == "Fat Tommy" {
				return "fat-tommy", nil
			}
			return "", datastore.ErrNotFound
		},
		// The work now sits with thomas-smith, moved off fat-tommy.
		LookupAssignmentOwnersByEntityFn: func(_ context.Context, _ string, _ []string) (map[string][]datastore.EntityAssignment, error) {
			return map[string][]datastore.EntityAssignment{
				"web-app": {{OwnerName: "thomas-smith"}},
			}, nil
		},
		InsertAssignmentFn: func(_ context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			writtenTo = append(writtenTo, p.OwnerName)
			return datastore.OwnershipAssignment{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/commit", "Owner,Repo\nFat Tommy,web-app\n", map[string]string{
		"field_map": repoFieldMap(t),
	}))

	report := decodeReport(t, w)
	row := rowFor(t, report, 1)

	// The assignment comes back under the owner it was moved off. The report
	// does say so — owned_by_other names who holds it now — but it is written
	// regardless, and a scheduled import has nobody reading the report.
	if row.Outcome != ownershipimport.OutcomeOwnedByOther {
		t.Errorf("Outcome = %q, want %q", row.Outcome, ownershipimport.OutcomeOwnedByOther)
	}
	if len(writtenTo) != 1 || writtenTo[0] != "fat-tommy" {
		t.Errorf("wrote %v, want the reassignment to be undone under fat-tommy", writtenTo)
	}
	if len(row.ExistingOwners) != 1 || row.ExistingOwners[0] != "thomas-smith" {
		t.Errorf("ExistingOwners = %v, want the corrected owner named", row.ExistingOwners)
	}
}

// ---------------------------------------------------------------------------
// Saved mappings CRUD
// ---------------------------------------------------------------------------

func mappingBody(t *testing.T, name string) string {
	t.Helper()
	return `{"name":"` + name + `","source_kind":"csv","delimiter":";","field_map":` + repoFieldMap(t) + `}`
}

func jsonRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestIntakeMappings_Create(t *testing.T) {
	var got datastore.InsertImportMappingParams
	store := &mockStore{
		InsertImportMappingFn: func(_ context.Context, p datastore.InsertImportMappingParams) (datastore.ImportMapping, error) {
			got = p
			return datastore.ImportMapping{ID: 1, Name: p.Name, Delimiter: p.Delimiter, SourceKind: "csv"}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, jsonRequest(http.MethodPost, "/api/v1/ownership/import/mappings", mappingBody(t, "acme-export")))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	if got.Name != "acme-export" || got.Delimiter != ";" {
		t.Errorf("stored params = %+v", got)
	}
}

func TestIntakeMappings_CreateValidatesTheDocument(t *testing.T) {
	// entity_type from a column is a mapping fault, and it must be caught at
	// save time rather than surfacing per row on a future import.
	body := `{"name":"bad","field_map":{"owner":{"source":{"kind":"column","column":"Owner"}},` +
		`"entity_type":{"source":{"kind":"column","column":"Kind"}},` +
		`"entity_key":{"source":{"kind":"column","column":"Repo"}}}}`
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, jsonRequest(http.MethodPost, "/api/v1/ownership/import/mappings", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("entity_type")) {
		t.Errorf("the error does not name the offending field: %s", w.Body.String())
	}
}

func TestIntakeMappings_DuplicateNameIs409(t *testing.T) {
	store := &mockStore{
		InsertImportMappingFn: func(_ context.Context, _ datastore.InsertImportMappingParams) (datastore.ImportMapping, error) {
			return datastore.ImportMapping{}, datastore.ErrAlreadyExists
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, jsonRequest(http.MethodPost, "/api/v1/ownership/import/mappings", mappingBody(t, "taken")))

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409: %s", w.Code, w.Body.String())
	}
}

func TestIntakeMappings_ListIsPaginated(t *testing.T) {
	store := &mockStore{
		ListImportMappingsFn: func(_ context.Context, limit, offset int) ([]datastore.ImportMapping, int, error) {
			return []datastore.ImportMapping{{ID: 1, Name: "acme-export", SourceKind: "csv"}}, 1, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/import/mappings", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data       []datastore.ImportMapping `json:"data"`
		Pagination PaginationResponse        `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body.Pagination.TotalItems != 1 || len(body.Data) != 1 {
		t.Errorf("body = %+v", body)
	}
}

func TestIntakeMappings_GetUpdateDelete(t *testing.T) {
	var (
		updated datastore.UpdateImportMappingParams
		deleted int64
	)
	store := &mockStore{
		GetImportMappingFn: func(_ context.Context, id int64) (datastore.ImportMapping, error) {
			return datastore.ImportMapping{ID: id, Name: "acme-export", FieldMap: json.RawMessage(repoFieldMap(t))}, nil
		},
		UpdateImportMappingFn: func(_ context.Context, _ int64, p datastore.UpdateImportMappingParams) (datastore.ImportMapping, error) {
			updated = p
			return datastore.ImportMapping{ID: 3, Name: p.Name}, nil
		},
		DeleteImportMappingFn: func(_ context.Context, id int64) error {
			deleted = id
			return nil
		},
	}
	r := ownershipRouter(store)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/import/mappings/3", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("field_map")) {
		t.Error("the item response omits the field map")
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, jsonRequest(http.MethodPut, "/api/v1/ownership/import/mappings/3", mappingBody(t, "renamed")))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d: %s", w.Code, w.Body.String())
	}
	if updated.Name != "renamed" {
		t.Errorf("update params = %+v", updated)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/ownership/import/mappings/3", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d: %s", w.Code, w.Body.String())
	}
	if deleted != 3 {
		t.Errorf("deleted id = %d, want 3", deleted)
	}
}

func TestIntakeMappings_WritesRequireOperatorOrAdmin(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodPost, "/api/v1/ownership/import/mappings", mappingBody(t, "x")},
		{http.MethodPut, "/api/v1/ownership/import/mappings/1", mappingBody(t, "x")},
		{http.MethodDelete, "/api/v1/ownership/import/mappings/1", ""},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, withViewerSession(jsonRequest(tc.method, tc.path, tc.body)))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, want 403", tc.method, tc.path, w.Code)
		}
	}
}

func TestIntakeMappings_NonNumericIDIs404(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/import/mappings/not-a-number", nil))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Regression: the fixed-header import is untouched
// ---------------------------------------------------------------------------

// The fixed-header path is the fast lane for administrators who already have a
// file in CMM's format. The new routes sit beneath the same prefix, so this
// asserts the old one still dispatches to the old handler.
func TestFixedHeaderImport_StillWorksUnchanged(t *testing.T) {
	var written []datastore.InsertAssignmentParams
	store := &mockStore{
		GetOwnerByNameFn: func(_ context.Context, name string) (datastore.Owner, error) {
			return datastore.Owner{Name: name}, nil
		},
		InsertAssignmentFn: func(_ context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			written = append(written, p)
			return datastore.OwnershipAssignment{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()

	csv := "owner,entity_type,entity_key,organisation,notes\n" +
		"alice,git_repo,web-app,,from the old flow\n"
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import", csv, map[string]string{"format": "csv"}))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var body struct {
		Imported int `json:"imported"`
		Skipped  int `json:"skipped"`
		Errors   []struct {
			Line  int    `json:"line"`
			Error string `json:"error"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body.Imported != 1 {
		t.Errorf("imported = %d, want 1 (errors: %+v)", body.Imported, body.Errors)
	}
	if len(written) != 1 || written[0].EntityKey != "web-app" {
		t.Errorf("wrote %+v", written)
	}
	// The old response shape is part of the contract — the shipped UI reads it.
	if body.Errors == nil {
		t.Error("errors must serialise as an array, never null")
	}
}

// ---------------------------------------------------------------------------
// Importing one kind of row from a consolidated export
//
// The real source is a database holding several kinds of row together —
// declared owners alongside inferred committers and group members. Splitting
// that outside CMM means an afternoon in a spreadsheet per import, so the
// filter belongs here.
// ---------------------------------------------------------------------------

const mixedCategoryCSV = `cookbook,person,category
apache,alice.brown,owner
apache,bob.jones,committer
nginx,carol.white,owner
nginx,dave.taylor,group member
mysql,erin.walsh,committer
`

func intakeFieldsWithFilter(col, val string) map[string]string {
	return map[string]string{
		"entity_type":   "git_repo",
		"filter_column": col,
		"filter_value":  val,
		"field_map": `{
			"owner":      {"source":{"kind":"column","column":"person"}},
			"entity_key": {"source":{"kind":"column","column":"cookbook"}},
			"entity_type":{"source":{"kind":"constant","value":"git_repo"}}
		}`,
	}
}

func TestIntakePreview_FiltersToOneCategory(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview",
		mixedCategoryCSV, intakeFieldsWithFilter("category", "owner")))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Rows []struct {
			Owner string `json:"owner"`
		} `json:"rows"`
		RowCount    int `json:"row_count"`
		FilteredOut int `json:"filtered_out"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.RowCount != 2 {
		t.Errorf("row_count = %d, want the 2 owner rows", resp.RowCount)
	}
	// The skipped rows are reported, not silently dropped — otherwise a
	// mistyped filter value looks like an empty file.
	if resp.FilteredOut != 3 {
		t.Errorf("filtered_out = %d, want 3", resp.FilteredOut)
	}
}

// "Owner" and "owner" are the same label — these values are written by people.
func TestIntakePreview_FilterIgnoresCaseAndSpace(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview",
		mixedCategoryCSV, intakeFieldsWithFilter("category", "  OWNER ")))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		RowCount int `json:"row_count"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.RowCount != 2 {
		t.Errorf("row_count = %d, want 2", resp.RowCount)
	}
}

// A filter naming a column the source does not have would otherwise match
// nothing and report an empty import as though the file were empty.
func TestIntakePreview_FilterOnAnUnknownColumnIsRefused(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview",
		mixedCategoryCSV, intakeFieldsWithFilter("nonesuch", "owner")))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "nonesuch") {
		t.Errorf("the refusal does not name the column: %s", w.Body.String())
	}
}

// The dangerous one. The response truncates its per-row detail so a large
// import stays readable; the commit must still write every row. Truncating
// report.Rows itself would silently shorten the import.
func TestIntakeCommit_TruncatedDetailDoesNotShortenTheImport(t *testing.T) {
	var b strings.Builder
	b.WriteString("cookbook,person,category\n")
	const rows = intakeMaxReportRows + 250
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&b, "cookbook-%d,alice.brown,owner\n", i)
	}

	created := 0
	store := &mockStore{
		InsertAssignmentFn: func(_ context.Context, _ datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			created++
			return datastore.OwnershipAssignment{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/commit",
		b.String(), intakeFieldsWithFilter("category", "owner")))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Rows          []json.RawMessage `json:"rows"`
		RowCount      int               `json:"row_count"`
		RowsTruncated bool              `json:"rows_truncated"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.RowCount != rows {
		t.Errorf("row_count = %d, want %d — the whole file was processed", resp.RowCount, rows)
	}
	if len(resp.Rows) != intakeMaxReportRows {
		t.Errorf("returned %d row details, want them capped at %d", len(resp.Rows), intakeMaxReportRows)
	}
	// Saying so is the difference between a shortened list and a short import.
	if !resp.RowsTruncated {
		t.Error("the response does not say its row detail was truncated")
	}
	if created != rows {
		t.Errorf("committed %d assignments, want %d — truncating the display must not shorten the import", created, rows)
	}
}

// Rejected rows are the only ones anybody has to act on, and there are few of
// them. Taking a flat prefix of the report dropped the ones sitting late in
// the file, so the list header disagreed with the outcome tally — 41 against
// 156 — with nothing to say which was right.
func TestIntakePreview_TruncationKeepsEveryRejectedRow(t *testing.T) {
	var b strings.Builder
	b.WriteString("cookbook,person,category\n")
	const total = intakeMaxReportRows + 400
	const rejectedEvery = 30
	wantRejected := 0
	for i := 0; i < total; i++ {
		// A blank owner is rejected. Scattered throughout, and deliberately
		// past the truncation point as well as before it.
		if i%rejectedEvery == 0 {
			fmt.Fprintf(&b, "cookbook-%d,,owner\n", i)
			wantRejected++
			continue
		}
		fmt.Fprintf(&b, "cookbook-%d,alice.brown,owner\n", i)
	}

	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, intakeRequest(t, "/api/v1/ownership/import/preview",
		b.String(), intakeFieldsWithFilter("category", "owner")))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Rows []struct {
			Outcome string `json:"outcome"`
		} `json:"rows"`
		Counts        map[string]int `json:"counts"`
		RowsTruncated bool           `json:"rows_truncated"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !resp.RowsTruncated {
		t.Fatal("precondition: this source should have overflowed the detail cap")
	}

	inList := 0
	for _, row := range resp.Rows {
		if row.Outcome == "rejected" {
			inList++
		}
	}
	// The list and the tally have to agree, or a reader cannot tell which of
	// two numbers on one page is the truth.
	if inList != wantRejected {
		t.Errorf("the returned list holds %d rejected rows but %d were rejected", inList, wantRejected)
	}
	if resp.Counts["rejected"] != wantRejected {
		t.Errorf("counts.rejected = %d, want %d", resp.Counts["rejected"], wantRejected)
	}
	if len(resp.Rows) > intakeMaxReportRows {
		t.Errorf("returned %d rows, which exceeds the cap of %d", len(resp.Rows), intakeMaxReportRows)
	}
}
