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
	"strings"
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
	EventGitRepoStatusChanged  = "git_repo_status_changed"
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

	// Git kitchen events.
	EventGitKitchenRunComplete = "git_kitchen_run_complete"
	EventBatchProgress         = "batch_progress"
	EventBatchComplete         = "batch_complete"
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
// Subscription-gated event types
// ---------------------------------------------------------------------------

// subscriptionGatedEvents is the set of event types that require an explicit
// client subscription before delivery. Events not in this set are broadcast
// unconditionally to all connected clients (preserving existing behaviour).
var subscriptionGatedEvents = map[string]bool{
	EventLogEntry: true,
}

// IsSubscriptionGated reports whether the given event type requires a client
// subscription before delivery.
func IsSubscriptionGated(eventType string) bool {
	return subscriptionGatedEvents[eventType]
}

// ---------------------------------------------------------------------------
// Subscription — per-client, per-event-type filter.
// ---------------------------------------------------------------------------

// Subscription holds per-event-type filter criteria. Currently only
// MinSeverity is supported (for log_entry events).
type Subscription struct {
	// MinSeverity is the minimum severity level for log_entry events.
	// Empty string means no filter (deliver all severities).
	MinSeverity string
}

// severityValue maps a severity string to a numeric value for comparison.
// Unknown values default to 0 (DEBUG) so that unknown severities are never
// filtered out.
func severityValue(s string) int {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return 0
	case "INFO":
		return 1
	case "WARN":
		return 2
	case "ERROR":
		return 3
	default:
		return 0
	}
}

// IsValidSeverity reports whether s is a recognised severity string.
func IsValidSeverity(s string) bool {
	switch strings.ToUpper(s) {
	case "DEBUG", "INFO", "WARN", "ERROR":
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Client — represents a single connected WebSocket client.
// ---------------------------------------------------------------------------

// client represents a single WebSocket connection registered with the hub.
// Each client has a buffered send channel; the WebSocket write goroutine
// drains this channel and writes JSON frames to the connection.
//
// Clients may subscribe to specific event types (e.g. log_entry) with
// optional filters. Subscription-gated events are only delivered to clients
// that have an active subscription for that event type.
type client struct {
	send chan Event

	// subscriptions tracks which event types this client has opted in to.
	// Only relevant for subscription-gated event types.
	subscriptions map[string]Subscription
	mu            sync.RWMutex
}

// Subscribe adds or updates a subscription for the given event type.
func (c *client) Subscribe(eventType string, sub Subscription) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subscriptions == nil {
		c.subscriptions = make(map[string]Subscription)
	}
	c.subscriptions[eventType] = sub
}

// Unsubscribe removes the subscription for the given event type.
func (c *client) Unsubscribe(eventType string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subscriptions, eventType)
}

// IsSubscribed reports whether the client has a subscription for the given
// event type.
func (c *client) IsSubscribed(eventType string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.subscriptions[eventType]
	return ok
}

// GetSubscription returns the subscription for the given event type and a
// boolean indicating whether one exists.
func (c *client) GetSubscription(eventType string) (Subscription, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sub, ok := c.subscriptions[eventType]
	return sub, ok
}

// shouldReceive reports whether this client should receive the given event.
// For non-subscription-gated events, it always returns true. For gated
// events, it checks subscriptions and applies filters.
func (c *client) shouldReceive(evt Event) bool {
	if !IsSubscriptionGated(evt.Type) {
		return true
	}

	sub, ok := c.GetSubscription(evt.Type)
	if !ok {
		return false
	}

	// Apply severity filter for log_entry events.
	if evt.Type == EventLogEntry && sub.MinSeverity != "" {
		data, ok := evt.Data.(map[string]any)
		if ok {
			if sev, ok := data["severity"].(string); ok {
				if severityValue(sev) < severityValue(sub.MinSeverity) {
					return false
				}
			}
		}
	}

	return true
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
// For subscription-gated event types (e.g. log_entry), events are only
// delivered to clients that have explicitly subscribed with matching filters.
// Non-gated events are broadcast unconditionally to all clients.
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
				// For subscription-gated events, check whether this
				// client should receive the event before queuing.
				if !c.shouldReceive(evt) {
					continue
				}
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
//
// For subscription-gated event types (e.g. log_entry), the run loop will
// check each client's subscriptions and filters before delivering the event.
// Non-gated events are delivered unconditionally.
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
		send:          make(chan Event, h.sendBufferSize),
		subscriptions: make(map[string]Subscription),
	}
	select {
	case h.register <- c:
		// The register channel is buffered, so this case can succeed even
		// after Run() has exited (the write lands in the buffer but nobody
		// will ever read it). Re-check done to detect that situation and
		// close the send channel so callers don't block forever.
		select {
		case <-h.done:
			close(c.send)
		default:
		}
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
