// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipimport"
)

// ---------------------------------------------------------------------------
// Discovery-driven ownership intake — /api/v1/ownership/import/*
//
// The fixed-header import at /api/v1/ownership/import is a separate,
// unchanged handler. See specifications/ownership-intake.md.
// ---------------------------------------------------------------------------

// intakeMaxUploadBytes bounds what is buffered from an upload. It matches the
// fixed-header import's limit.
const intakeMaxUploadBytes = 10 << 20

// intakeMaxRows bounds a single import, matching the fixed-header path's cap.
// Profiling reads to the end of the source, so an unbounded file would be an
// unbounded allocation.
const intakeMaxRows = 10000

// aliasResolutionOrder is the order the raw owner string is tried against
// owner_aliases.
//
// "custom" leads because it is what this import seeds: a re-run of the same
// file must resolve to the owners the previous run created. "git_name" is here
// because incoming ownership data routinely identifies people by name rather
// than by any handle, and that is the alias type a person's name belongs to.
//
// It deliberately does NOT include a bare-localpart tier. A commit address is
// not a corporate address, and the committer-assign path already forces owner
// names to the email localpart — so matching on localpart alone is exactly how
// one person inherits another's identity. That signal is offered as a
// suggestion instead.
var aliasResolutionOrder = []string{"custom", "email", "username", "git_name", "git_email"}

func (r *Router) handleOwnershipIntake(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path

	switch {
	case path == "/api/v1/ownership/import/profile":
		r.handleIntakeProfile(w, req)
	case path == "/api/v1/ownership/import/preview":
		r.handleIntakeRun(w, req, false)
	case path == "/api/v1/ownership/import/commit":
		r.handleIntakeRun(w, req, true)
	case path == "/api/v1/ownership/import/mappings":
		r.handleIntakeMappingCollection(w, req)
	case strings.HasPrefix(path, "/api/v1/ownership/import/mappings/"):
		r.handleIntakeMappingItem(w, req, strings.TrimPrefix(path, "/api/v1/ownership/import/mappings/"))
	default:
		WriteNotFound(w, fmt.Sprintf("Unknown ownership import endpoint: %s", path))
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/import/profile
// ---------------------------------------------------------------------------

func (r *Router) handleIntakeProfile(w http.ResponseWriter, req *http.Request) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}

	src, cleanup, ok := r.openIntakeSource(w, req)
	if !ok {
		return
	}
	defer cleanup()

	profile, err := ownershipimport.Profile(src)
	if err != nil {
		WriteBadRequest(w, "Could not read the source: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, profile)
}

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/import/preview and /commit
// ---------------------------------------------------------------------------

func (r *Router) handleIntakeRun(w http.ResponseWriter, req *http.Request, commit bool) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}
	// Preview writes nothing, so it needs only standard protected auth.
	// Requiring the operator role to look would stop the people who own the
	// data from checking it before an administrator commits it.
	if commit && !requireOperatorOrAdmin(w, req) {
		return
	}

	src, cleanup, ok := r.openIntakeSource(w, req)
	if !ok {
		return
	}
	defer cleanup()

	fieldMap, ok := r.resolveFieldMap(w, req)
	if !ok {
		return
	}

	columns := src.Columns()
	if errs := fieldMap.Validate(columns); len(errs) > 0 {
		WriteError(w, http.StatusBadRequest, ErrCodeValidationError, formatMappingErrors(errs))
		return
	}

	mapper, err := ownershipimport.NewMapper(fieldMap, columns)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrCodeValidationError, err.Error())
		return
	}

	mapped := make([]ownershipimport.MappedRow, 0, 64)
	for src.Next() {
		if len(mapped) >= intakeMaxRows {
			WriteBadRequest(w, fmt.Sprintf("Import is limited to %d rows per request.", intakeMaxRows))
			return
		}
		mapped = append(mapped, mapper.MapRow(src.Row()))
	}
	if err := src.Err(); err != nil {
		// A source that failed part way through must not be reported as a
		// short one — the administrator would commit a partial import
		// believing it was the whole file.
		WriteBadRequest(w, "Could not read the source: "+err.Error())
		return
	}

	// createOwners is on by default. An import's job is to get the data in:
	// requiring owners to pre-exist would make a first import impossible, and
	// stopping to adjudicate names mid-file is the expensive thing at any real
	// size. A person who turns out to be a duplicate stays correctable
	// afterwards by reassignment. Switch it off for a strict import against an
	// established owner catalogue.
	createOwners := req.FormValue("create_owners") != "false"

	report := r.classifyIntakeRows(req.Context(), mapped, createOwners)

	if commit {
		r.commitIntakeRows(req, &report)
	}

	WriteJSON(w, http.StatusOK, report)
}

// classifyIntakeRows resolves owners and entities and decides what each row
// would do. It performs no writes.
func (r *Router) classifyIntakeRows(ctx context.Context, mapped []ownershipimport.MappedRow, createOwners bool) ownershipimport.Report {
	report := ownershipimport.Report{
		Rows:            make([]ownershipimport.ReportRow, 0, len(mapped)),
		Counts:          map[string]int{},
		RowCount:        len(mapped),
		UnmatchedOwners: []ownershipimport.UnmatchedOwner{},
		NewOwners:       []ownershipimport.NewOwner{},
	}

	existing := r.lookupExistingAssignments(ctx, mapped)
	collected := r.lookupCollectedEntities(ctx, mapped)

	resolved := make(map[string]ownerResolution)
	unmatched := map[string]int{}
	newOwners := map[string]ownershipimport.NewOwner{}

	for _, m := range mapped {
		row := ownershipimport.ReportRow{
			MappedRow:   m,
			EntityMatch: ownershipimport.EntityMatchNotFound,
		}

		if m.EntityKey != "" && collected[m.EntityType][m.EntityKey] {
			row.EntityMatch = ownershipimport.EntityMatchFound
		}

		if m.RejectedReason != "" {
			row.OwnerMatch = ownershipimport.OwnerMatchUnknown
			row.Outcome = ownershipimport.OutcomeRejected
			report.Rows = append(report.Rows, row)
			report.Counts[row.Outcome]++
			continue
		}

		res, seen := resolved[m.OwnerRaw]
		if !seen {
			res = r.resolveImportOwner(ctx, m)
			resolved[m.OwnerRaw] = res
		}

		row.OwnerMatch = res.match
		row.OwnerSuggestions = res.suggestions
		row.AliasConflict = res.aliasConflict
		row.AliasConflictOwner = res.aliasConflictOwner

		switch {
		case res.match == ownershipimport.OwnerMatchExact || res.match == ownershipimport.OwnerMatchAlias:
			row.Owner = res.ownerName

		case !createOwners:
			// Strict matching, for an import against an established owner
			// catalogue where a new person really is a mistake.
			row.Owner = ""
			row.Outcome = ownershipimport.OutcomeRejected
			row.RejectedReason = ownershipimport.ReasonUnknownOwner
			unmatched[m.OwnerRaw]++
			report.Rows = append(report.Rows, row)
			report.Counts[row.Outcome]++
			continue

		default:
			// Nothing resolved, with or without candidates. The row is
			// attributed to a new owner built from the raw value — never to a
			// suggested one, which needs a human — and any suggestion travels
			// with it.
			//
			// It deliberately does not reject. Adjudicating names mid-import
			// is the expensive thing on a file of any size, and a mistaken
			// owner stays correctable long afterwards by reassignment, at the
			// point where somebody is actually looking at the repo and
			// wondering who this is. Rejecting would instead lose the
			// assignment at ingest, when nobody has that context yet.
			row.CreatesOwner = true
			newOwners[row.Owner] = accumulateNewOwner(newOwners[row.Owner], row, res)
		}

		row.Outcome = classifyAssignmentOutcome(row.Owner, m, existing[m.EntityType][m.EntityKey], &row)
		report.Rows = append(report.Rows, row)
		report.Counts[row.Outcome]++
		if row.AliasConflict {
			report.AliasConflict++
		}
	}

	report.UnmatchedOwners = topUnmatchedOwners(unmatched)
	report.NewOwners = sortedNewOwners(newOwners)
	return report
}

// accumulateNewOwner folds one row into the entry for the person it names. One
// person appears once however many rows name them, so the list a human scans
// stays the length of the team rather than the length of the file.
func accumulateNewOwner(entry ownershipimport.NewOwner, row ownershipimport.ReportRow, res ownerResolution) ownershipimport.NewOwner {
	entry.Name = row.Owner
	entry.DisplayName = row.DisplayName
	entry.SourceValue = row.OwnerRaw
	entry.RowCount++
	if len(entry.Suggestions) == 0 {
		entry.Suggestions = res.suggestions
	}
	return entry
}

func sortedNewOwners(byName map[string]ownershipimport.NewOwner) []ownershipimport.NewOwner {
	out := make([]ownershipimport.NewOwner, 0, len(byName))
	for _, entry := range byName {
		out = append(out, entry)
	}
	// Most rows first: the person who turns up all over the file is the one
	// worth recognising before the import lands.
	sort.Slice(out, func(i, j int) bool {
		if out[i].RowCount != out[j].RowCount {
			return out[i].RowCount > out[j].RowCount
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// classifyAssignmentOutcome decides what an assignment would do against what
// already exists on the entity.
func classifyAssignmentOutcome(owner string, m ownershipimport.MappedRow, held []datastore.EntityAssignment, row *ownershipimport.ReportRow) string {
	var others []string
	for _, a := range held {
		// The uniqueness rule is organisation-scoped: the same owner and entity
		// in two organisations are two distinct assignments, so an absent
		// organisation only duplicates another absent one.
		if a.OwnerName == owner && a.OrganisationName == m.Organisation {
			return ownershipimport.OutcomeDuplicateExists
		}
		if a.OwnerName != owner {
			others = append(others, a.OwnerName)
		}
	}

	if len(others) > 0 {
		sort.Strings(others)
		row.ExistingOwners = others
		// Reported so the administrator sees the overlap. The assignment is
		// still created — ownership_assignments is many-to-many and skipping
		// the write would silently drop what the operator asked for.
		return ownershipimport.OutcomeOwnedByOther
	}
	return ownershipimport.OutcomeWouldCreate
}

type ownerResolution struct {
	ownerName          string
	match              string
	suggestions        []ownershipimport.OwnerSuggestion
	aliasConflict      bool
	aliasConflictOwner string
}

// resolveImportOwner runs the resolution chain for one raw owner string:
// exact owner name, then each alias type, then suggestions.
func (r *Router) resolveImportOwner(ctx context.Context, m ownershipimport.MappedRow) ownerResolution {
	// A record with an empty name is not a resolution. Treating one as a match
	// would attribute the assignment to an empty owner_name, which the foreign
	// key rejects at write time rather than here, where it can be reported.
	if owner, err := r.db.GetOwnerByName(ctx, m.Owner); err == nil && owner.Name != "" {
		return ownerResolution{ownerName: owner.Name, match: ownershipimport.OwnerMatchExact}
	} else if err != nil && !errors.Is(err, datastore.ErrNotFound) {
		r.logf("ERROR", "ownership/import: looking up owner %s: %v", m.Owner, err)
	}

	// Try the raw string, then its lowercase form: owner_aliases has no citext
	// and no lower() index, so matching is byte-exact while the case of a name
	// or an address in an export varies freely.
	candidates := []string{m.OwnerRaw}
	if lower := strings.ToLower(m.OwnerRaw); lower != m.OwnerRaw {
		candidates = append(candidates, lower)
	}

	for _, aliasType := range aliasResolutionOrder {
		for _, value := range candidates {
			name, err := r.db.ResolveOwnerByAlias(ctx, aliasType, value)
			if err == nil && name != "" {
				return ownerResolution{ownerName: name, match: ownershipimport.OwnerMatchAlias}
			}
			if err != nil && !errors.Is(err, datastore.ErrNotFound) {
				r.logf("ERROR", "ownership/import: resolving %s alias %q: %v", aliasType, value, err)
			}
		}
	}

	res := ownerResolution{match: ownershipimport.OwnerMatchUnknown}

	// Nothing resolved. Gather candidates so a human can recognise the person
	// later — they are shown, never picked.
	res.suggestions = r.suggestImportOwners(ctx, m.OwnerRaw)
	if len(res.suggestions) > 0 {
		res.match = ownershipimport.OwnerMatchFuzzy
	}

	// Either way this owner would be created, so check whether the raw string
	// is already somebody else's custom alias: the alias uniqueness constraint
	// is global, so the seed would fail. That skips the seed and nothing else.
	// In practice the resolution loop above catches most of these, since it
	// tries "custom" first; this guards the case where an owner appears between
	// a preview and its commit.
	if holder, err := r.db.ResolveOwnerByAlias(ctx, "custom", m.OwnerRaw); err == nil && holder != "" && holder != m.Owner {
		res.aliasConflict = true
		res.aliasConflictOwner = holder
	}
	return res
}

// suggestImportOwners gathers candidates from two independent signals: an exact
// email-localpart hit, and trigram similarity over alias values.
func (r *Router) suggestImportOwners(ctx context.Context, raw string) []ownershipimport.OwnerSuggestion {
	var out []ownershipimport.OwnerSuggestion
	seen := map[string]bool{}

	add := func(suggestions []datastore.AliasSuggestion) {
		for _, s := range suggestions {
			if seen[s.OwnerName] || len(out) >= ownershipimport.MaxOwnerSuggestions {
				continue
			}
			seen[s.OwnerName] = true
			out = append(out, ownershipimport.OwnerSuggestion{OwnerName: s.OwnerName, Score: s.Similarity})
		}
	}

	// The localpart signal goes first: an exact localpart hit across two
	// domains is a much stronger lead than any similarity score, and git
	// history routinely carries one person under several domains.
	if localpart, _, isEmail := strings.Cut(raw, "@"); isEmail && localpart != "" {
		hits, err := r.db.SuggestOwnersByEmailLocalpart(ctx, localpart, ownershipimport.MaxOwnerSuggestions)
		if err != nil {
			r.logf("ERROR", "ownership/import: suggesting owners for localpart %q: %v", localpart, err)
		}
		add(hits)
	}

	hits, err := r.db.SuggestOwnerAliases(ctx, raw, ownershipimport.MaxOwnerSuggestions)
	if err != nil {
		r.logf("ERROR", "ownership/import: suggesting owners for %q: %v", raw, err)
	}
	add(hits)

	return out
}

// lookupExistingAssignments batches the assignment lookup by entity type. A
// per-row query would make a ten-thousand-row preview ten thousand round trips.
func (r *Router) lookupExistingAssignments(ctx context.Context, mapped []ownershipimport.MappedRow) map[string]map[string][]datastore.EntityAssignment {
	out := map[string]map[string][]datastore.EntityAssignment{}

	for entityType, keys := range keysByEntityType(mapped) {
		found, err := r.db.LookupAssignmentOwnersByEntity(ctx, entityType, keys)
		if err != nil {
			// A lookup failure must not fail the import: the outcome degrades
			// to "would create", which the administrator sees in the preview.
			r.logf("ERROR", "ownership/import: looking up %s assignments: %v", entityType, err)
			continue
		}
		out[entityType] = found
	}
	return out
}

func (r *Router) lookupCollectedEntities(ctx context.Context, mapped []ownershipimport.MappedRow) map[string]map[string]bool {
	out := map[string]map[string]bool{}

	for entityType, keys := range keysByEntityType(mapped) {
		found, err := r.db.EntityKeysExist(ctx, entityType, keys)
		if err != nil {
			r.logf("ERROR", "ownership/import: checking %s entity keys: %v", entityType, err)
			continue
		}
		out[entityType] = found
	}
	return out
}

func keysByEntityType(mapped []ownershipimport.MappedRow) map[string][]string {
	seen := map[string]map[string]bool{}
	out := map[string][]string{}

	for _, m := range mapped {
		if m.EntityType == "" || m.EntityKey == "" {
			continue
		}
		if seen[m.EntityType] == nil {
			seen[m.EntityType] = map[string]bool{}
		}
		if seen[m.EntityType][m.EntityKey] {
			continue
		}
		seen[m.EntityType][m.EntityKey] = true
		out[m.EntityType] = append(out[m.EntityType], m.EntityKey)
	}
	return out
}

func topUnmatchedOwners(counts map[string]int) []ownershipimport.UnmatchedOwner {
	out := make([]ownershipimport.UnmatchedOwner, 0, len(counts))
	for value, count := range counts {
		out = append(out, ownershipimport.UnmatchedOwner{Value: value, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Value < out[j].Value
	})
	if len(out) > ownershipimport.MaxUnmatchedOwners {
		out = out[:ownershipimport.MaxUnmatchedOwners]
	}
	return out
}

// ---------------------------------------------------------------------------
// Commit
// ---------------------------------------------------------------------------

// commitIntakeRows writes the assignments the report says would be created.
// Rejected rows are never written.
func (r *Router) commitIntakeRows(req *http.Request, report *ownershipimport.Report) {
	ctx := req.Context()
	report.Committed = true
	createdOwners := map[string]bool{}

	for i := range report.Rows {
		row := &report.Rows[i]
		if row.Outcome == ownershipimport.OutcomeRejected || row.Outcome == ownershipimport.OutcomeDuplicateExists {
			continue
		}

		if row.CreatesOwner && !createdOwners[row.Owner] {
			if !r.createIntakeOwner(req, row) {
				continue
			}
			createdOwners[row.Owner] = true
		}

		_, err := r.db.InsertAssignment(ctx, datastore.InsertAssignmentParams{
			OwnerName:        row.Owner,
			EntityType:       row.EntityType,
			EntityKey:        row.EntityKey,
			OrganisationName: row.Organisation,
			AssignmentSource: "import",
			Confidence:       "definitive",
			Notes:            row.Notes,
		})
		if errors.Is(err, datastore.ErrAlreadyExists) {
			// Something created it between the preview and now. That is the
			// duplicate outcome, not a failure.
			row.Outcome = ownershipimport.OutcomeDuplicateExists
			continue
		}
		if err != nil {
			r.logf("ERROR", "ownership/import: creating assignment for %s at row %d: %v", row.Owner, row.SourceRow, err)
			row.Outcome = ownershipimport.OutcomeRejected
			row.RejectedReason = ownershipimport.ReasonMalformedRow
			continue
		}

		details, _ := json.Marshal(map[string]any{
			"assignment_source": "import",
			"confidence":        "definitive",
			"source_row":        row.SourceRow,
		})
		r.auditOwnership(req, "assignment_created", row.Owner, row.EntityType, row.EntityKey, row.Organisation, details)
		report.Created++
	}

	// The outcome counts described the preview. Recount so the response
	// describes what actually happened.
	report.Counts = map[string]int{}
	for _, row := range report.Rows {
		report.Counts[row.Outcome]++
	}
}

// createIntakeOwner creates the owner a row names, and seeds the raw string as
// a custom alias so the original is what future imports compare against.
func (r *Router) createIntakeOwner(req *http.Request, row *ownershipimport.ReportRow) bool {
	ctx := req.Context()

	owner, err := r.db.InsertOwner(ctx, datastore.InsertOwnerParams{
		Name:        row.Owner,
		DisplayName: row.DisplayName,
		OwnerType:   "individual",
	})
	if errors.Is(err, datastore.ErrAlreadyExists) {
		// Created concurrently, or twice within this file. Either way the owner
		// now exists, which is all the assignment needs.
		row.CreatesOwner = false
	} else if err != nil {
		r.logf("ERROR", "ownership/import: creating owner %s: %v", row.Owner, err)
		row.Outcome = ownershipimport.OutcomeRejected
		row.RejectedReason = ownershipimport.ReasonUnknownOwner
		return false
	} else {
		details, _ := json.Marshal(map[string]any{"display_name": row.DisplayName, "source": "import"})
		r.auditOwnership(req, "owner_created", owner.Name, "", "", "", details)
	}

	if row.AliasConflict {
		// The alias uniqueness constraint is global, so the seed would fail.
		// The assignment goes ahead regardless — this is a fact about the seed,
		// not about the assignment.
		return true
	}

	_, err = r.db.InsertOwnerAlias(ctx, datastore.InsertOwnerAliasParams{
		OwnerName:  row.Owner,
		AliasType:  "custom",
		AliasValue: row.OwnerRaw,
		Source:     "import",
	})
	if err != nil && !errors.Is(err, datastore.ErrAlreadyExists) {
		// A failed seed costs future matching accuracy, not this import.
		r.logf("WARN", "ownership/import: seeding custom alias %q for %s: %v", row.OwnerRaw, row.Owner, err)
	}
	return true
}

// ---------------------------------------------------------------------------
// Saved mappings
// ---------------------------------------------------------------------------

type mappingRequestBody struct {
	Name       string                   `json:"name"`
	SourceKind string                   `json:"source_kind"`
	Delimiter  string                   `json:"delimiter"`
	FieldMap   ownershipimport.FieldMap `json:"field_map"`
	rawMap     json.RawMessage          `json:"-"`
}

func (r *Router) handleIntakeMappingCollection(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.listIntakeMappings(w, req)
	case http.MethodPost:
		r.createIntakeMapping(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Method not allowed.")
	}
}

func (r *Router) handleIntakeMappingItem(w http.ResponseWriter, req *http.Request, idPart string) {
	id, err := strconv.ParseInt(strings.Trim(idPart, "/"), 10, 64)
	if err != nil {
		WriteNotFound(w, fmt.Sprintf("No import mapping with id %q.", idPart))
		return
	}

	switch req.Method {
	case http.MethodGet:
		r.getIntakeMapping(w, req, id)
	case http.MethodPut:
		r.updateIntakeMapping(w, req, id)
	case http.MethodDelete:
		r.deleteIntakeMapping(w, req, id)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Method not allowed.")
	}
}

func (r *Router) listIntakeMappings(w http.ResponseWriter, req *http.Request) {
	pg := ParsePagination(req)

	mappings, total, err := r.db.ListImportMappings(req.Context(), pg.Limit(), pg.Offset())
	if err != nil {
		r.logf("ERROR", "ownership/import: listing mappings: %v", err)
		WriteInternalError(w, "Failed to list import mappings.")
		return
	}
	if mappings == nil {
		mappings = []datastore.ImportMapping{}
	}
	WritePaginated(w, mappings, pg, total)
}

func (r *Router) createIntakeMapping(w http.ResponseWriter, req *http.Request) {
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	body, ok := decodeMappingBody(w, req)
	if !ok {
		return
	}

	mapping, err := r.db.InsertImportMapping(req.Context(), datastore.InsertImportMappingParams{
		Name:       body.Name,
		SourceKind: body.SourceKind,
		Delimiter:  body.Delimiter,
		FieldMap:   body.rawMap,
		CreatedBy:  adminUsername(req),
	})
	if errors.Is(err, datastore.ErrAlreadyExists) {
		WriteError(w, http.StatusConflict, ErrCodeValidationError,
			fmt.Sprintf("An import mapping named %q already exists.", body.Name))
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership/import: creating mapping %s: %v", body.Name, err)
		WriteInternalError(w, "Failed to create the import mapping.")
		return
	}

	WriteJSON(w, http.StatusCreated, mapping)
}

func (r *Router) getIntakeMapping(w http.ResponseWriter, req *http.Request, id int64) {
	mapping, err := r.db.GetImportMapping(req.Context(), id)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("No import mapping with id %d.", id))
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership/import: getting mapping %d: %v", id, err)
		WriteInternalError(w, "Failed to load the import mapping.")
		return
	}
	WriteJSON(w, http.StatusOK, mapping)
}

func (r *Router) updateIntakeMapping(w http.ResponseWriter, req *http.Request, id int64) {
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	body, ok := decodeMappingBody(w, req)
	if !ok {
		return
	}

	// Editing a mapping never re-runs a past import — a mapping is a template,
	// not a record of what happened.
	mapping, err := r.db.UpdateImportMapping(req.Context(), id, datastore.UpdateImportMappingParams{
		Name:      body.Name,
		Delimiter: body.Delimiter,
		FieldMap:  body.rawMap,
	})
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("No import mapping with id %d.", id))
		return
	}
	if errors.Is(err, datastore.ErrAlreadyExists) {
		WriteError(w, http.StatusConflict, ErrCodeValidationError,
			fmt.Sprintf("An import mapping named %q already exists.", body.Name))
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership/import: updating mapping %d: %v", id, err)
		WriteInternalError(w, "Failed to update the import mapping.")
		return
	}

	WriteJSON(w, http.StatusOK, mapping)
}

func (r *Router) deleteIntakeMapping(w http.ResponseWriter, req *http.Request, id int64) {
	if !requireOperatorOrAdmin(w, req) {
		return
	}

	err := r.db.DeleteImportMapping(req.Context(), id)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("No import mapping with id %d.", id))
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership/import: deleting mapping %d: %v", id, err)
		WriteInternalError(w, "Failed to delete the import mapping.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeMappingBody(w http.ResponseWriter, req *http.Request) (mappingRequestBody, bool) {
	var raw struct {
		Name       string          `json:"name"`
		SourceKind string          `json:"source_kind"`
		Delimiter  string          `json:"delimiter"`
		FieldMap   json.RawMessage `json:"field_map"`
	}
	if err := json.NewDecoder(req.Body).Decode(&raw); err != nil {
		WriteBadRequest(w, "Invalid or malformed JSON request body.")
		return mappingRequestBody{}, false
	}
	if strings.TrimSpace(raw.Name) == "" {
		WriteBadRequest(w, "name is required.")
		return mappingRequestBody{}, false
	}
	if len(raw.FieldMap) == 0 {
		WriteBadRequest(w, "field_map is required.")
		return mappingRequestBody{}, false
	}

	var fieldMap ownershipimport.FieldMap
	if err := json.Unmarshal(raw.FieldMap, &fieldMap); err != nil {
		WriteBadRequest(w, "field_map is not a valid mapping document: "+err.Error())
		return mappingRequestBody{}, false
	}

	// A saved mapping is validated without a column list: the columns belong to
	// whatever file it is later applied to, so a "no such column" fault can
	// only be raised at preview time. Everything else — unknown target field,
	// non-constant entity_type, uncompilable pattern — is caught here, at save
	// time, rather than surfacing per row on a future import.
	if errs := validateMappingWithoutColumns(fieldMap); len(errs) > 0 {
		WriteError(w, http.StatusBadRequest, ErrCodeValidationError, formatMappingErrors(errs))
		return mappingRequestBody{}, false
	}

	return mappingRequestBody{
		Name:       strings.TrimSpace(raw.Name),
		SourceKind: raw.SourceKind,
		Delimiter:  raw.Delimiter,
		FieldMap:   fieldMap,
		rawMap:     raw.FieldMap,
	}, true
}

// validateMappingWithoutColumns validates everything that does not depend on a
// particular source's column list, by validating against the columns the
// document itself names.
func validateMappingWithoutColumns(fm ownershipimport.FieldMap) []ownershipimport.ValidationError {
	var columns []string
	for _, mapping := range fm {
		switch mapping.Source.Kind {
		case ownershipimport.SourceColumn:
			columns = append(columns, mapping.Source.Column)
		case ownershipimport.SourceConcat:
			columns = append(columns, mapping.Source.Columns...)
		}
	}
	return fm.Validate(columns)
}

func formatMappingErrors(errs []ownershipimport.ValidationError) string {
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = e.Error()
	}
	return "The mapping is not valid: " + strings.Join(parts, "; ")
}

// ---------------------------------------------------------------------------
// Request plumbing
// ---------------------------------------------------------------------------

// openIntakeSource opens a RowSource over the uploaded file. Every endpoint
// opens its own: a source is single-pass and not re-readable, because a future
// SQL source is a streaming cursor.
func (r *Router) openIntakeSource(w http.ResponseWriter, req *http.Request) (ownershipimport.RowSource, func(), bool) {
	if err := req.ParseMultipartForm(intakeMaxUploadBytes); err != nil {
		WriteBadRequest(w, "Invalid multipart/form-data request.")
		return nil, nil, false
	}

	file, _, err := req.FormFile("file")
	if err != nil {
		WriteBadRequest(w, "file field is required.")
		return nil, nil, false
	}

	delimiter, ok := r.resolveDelimiter(w, req, file)
	if !ok {
		_ = file.Close()
		return nil, nil, false
	}

	src, err := ownershipimport.NewCSVSource(file, delimiter)
	if err != nil {
		_ = file.Close()
		WriteBadRequest(w, err.Error())
		return nil, nil, false
	}

	return src, func() { _ = file.Close() }, true
}

// resolveDelimiter returns the delimiter to parse with. An explicit one is used
// verbatim with no detection, so a misdetection costs one field edit and never
// a failed import.
func (r *Router) resolveDelimiter(w http.ResponseWriter, req *http.Request, file multipart.File) (rune, bool) {
	if explicit := req.FormValue("delimiter"); explicit != "" {
		runes := []rune(explicit)
		if len(runes) != 1 {
			WriteBadRequest(w, "delimiter must be a single character.")
			return 0, false
		}
		return runes[0], true
	}

	if mappingID := req.FormValue("mapping_id"); mappingID != "" {
		if id, err := strconv.ParseInt(mappingID, 10, 64); err == nil {
			if mapping, err := r.db.GetImportMapping(req.Context(), id); err == nil && mapping.Delimiter != "" {
				return []rune(mapping.Delimiter)[0], true
			}
		}
	}

	// Sniff the head of the file, then rewind. Detection is advisory.
	sample := make([]byte, 64<<10)
	n, err := file.Read(sample)
	if err != nil && err != io.EOF {
		WriteBadRequest(w, "Could not read the uploaded file.")
		return 0, false
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		WriteBadRequest(w, "Could not rewind the uploaded file.")
		return 0, false
	}
	return ownershipimport.DetectDelimiter(sample[:n]), true
}

// resolveFieldMap takes the mapping from either an inline document or a saved
// mapping id, never both.
func (r *Router) resolveFieldMap(w http.ResponseWriter, req *http.Request) (ownershipimport.FieldMap, bool) {
	inline := req.FormValue("field_map")
	mappingID := req.FormValue("mapping_id")

	if inline != "" && mappingID != "" {
		WriteBadRequest(w, "Supply either field_map or mapping_id, not both.")
		return nil, false
	}
	if inline == "" && mappingID == "" {
		WriteBadRequest(w, "Either field_map or mapping_id is required.")
		return nil, false
	}

	raw := []byte(inline)
	if mappingID != "" {
		id, err := strconv.ParseInt(mappingID, 10, 64)
		if err != nil {
			WriteNotFound(w, fmt.Sprintf("No import mapping with id %q.", mappingID))
			return nil, false
		}
		mapping, err := r.db.GetImportMapping(req.Context(), id)
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("No import mapping with id %d.", id))
			return nil, false
		}
		if err != nil {
			r.logf("ERROR", "ownership/import: loading mapping %d: %v", id, err)
			WriteInternalError(w, "Failed to load the import mapping.")
			return nil, false
		}
		raw = mapping.FieldMap
	}

	var fieldMap ownershipimport.FieldMap
	if err := json.Unmarshal(raw, &fieldMap); err != nil {
		WriteBadRequest(w, "field_map is not a valid mapping document: "+err.Error())
		return nil, false
	}
	return fieldMap, true
}
