// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import (
	"fmt"
	"sort"
	"strings"
)

// Target fields. owner, entity_type and entity_key are required; the rest are
// optional.
const (
	FieldOwner        = "owner"
	FieldEntityType   = "entity_type"
	FieldEntityKey    = "entity_key"
	FieldOrganisation = "organisation"
	FieldNotes        = "notes"
	FieldDisplayName  = "display_name"
)

// Source kinds.
const (
	SourceColumn   = "column"
	SourceConstant = "constant"
	SourceConcat   = "concat"
)

// TargetFields is the closed set of fields a mapping may address, in the order
// a UI should present them.
var TargetFields = []string{
	FieldOwner, FieldEntityType, FieldEntityKey,
	FieldOrganisation, FieldNotes, FieldDisplayName,
}

var requiredFields = []string{FieldOwner, FieldEntityType, FieldEntityKey}

// EntityTypes mirrors the CHECK on ownership_assignments.entity_type
// (migrations/0001_initial_schema.up.sql:744). entity_type is an enum, so a
// mapping supplies it as a constant and it is checked against this set before
// any row is read.
var EntityTypes = []string{"node", "cookbook", "git_repo", "role", "policy"}

// Source says which cells a field is built from. It is a tagged union,
// evaluated once per row to produce the initial string.
type Source struct {
	Kind string `json:"kind"`

	// Column names the single source column for kind "column".
	Column string `json:"column,omitempty"`

	// Value is the literal for kind "constant". Row cells are not read.
	Value string `json:"value,omitempty"`

	// Columns and Separator drive kind "concat", the only N-column source.
	Columns   []string `json:"columns,omitempty"`
	Separator string   `json:"separator,omitempty"`
}

// FieldMapping is one target field: exactly one source, then an ordered chain
// of transforms.
type FieldMapping struct {
	Source     Source      `json:"source"`
	Transforms []Transform `json:"transforms,omitempty"`
}

// FieldMap is the mapping document. It is persisted as JSON and outlives the
// code that reads it, so its wire shape is a contract.
type FieldMap map[string]FieldMapping

// ValidationError names the offending field path. Naming the path matters: a
// document with six fields and several transforms each has many places to be
// wrong, and "invalid mapping" sends the administrator hunting.
type ValidationError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string { return e.Path + ": " + e.Message }

// Validate checks the whole document against the source's columns and returns
// every fault at once. Returning only the first would send the administrator
// round the profile-map-preview loop once per mistake.
func (fm FieldMap) Validate(columns []string) []ValidationError {
	known := make(map[string]bool, len(columns))
	for _, c := range columns {
		known[c] = true
	}

	var errs []ValidationError
	add := func(path, format string, args ...any) {
		errs = append(errs, ValidationError{Path: path, Message: fmt.Sprintf(format, args...)})
	}

	for _, field := range requiredFields {
		if _, ok := fm[field]; !ok {
			add(field, "%s is required", field)
		}
	}

	for field, mapping := range fm {
		if !isTargetField(field) {
			add(field, "%q is not an ownership field — expected one of %s", field, strings.Join(TargetFields, ", "))
			continue
		}

		validateSource(field, mapping.Source, known, add)

		if field == FieldEntityType {
			validateEntityType(mapping.Source, add)
		}

		if _, err := CompileTransforms(mapping.Transforms); err != nil {
			add(field, "%v", err)
		}
	}

	// A stable order keeps the UI's error list from reshuffling between
	// otherwise identical requests — map iteration order is not stable.
	sort.Slice(errs, func(i, j int) bool {
		if errs[i].Path != errs[j].Path {
			return errs[i].Path < errs[j].Path
		}
		return errs[i].Message < errs[j].Message
	})
	return errs
}

func validateSource(field string, src Source, known map[string]bool, add func(string, string, ...any)) {
	switch src.Kind {
	case SourceColumn:
		if src.Column == "" {
			add(field, "a column source needs a column name")
			return
		}
		// A header the source does not have is a mapping fault, not a row
		// fault: every row would carry it, and reporting it ten thousand times
		// buries the one thing the administrator has to fix.
		if !known[src.Column] {
			add(field, "the source has no column named %q", src.Column)
		}

	case SourceConstant:
		if src.Value == "" {
			add(field, "a constant source needs a value")
		}

	case SourceConcat:
		if len(src.Columns) == 0 {
			add(field, "a concat source needs at least one column")
			return
		}
		for _, c := range src.Columns {
			if c == "" {
				add(field, "a concat source cannot include an unnamed column")
				continue
			}
			if !known[c] {
				add(field, "the source has no column named %q", c)
			}
		}

	case "":
		add(field, "a source needs a kind — one of %s, %s, %s", SourceColumn, SourceConstant, SourceConcat)

	default:
		add(field, "unknown source kind %q — expected one of %s, %s, %s", src.Kind, SourceColumn, SourceConstant, SourceConcat)
	}
}

func validateEntityType(src Source, add func(string, string, ...any)) {
	// entity_type is an enum. Per-row variation from a column is not supported,
	// and this is caught before any row is read — the per-row
	// invalid_entity_type reason survives only on the fixed-header path, where
	// the value genuinely does come from the row.
	if src.Kind != "" && src.Kind != SourceConstant {
		add(FieldEntityType, "entity_type must be a constant, not a %s — it is an enum, not a per-row value", src.Kind)
		return
	}
	if src.Value == "" {
		return // already reported as an empty constant
	}
	if !isEntityType(src.Value) {
		add(FieldEntityType, "%q is not an entity type — expected one of %s", src.Value, strings.Join(EntityTypes, ", "))
	}
}

func isTargetField(name string) bool {
	for _, f := range TargetFields {
		if f == name {
			return true
		}
	}
	return false
}

func isEntityType(name string) bool {
	for _, t := range EntityTypes {
		if t == name {
			return true
		}
	}
	return false
}
