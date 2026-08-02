// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import (
	"strings"
	"testing"
)

func mapOneRow(t *testing.T, fm FieldMap, columns []string, row Row) MappedRow {
	t.Helper()
	m, err := NewMapper(fm, columns)
	if err != nil {
		t.Fatalf("NewMapper: %v", err)
	}
	return m.MapRow(row)
}

func rowOf(n int, kv map[string]string) Row {
	return Row{Number: n, Values: kv}
}

func TestMapper_MapsEveryField(t *testing.T) {
	fm := FieldMap{
		FieldOwner: {
			Source:     Source{Kind: SourceColumn, Column: "Owner Email"},
			Transforms: []Transform{{Kind: "trim"}, {Kind: "lowercase"}, {Kind: "strip_domain"}},
		},
		FieldEntityType:   {Source: Source{Kind: SourceConstant, Value: "git_repo"}},
		FieldEntityKey:    {Source: Source{Kind: SourceColumn, Column: "Repo"}},
		FieldOrganisation: {Source: Source{Kind: SourceColumn, Column: "Org"}},
		FieldNotes:        {Source: Source{Kind: SourceColumn, Column: "Comment"}},
	}
	columns := []string{"Owner Email", "Repo", "Org", "Comment"}

	got := mapOneRow(t, fm, columns, rowOf(7, map[string]string{
		"Owner Email": "  Alice.Smith@example.com ",
		"Repo":        "web-app",
		"Org":         "acme",
		"Comment":     "from the old spreadsheet",
	}))

	if got.SourceRow != 7 {
		t.Errorf("SourceRow = %d, want 7", got.SourceRow)
	}
	if got.RejectedReason != "" {
		t.Fatalf("RejectedReason = %q, want none", got.RejectedReason)
	}
	if got.Owner != "alice.smith" {
		t.Errorf("Owner = %q, want %q", got.Owner, "alice.smith")
	}
	if got.EntityType != "git_repo" {
		t.Errorf("EntityType = %q", got.EntityType)
	}
	if got.EntityKey != "web-app" {
		t.Errorf("EntityKey = %q", got.EntityKey)
	}
	if got.Organisation != "acme" {
		t.Errorf("Organisation = %q", got.Organisation)
	}
	if got.Notes != "from the old spreadsheet" {
		t.Errorf("Notes = %q", got.Notes)
	}
}

// The raw values are carried so the administrator can see what their mapping
// did to each row without re-reading the file.
func TestMapper_CarriesRawValuesAlongsideMapped(t *testing.T) {
	fm := minimalMap()
	got := mapOneRow(t, fm, columnsOf(), rowOf(1, map[string]string{
		"Owner Email": "Alice@example.com",
		"Repo":        "web-app",
	}))
	if got.Raw[FieldOwner] != "Alice@example.com" {
		t.Errorf("Raw[owner] = %q, want the untouched cell", got.Raw[FieldOwner])
	}
}

// display_name receives the value before slugification, and absent an explicit
// mapping it defaults to the owner field's pre-slugify output — not to a column.
// That is what preserves the original for fuzzy matching and future imports.
func TestMapper_DisplayNameDefaultsToThePreSlugifyOwner(t *testing.T) {
	fm := FieldMap{
		FieldOwner: {
			Source:     Source{Kind: SourceColumn, Column: "Owner Email"},
			Transforms: []Transform{{Kind: "trim"}},
		},
		FieldEntityType: {Source: Source{Kind: SourceConstant, Value: "git_repo"}},
		FieldEntityKey:  {Source: Source{Kind: SourceColumn, Column: "Repo"}},
	}
	got := mapOneRow(t, fm, columnsOf(), rowOf(1, map[string]string{
		"Owner Email": " Renée Dubois ",
		"Repo":        "web-app",
	}))

	if got.Owner != "ren-e-dubois" {
		t.Errorf("Owner = %q, want the slug", got.Owner)
	}
	if got.OwnerRaw != "Renée Dubois" {
		t.Errorf("OwnerRaw = %q, want the transformed but un-slugified value", got.OwnerRaw)
	}
	if got.DisplayName != "Renée Dubois" {
		t.Errorf("DisplayName = %q, want the pre-slugify owner", got.DisplayName)
	}
}

func TestMapper_ExplicitDisplayNameWins(t *testing.T) {
	fm := minimalMap()
	fm[FieldDisplayName] = FieldMapping{Source: Source{Kind: SourceColumn, Column: "Comment"}}
	got := mapOneRow(t, fm, columnsOf(), rowOf(1, map[string]string{
		"Owner Email": "alice@example.com",
		"Repo":        "web-app",
		"Comment":     "Alice Smith",
	}))
	if got.DisplayName != "Alice Smith" {
		t.Errorf("DisplayName = %q, want the explicitly mapped column", got.DisplayName)
	}
}

func TestMapper_ConcatSource(t *testing.T) {
	fm := minimalMap()
	fm[FieldEntityKey] = FieldMapping{Source: Source{
		Kind: SourceConcat, Columns: []string{"Org", "Repo"}, Separator: "/",
	}}
	got := mapOneRow(t, fm, columnsOf(), rowOf(1, map[string]string{
		"Owner Email": "alice@example.com",
		"Org":         "acme",
		"Repo":        "web-app",
	}))
	if got.EntityKey != "acme/web-app" {
		t.Errorf("EntityKey = %q, want %q", got.EntityKey, "acme/web-app")
	}
}

// An empty cell contributes an empty segment and separators are not collapsed,
// so a missing middle component is visible rather than silently closing up.
func TestMapper_ConcatDoesNotCollapseEmptySegments(t *testing.T) {
	fm := minimalMap()
	fm[FieldEntityKey] = FieldMapping{Source: Source{
		Kind: SourceConcat, Columns: []string{"Org", "Comment", "Repo"}, Separator: "/",
	}}
	got := mapOneRow(t, fm, columnsOf(), rowOf(1, map[string]string{
		"Owner Email": "alice@example.com",
		"Org":         "acme",
		"Comment":     "",
		"Repo":        "web-app",
	}))
	if got.EntityKey != "acme//web-app" {
		t.Errorf("EntityKey = %q, want %q — separators must not collapse", got.EntityKey, "acme//web-app")
	}
}

func TestMapper_ConstantSourceDoesNotReadTheRow(t *testing.T) {
	fm := minimalMap()
	fm[FieldOrganisation] = FieldMapping{Source: Source{Kind: SourceConstant, Value: "acme"}}
	got := mapOneRow(t, fm, columnsOf(), rowOf(1, map[string]string{
		"Owner Email": "alice@example.com",
		"Repo":        "web-app",
		"Org":         "somewhere-else",
	}))
	if got.Organisation != "acme" {
		t.Errorf("Organisation = %q, want the constant", got.Organisation)
	}
}

func TestMapper_UnmappedOptionalFieldsAreEmpty(t *testing.T) {
	got := mapOneRow(t, minimalMap(), columnsOf(), rowOf(1, map[string]string{
		"Owner Email": "alice@example.com",
		"Repo":        "web-app",
		"Org":         "acme",
		"Comment":     "ignore me",
	}))
	if got.Organisation != "" {
		t.Errorf("Organisation = %q, want empty — it was not mapped", got.Organisation)
	}
	if got.Notes != "" {
		t.Errorf("Notes = %q, want empty — it was not mapped", got.Notes)
	}
}

func TestMapper_RejectionReasons(t *testing.T) {
	tests := []struct {
		name      string
		row       Row
		wantMatch string
	}{
		{
			"an empty owner cell in a well-formed row is a missing field",
			rowOf(1, map[string]string{"Owner Email": "", "Repo": "web-app"}),
			ReasonMissingRequiredField,
		},
		{
			"an empty entity_key cell is a missing field",
			rowOf(1, map[string]string{"Owner Email": "alice@example.com", "Repo": ""}),
			ReasonMissingRequiredField,
		},
		{
			"whitespace is not a value",
			rowOf(1, map[string]string{"Owner Email": "   ", "Repo": "web-app"}),
			ReasonMissingRequiredField,
		},
		{
			"an owner that cannot become a name is its own class",
			rowOf(1, map[string]string{"Owner Email": "???", "Repo": "web-app"}),
			ReasonInvalidOwnerName,
		},
		{
			"punctuation only is invalid_owner_name, not a missing field",
			rowOf(1, map[string]string{"Owner Email": "---", "Repo": "web-app"}),
			ReasonInvalidOwnerName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapOneRow(t, minimalMap(), columnsOf(), tt.row)
			if got.RejectedReason != tt.wantMatch {
				t.Errorf("RejectedReason = %q, want %q", got.RejectedReason, tt.wantMatch)
			}
		})
	}
}

// A ragged row still yields the cells it does have. It is rejected only when the
// malformation actually cost a required field — and then the reason names the
// cause, because "missing field" would send the administrator looking at the
// mapping instead of at the file.
func TestMapper_MalformedRowIsRejectedOnlyWhenItCostsARequiredField(t *testing.T) {
	fm := minimalMap()

	complete := Row{Number: 1, Malformed: true, Values: map[string]string{
		"Owner Email": "alice@example.com", "Repo": "web-app",
	}}
	got := mapOneRow(t, fm, columnsOf(), complete)
	if got.RejectedReason != "" {
		t.Errorf("RejectedReason = %q, want none — every required field resolved", got.RejectedReason)
	}
	if !got.Malformed {
		t.Error("Malformed flag not carried through to the report")
	}

	short := Row{Number: 2, Malformed: true, Values: map[string]string{
		"Owner Email": "alice@example.com", "Repo": "",
	}}
	got = mapOneRow(t, fm, columnsOf(), short)
	if got.RejectedReason != ReasonMalformedRow {
		t.Errorf("RejectedReason = %q, want %q", got.RejectedReason, ReasonMalformedRow)
	}
}

// A mapper is built once and used for every row of an import.
func TestMapper_IsReusableAcrossRows(t *testing.T) {
	fm := minimalMap()
	fm[FieldOwner] = FieldMapping{
		Source:     Source{Kind: SourceColumn, Column: "Owner Email"},
		Transforms: []Transform{{Kind: "strip_domain"}},
	}
	m, err := NewMapper(fm, columnsOf())
	if err != nil {
		t.Fatalf("NewMapper: %v", err)
	}
	for i, want := range []string{"alice", "bob", "carol"} {
		got := m.MapRow(rowOf(i+1, map[string]string{
			"Owner Email": want + "@example.com",
			"Repo":        "repo-" + want,
		}))
		if got.Owner != want {
			t.Errorf("row %d: Owner = %q, want %q", i+1, got.Owner, want)
		}
		if got.SourceRow != i+1 {
			t.Errorf("row %d: SourceRow = %d", i+1, got.SourceRow)
		}
	}
}

func TestNewMapper_RejectsAnInvalidDocument(t *testing.T) {
	fm := minimalMap()
	fm[FieldOwner] = FieldMapping{
		Source:     Source{Kind: SourceColumn, Column: "Owner Email"},
		Transforms: []Transform{{Kind: "regex_extract", Pattern: "([a-z"}},
	}
	_, err := NewMapper(fm, columnsOf())
	if err == nil {
		t.Fatal("NewMapper with an uncompilable pattern = nil error")
	}
	if !strings.Contains(err.Error(), FieldOwner) {
		t.Errorf("error %q does not name the offending field", err)
	}
}
