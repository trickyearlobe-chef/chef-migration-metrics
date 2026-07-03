// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import "testing"

func TestOffensesWontParse(t *testing.T) {
	cases := []struct {
		name     string
		offenses []CookstyleOffense
		want     bool
	}{
		{"empty", nil, false},
		{"no fatal", []CookstyleOffense{
			{CopName: "Chef/Deprecations/NodeSet", Severity: "warning"},
			{CopName: "Style/TrailingWhitespace", Severity: "convention"},
		}, false},
		{"fatal present", []CookstyleOffense{
			{CopName: "Chef/Deprecations/NodeSet", Severity: "warning"},
			{CopName: "Lint/Syntax", Severity: SeverityFatal},
		}, true},
		{"only fatal", []CookstyleOffense{
			{CopName: "Lint/Syntax", Severity: "fatal"},
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OffensesWontParse(tc.offenses); got != tc.want {
				t.Errorf("OffensesWontParse() = %v, want %v", got, tc.want)
			}
		})
	}
}
