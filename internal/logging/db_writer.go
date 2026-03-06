// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// DBInserter interface — decouples logging from the datastore package
// ---------------------------------------------------------------------------

// LogEntryParams mirrors datastore.InsertLogEntryParams without importing the
// datastore package, keeping the dependency direction clean (datastore does
// not import logging, and logging does not import datastore).
type LogEntryParams struct {
	Timestamp           time.Time
	Severity            string
	Scope               string
	Message             string
	Organisation        string
	CookbookName        string
	CookbookVersion     string
	CommitSHA           string
	ChefClientVersion   string
	ProcessOutput       string
	CollectionRunID     string
	NotificationChannel string
	ExportJobID         string
	TLSDomain           string
}

// DBInserter is the interface that the datastore must satisfy for the
// DBWriter to persist log entries. The datastore.DB type satisfies this
// interface via a thin adapter (see NewDBWriterFromDatastore).
type DBInserter interface {
	// InsertLogEntry persists a single log entry and returns its ID.
	InsertLogEntry(ctx context.Context, p LogEntryParams) (string, error)
}

// ---------------------------------------------------------------------------
// DBWriter
// ---------------------------------------------------------------------------

// DBWriter is a Writer that persists log entries to the PostgreSQL datastore
// via the DBInserter interface. It is safe for concurrent use.
//
// If the database insert fails, the error is silently dropped by default so
// that a database outage does not prevent the application from logging to
// stdout. An optional OnError callback can be set to capture these failures
// (e.g. for metrics or stderr fallback).
type DBWriter struct {
	inserter DBInserter

	// ctx is the base context used for all database inserts. If it is
	// cancelled the writer stops persisting entries.
	ctx context.Context

	// onError is called whenever a database insert fails. It may be nil.
	onError func(entry Entry, err error)

	// mu protects onError from concurrent read/write if SetOnError is
	// called after construction (unlikely but safe).
	mu sync.RWMutex
}

// DBWriterOption configures a DBWriter.
type DBWriterOption func(*DBWriter)

// WithContext sets the base context for database operations. If not set,
// context.Background() is used.
func WithContext(ctx context.Context) DBWriterOption {
	return func(dw *DBWriter) { dw.ctx = ctx }
}

// WithOnError sets a callback that is invoked whenever the database insert
// fails. The callback receives the entry that failed and the error.
func WithOnError(fn func(entry Entry, err error)) DBWriterOption {
	return func(dw *DBWriter) { dw.onError = fn }
}

// NewDBWriter creates a new DBWriter that persists log entries via the given
// DBInserter.
func NewDBWriter(inserter DBInserter, opts ...DBWriterOption) *DBWriter {
	if inserter == nil {
		panic("logging: NewDBWriter called with nil inserter")
	}
	dw := &DBWriter{
		inserter: inserter,
		ctx:      context.Background(),
	}
	for _, opt := range opts {
		opt(dw)
	}
	return dw
}

// WriteEntry persists a single log entry to the database. If the insert
// fails, the error is passed to the OnError callback (if set) and nil is
// returned so that other writers are not affected.
func (dw *DBWriter) WriteEntry(entry Entry) error {
	p := LogEntryParams{
		Timestamp:           entry.Timestamp,
		Severity:            entry.Severity.String(),
		Scope:               string(entry.Scope),
		Message:             entry.Message,
		Organisation:        entry.Organisation,
		CookbookName:        entry.CookbookName,
		CookbookVersion:     entry.CookbookVersion,
		CommitSHA:           entry.CommitSHA,
		ChefClientVersion:   entry.ChefClientVersion,
		ProcessOutput:       entry.ProcessOutput,
		CollectionRunID:     entry.CollectionRunID,
		NotificationChannel: entry.NotificationChannel,
		ExportJobID:         entry.ExportJobID,
		TLSDomain:           entry.TLSDomain,
	}

	_, err := dw.inserter.InsertLogEntry(dw.ctx, p)
	if err != nil {
		dw.mu.RLock()
		onErr := dw.onError
		dw.mu.RUnlock()
		if onErr != nil {
			onErr(entry, err)
		}
		// Swallow the error — we don't want a database failure to
		// prevent other writers (e.g. stdout) from receiving the entry.
		return nil
	}
	return nil
}

// SetOnError replaces the error callback. It is safe to call concurrently
// with WriteEntry.
func (dw *DBWriter) SetOnError(fn func(entry Entry, err error)) {
	dw.mu.Lock()
	dw.onError = fn
	dw.mu.Unlock()
}

// ---------------------------------------------------------------------------
// DBInserter adapter for datastore.DB
// ---------------------------------------------------------------------------

// DatastoreAdapter wraps a type that has an InsertLogEntry method matching
// the datastore.DB signature (returning a full struct) and adapts it to the
// DBInserter interface (returning just the ID string).
//
// Usage:
//
//	adapter := logging.NewDatastoreAdapter(db)
//	dbWriter := logging.NewDBWriter(adapter)
//
// where db is a *datastore.DB.
type DatastoreAdapter struct {
	// insertFn is a function that inserts a log entry and returns its ID.
	insertFn func(ctx context.Context, p LogEntryParams) (string, error)
}

// InsertLogEntry implements DBInserter.
func (a *DatastoreAdapter) InsertLogEntry(ctx context.Context, p LogEntryParams) (string, error) {
	return a.insertFn(ctx, p)
}

// NewDatastoreAdapter creates a DatastoreAdapter from a function. This
// allows the caller to provide an adapter without the logging package
// needing to import the datastore package.
//
// Example (in cmd/main.go or a wiring package):
//
//	adapter := logging.NewDatastoreAdapter(func(ctx context.Context, p logging.LogEntryParams) (string, error) {
//	    entry, err := db.InsertLogEntry(ctx, datastore.InsertLogEntryParams{
//	        Timestamp:           p.Timestamp,
//	        Severity:            p.Severity,
//	        Scope:               p.Scope,
//	        Message:             p.Message,
//	        Organisation:        p.Organisation,
//	        CookbookName:        p.CookbookName,
//	        CookbookVersion:     p.CookbookVersion,
//	        CommitSHA:           p.CommitSHA,
//	        ChefClientVersion:   p.ChefClientVersion,
//	        ProcessOutput:       p.ProcessOutput,
//	        CollectionRunID:     p.CollectionRunID,
//	        NotificationChannel: p.NotificationChannel,
//	        ExportJobID:         p.ExportJobID,
//	        TLSDomain:           p.TLSDomain,
//	    })
//	    if err != nil {
//	        return "", err
//	    }
//	    return entry.ID, nil
//	})
func NewDatastoreAdapter(fn func(ctx context.Context, p LogEntryParams) (string, error)) *DatastoreAdapter {
	if fn == nil {
		panic("logging: NewDatastoreAdapter called with nil function")
	}
	return &DatastoreAdapter{insertFn: fn}
}

// ---------------------------------------------------------------------------
// FailingDBInserter (for testing)
// ---------------------------------------------------------------------------

// FailingDBInserter is a DBInserter that always returns an error. It is
// intended for testing DBWriter error handling.
type FailingDBInserter struct {
	Err error
}

// InsertLogEntry always returns the configured error.
func (f *FailingDBInserter) InsertLogEntry(_ context.Context, _ LogEntryParams) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	return "", fmt.Errorf("logging: simulated DB insert failure")
}

// ---------------------------------------------------------------------------
// RecordingDBInserter (for testing)
// ---------------------------------------------------------------------------

// RecordingDBInserter is a DBInserter that records all inserted entries in
// memory. It is intended for testing.
type RecordingDBInserter struct {
	mu      sync.Mutex
	entries []LogEntryParams
	nextID  int
}

// NewRecordingDBInserter creates a new RecordingDBInserter.
func NewRecordingDBInserter() *RecordingDBInserter {
	return &RecordingDBInserter{}
}

// InsertLogEntry records the entry and returns a sequential ID.
func (r *RecordingDBInserter) InsertLogEntry(_ context.Context, p LogEntryParams) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	r.entries = append(r.entries, p)
	return fmt.Sprintf("log-%d", r.nextID), nil
}

// Entries returns a copy of all recorded entries.
func (r *RecordingDBInserter) Entries() []LogEntryParams {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]LogEntryParams, len(r.entries))
	copy(cp, r.entries)
	return cp
}

// Len returns the number of recorded entries.
func (r *RecordingDBInserter) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

// Reset clears all recorded entries.
func (r *RecordingDBInserter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = nil
	r.nextID = 0
}
