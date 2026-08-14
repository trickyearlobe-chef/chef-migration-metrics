// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import (
	"fmt"
	"time"
)

// MaxSampleValues caps the de-duplicated sample values reported per column.
// Ten is enough to recognise what a column holds; identical samples tell the
// administrator nothing, which is why they are de-duplicated rather than taken
// from the first ten rows.
const MaxSampleValues = 10

// MaxDistinctTracked bounds the distinct values held per column while
// profiling.
//
// The set exists to tell a categorical column from a free one. On a
// consolidated export of several hundred thousand rows, columns like cookbook
// or owner name are nearly all-distinct, so an unbounded set is one retained
// string per row per column — hundreds of megabytes to compute a number
// nobody uses. Anything past this cap is definitively not categorical, which
// is the only question the count answers.
const MaxDistinctTracked = 10000

// ColumnProfile describes one source column.
type ColumnProfile struct {
	Name         string   `json:"name"`
	SampleValues []string `json:"sample_values"`

	// NonEmptyPct and DistinctCount are what let an administrator tell an
	// identifier column from a free-text one without opening the file.
	NonEmptyPct   float64 `json:"non_empty_pct"`
	DistinctCount int     `json:"distinct_count"`

	// DistinctCapped reports that counting stopped at MaxDistinctTracked, so
	// DistinctCount is a floor rather than a total. Without it the number
	// reads as exact and a reader would conclude the column has exactly that
	// many values.
	DistinctCapped bool `json:"distinct_capped"`
}

// SourceProfile is what the profile endpoint returns. It persists nothing.
type SourceProfile struct {
	Columns  []ColumnProfile `json:"columns"`
	RowCount int             `json:"row_count"`
	// DurationMS is how long reading the source took.
	//
	// Beside the row count it is a throughput, and that is what decides
	// whether a source of a given size can be read across a given link at all.
	// A customer whose read was killed part-way could not answer that question
	// from anything on screen, and it is the question that comes first.
	DurationMS    int64    `json:"duration_ms"`
	MalformedRows int      `json:"malformed_rows"`
	Warnings      []string `json:"warnings"`
}

// Profile reads a source to the end and describes its columns.
//
// Columns are reported in source order, because that is how the administrator
// sees them in their own tooling.
func Profile(src RowSource) (SourceProfile, error) {
	startedAt := time.Now()
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
			// Once the cap is reached the column is known not to be
			// categorical, so nothing further is retained.
			if len(distinct[i]) >= MaxDistinctTracked {
				continue
			}
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
			Name:           name,
			SampleValues:   samples[i],
			NonEmptyPct:    pct,
			DistinctCount:  len(distinct[i]),
			DistinctCapped: len(distinct[i]) >= MaxDistinctTracked,
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

	profile.DurationMS = time.Since(startedAt).Milliseconds()
	return profile, nil
}
