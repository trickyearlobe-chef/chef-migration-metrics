// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Event tests
// ---------------------------------------------------------------------------

func TestNewEvent(t *testing.T) {
	before := time.Now().UTC()
	evt := NewEvent(EventCollectionComplete, map[string]string{"org": "prod"})
	after := time.Now().UTC()

	if evt.Type != EventCollectionComplete {
		t.Errorf("Type = %q, want %q", evt.Type, EventCollectionComplete)
	}
	if evt.Timestamp.Before(before) || evt.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", evt.Timestamp, before, after)
	}
	data, ok := evt.Data.(map[string]string)
	if !ok {
		t.Fatalf("Data type = %T, want map[string]string", evt.Data)
	}
	if data["org"] != "prod" {
		t.Errorf("Data[org] = %q, want %q", data["org"], "prod")
	}
}

func TestEventMarshalJSON_NilData(t *testing.T) {
	evt := Event{Type: "test", Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	b, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	// Data should be serialised as {} not omitted.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if _, ok := m["data"]; !ok {
		t.Error("expected 'data' key in JSON output when Data is nil")
	}
}

func TestEventMarshalJSON_WithData(t *testing.T) {
	evt := NewEvent(EventLogEntry, map[string]int{"count": 42})
	b, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if string(m["event"]) != `"log_entry"` {
		t.Errorf("event = %s, want %q", m["event"], "log_entry")
	}
}

// ---------------------------------------------------------------------------
// EventHub lifecycle tests
// ---------------------------------------------------------------------------

func TestEventHub_RegisterAndBroadcast(t *testing.T) {
	hub := NewEventHub(WithMaxConnections(10), WithSendBufferSize(8))
	go hub.Run()
	defer hub.Stop()

	c := hub.Register()

	// Give the run loop time to process registration.
	time.Sleep(20 * time.Millisecond)

	evt := NewEvent(EventCollectionComplete, nil)
	hub.Broadcast(evt)

	select {
	case received, ok := <-c.send:
		if !ok {
			t.Fatal("send channel closed unexpectedly")
		}
		if received.Type != EventCollectionComplete {
			t.Errorf("Type = %q, want %q", received.Type, EventCollectionComplete)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventHub_MultipleClients(t *testing.T) {
	hub := NewEventHub(WithMaxConnections(10), WithSendBufferSize(8))
	go hub.Run()
	defer hub.Stop()

	clients := make([]*client, 3)
	for i := range clients {
		clients[i] = hub.Register()
	}
	time.Sleep(20 * time.Millisecond)

	hub.Broadcast(NewEvent("test_event", nil))

	for i, c := range clients {
		select {
		case evt, ok := <-c.send:
			if !ok {
				t.Fatalf("client %d: send channel closed", i)
			}
			if evt.Type != "test_event" {
				t.Errorf("client %d: Type = %q, want %q", i, evt.Type, "test_event")
			}
		case <-time.After(time.Second):
			t.Fatalf("client %d: timed out", i)
		}
	}
}

func TestEventHub_Unregister(t *testing.T) {
	hub := NewEventHub(WithMaxConnections(10), WithSendBufferSize(8))
	go hub.Run()
	defer hub.Stop()

	c := hub.Register()
	time.Sleep(20 * time.Millisecond)

	hub.Unregister(c)
	time.Sleep(20 * time.Millisecond)

	// After unregister, the send channel should be closed.
	_, ok := <-c.send
	if ok {
		t.Error("expected send channel to be closed after Unregister")
	}
}

func TestEventHub_SlowConsumer(t *testing.T) {
	// Buffer size 1 — second event should trigger slow consumer eviction.
	hub := NewEventHub(WithMaxConnections(10), WithSendBufferSize(1))
	go hub.Run()
	defer hub.Stop()

	c := hub.Register()
	time.Sleep(20 * time.Millisecond)

	// Fill the buffer.
	hub.Broadcast(NewEvent("evt1", nil))
	time.Sleep(10 * time.Millisecond)

	// This should overflow the buffer and close the client.
	hub.Broadcast(NewEvent("evt2", nil))
	time.Sleep(10 * time.Millisecond)

	// The channel may have evt1 then be closed, or just be closed.
	drained := false
	for !drained {
		select {
		case _, ok := <-c.send:
			if !ok {
				drained = true
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("timed out; channel not closed (slow consumer not evicted)")
			return
		}
	}
}

func TestEventHub_MaxConnections(t *testing.T) {
	hub := NewEventHub(WithMaxConnections(2), WithSendBufferSize(4))
	go hub.Run()
	defer hub.Stop()

	c1 := hub.Register()
	c2 := hub.Register()
	time.Sleep(20 * time.Millisecond)

	// Third client should be rejected (channel closed immediately).
	c3 := hub.Register()
	time.Sleep(20 * time.Millisecond)

	_, ok := <-c3.send
	if ok {
		t.Error("expected third client to be rejected (channel closed)")
	}

	// First two should still work.
	hub.Broadcast(NewEvent("test", nil))
	for i, c := range []*client{c1, c2} {
		select {
		case _, ok := <-c.send:
			if !ok {
				t.Errorf("client %d: unexpected channel close", i)
			}
		case <-time.After(time.Second):
			t.Errorf("client %d: timed out", i)
		}
	}
}

func TestEventHub_ClientCount(t *testing.T) {
	hub := NewEventHub(WithMaxConnections(10), WithSendBufferSize(4))
	go hub.Run()
	defer hub.Stop()

	if n := hub.ClientCount(); n != 0 {
		t.Errorf("initial ClientCount = %d, want 0", n)
	}

	c1 := hub.Register()
	hub.Register()
	time.Sleep(20 * time.Millisecond)

	if n := hub.ClientCount(); n != 2 {
		t.Errorf("after 2 registers, ClientCount = %d, want 2", n)
	}

	hub.Unregister(c1)
	time.Sleep(20 * time.Millisecond)

	if n := hub.ClientCount(); n != 1 {
		t.Errorf("after unregister, ClientCount = %d, want 1", n)
	}
}

func TestEventHub_StopClosesAllClients(t *testing.T) {
	hub := NewEventHub(WithMaxConnections(10), WithSendBufferSize(4))
	go hub.Run()

	c1 := hub.Register()
	c2 := hub.Register()
	time.Sleep(20 * time.Millisecond)

	hub.Stop()
	time.Sleep(20 * time.Millisecond)

	for i, c := range []*client{c1, c2} {
		_, ok := <-c.send
		if ok {
			t.Errorf("client %d: channel not closed after Stop", i)
		}
	}
}

func TestEventHub_StopIdempotent(t *testing.T) {
	hub := NewEventHub()
	go hub.Run()

	hub.Stop()
	hub.Stop() // Should not panic.
}

func TestEventHub_BroadcastAfterStop(t *testing.T) {
	hub := NewEventHub()
	go hub.Run()
	hub.Stop()
	time.Sleep(20 * time.Millisecond)

	// Should not panic or block.
	hub.Broadcast(NewEvent("after_stop", nil))
}

func TestEventHub_RegisterAfterStop(t *testing.T) {
	hub := NewEventHub()
	go hub.Run()
	hub.Stop()
	time.Sleep(20 * time.Millisecond)

	c := hub.Register()
	// Channel should be closed immediately.
	_, ok := <-c.send
	if ok {
		t.Error("expected channel closed when registering after stop")
	}
}

func TestEventHub_DefaultOptions(t *testing.T) {
	hub := NewEventHub()
	if hub.maxConnections != 100 {
		t.Errorf("default maxConnections = %d, want 100", hub.maxConnections)
	}
	if hub.sendBufferSize != 64 {
		t.Errorf("default sendBufferSize = %d, want 64", hub.sendBufferSize)
	}
}

func TestWithMaxConnections_IgnoresNonPositive(t *testing.T) {
	hub := NewEventHub(WithMaxConnections(0), WithMaxConnections(-5))
	if hub.maxConnections != 100 {
		t.Errorf("maxConnections = %d, want 100 (default)", hub.maxConnections)
	}
}

func TestWithSendBufferSize_IgnoresNonPositive(t *testing.T) {
	hub := NewEventHub(WithSendBufferSize(0), WithSendBufferSize(-1))
	if hub.sendBufferSize != 64 {
		t.Errorf("sendBufferSize = %d, want 64 (default)", hub.sendBufferSize)
	}
}

// ---------------------------------------------------------------------------
// Event type constant tests
// ---------------------------------------------------------------------------

func TestEventTypeConstants(t *testing.T) {
	// Ensure all constants are non-empty and unique.
	types := []string{
		EventConnected,
		EventCollectionStarted, EventCollectionProgress,
		EventCollectionComplete, EventCollectionFailed,
		EventCookbookStatusChanged, EventReadinessUpdated,
		EventComplexityUpdated, EventRescanStarted, EventRescanComplete,
		EventExportStarted, EventExportComplete, EventExportFailed,
		EventLogEntry,
		EventNotificationSent, EventNotificationFailed,
	}
	seen := make(map[string]bool, len(types))
	for _, typ := range types {
		if typ == "" {
			t.Error("found empty event type constant")
		}
		if seen[typ] {
			t.Errorf("duplicate event type constant: %q", typ)
		}
		seen[typ] = true
	}
}

// ---------------------------------------------------------------------------
// Response helper tests
// ---------------------------------------------------------------------------

func TestParsePagination_Defaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	p := ParsePagination(r)
	if p.Page != 1 {
		t.Errorf("Page = %d, want 1", p.Page)
	}
	if p.PerPage != 50 {
		t.Errorf("PerPage = %d, want 50", p.PerPage)
	}
}

func TestParsePagination_Custom(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?page=3&per_page=25", nil)
	p := ParsePagination(r)
	if p.Page != 3 {
		t.Errorf("Page = %d, want 3", p.Page)
	}
	if p.PerPage != 25 {
		t.Errorf("PerPage = %d, want 25", p.PerPage)
	}
}

func TestParsePagination_ClampMax(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?per_page=999", nil)
	p := ParsePagination(r)
	if p.PerPage != 500 {
		t.Errorf("PerPage = %d, want 500 (clamped)", p.PerPage)
	}
}

func TestParsePagination_InvalidFallback(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?page=abc&per_page=-1", nil)
	p := ParsePagination(r)
	if p.Page != 1 {
		t.Errorf("Page = %d, want 1 (default)", p.Page)
	}
	if p.PerPage != 50 {
		t.Errorf("PerPage = %d, want 50 (default)", p.PerPage)
	}
}

func TestPaginationParams_OffsetAndLimit(t *testing.T) {
	p := PaginationParams{Page: 3, PerPage: 25}
	if p.Offset() != 50 {
		t.Errorf("Offset = %d, want 50", p.Offset())
	}
	if p.Limit() != 25 {
		t.Errorf("Limit = %d, want 25", p.Limit())
	}
}

func TestNewPaginationResponse(t *testing.T) {
	tests := []struct {
		name      string
		page      int
		perPage   int
		total     int
		wantPages int
	}{
		{"exact", 1, 10, 30, 3},
		{"remainder", 1, 10, 31, 4},
		{"single page", 1, 50, 10, 1},
		{"zero items", 1, 50, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := NewPaginationResponse(PaginationParams{Page: tt.page, PerPage: tt.perPage}, tt.total)
			if pr.TotalPages != tt.wantPages {
				t.Errorf("TotalPages = %d, want %d", pr.TotalPages, tt.wantPages)
			}
			if pr.TotalItems != tt.total {
				t.Errorf("TotalItems = %d, want %d", pr.TotalItems, tt.total)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
	var m map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if m["key"] != "value" {
		t.Errorf("key = %q, want %q", m["key"], "value")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusNotFound, ErrCodeNotFound, "Node not found.")

	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Error != "not_found" {
		t.Errorf("error = %q, want %q", resp.Error, "not_found")
	}
	if resp.Message != "Node not found." {
		t.Errorf("message = %q, want %q", resp.Message, "Node not found.")
	}
}

func TestWritePaginated(t *testing.T) {
	w := httptest.NewRecorder()
	data := []string{"a", "b"}
	WritePaginated(w, data, PaginationParams{Page: 2, PerPage: 10}, 25)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if resp.Pagination.Page != 2 {
		t.Errorf("page = %d, want 2", resp.Pagination.Page)
	}
	if resp.Pagination.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", resp.Pagination.TotalPages)
	}
}

func TestParseSort_Default(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	s := ParseSort(r, "name", []string{"name", "count"})
	if s.Field != "name" {
		t.Errorf("Field = %q, want %q", s.Field, "name")
	}
	if s.Order != "asc" {
		t.Errorf("Order = %q, want %q", s.Order, "asc")
	}
}

func TestParseSort_Custom(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?sort=count&order=desc", nil)
	s := ParseSort(r, "name", []string{"name", "count"})
	if s.Field != "count" {
		t.Errorf("Field = %q, want %q", s.Field, "count")
	}
	if s.Order != "desc" {
		t.Errorf("Order = %q, want %q", s.Order, "desc")
	}
}

func TestParseSort_DisallowedField(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?sort=evil_field", nil)
	s := ParseSort(r, "name", []string{"name", "count"})
	if s.Field != "name" {
		t.Errorf("Field = %q, want %q (default, disallowed field ignored)", s.Field, "name")
	}
}

// ---------------------------------------------------------------------------
// secondsToDuration helper test
// ---------------------------------------------------------------------------

func TestSecondsToDuration(t *testing.T) {
	if d := secondsToDuration(5); d != 5*time.Second {
		t.Errorf("secondsToDuration(5) = %v, want 5s", d)
	}
	if d := secondsToDuration(0); d != 10*time.Second {
		t.Errorf("secondsToDuration(0) = %v, want 10s (default)", d)
	}
	if d := secondsToDuration(-1); d != 10*time.Second {
		t.Errorf("secondsToDuration(-1) = %v, want 10s (default)", d)
	}
}
