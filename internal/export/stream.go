// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package export streams the current filtered list of a list-view entity
// (nodes, cookbooks, roles, git repos) to CSV, JSON, or — for nodes — a Chef
// search query string. Rows are produced by a RowSource one page at a time and
// written straight to an io.Writer, so a full 120k-node export never holds the
// whole result set in memory. See specifications/web-api-exports.md.
package export

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Column defines one export column. Header is used verbatim as the CSV header
// and the JSON object key, so CSV and JSON always carry the same columns in the
// same order — one source of truth. Value extracts the cell from a row; it may
// return a string, a number, a bool, a *int/*bool (nil → empty/null), a
// time.Time (zero → empty/null), or a json.RawMessage (embedded raw in JSON,
// compacted to a string in CSV).
type Column struct {
	Header string
	Value  func(row any) any
}

// RowSource yields export rows one page at a time. Next returns the next page,
// or an empty slice when the stream is exhausted. Holding only one page bounds
// memory regardless of total row count.
type RowSource interface {
	Next(ctx context.Context) ([]any, error)
}

// StreamCSV writes the header then one record per row from src to w, flushing
// per page. It returns the number of data rows written.
func StreamCSV(ctx context.Context, w io.Writer, cols []Column, src RowSource) (int, error) {
	cw := csv.NewWriter(w)

	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = c.Header
	}
	if err := cw.Write(header); err != nil {
		return 0, fmt.Errorf("export: writing CSV header: %w", err)
	}

	count := 0
	rec := make([]string, len(cols))
	for {
		rows, err := src.Next(ctx)
		if err != nil {
			return count, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			for i, c := range cols {
				rec[i] = csvCell(c.Value(row))
			}
			if err := cw.Write(rec); err != nil {
				return count, fmt.Errorf("export: writing CSV row: %w", err)
			}
			count++
		}
		cw.Flush()
		if err := cw.Error(); err != nil {
			return count, fmt.Errorf("export: flushing CSV: %w", err)
		}
	}
	cw.Flush()
	return count, cw.Error()
}

// StreamJSON writes a JSON array of objects from src to w, preserving column
// order in each object. It returns the number of objects written.
func StreamJSON(ctx context.Context, w io.Writer, cols []Column, src RowSource) (int, error) {
	bw := bufio.NewWriter(w)

	// put/putBytes accumulate the first write error; subsequent calls are no-ops
	// (mirroring bufio's sticky error) so we can write freely and check werr at
	// page boundaries and before Flush.
	var werr error
	put := func(s string) {
		if werr == nil {
			_, werr = bw.WriteString(s)
		}
	}
	putBytes := func(b []byte) {
		if werr == nil {
			_, werr = bw.Write(b)
		}
	}

	put("[")

	// Pre-marshal the keys once.
	keys := make([]string, len(cols))
	for i, c := range cols {
		kb, _ := json.Marshal(c.Header)
		keys[i] = string(kb)
	}

	count := 0
	for {
		rows, err := src.Next(ctx)
		if err != nil {
			return count, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			if count > 0 {
				put(",")
			}
			put("\n  {")
			for i, c := range cols {
				if i > 0 {
					put(",")
				}
				put(keys[i])
				put(":")
				vb, err := json.Marshal(c.Value(row))
				if err != nil {
					return count, fmt.Errorf("export: marshalling JSON cell %q: %w", c.Header, err)
				}
				putBytes(vb)
			}
			put("}")
			count++
		}
		if werr != nil {
			return count, werr
		}
	}
	if count > 0 {
		put("\n")
	}
	put("]\n")
	if werr != nil {
		return count, werr
	}
	return count, bw.Flush()
}

// StreamChefSearchQuery writes "name:<v1> OR name:<v2> ..." using name(row) as
// the per-row node name (spaces escaped for Chef search syntax). Rows with an
// empty name are skipped. It returns the number of names written.
func StreamChefSearchQuery(ctx context.Context, w io.Writer, name func(row any) string, src RowSource) (int, error) {
	bw := bufio.NewWriter(w)
	var werr error
	put := func(s string) {
		if werr == nil {
			_, werr = bw.WriteString(s)
		}
	}
	count := 0
	for {
		rows, err := src.Next(ctx)
		if err != nil {
			return count, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			n := name(row)
			if n == "" {
				continue
			}
			if count > 0 {
				put(" OR ")
			}
			put("name:")
			put(strings.ReplaceAll(n, " ", `\ `))
			count++
		}
		if werr != nil {
			return count, werr
		}
	}
	if count > 0 {
		put("\n")
	}
	if werr != nil {
		return count, werr
	}
	return count, bw.Flush()
}

// csvCell renders a Value result as a CSV string cell.
func csvCell(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []string:
		// Array column joined into one cell. Comma matches the multi-value
		// query-param convention (e.g. ?tags=a,b); encoding/csv auto-quotes the
		// cell if any element itself contains a comma. Empty slice → "".
		return strings.Join(t, ",")
	case json.RawMessage:
		if len(t) == 0 || string(t) == "null" {
			return ""
		}
		return string(t)
	case bool:
		return strconv.FormatBool(t)
	case *bool:
		if t == nil {
			return ""
		}
		return strconv.FormatBool(*t)
	case int:
		return strconv.Itoa(t)
	case *int:
		if t == nil {
			return ""
		}
		return strconv.Itoa(*t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case time.Time:
		if t.IsZero() {
			return ""
		}
		return t.Format(time.RFC3339)
	default:
		return fmt.Sprint(t)
	}
}

// SliceSource is a RowSource backed by a single in-memory slice. It yields the
// whole slice on the first Next call and then signals completion. Useful for
// small entities (roles, cookbooks, git repos) that are fetched at once, and
// for tests.
type SliceSource struct {
	rows []any
	done bool
}

// NewSliceSource wraps an already-materialised slice of rows.
func NewSliceSource(rows []any) *SliceSource {
	return &SliceSource{rows: rows}
}

// Next returns the whole slice once, then empty.
func (s *SliceSource) Next(context.Context) ([]any, error) {
	if s.done {
		return nil, nil
	}
	s.done = true
	return s.rows, nil
}
