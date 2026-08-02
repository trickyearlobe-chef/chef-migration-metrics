// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/merge
//
// Folding one owner into another is how a recognised duplicate is corrected
// durably. Reassigning the work alone leaves the source's aliases behind, so
// the raw string from the original export still resolves to the emptied owner
// and the next ingest undoes the correction.
// ---------------------------------------------------------------------------

func (r *Router) handleOwnershipMerge(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}
	// A merge deletes a person, which is what DELETE /owners/:name requires
	// admin for.
	if !requireAdminRole(w, req) {
		return
	}

	var body struct {
		FromOwner string `json:"from_owner"`
		IntoOwner string `json:"into_owner"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}

	if body.FromOwner == "" || body.IntoOwner == "" {
		WriteBadRequest(w, "from_owner and into_owner are required.")
		return
	}
	if body.FromOwner == body.IntoOwner {
		WriteBadRequest(w, "from_owner and into_owner must be different.")
		return
	}

	result, err := r.db.MergeOwners(req.Context(), body.FromOwner, body.IntoOwner)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, "One of the owners named does not exist.")
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership/merge: folding %s into %s: %v", body.FromOwner, body.IntoOwner, err)
		WriteInternalError(w, "Failed to merge the owners.")
		return
	}

	details, _ := json.Marshal(result)
	r.auditOwnership(req, "owner_merged", result.IntoOwner, "", "", "", details)

	WriteJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// GET  /api/v1/ownership/duplicates
// POST /api/v1/ownership/duplicates/rescan
//
// The standing list of people who may already be somebody else. The import
// invents owners deliberately, and the only screen that ever paired a new
// person with who they might be was the import report, which is gone the
// moment you navigate away.
//
// The list is read from a stored scan rather than computed per request:
// comparing every owner with every other one takes minutes on a catalogue of
// any size, because owner names cluster.
// ---------------------------------------------------------------------------

// duplicateScanTimeout bounds a runaway scan. The scan itself is bounded by
// construction; this is the backstop for a database that has stopped
// responding rather than an expected duration.
const duplicateScanTimeout = 30 * time.Minute

func (r *Router) handleOwnershipDuplicatesRescan(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	// The scan walks every owner and every alias. On a large catalogue that
	// is tens of seconds — long enough that holding the request open invites
	// a proxy timeout and a retry that starts a second scan. It runs
	// detached instead, and the list page reports when it last finished.
	if !r.duplicateScanRunning.CompareAndSwap(false, true) {
		WriteJSON(w, http.StatusAccepted, map[string]any{
			"started": false,
			"reason":  "A scan is already running.",
		})
		return
	}

	go func() {
		defer r.duplicateScanRunning.Store(false)

		ctx, cancel := context.WithTimeout(context.Background(), duplicateScanTimeout)
		defer cancel()

		started := time.Now()
		found, err := r.db.RecomputeOwnerDuplicateCandidates(ctx)
		if err != nil {
			r.logf("ERROR", "ownership/duplicates: scan failed after %s: %v",
				time.Since(started).Round(time.Second), err)
			return
		}
		r.logf("INFO", "ownership/duplicates: scan found %d possible duplicate pair(s) in %s",
			found, time.Since(started).Round(time.Second))
	}()

	WriteJSON(w, http.StatusAccepted, map[string]any{"started": true})
}

// POST /api/v1/ownership/duplicates/dismiss
//
// "These two are not the same person." Without it the view offers a merge and
// nothing else, so a pair somebody has already rejected returns on every scan
// and the list can never be worked down to nothing — which is the only state
// that makes it worth opening.
func (r *Router) handleOwnershipDuplicatesDismiss(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	var body struct {
		OwnerA string `json:"owner_a"`
		OwnerB string `json:"owner_b"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return
	}
	if body.OwnerA == "" || body.OwnerB == "" {
		WriteBadRequest(w, "owner_a and owner_b are required.")
		return
	}
	if body.OwnerA == body.OwnerB {
		WriteBadRequest(w, "owner_a and owner_b must be different.")
		return
	}

	if err := r.db.DismissOwnerDuplicate(req.Context(), body.OwnerA, body.OwnerB,
		body.Reason, adminUsername(req)); err != nil {
		r.logf("ERROR", "ownership/duplicates: dismissing %s / %s: %v", body.OwnerA, body.OwnerB, err)
		WriteBadRequest(w, "Failed to dismiss the pair: "+err.Error())
		return
	}

	details, _ := json.Marshal(body)
	r.auditOwnership(req, "owner_duplicate_dismissed", body.OwnerA, "", "", "", details)

	WriteJSON(w, http.StatusOK, map[string]any{"dismissed": true})
}

func (r *Router) handleOwnershipDuplicates(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	pg := ParsePagination(req)
	f := datastore.OwnerDuplicateFilter{
		Limit:  pg.Limit(),
		Offset: pg.Offset(),
	}
	if v := req.URL.Query().Get("min_similarity"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			f.MinSimilarity = parsed
		}
	}

	candidates, total, err := r.db.ListOwnerDuplicateCandidates(req.Context(), f)
	if err != nil {
		r.logf("ERROR", "ownership/duplicates: listing candidates: %v", err)
		WriteInternalError(w, "Failed to list possible duplicate owners.")
		return
	}
	if candidates == nil {
		candidates = []datastore.OwnerDuplicateCandidate{}
	}

	// How much of the catalogue this covers. An owner with no alias is still
	// compared by name, but only under the one name it was created with —
	// which the reader has to be told, because an empty list otherwise reads
	// as "there are no duplicates".
	coverage := map[string]any{}
	ownersTotal, ownersWithoutAlias, err := r.db.CountOwnersMissingAliases(req.Context())
	if err != nil {
		// The caveat failing must not take the list with it.
		r.logf("WARN", "ownership/duplicates: counting owners without aliases: %v", err)
	} else {
		coverage["owners_total"] = ownersTotal
		coverage["owners_without_alias"] = ownersWithoutAlias
	}

	// When the list last came from. An empty list from a scan that found
	// nothing and an empty list because nobody has scanned yet mean opposite
	// things, so the reader is told which this is.
	body := map[string]any{
		"data":         candidates,
		"pagination":   NewPaginationResponse(pg, total),
		"coverage":     coverage,
		"scan_running": r.duplicateScanRunning.Load(),
	}

	// An empty list that was worked down to nothing means something different
	// from one nobody has looked at, so the count of rejections is reported
	// alongside it. Failing to read it must not take the list with it.
	if dismissed, derr := r.db.CountOwnerDuplicateDismissals(req.Context()); derr != nil {
		r.logf("WARN", "ownership/duplicates: counting dismissals: %v", derr)
	} else {
		body["dismissed_pairs"] = dismissed
	}
	scan, err := r.db.GetOwnerDuplicateScan(req.Context())
	if err == nil {
		body["scan"] = scan
	} else if !errors.Is(err, datastore.ErrNotFound) {
		r.logf("WARN", "ownership/duplicates: reading the scan record: %v", err)
	}

	WriteJSON(w, http.StatusOK, body)
}
