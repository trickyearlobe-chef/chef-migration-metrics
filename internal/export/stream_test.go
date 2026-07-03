// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
)

type row struct {
	Name string
	Free *int
	Deps json.RawMessage
}

func testColumns() []Column {
	return []Column{
		{Header: "name", Value: func(r any) any { return r.(row).Name }},
		{Header: "available_disk_mb", Value: func(r any) any { return r.(row).Free }},
		{Header: "dependencies", Value: func(r any) any { return r.(row).Deps }},
	}
}

func sampleRows() []any {
	five := 5
	return []any{
		row{Name: "web-01", Free: &five, Deps: json.RawMessage(`{"apt":">=1.0"}`)},
		row{Name: "db 02", Free: nil, Deps: nil},
	}
}

func TestStreamCSV_HeaderAndRows(t *testing.T) {
	var buf bytes.Buffer
	n, err := StreamCSV(context.Background(), &buf, testColumns(), NewSliceSource(sampleRows()))
	if err != nil {
		t.Fatalf("StreamCSV: %v", err)
	}
	if n != 2 {
		t.Fatalf("row count = %d, want 2", n)
	}
	recs, err := csv.NewReader(strings.NewReader(buf.String())).ReadAll()
	if err != nil {
		t.Fatalf("parsing CSV: %v", err)
	}
	if got, want := recs[0], []string{"name", "available_disk_mb", "dependencies"}; !equal(got, want) {
		t.Errorf("header = %v, want %v", got, want)
	}
	if got, want := recs[1], []string{"web-01", "5", `{"apt":">=1.0"}`}; !equal(got, want) {
		t.Errorf("row 1 = %v, want %v", got, want)
	}
	// nil *int and nil RawMessage render as empty cells.
	if got, want := recs[2], []string{"db 02", "", ""}; !equal(got, want) {
		t.Errorf("row 2 = %v, want %v", got, want)
	}
}

func TestStreamJSON_OrderedObjects(t *testing.T) {
	var buf bytes.Buffer
	n, err := StreamJSON(context.Background(), &buf, testColumns(), NewSliceSource(sampleRows()))
	if err != nil {
		t.Fatalf("StreamJSON: %v", err)
	}
	if n != 2 {
		t.Fatalf("row count = %d, want 2", n)
	}
	var out []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	// Typed values survive: number stays a number, JSONB stays nested.
	if out[0]["available_disk_mb"] != float64(5) {
		t.Errorf("available_disk_mb = %v (%T), want 5", out[0]["available_disk_mb"], out[0]["available_disk_mb"])
	}
	if deps, ok := out[0]["dependencies"].(map[string]any); !ok || deps["apt"] != ">=1.0" {
		t.Errorf("dependencies should be nested JSON, got %v", out[0]["dependencies"])
	}
	// Column order preserved in the raw text.
	first := buf.String()[strings.Index(buf.String(), "{"):]
	if strings.Index(first, `"name"`) > strings.Index(first, `"available_disk_mb"`) {
		t.Errorf("JSON key order not preserved:\n%s", first)
	}
}

func TestStreamChefSearchQuery_EscapesAndJoins(t *testing.T) {
	var buf bytes.Buffer
	name := func(r any) string { return r.(row).Name }
	n, err := StreamChefSearchQuery(context.Background(), &buf, name, NewSliceSource(sampleRows()))
	if err != nil {
		t.Fatalf("StreamChefSearchQuery: %v", err)
	}
	if n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
	got := strings.TrimSpace(buf.String())
	want := `name:web-01 OR name:db\ 02`
	if got != want {
		t.Errorf("chef search = %q, want %q", got, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
