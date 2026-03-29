// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"strings"
	"testing"
)

func TestBuildCollectionRunFilterQuery_NoFilters(t *testing.T) {
	where, args := buildCollectionRunFilterQuery(CollectionRunFilter{})
	if where != " WHERE 1=1" {
		t.Errorf("where = %q, want %q", where, " WHERE 1=1")
	}
	if len(args) != 0 {
		t.Errorf("args len = %d, want 0", len(args))
	}
}

func TestBuildCollectionRunFilterQuery_Organisation(t *testing.T) {
	where, args := buildCollectionRunFilterQuery(CollectionRunFilter{Organisation: "prod"})
	if !strings.Contains(where, "AND o.name = $1") {
		t.Errorf("where missing organisation clause: %s", where)
	}
	if len(args) != 1 || args[0] != "prod" {
		t.Errorf("args = %v, want [prod]", args)
	}
}

func TestBuildCollectionRunFilterQuery_Status(t *testing.T) {
	where, args := buildCollectionRunFilterQuery(CollectionRunFilter{Status: "completed"})
	if !strings.Contains(where, "AND cr.status = $1") {
		t.Errorf("where missing status clause: %s", where)
	}
	if len(args) != 1 || args[0] != "completed" {
		t.Errorf("args = %v, want [completed]", args)
	}
}

func TestBuildCollectionRunFilterQuery_BothFilters(t *testing.T) {
	where, args := buildCollectionRunFilterQuery(CollectionRunFilter{
		Organisation: "staging",
		Status:       "failed",
	})
	if !strings.Contains(where, "AND o.name = $1") {
		t.Errorf("where missing organisation clause: %s", where)
	}
	if !strings.Contains(where, "AND cr.status = $2") {
		t.Errorf("where missing status clause: %s", where)
	}
	if len(args) != 2 {
		t.Fatalf("args len = %d, want 2", len(args))
	}
	if args[0] != "staging" {
		t.Errorf("args[0] = %v, want staging", args[0])
	}
	if args[1] != "failed" {
		t.Errorf("args[1] = %v, want failed", args[1])
	}
}

func TestBuildCollectionRunFilterQuery_EmptyStringsIgnored(t *testing.T) {
	where, args := buildCollectionRunFilterQuery(CollectionRunFilter{
		Organisation: "",
		Status:       "",
	})
	if where != " WHERE 1=1" {
		t.Errorf("where = %q, want %q", where, " WHERE 1=1")
	}
	if len(args) != 0 {
		t.Errorf("args len = %d, want 0", len(args))
	}
}

func TestBuildCollectionRunFilterQuery_LimitOffsetNotIncluded(t *testing.T) {
	where, args := buildCollectionRunFilterQuery(CollectionRunFilter{
		Organisation: "prod",
		Limit:        50,
		Offset:       100,
	})
	if strings.Contains(strings.ToUpper(where), "LIMIT") {
		t.Errorf("where should not contain LIMIT: %s", where)
	}
	if strings.Contains(strings.ToUpper(where), "OFFSET") {
		t.Errorf("where should not contain OFFSET: %s", where)
	}
	// Only the organisation filter should produce an arg.
	if len(args) != 1 {
		t.Errorf("args len = %d, want 1", len(args))
	}
}

func TestBuildCollectionRunFilterQuery_ParameterNumbering(t *testing.T) {
	where, args := buildCollectionRunFilterQuery(CollectionRunFilter{
		Organisation: "prod",
		Status:       "running",
	})
	if len(args) != 2 {
		t.Fatalf("args len = %d, want 2", len(args))
	}
	if !strings.Contains(where, "$1") {
		t.Errorf("where missing $1: %s", where)
	}
	if !strings.Contains(where, "$2") {
		t.Errorf("where missing $2: %s", where)
	}
	if strings.Contains(where, "$3") {
		t.Errorf("where should not contain $3: %s", where)
	}
}

func TestBuildCollectionRunFilterQuery_WhereStartsWith1Eq1(t *testing.T) {
	tests := []CollectionRunFilter{
		{},
		{Organisation: "prod"},
		{Status: "completed"},
		{Organisation: "prod", Status: "completed"},
	}
	for _, f := range tests {
		where, _ := buildCollectionRunFilterQuery(f)
		if !strings.HasPrefix(where, " WHERE 1=1") {
			t.Errorf("where does not start with ' WHERE 1=1': %s", where)
		}
	}
}
