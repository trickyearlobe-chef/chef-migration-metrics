// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"strings"
	"testing"
	"time"
)

func TestBuildLogEntryFilterWhere_NoFilters(t *testing.T) {
	where, args := buildLogEntryFilterWhere(LogEntryFilter{})
	if where != " WHERE 1=1" {
		t.Errorf("where = %q, want %q", where, " WHERE 1=1")
	}
	if len(args) != 0 {
		t.Errorf("args len = %d, want 0", len(args))
	}
}

func TestBuildLogEntryFilterWhere_Scope(t *testing.T) {
	where, args := buildLogEntryFilterWhere(LogEntryFilter{Scope: "collection"})
	if !strings.Contains(where, "AND scope = $1") {
		t.Errorf("where missing scope clause: %s", where)
	}
	if len(args) != 1 || args[0] != "collection" {
		t.Errorf("args = %v, want [collection]", args)
	}
}

func TestBuildLogEntryFilterWhere_Severity(t *testing.T) {
	where, args := buildLogEntryFilterWhere(LogEntryFilter{Severity: "ERROR"})
	if !strings.Contains(where, "AND severity = $1") {
		t.Errorf("where missing severity clause: %s", where)
	}
	if len(args) != 1 || args[0] != "ERROR" {
		t.Errorf("args = %v, want [ERROR]", args)
	}
}

func TestBuildLogEntryFilterWhere_MinSeverity(t *testing.T) {
	where, args := buildLogEntryFilterWhere(LogEntryFilter{MinSeverity: "WARN"})
	if !strings.Contains(where, "AND severity = ANY($1)") {
		t.Errorf("where missing min_severity clause: %s", where)
	}
	if len(args) != 1 {
		t.Fatalf("args len = %d, want 1", len(args))
	}
}

func TestBuildLogEntryFilterWhere_SeverityPrecedesMinSeverity(t *testing.T) {
	where, args := buildLogEntryFilterWhere(LogEntryFilter{
		Severity:    "ERROR",
		MinSeverity: "WARN",
	})
	// Exact severity takes precedence — MinSeverity should be ignored.
	if !strings.Contains(where, "AND severity = $1") {
		t.Errorf("where missing severity clause: %s", where)
	}
	if strings.Contains(where, "ANY") {
		t.Errorf("where should not contain ANY when Severity is set: %s", where)
	}
	if len(args) != 1 || args[0] != "ERROR" {
		t.Errorf("args = %v, want [ERROR]", args)
	}
}

func TestBuildLogEntryFilterWhere_MinSeverity_InvalidIgnored(t *testing.T) {
	where, args := buildLogEntryFilterWhere(LogEntryFilter{MinSeverity: "BOGUS"})
	// Invalid min severity should not add any clause.
	if where != " WHERE 1=1" {
		t.Errorf("where = %q, want %q", where, " WHERE 1=1")
	}
	if len(args) != 0 {
		t.Errorf("args len = %d, want 0", len(args))
	}
}

func TestBuildLogEntryFilterWhere_Organisation(t *testing.T) {
	where, args := buildLogEntryFilterWhere(LogEntryFilter{Organisation: "prod"})
	if !strings.Contains(where, "AND organisation = $1") {
		t.Errorf("where missing organisation clause: %s", where)
	}
	if len(args) != 1 || args[0] != "prod" {
		t.Errorf("args = %v, want [prod]", args)
	}
}

func TestBuildLogEntryFilterWhere_CookbookName(t *testing.T) {
	where, args := buildLogEntryFilterWhere(LogEntryFilter{CookbookName: "apt"})
	if !strings.Contains(where, "AND cookbook_name = $1") {
		t.Errorf("where missing cookbook_name clause: %s", where)
	}
	if len(args) != 1 || args[0] != "apt" {
		t.Errorf("args = %v, want [apt]", args)
	}
}

func TestBuildLogEntryFilterWhere_CollectionRunID(t *testing.T) {
	where, args := buildLogEntryFilterWhere(LogEntryFilter{CollectionRunID: "run-123"})
	if !strings.Contains(where, "AND collection_run_id = $1") {
		t.Errorf("where missing collection_run_id clause: %s", where)
	}
	if len(args) != 1 || args[0] != "run-123" {
		t.Errorf("args = %v, want [run-123]", args)
	}
}

func TestBuildLogEntryFilterWhere_Since(t *testing.T) {
	ts := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	where, args := buildLogEntryFilterWhere(LogEntryFilter{Since: ts})
	if !strings.Contains(where, "AND timestamp >= $1") {
		t.Errorf("where missing since clause: %s", where)
	}
	if len(args) != 1 {
		t.Fatalf("args len = %d, want 1", len(args))
	}
	if args[0] != ts {
		t.Errorf("args[0] = %v, want %v", args[0], ts)
	}
}

func TestBuildLogEntryFilterWhere_Until(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	where, args := buildLogEntryFilterWhere(LogEntryFilter{Until: ts})
	if !strings.Contains(where, "AND timestamp < $1") {
		t.Errorf("where missing until clause: %s", where)
	}
	if len(args) != 1 {
		t.Fatalf("args len = %d, want 1", len(args))
	}
	if args[0] != ts {
		t.Errorf("args[0] = %v, want %v", args[0], ts)
	}
}

func TestBuildLogEntryFilterWhere_SinceAndUntil(t *testing.T) {
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	where, args := buildLogEntryFilterWhere(LogEntryFilter{Since: since, Until: until})
	if !strings.Contains(where, "AND timestamp >= $1") {
		t.Errorf("where missing since clause: %s", where)
	}
	if !strings.Contains(where, "AND timestamp < $2") {
		t.Errorf("where missing until clause: %s", where)
	}
	if len(args) != 2 {
		t.Fatalf("args len = %d, want 2", len(args))
	}
}

func TestBuildLogEntryFilterWhere_AllFilters(t *testing.T) {
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	f := LogEntryFilter{
		Scope:           "collection",
		Severity:        "ERROR",
		Organisation:    "prod",
		CookbookName:    "apt",
		CollectionRunID: "run-1",
		Since:           since,
		Until:           until,
	}
	where, args := buildLogEntryFilterWhere(f)

	// 7 filter fields set (Severity takes precedence over MinSeverity).
	if len(args) != 7 {
		t.Fatalf("args len = %d, want 7", len(args))
	}

	// Verify sequential parameter numbering.
	for i := 1; i <= 7; i++ {
		placeholder := "$" + intToStrLog(i)
		if !strings.Contains(where, placeholder) {
			t.Errorf("where missing placeholder %s: %s", placeholder, where)
		}
	}
}

func TestBuildLogEntryFilterWhere_ParameterNumbering(t *testing.T) {
	f := LogEntryFilter{
		Scope:        "startup",
		Organisation: "dev",
		CookbookName: "nginx",
	}
	where, args := buildLogEntryFilterWhere(f)

	if len(args) != 3 {
		t.Fatalf("args len = %d, want 3", len(args))
	}

	if !strings.Contains(where, "$1") {
		t.Errorf("where missing $1: %s", where)
	}
	if !strings.Contains(where, "$2") {
		t.Errorf("where missing $2: %s", where)
	}
	if !strings.Contains(where, "$3") {
		t.Errorf("where missing $3: %s", where)
	}
	if strings.Contains(where, "$4") {
		t.Errorf("where should not contain $4: %s", where)
	}
}

func TestBuildLogEntryFilterWhere_LimitOffsetNotIncluded(t *testing.T) {
	// buildLogEntryFilterWhere does not handle Limit/Offset — callers do.
	f := LogEntryFilter{
		Scope:  "collection",
		Limit:  50,
		Offset: 100,
	}
	where, args := buildLogEntryFilterWhere(f)

	if strings.Contains(strings.ToUpper(where), "LIMIT") {
		t.Errorf("where should not contain LIMIT: %s", where)
	}
	if strings.Contains(strings.ToUpper(where), "OFFSET") {
		t.Errorf("where should not contain OFFSET: %s", where)
	}
	// Only the scope filter should produce an arg.
	if len(args) != 1 {
		t.Errorf("args len = %d, want 1", len(args))
	}
}

func TestBuildLogEntryFilterWhere_EmptyStringsIgnored(t *testing.T) {
	f := LogEntryFilter{
		Scope:           "",
		Severity:        "",
		MinSeverity:     "",
		Organisation:    "",
		CookbookName:    "",
		CollectionRunID: "",
	}
	where, args := buildLogEntryFilterWhere(f)
	if where != " WHERE 1=1" {
		t.Errorf("where = %q, want %q", where, " WHERE 1=1")
	}
	if len(args) != 0 {
		t.Errorf("args len = %d, want 0", len(args))
	}
}

func TestBuildLogEntryFilterWhere_ZeroTimeIgnored(t *testing.T) {
	f := LogEntryFilter{
		Since: time.Time{},
		Until: time.Time{},
	}
	where, args := buildLogEntryFilterWhere(f)
	if where != " WHERE 1=1" {
		t.Errorf("where = %q, want %q", where, " WHERE 1=1")
	}
	if len(args) != 0 {
		t.Errorf("args len = %d, want 0", len(args))
	}
}

// intToStrLog converts an int to a string for test assertions.
func intToStrLog(n int) string {
	digits := "0123456789"
	if n == 0 {
		return "0"
	}
	var result []byte
	for n > 0 {
		result = append([]byte{digits[n%10]}, result...)
		n /= 10
	}
	return string(result)
}
