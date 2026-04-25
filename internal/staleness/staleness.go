// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package staleness

import (
	"fmt"
	"time"
)

// Tier represents the staleness state of a node.
type Tier string

const (
	Fresh    Tier = "fresh"
	Warning  Tier = "warning"
	Critical Tier = "critical"
)

// Thresholds holds the two staleness thresholds.
type Thresholds struct {
	WarningHours int
	CriticalDays int
}

// ComputeTier determines the staleness tier of a node given its ohai_time.
// If ohaiTime is zero, the node is critical (no data available).
func ComputeTier(ohaiTime time.Time, now time.Time, t Thresholds) Tier {
	if ohaiTime.IsZero() {
		return Critical
	}
	age := now.Sub(ohaiTime)
	warningDur := time.Duration(t.WarningHours) * time.Hour
	criticalDur := time.Duration(t.CriticalDays) * 24 * time.Hour

	if age >= criticalDur {
		return Critical
	}
	if age >= warningDur {
		return Warning
	}
	return Fresh
}

// FormatAge returns a human-readable age string per the spec:
// < 1 hour → "Nm", 1-47 hours → "Nh", >= 48 hours → "Nd"
func FormatAge(age time.Duration) string {
	if age < 0 {
		age = 0
	}
	hours := age.Hours()
	if hours < 1 {
		minutes := int(age.Minutes())
		if minutes < 1 {
			minutes = 1
		}
		return fmt.Sprintf("%dm", minutes)
	}
	if hours < 48 {
		return fmt.Sprintf("%dh", int(hours))
	}
	days := int(hours / 24)
	return fmt.Sprintf("%dd", days)
}

// IsStaleCompat returns the backward-compatible is_stale boolean.
// Warning and critical are both "stale" in the old model.
func IsStaleCompat(tier Tier) bool {
	return tier != Fresh
}
