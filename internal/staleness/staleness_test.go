// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package staleness

import (
	"testing"
	"time"
)

func TestComputeTier(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	defaultThresholds := Thresholds{WarningHours: 72, CriticalDays: 7}

	tests := []struct {
		name     string
		ohaiTime time.Time
		thresh   Thresholds
		want     Tier
	}{
		{
			name:     "fresh - 1 hour ago",
			ohaiTime: now.Add(-1 * time.Hour),
			thresh:   defaultThresholds,
			want:     Fresh,
		},
		{
			name:     "warning - 4 days ago",
			ohaiTime: now.Add(-4 * 24 * time.Hour),
			thresh:   defaultThresholds,
			want:     Warning,
		},
		{
			name:     "critical - 10 days ago",
			ohaiTime: now.Add(-10 * 24 * time.Hour),
			thresh:   defaultThresholds,
			want:     Critical,
		},
		{
			name:     "zero ohai_time returns critical",
			ohaiTime: time.Time{},
			thresh:   defaultThresholds,
			want:     Critical,
		},
		{
			name:     "exact warning boundary returns warning",
			ohaiTime: now.Add(-72 * time.Hour),
			thresh:   defaultThresholds,
			want:     Warning,
		},
		{
			name:     "exact critical boundary returns critical",
			ohaiTime: now.Add(-7 * 24 * time.Hour),
			thresh:   defaultThresholds,
			want:     Critical,
		},
		{
			name:     "custom thresholds - 24h warning 3d critical - node at 2d is warning",
			ohaiTime: now.Add(-2 * 24 * time.Hour),
			thresh:   Thresholds{WarningHours: 24, CriticalDays: 3},
			want:     Warning,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeTier(tc.ohaiTime, now, tc.thresh)
			if got != tc.want {
				t.Errorf("ComputeTier() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		name string
		age  time.Duration
		want string
	}{
		{name: "30 minutes", age: 30 * time.Minute, want: "30m"},
		{name: "zero duration", age: 0, want: "1m"},
		{name: "5 hours", age: 5 * time.Hour, want: "5h"},
		{name: "36 hours", age: 36 * time.Hour, want: "36h"},
		{name: "48 hours", age: 48 * time.Hour, want: "2d"},
		{name: "72 hours", age: 72 * time.Hour, want: "3d"},
		{name: "10 days", age: 240 * time.Hour, want: "10d"},
		{name: "100 days", age: 2400 * time.Hour, want: "100d"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatAge(tc.age)
			if got != tc.want {
				t.Errorf("FormatAge(%v) = %q, want %q", tc.age, got, tc.want)
			}
		})
	}
}

func TestIsStaleCompat(t *testing.T) {
	tests := []struct {
		tier Tier
		want bool
	}{
		{Fresh, false},
		{Warning, true},
		{Critical, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.tier), func(t *testing.T) {
			got := IsStaleCompat(tc.tier)
			if got != tc.want {
				t.Errorf("IsStaleCompat(%q) = %v, want %v", tc.tier, got, tc.want)
			}
		})
	}
}
