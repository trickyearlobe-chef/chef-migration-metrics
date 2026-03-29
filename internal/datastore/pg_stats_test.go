// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"encoding/json"
	"testing"
)

func TestTopQueryStat_JSONMarshalling(t *testing.T) {
	original := TopQueryStat{
		Query:          "SELECT * FROM nodes WHERE id = $1",
		Calls:          1500,
		TotalTimeMs:    4500.75,
		MeanTimeMs:     3.0005,
		MinTimeMs:      0.12,
		MaxTimeMs:      250.5,
		Rows:           1500,
		SharedBlksHit:  80000,
		SharedBlksRead: 200,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal(TopQueryStat) failed: %v", err)
	}

	var decoded TopQueryStat
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(TopQueryStat) failed: %v", err)
	}

	if decoded.Query != original.Query {
		t.Errorf("Query = %q, want %q", decoded.Query, original.Query)
	}
	if decoded.Calls != original.Calls {
		t.Errorf("Calls = %d, want %d", decoded.Calls, original.Calls)
	}
	if decoded.TotalTimeMs != original.TotalTimeMs {
		t.Errorf("TotalTimeMs = %f, want %f", decoded.TotalTimeMs, original.TotalTimeMs)
	}
	if decoded.MeanTimeMs != original.MeanTimeMs {
		t.Errorf("MeanTimeMs = %f, want %f", decoded.MeanTimeMs, original.MeanTimeMs)
	}
	if decoded.MinTimeMs != original.MinTimeMs {
		t.Errorf("MinTimeMs = %f, want %f", decoded.MinTimeMs, original.MinTimeMs)
	}
	if decoded.MaxTimeMs != original.MaxTimeMs {
		t.Errorf("MaxTimeMs = %f, want %f", decoded.MaxTimeMs, original.MaxTimeMs)
	}
	if decoded.Rows != original.Rows {
		t.Errorf("Rows = %d, want %d", decoded.Rows, original.Rows)
	}
	if decoded.SharedBlksHit != original.SharedBlksHit {
		t.Errorf("SharedBlksHit = %d, want %d", decoded.SharedBlksHit, original.SharedBlksHit)
	}
	if decoded.SharedBlksRead != original.SharedBlksRead {
		t.Errorf("SharedBlksRead = %d, want %d", decoded.SharedBlksRead, original.SharedBlksRead)
	}
}

func TestTableStat_JSONMarshalling(t *testing.T) {
	vacuum := "2025-06-15T10:30:00Z"
	var nilAnalyze *string

	original := TableStat{
		TableName:   "nodes",
		SeqScan:     120,
		SeqTupRead:  50000,
		IdxScan:     9800,
		IdxTupFetch: 9750,
		NLiveTup:    4200,
		NDeadTup:    15,
		LastVacuum:  &vacuum,
		LastAnalyze: nilAnalyze,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal(TableStat) failed: %v", err)
	}

	var decoded TableStat
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(TableStat) failed: %v", err)
	}

	if decoded.TableName != original.TableName {
		t.Errorf("TableName = %q, want %q", decoded.TableName, original.TableName)
	}
	if decoded.SeqScan != original.SeqScan {
		t.Errorf("SeqScan = %d, want %d", decoded.SeqScan, original.SeqScan)
	}
	if decoded.SeqTupRead != original.SeqTupRead {
		t.Errorf("SeqTupRead = %d, want %d", decoded.SeqTupRead, original.SeqTupRead)
	}
	if decoded.IdxScan != original.IdxScan {
		t.Errorf("IdxScan = %d, want %d", decoded.IdxScan, original.IdxScan)
	}
	if decoded.IdxTupFetch != original.IdxTupFetch {
		t.Errorf("IdxTupFetch = %d, want %d", decoded.IdxTupFetch, original.IdxTupFetch)
	}
	if decoded.NLiveTup != original.NLiveTup {
		t.Errorf("NLiveTup = %d, want %d", decoded.NLiveTup, original.NLiveTup)
	}
	if decoded.NDeadTup != original.NDeadTup {
		t.Errorf("NDeadTup = %d, want %d", decoded.NDeadTup, original.NDeadTup)
	}
	if decoded.LastVacuum == nil {
		t.Fatal("LastVacuum should not be nil")
	}
	if *decoded.LastVacuum != vacuum {
		t.Errorf("LastVacuum = %q, want %q", *decoded.LastVacuum, vacuum)
	}
	if decoded.LastAnalyze != nil {
		t.Errorf("LastAnalyze = %v, want nil", decoded.LastAnalyze)
	}
}

func TestIndexStat_JSONMarshalling(t *testing.T) {
	original := IndexStat{
		TableName:   "nodes",
		IndexName:   "idx_nodes_org_id",
		IdxScan:     15000,
		IdxTupRead:  15000,
		IdxTupFetch: 14800,
		SizeBytes:   2097152,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal(IndexStat) failed: %v", err)
	}

	var decoded IndexStat
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(IndexStat) failed: %v", err)
	}

	if decoded.TableName != original.TableName {
		t.Errorf("TableName = %q, want %q", decoded.TableName, original.TableName)
	}
	if decoded.IndexName != original.IndexName {
		t.Errorf("IndexName = %q, want %q", decoded.IndexName, original.IndexName)
	}
	if decoded.IdxScan != original.IdxScan {
		t.Errorf("IdxScan = %d, want %d", decoded.IdxScan, original.IdxScan)
	}
	if decoded.IdxTupRead != original.IdxTupRead {
		t.Errorf("IdxTupRead = %d, want %d", decoded.IdxTupRead, original.IdxTupRead)
	}
	if decoded.IdxTupFetch != original.IdxTupFetch {
		t.Errorf("IdxTupFetch = %d, want %d", decoded.IdxTupFetch, original.IdxTupFetch)
	}
	if decoded.SizeBytes != original.SizeBytes {
		t.Errorf("SizeBytes = %d, want %d", decoded.SizeBytes, original.SizeBytes)
	}
}

func TestActiveQuery_JSONMarshalling(t *testing.T) {
	waitType := "Lock"
	waitEvent := "relation"

	original := ActiveQuery{
		PID:           12345,
		State:         "active",
		Query:         "UPDATE nodes SET updated_at = now() WHERE id = $1",
		DurationMs:    1500.25,
		WaitEventType: &waitType,
		WaitEvent:     &waitEvent,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal(ActiveQuery) failed: %v", err)
	}

	var decoded ActiveQuery
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(ActiveQuery) failed: %v", err)
	}

	if decoded.PID != original.PID {
		t.Errorf("PID = %d, want %d", decoded.PID, original.PID)
	}
	if decoded.State != original.State {
		t.Errorf("State = %q, want %q", decoded.State, original.State)
	}
	if decoded.Query != original.Query {
		t.Errorf("Query = %q, want %q", decoded.Query, original.Query)
	}
	if decoded.DurationMs != original.DurationMs {
		t.Errorf("DurationMs = %f, want %f", decoded.DurationMs, original.DurationMs)
	}
	if decoded.WaitEventType == nil {
		t.Fatal("WaitEventType should not be nil")
	}
	if *decoded.WaitEventType != waitType {
		t.Errorf("WaitEventType = %q, want %q", *decoded.WaitEventType, waitType)
	}
	if decoded.WaitEvent == nil {
		t.Fatal("WaitEvent should not be nil")
	}
	if *decoded.WaitEvent != waitEvent {
		t.Errorf("WaitEvent = %q, want %q", *decoded.WaitEvent, waitEvent)
	}

	// Test with nil optional fields.
	noWait := ActiveQuery{
		PID:           99,
		State:         "active",
		Query:         "SELECT 1",
		DurationMs:    0.5,
		WaitEventType: nil,
		WaitEvent:     nil,
	}

	data, err = json.Marshal(noWait)
	if err != nil {
		t.Fatalf("json.Marshal(ActiveQuery with nils) failed: %v", err)
	}

	var decodedNoWait ActiveQuery
	if err := json.Unmarshal(data, &decodedNoWait); err != nil {
		t.Fatalf("json.Unmarshal(ActiveQuery with nils) failed: %v", err)
	}

	if decodedNoWait.WaitEventType != nil {
		t.Errorf("WaitEventType = %v, want nil", decodedNoWait.WaitEventType)
	}
	if decodedNoWait.WaitEvent != nil {
		t.Errorf("WaitEvent = %v, want nil", decodedNoWait.WaitEvent)
	}
}
