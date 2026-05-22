// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import "testing"

func TestComputeGitRepoCompatibilityFromResult(t *testing.T) {
	tests := []struct {
		name         string
		passed       bool
		errorMessage string
		want         string
	}{
		{"passed with no error", true, "", "compatible"},
		{"failed with no error", false, "", "incompatible"},
		{"error message present (passed=true)", true, "scan error", "error"},
		{"error message present (passed=false)", false, "timeout", "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeGitRepoCompatibilityFromResult(tt.passed, tt.errorMessage)
			if got != tt.want {
				t.Errorf("ComputeGitRepoCompatibilityFromResult(%v, %q) = %q, want %q",
					tt.passed, tt.errorMessage, got, tt.want)
			}
		})
	}
}

func TestComputeGitRepoTKStatusFromCounts(t *testing.T) {
	tests := []struct {
		name   string
		passed int
		failed int
		want   string
	}{
		{"no data", 0, 0, "untested"},
		{"all passed", 5, 0, "passed"},
		{"all failed", 0, 3, "failed"},
		{"mixed", 2, 1, "partial"},
		{"one passed", 1, 0, "passed"},
		{"one failed", 0, 1, "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeGitRepoTKStatusFromCounts(tt.passed, tt.failed)
			if got != tt.want {
				t.Errorf("ComputeGitRepoTKStatusFromCounts(%d, %d) = %q, want %q",
					tt.passed, tt.failed, got, tt.want)
			}
		})
	}
}
