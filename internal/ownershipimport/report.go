// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

// Owner match classes. These say how the owner was resolved, not what happened
// to the row — outcome says that, and the two are always read together.
const (
	OwnerMatchExact = "exact"
	OwnerMatchAlias = "alias"
	// OwnerMatchFuzzy means candidates exist but none was applied. The row is
	// attributed to a new owner built from the raw value, never to the
	// suggested one — that attribution needs a human. It does not reject
	// either: the suggestion travels with the row instead, so the clue is
	// there for whoever later asks who this person is.
	OwnerMatchFuzzy   = "fuzzy_suggestion"
	OwnerMatchUnknown = "unknown"
)

// Entity match classes. not_found never rejects a row: ownership assignments
// are soft references with no foreign key, and assigning ownership before
// collection has run is a primary use case.
const (
	EntityMatchFound    = "found"
	EntityMatchNotFound = "not_found"
)

// Row outcomes.
const (
	OutcomeWouldCreate     = "would_create"
	OutcomeDuplicateExists = "duplicate_exists"
	// OutcomeOwnedByOther reports an overlap, not a failure. The assignment is
	// still created: ownership_assignments is many-to-many and overlapping
	// ownership is legitimate by design.
	OutcomeOwnedByOther = "owned_by_other"
	OutcomeRejected     = "rejected"
)

// Rejection reasons.
const (
	ReasonUnknownOwner         = "unknown_owner"
	ReasonInvalidEntityType    = "invalid_entity_type"
	ReasonMissingRequiredField = "missing_required_field"
	ReasonMalformedRow         = "malformed_row"
	// ReasonInvalidOwnerName is its own class because a raw string that
	// slugifies to nothing is neither missing nor malformed, and conflating it
	// hides the most actionable class of import miss.
	ReasonInvalidOwnerName = "invalid_owner_name"
)

// OwnerSuggestion is one trigram-scored candidate from the existing alias
// suggestion facility. Suggestions are never applied automatically.
type OwnerSuggestion struct {
	OwnerName string  `json:"owner_name"`
	Score     float64 `json:"score"`
}

// MaxOwnerSuggestions caps how many candidates a rejected row carries. Three is
// enough to recognise the right person and few enough to read at a glance.
const MaxOwnerSuggestions = 3

// ReportRow is one row of the match report.
type ReportRow struct {
	MappedRow

	OwnerMatch  string `json:"owner_match"`
	EntityMatch string `json:"entity_match"`
	Outcome     string `json:"outcome"`

	// CreatesOwner reports that committing this row would create the owner as
	// well as the assignment. It is orthogonal to the outcome, which describes
	// the assignment.
	//
	// An owner is only ever created when nothing resolved *and* there were no
	// close candidates — with nobody to confuse them with, creating is safe,
	// and requiring owners to pre-exist would make a first import impossible.
	// Where candidates do exist the row is rejected instead, which is where
	// mis-attribution would actually happen.
	CreatesOwner bool `json:"creates_owner"`

	// ExistingOwners names who already holds the entity, when the outcome is
	// owned_by_other. It exists so the administrator sees the overlap.
	ExistingOwners []string `json:"existing_owners,omitempty"`

	// AliasConflict reports that the raw owner string is already registered as
	// an alias of a different owner. The owner-alias uniqueness constraint is
	// global, not per owner, so the same string cannot be seeded twice.
	//
	// This is a fact about the alias seed, not about the assignment: when it is
	// true the assignment is created normally and only the alias seed is
	// skipped. It must never suppress or recolour the outcome — a row may
	// legitimately be would_create with a conflicting alias, and that is
	// exactly the case an administrator needs to see.
	AliasConflict      bool   `json:"alias_conflict"`
	AliasConflictOwner string `json:"alias_conflict_owner,omitempty"`

	OwnerSuggestions []OwnerSuggestion `json:"owner_suggestions,omitempty"`
}

// UnmatchedOwner is one owner string the import could not resolve, with how
// often it appeared. Unmatched strings are recorded, never dropped: an import
// that quietly discards what it could not match cannot be corrected, because
// nobody knows what was lost.
type UnmatchedOwner struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// MaxUnmatchedOwners caps the unmatched-strings list in the report.
const MaxUnmatchedOwners = 20

// NewOwner is a person the import would add to CMM, listed once however many
// rows name them.
//
// This list exists because similarity scoring cannot recognise a person. A
// nickname shares almost no characters with the name on the account, so no
// threshold will ever surface it — but somebody reading the list will. Burying
// new people inside an assignment count is what makes a duplicate person
// invisible at the one moment it is cheap to spot.
type NewOwner struct {
	// Name is the slug that becomes owners.name.
	Name string `json:"name"`

	// DisplayName and SourceValue are the raw string — what a human actually
	// recognises. The slug is not.
	DisplayName string `json:"display_name"`
	SourceValue string `json:"source_value"`

	RowCount int `json:"row_count"`

	// Suggestions are existing owners this person might already be. They are
	// shown, never applied.
	Suggestions []OwnerSuggestion `json:"suggestions,omitempty"`
}

// Report is the whole match report for one preview or commit.
type Report struct {
	Rows []ReportRow `json:"rows"`

	// NewOwners lists every person the import would add, for review. Reviewing
	// is not a gate: correcting twenty thousand names at ingest is the
	// expensive thing, and a mistaken owner stays correctable afterwards by
	// reassignment, so the import proceeds either way.
	NewOwners []NewOwner `json:"new_owners"`

	// Counts is keyed by outcome. Alias conflicts are counted separately and
	// are never summed with these — they are an orthogonal fact.
	Counts        map[string]int `json:"counts"`
	AliasConflict int            `json:"alias_conflict_count"`

	RowCount        int              `json:"row_count"`
	UnmatchedOwners []UnmatchedOwner `json:"unmatched_owners"`

	// FilteredOut counts source rows skipped by the import filter before
	// mapping. They are not failures and are never counted as outcomes — the
	// caller asked for them to be left out.
	FilteredOut int `json:"filtered_out"`

	// RowsTruncated reports that Rows carries only the first slice of the
	// detail. Counts, RowCount and the commit are unaffected: every row was
	// processed. Without this a shortened list would read as a short import.
	RowsTruncated bool `json:"rows_truncated"`

	// Committed reports whether the assignments were written. Preview and
	// commit return the same shape, so a caller reading a report needs this to
	// know which it is holding.
	Committed bool `json:"committed"`
	Created   int  `json:"created"`
}
