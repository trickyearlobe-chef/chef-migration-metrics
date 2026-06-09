// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package pathsafe

import (
	"path/filepath"
	"testing"
)

func TestSafeJoin_Allows(t *testing.T) {
	base := "/data/work"
	cases := []struct{ rel, want string }{
		{"apache", filepath.Join(base, "apache")},
		{"cookbooks/apache/recipes/default.rb", filepath.Join(base, "cookbooks/apache/recipes/default.rb")},
		{"role.json", filepath.Join(base, "role.json")},
		{"./nested/file", filepath.Join(base, "nested/file")},
	}
	for _, c := range cases {
		got, err := SafeJoin(base, c.rel)
		if err != nil {
			t.Errorf("SafeJoin(%q): unexpected error %v", c.rel, err)
			continue
		}
		if got != c.want {
			t.Errorf("SafeJoin(%q) = %q, want %q", c.rel, got, c.want)
		}
	}
}

func TestSafeJoin_Rejects(t *testing.T) {
	base := "/data/work"
	bad := []string{
		"../etc/passwd",
		"../../etc/passwd",
		"cookbooks/../../etc/passwd",
		"/etc/passwd",
		"..",
		"foo/../../bar",
	}
	for _, rel := range bad {
		if got, err := SafeJoin(base, rel); err == nil {
			t.Errorf("SafeJoin(%q) = %q, want error", rel, got)
		}
	}
}
