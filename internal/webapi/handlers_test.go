// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import "testing"

func TestSafeName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOK  bool
		wantOut string
	}{
		// Valid cookbook / repo names.
		{name: "simple name", input: "nginx", wantOK: true, wantOut: "nginx"},
		{name: "hyphenated", input: "my-cookbook", wantOK: true, wantOut: "my-cookbook"},
		{name: "underscored", input: "my_cookbook", wantOK: true, wantOut: "my_cookbook"},
		{name: "with digits", input: "app2", wantOK: true, wantOut: "app2"},
		{name: "dotfile safe", input: "my.cookbook", wantOK: true, wantOut: "my.cookbook"},

		// Path traversal attempts.
		{name: "dot-dot", input: "..", wantOK: false},
		{name: "dot-dot-slash", input: "../etc", wantOK: false},
		{name: "double traversal", input: "../../etc/passwd", wantOK: false},
		{name: "trailing dot-dot", input: "foo/..", wantOK: false},
		{name: "mid traversal", input: "foo/../bar", wantOK: false},
		{name: "dot-dot backslash", input: `..\\etc`, wantOK: false},

		// Absolute paths.
		{name: "absolute unix", input: "/etc/passwd", wantOK: false},
		{name: "absolute windows", input: `C:\Windows`, wantOK: false},

		// Path separators.
		{name: "forward slash", input: "foo/bar", wantOK: false},
		{name: "backslash", input: `foo\bar`, wantOK: false},

		// Dot names.
		{name: "single dot", input: ".", wantOK: false},

		// Empty / whitespace.
		{name: "empty", input: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := safeName(tt.input)
			if ok != tt.wantOK {
				t.Errorf("safeName(%q): ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok && got != tt.wantOut {
				t.Errorf("safeName(%q) = %q, want %q", tt.input, got, tt.wantOut)
			}
		})
	}
}
