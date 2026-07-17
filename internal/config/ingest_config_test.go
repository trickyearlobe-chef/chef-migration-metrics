// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

// Ingest is off by default — an unauthenticated inbound endpoint must be opt-in.
func TestIngestConfig_IsEnabledDefaultsFalse(t *testing.T) {
	var ic IngestConfig
	if ic.IsEnabled() {
		t.Error("IsEnabled() = true for zero value, want false (opt-in)")
	}
	on := true
	ic.Enabled = &on
	if !ic.IsEnabled() {
		t.Error("IsEnabled() = false when Enabled=true, want true")
	}
	off := false
	ic.Enabled = &off
	if ic.IsEnabled() {
		t.Error("IsEnabled() = true when Enabled=false, want false")
	}
}

// Both new toggles default OFF: the feature stays in reserve (views hidden) and
// the sink keeps all runs unless explicitly told to drop successes.
func TestIngestConfig_ShowRunEventsAndFailuresOnlyDefaultFalse(t *testing.T) {
	var ic IngestConfig
	if ic.ShowsRunEvents() {
		t.Error("ShowsRunEvents() = true for zero value, want false (feature in reserve)")
	}
	if ic.IsFailuresOnly() {
		t.Error("IsFailuresOnly() = true for zero value, want false (keep all runs)")
	}
	on := true
	ic.ShowRunEvents = &on
	ic.FailuresOnly = &on
	if !ic.ShowsRunEvents() || !ic.IsFailuresOnly() {
		t.Error("accessors did not reflect explicit true")
	}
	off := false
	ic.ShowRunEvents = &off
	ic.FailuresOnly = &off
	if ic.ShowsRunEvents() || ic.IsFailuresOnly() {
		t.Error("accessors did not reflect explicit false")
	}
}

func TestIngestConfig_Defaults(t *testing.T) {
	var c Config
	c.ApplyDefaults()

	if got := c.Ingest.RetentionDays; got != 2 {
		t.Errorf("RetentionDays default = %d, want 2", got)
	}
	if c.Ingest.MaxBodyBytes <= 0 {
		t.Errorf("MaxBodyBytes default = %d, want > 0", c.Ingest.MaxBodyBytes)
	}
	if c.Ingest.MaxRecordsPerBody <= 0 {
		t.Errorf("MaxRecordsPerBody default = %d, want > 0", c.Ingest.MaxRecordsPerBody)
	}
	// A pre-set value must be preserved, not overwritten by defaults.
	c2 := Config{Ingest: IngestConfig{RetentionDays: 7}}
	c2.ApplyDefaults()
	if c2.Ingest.RetentionDays != 7 {
		t.Errorf("RetentionDays = %d, want 7 preserved", c2.Ingest.RetentionDays)
	}
}
