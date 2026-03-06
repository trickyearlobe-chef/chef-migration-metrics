// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package webapi provides the HTTP API layer for Chef Migration Metrics.
// It includes the REST endpoint router, WebSocket real-time event hub,
// authentication middleware, and response helpers.
//
// HTTP handlers in this package are thin — they validate input, call domain
// logic in other packages, and serialise output. Business logic lives in the
// domain packages (analysis/, collector/, etc.).
package webapi

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Event types — constants for the "event" field in the JSON envelope.
// ---------------------------------------------------------------------------

const (
	// Connection events.
	EventConnected = "connected"

	// Collection events.
	EventCollectionStarted  = "collection_started"
	EventCollectionProgress = "collection_progress"
	EventCollectionComplete = "collection_complete"
	EventCollectionFailed   = "collection_failed"

	// Analysis events.
	EventCookbookStatusChanged = "cookbook_status_changed"
	EventReadinessUpdated      = "readiness_updated"
	EventComplexityUpdated     = "complexity_updated"
	EventRescanStarted         = "rescan_started"
	EventRescanComplete        = "rescan_complete"

	// Export events.
	EventExportStarted  = "export_started"
	EventExportComplete = "export_complete"
	EventExportFailed   = "export_failed"

	// Log events.
	EventLogEntry = "log_entry"

	// Notification events.
	EventNotificationSent   = "notification_sent"
	EventNotificationFailed = "notification_failed"
)

// ---------------------------------------------------------------------------
// Event — the envelope sent over WebSocket connections.
// ---------------------------------------------------------------------------

// Event is the JSON envelope pushed to WebSocket clients. All events share
// this structure; the Data field carries event-specific payload.
type Event struct {
	// Type is the event type identifier (e.g. "collection_complete").
	Type string `json:"event"`

	// Timestamp is when the event occurred (UTC).
	Timestamp time.Time `json:"timestamp"`

	// Data is the event-specific payload. It may be nil for simple signals.
	Data any `json:"data,omitempty"`
}

// MarshalJSON implements json.Marshaler. If Data is nil it is serialised as
// an empty object rather than being omitted, matching the spec envelope.
func (e Event) MarshalJSON() ([]byte, error) {
	type alias Event // prevent recursion
	if e.Data == nil {
		e.Data = struct{}{}
	}
	return json.Marshal(alias(e))
}

// NewEvent creates an Event with the given type and data, timestamped to now.
func NewEvent(eventType string, data any) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}
}

// ---------------------------------------------------------------------------
// Client — represents a single connected WebSocket client.
// ---------------------------------------------------------------------------

// client represents a single WebSocket connection registered with the hub.
// Each client has a buffered send channel; the WebSocket write goroutine
// drains this channel and writes JSON frames to the connection.
type client struct {
	send chan Event
}

// ---------------------------------------------------------------------------
// EventHub — fan-out broadcaster.
// ---------------------------------------------------------------------------

// EventHub manages WebSocket client connections and broadcasts events to all
// connected clients. It uses a hub-and-spoke pattern:
//
//   - Components call Broadcast() to push an event.
//   - The hub's run loop fans the event out to every registered client's
//     send channel.
//   - If a client's send channel is full (slow consumer), that client is
//     removed and its channel is closed. The WebSocket write goroutine
//     detects the closed channel and terminates the connection.
//
// Broadcast() is safe to call from any goroutine. Client registration and
// deregistration are serialised through the hub's internal channels.
type EventHub struct {
	// maxConnections is the upper limit on simultaneous WebSocket clients.
	maxConnections int

	// sendBufferSize is the capacity of each client's send channel.
	sendBufferSize int

	// register requests a new client be added.
	register chan *client

	// unregister requests a client be removed.
	unregister chan *client

	// broadcast receives events to fan out to all clients.
	broadcast chan Event

	// clients is the set of currently registered clients. Only accessed by
	// the run goroutine — no external lock required.
	clients map[*client]struct{}

	// clientCount is an atomic counter tracking len(clients). It is
	// updated exclusively by the run goroutine but may be read from any
	// goroutine via ClientCount().
	clientCount atomic.Int64

	// done is closed when Stop() is called to terminate the run loop.
	done chan struct{}

	// stopped guards against double-close of done.
	stopOnce sync.Once
}

// EventHubOption is a functional option for NewEventHub.
type EventHubOption func(*EventHub)

// WithMaxConnections sets the maximum number of concurrent WebSocket clients.
func WithMaxConnections(n int) EventHubOption {
	return func(h *EventHub) {
		if n > 0 {
			h.maxConnections = n
		}
	}
}

// WithSendBufferSize sets the per-client send channel buffer capacity.
func WithSendBufferSize(n int) EventHubOption {
	return func(h *EventHub) {
		if n > 0 {
			h.sendBufferSize = n
		}
	}
}

// NewEventHub creates a new EventHub. Call Run() to start the event loop and
// Stop() to shut it down.
func NewEventHub(opts ...EventHubOption) *EventHub {
	h := &EventHub{
		maxConnections: 100,
		sendBufferSize: 64,
		register:       make(chan *client, 16),
		unregister:     make(chan *client, 16),
		broadcast:      make(chan Event, 256),
		clients:        make(map[*client]struct{}),
		done:           make(chan struct{}),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Run starts the hub's event loop. It blocks until Stop() is called. Callers
// should run this in a dedicated goroutine:
//
//	go hub.Run()
func (h *EventHub) Run() {
	for {
		select {
		case <-h.done:
			// Shutdown: close all client send channels so their write
			// goroutines can terminate.
			for c := range h.clients {
				close(c.send)
				delete(h.clients, c)
			}
			h.clientCount.Store(0)
			return

		case c := <-h.register:
			if len(h.clients) >= h.maxConnections {
				// At capacity — reject the client immediately.
				close(c.send)
				continue
			}
			h.clients[c] = struct{}{}
			h.clientCount.Store(int64(len(h.clients)))

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				close(c.send)
				delete(h.clients, c)
				h.clientCount.Store(int64(len(h.clients)))
			}

		case evt := <-h.broadcast:
			for c := range h.clients {
				select {
				case c.send <- evt:
					// Delivered.
				default:
					// Slow consumer — drop the client.
					close(c.send)
					delete(h.clients, c)
				}
			}
			h.clientCount.Store(int64(len(h.clients)))
		}
	}
}

// Stop shuts down the hub's event loop. It is safe to call multiple times.
// After Stop returns, no further events will be delivered.
func (h *EventHub) Stop() {
	h.stopOnce.Do(func() {
		close(h.done)
	})
}

// Broadcast sends an event to all connected clients. It is safe to call from
// any goroutine. If the hub's internal broadcast buffer is full the call
// drops the event silently (non-blocking) — this protects producers from
// being slowed by WebSocket delivery.
func (h *EventHub) Broadcast(evt Event) {
	select {
	case h.broadcast <- evt:
	default:
		// Hub broadcast buffer full — drop the event. The REST API remains
		// the source of truth; clients will catch up on their next fetch.
	}
}

// Register adds a new client to the hub and returns it. The caller is
// responsible for reading from the client's send channel and writing frames
// to the WebSocket. When the connection closes, the caller must call
// Unregister.
//
// If the hub has been stopped or is at capacity, the returned client's send
// channel will be closed immediately.
func (h *EventHub) Register() *client {
	c := &client{
		send: make(chan Event, h.sendBufferSize),
	}
	select {
	case h.register <- c:
	case <-h.done:
		// Hub already stopped.
		close(c.send)
	}
	return c
}

// Unregister removes a client from the hub. It is safe to call multiple
// times or after the hub has been stopped.
func (h *EventHub) Unregister(c *client) {
	select {
	case h.unregister <- c:
	case <-h.done:
		// Hub already stopped — nothing to do.
	}
}

// ClientCount returns the number of connected WebSocket clients. The value
// is maintained atomically by the run loop so it is safe to call from any
// goroutine, but may be momentarily stale.
func (h *EventHub) ClientCount() int {
	return int(h.clientCount.Load())
}
