// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"os"
	"path/filepath"
	"testing"
)

// sampleCop is a minimal but well-formed RuboCop cop so the resolver can parse
// a cop name out of each test .rb file.
const sampleCop = `module RuboCop
  module Cop
    module Cmm
      class Sample < RuboCop::Cop::Base
      end
    end
  end
end
`

// writeFile is a tiny test helper that creates a cop file.
func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(sampleCop), 0o644); err != nil {
		t.Fatal(err)
	}
}

// resolvedPaths extracts the .rb paths from resolved addon cops.
func resolvedPaths(cops []AddonCop) []string {
	out := make([]string, len(cops))
	for i, c := range cops {
		out[i] = c.Path
	}
	return out
}

// TestResolveAddonCopFiles_File resolves a direct .rb file path.
func TestResolveAddonCopFiles_File(t *testing.T) {
	dir := t.TempDir()
	cop := filepath.Join(dir, "no_eval.rb")
	writeFile(t, cop)

	resolved, problems := ResolveAddonCopFiles([]string{cop})
	if len(problems) != 0 {
		t.Fatalf("expected no problems, got %v", problems)
	}
	if len(resolved) != 1 || resolved[0].Path != cop {
		t.Fatalf("expected [%s], got %v", cop, resolvedPaths(resolved))
	}
	if len(resolved[0].CopNames) != 1 || resolved[0].CopNames[0] != "Cmm/Sample" {
		t.Fatalf("expected parsed cop name Cmm/Sample, got %v", resolved[0].CopNames)
	}
}

// TestResolveAddonCopFiles_Directory expands a directory to its *.rb files,
// ignoring non-.rb files.
func TestResolveAddonCopFiles_Directory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a_cop.rb"))
	writeFile(t, filepath.Join(dir, "b_cop.rb"))
	writeFile(t, filepath.Join(dir, "README.md")) // not a cop

	resolved, problems := ResolveAddonCopFiles([]string{dir})
	if len(problems) != 0 {
		t.Fatalf("expected no problems, got %v", problems)
	}
	want := []string{filepath.Join(dir, "a_cop.rb"), filepath.Join(dir, "b_cop.rb")}
	got := resolvedPaths(resolved)
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want sorted %v, got %v", want, got)
		}
	}
}

// TestResolveAddonCopFiles_Glob expands a glob pattern to matching .rb files.
func TestResolveAddonCopFiles_Glob(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "one.rb"))
	writeFile(t, filepath.Join(dir, "two.rb"))
	writeFile(t, filepath.Join(dir, "skip.txt"))

	resolved, problems := ResolveAddonCopFiles([]string{filepath.Join(dir, "*.rb")})
	if len(problems) != 0 {
		t.Fatalf("expected no problems, got %v", problems)
	}
	if len(resolved) != 2 {
		t.Fatalf("expected 2 .rb files, got %v", resolved)
	}
}

// TestResolveAddonCopFiles_MissingPath surfaces a non-existent path as a problem
// without aborting resolution of the other (valid) entries.
func TestResolveAddonCopFiles_MissingPath(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.rb")
	writeFile(t, good)
	missing := filepath.Join(dir, "does_not_exist.rb")

	resolved, problems := ResolveAddonCopFiles([]string{missing, good})
	if len(resolved) != 1 || resolved[0].Path != good {
		t.Fatalf("valid entry must still resolve, got %v", resolvedPaths(resolved))
	}
	if len(problems) != 1 || problems[0].Path != missing {
		t.Fatalf("missing path must be one problem, got %v", problems)
	}
}

// TestResolveAddonCopFiles_NonRubyFile surfaces a non-.rb file as a problem.
func TestResolveAddonCopFiles_NonRubyFile(t *testing.T) {
	dir := t.TempDir()
	txt := filepath.Join(dir, "notes.txt")
	writeFile(t, txt)

	resolved, problems := ResolveAddonCopFiles([]string{txt})
	if len(resolved) != 0 {
		t.Fatalf("a non-.rb file must not resolve, got %v", resolved)
	}
	if len(problems) != 1 {
		t.Fatalf("expected 1 problem, got %v", problems)
	}
}

// TestResolveAddonCopFiles_EmptyDirectory surfaces a directory with no .rb
// files as a problem.
func TestResolveAddonCopFiles_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	resolved, problems := ResolveAddonCopFiles([]string{dir})
	if len(resolved) != 0 {
		t.Fatalf("expected nothing resolved, got %v", resolved)
	}
	if len(problems) != 1 {
		t.Fatalf("expected 1 problem for empty dir, got %v", problems)
	}
}

// TestResolveAddonCopFiles_Dedup removes duplicate resolutions (a file matched
// by both a direct path and a glob).
func TestResolveAddonCopFiles_Dedup(t *testing.T) {
	dir := t.TempDir()
	cop := filepath.Join(dir, "dup.rb")
	writeFile(t, cop)

	resolved, _ := ResolveAddonCopFiles([]string{cop, filepath.Join(dir, "*.rb")})
	if len(resolved) != 1 {
		t.Fatalf("duplicate resolutions must dedup, got %v", resolved)
	}
}

// TestResolveAddonCopFiles_EmptyAndBlankEntriesIgnored skips empty entries.
func TestResolveAddonCopFiles_EmptyAndBlankEntriesIgnored(t *testing.T) {
	resolved, problems := ResolveAddonCopFiles([]string{"", "  "})
	if len(resolved) != 0 || len(problems) != 0 {
		t.Fatalf("blank entries must be ignored, got resolved=%v problems=%v", resolved, problems)
	}
}

// TestResolveAddonCopFiles_GlobNoMatch surfaces a zero-match glob as a problem.
func TestResolveAddonCopFiles_GlobNoMatch(t *testing.T) {
	dir := t.TempDir()
	_, problems := ResolveAddonCopFiles([]string{filepath.Join(dir, "*.rb")})
	if len(problems) != 1 {
		t.Fatalf("a glob with no matches must be one problem, got %v", problems)
	}
}

// TestResolveAddonCopFiles_UnparseableCopSurfaced flags a .rb with no cop class
// as a problem (it is loaded but cannot be enabled).
func TestResolveAddonCopFiles_UnparseableCopSurfaced(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "not_a_cop.rb")
	if err := os.WriteFile(bad, []byte("puts 'hi'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, problems := ResolveAddonCopFiles([]string{bad})
	if len(resolved) != 1 || resolved[0].Path != bad {
		t.Fatalf("file is still required, got %v", resolvedPaths(resolved))
	}
	if len(resolved[0].CopNames) != 0 {
		t.Fatalf("expected no cop names parsed, got %v", resolved[0].CopNames)
	}
	if len(problems) != 1 {
		t.Fatalf("an unparseable cop file must be one problem, got %v", problems)
	}
}

// TestParseAddonCopNames covers the nested and flat class-declaration forms.
func TestParseAddonCopNames(t *testing.T) {
	cases := map[string]string{
		"nested": `module RuboCop
  module Cop
    module Cmm
      class NoNodeRegexMatch < RuboCop::Cop::Base
      end
    end
  end
end
`,
		"flat": "class RuboCop::Cop::Cmm::NoNodeRegexMatch < RuboCop::Cop::Base\nend\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "cop.rb")
			if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			got := ParseAddonCopNames(p)
			if len(got) != 1 || got[0] != "Cmm/NoNodeRegexMatch" {
				t.Fatalf("want [Cmm/NoNodeRegexMatch], got %v", got)
			}
		})
	}
}
