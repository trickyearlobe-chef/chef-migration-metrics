// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import (
	"errors"
	"strings"
)

// MappedRow is one source row after the mapping has been applied, before any
// database lookup. The matcher in internal/webapi takes it from here.
type MappedRow struct {
	// SourceRow is the 1-based row number the match report is keyed by.
	SourceRow int `json:"source_row"`

	// Malformed carries the source's structural verdict through to the report,
	// whether or not the row was rejected.
	Malformed bool `json:"malformed"`

	// Raw holds each target field's value as the source produced it, before
	// transforms, so the administrator can see what their mapping did to each
	// row without re-reading the file.
	Raw map[string]string `json:"raw"`

	Owner string `json:"owner"`

	// OwnerRaw is the owner value after its transform chain but before
	// slugification. It is what fuzzy matching and future imports compare
	// against, and it is seeded as a custom alias.
	OwnerRaw string `json:"owner_raw"`

	EntityType   string `json:"entity_type"`
	EntityKey    string `json:"entity_key"`
	Organisation string `json:"organisation"`
	Notes        string `json:"notes"`
	DisplayName  string `json:"display_name"`

	// RejectedReason is empty when the row mapped cleanly.
	RejectedReason string `json:"rejected_reason,omitempty"`
}

// Mapper applies a validated mapping document to rows. It holds no per-row
// state, so one mapper serves a whole import.
type Mapper struct {
	fields map[string]compiledField
}

type compiledField struct {
	source Source
	chain  CompiledChain
}

// NewMapper validates the document against the source's columns and compiles
// its transforms. Every fault is reported here rather than per row.
func NewMapper(fm FieldMap, columns []string) (*Mapper, error) {
	if errs := fm.Validate(columns); len(errs) > 0 {
		messages := make([]string, len(errs))
		for i, e := range errs {
			messages[i] = e.Error()
		}
		return nil, errors.New("ownershipimport: invalid mapping — " + strings.Join(messages, "; "))
	}

	fields := make(map[string]compiledField, len(fm))
	for name, mapping := range fm {
		chain, err := CompileTransforms(mapping.Transforms)
		if err != nil {
			// Unreachable — Validate compiles the same chains — but a mapper
			// that silently dropped a chain would produce plausible wrong
			// values rather than an error.
			return nil, errors.New("ownershipimport: " + name + ": " + err.Error())
		}
		fields[name] = compiledField{source: mapping.Source, chain: chain}
	}
	return &Mapper{fields: fields}, nil
}

// MapRow applies the mapping to one row. It never returns an error: a row that
// cannot be mapped carries a rejection reason instead, because an import that
// stops at the first bad row cannot report what else was wrong with the file.
func (m *Mapper) MapRow(row Row) MappedRow {
	out := MappedRow{
		SourceRow: row.Number,
		Malformed: row.Malformed,
		Raw:       make(map[string]string, len(m.fields)),
	}

	mapped := make(map[string]string, len(m.fields))
	for name, field := range m.fields {
		raw := evaluateSource(field.source, row.Values)
		out.Raw[name] = raw
		mapped[name] = field.chain.Apply(raw)
	}

	out.OwnerRaw = mapped[FieldOwner]
	out.EntityType = mapped[FieldEntityType]
	out.EntityKey = mapped[FieldEntityKey]
	out.Organisation = mapped[FieldOrganisation]
	out.Notes = mapped[FieldNotes]

	// display_name receives the value before slugification, and absent an
	// explicit mapping it defaults to the owner field's pre-slugify output
	// rather than to any column. That is what preserves the original string.
	if explicit, ok := mapped[FieldDisplayName]; ok && explicit != "" {
		out.DisplayName = explicit
	} else {
		out.DisplayName = out.OwnerRaw
	}

	// A malformation that cost a required field is reported as the cause.
	// Calling it a missing field would send the administrator to the mapping
	// when the fault is in the file.
	missingReason := ReasonMissingRequiredField
	if row.Malformed {
		missingReason = ReasonMalformedRow
	}

	if strings.TrimSpace(out.OwnerRaw) == "" || strings.TrimSpace(out.EntityKey) == "" {
		out.RejectedReason = missingReason
		return out
	}

	slug, err := SlugifyOwnerName(out.OwnerRaw)
	if err != nil {
		// A value that is entirely punctuation is neither a missing field nor a
		// malformed row. Folding it into missing_required_field would hide the
		// most actionable class of import miss behind the least actionable one.
		out.RejectedReason = ReasonInvalidOwnerName
		return out
	}
	out.Owner = slug

	return out
}

func evaluateSource(src Source, cells map[string]string) string {
	switch src.Kind {
	case SourceColumn:
		return cells[src.Column]

	case SourceConstant:
		return src.Value

	case SourceConcat:
		// An empty cell contributes an empty segment and separators are not
		// collapsed, so a missing middle component is visible rather than
		// silently closing up.
		parts := make([]string, len(src.Columns))
		for i, c := range src.Columns {
			parts[i] = cells[c]
		}
		return strings.Join(parts, src.Separator)

	default:
		return ""
	}
}
