// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

func TestScanCustomCops_EmptyDefs(t *testing.T) {
	offenses := ScanCustomCops("/nonexistent", nil)
	if len(offenses) != 0 {
		t.Errorf("expected 0 offenses for nil defs, got %d", len(offenses))
	}
}

func TestScanCustomCops_LiteralMatch(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "recipes/default.rb", `
node.set[:foo] = "bar"
node.normal[:baz] = "qux"
`)

	defs := []datastore.CustomCopDefinition{
		{
			CopName:        "Custom/Test/NodeSet",
			Description:    "node.set is removed",
			PatternType:    "literal",
			Pattern:        "node.set[",
			FileGlob:       "*.rb",
			Classification: "blocker",
			Enabled:        true,
		},
	}

	offenses := ScanCustomCops(dir, defs)
	if len(offenses) != 1 {
		t.Fatalf("expected 1 offense, got %d", len(offenses))
	}
	off := offenses[0]
	if off.CopName != "Custom/Test/NodeSet" {
		t.Errorf("cop_name = %q, want Custom/Test/NodeSet", off.CopName)
	}
	if off.Location.StartLine != 2 {
		t.Errorf("start_line = %d, want 2", off.Location.StartLine)
	}
	if off.File != filepath.Join("recipes", "default.rb") {
		t.Errorf("file = %q, want recipes/default.rb", off.File)
	}
	if off.Severity != "error" {
		t.Errorf("severity = %q, want error (blocker→error)", off.Severity)
	}
}

func TestScanCustomCops_RegexMatch(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "recipes/default.rb", `
x = nil
if x =~ /pattern/
  puts "matched"
end
result = "hello" =~ /world/
`)

	defs := []datastore.CustomCopDefinition{
		{
			CopName:        "Custom/Ruby3/NilRegexpMatch",
			Description:    "=~ on nil removed in Ruby 3",
			PatternType:    "regex",
			Pattern:        `=~`,
			FileGlob:       "*.rb",
			Classification: "blocker",
			Enabled:        true,
		},
	}

	offenses := ScanCustomCops(dir, defs)
	if len(offenses) != 2 {
		t.Fatalf("expected 2 offenses (lines 3 and 6), got %d", len(offenses))
	}
	if offenses[0].Location.StartLine != 3 {
		t.Errorf("first offense start_line = %d, want 3", offenses[0].Location.StartLine)
	}
	if offenses[1].Location.StartLine != 6 {
		t.Errorf("second offense start_line = %d, want 6", offenses[1].Location.StartLine)
	}
}

func TestScanCustomCops_FileGlobFiltering(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "recipes/default.rb", `node.set[:foo] = "bar"`)
	writeTestFile(t, dir, "README.md", `node.set[:foo] = "bar"`)

	defs := []datastore.CustomCopDefinition{
		{
			CopName:        "Custom/Test/NodeSet",
			Description:    "node.set is removed",
			PatternType:    "literal",
			Pattern:        "node.set[",
			FileGlob:       "*.rb",
			Classification: "blocker",
			Enabled:        true,
		},
	}

	offenses := ScanCustomCops(dir, defs)
	if len(offenses) != 1 {
		t.Fatalf("expected 1 offense (only .rb files), got %d", len(offenses))
	}
	if off := offenses[0]; off.File != filepath.Join("recipes", "default.rb") {
		t.Errorf("offense in wrong file: %s", off.File)
	}
}

func TestScanCustomCops_DefaultGlob(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "recipes/default.rb", `node.set[:foo] = "bar"`)

	defs := []datastore.CustomCopDefinition{
		{
			CopName:        "Custom/Test/NodeSet",
			Description:    "node.set is removed",
			PatternType:    "literal",
			Pattern:        "node.set[",
			FileGlob:       "", // empty → defaults to *.rb
			Classification: "review",
			Enabled:        true,
		},
	}

	offenses := ScanCustomCops(dir, defs)
	if len(offenses) != 1 {
		t.Fatalf("expected 1 offense with default glob, got %d", len(offenses))
	}
	if offenses[0].Severity != "warning" {
		t.Errorf("severity = %q, want warning (review→warning)", offenses[0].Severity)
	}
}

func TestScanCustomCops_InvalidRegexSkipped(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "recipes/default.rb", `some code`)

	defs := []datastore.CustomCopDefinition{
		{
			CopName:        "Custom/Bad/Regex",
			Description:    "bad regex",
			PatternType:    "regex",
			Pattern:        `[invalid`,
			FileGlob:       "*.rb",
			Classification: "blocker",
			Enabled:        true,
		},
	}

	offenses := ScanCustomCops(dir, defs)
	if len(offenses) != 0 {
		t.Errorf("expected 0 offenses for invalid regex, got %d", len(offenses))
	}
}

func TestScanCustomCops_MultipleDefs(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "recipes/default.rb", `
node.set[:foo] = "bar"
x =~ /pattern/
`)

	defs := []datastore.CustomCopDefinition{
		{
			CopName:        "Custom/Test/NodeSet",
			Description:    "node.set is removed",
			PatternType:    "literal",
			Pattern:        "node.set[",
			FileGlob:       "*.rb",
			Classification: "blocker",
			Enabled:        true,
		},
		{
			CopName:        "Custom/Ruby3/NilRegexpMatch",
			Description:    "=~ on nil removed",
			PatternType:    "regex",
			Pattern:        `=~`,
			FileGlob:       "*.rb",
			Classification: "review",
			Enabled:        true,
		},
	}

	offenses := ScanCustomCops(dir, defs)
	if len(offenses) != 2 {
		t.Fatalf("expected 2 offenses from 2 defs, got %d", len(offenses))
	}

	copNames := map[string]bool{}
	for _, o := range offenses {
		copNames[o.CopName] = true
	}
	if !copNames["Custom/Test/NodeSet"] {
		t.Error("missing offense from Custom/Test/NodeSet")
	}
	if !copNames["Custom/Ruby3/NilRegexpMatch"] {
		t.Error("missing offense from Custom/Ruby3/NilRegexpMatch")
	}
}

func TestClassificationToSeverity(t *testing.T) {
	tests := []struct {
		classification string
		want           string
	}{
		{"blocker", "error"},
		{"review", "warning"},
		{"noise", "convention"},
		{"", "convention"},
	}
	for _, tt := range tests {
		got := classificationToSeverity(tt.classification)
		if got != tt.want {
			t.Errorf("classificationToSeverity(%q) = %q, want %q", tt.classification, got, tt.want)
		}
	}
}

func TestFileMatchesGlob(t *testing.T) {
	tests := []struct {
		relPath string
		glob    string
		want    bool
	}{
		{"recipes/default.rb", "*.rb", true},
		{"recipes/default.rb", "*.md", false},
		{"README.md", "*.rb", false},
		{"README.md", "*.md", true},
		{"attributes/default.rb", "*.rb", true},
	}
	for _, tt := range tests {
		got := fileMatchesGlob(tt.relPath, tt.glob)
		if got != tt.want {
			t.Errorf("fileMatchesGlob(%q, %q) = %v, want %v", tt.relPath, tt.glob, got, tt.want)
		}
	}
}

// writeTestFile creates a file in a subdirectory of dir.
func writeTestFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
