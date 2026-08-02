// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package ownershipimport implements the discovery-driven ownership intake
// pipeline: reading a source of unknown shape, mapping its columns onto CMM's
// ownership fields, and classifying what each row would do.
//
// Everything in this package is pure. It performs no database access, so the
// mapping and reporting logic is testable without a database. The DB-backed
// matcher that resolves owners and entities lives in internal/webapi and takes
// this package's output as its input.
//
// See specifications/ownership-intake.md.
package ownershipimport

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Row is one record from a source.
type Row struct {
	// Number is 1-based over data rows, with any header not counted. The match
	// report is keyed by it, so the source owns the numbering — a consumer
	// counting its own iterations would drift from what the administrator sees
	// as soon as a record spans several lines.
	Number int

	// Values holds the row's cells keyed by column name. Absent columns are
	// present with an empty string; a source never omits a key it declared in
	// Columns.
	Values map[string]string

	// Malformed reports that the record did not have the shape the source
	// expected — a ragged CSV line, for instance. The cells that are present
	// are still populated: the point of this path is to succeed while the
	// source data is inconsistent, so a structurally odd row is reported, not
	// discarded.
	Malformed bool
}

// RowSource yields an ordered list of column names and then a sequence of rows
// keyed by those names.
//
// A source is single-pass and not re-readable. Profile, preview and commit each
// open their own source, because a future SQL source is a streaming cursor and
// cannot be rewound.
//
// Iteration ends when Next reports false. Err must then be consulted: a source
// that failed part way through must not be mistaken for a short one.
type RowSource interface {
	Columns() []string
	Next() bool
	Row() Row
	Err() error
	Close() error
}

// csvSource adapts a delimited text stream to RowSource.
type csvSource struct {
	reader  *csv.Reader
	closer  io.Closer
	columns []string
	current Row
	err     error
	rowNum  int
}

// NewCSVSource reads the header from r and returns a source over the remaining
// records. The delimiter is used verbatim; detection, where it happens at all,
// is the caller's business and always overridable.
func NewCSVSource(r io.Reader, delimiter rune) (RowSource, error) {
	if delimiter == 0 {
		delimiter = ','
	}

	cr := csv.NewReader(bufio.NewReader(r))
	cr.Comma = delimiter
	cr.TrimLeadingSpace = true
	// A ragged record is a per-row fact this package reports, not a parse
	// error that abandons the import.
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = false

	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return nil, errors.New("ownershipimport: the source is empty — there is no header row to read")
	}
	if err != nil {
		return nil, fmt.Errorf("ownershipimport: reading the header row: %w", err)
	}

	// Spreadsheet exports routinely carry a byte-order mark. Left in place it
	// becomes part of the first column's name, so every mapping onto that
	// column fails validation with a message indistinguishable from a correct
	// one.
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}

	seen := make(map[string]bool, len(header))
	columns := make([]string, len(header))
	for i, h := range header {
		name := strings.TrimSpace(h)
		if seen[name] {
			return nil, fmt.Errorf("ownershipimport: the source has two columns named %q — a name cannot key two columns", name)
		}
		seen[name] = true
		columns[i] = name
	}

	src := &csvSource{reader: cr, columns: columns}
	if c, ok := r.(io.Closer); ok {
		src.closer = c
	}
	return src, nil
}

func (s *csvSource) Columns() []string { return s.columns }

func (s *csvSource) Next() bool {
	if s.err != nil {
		return false
	}

	record, err := s.reader.Read()
	if errors.Is(err, io.EOF) {
		return false
	}
	if err != nil {
		s.err = fmt.Errorf("ownershipimport: reading row %d: %w", s.rowNum+1, err)
		return false
	}

	s.rowNum++
	values := make(map[string]string, len(s.columns))
	for i, name := range s.columns {
		if i < len(record) {
			values[name] = record[i]
			continue
		}
		values[name] = ""
	}

	s.current = Row{
		Number:    s.rowNum,
		Values:    values,
		Malformed: len(record) != len(s.columns),
	}
	return true
}

func (s *csvSource) Row() Row { return s.current }

func (s *csvSource) Err() error { return s.err }

func (s *csvSource) Close() error {
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

// candidateDelimiters is ordered by preference, which is also the tie-break when
// two candidates score identically.
var candidateDelimiters = []rune{',', ';', '\t', '|'}

// detectSampleLines is how many records DetectDelimiter examines. Enough to
// establish consistency, few enough that profiling a large file stays cheap.
const detectSampleLines = 20

// DetectDelimiter guesses which delimiter a sample uses.
//
// It is advisory. Every endpoint accepts an explicit delimiter that is used
// verbatim with no detection, so a wrong guess costs one field edit and never a
// failed or wrong import. It always returns a usable delimiter rather than
// failing, because a detection failure that blocks the administrator is worse
// than a guess they can override.
//
// Consistency across records is the discriminator, not raw frequency: prose in
// a free-text column defeats a frequency count and does not defeat a
// consistency check.
func DetectDelimiter(sample []byte) rune {
	best := ','
	bestFields := 1

	for _, candidate := range candidateDelimiters {
		fields, consistent := fieldShape(sample, candidate)
		if !consistent || fields <= 1 {
			continue
		}
		if fields > bestFields {
			best = candidate
			bestFields = fields
		}
	}
	return best
}

// fieldShape reports how many fields each sampled record yields under the given
// delimiter, and whether every record agreed.
func fieldShape(sample []byte, delimiter rune) (fields int, consistent bool) {
	cr := csv.NewReader(strings.NewReader(string(sample)))
	cr.Comma = delimiter
	cr.FieldsPerRecord = -1
	// A candidate that is not the real delimiter routinely splits mid-quote.
	// That is evidence about the candidate, not a reason to abandon the sample.
	cr.LazyQuotes = true

	count := -1
	for i := 0; i < detectSampleLines; i++ {
		record, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, false
		}
		if count == -1 {
			count = len(record)
			continue
		}
		if len(record) != count {
			return 0, false
		}
	}

	if count == -1 {
		return 0, false
	}
	return count, true
}
