// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// GET /api/v1/ownership/import/rejections
// ---------------------------------------------------------------------------
//
// The rows an import could not use, as a worklist — see
// journeys/ownership-intake.md: "which row, and what was wrong with it — so I
// can get the source fixed rather than silently importing three quarters of it."
//
// These have been stored since imports existed and were readable only by taking
// an export. So the rules about them were proven — each run replaces the last,
// one import never clears another's — while the list itself never reached the
// person who had to act on it. An import that quietly drops a quarter of its
// rows and says nothing is the failure the journey names.
//
// Read-only and admin-gated, matching the rest of intake: what a source got
// wrong is a statement about somebody else's system of record, and it names
// people.

// importRejectionItem is one unusable row as the worklist shows it. It mirrors
// the stored shape rather than reinterpreting it — a worklist that summarises
// is a worklist you cannot act on.
type importRejectionItem struct {
	ImportLabel string `json:"import_label"`
	RunAt       string `json:"run_at"`
	SourceRow   int    `json:"source_row"`
	Reason      string `json:"reason"`
	OwnerRaw    string `json:"owner_raw,omitempty"`
	EntityType  string `json:"entity_type,omitempty"`
	EntityKey   string `json:"entity_key,omitempty"`
}

type importRejectionsResponse struct {
	Data       []importRejectionItem `json:"data"`
	Pagination PaginationResponse    `json:"pagination"`
}

// handleOwnershipImportRejections handles GET /api/v1/ownership/import/rejections.
func (r *Router) handleOwnershipImportRejections(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	if !requireAdminRole(w, req) {
		return
	}

	ctx := req.Context()
	pg := ParsePagination(req)

	// Asks for one more than the page so "is there another page" is answered
	// without a second count query. The stored set is bounded by one run of one
	// import, so this stays small.
	rejections, err := r.db.ListImportRejections(ctx, pg.PerPage+1, (pg.Page-1)*pg.PerPage)
	if err != nil {
		r.logf("ERROR", "listing import rejections: %v", err)
		WriteInternalError(w, "Failed to load the rows the import could not use.")
		return
	}

	hasMore := len(rejections) > pg.PerPage
	if hasMore {
		rejections = rejections[:pg.PerPage]
	}

	items := make([]importRejectionItem, 0, len(rejections))
	for i := range rejections {
		items = append(items, rejectionItem(rejections[i]))
	}

	// The total is not known without counting, and counting a list whose whole
	// purpose is to be worked down to empty is not worth a second query. The
	// pagination reports what this page holds plus whether another follows.
	total := (pg.Page-1)*pg.PerPage + len(items)
	if hasMore {
		total++
	}

	WriteJSON(w, http.StatusOK, importRejectionsResponse{
		Data:       items,
		Pagination: NewPaginationResponse(pg, total),
	})
}

func rejectionItem(r datastore.ImportRejection) importRejectionItem {
	item := importRejectionItem{
		ImportLabel: r.ImportLabel,
		SourceRow:   r.SourceRow,
		Reason:      r.Reason,
		OwnerRaw:    r.OwnerRaw,
		EntityType:  r.EntityType,
		EntityKey:   r.EntityKey,
	}
	if !r.RunAt.IsZero() {
		item.RunAt = r.RunAt.Format("2006-01-02T15:04:05Z")
	}
	return item
}
