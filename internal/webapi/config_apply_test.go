// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import "testing"

// Granularities are ordered by severity: applied < subsystem < listener < process.
// restart_required derives from whether the worst granularity is process.
func TestReloadGranularityOrdering(t *testing.T) {
	order := []ReloadGranularity{ReloadApplied, ReloadSubsystem, ReloadListener, ReloadProcess}
	for i := 1; i < len(order); i++ {
		if !(order[i-1] < order[i]) {
			t.Errorf("granularity %s should be less severe than %s", order[i-1], order[i])
		}
	}
}

func TestReloadGranularityString(t *testing.T) {
	cases := map[ReloadGranularity]string{
		ReloadApplied:   "applied",
		ReloadSubsystem: "subsystem",
		ReloadListener:  "listener",
		ReloadProcess:   "process",
	}
	for g, want := range cases {
		if got := g.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", g, got, want)
		}
	}
}

// worstGranularity returns the most severe granularity among results, and
// defaults to process (pessimistic) when no applier ran.
func TestWorstGranularity(t *testing.T) {
	tests := []struct {
		name    string
		results []ApplyResult
		want    ReloadGranularity
	}{
		{"no appliers defaults pessimistically to process", nil, ReloadProcess},
		{"single applied", []ApplyResult{{Reload: ReloadApplied}}, ReloadApplied},
		{"applied+subsystem -> subsystem", []ApplyResult{{Reload: ReloadApplied}, {Reload: ReloadSubsystem}}, ReloadSubsystem},
		{"subsystem+process -> process", []ApplyResult{{Reload: ReloadSubsystem}, {Reload: ReloadProcess}}, ReloadProcess},
		{"listener beats subsystem", []ApplyResult{{Reload: ReloadSubsystem}, {Reload: ReloadListener}}, ReloadListener},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := worstGranularity(tt.results); got != tt.want {
				t.Errorf("worstGranularity = %s, want %s", got, tt.want)
			}
		})
	}
}

// restart_required is true only when the worst granularity is process.
func TestWorstGranularityRestartRequired(t *testing.T) {
	if (worstGranularity(nil) == ReloadProcess) != true {
		t.Error("no appliers should require restart (process)")
	}
	if worstGranularity([]ApplyResult{{Reload: ReloadApplied}}) == ReloadProcess {
		t.Error("applied should not require restart")
	}
	if worstGranularity([]ApplyResult{{Reload: ReloadListener}}) == ReloadProcess {
		t.Error("listener should not require a process restart")
	}
}
