// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// The failure register
//
//	GET   /api/v1/failure-register              — the standup view (journey 6)
//	POST  /api/v1/failure-register              — record a verdict (journey 4)
//	GET   /api/v1/failure-register/{id}
//	PATCH /api/v1/failure-register/{id}         — the diagnosis, plan and holder
//	POST  /api/v1/failure-register/{id}/resolve
//	GET   /api/v1/failure-register/subject/{name} — every verdict about one subject
//
// A person's verdict on whether a cookbook actually works on the target
// version, recorded with a reason. It exists because the automated signals are
// wrong in both directions. Behaviour: journeys/human-verdict.md.
// ---------------------------------------------------------------------------

const failureRegisterPrefix = "/api/v1/failure-register"

// defaultRegisterWindowDays is the period the direction of travel is measured
// over. A week is what a standup compares against.
const defaultRegisterWindowDays = 7

func (r *Router) handleFailureRegister(w http.ResponseWriter, req *http.Request) {
	rest := strings.TrimPrefix(req.URL.Path, failureRegisterPrefix)
	rest = strings.Trim(rest, "/")

	switch {
	case rest == "":
		switch req.Method {
		case http.MethodGet:
			r.handleListFailureRegister(w, req)
		case http.MethodPost:
			r.handleRecordFailureVerdict(w, req)
		default:
			WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Allowed: GET, POST")
		}
		return

	case strings.HasPrefix(rest, "subject/"):
		// The rest of the path is the subject name, taken whole: some hosting
		// platforms allow a slash in a repo name, and truncating at the first
		// separator would silently read the history of a different subject.
		r.handleFailureRegisterHistory(w, req, strings.TrimPrefix(rest, "subject/"))
		return

	case strings.HasSuffix(rest, "/resolve"):
		if req.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Allowed: POST")
			return
		}
		r.handleResolveFailureEntry(w, req, strings.TrimSuffix(rest, "/resolve"))
		return

	default:
		switch req.Method {
		case http.MethodGet:
			r.handleGetFailureEntry(w, req, rest)
		case http.MethodPatch:
			r.handleReviseFailureEntry(w, req, rest)
		default:
			// Deliberately no DELETE: resolution is recorded, never deleted,
			// because the standup view needs the direction of travel.
			WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Allowed: GET, PATCH")
		}
	}
}

// ---------------------------------------------------------------------------
// Journey 6 — the standup view
// ---------------------------------------------------------------------------

func (r *Router) handleListFailureRegister(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()

	// The default read is what is standing. The whole history is available by
	// asking for it, but a standup opens on what is broken now.
	status := q.Get("status")
	if status == "" {
		status = datastore.FailureStatusOpen
	}
	if status == "all" {
		status = ""
	}

	pg := ParsePagination(req)
	entries, total, err := r.db.ListFailureRegisterEntries(req.Context(), datastore.FailureRegisterFilter{
		Status:      status,
		Verdict:     q.Get("verdict"),
		SubjectName: q.Get("subject_name"),
		Limit:       pg.Limit(),
		Offset:      pg.Offset(),
	})
	if err != nil {
		r.logf("ERROR", "failure-register: listing entries: %v", err)
		WriteInternalError(w, "Failed to read the failure register.")
		return
	}
	if entries == nil {
		// An empty array, so the view can tell "nothing is broken" from
		// "this failed to load".
		entries = []datastore.FailureRegisterEntry{}
	}

	body := map[string]any{
		"data":       entries,
		"pagination": NewPaginationResponse(pg, total),
	}

	// Whether the list is getting too large. The size and the direction matter
	// as much as the contents — but a standup that can see what is broken and
	// not the trend is still worth having, so this failing does not take the
	// list with it.
	windowDays := defaultRegisterWindowDays
	if v := q.Get("window_days"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			windowDays = parsed
		}
	}
	summary, err := r.db.FailureRegisterSummary(req.Context(), windowDays)
	if err != nil {
		r.logf("WARN", "failure-register: summarising: %v", err)
	} else {
		body["summary"] = summary
	}

	WriteJSON(w, http.StatusOK, body)
}

func (r *Router) handleFailureRegisterHistory(w http.ResponseWriter, req *http.Request, subjectName string) {
	if !requireGET(w, req) {
		return
	}
	if subjectName == "" {
		WriteBadRequest(w, "A subject name is required.")
		return
	}

	entries, err := r.db.ListFailureRegisterHistory(req.Context(), subjectName)
	if err != nil {
		r.logf("ERROR", "failure-register: reading the history for %s: %v", subjectName, err)
		WriteInternalError(w, "Failed to read the register history for this subject.")
		return
	}
	if entries == nil {
		entries = []datastore.FailureRegisterEntry{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": entries})
}

func (r *Router) handleGetFailureEntry(w http.ResponseWriter, req *http.Request, id string) {
	entry, err := r.db.GetFailureRegisterEntry(req.Context(), id)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, "No such register entry.")
		return
	}
	if err != nil {
		r.logf("ERROR", "failure-register: reading entry %s: %v", id, err)
		WriteInternalError(w, "Failed to read the register entry.")
		return
	}
	WriteJSON(w, http.StatusOK, entry)
}

// ---------------------------------------------------------------------------
// Journey 4 — record a failure nobody predicted
// ---------------------------------------------------------------------------

func (r *Router) handleRecordFailureVerdict(w http.ResponseWriter, req *http.Request) {
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	var body struct {
		SubjectName  string `json:"subject_name"`
		SubjectType  string `json:"subject_type"`
		CookbookName string `json:"cookbook_name"`
		Verdict      string `json:"verdict"`
		Reason       string `json:"reason"`
		Evidence     string `json:"evidence"`
		Diagnosis    string `json:"diagnosis"`
		Plan         string `json:"plan"`
		TargetDate   string `json:"target_date"`
		HolderType   string `json:"holder_type"`
		HolderRef    string `json:"holder_ref"`

		// Accepted only so it can be refused. A verdict is about the thing,
		// not one of its versions: several are in use at once and the failure
		// is discussed version-agnostically. Silently ignoring it would let a
		// caller believe it had recorded something narrower than it had.
		CookbookVersion string `json:"cookbook_version"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	if body.CookbookVersion != "" {
		WriteBadRequest(w, "A verdict is about a repo or a cookbook, not one of its versions. "+
			"Several versions are usually in use at once, so entries are never keyed on one.")
		return
	}
	if strings.TrimSpace(body.SubjectName) == "" {
		WriteBadRequest(w, "subject_name is required — the repo where the fix is made, or the cookbook itself where no repo has been collected.")
		return
	}
	if strings.TrimSpace(body.CookbookName) == "" {
		WriteBadRequest(w, "cookbook_name is required — it is what the failure is called at standup.")
		return
	}
	switch body.Verdict {
	case datastore.VerdictBroken, datastore.VerdictNotBroken:
	default:
		WriteBadRequest(w, `verdict must be "broken" or "not_broken".`)
		return
	}
	if strings.TrimSpace(body.Reason) == "" {
		WriteBadRequest(w, "A reason is required. A verdict with no reason is an opinion, "+
			"and the reason is what lets a later reader judge whether it still holds.")
		return
	}

	targetDate, err := parseOptionalDate(body.TargetDate)
	if err != nil {
		WriteBadRequest(w, "target_date must be a date in the form YYYY-MM-DD.")
		return
	}

	entry, err := r.db.RecordFailureVerdict(req.Context(), datastore.RecordFailureVerdictParams{
		SubjectName:  strings.TrimSpace(body.SubjectName),
		SubjectType:  body.SubjectType,
		CookbookName: strings.TrimSpace(body.CookbookName),
		Verdict:      body.Verdict,
		Reason:       body.Reason,
		Evidence:     body.Evidence,
		Diagnosis:    body.Diagnosis,
		Plan:         body.Plan,
		TargetDate:   targetDate,
		HolderType:   body.HolderType,
		HolderRef:    body.HolderRef,
		RaisedBy:     adminUsername(req),
	})
	if err != nil {
		r.logf("ERROR", "failure-register: recording a verdict about %s: %v", body.SubjectName, err)
		WriteBadRequest(w, "Failed to record the verdict: "+err.Error())
		return
	}

	details, _ := json.Marshal(entry)
	r.auditOwnership(req, "failure_recorded", "", entry.SubjectType, entry.SubjectName, "", details)

	// A human verdict outranks every automated one, so which nodes are ready
	// has just changed. Without this the register is a list nothing reflects
	// until the next collection.
	r.recomputeReadinessAfterRegisterChange("recording a verdict")

	WriteJSON(w, http.StatusCreated, entry)
}

func (r *Router) handleReviseFailureEntry(w http.ResponseWriter, req *http.Request, id string) {
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	var body struct {
		Diagnosis  *string `json:"diagnosis"`
		Plan       *string `json:"plan"`
		Evidence   *string `json:"evidence"`
		TargetDate *string `json:"target_date"`
		HolderType *string `json:"holder_type"`
		HolderRef  *string `json:"holder_ref"`

		// Accepted only so they can be refused — see below.
		Verdict *string `json:"verdict"`
		Reason  *string `json:"reason"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	// Verdicts are superseded, never silently replaced. Letting a revision
	// rewrite the verdict or the reason would erase who said what and why,
	// which is the point of the register.
	if body.Verdict != nil || body.Reason != nil {
		WriteBadRequest(w, "A verdict and its reason cannot be edited. "+
			"Record a new verdict instead — it supersedes this one and the original stays readable.")
		return
	}

	var targetDate *time.Time
	if body.TargetDate != nil {
		parsed, err := parseOptionalDate(*body.TargetDate)
		if err != nil {
			WriteBadRequest(w, "target_date must be a date in the form YYYY-MM-DD.")
			return
		}
		targetDate = parsed
	}

	entry, err := r.db.ReviseFailureEntry(req.Context(), id, datastore.ReviseFailureEntryParams{
		Diagnosis:  body.Diagnosis,
		Plan:       body.Plan,
		Evidence:   body.Evidence,
		TargetDate: targetDate,
		HolderType: body.HolderType,
		HolderRef:  body.HolderRef,
	})
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, "No such register entry.")
		return
	}
	if err != nil {
		r.logf("ERROR", "failure-register: revising entry %s: %v", id, err)
		WriteBadRequest(w, "Failed to revise the entry: "+err.Error())
		return
	}

	details, _ := json.Marshal(entry)
	r.auditOwnership(req, "failure_revised", "", entry.SubjectType, entry.SubjectName, "", details)

	WriteJSON(w, http.StatusOK, entry)
}

func (r *Router) handleResolveFailureEntry(w http.ResponseWriter, req *http.Request, id string) {
	if !requireOperatorOrAdmin(w, req) {
		return
	}
	if id == "" {
		WriteBadRequest(w, "A register entry id is required.")
		return
	}

	var body struct {
		Note string `json:"note"`
	}
	// An empty body is legitimate: the note is optional.
	_ = json.NewDecoder(req.Body).Decode(&body)

	entry, err := r.db.ResolveFailureEntry(req.Context(), id, adminUsername(req), body.Note)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, "No such register entry.")
		return
	}
	if err != nil {
		r.logf("ERROR", "failure-register: resolving entry %s: %v", id, err)
		WriteBadRequest(w, "Failed to resolve the entry: "+err.Error())
		return
	}

	details, _ := json.Marshal(entry)
	r.auditOwnership(req, "failure_resolved", "", entry.SubjectType, entry.SubjectName, "", details)

	// A resolved verdict stops outranking the automated ones.
	r.recomputeReadinessAfterRegisterChange("resolving a verdict")

	WriteJSON(w, http.StatusOK, entry)
}

// recomputeReadinessAfterRegisterChange kicks a readiness recompute so the
// rollups, list views and exports reflect the new human verdict.
//
// Best-effort by design: the verdict is already recorded, and the next
// collection materialises it regardless. Failing to recompute must never lose
// the entry somebody just took the trouble to write down.
func (r *Router) recomputeReadinessAfterRegisterChange(what string) {
	if r.readinessReconciler == nil {
		return
	}
	if err := r.readinessReconciler(); err != nil {
		r.logf("WARN", "failure-register: recomputing readiness after %s: %v", what, err)
	}
}

// parseOptionalDate reads a YYYY-MM-DD date. An empty string is no date rather
// than an error: a failure is worth recording before anybody has committed to
// a date.
func parseOptionalDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, err
	}
	return &d, nil
}
