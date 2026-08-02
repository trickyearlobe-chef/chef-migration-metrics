// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import (
	"reflect"
	"strings"
	"testing"
)

func drain(t *testing.T, src RowSource) []Row {
	t.Helper()
	var rows []Row
	for src.Next() {
		r := src.Row()
		// Row() must return a value the caller can keep. If the source reuses
		// its backing map, the slice below ends up full of identical rows.
		copied := make(map[string]string, len(r.Values))
		for k, v := range r.Values {
			copied[k] = v
		}
		r.Values = copied
		rows = append(rows, r)
	}
	if err := src.Err(); err != nil {
		t.Fatalf("source error: %v", err)
	}
	return rows
}

func TestCSVSource_ColumnsPreserveSourceOrder(t *testing.T) {
	// Source order is what the administrator sees in their own tooling, so
	// profiling must present columns the same way. A map cannot express this.
	in := "zeta,alpha,middle,beta\n1,2,3,4\n"
	src, err := NewCSVSource(strings.NewReader(in), ',')
	if err != nil {
		t.Fatalf("NewCSVSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	want := []string{"zeta", "alpha", "middle", "beta"}
	if got := src.Columns(); !reflect.DeepEqual(got, want) {
		t.Errorf("Columns() = %v, want %v", got, want)
	}
}

func TestCSVSource_YieldsRowsKeyedByHeader(t *testing.T) {
	in := "owner,repo\nalice,web-app\nbob,db-tools\n"
	src, err := NewCSVSource(strings.NewReader(in), ',')
	if err != nil {
		t.Fatalf("NewCSVSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	rows := drain(t, src)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Values["owner"] != "alice" || rows[0].Values["repo"] != "web-app" {
		t.Errorf("row 1 = %v", rows[0].Values)
	}
	if rows[1].Values["owner"] != "bob" || rows[1].Values["repo"] != "db-tools" {
		t.Errorf("row 2 = %v", rows[1].Values)
	}
}

// The match report is keyed by source_row, so the source owns the numbering.
// It is 1-based over data rows, with the header not counted.
func TestCSVSource_NumbersDataRowsFromOne(t *testing.T) {
	in := "owner\na\nb\nc\n"
	src, err := NewCSVSource(strings.NewReader(in), ',')
	if err != nil {
		t.Fatalf("NewCSVSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	rows := drain(t, src)
	for i, r := range rows {
		if r.Number != i+1 {
			t.Errorf("row %d has Number %d, want %d", i, r.Number, i+1)
		}
	}
}

// A ragged row is a per-row fact, not a reason to abandon the import. The whole
// point of this path is that it succeeds while the source data is inconsistent.
func TestCSVSource_RaggedRowsAreFlaggedNotFatal(t *testing.T) {
	in := "owner,repo,org\n" +
		"alice,web-app,acme\n" +
		"bob,db-tools\n" + // short
		"carol,api,acme,extra\n" + // long
		"dave,cli,acme\n"
	src, err := NewCSVSource(strings.NewReader(in), ',')
	if err != nil {
		t.Fatalf("NewCSVSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	rows := drain(t, src)
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4 — a ragged row must not stop iteration", len(rows))
	}
	if rows[0].Malformed || rows[3].Malformed {
		t.Errorf("well-formed rows marked malformed: %v %v", rows[0], rows[3])
	}
	if !rows[1].Malformed {
		t.Error("short row not marked malformed")
	}
	if !rows[2].Malformed {
		t.Error("long row not marked malformed")
	}
	// A short row still yields the cells it does have; the missing ones are
	// empty. Discarding the whole row would lose recoverable data.
	if rows[1].Values["owner"] != "bob" || rows[1].Values["repo"] != "db-tools" {
		t.Errorf("short row lost its present cells: %v", rows[1].Values)
	}
	if rows[1].Values["org"] != "" {
		t.Errorf("short row org = %q, want empty", rows[1].Values["org"])
	}
	// A long row keeps its mapped cells; the surplus has nowhere to go.
	if rows[2].Values["owner"] != "carol" || rows[2].Values["org"] != "acme" {
		t.Errorf("long row lost its mapped cells: %v", rows[2].Values)
	}
}

func TestCSVSource_EmptyInputIsAnError(t *testing.T) {
	if _, err := NewCSVSource(strings.NewReader(""), ','); err == nil {
		t.Error("NewCSVSource on empty input = nil error, want an error")
	}
}

func TestCSVSource_HeaderOnlyYieldsNoRows(t *testing.T) {
	src, err := NewCSVSource(strings.NewReader("owner,repo\n"), ',')
	if err != nil {
		t.Fatalf("NewCSVSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	if rows := drain(t, src); len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
	if got := src.Columns(); len(got) != 2 {
		t.Errorf("Columns() = %v, want 2 columns", got)
	}
}

func TestCSVSource_RespectsDelimiter(t *testing.T) {
	src, err := NewCSVSource(strings.NewReader("owner;repo\nalice;web-app\n"), ';')
	if err != nil {
		t.Fatalf("NewCSVSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	if got := src.Columns(); !reflect.DeepEqual(got, []string{"owner", "repo"}) {
		t.Fatalf("Columns() = %v", got)
	}
	rows := drain(t, src)
	if len(rows) != 1 || rows[0].Values["repo"] != "web-app" {
		t.Errorf("rows = %v", rows)
	}
}

func TestCSVSource_StripsUTF8BOMFromFirstHeader(t *testing.T) {
	// Spreadsheet exports routinely carry a BOM. Left in place it becomes part
	// of the first column's name, so every mapping onto that column fails
	// validation with a message that looks identical to a correct one.
	src, err := NewCSVSource(strings.NewReader("\ufeffowner,repo\nalice,web-app\n"), ',')
	if err != nil {
		t.Fatalf("NewCSVSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	if got := src.Columns()[0]; got != "owner" {
		t.Errorf("first column = %q, want %q", got, "owner")
	}
}

func TestCSVSource_DuplicateHeadersAreAnError(t *testing.T) {
	// One name cannot key two columns, and silently keeping the last would
	// discard a column the administrator can see in their file.
	if _, err := NewCSVSource(strings.NewReader("owner,repo,owner\na,b,c\n"), ','); err == nil {
		t.Error("duplicate header = nil error, want an error")
	}
}

func TestCSVSource_QuotedFieldsAndEmbeddedNewlines(t *testing.T) {
	in := "owner,notes\n" + "alice,\"line one\nline two\"\n" + "bob,\"has, a comma\"\n"
	src, err := NewCSVSource(strings.NewReader(in), ',')
	if err != nil {
		t.Fatalf("NewCSVSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	rows := drain(t, src)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Values["notes"] != "line one\nline two" {
		t.Errorf("embedded newline lost: %q", rows[0].Values["notes"])
	}
	if rows[1].Values["notes"] != "has, a comma" {
		t.Errorf("quoted comma lost: %q", rows[1].Values["notes"])
	}
	// An embedded newline must not desynchronise the report's row numbering
	// from what the administrator counts as a record.
	if rows[1].Number != 2 {
		t.Errorf("row after an embedded newline numbered %d, want 2", rows[1].Number)
	}
}

func TestDetectDelimiter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want rune
	}{
		{"comma", "owner,repo,org\nalice,web-app,acme\n", ','},
		{"semicolon", "owner;repo;org\nalice;web-app;acme\n", ';'},
		{"tab", "owner\trepo\torg\nalice\tweb-app\tacme\n", '\t'},
		{"pipe", "owner|repo|org\nalice|web-app|acme\n", '|'},
		{"single column defaults to comma", "owner\nalice\nbob\n", ','},
		{
			// Consistency, not frequency, is the discriminator. Commas appear
			// far more often here, but only the semicolon yields the same field
			// count on every line.
			"prose full of commas does not beat a consistent semicolon",
			"owner;notes\n" +
				"alice;one, two, three, four, five\n" +
				"bob;six, seven\n" +
				"carol;eight, nine, ten, eleven\n",
			';',
		},
		{
			"a delimiter inside quotes does not count",
			"owner,notes\n" + "alice,\"a;b;c;d;e\"\n" + "bob,\"f;g;h;i;j\"\n",
			',',
		},
		{"CRLF line endings", "owner;repo\r\nalice;web-app\r\n", ';'},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectDelimiter([]byte(tt.in)); got != tt.want {
				t.Errorf("DetectDelimiter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectDelimiter_NeverReturnsZero(t *testing.T) {
	// Detection is advisory and always overridable, so it must always return
	// something usable rather than failing and blocking the administrator.
	for _, in := range []string{"", "\n", "   ", "no delimiters at all"} {
		if got := DetectDelimiter([]byte(in)); got == 0 {
			t.Errorf("DetectDelimiter(%q) returned the zero rune", in)
		}
	}
}
