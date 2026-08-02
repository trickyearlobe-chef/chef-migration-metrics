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
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/merge
// ---------------------------------------------------------------------------

func mergeRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/merge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestMergeOwners_ReportsWhatMoved(t *testing.T) {
	var got [2]string
	var audited []string
	store := &mockStore{
		MergeOwnersFn: func(_ context.Context, from, into string) (datastore.MergeOwnersResult, error) {
			got = [2]string{from, into}
			return datastore.MergeOwnersResult{
				FromOwner: from, IntoOwner: into,
				Reassigned: 3, Skipped: 1, AliasesMoved: 2, SourceNameAliased: true,
			}, nil
		},
		InsertAuditEntryFn: func(_ context.Context, p datastore.InsertAuditEntryParams) error {
			audited = append(audited, p.Action)
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, mergeRequest(`{"from_owner":"fat-tommy","into_owner":"thomas-smith"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if got != [2]string{"fat-tommy", "thomas-smith"} {
		t.Errorf("merged %v, want fat-tommy into thomas-smith", got)
	}

	var resp datastore.MergeOwnersResult
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Reassigned != 3 || resp.Skipped != 1 || resp.AliasesMoved != 2 {
		t.Errorf("response = %+v, want the store's counts reported verbatim", resp)
	}
	if !resp.SourceNameAliased {
		t.Error("source_name_aliased was dropped from the response")
	}

	// A merge deletes a person. It has to be answerable for afterwards.
	if len(audited) != 1 || audited[0] != "owner_merged" {
		t.Errorf("audit actions = %v, want one owner_merged entry", audited)
	}
}

func TestMergeOwners_RejectsMissingAndSelfMerge(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"no source", `{"into_owner":"thomas-smith"}`},
		{"no target", `{"from_owner":"fat-tommy"}`},
		{"itself", `{"from_owner":"thomas-smith","into_owner":"thomas-smith"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			store := &mockStore{
				MergeOwnersFn: func(_ context.Context, _, _ string) (datastore.MergeOwnersResult, error) {
					called = true
					return datastore.MergeOwnersResult{}, nil
				},
			}
			r := ownershipRouter(store)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, mergeRequest(tc.body))

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400: %s", w.Code, w.Body.String())
			}
			if called {
				t.Error("the store was asked to merge anyway")
			}
		})
	}
}

func TestMergeOwners_UnknownOwnerIsNotFound(t *testing.T) {
	store := &mockStore{
		MergeOwnersFn: func(_ context.Context, _, _ string) (datastore.MergeOwnersResult, error) {
			return datastore.MergeOwnersResult{}, datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, mergeRequest(`{"from_owner":"nobody","into_owner":"thomas-smith"}`))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestMergeOwners_MethodNotAllowed(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/merge", nil))

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/ownership/duplicates
// ---------------------------------------------------------------------------

type duplicatesResponse struct {
	Data []datastore.OwnerDuplicateCandidate `json:"data"`
	// Pagination is decoded loosely: the test cares that it is present and
	// carries the total, not about the shape the helper produces.
	Pagination struct {
		TotalItems int `json:"total_items"`
	} `json:"pagination"`
	Coverage struct {
		OwnersTotal        int `json:"owners_total"`
		OwnersWithoutAlias int `json:"owners_without_alias"`
	} `json:"coverage"`
	Scan *datastore.OwnerDuplicateScan `json:"scan"`
}

// The scan is tens of seconds on a large catalogue, so the request that asks
// for it must not wait for it — a proxy timeout would turn one scan into a
// retry loop of them.
func TestDuplicateOwners_RescanRunsDetachedFromTheRequest(t *testing.T) {
	release := make(chan struct{})
	done := make(chan struct{})
	store := &mockStore{
		RecomputeOwnerDuplicateCandidatesFn: func(ctx context.Context) (int, error) {
			<-release
			// The request's context is long gone by now; the scan must not
			// have been cancelled with it.
			if err := ctx.Err(); err != nil {
				t.Errorf("the scan context was cancelled: %v", err)
			}
			close(done)
			return 17, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/ownership/duplicates/rescan", nil))

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Started bool `json:"started"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Started {
		t.Error("started = false, want the scan to have been started")
	}

	// A second request while one is running must not start another.
	second := httptest.NewRecorder()
	r.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/v1/ownership/duplicates/rescan", nil))
	var secondResp struct {
		Started bool `json:"started"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if secondResp.Started {
		t.Error("a second scan was started while the first was still running")
	}

	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the scan never ran")
	}
}

// While a scan is running the list is the previous one, and the page has to
// be able to say so rather than presenting stale counts as current.
func TestDuplicateOwners_ReportsThatAScanIsRunning(t *testing.T) {
	release := make(chan struct{})
	store := &mockStore{
		RecomputeOwnerDuplicateCandidatesFn: func(_ context.Context) (int, error) {
			<-release
			return 0, nil
		},
		ListOwnerDuplicateCandidatesFn: func(_ context.Context, _ datastore.OwnerDuplicateFilter) ([]datastore.OwnerDuplicateCandidate, int, error) {
			return nil, 0, nil
		},
		CountOwnersMissingAliasesFn: func(_ context.Context) (int, int, error) { return 0, 0, nil },
	}
	r := ownershipRouter(store)
	defer close(release)

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/ownership/duplicates/rescan", nil))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/duplicates", nil))

	var resp struct {
		ScanRunning bool `json:"scan_running"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.ScanRunning {
		t.Error("scan_running = false while a scan is running")
	}
}

func TestDuplicateOwners_RescanIsPostOnly(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/duplicates/rescan", nil))

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// "Nothing looks alike" and "nobody has looked yet" are opposite messages and
// must not both render as an empty list.
func TestDuplicateOwners_SaysWhetherTheCatalogueHasEverBeenScanned(t *testing.T) {
	store := &mockStore{
		ListOwnerDuplicateCandidatesFn: func(_ context.Context, _ datastore.OwnerDuplicateFilter) ([]datastore.OwnerDuplicateCandidate, int, error) {
			return nil, 0, nil
		},
		CountOwnersMissingAliasesFn: func(_ context.Context) (int, int, error) { return 0, 0, nil },
		// The default mock returns ErrNotFound: never scanned.
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/duplicates", nil))

	var never duplicatesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &never); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if never.Scan != nil {
		t.Errorf("scan = %+v, want absent when the catalogue has never been scanned", never.Scan)
	}

	store.GetOwnerDuplicateScanFn = func(_ context.Context) (datastore.OwnerDuplicateScan, error) {
		return datastore.OwnerDuplicateScan{ScannedAt: time.Unix(1770000000, 0).UTC(), PairsFound: 0}, nil
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/duplicates", nil))

	var scanned duplicatesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &scanned); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if scanned.Scan == nil || scanned.Scan.ScannedAt.IsZero() {
		t.Errorf("scan = %+v, want the time of the scan that found nothing", scanned.Scan)
	}
}

func TestDuplicateOwners_ListsCandidatesWithCoverage(t *testing.T) {
	store := &mockStore{
		ListOwnerDuplicateCandidatesFn: func(_ context.Context, f datastore.OwnerDuplicateFilter) ([]datastore.OwnerDuplicateCandidate, int, error) {
			return []datastore.OwnerDuplicateCandidate{{
				OwnerA: "thomas-smith", OwnerB: "tomas-smith",
				MatchedOn: "name", ValueA: "thomas-smith", ValueB: "tomas-smith",
				Similarity: 0.82, AssignmentsA: 12, AssignmentsB: 1,
			}}, 1, nil
		},
		CountOwnersMissingAliasesFn: func(_ context.Context) (int, int, error) {
			return 40, 7, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/duplicates", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	var resp duplicatesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("got %d candidates, want 1", len(resp.Data))
	}
	if resp.Data[0].OwnerA != "thomas-smith" || resp.Data[0].AssignmentsA != 12 {
		t.Errorf("candidate = %+v", resp.Data[0])
	}
	if resp.Pagination.TotalItems != 1 {
		t.Errorf("total_items = %d, want 1", resp.Pagination.TotalItems)
	}

	// Without this the reader cannot tell how much of the catalogue the
	// report covers, and an empty list reads as "no duplicates".
	if resp.Coverage.OwnersTotal != 40 || resp.Coverage.OwnersWithoutAlias != 7 {
		t.Errorf("coverage = %+v, want 40 owners and 7 without an alias", resp.Coverage)
	}
}

func TestDuplicateOwners_EmptyListIsAnEmptyArray(t *testing.T) {
	store := &mockStore{
		ListOwnerDuplicateCandidatesFn: func(_ context.Context, _ datastore.OwnerDuplicateFilter) ([]datastore.OwnerDuplicateCandidate, int, error) {
			return nil, 0, nil
		},
		CountOwnersMissingAliasesFn: func(_ context.Context) (int, int, error) { return 0, 0, nil },
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/duplicates", nil))

	if !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Errorf("body = %s, want an empty array rather than null", w.Body.String())
	}
}

func TestDuplicateOwners_PassesTheSimilarityFloorThrough(t *testing.T) {
	var got datastore.OwnerDuplicateFilter
	store := &mockStore{
		ListOwnerDuplicateCandidatesFn: func(_ context.Context, f datastore.OwnerDuplicateFilter) ([]datastore.OwnerDuplicateCandidate, int, error) {
			got = f
			return nil, 0, nil
		},
		CountOwnersMissingAliasesFn: func(_ context.Context) (int, int, error) { return 0, 0, nil },
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/duplicates?min_similarity=0.7&per_page=10&page=2", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if got.MinSimilarity != 0.7 {
		t.Errorf("MinSimilarity = %v, want 0.7", got.MinSimilarity)
	}
	if got.Limit != 10 || got.Offset != 10 {
		t.Errorf("Limit/Offset = %d/%d, want 10/10", got.Limit, got.Offset)
	}
}

// ---------------------------------------------------------------------------
// Owners created from git committers
// ---------------------------------------------------------------------------

// The committer path is the main way owners get created, and it wrote no alias
// row at all — so the person it invented could not be found by any identity
// they are actually known by, and could not be paired with a duplicate.
func TestCookbookCommittersAssign_SeedsTheCommitAddressAsAnAlias(t *testing.T) {
	var seeded []datastore.InsertOwnerAliasParams
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(_ context.Context, _ string) (string, error) {
			return "https://git.example.com/cookbooks/nginx.git", nil
		},
		GetOwnerByNameFn: func(_ context.Context, _ string) (datastore.Owner, error) {
			return datastore.Owner{}, datastore.ErrNotFound
		},
		InsertOwnerFn: func(_ context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error) {
			return datastore.Owner{Name: p.Name, OwnerType: "individual"}, nil
		},
		InsertOwnerAliasFn: func(_ context.Context, p datastore.InsertOwnerAliasParams) (datastore.OwnerAlias, error) {
			seeded = append(seeded, p)
			return datastore.OwnerAlias{}, nil
		},
		InsertAssignmentFn: func(_ context.Context, _ datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			return datastore.OwnershipAssignment{ID: 1}, nil
		},
		InsertAuditEntryFn: func(_ context.Context, _ datastore.InsertAuditEntryParams) error { return nil },
		ListServerCookbooksByNameFn: func(_ context.Context, _ string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"committers":[{"author_email":"jsmith@example.com","owner_name":"jsmith","display_name":"Jane Smith"}]}`
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/nginx/committers/assign", strings.NewReader(body)))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if len(seeded) != 1 {
		t.Fatalf("seeded %d aliases, want 1", len(seeded))
	}
	// A commit address is a git address. Recording it as the corporate one
	// would attach work to the wrong person the moment the two differ.
	if seeded[0].AliasType != "git_email" {
		t.Errorf("AliasType = %q, want %q", seeded[0].AliasType, "git_email")
	}
	if seeded[0].AliasValue != "jsmith@example.com" {
		t.Errorf("AliasValue = %q, want the commit address", seeded[0].AliasValue)
	}
	if seeded[0].OwnerName != "jsmith" {
		t.Errorf("OwnerName = %q, want the owner just created", seeded[0].OwnerName)
	}
	if seeded[0].Source == "" || seeded[0].Source == "manual" {
		t.Errorf("Source = %q, want a source that says where it came from", seeded[0].Source)
	}
}

// The alias is a lead for later matching, not part of the assignment. Failing
// to seed it must not fail the assignment the operator asked for.
func TestCookbookCommittersAssign_AliasSeedFailureDoesNotFailTheAssignment(t *testing.T) {
	store := &mockStore{
		GetGitRepoURLForCookbookFn: func(_ context.Context, _ string) (string, error) {
			return "https://git.example.com/cookbooks/nginx.git", nil
		},
		GetOwnerByNameFn: func(_ context.Context, _ string) (datastore.Owner, error) {
			return datastore.Owner{}, datastore.ErrNotFound
		},
		InsertOwnerFn: func(_ context.Context, p datastore.InsertOwnerParams) (datastore.Owner, error) {
			return datastore.Owner{Name: p.Name, OwnerType: "individual"}, nil
		},
		InsertOwnerAliasFn: func(_ context.Context, _ datastore.InsertOwnerAliasParams) (datastore.OwnerAlias, error) {
			// Somebody else already answers to this address.
			return datastore.OwnerAlias{}, datastore.ErrAlreadyExists
		},
		InsertAssignmentFn: func(_ context.Context, _ datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			return datastore.OwnershipAssignment{ID: 1}, nil
		},
		InsertAuditEntryFn: func(_ context.Context, _ datastore.InsertAuditEntryParams) error { return nil },
		ListServerCookbooksByNameFn: func(_ context.Context, _ string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	body := `{"committers":[{"author_email":"jsmith@example.com","owner_name":"jsmith"}]}`
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/nginx/committers/assign", strings.NewReader(body)))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp struct {
		AssignmentsCreated int `json:"assignments_created"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.AssignmentsCreated != 1 {
		t.Errorf("assignments_created = %d, want 1", resp.AssignmentsCreated)
	}
}

// A failing coverage count must not blank the candidates: the list is the
// point, the coverage is the caveat on it.
func TestDuplicateOwners_ListSurvivesACoverageCountFailure(t *testing.T) {
	store := &mockStore{
		ListOwnerDuplicateCandidatesFn: func(_ context.Context, _ datastore.OwnerDuplicateFilter) ([]datastore.OwnerDuplicateCandidate, int, error) {
			return []datastore.OwnerDuplicateCandidate{{OwnerA: "a-person", OwnerB: "a-persona"}}, 1, nil
		},
		CountOwnersMissingAliasesFn: func(_ context.Context) (int, int, error) {
			return 0, 0, context.DeadlineExceeded
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ownership/duplicates", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp duplicatesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Errorf("got %d candidates, want the list regardless of the coverage count", len(resp.Data))
	}
}
