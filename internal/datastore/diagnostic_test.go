// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"testing"
)

func TestDepthBucket(t *testing.T) {
	cases := []struct {
		depth int
		want  string
	}{
		{0, "0"},
		{1, "1-5"},
		{5, "1-5"},
		{6, "6-10"},
		{10, "6-10"},
		{11, "11+"},
		{100, "11+"},
	}
	for _, tc := range cases {
		got := depthBucket(tc.depth)
		if got != tc.want {
			t.Errorf("depthBucket(%d) = %q, want %q", tc.depth, got, tc.want)
		}
	}
}

func TestDepthDistribution(t *testing.T) {
	t.Run("empty input has all buckets at zero", func(t *testing.T) {
		got := buildDepthDistribution(nil)
		for _, key := range []string{"0", "1-5", "6-10", "11+"} {
			if _, ok := got[key]; !ok {
				t.Errorf("missing bucket %q", key)
			}
			if got[key] != 0 {
				t.Errorf("bucket %q = %d, want 0", key, got[key])
			}
		}
	})

	t.Run("mixed depths produce correct counts", func(t *testing.T) {
		got := buildDepthDistribution([]int{0, 1, 5, 6, 10, 11})
		want := map[string]int{"0": 1, "1-5": 2, "6-10": 2, "11+": 1}
		for key, wantCount := range want {
			if got[key] != wantCount {
				t.Errorf("bucket %q = %d, want %d", key, got[key], wantCount)
			}
		}
	})
}
