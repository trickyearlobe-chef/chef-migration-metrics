// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package perf

import (
	"sort"
	"sync"
	"time"
)

// KeyStats holds aggregated statistics for a single tracked key.
type KeyStats struct {
	Key        string
	Count      int
	ErrorCount int
	P50        time.Duration
	P95        time.Duration
	P99        time.Duration
	Max        time.Duration
}

// sample is a single latency observation with a timestamp for window expiry.
type sample struct {
	d  time.Duration
	ts time.Time
}

// keyBuffer is a circular buffer of samples for one key.
type keyBuffer struct {
	samples    []sample
	head       int
	len        int
	errorCount int
}

func newKeyBuffer(capacity int) *keyBuffer {
	return &keyBuffer{
		samples: make([]sample, capacity),
	}
}

func (kb *keyBuffer) add(d time.Duration, ts time.Time) {
	kb.samples[kb.head] = sample{d: d, ts: ts}
	kb.head = (kb.head + 1) % len(kb.samples)
	if kb.len < len(kb.samples) {
		kb.len++
	}
}

// activeSamples returns samples that are within the given window relative to
// now. The returned slice is freshly allocated and sorted by duration.
func (kb *keyBuffer) activeSamples(now time.Time, window time.Duration) []time.Duration {
	cutoff := now.Add(-window)
	active := make([]time.Duration, 0, kb.len)
	capacity := len(kb.samples)
	start := kb.head - kb.len
	if start < 0 {
		start += capacity
	}
	for i := 0; i < kb.len; i++ {
		idx := (start + i) % capacity
		s := kb.samples[idx]
		if s.ts.After(cutoff) {
			active = append(active, s.d)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i] < active[j] })
	return active
}

// Recorder tracks per-key latency samples in a rolling time window.
// It is safe for concurrent use.
type Recorder struct {
	mu               sync.Mutex
	window           time.Duration
	maxKeys          int
	maxSamplesPerKey int
	buffers          map[string]*keyBuffer
}

// NewRecorder creates a Recorder with the given window duration, maximum
// number of tracked keys, and maximum samples retained per key.
func NewRecorder(window time.Duration, maxKeys, maxSamplesPerKey int) *Recorder {
	return &Recorder{
		window:           window,
		maxKeys:          maxKeys,
		maxSamplesPerKey: maxSamplesPerKey,
		buffers:          make(map[string]*keyBuffer),
	}
}

// Record stores a latency sample for the given key. If the maximum number of
// keys has been reached and the key is new, the sample is silently dropped.
func (r *Recorder) Record(key string, d time.Duration) {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	kb, exists := r.buffers[key]
	if !exists {
		if len(r.buffers) >= r.maxKeys {
			return
		}
		kb = newKeyBuffer(r.maxSamplesPerKey)
		r.buffers[key] = kb
	}
	kb.add(d, now)
}

// RecordError increments the error counter for a key. The key must already
// exist (created via Record); if it does not, the call is a no-op.
func (r *Recorder) RecordError(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if kb, ok := r.buffers[key]; ok {
		kb.errorCount++
	}
}

// Snapshot returns a point-in-time copy of stats for all keys, sorted by P95
// descending. Samples older than the configured window are excluded from
// percentile calculations. Keys with zero active samples are omitted.
func (r *Recorder) Snapshot() []KeyStats {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	result := make([]KeyStats, 0, len(r.buffers))

	for key, kb := range r.buffers {
		active := kb.activeSamples(now, r.window)
		if len(active) == 0 {
			continue
		}
		ks := KeyStats{
			Key:        key,
			Count:      len(active),
			ErrorCount: kb.errorCount,
			P50:        percentile(active, 0.50),
			P95:        percentile(active, 0.95),
			P99:        percentile(active, 0.99),
			Max:        active[len(active)-1],
		}
		result = append(result, ks)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].P95 > result[j].P95
	})

	return result
}

// Reset clears all recorded data.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buffers = make(map[string]*keyBuffer)
}

// percentile returns the value at the given percentile p (0.0–1.0) from a
// sorted slice of durations. The slice must be non-empty and sorted ascending.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	// Use nearest-rank method.
	rank := p * float64(len(sorted)-1)
	idx := int(rank + 0.5)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
