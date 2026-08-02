// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import (
	"encoding/json"
	"strings"
	"testing"
)

func columnsOf() []string { return []string{"Owner Email", "Repo", "Org", "Comment"} }

func minimalMap() FieldMap {
	return FieldMap{
		FieldOwner:      {Source: Source{Kind: SourceColumn, Column: "Owner Email"}},
		FieldEntityType: {Source: Source{Kind: SourceConstant, Value: "git_repo"}},
		FieldEntityKey:  {Source: Source{Kind: SourceColumn, Column: "Repo"}},
	}
}

func TestFieldMap_ValidateAcceptsAMinimalDocument(t *testing.T) {
	if errs := minimalMap().Validate(columnsOf()); len(errs) != 0 {
		t.Errorf("Validate() = %v, want no errors", errs)
	}
}

func TestFieldMap_ValidateRequiresOwnerAndEntityKey(t *testing.T) {
	tests := []struct {
		name    string
		missing string
	}{
		{"owner is required", FieldOwner},
		{"entity_key is required", FieldEntityKey},
		{"entity_type is required", FieldEntityType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := minimalMap()
			delete(fm, tt.missing)
			errs := fm.Validate(columnsOf())
			if len(errs) == 0 {
				t.Fatalf("Validate() with %s missing = no errors", tt.missing)
			}
			if !mentionsPath(errs, tt.missing) {
				t.Errorf("errors %v do not name the field path %q", errs, tt.missing)
			}
		})
	}
}

func TestFieldMap_ValidateRejectsAnUnknownTargetField(t *testing.T) {
	fm := minimalMap()
	fm["owner_team"] = FieldMapping{Source: Source{Kind: SourceColumn, Column: "Org"}}
	errs := fm.Validate(columnsOf())
	if !mentionsPath(errs, "owner_team") {
		t.Errorf("Validate() = %v, want an error naming the unknown field", errs)
	}
}

// A header the source does not have is a mapping fault, not a row fault. Every
// row would carry it, and reporting it ten thousand times buries the one thing
// the administrator has to fix.
func TestFieldMap_ValidateRejectsAColumnTheSourceDoesNotHave(t *testing.T) {
	fm := minimalMap()
	fm[FieldOrganisation] = FieldMapping{Source: Source{Kind: SourceColumn, Column: "Business Unit"}}
	errs := fm.Validate(columnsOf())
	if len(errs) == 0 {
		t.Fatal("Validate() = no errors, want one for the absent column")
	}
	if !mentionsPath(errs, FieldOrganisation) {
		t.Errorf("errors %v do not name the offending field path", errs)
	}
	if !strings.Contains(strings.Join(messages(errs), " "), "Business Unit") {
		t.Errorf("errors %v do not name the absent column", errs)
	}
}

func TestFieldMap_ValidateChecksEveryConcatColumn(t *testing.T) {
	fm := minimalMap()
	fm[FieldEntityKey] = FieldMapping{Source: Source{
		Kind:      SourceConcat,
		Columns:   []string{"Org", "Nope", "Repo"},
		Separator: "/",
	}}
	errs := fm.Validate(columnsOf())
	if !mentionsPath(errs, FieldEntityKey) {
		t.Errorf("Validate() = %v, want an error for the absent concat column", errs)
	}
}

func TestFieldMap_ValidateRejectsMalformedSources(t *testing.T) {
	tests := []struct {
		name string
		src  Source
	}{
		{"unknown kind", Source{Kind: "lookup", Column: "Repo"}},
		{"empty kind", Source{Column: "Repo"}},
		{"column without a column name", Source{Kind: SourceColumn}},
		{"concat with no columns", Source{Kind: SourceConcat, Separator: "/"}},
		{"constant with an empty value", Source{Kind: SourceConstant, Value: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := minimalMap()
			fm[FieldEntityKey] = FieldMapping{Source: tt.src}
			if errs := fm.Validate(columnsOf()); len(errs) == 0 {
				t.Errorf("Validate() with %+v = no errors", tt.src)
			}
		})
	}
}

// entity_type is an enum. Per-row variation from a column is not supported, and
// this is a mapping fault caught before any row is read — not a per-row
// invalid_entity_type, which survives only on the fixed-header path.
func TestFieldMap_ValidateRequiresEntityTypeToBeAConstant(t *testing.T) {
	for _, src := range []Source{
		{Kind: SourceColumn, Column: "Repo"},
		{Kind: SourceConcat, Columns: []string{"Org", "Repo"}, Separator: "-"},
	} {
		fm := minimalMap()
		fm[FieldEntityType] = FieldMapping{Source: src}
		errs := fm.Validate(columnsOf())
		if !mentionsPath(errs, FieldEntityType) {
			t.Errorf("Validate() with entity_type source %+v = %v, want a mapping error", src, errs)
		}
	}
}

func TestFieldMap_ValidateRejectsAnEntityTypeOutsideTheSchemaConstraint(t *testing.T) {
	fm := minimalMap()
	fm[FieldEntityType] = FieldMapping{Source: Source{Kind: SourceConstant, Value: "repository"}}
	errs := fm.Validate(columnsOf())
	if !mentionsPath(errs, FieldEntityType) {
		t.Errorf("Validate() = %v, want an error for the unknown entity type", errs)
	}
}

func TestFieldMap_ValidateAcceptsEveryEntityTypeTheSchemaPermits(t *testing.T) {
	// Mirrors the CHECK on ownership_assignments.entity_type
	// (migrations/0001_initial_schema.up.sql:744).
	for _, et := range []string{"node", "cookbook", "git_repo", "role", "policy"} {
		fm := minimalMap()
		fm[FieldEntityType] = FieldMapping{Source: Source{Kind: SourceConstant, Value: et}}
		if errs := fm.Validate(columnsOf()); len(errs) != 0 {
			t.Errorf("Validate() with entity_type %q = %v, want no errors", et, errs)
		}
	}
}

func TestFieldMap_ValidateReportsBadTransforms(t *testing.T) {
	fm := minimalMap()
	fm[FieldOwner] = FieldMapping{
		Source:     Source{Kind: SourceColumn, Column: "Owner Email"},
		Transforms: []Transform{{Kind: "trim"}, {Kind: "regex_extract", Pattern: "([a-z"}},
	}
	errs := fm.Validate(columnsOf())
	if !mentionsPath(errs, FieldOwner) {
		t.Errorf("Validate() = %v, want an error naming the owner field", errs)
	}
}

// Several faults in one document must all come back at once. Returning the
// first sends the administrator round the loop once per mistake.
func TestFieldMap_ValidateReportsEveryFaultAtOnce(t *testing.T) {
	fm := FieldMap{
		FieldOwner:      {Source: Source{Kind: SourceColumn, Column: "Nope"}},
		FieldEntityType: {Source: Source{Kind: SourceColumn, Column: "Repo"}},
		FieldEntityKey:  {Source: Source{Kind: "bogus"}},
		"nonsense":      {Source: Source{Kind: SourceConstant, Value: "x"}},
	}
	errs := fm.Validate(columnsOf())
	if len(errs) < 4 {
		t.Errorf("Validate() returned %d errors (%v), want at least 4", len(errs), errs)
	}
}

// The document is persisted as JSON and outlives the code that reads it, so the
// wire shape is a contract and must survive a round trip unchanged.
func TestFieldMap_JSONRoundTrip(t *testing.T) {
	original := FieldMap{
		FieldOwner: {
			Source:     Source{Kind: SourceColumn, Column: "Owner Email"},
			Transforms: []Transform{{Kind: "trim"}, {Kind: "lowercase"}, {Kind: "strip_domain"}},
		},
		FieldEntityType: {Source: Source{Kind: SourceConstant, Value: "git_repo"}},
		FieldEntityKey: {
			Source: Source{Kind: SourceConcat, Columns: []string{"Org", "Repo"}, Separator: "/"},
		},
		FieldNotes: {
			Source:     Source{Kind: SourceColumn, Column: "Comment"},
			Transforms: []Transform{{Kind: "default", Value: "imported"}},
		},
	}

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded FieldMap
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if errs := decoded.Validate(columnsOf()); len(errs) != 0 {
		t.Fatalf("round-tripped document no longer validates: %v", errs)
	}

	reEncoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-Marshal: %v", err)
	}
	if string(encoded) != string(reEncoded) {
		t.Errorf("round trip changed the document:\n first: %s\nsecond: %s", encoded, reEncoded)
	}
}

func mentionsPath(errs []ValidationError, path string) bool {
	for _, e := range errs {
		if strings.HasPrefix(e.Path, path) {
			return true
		}
	}
	return false
}

func messages(errs []ValidationError) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Message
	}
	return out
}
