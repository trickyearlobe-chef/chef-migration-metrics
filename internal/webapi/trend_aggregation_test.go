// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// truncateToHour
// ---------------------------------------------------------------------------

func TestTruncateToHour(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			name: "already on the hour",
			in:   time.Date(2025, 6, 15, 14, 0, 0, 0, time.UTC),
			want: time.Date(2025, 6, 15, 14, 0, 0, 0, time.UTC),
		},
		{
			name: "mid-hour",
			in:   time.Date(2025, 6, 15, 14, 37, 42, 123456789, time.UTC),
			want: time.Date(2025, 6, 15, 14, 0, 0, 0, time.UTC),
		},
		{
			name: "midnight",
			in:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			want: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "last second of hour",
			in:   time.Date(2025, 6, 15, 23, 59, 59, 999999999, time.UTC),
			want: time.Date(2025, 6, 15, 23, 0, 0, 0, time.UTC),
		},
		{
			name: "non-UTC timezone is converted",
			in:   time.Date(2025, 6, 15, 10, 30, 0, 0, time.FixedZone("EST", -5*3600)),
			want: time.Date(2025, 6, 15, 15, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateToHour(tc.in)
			if !got.Equal(tc.want) {
				t.Errorf("truncateToHour(%v) = %v, want %v", tc.in, got, tc.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("truncateToHour result location = %v, want UTC", got.Location())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// mergeVersionDistributionSnapshots
// ---------------------------------------------------------------------------

func TestMergeVersionDistributionSnapshots_Empty(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		got := mergeVersionDistributionSnapshots(nil)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
	t.Run("empty slice", func(t *testing.T) {
		got := mergeVersionDistributionSnapshots([]versionDistTrendPoint{})
		if len(got) != 0 {
			t.Errorf("expected empty slice, got %d items", len(got))
		}
	})
}

func TestMergeVersionDistributionSnapshots_SingleOrg(t *testing.T) {
	points := []versionDistTrendPoint{
		{
			OrganisationName: "org-a",
			CollectionRunOrg: "org-a",
			CompletedAt:      "2025-06-15T14:05:00Z",
			TotalNodes:       100,
			Distribution:     map[string]int{"18.4.2": 60, "17.10.0": 40},
		},
		{
			OrganisationName: "org-a",
			CollectionRunOrg: "org-a",
			CompletedAt:      "2025-06-15T15:05:00Z",
			TotalNodes:       100,
			Distribution:     map[string]int{"18.4.2": 65, "17.10.0": 35},
		},
	}

	got := mergeVersionDistributionSnapshots(points)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}

	// First bucket: 14:00
	if got[0].CompletedAt != "2025-06-15T14:00:00Z" {
		t.Errorf("got[0].CompletedAt = %q, want %q", got[0].CompletedAt, "2025-06-15T14:00:00Z")
	}
	if got[0].TotalNodes != 100 {
		t.Errorf("got[0].TotalNodes = %d, want 100", got[0].TotalNodes)
	}
	if got[0].Distribution["18.4.2"] != 60 {
		t.Errorf("got[0].Distribution[18.4.2] = %d, want 60", got[0].Distribution["18.4.2"])
	}
	if got[0].Distribution["17.10.0"] != 40 {
		t.Errorf("got[0].Distribution[17.10.0] = %d, want 40", got[0].Distribution["17.10.0"])
	}
	if got[0].OrganisationName != "" {
		t.Errorf("got[0].OrganisationName = %q, want empty", got[0].OrganisationName)
	}
	if got[0].CollectionRunOrg != "" {
		t.Errorf("got[0].CollectionRunOrg = %q, want empty", got[0].CollectionRunOrg)
	}

	// Second bucket: 15:00
	if got[1].CompletedAt != "2025-06-15T15:00:00Z" {
		t.Errorf("got[1].CompletedAt = %q, want %q", got[1].CompletedAt, "2025-06-15T15:00:00Z")
	}
	if got[1].TotalNodes != 100 {
		t.Errorf("got[1].TotalNodes = %d, want 100", got[1].TotalNodes)
	}
}

func TestMergeVersionDistributionSnapshots_MultipleOrgs_SameHour(t *testing.T) {
	points := []versionDistTrendPoint{
		{
			OrganisationName: "org-a",
			CollectionRunOrg: "org-a",
			CompletedAt:      "2025-06-15T14:02:00Z",
			TotalNodes:       100,
			Distribution:     map[string]int{"18.4.2": 60, "17.10.0": 40},
		},
		{
			OrganisationName: "org-b",
			CollectionRunOrg: "org-b",
			CompletedAt:      "2025-06-15T14:07:00Z",
			TotalNodes:       50,
			Distribution:     map[string]int{"18.4.2": 30, "16.0.0": 20},
		},
	}

	got := mergeVersionDistributionSnapshots(points)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}

	p := got[0]
	if p.CompletedAt != "2025-06-15T14:00:00Z" {
		t.Errorf("CompletedAt = %q, want %q", p.CompletedAt, "2025-06-15T14:00:00Z")
	}
	if p.TotalNodes != 150 {
		t.Errorf("TotalNodes = %d, want 150", p.TotalNodes)
	}
	if p.Distribution["18.4.2"] != 90 {
		t.Errorf("Distribution[18.4.2] = %d, want 90", p.Distribution["18.4.2"])
	}
	if p.Distribution["17.10.0"] != 40 {
		t.Errorf("Distribution[17.10.0] = %d, want 40", p.Distribution["17.10.0"])
	}
	if p.Distribution["16.0.0"] != 20 {
		t.Errorf("Distribution[16.0.0] = %d, want 20", p.Distribution["16.0.0"])
	}
	if p.OrganisationName != "" {
		t.Errorf("OrganisationName = %q, want empty", p.OrganisationName)
	}
	if p.CollectionRunOrg != "" {
		t.Errorf("CollectionRunOrg = %q, want empty", p.CollectionRunOrg)
	}
}

func TestMergeVersionDistributionSnapshots_MultipleOrgs_DifferentHours(t *testing.T) {
	points := []versionDistTrendPoint{
		{
			OrganisationName: "org-a",
			CollectionRunOrg: "org-a",
			CompletedAt:      "2025-06-15T14:02:00Z",
			TotalNodes:       100,
			Distribution:     map[string]int{"18.4.2": 100},
		},
		{
			OrganisationName: "org-b",
			CollectionRunOrg: "org-b",
			CompletedAt:      "2025-06-15T14:07:00Z",
			TotalNodes:       50,
			Distribution:     map[string]int{"18.4.2": 50},
		},
		{
			OrganisationName: "org-a",
			CollectionRunOrg: "org-a",
			CompletedAt:      "2025-06-15T15:03:00Z",
			TotalNodes:       110,
			Distribution:     map[string]int{"18.4.2": 110},
		},
		{
			OrganisationName: "org-b",
			CollectionRunOrg: "org-b",
			CompletedAt:      "2025-06-15T15:08:00Z",
			TotalNodes:       55,
			Distribution:     map[string]int{"18.4.2": 55},
		},
	}

	got := mergeVersionDistributionSnapshots(points)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}

	// Verify ascending sort.
	if got[0].CompletedAt != "2025-06-15T14:00:00Z" {
		t.Errorf("got[0].CompletedAt = %q, want %q", got[0].CompletedAt, "2025-06-15T14:00:00Z")
	}
	if got[1].CompletedAt != "2025-06-15T15:00:00Z" {
		t.Errorf("got[1].CompletedAt = %q, want %q", got[1].CompletedAt, "2025-06-15T15:00:00Z")
	}

	// Hour 14: 100 + 50
	if got[0].TotalNodes != 150 {
		t.Errorf("got[0].TotalNodes = %d, want 150", got[0].TotalNodes)
	}
	if got[0].Distribution["18.4.2"] != 150 {
		t.Errorf("got[0].Distribution[18.4.2] = %d, want 150", got[0].Distribution["18.4.2"])
	}

	// Hour 15: 110 + 55
	if got[1].TotalNodes != 165 {
		t.Errorf("got[1].TotalNodes = %d, want 165", got[1].TotalNodes)
	}
	if got[1].Distribution["18.4.2"] != 165 {
		t.Errorf("got[1].Distribution[18.4.2] = %d, want 165", got[1].Distribution["18.4.2"])
	}
}

// ---------------------------------------------------------------------------
// mergeStaleTrendSnapshots
// ---------------------------------------------------------------------------

func TestMergeStaleTrendSnapshots_Empty(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		got := mergeStaleTrendSnapshots(nil)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
	t.Run("empty slice", func(t *testing.T) {
		got := mergeStaleTrendSnapshots([]staleTrendPoint{})
		if len(got) != 0 {
			t.Errorf("expected empty slice, got %d items", len(got))
		}
	})
}

func TestMergeStaleTrendSnapshots_MultipleOrgs_SameHour(t *testing.T) {
	points := []staleTrendPoint{
		{
			OrganisationName: "org-a",
			CollectionRunOrg: "org-a",
			CompletedAt:      "2025-06-15T14:02:00Z",
			TotalNodes:       100,
			StaleNodes:       20,
			FreshNodes:       80,
		},
		{
			OrganisationName: "org-b",
			CollectionRunOrg: "org-b",
			CompletedAt:      "2025-06-15T14:07:00Z",
			TotalNodes:       50,
			StaleNodes:       10,
			FreshNodes:       40,
		},
		{
			OrganisationName: "org-c",
			CollectionRunOrg: "org-c",
			CompletedAt:      "2025-06-15T14:12:00Z",
			TotalNodes:       200,
			StaleNodes:       50,
			FreshNodes:       150,
		},
	}

	got := mergeStaleTrendSnapshots(points)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}

	p := got[0]
	if p.CompletedAt != "2025-06-15T14:00:00Z" {
		t.Errorf("CompletedAt = %q, want %q", p.CompletedAt, "2025-06-15T14:00:00Z")
	}
	if p.TotalNodes != 350 {
		t.Errorf("TotalNodes = %d, want 350", p.TotalNodes)
	}
	if p.StaleNodes != 80 {
		t.Errorf("StaleNodes = %d, want 80", p.StaleNodes)
	}
	if p.FreshNodes != 270 {
		t.Errorf("FreshNodes = %d, want 270", p.FreshNodes)
	}
	if p.OrganisationName != "" {
		t.Errorf("OrganisationName = %q, want empty", p.OrganisationName)
	}
	if p.CollectionRunOrg != "" {
		t.Errorf("CollectionRunOrg = %q, want empty", p.CollectionRunOrg)
	}
}

func TestMergeStaleTrendSnapshots_DifferentHours_Sorted(t *testing.T) {
	points := []staleTrendPoint{
		{
			OrganisationName: "org-a",
			CollectionRunOrg: "org-a",
			CompletedAt:      "2025-06-15T16:01:00Z",
			TotalNodes:       10,
			StaleNodes:       5,
			FreshNodes:       5,
		},
		{
			OrganisationName: "org-a",
			CollectionRunOrg: "org-a",
			CompletedAt:      "2025-06-15T14:01:00Z",
			TotalNodes:       20,
			StaleNodes:       10,
			FreshNodes:       10,
		},
	}

	got := mergeStaleTrendSnapshots(points)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].CompletedAt != "2025-06-15T14:00:00Z" {
		t.Errorf("got[0].CompletedAt = %q, want %q", got[0].CompletedAt, "2025-06-15T14:00:00Z")
	}
	if got[1].CompletedAt != "2025-06-15T16:00:00Z" {
		t.Errorf("got[1].CompletedAt = %q, want %q", got[1].CompletedAt, "2025-06-15T16:00:00Z")
	}
}

// ---------------------------------------------------------------------------
// mergeReadinessTrendSnapshots
// ---------------------------------------------------------------------------

func TestMergeReadinessTrendSnapshots_Empty(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		got := mergeReadinessTrendSnapshots(nil)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
	t.Run("empty slice", func(t *testing.T) {
		got := mergeReadinessTrendSnapshots([]readinessTrendPoint{})
		if len(got) != 0 {
			t.Errorf("expected empty slice, got %d items", len(got))
		}
	})
}

func TestMergeReadinessTrendSnapshots_MultipleOrgs_SameHour(t *testing.T) {
	points := []readinessTrendPoint{
		{
			OrganisationName:  "org-a",
			CollectionRunOrg:  "org-a",
			CompletedAt:       "2025-06-15T14:02:00Z",
			TargetChefVersion: "18.4.2",
			TotalNodes:        100,
			ReadyNodes:        80,
			BlockedNodes:      20,
			ReadyPercent:      80.0,
		},
		{
			OrganisationName:  "org-b",
			CollectionRunOrg:  "org-b",
			CompletedAt:       "2025-06-15T14:07:00Z",
			TargetChefVersion: "18.4.2",
			TotalNodes:        50,
			ReadyNodes:        10,
			BlockedNodes:      40,
			ReadyPercent:      20.0,
		},
	}

	got := mergeReadinessTrendSnapshots(points)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}

	p := got[0]
	if p.CompletedAt != "2025-06-15T14:00:00Z" {
		t.Errorf("CompletedAt = %q, want %q", p.CompletedAt, "2025-06-15T14:00:00Z")
	}
	if p.TargetChefVersion != "18.4.2" {
		t.Errorf("TargetChefVersion = %q, want %q", p.TargetChefVersion, "18.4.2")
	}
	if p.TotalNodes != 150 {
		t.Errorf("TotalNodes = %d, want 150", p.TotalNodes)
	}
	if p.ReadyNodes != 90 {
		t.Errorf("ReadyNodes = %d, want 90", p.ReadyNodes)
	}
	if p.BlockedNodes != 60 {
		t.Errorf("BlockedNodes = %d, want 60", p.BlockedNodes)
	}
	// ReadyPercent should be recomputed: 90/150 * 100 = 60.0
	if p.ReadyPercent != 60.0 {
		t.Errorf("ReadyPercent = %f, want 60.0", p.ReadyPercent)
	}
	if p.OrganisationName != "" {
		t.Errorf("OrganisationName = %q, want empty", p.OrganisationName)
	}
	if p.CollectionRunOrg != "" {
		t.Errorf("CollectionRunOrg = %q, want empty", p.CollectionRunOrg)
	}
}

func TestMergeReadinessTrendSnapshots_DifferentVersions_SameHour(t *testing.T) {
	points := []readinessTrendPoint{
		{
			OrganisationName:  "org-a",
			CollectionRunOrg:  "org-a",
			CompletedAt:       "2025-06-15T14:02:00Z",
			TargetChefVersion: "18.4.2",
			TotalNodes:        100,
			ReadyNodes:        80,
			BlockedNodes:      20,
			ReadyPercent:      80.0,
		},
		{
			OrganisationName:  "org-a",
			CollectionRunOrg:  "org-a",
			CompletedAt:       "2025-06-15T14:02:00Z",
			TargetChefVersion: "17.10.0",
			TotalNodes:        100,
			ReadyNodes:        50,
			BlockedNodes:      50,
			ReadyPercent:      50.0,
		},
		{
			OrganisationName:  "org-b",
			CollectionRunOrg:  "org-b",
			CompletedAt:       "2025-06-15T14:07:00Z",
			TargetChefVersion: "18.4.2",
			TotalNodes:        50,
			ReadyNodes:        20,
			BlockedNodes:      30,
			ReadyPercent:      40.0,
		},
		{
			OrganisationName:  "org-b",
			CollectionRunOrg:  "org-b",
			CompletedAt:       "2025-06-15T14:07:00Z",
			TargetChefVersion: "17.10.0",
			TotalNodes:        50,
			ReadyNodes:        40,
			BlockedNodes:      10,
			ReadyPercent:      80.0,
		},
	}

	got := mergeReadinessTrendSnapshots(points)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}

	// Sorted by (time, version) — "17.10.0" < "18.4.2"
	if got[0].TargetChefVersion != "17.10.0" {
		t.Errorf("got[0].TargetChefVersion = %q, want %q", got[0].TargetChefVersion, "17.10.0")
	}
	if got[1].TargetChefVersion != "18.4.2" {
		t.Errorf("got[1].TargetChefVersion = %q, want %q", got[1].TargetChefVersion, "18.4.2")
	}

	// 17.10.0: 100+50 total, 50+40 ready, 50+10 blocked
	if got[0].TotalNodes != 150 {
		t.Errorf("got[0].TotalNodes = %d, want 150", got[0].TotalNodes)
	}
	if got[0].ReadyNodes != 90 {
		t.Errorf("got[0].ReadyNodes = %d, want 90", got[0].ReadyNodes)
	}
	if got[0].BlockedNodes != 60 {
		t.Errorf("got[0].BlockedNodes = %d, want 60", got[0].BlockedNodes)
	}
	// 90/150 * 100 = 60.0
	if got[0].ReadyPercent != 60.0 {
		t.Errorf("got[0].ReadyPercent = %f, want 60.0", got[0].ReadyPercent)
	}

	// 18.4.2: 100+50 total, 80+20 ready, 20+30 blocked
	if got[1].TotalNodes != 150 {
		t.Errorf("got[1].TotalNodes = %d, want 150", got[1].TotalNodes)
	}
	if got[1].ReadyNodes != 100 {
		t.Errorf("got[1].ReadyNodes = %d, want 100", got[1].ReadyNodes)
	}
	if got[1].BlockedNodes != 50 {
		t.Errorf("got[1].BlockedNodes = %d, want 50", got[1].BlockedNodes)
	}
	// 100/150 * 100 ≈ 66.666...
	wantPct := float64(100) / float64(150) * 100
	if got[1].ReadyPercent != wantPct {
		t.Errorf("got[1].ReadyPercent = %f, want %f", got[1].ReadyPercent, wantPct)
	}
}

func TestMergeReadinessTrendSnapshots_ZeroTotalNodes_ReadyPercentZero(t *testing.T) {
	points := []readinessTrendPoint{
		{
			OrganisationName:  "org-a",
			CollectionRunOrg:  "org-a",
			CompletedAt:       "2025-06-15T14:02:00Z",
			TargetChefVersion: "18.4.2",
			TotalNodes:        0,
			ReadyNodes:        0,
			BlockedNodes:      0,
			ReadyPercent:      0,
		},
	}

	got := mergeReadinessTrendSnapshots(points)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ReadyPercent != 0.0 {
		t.Errorf("ReadyPercent = %f, want 0.0", got[0].ReadyPercent)
	}
}

func TestMergeReadinessTrendSnapshots_DifferentHours_Sorted(t *testing.T) {
	points := []readinessTrendPoint{
		{
			OrganisationName:  "org-a",
			CollectionRunOrg:  "org-a",
			CompletedAt:       "2025-06-15T16:02:00Z",
			TargetChefVersion: "18.4.2",
			TotalNodes:        10,
			ReadyNodes:        5,
			BlockedNodes:      5,
			ReadyPercent:      50.0,
		},
		{
			OrganisationName:  "org-a",
			CollectionRunOrg:  "org-a",
			CompletedAt:       "2025-06-15T14:02:00Z",
			TargetChefVersion: "18.4.2",
			TotalNodes:        20,
			ReadyNodes:        10,
			BlockedNodes:      10,
			ReadyPercent:      50.0,
		},
	}

	got := mergeReadinessTrendSnapshots(points)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].CompletedAt != "2025-06-15T14:00:00Z" {
		t.Errorf("got[0].CompletedAt = %q, want %q", got[0].CompletedAt, "2025-06-15T14:00:00Z")
	}
	if got[1].CompletedAt != "2025-06-15T16:00:00Z" {
		t.Errorf("got[1].CompletedAt = %q, want %q", got[1].CompletedAt, "2025-06-15T16:00:00Z")
	}
}

// ---------------------------------------------------------------------------
// Deduplication tests — same org, two snapshots in same hour bucket.
// Only the latest snapshot per org should be used.
// ---------------------------------------------------------------------------

func TestMergeVersionDistributionSnapshots_DedupSameOrgSameHour(t *testing.T) {
	points := []versionDistTrendPoint{
		{
			OrganisationName: "org-a",
			CollectionRunOrg: "org-a",
			CompletedAt:      "2025-06-15T10:05:00Z",
			TotalNodes:       100,
			Distribution:     map[string]int{"18.0.0": 60, "17.0.0": 40},
		},
		{
			OrganisationName: "org-a",
			CollectionRunOrg: "org-a",
			CompletedAt:      "2025-06-15T10:45:00Z", // later — should win
			TotalNodes:       105,
			Distribution:     map[string]int{"18.0.0": 65, "17.0.0": 40},
		},
		{
			OrganisationName: "org-b",
			CollectionRunOrg: "org-b",
			CompletedAt:      "2025-06-15T10:10:00Z",
			TotalNodes:       50,
			Distribution:     map[string]int{"18.0.0": 50},
		},
	}

	got := mergeVersionDistributionSnapshots(points)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	// org-a latest (105) + org-b (50) = 155
	if got[0].TotalNodes != 155 {
		t.Errorf("TotalNodes = %d, want 155", got[0].TotalNodes)
	}
	// 18.0.0: org-a latest 65 + org-b 50 = 115
	if got[0].Distribution["18.0.0"] != 115 {
		t.Errorf("Distribution[18.0.0] = %d, want 115", got[0].Distribution["18.0.0"])
	}
	// 17.0.0: only org-a latest = 40
	if got[0].Distribution["17.0.0"] != 40 {
		t.Errorf("Distribution[17.0.0] = %d, want 40", got[0].Distribution["17.0.0"])
	}
}

func TestMergeStaleTrendSnapshots_DedupSameOrgSameHour(t *testing.T) {
	points := []staleTrendPoint{
		{
			OrganisationName: "org-a",
			CollectionRunOrg: "org-a",
			CompletedAt:      "2025-06-15T10:05:00Z",
			TotalNodes:       100,
			StaleNodes:       20,
			FreshNodes:       80,
		},
		{
			OrganisationName: "org-a",
			CollectionRunOrg: "org-a",
			CompletedAt:      "2025-06-15T10:45:00Z", // later — should win
			TotalNodes:       100,
			StaleNodes:       18,
			FreshNodes:       82,
		},
		{
			OrganisationName: "org-b",
			CollectionRunOrg: "org-b",
			CompletedAt:      "2025-06-15T10:10:00Z",
			TotalNodes:       50,
			StaleNodes:       5,
			FreshNodes:       45,
		},
	}

	got := mergeStaleTrendSnapshots(points)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	// org-a latest (100) + org-b (50) = 150
	if got[0].TotalNodes != 150 {
		t.Errorf("TotalNodes = %d, want 150", got[0].TotalNodes)
	}
	// stale: org-a latest 18 + org-b 5 = 23
	if got[0].StaleNodes != 23 {
		t.Errorf("StaleNodes = %d, want 23", got[0].StaleNodes)
	}
	// fresh: org-a latest 82 + org-b 45 = 127
	if got[0].FreshNodes != 127 {
		t.Errorf("FreshNodes = %d, want 127", got[0].FreshNodes)
	}
}

func TestMergeReadinessTrendSnapshots_DedupSameOrgSameHour(t *testing.T) {
	points := []readinessTrendPoint{
		{
			OrganisationName:  "org-a",
			CollectionRunOrg:  "org-a",
			CompletedAt:       "2025-06-15T10:05:00Z",
			TargetChefVersion: "18.4.2",
			TotalNodes:        100,
			ReadyNodes:        40,
			BlockedNodes:      60,
			ReadyPercent:      40.0,
		},
		{
			OrganisationName:  "org-a",
			CollectionRunOrg:  "org-a",
			CompletedAt:       "2025-06-15T10:45:00Z", // later — should win
			TargetChefVersion: "18.4.2",
			TotalNodes:        100,
			ReadyNodes:        45,
			BlockedNodes:      55,
			ReadyPercent:      45.0,
		},
		{
			OrganisationName:  "org-b",
			CollectionRunOrg:  "org-b",
			CompletedAt:       "2025-06-15T10:10:00Z",
			TargetChefVersion: "18.4.2",
			TotalNodes:        50,
			ReadyNodes:        25,
			BlockedNodes:      25,
			ReadyPercent:      50.0,
		},
	}

	got := mergeReadinessTrendSnapshots(points)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	// org-a latest (100) + org-b (50) = 150
	if got[0].TotalNodes != 150 {
		t.Errorf("TotalNodes = %d, want 150", got[0].TotalNodes)
	}
	// ready: org-a latest 45 + org-b 25 = 70
	if got[0].ReadyNodes != 70 {
		t.Errorf("ReadyNodes = %d, want 70", got[0].ReadyNodes)
	}
	// blocked: org-a latest 55 + org-b 25 = 80
	if got[0].BlockedNodes != 80 {
		t.Errorf("BlockedNodes = %d, want 80", got[0].BlockedNodes)
	}
	// percent: 70/150 * 100 = 46.666...
	wantPct := float64(70) / float64(150) * 100
	if got[0].ReadyPercent != wantPct {
		t.Errorf("ReadyPercent = %f, want %f", got[0].ReadyPercent, wantPct)
	}
}
