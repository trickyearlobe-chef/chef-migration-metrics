// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeLogRetentionStore struct {
	cutoffs chan time.Time

	mu   sync.Mutex
	err  error
	rows int64
}

func (f *fakeLogRetentionStore) PurgeLogEntryPartitions(ctx context.Context, olderThan time.Time) (int, error) {
	f.mu.Lock()
	err, rows := f.err, f.rows
	f.mu.Unlock()

	select {
	case f.cutoffs <- olderThan:
	default:
	}
	return int(rows), err
}

func (f *fakeLogRetentionStore) setErr(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

func newFakeStore() *fakeLogRetentionStore {
	return &fakeLogRetentionStore{cutoffs: make(chan time.Time, 8)}
}

// Expiry must not wait for the first tick: a process that restarts more often
// than the interval would otherwise never purge at all.
func TestStartLogRetentionTicker_PurgesOnStart(t *testing.T) {
	store := newFakeStore()
	stop := StartRetentionTicker(store, func() int { return 90 }, time.Hour, nil)
	defer stop()

	select {
	case cutoff := <-store.cutoffs:
		want := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -90)
		if !cutoff.Equal(want) {
			t.Errorf("cutoff = %v, want %v (UTC midnight, 90 days ago)", cutoff, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("purge was not called on start")
	}
}

// The whole point of the change: expiry runs on its own schedule rather than
// only at the tail of a collection run.
func TestStartLogRetentionTicker_PurgesOnEachTick(t *testing.T) {
	store := newFakeStore()
	stop := StartRetentionTicker(store, func() int { return 30 }, 50*time.Millisecond, nil)
	defer stop()

	for i := 0; i < 3; i++ {
		select {
		case <-store.cutoffs:
		case <-time.After(2 * time.Second):
			t.Fatalf("purge %d did not happen", i+1)
		}
	}
}

// Retention is read live each tick so a change made in the UI takes effect
// without a restart, per the dynamic-configuration rule.
func TestStartLogRetentionTicker_ReadsRetentionLive(t *testing.T) {
	store := newFakeStore()

	var mu sync.Mutex
	days := 90
	stop := StartRetentionTicker(store, func() int {
		mu.Lock()
		defer mu.Unlock()
		return days
	}, 50*time.Millisecond, nil)
	defer stop()

	select {
	case cutoff := <-store.cutoffs:
		want := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -90)
		if !cutoff.Equal(want) {
			t.Fatalf("first cutoff = %v, want %v", cutoff, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no purge on start")
	}

	mu.Lock()
	days = 7
	mu.Unlock()

	want := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -7)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case cutoff := <-store.cutoffs:
			if cutoff.Equal(want) {
				return
			}
		case <-deadline:
			t.Fatal("retention change was not picked up without a restart")
		}
	}
}

// Retention of zero means "keep everything" — the existing collector gate was
// `RetentionDays > 0`. Purging on a zero would delete the entire log.
func TestStartLogRetentionTicker_ZeroDaysDisablesPurge(t *testing.T) {
	for _, days := range []int{0, -1} {
		store := newFakeStore()
		stop := StartRetentionTicker(store, func() int { return days }, 20*time.Millisecond, nil)

		select {
		case cutoff := <-store.cutoffs:
			stop()
			t.Fatalf("retention_days=%d purged with cutoff %v; expected no purge", days, cutoff)
		case <-time.After(300 * time.Millisecond):
		}
		stop()
	}
}

// A failing purge must not kill the loop — the next tick has to try again, or a
// transient database error would silently disable expiry until restart.
func TestStartLogRetentionTicker_SurvivesPurgeErrors(t *testing.T) {
	store := newFakeStore()
	store.setErr(errors.New("connection reset"))

	var (
		mu   sync.Mutex
		logs []string
	)
	stop := StartRetentionTicker(store, func() int { return 30 }, 50*time.Millisecond, func(level, msg string) {
		mu.Lock()
		logs = append(logs, level)
		mu.Unlock()
	})
	defer stop()

	for i := 0; i < 2; i++ {
		select {
		case <-store.cutoffs:
		case <-time.After(2 * time.Second):
			t.Fatalf("purge %d did not happen after an error", i+1)
		}
	}

	store.setErr(nil)

	mu.Lock()
	defer mu.Unlock()
	if len(logs) == 0 {
		t.Error("purge failure was not logged")
	}
	for _, level := range logs {
		if level == "ERROR" {
			return
		}
	}
	t.Errorf("expected an ERROR log for the purge failure, got %v", logs)
}

func TestStartLogRetentionTicker_StopHaltsTheLoop(t *testing.T) {
	store := newFakeStore()
	stop := StartRetentionTicker(store, func() int { return 30 }, 20*time.Millisecond, nil)

	select {
	case <-store.cutoffs:
	case <-time.After(2 * time.Second):
		t.Fatal("no purge on start")
	}

	stop()
	// Drain anything already queued, then confirm the loop is quiet.
	time.Sleep(100 * time.Millisecond)
	for len(store.cutoffs) > 0 {
		<-store.cutoffs
	}

	select {
	case cutoff := <-store.cutoffs:
		t.Errorf("purge ran after stop (cutoff %v)", cutoff)
	case <-time.After(200 * time.Millisecond):
	}
}
