// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func profileCSV(t *testing.T, in string) SourceProfile {
	t.Helper()
	src, err := NewCSVSource(strings.NewReader(in), ',')
	if err != nil {
		t.Fatalf("NewCSVSource: %v", err)
	}
	defer func() { _ = src.Close() }()

	p, err := Profile(src)
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}
	return p
}

func columnNamed(t *testing.T, p SourceProfile, name string) ColumnProfile {
	t.Helper()
	for _, c := range p.Columns {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no column %q in profile", name)
	return ColumnProfile{}
}

func TestProfile_ReportsColumnsInSourceOrder(t *testing.T) {
	p := profileCSV(t, "zeta,alpha,middle\n1,2,3\n")
	var got []string
	for _, c := range p.Columns {
		got = append(got, c.Name)
	}
	want := []string{"zeta", "alpha", "middle"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("column order = %v, want %v", got, want)
		}
	}
}

func TestProfile_CountsRows(t *testing.T) {
	p := profileCSV(t, "owner\na\nb\nc\nd\n")
	if p.RowCount != 4 {
		t.Errorf("RowCount = %d, want 4", p.RowCount)
	}
}

// Fill rate and distinct count are what let an administrator tell an identifier
// column from a free-text one without opening the file.
func TestProfile_FillRateAndDistinctCount(t *testing.T) {
	in := "id,mostly_empty,constant\n" +
		"a,x,same\n" +
		"b,,same\n" +
		"c,,same\n" +
		"d,,same\n"
	p := profileCSV(t, in)

	id := columnNamed(t, p, "id")
	if id.NonEmptyPct != 100 {
		t.Errorf("id NonEmptyPct = %v, want 100", id.NonEmptyPct)
	}
	if id.DistinctCount != 4 {
		t.Errorf("id DistinctCount = %d, want 4", id.DistinctCount)
	}

	sparse := columnNamed(t, p, "mostly_empty")
	if math.Abs(sparse.NonEmptyPct-25) > 0.001 {
		t.Errorf("mostly_empty NonEmptyPct = %v, want 25", sparse.NonEmptyPct)
	}

	constant := columnNamed(t, p, "constant")
	if constant.DistinctCount != 1 {
		t.Errorf("constant DistinctCount = %d, want 1", constant.DistinctCount)
	}
}

func TestProfile_EmptyCellsAreNotDistinctValues(t *testing.T) {
	p := profileCSV(t, "col\na\n\nb\n\n")
	c := columnNamed(t, p, "col")
	if c.DistinctCount != 2 {
		t.Errorf("DistinctCount = %d, want 2 — empty is absence, not a value", c.DistinctCount)
	}
}

func TestProfile_SampleValuesAreDedupedAndCapped(t *testing.T) {
	var b strings.Builder
	b.WriteString("col\n")
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, "value-%d\n", i)
	}
	c := columnNamed(t, profileCSV(t, b.String()), "col")
	if len(c.SampleValues) != MaxSampleValues {
		t.Errorf("len(SampleValues) = %d, want %d", len(c.SampleValues), MaxSampleValues)
	}

	// Ten identical cells are one sample, not ten. A column of samples that all
	// read the same tells the administrator nothing about the column.
	repeated := "col\n" + strings.Repeat("same\n", 20)
	c = columnNamed(t, profileCSV(t, repeated), "col")
	if len(c.SampleValues) != 1 {
		t.Errorf("SampleValues = %v, want one deduplicated value", c.SampleValues)
	}
}

func TestProfile_SampleValuesExcludeEmptyCells(t *testing.T) {
	c := columnNamed(t, profileCSV(t, "col\n\na\n\nb\n"), "col")
	for _, v := range c.SampleValues {
		if v == "" {
			t.Fatalf("SampleValues %v contains an empty cell", c.SampleValues)
		}
	}
}

func TestProfile_HeaderOnlyFileIsNotAnError(t *testing.T) {
	p := profileCSV(t, "owner,repo\n")
	if p.RowCount != 0 {
		t.Errorf("RowCount = %d, want 0", p.RowCount)
	}
	if len(p.Columns) != 2 {
		t.Fatalf("Columns = %v, want 2", p.Columns)
	}
	for _, c := range p.Columns {
		if c.NonEmptyPct != 0 {
			t.Errorf("%s NonEmptyPct = %v, want 0 with no rows", c.Name, c.NonEmptyPct)
		}
	}
	if len(p.Warnings) == 0 {
		t.Error("a file with no data rows should warn — it is almost always the wrong file")
	}
}

func TestProfile_WarnsAboutMalformedRows(t *testing.T) {
	in := "owner,repo,org\n" +
		"alice,web-app,acme\n" +
		"bob,db-tools\n" +
		"carol,api,acme\n"
	p := profileCSV(t, in)
	if p.MalformedRows != 1 {
		t.Errorf("MalformedRows = %d, want 1", p.MalformedRows)
	}
	if len(p.Warnings) == 0 {
		t.Error("a ragged file should warn — it usually means the wrong delimiter")
	}
	if p.RowCount != 3 {
		t.Errorf("RowCount = %d, want 3 — a ragged row is still a row", p.RowCount)
	}
}

// Profiling reads the source to the end. It must surface a mid-stream failure
// rather than returning a short profile that looks like a small file.
func TestProfile_PropagatesSourceErrors(t *testing.T) {
	src := &failingSource{
		columns: []string{"owner"},
		rows: []Row{
			{Number: 1, Values: map[string]string{"owner": "alice"}},
		},
		err: errBoom,
	}
	if _, err := Profile(src); err == nil {
		t.Error("Profile() = nil error, want the source's error")
	}
}

// Profiling holds a set of distinct values per column so it can report
// cardinality. On a consolidated export — hundreds of thousands of rows, and
// columns like cookbook and owner name where nearly every value is different —
// that set is one string per row per column, which is the unbounded allocation
// this file's own comment warns about.
//
// The cap costs nothing that matters: the number exists to tell a categorical
// column from a free one, and anything past the cap is definitively not
// categorical, so its exact count is not a fact anybody uses.
func TestProfile_BoundsDistinctTracking(t *testing.T) {
	var b strings.Builder
	b.WriteString("category,unique_id\n")
	for i := 0; i < MaxDistinctTracked+500; i++ {
		fmt.Fprintf(&b, "owner,value-%d\n", i)
	}

	src, err := NewCSVSource(strings.NewReader(b.String()), ',')
	if err != nil {
		t.Fatalf("NewCSVSource: %v", err)
	}
	profile, err := Profile(src)
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}

	if profile.RowCount != MaxDistinctTracked+500 {
		t.Errorf("row_count = %d — every row is still counted", profile.RowCount)
	}

	byName := map[string]ColumnProfile{}
	for _, c := range profile.Columns {
		byName[c.Name] = c
	}

	// The low-cardinality column is the one somebody filters on, and it must
	// still report exactly.
	if got := byName["category"]; got.DistinctCount != 1 || got.DistinctCapped {
		t.Errorf("category: distinct=%d capped=%v, want an exact 1", got.DistinctCount, got.DistinctCapped)
	}

	// The high-cardinality one stops counting and says so, rather than
	// reporting a number that took a gigabyte to produce.
	high := byName["unique_id"]
	if high.DistinctCount != MaxDistinctTracked {
		t.Errorf("unique_id: distinct=%d, want it held at %d", high.DistinctCount, MaxDistinctTracked)
	}
	if !high.DistinctCapped {
		t.Error("unique_id: the count is capped but does not say so, which reads as an exact figure")
	}
}
