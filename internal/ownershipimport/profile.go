// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import "fmt"

// MaxSampleValues caps the de-duplicated sample values reported per column.
// Ten is enough to recognise what a column holds; identical samples tell the
// administrator nothing, which is why they are de-duplicated rather than taken
// from the first ten rows.
const MaxSampleValues = 10

// ColumnProfile describes one source column.
type ColumnProfile struct {
	Name         string   `json:"name"`
	SampleValues []string `json:"sample_values"`

	// NonEmptyPct and DistinctCount are what let an administrator tell an
	// identifier column from a free-text one without opening the file.
	NonEmptyPct   float64 `json:"non_empty_pct"`
	DistinctCount int     `json:"distinct_count"`
}

// SourceProfile is what the profile endpoint returns. It persists nothing.
type SourceProfile struct {
	Columns       []ColumnProfile `json:"columns"`
	RowCount      int             `json:"row_count"`
	MalformedRows int             `json:"malformed_rows"`
	Warnings      []string        `json:"warnings"`
}

// Profile reads a source to the end and describes its columns.
//
// Columns are reported in source order, because that is how the administrator
// sees them in their own tooling.
func Profile(src RowSource) (SourceProfile, error) {
	columns := src.Columns()

	distinct := make([]map[string]bool, len(columns))
	samples := make([][]string, len(columns))
	nonEmpty := make([]int, len(columns))
	for i := range columns {
		distinct[i] = make(map[string]bool)
	}

	profile := SourceProfile{Warnings: []string{}}

	for src.Next() {
		row := src.Row()
		profile.RowCount++
		if row.Malformed {
			profile.MalformedRows++
		}

		for i, name := range columns {
			value := row.Values[name]
			// An empty cell is an absence, not a value: counting it as one
			// would make a mostly-blank column look well populated.
			if value == "" {
				continue
			}
			nonEmpty[i]++
			if !distinct[i][value] {
				distinct[i][value] = true
				if len(samples[i]) < MaxSampleValues {
					samples[i] = append(samples[i], value)
				}
			}
		}
	}

	// A source that failed part way through must not be mistaken for a short
	// one — a truncated profile reads exactly like a small file.
	if err := src.Err(); err != nil {
		return SourceProfile{}, fmt.Errorf("ownershipimport: profiling the source: %w", err)
	}

	profile.Columns = make([]ColumnProfile, len(columns))
	for i, name := range columns {
		var pct float64
		if profile.RowCount > 0 {
			pct = float64(nonEmpty[i]) / float64(profile.RowCount) * 100
		}
		if samples[i] == nil {
			samples[i] = []string{}
		}
		profile.Columns[i] = ColumnProfile{
			Name:          name,
			SampleValues:  samples[i],
			NonEmptyPct:   pct,
			DistinctCount: len(distinct[i]),
		}
	}

	if profile.RowCount == 0 {
		profile.Warnings = append(profile.Warnings,
			"The source has a header but no data rows. That is almost always the wrong file, or the wrong delimiter.")
	}
	if profile.MalformedRows > 0 {
		profile.Warnings = append(profile.Warnings, fmt.Sprintf(
			"%d of %d rows do not have %d fields. That usually means the delimiter is wrong, or some rows are quoted inconsistently.",
			profile.MalformedRows, profile.RowCount, len(columns)))
	}
	for _, c := range profile.Columns {
		if profile.RowCount > 0 && c.NonEmptyPct == 0 {
			profile.Warnings = append(profile.Warnings, fmt.Sprintf(
				"Column %q is empty in every row.", c.Name))
		}
	}

	return profile, nil
}
