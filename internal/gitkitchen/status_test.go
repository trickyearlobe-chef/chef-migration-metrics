// SPDX-License-Identifier: Apache-2.0

package gitkitchen

import "testing"

func TestComputeTKStatus(t *testing.T) {
	tests := []struct {
		name   string
		passed int
		failed int
		want   string
	}{
		{"all passed", 5, 0, "passed"},
		{"all failed", 0, 3, "failed"},
		{"mixed", 2, 1, "partial"},
		{"no data", 0, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeTKStatus(tt.passed, tt.failed)
			if got != tt.want {
				t.Errorf("ComputeTKStatus(%d, %d) = %q, want %q", tt.passed, tt.failed, got, tt.want)
			}
		})
	}
}
