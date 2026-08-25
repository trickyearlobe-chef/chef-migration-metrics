//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// The journey suite for journeys/ownership-identity.md — "One person, written
// down five different ways". Run it with `make journey`.
//
// One test per thing the journey says has to be in place. A green one is
// built; a red one is not yet, and that makes this the journey's todo list.
//
// Two things this suite deliberately does NOT do:
//
//   - It does not re-prove what a functional test already holds. Merging,
//     duplicate detection and the audit constraint all need a real database
//     and live under the `functional` tag, which the journey itself says. Where
//     that is the case the test here asserts the seam a person actually reaches
//     — the endpoint, the request, what comes back — and names the functional
//     test that holds the depth.
//   - It does not manufacture a red. Most of this journey is built, so most of
//     this suite is green. Green is the proof it was built; the reds and the
//     skips below are the honest remainder.

// identityRouter is journeyRouter with a store the test controls, so an
// assertion can be about what the handler asked the store to do rather than
// only about the status code.
func identityRouter(store *mockStore) *Router {
	return newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0"))
}

// ---------------------------------------------------------------------------
// What I need
// ---------------------------------------------------------------------------

// "To attach the other names a person is known by to the one record for them."
func TestJourney_OtherNamesAttachToTheOneRecord(t *testing.T) {
	var got datastore.InsertOwnerAliasParams
	store := &mockStore{
		InsertOwnerAliasFn: func(_ context.Context, p datastore.InsertOwnerAliasParams) (datastore.OwnerAlias, error) {
			got = p
			return datastore.OwnerAlias{OwnerName: p.OwnerName, AliasType: p.AliasType, AliasValue: p.AliasValue}, nil
		},
	}
	body := `{"owner_name":"alice.brown","alias_type":"git_name","alias_value":"Alice B"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/aliases", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	identityRouter(store).ServeHTTP(w, withAdminSession(req))

	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("attaching another name to a person answered %d: %s", w.Code, w.Body.String())
	}
	// The name the commit history holds has to land on the record the journey
	// names, not on a record of its own.
	if got.OwnerName != "alice.brown" || got.AliasValue != "Alice B" {
		t.Errorf("the name was not attached to the one record for the person: %+v", got)
	}
}

// "The names arrive from everywhere. An asset database has a mail address. The
// commit history has whatever somebody configured on their laptop. The
// sign-in system has a corporate username. The spreadsheet has a display name
// with a middle initial."
//
// Every one of those shapes has to be attachable, or the form that cannot be
// recorded is the form that goes on making strangers.
func TestJourney_EveryShapeAPersonArrivesInCanBeRecorded(t *testing.T) {
	accepted := map[string]bool{}
	store := &mockStore{
		InsertOwnerAliasFn: func(_ context.Context, p datastore.InsertOwnerAliasParams) (datastore.OwnerAlias, error) {
			accepted[p.AliasType] = true
			return datastore.OwnerAlias{OwnerName: p.OwnerName, AliasType: p.AliasType, AliasValue: p.AliasValue}, nil
		},
	}
	r := identityRouter(store)
	// The four the journey names, in the order it names them.
	for _, shape := range []string{"email", "git_name", "username", "custom"} {
		body := `{"owner_name":"alice.brown","alias_type":"` + shape + `","alias_value":"whatever"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/aliases", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, withAdminSession(req))
		if w.Code != http.StatusOK && w.Code != http.StatusCreated {
			t.Errorf("a person known by a %s cannot have it recorded (answered %d): %s",
				shape, w.Code, w.Body.String())
		}
	}
	for _, shape := range []string{"email", "git_name", "username", "custom"} {
		if !accepted[shape] {
			t.Errorf("a %s never reached the record", shape)
		}
	}
}

// "so that whichever form arrives next, it lands on the person and not on a
// new stranger."
//
// The baseline is asserted first: with nothing recorded, the same import
// invents a person. Without that, a store that resolved everything to
// alice.brown would make this pass while proving nothing.
func TestJourney_AnArrivingNameLandsOnThePersonNotAStranger(t *testing.T) {
	const rows = "Owner,Repo\nalice.b@example.com,web-app\n"

	preview := func(t *testing.T, store *mockStore) map[string]any {
		t.Helper()
		req := intakeRequest(t, "/api/v1/ownership/import/preview", rows, map[string]string{
			"field_map": repoFieldMap(t),
		})
		w := httptest.NewRecorder()
		identityRouter(store).ServeHTTP(w, withAdminSession(req))
		if w.Code != http.StatusOK {
			t.Fatalf("previewing an import answered %d: %s", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decoding the preview: %v", err)
		}
		return body
	}

	// Baseline: nobody is known by that address, so a stranger is created.
	stranger := preview(t, &mockStore{})
	if newOwners, _ := stranger["new_owners"].([]any); len(newOwners) != 1 {
		t.Fatalf("the fixture proves nothing: an unrecognised address did not create a "+
			"new owner (new_owners = %v)", stranger["new_owners"])
	}

	// The same address, now recorded as one of alice.brown's names.
	known := preview(t, &mockStore{
		ResolveOwnerByAliasFn: func(_ context.Context, aliasType, value string) (string, error) {
			if value == "alice.b@example.com" {
				return "alice.brown", nil
			}
			return "", datastore.ErrNotFound
		},
	})
	if newOwners, _ := known["new_owners"].([]any); len(newOwners) != 0 {
		t.Errorf("an address already recorded against a person still created a new "+
			"stranger: %v", known["new_owners"])
	}
}

// "A standing list of people who look like they might already be somebody
// else, so I can work through it."
//
// Standing, not computed on the way past: the list is read from a stored scan,
// so it survives being navigated away from.
func TestJourney_AStandingListOfPeopleWhoMayAlreadyBeSomebodyElse(t *testing.T) {
	listed := false
	store := &mockStore{
		ListOwnerDuplicateCandidatesFn: func(_ context.Context, _ datastore.OwnerDuplicateFilter) ([]datastore.OwnerDuplicateCandidate, int, error) {
			listed = true
			return []datastore.OwnerDuplicateCandidate{
				{OwnerA: "alice.brown", OwnerB: "a.brown", MatchedOn: "name", Similarity: 0.8},
			}, 1, nil
		},
	}
	w := httptest.NewRecorder()
	identityRouter(store).ServeHTTP(w, withAdminSession(
		httptest.NewRequest(http.MethodGet, "/api/v1/ownership/duplicates", nil)))

	if w.Code != http.StatusOK {
		t.Fatalf("there is no list of people who may already be somebody else (answered %d): %s",
			w.Code, w.Body.String())
	}
	if !listed {
		t.Error("the list was answered without reading the stored scan, so it is computed " +
			"on the way past rather than standing")
	}
}

// "Looking for duplicates has to compare the record names, not only the
// alternative names. An owner created from commit history has no alternative
// names at all."
//
// The comparison itself needs a real database and is held by
// internal/datastore/owner_duplicates_functional_test.go
// #TestFunctional_OwnerDuplicates_MatchesOnAliasValueToo and its name-matching
// neighbour. What is asserted here is the half those cannot reach: that the
// answer says which side matched, so a reader can tell a name match from an
// alias match rather than having to trust that both were tried.
func TestJourney_DuplicateSearchComparesRecordNamesNotOnlyAlternativeNames(t *testing.T) {
	store := &mockStore{
		ListOwnerDuplicateCandidatesFn: func(_ context.Context, _ datastore.OwnerDuplicateFilter) ([]datastore.OwnerDuplicateCandidate, int, error) {
			return []datastore.OwnerDuplicateCandidate{
				{OwnerA: "a.brown", OwnerB: "alice.brown", MatchedOn: "name", Similarity: 0.8},
			}, 1, nil
		},
	}
	w := httptest.NewRecorder()
	identityRouter(store).ServeHTTP(w, withAdminSession(
		httptest.NewRequest(http.MethodGet, "/api/v1/ownership/duplicates", nil)))

	var body struct {
		Data []struct {
			MatchedOn string `json:"matched_on"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the candidate list: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].MatchedOn != "name" {
		t.Errorf("a candidate does not say that it was the record names that matched "+
			"(got %+v), so an owner created from commit history cannot be told from "+
			"one matched on a recorded identity", body.Data)
	}
}

// "To merge two records when they are the same person."
func TestJourney_CanMergeTwoRecordsIntoOne(t *testing.T) {
	var from, into string
	store := &mockStore{
		MergeOwnersFn: func(_ context.Context, fromOwner, intoOwner string) (datastore.MergeOwnersResult, error) {
			from, into = fromOwner, intoOwner
			return datastore.MergeOwnersResult{FromOwner: fromOwner, IntoOwner: intoOwner}, nil
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/merge",
		strings.NewReader(`{"from_owner":"a.brown","into_owner":"alice.brown"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	identityRouter(store).ServeHTTP(w, withAdminSession(req))

	if w.Code != http.StatusOK {
		t.Fatalf("two records for one person cannot be merged (answered %d): %s",
			w.Code, w.Body.String())
	}
	if from != "a.brown" || into != "alice.brown" {
		t.Errorf("the merge went the wrong way round: %q into %q", from, into)
	}
}

// "and have all of the work follow — every assignment, every alternative name,
// and the name I merged away."
//
// The move itself needs a real database:
// internal/datastore/owner_merge_functional_test.go
// #TestFunctional_MergeOwners_MovesWorkAliasesAndTheSourceName. What is
// asserted here is that all three parts are reported back, because a merge that
// moved two of them and said nothing is indistinguishable from one that moved
// all three.
func TestJourney_AMergeSaysWhatFollowed(t *testing.T) {
	store := &mockStore{
		MergeOwnersFn: func(_ context.Context, fromOwner, intoOwner string) (datastore.MergeOwnersResult, error) {
			return datastore.MergeOwnersResult{
				FromOwner: fromOwner, IntoOwner: intoOwner,
				Reassigned: 3, AliasesMoved: 2, SourceNameAliased: true,
			}, nil
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/merge",
		strings.NewReader(`{"from_owner":"a.brown","into_owner":"alice.brown"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	identityRouter(store).ServeHTTP(w, withAdminSession(req))

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the merge result: %v", err)
	}
	for _, part := range []string{"reassigned", "aliases_moved", "source_name_aliased"} {
		if _, ok := got[part]; !ok {
			t.Errorf("a merge does not report %q, so what followed the person cannot be "+
				"checked without reading the database", part)
		}
	}
}

// "Merging keeps the name it absorbed. Dropping it would leave the next import
// free to create it again, and the duplicate would reappear on a schedule."
func TestJourney_MergingKeepsTheNameItAbsorbed(t *testing.T) {
	store := &mockStore{
		MergeOwnersFn: func(_ context.Context, fromOwner, intoOwner string) (datastore.MergeOwnersResult, error) {
			return datastore.MergeOwnersResult{
				FromOwner: fromOwner, IntoOwner: intoOwner, SourceNameAliased: true,
			}, nil
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/merge",
		strings.NewReader(`{"from_owner":"a.brown","into_owner":"alice.brown"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	identityRouter(store).ServeHTTP(w, withAdminSession(req))

	var got struct {
		SourceNameAliased bool `json:"source_name_aliased"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the merge result: %v", err)
	}
	if !got.SourceNameAliased {
		t.Error("a merge does not say the absorbed name was kept, so nothing stops the " +
			"next import recreating the duplicate")
	}
}

// "To see who changed the ownership record and when."
func TestJourney_CanSeeWhoChangedTheRecordAndWhen(t *testing.T) {
	store := &mockStore{
		ListAuditLogFn: func(_ context.Context, _ datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error) {
			return []datastore.OwnershipAuditEntry{{Action: "owner_merged", Actor: "admin"}}, 1, nil
		},
	}
	w := httptest.NewRecorder()
	identityRouter(store).ServeHTTP(w, withAdminSession(
		httptest.NewRequest(http.MethodGet, "/api/v1/ownership/audit-log", nil)))

	if w.Code != http.StatusOK {
		t.Fatalf("there is no way to see who changed the ownership record (answered %d): %s",
			w.Code, w.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the audit log: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("the audit log returned %d entries, want 1", len(body.Data))
	}
	for _, field := range []string{"actor", "timestamp"} {
		if _, ok := body.Data[0][field]; !ok {
			t.Errorf("an audit entry carries no %q, so it cannot answer who changed this and when", field)
		}
	}
}

// "This decides who is accountable for thousands of things, so a change to it
// without a name against it is not acceptable."
//
// The name has to be the person who made the change, not a constant the writer
// chose — so the assertion is that the session's own name reaches the entry.
func TestJourney_AChangeWithoutANameAgainstItIsNotAcceptable(t *testing.T) {
	var entries []datastore.InsertAuditEntryParams
	store := &mockStore{
		MergeOwnersFn: func(_ context.Context, fromOwner, intoOwner string) (datastore.MergeOwnersResult, error) {
			return datastore.MergeOwnersResult{FromOwner: fromOwner, IntoOwner: intoOwner}, nil
		},
		InsertAuditEntryFn: func(_ context.Context, p datastore.InsertAuditEntryParams) error {
			entries = append(entries, p)
			return nil
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/merge",
		strings.NewReader(`{"from_owner":"a.brown","into_owner":"alice.brown"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	// withAdminSession signs the request as "admin".
	identityRouter(store).ServeHTTP(w, withAdminSession(req))

	if len(entries) == 0 {
		t.Fatal("merging two people wrote no audit entry at all")
	}
	for _, e := range entries {
		if e.Actor != "admin" {
			t.Errorf("an ownership change was recorded against %q rather than the person "+
				"who made it", e.Actor)
		}
	}
}

// "An audit entry either identifies a thing completely or does not mention one.
// A record saying something was assigned, without saying what, is worse than
// one that says only that an assignment happened."
//
// The database refuses half a reference, which
// internal/datastore/owner_merge_functional_test.go
// #TestFunctional_AuditLogRefusesHalfAnEntityReference holds. What is asserted
// here is the other side of the same rule: an owner-level action names no thing
// at all rather than naming a kind without which one.
func TestJourney_AnAuditEntryNamesAThingCompletelyOrNotAtAll(t *testing.T) {
	var entries []datastore.InsertAuditEntryParams
	store := &mockStore{
		MergeOwnersFn: func(_ context.Context, fromOwner, intoOwner string) (datastore.MergeOwnersResult, error) {
			return datastore.MergeOwnersResult{FromOwner: fromOwner, IntoOwner: intoOwner}, nil
		},
		InsertAuditEntryFn: func(_ context.Context, p datastore.InsertAuditEntryParams) error {
			entries = append(entries, p)
			return nil
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/merge",
		strings.NewReader(`{"from_owner":"a.brown","into_owner":"alice.brown"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	identityRouter(store).ServeHTTP(w, withAdminSession(req))

	if len(entries) == 0 {
		t.Fatal("merging two people wrote no audit entry at all")
	}
	for _, e := range entries {
		if (e.EntityType == "") != (e.EntityKey == "") {
			t.Errorf("an audit entry names a %q without saying which one (key %q), which "+
				"reads as a full account of a change while being unreconstructable",
				e.EntityType, e.EntityKey)
		}
	}
}

// ---------------------------------------------------------------------------
// The anchor
// ---------------------------------------------------------------------------

// "A person's sign-in name is the anchor. It is the one identifier the
// organisation controls and does not recycle. Everything else — mail addresses,
// commit names, display names — hangs off it as an alternative."
//
// The shape is first-class: when a name arrives, the sign-in name is one of the
// identities it is tried against, alongside the mail addresses, commit names
// and display names that hang off it.
func TestJourney_ASignInNameIsOneOfTheIdentitiesAnArrivingNameIsTriedAgainst(t *testing.T) {
	tried := map[string]bool{}
	store := &mockStore{
		ResolveOwnerByAliasFn: func(_ context.Context, aliasType, _ string) (string, error) {
			tried[aliasType] = true
			return "", datastore.ErrNotFound
		},
	}
	req := intakeRequest(t, "/api/v1/ownership/import/preview", "Owner,Repo\nabrown,web-app\n",
		map[string]string{"field_map": repoFieldMap(t)})
	w := httptest.NewRecorder()
	identityRouter(store).ServeHTTP(w, withAdminSession(req))
	if w.Code != http.StatusOK {
		t.Fatalf("previewing an import answered %d: %s", w.Code, w.Body.String())
	}

	// The mail address, the commit name, the commit address and the sign-in
	// name — the forms the journey says one person arrives in.
	for _, shape := range []string{"email", "git_name", "git_email", "username"} {
		if !tried[shape] {
			t.Errorf("an arriving name is never tried as a %s, so a person known by one "+
				"becomes a stranger", shape)
		}
	}
}

// "Everything else ... hangs off it as an alternative. Anchoring on anything
// else means the record drifts as people change teams or the directory is
// reconfigured."
//
// The anchor is available and unused: nothing puts a person's sign-in name on
// their record. The only writers of an alias are the import and the alias
// endpoints, both of which take whatever the administrator or the source
// supplies, and a record created from commit history is named after an email
// localpart instead.
func TestJourney_SigningInAttachesTheSignInNameToTheRecord(t *testing.T) {
	t.Skip("TODO: nothing links a session to an owner, so signing in attaches nothing " +
		"and the anchor is a decision rather than a property. Proving it needs the login " +
		"path driven against a store, which this suite cannot do from the router alone. " +
		"The work is plans/todo-ownership.md § Matching app users to owners; the " +
		"user-visible half is § \"My stuff\".")
}

// ---------------------------------------------------------------------------
// What the journey says nothing can prove
// ---------------------------------------------------------------------------

// "Nothing proves two records are the same person. The list is candidates,
// ranked by looking similar. Confirming is a human act and always will be."
func TestJourney_NothingProvesTwoRecordsAreTheSamePerson(t *testing.T) {
	t.Skip("Deliberate, and recorded so it is not mistaken for a gap: two engineers " +
		"really can have the same name, so the list is candidates and confirming is a " +
		"human act. Nothing here should ever assert that a pair IS one person.")
}

// "Nothing proves a merge can be undone. It moves work and absorbs a name;
// there is no test for reversing it. Treat a merge as one-way."
func TestJourney_NothingProvesAMergeCanBeUndone(t *testing.T) {
	t.Skip("Deliberate: the absorbed name is kept as an alternative rather than " +
		"restored, so a merge is one-way by construction. Recorded here so nobody " +
		"builds an undo believing the data supports one.")
}

// "The load-bearing assumption: that a person's sign-in name is stable and not
// reissued. If the directory ever recycles one, work silently transfers to a
// different human being, and nothing here would detect it."
func TestJourney_SignInNamesAreNeverReissued(t *testing.T) {
	t.Skip("Not answerable from this product at all — it is a property of the directory " +
		"itself. Verify it in the organisation before relying on the anchor; if it is " +
		"false, every list here is confidently wrong and nothing in the code would say so.")
}
