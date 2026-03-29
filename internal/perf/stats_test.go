// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package perf

import (
	"sync"
	"testing"
	"time"
)

func TestRecorder_BasicRecording(t *testing.T) {
	r := NewRecorder(10*time.Second, 100, 1000)

	r.Record("api/endpoint", 10*time.Millisecond)
	r.Record("api/endpoint", 20*time.Millisecond)
	r.Record("api/endpoint", 30*time.Millisecond)

	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 key, got %d", len(snap))
	}

	s := snap[0]
	if s.Key != "api/endpoint" {
		t.Errorf("expected key %q, got %q", "api/endpoint", s.Key)
	}
	if s.Count != 3 {
		t.Errorf("expected count 3, got %d", s.Count)
	}
	if s.ErrorCount != 0 {
		t.Errorf("expected error count 0, got %d", s.ErrorCount)
	}
	if s.P50 != 20*time.Millisecond {
		t.Errorf("expected P50 20ms, got %v", s.P50)
	}
	if s.P95 != 30*time.Millisecond {
		t.Errorf("expected P95 30ms, got %v", s.P95)
	}
	if s.P99 != 30*time.Millisecond {
		t.Errorf("expected P99 30ms, got %v", s.P99)
	}
	if s.Max != 30*time.Millisecond {
		t.Errorf("expected Max 30ms, got %v", s.Max)
	}
}

func TestRecorder_MultipleKeys(t *testing.T) {
	r := NewRecorder(10*time.Second, 100, 1000)

	r.Record("fast", 1*time.Millisecond)
	r.Record("fast", 2*time.Millisecond)
	r.Record("fast", 3*time.Millisecond)

	r.Record("slow", 100*time.Millisecond)
	r.Record("slow", 200*time.Millisecond)
	r.Record("slow", 300*time.Millisecond)

	snap := r.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(snap))
	}

	if snap[0].Key != "slow" {
		t.Errorf("expected first key %q (highest P95), got %q", "slow", snap[0].Key)
	}
	if snap[1].Key != "fast" {
		t.Errorf("expected second key %q, got %q", "fast", snap[1].Key)
	}
}

func TestRecorder_WindowExpiry(t *testing.T) {
	r := NewRecorder(100*time.Millisecond, 100, 1000)

	r.Record("key", 5*time.Millisecond)
	time.Sleep(150 * time.Millisecond)
	r.Record("key", 10*time.Millisecond)

	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 key, got %d", len(snap))
	}
	if snap[0].Count != 1 {
		t.Errorf("expected count 1 (expired sample excluded), got %d", snap[0].Count)
	}
}

func TestRecorder_ConcurrentRecording(t *testing.T) {
	r := NewRecorder(10*time.Second, 100, 10000)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r.Record("concurrent", 1*time.Millisecond)
			}
		}()
	}
	wg.Wait()

	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 key, got %d", len(snap))
	}
	if snap[0].Count != 1000 {
		t.Errorf("expected count 1000, got %d", snap[0].Count)
	}
}

func TestRecorder_Reset(t *testing.T) {
	r := NewRecorder(10*time.Second, 100, 1000)

	r.Record("a", 10*time.Millisecond)
	r.Record("b", 20*time.Millisecond)
	r.Reset()

	snap := r.Snapshot()
	if len(snap) != 0 {
		t.Errorf("expected empty snapshot after Reset, got %d keys", len(snap))
	}
}

func TestRecorder_MaxKeys(t *testing.T) {
	r := NewRecorder(10*time.Second, 3, 1000)

	r.Record("key1", 1*time.Millisecond)
	r.Record("key2", 2*time.Millisecond)
	r.Record("key3", 3*time.Millisecond)
	r.Record("key4", 4*time.Millisecond)
	r.Record("key5", 5*time.Millisecond)

	snap := r.Snapshot()
	if len(snap) > 3 {
		t.Errorf("expected at most 3 keys, got %d", len(snap))
	}
}

func TestRecorder_Percentiles_LargeSample(t *testing.T) {
	r := NewRecorder(10*time.Second, 100, 1000)

	for i := 1; i <= 100; i++ {
		r.Record("key", time.Duration(i)*time.Millisecond)
	}

	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 key, got %d", len(snap))
	}

	s := snap[0]
	if s.P50 < 49*time.Millisecond || s.P50 > 51*time.Millisecond {
		t.Errorf("expected P50 between 49ms-51ms, got %v", s.P50)
	}
	if s.P95 < 94*time.Millisecond || s.P95 > 96*time.Millisecond {
		t.Errorf("expected P95 between 94ms-96ms, got %v", s.P95)
	}
	if s.P99 < 98*time.Millisecond || s.P99 > 100*time.Millisecond {
		t.Errorf("expected P99 between 98ms-100ms, got %v", s.P99)
	}
	if s.Max != 100*time.Millisecond {
		t.Errorf("expected Max 100ms, got %v", s.Max)
	}
}

func TestRecorder_SingleSample(t *testing.T) {
	r := NewRecorder(10*time.Second, 100, 1000)

	r.Record("only", 42*time.Millisecond)

	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 key, got %d", len(snap))
	}

	s := snap[0]
	if s.Count != 1 {
		t.Errorf("expected count 1, got %d", s.Count)
	}
	if s.P50 != 42*time.Millisecond {
		t.Errorf("expected P50 42ms, got %v", s.P50)
	}
	if s.P95 != 42*time.Millisecond {
		t.Errorf("expected P95 42ms, got %v", s.P95)
	}
	if s.P99 != 42*time.Millisecond {
		t.Errorf("expected P99 42ms, got %v", s.P99)
	}
	if s.Max != 42*time.Millisecond {
		t.Errorf("expected Max 42ms, got %v", s.Max)
	}
}

func TestRecorder_EmptySnapshot(t *testing.T) {
	r := NewRecorder(10*time.Second, 100, 1000)

	snap := r.Snapshot()
	if snap == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(snap) != 0 {
		t.Errorf("expected 0 keys, got %d", len(snap))
	}
}
