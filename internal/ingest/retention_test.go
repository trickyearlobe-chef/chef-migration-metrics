// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ingest

import (
	"context"
	"testing"
	"time"
)

type fakeRetentionStore struct {
	cutoffs chan time.Time
}

func (f fakeRetentionStore) PurgeConvergeRunPartitions(ctx context.Context, olderThan time.Time) (int, error) {
	f.cutoffs <- olderThan
	return 0, nil
}

// The ticker purges immediately on start (so a restart reclaims promptly) with a
// cutoff at UTC midnight retention_days ago.
func TestStartRetentionTicker_PurgesOnStartWithCutoff(t *testing.T) {
	store := fakeRetentionStore{cutoffs: make(chan time.Time, 4)}
	stop := StartRetentionTicker(store, func() int { return 2 }, time.Hour, nil)
	defer stop()

	select {
	case cutoff := <-store.cutoffs:
		want := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -2)
		if !cutoff.Equal(want) {
			t.Errorf("cutoff = %v, want %v (UTC midnight, 2 days ago)", cutoff, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("purge was not called on start")
	}
}

// A zero/invalid retention_days falls back to 2 days, never purging everything.
func TestStartRetentionTicker_ZeroDaysFallsBack(t *testing.T) {
	store := fakeRetentionStore{cutoffs: make(chan time.Time, 4)}
	stop := StartRetentionTicker(store, func() int { return 0 }, time.Hour, nil)
	defer stop()

	select {
	case cutoff := <-store.cutoffs:
		want := time.Now().UTC().Truncate(24 * time.Hour).AddDate(0, 0, -2)
		if !cutoff.Equal(want) {
			t.Errorf("cutoff = %v, want %v (fallback 2 days)", cutoff, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("purge was not called on start")
	}
}
