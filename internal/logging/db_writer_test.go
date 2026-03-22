// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"errors"
	"testing"
	"time"
)

func TestDBWriter_WriteEntry_Success(t *testing.T) {
	rec := NewRecordingDBInserter()
	dw := NewDBWriter(rec)

	entry := Entry{
		Timestamp: refTime,
		Severity:  INFO,
		Scope:     ScopeCollectionRun,
		Message:   "collection started",
	}

	err := dw.WriteEntry(entry)
	if err != nil {
		t.Fatalf("WriteEntry returned error: %v", err)
	}

	if rec.Len() != 1 {
		t.Fatalf("got %d recorded entries, want 1", rec.Len())
	}

	p := rec.Entries()[0]
	if p.Severity != "INFO" {
		t.Errorf("Severity = %q, want %q", p.Severity, "INFO")
	}
	if p.Scope != string(ScopeCollectionRun) {
		t.Errorf("Scope = %q, want %q", p.Scope, string(ScopeCollectionRun))
	}
	if p.Message != "collection started" {
		t.Errorf("Message = %q, want %q", p.Message, "collection started")
	}
	if !p.Timestamp.Equal(refTime) {
		t.Errorf("Timestamp = %v, want %v", p.Timestamp, refTime)
	}
}

func TestDBWriter_WriteEntry_ErrorSwallowed(t *testing.T) {
	fi := &FailingDBInserter{Err: errors.New("connection refused")}
	dw := NewDBWriter(fi)

	err := dw.WriteEntry(Entry{
		Timestamp: refTime,
		Severity:  WARN,
		Scope:     ScopeStartup,
		Message:   "should not panic",
	})

	if err != nil {
		t.Errorf("WriteEntry should swallow DB errors, got %v", err)
	}
}

func TestDBWriter_OnError_Called(t *testing.T) {
	dbErr := errors.New("disk full")
	fi := &FailingDBInserter{Err: dbErr}

	var capturedEntry Entry
	var capturedErr error
	called := false

	dw := NewDBWriter(fi, WithOnError(func(entry Entry, err error) {
		called = true
		capturedEntry = entry
		capturedErr = err
	}))

	entry := Entry{
		Timestamp: refTime,
		Severity:  ERROR,
		Scope:     ScopeCookstyleScan,
		Message:   "scan failed",
	}
	_ = dw.WriteEntry(entry)

	if !called {
		t.Fatal("OnError callback was not invoked")
	}
	if !errors.Is(capturedErr, dbErr) {
		t.Errorf("OnError error = %v, want %v", capturedErr, dbErr)
	}
	if capturedEntry.Message != "scan failed" {
		t.Errorf("OnError entry message = %q, want %q", capturedEntry.Message, "scan failed")
	}
	if capturedEntry.Severity != ERROR {
		t.Errorf("OnError entry severity = %v, want %v", capturedEntry.Severity, ERROR)
	}
}

func TestDBWriter_OnBroadcast_CalledOnSuccess(t *testing.T) {
	rec := NewRecordingDBInserter()

	called := false
	dw := NewDBWriter(rec, WithOnBroadcast(func(entry Entry) {
		called = true
	}))

	err := dw.WriteEntry(Entry{
		Timestamp: refTime,
		Severity:  INFO,
		Scope:     ScopeCollectionRun,
		Message:   "node collected",
	})
	if err != nil {
		t.Fatalf("WriteEntry returned error: %v", err)
	}

	if !called {
		t.Error("OnBroadcast callback was not invoked on successful insert")
	}
}

func TestDBWriter_OnBroadcast_NotCalledOnError(t *testing.T) {
	fi := &FailingDBInserter{Err: errors.New("db unavailable")}

	called := false
	dw := NewDBWriter(fi, WithOnBroadcast(func(entry Entry) {
		called = true
	}))

	_ = dw.WriteEntry(Entry{
		Timestamp: refTime,
		Severity:  WARN,
		Scope:     ScopeStartup,
		Message:   "this should fail",
	})

	if called {
		t.Error("OnBroadcast callback should not be invoked when insert fails")
	}
}

func TestDBWriter_OnBroadcast_Nil(t *testing.T) {
	rec := NewRecordingDBInserter()
	// No WithOnBroadcast option — onBroadcast is nil.
	dw := NewDBWriter(rec)

	err := dw.WriteEntry(Entry{
		Timestamp: refTime,
		Severity:  DEBUG,
		Scope:     ScopeWebAPI,
		Message:   "request received",
	})
	if err != nil {
		t.Fatalf("WriteEntry returned error: %v", err)
	}

	if rec.Len() != 1 {
		t.Fatalf("got %d recorded entries, want 1", rec.Len())
	}
}

func TestDBWriter_SetOnBroadcast(t *testing.T) {
	rec := NewRecordingDBInserter()
	dw := NewDBWriter(rec)

	// Initially no broadcast callback — write should succeed silently.
	_ = dw.WriteEntry(Entry{
		Timestamp: refTime,
		Severity:  INFO,
		Scope:     ScopeStartup,
		Message:   "before callback set",
	})

	// Now set a broadcast callback after construction.
	var capturedMessage string
	dw.SetOnBroadcast(func(entry Entry) {
		capturedMessage = entry.Message
	})

	_ = dw.WriteEntry(Entry{
		Timestamp: refTime,
		Severity:  INFO,
		Scope:     ScopeStartup,
		Message:   "after callback set",
	})

	if capturedMessage != "after callback set" {
		t.Errorf("SetOnBroadcast callback message = %q, want %q", capturedMessage, "after callback set")
	}

	// Replace with a new callback to verify it truly replaces.
	var secondMessage string
	dw.SetOnBroadcast(func(entry Entry) {
		secondMessage = entry.Message
	})

	_ = dw.WriteEntry(Entry{
		Timestamp: refTime,
		Severity:  INFO,
		Scope:     ScopeStartup,
		Message:   "replacement callback",
	})

	if secondMessage != "replacement callback" {
		t.Errorf("replacement callback message = %q, want %q", secondMessage, "replacement callback")
	}
	// The previous callback should NOT have been called again with the new message.
	if capturedMessage != "after callback set" {
		t.Errorf("original callback was unexpectedly invoked again, message = %q", capturedMessage)
	}
}

func TestDBWriter_OnBroadcast_ReceivesCorrectEntry(t *testing.T) {
	rec := NewRecordingDBInserter()

	var broadcastEntry Entry
	dw := NewDBWriter(rec, WithOnBroadcast(func(entry Entry) {
		broadcastEntry = entry
	}))

	ts := time.Date(2025, 6, 15, 8, 30, 0, 0, time.UTC)
	entry := Entry{
		Timestamp:           ts,
		Severity:            WARN,
		Scope:               ScopeCookstyleScan,
		Message:             "deprecated resource usage",
		Organisation:        "acme-corp",
		CookbookName:        "nginx",
		CookbookVersion:     "3.1.0",
		CommitSHA:           "deadbeef",
		ChefClientVersion:   "18.4.2",
		ProcessOutput:       "warning on line 42",
		CollectionRunID:     "run-99",
		NotificationChannel: "email",
		ExportJobID:         "export-7",
		TLSDomain:           "chef.acme.com",
	}

	err := dw.WriteEntry(entry)
	if err != nil {
		t.Fatalf("WriteEntry returned error: %v", err)
	}

	if !broadcastEntry.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v, want %v", broadcastEntry.Timestamp, ts)
	}
	if broadcastEntry.Severity != WARN {
		t.Errorf("Severity = %v, want %v", broadcastEntry.Severity, WARN)
	}
	if broadcastEntry.Scope != ScopeCookstyleScan {
		t.Errorf("Scope = %q, want %q", broadcastEntry.Scope, ScopeCookstyleScan)
	}
	if broadcastEntry.Message != "deprecated resource usage" {
		t.Errorf("Message = %q, want %q", broadcastEntry.Message, "deprecated resource usage")
	}
	if broadcastEntry.Organisation != "acme-corp" {
		t.Errorf("Organisation = %q, want %q", broadcastEntry.Organisation, "acme-corp")
	}
	if broadcastEntry.CookbookName != "nginx" {
		t.Errorf("CookbookName = %q, want %q", broadcastEntry.CookbookName, "nginx")
	}
	if broadcastEntry.CookbookVersion != "3.1.0" {
		t.Errorf("CookbookVersion = %q, want %q", broadcastEntry.CookbookVersion, "3.1.0")
	}
	if broadcastEntry.CommitSHA != "deadbeef" {
		t.Errorf("CommitSHA = %q, want %q", broadcastEntry.CommitSHA, "deadbeef")
	}
	if broadcastEntry.ChefClientVersion != "18.4.2" {
		t.Errorf("ChefClientVersion = %q, want %q", broadcastEntry.ChefClientVersion, "18.4.2")
	}
	if broadcastEntry.ProcessOutput != "warning on line 42" {
		t.Errorf("ProcessOutput = %q, want %q", broadcastEntry.ProcessOutput, "warning on line 42")
	}
	if broadcastEntry.CollectionRunID != "run-99" {
		t.Errorf("CollectionRunID = %q, want %q", broadcastEntry.CollectionRunID, "run-99")
	}
	if broadcastEntry.NotificationChannel != "email" {
		t.Errorf("NotificationChannel = %q, want %q", broadcastEntry.NotificationChannel, "email")
	}
	if broadcastEntry.ExportJobID != "export-7" {
		t.Errorf("ExportJobID = %q, want %q", broadcastEntry.ExportJobID, "export-7")
	}
	if broadcastEntry.TLSDomain != "chef.acme.com" {
		t.Errorf("TLSDomain = %q, want %q", broadcastEntry.TLSDomain, "chef.acme.com")
	}
}
