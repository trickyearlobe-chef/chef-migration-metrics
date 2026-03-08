// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// buildAutocorrectArgs
// ---------------------------------------------------------------------------

func TestBuildAutocorrectArgs_WithTargetVersion_NoCookbookConfig(t *testing.T) {
	cookbookDir := t.TempDir()
	args := buildAutocorrectArgs(cookbookDir, "18.0")
	if len(args) == 0 {
		t.Fatal("expected non-empty args")
	}

	// Should contain --auto-correct, --format json, --config, --only, and the directory.
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--auto-correct") {
		t.Error("args should contain --auto-correct")
	}
	if !strings.Contains(joined, "--format json") {
		t.Error("args should contain --format json")
	}
	if !strings.Contains(joined, "--config") {
		t.Error("args should contain --config when target version is set")
	}
	if !strings.Contains(joined, "--only") {
		t.Error("args should contain --only when target version is set")
	}
	if !strings.Contains(joined, "Chef/Deprecations,Chef/Correctness") {
		t.Error("args should restrict to Chef/Deprecations,Chef/Correctness")
	}
	if args[len(args)-1] != cookbookDir {
		t.Errorf("last arg = %q, want %s", args[len(args)-1], cookbookDir)
	}

	// --target-chef-version is NOT a CLI flag.
	if strings.Contains(joined, "--target-chef-version") {
		t.Error("--target-chef-version should not be a CLI arg; it belongs in .rubocop_cmm.yml")
	}

	// Verify sidecar .rubocop_cmm.yml was written with TargetChefVersion.
	data, err := os.ReadFile(filepath.Join(cookbookDir, cmmConfigName))
	if err != nil {
		t.Fatalf("expected %s to be written: %v", cmmConfigName, err)
	}
	content := string(data)
	if !strings.Contains(content, "TargetChefVersion: 18.0") {
		t.Errorf("%s should contain TargetChefVersion: 18.0, got:\n%s", cmmConfigName, content)
	}

	// Without an existing .rubocop.yml the sidecar should require cookstyle
	// so the TargetChefVersion parameter is recognised.
	if !strings.Contains(content, "require:") || !strings.Contains(content, "cookstyle") {
		t.Errorf("%s should require cookstyle when no cookbook config exists, got:\n%s", cmmConfigName, content)
	}
	if strings.Contains(content, "inherit_from") {
		t.Errorf("%s should NOT inherit_from when no cookbook .rubocop.yml exists, got:\n%s", cmmConfigName, content)
	}

	// Original .rubocop.yml must not exist (we must not clobber cookbook config).
	if _, err := os.Stat(filepath.Join(cookbookDir, ".rubocop.yml")); err == nil {
		t.Error("should not have written .rubocop.yml — only the sidecar .rubocop_cmm.yml")
	}
}

func TestBuildAutocorrectArgs_WithTargetVersion_WithCookbookConfig(t *testing.T) {
	cookbookDir := t.TempDir()

	// Simulate a cookbook that already has its own .rubocop.yml.
	existingConfig := "require:\n  - cookstyle\n\nChef/Style/TrueClassFalseClassResourceProperties:\n  Enabled: false\n"
	if err := os.WriteFile(filepath.Join(cookbookDir, ".rubocop.yml"), []byte(existingConfig), 0644); err != nil {
		t.Fatal(err)
	}

	args := buildAutocorrectArgs(cookbookDir, "17.0")
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "--config") {
		t.Error("args should contain --config when target version is set")
	}

	// Verify sidecar inherits from the cookbook's own config.
	data, err := os.ReadFile(filepath.Join(cookbookDir, cmmConfigName))
	if err != nil {
		t.Fatalf("expected %s to be written: %v", cmmConfigName, err)
	}
	content := string(data)
	if !strings.Contains(content, "inherit_from: .rubocop.yml") {
		t.Errorf("%s should inherit_from .rubocop.yml, got:\n%s", cmmConfigName, content)
	}
	if !strings.Contains(content, "TargetChefVersion: 17.0") {
		t.Errorf("%s should contain TargetChefVersion: 17.0, got:\n%s", cmmConfigName, content)
	}

	// The cookbook's original .rubocop.yml must be preserved.
	origData, err := os.ReadFile(filepath.Join(cookbookDir, ".rubocop.yml"))
	if err != nil {
		t.Fatal("cookbook .rubocop.yml should still exist")
	}
	if string(origData) != existingConfig {
		t.Errorf("cookbook .rubocop.yml was modified; expected original content to be preserved")
	}
}

func TestBuildAutocorrectArgs_WithoutTargetVersion(t *testing.T) {
	cookbookDir := t.TempDir()
	args := buildAutocorrectArgs(cookbookDir, "")
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "--auto-correct") {
		t.Error("args should contain --auto-correct")
	}
	if !strings.Contains(joined, "--format json") {
		t.Error("args should contain --format json")
	}
	if strings.Contains(joined, "--only") {
		t.Error("args should NOT contain --only when target version is empty")
	}
	if strings.Contains(joined, "--config") {
		t.Error("args should NOT contain --config when target version is empty")
	}
	if args[len(args)-1] != cookbookDir {
		t.Errorf("last arg = %q, want %s", args[len(args)-1], cookbookDir)
	}

	// No sidecar config should be written when target version is empty.
	if _, err := os.Stat(filepath.Join(cookbookDir, cmmConfigName)); err == nil {
		t.Errorf("%s should not be written when target version is empty", cmmConfigName)
	}
}

func TestBuildAutocorrectArgs_AlwaysHasFormatJSON(t *testing.T) {
	for _, tv := range []string{"", "17.0", "18.5"} {
		args := buildAutocorrectArgs("/dir", tv)
		found := false
		for i, a := range args {
			if a == "--format" && i+1 < len(args) && args[i+1] == "json" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("buildAutocorrectArgs(%q, %q) missing --format json", "/dir", tv)
		}
	}
}

// ---------------------------------------------------------------------------
// splitLines
// ---------------------------------------------------------------------------

func TestSplitLines_Empty(t *testing.T) {
	lines := splitLines("")
	if lines != nil {
		t.Errorf("splitLines(\"\") = %v, want nil", lines)
	}
}

func TestSplitLines_SingleLine(t *testing.T) {
	lines := splitLines("hello\n")
	if len(lines) != 1 {
		t.Fatalf("splitLines single = %d lines, want 1", len(lines))
	}
	if lines[0] != "hello\n" {
		t.Errorf("lines[0] = %q, want %q", lines[0], "hello\n")
	}
}

func TestSplitLines_MultipleLines(t *testing.T) {
	lines := splitLines("a\nb\nc\n")
	if len(lines) != 3 {
		t.Fatalf("splitLines multi = %d lines, want 3", len(lines))
	}
	if lines[0] != "a\n" {
		t.Errorf("lines[0] = %q", lines[0])
	}
	if lines[1] != "b\n" {
		t.Errorf("lines[1] = %q", lines[1])
	}
	if lines[2] != "c\n" {
		t.Errorf("lines[2] = %q", lines[2])
	}
}

func TestSplitLines_NoTrailingNewline(t *testing.T) {
	lines := splitLines("a\nb")
	if len(lines) != 2 {
		t.Fatalf("splitLines no-trailing = %d lines, want 2", len(lines))
	}
	if lines[0] != "a\n" {
		t.Errorf("lines[0] = %q", lines[0])
	}
	if lines[1] != "b" {
		t.Errorf("lines[1] = %q", lines[1])
	}
}

// ---------------------------------------------------------------------------
// stripNewline
// ---------------------------------------------------------------------------

func TestStripNewline_WithNewline(t *testing.T) {
	if got := stripNewline("hello\n"); got != "hello" {
		t.Errorf("stripNewline = %q, want %q", got, "hello")
	}
}

func TestStripNewline_WithCRLF(t *testing.T) {
	if got := stripNewline("hello\r\n"); got != "hello" {
		t.Errorf("stripNewline = %q, want %q", got, "hello")
	}
}

func TestStripNewline_WithoutNewline(t *testing.T) {
	if got := stripNewline("hello"); got != "hello" {
		t.Errorf("stripNewline = %q, want %q", got, "hello")
	}
}

func TestStripNewline_Empty(t *testing.T) {
	if got := stripNewline(""); got != "" {
		t.Errorf("stripNewline = %q, want %q", got, "")
	}
}

// ---------------------------------------------------------------------------
// sortStringSlice
// ---------------------------------------------------------------------------

func TestSortStringSlice_AlreadySorted(t *testing.T) {
	s := []string{"a", "b", "c"}
	sortStringSlice(s)
	if s[0] != "a" || s[1] != "b" || s[2] != "c" {
		t.Errorf("sortStringSlice = %v, want [a b c]", s)
	}
}

func TestSortStringSlice_Reversed(t *testing.T) {
	s := []string{"c", "b", "a"}
	sortStringSlice(s)
	if s[0] != "a" || s[1] != "b" || s[2] != "c" {
		t.Errorf("sortStringSlice = %v, want [a b c]", s)
	}
}

func TestSortStringSlice_Duplicates(t *testing.T) {
	s := []string{"b", "a", "b", "a"}
	sortStringSlice(s)
	if s[0] != "a" || s[1] != "a" || s[2] != "b" || s[3] != "b" {
		t.Errorf("sortStringSlice = %v, want [a a b b]", s)
	}
}

func TestSortStringSlice_Single(t *testing.T) {
	s := []string{"x"}
	sortStringSlice(s)
	if s[0] != "x" {
		t.Errorf("sortStringSlice = %v, want [x]", s)
	}
}

func TestSortStringSlice_Empty(t *testing.T) {
	var s []string
	sortStringSlice(s) // Should not panic.
}

func TestSortStringSlice_Paths(t *testing.T) {
	s := []string{"recipes/default.rb", "attributes/default.rb", "metadata.rb"}
	sortStringSlice(s)
	if s[0] != "attributes/default.rb" || s[1] != "metadata.rb" || s[2] != "recipes/default.rb" {
		t.Errorf("sortStringSlice paths = %v", s)
	}
}

// ---------------------------------------------------------------------------
// computeEdits
// ---------------------------------------------------------------------------

func TestComputeEdits_Identical(t *testing.T) {
	a := []string{"line1\n", "line2\n", "line3\n"}
	b := []string{"line1\n", "line2\n", "line3\n"}
	edits := computeEdits(a, b)

	for _, e := range edits {
		if e.op != editEqual {
			t.Errorf("expected all editEqual, got op=%d for %q", e.op, e.line)
		}
	}
}

func TestComputeEdits_SingleInsertion(t *testing.T) {
	a := []string{"line1\n", "line3\n"}
	b := []string{"line1\n", "line2\n", "line3\n"}
	edits := computeEdits(a, b)

	inserts := 0
	for _, e := range edits {
		if e.op == editInsert {
			inserts++
			if stripNewline(e.line) != "line2" {
				t.Errorf("inserted line = %q, want %q", e.line, "line2\n")
			}
		}
	}
	if inserts != 1 {
		t.Errorf("inserts = %d, want 1", inserts)
	}
}

func TestComputeEdits_SingleDeletion(t *testing.T) {
	a := []string{"line1\n", "line2\n", "line3\n"}
	b := []string{"line1\n", "line3\n"}
	edits := computeEdits(a, b)

	deletes := 0
	for _, e := range edits {
		if e.op == editDelete {
			deletes++
			if stripNewline(e.line) != "line2" {
				t.Errorf("deleted line = %q, want %q", e.line, "line2\n")
			}
		}
	}
	if deletes != 1 {
		t.Errorf("deletes = %d, want 1", deletes)
	}
}

func TestComputeEdits_SingleReplacement(t *testing.T) {
	a := []string{"old_line\n"}
	b := []string{"new_line\n"}
	edits := computeEdits(a, b)

	hasDelete := false
	hasInsert := false
	for _, e := range edits {
		if e.op == editDelete {
			hasDelete = true
		}
		if e.op == editInsert {
			hasInsert = true
		}
	}
	if !hasDelete || !hasInsert {
		t.Error("expected both a delete and an insert for a replacement")
	}
}

func TestComputeEdits_EmptyToNonEmpty(t *testing.T) {
	var a []string
	b := []string{"line1\n", "line2\n"}
	edits := computeEdits(a, b)

	if len(edits) != 2 {
		t.Fatalf("edits = %d, want 2", len(edits))
	}
	for _, e := range edits {
		if e.op != editInsert {
			t.Errorf("expected editInsert, got op=%d", e.op)
		}
	}
}

func TestComputeEdits_NonEmptyToEmpty(t *testing.T) {
	a := []string{"line1\n", "line2\n"}
	var b []string
	edits := computeEdits(a, b)

	if len(edits) != 2 {
		t.Fatalf("edits = %d, want 2", len(edits))
	}
	for _, e := range edits {
		if e.op != editDelete {
			t.Errorf("expected editDelete, got op=%d", e.op)
		}
	}
}

func TestComputeEdits_BothEmpty(t *testing.T) {
	edits := computeEdits(nil, nil)
	if len(edits) != 0 {
		t.Errorf("edits = %d, want 0", len(edits))
	}
}

// ---------------------------------------------------------------------------
// generateUnifiedDiffs
// ---------------------------------------------------------------------------

func TestGenerateUnifiedDiffs_NoDifference(t *testing.T) {
	orig := map[string]string{"recipes/default.rb": "line1\nline2\n"}
	mod := map[string]string{"recipes/default.rb": "line1\nline2\n"}

	diff, changed := generateUnifiedDiffs(orig, mod)
	if changed != 0 {
		t.Errorf("changed = %d, want 0", changed)
	}
	if diff != "" {
		t.Errorf("diff should be empty, got:\n%s", diff)
	}
}

func TestGenerateUnifiedDiffs_SingleFileModified(t *testing.T) {
	orig := map[string]string{
		"recipes/default.rb": "node.set['foo'] = 'bar'\n",
	}
	mod := map[string]string{
		"recipes/default.rb": "node.normal['foo'] = 'bar'\n",
	}

	diff, changed := generateUnifiedDiffs(orig, mod)
	if changed != 1 {
		t.Errorf("changed = %d, want 1", changed)
	}
	if !strings.Contains(diff, "--- a/recipes/default.rb") {
		t.Error("diff should contain --- a/ header")
	}
	if !strings.Contains(diff, "+++ b/recipes/default.rb") {
		t.Error("diff should contain +++ b/ header")
	}
	if !strings.Contains(diff, "@@") {
		t.Error("diff should contain @@ hunk header")
	}
	if !strings.Contains(diff, "-node.set") {
		t.Error("diff should contain deleted line with -")
	}
	if !strings.Contains(diff, "+node.normal") {
		t.Error("diff should contain added line with +")
	}
}

func TestGenerateUnifiedDiffs_MultipleFilesModified(t *testing.T) {
	orig := map[string]string{
		"recipes/default.rb": "old1\n",
		"metadata.rb":        "old2\n",
	}
	mod := map[string]string{
		"recipes/default.rb": "new1\n",
		"metadata.rb":        "new2\n",
	}

	diff, changed := generateUnifiedDiffs(orig, mod)
	if changed != 2 {
		t.Errorf("changed = %d, want 2", changed)
	}
	// Both files should appear in the diff.
	if !strings.Contains(diff, "recipes/default.rb") {
		t.Error("diff should contain recipes/default.rb")
	}
	if !strings.Contains(diff, "metadata.rb") {
		t.Error("diff should contain metadata.rb")
	}
}

func TestGenerateUnifiedDiffs_FileAdded(t *testing.T) {
	orig := map[string]string{}
	mod := map[string]string{
		"new_file.rb": "content\n",
	}

	diff, changed := generateUnifiedDiffs(orig, mod)
	if changed != 1 {
		t.Errorf("changed = %d, want 1", changed)
	}
	if !strings.Contains(diff, "new_file.rb") {
		t.Error("diff should contain new_file.rb")
	}
}

func TestGenerateUnifiedDiffs_FileRemoved(t *testing.T) {
	orig := map[string]string{
		"old_file.rb": "content\n",
	}
	mod := map[string]string{}

	diff, changed := generateUnifiedDiffs(orig, mod)
	if changed != 1 {
		t.Errorf("changed = %d, want 1", changed)
	}
	if !strings.Contains(diff, "old_file.rb") {
		t.Error("diff should contain old_file.rb")
	}
}

func TestGenerateUnifiedDiffs_EmptyMaps(t *testing.T) {
	diff, changed := generateUnifiedDiffs(map[string]string{}, map[string]string{})
	if changed != 0 {
		t.Errorf("changed = %d, want 0", changed)
	}
	if diff != "" {
		t.Errorf("diff should be empty, got:\n%s", diff)
	}
}

func TestGenerateUnifiedDiffs_DeterministicOrder(t *testing.T) {
	orig := map[string]string{
		"z_file.rb": "old_z\n",
		"a_file.rb": "old_a\n",
		"m_file.rb": "old_m\n",
	}
	mod := map[string]string{
		"z_file.rb": "new_z\n",
		"a_file.rb": "new_a\n",
		"m_file.rb": "new_m\n",
	}

	diff1, _ := generateUnifiedDiffs(orig, mod)
	diff2, _ := generateUnifiedDiffs(orig, mod)

	if diff1 != diff2 {
		t.Error("generateUnifiedDiffs should produce deterministic output")
	}

	// Verify alphabetical order: a_file should appear before m_file before z_file.
	aIdx := strings.Index(diff1, "a_file.rb")
	mIdx := strings.Index(diff1, "m_file.rb")
	zIdx := strings.Index(diff1, "z_file.rb")
	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("files not in alphabetical order: a=%d, m=%d, z=%d", aIdx, mIdx, zIdx)
	}
}

// ---------------------------------------------------------------------------
// computeUnifiedDiff
// ---------------------------------------------------------------------------

func TestComputeUnifiedDiff_Headers(t *testing.T) {
	a := []string{"old\n"}
	b := []string{"new\n"}
	diff := computeUnifiedDiff("test.rb", a, b)

	if !strings.HasPrefix(diff, "--- a/test.rb\n") {
		t.Errorf("diff should start with --- a/test.rb header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+++ b/test.rb\n") {
		t.Errorf("diff should contain +++ b/test.rb header, got:\n%s", diff)
	}
}

func TestComputeUnifiedDiff_HunkHeader(t *testing.T) {
	a := []string{"old\n"}
	b := []string{"new\n"}
	diff := computeUnifiedDiff("test.rb", a, b)

	if !strings.Contains(diff, "@@ -") {
		t.Errorf("diff should contain @@ hunk header, got:\n%s", diff)
	}
}

func TestComputeUnifiedDiff_ContextLines(t *testing.T) {
	// Build a file with a change in the middle, surrounded by context.
	a := []string{
		"line1\n", "line2\n", "line3\n", "line4\n",
		"old_line\n",
		"line6\n", "line7\n", "line8\n", "line9\n",
	}
	b := []string{
		"line1\n", "line2\n", "line3\n", "line4\n",
		"new_line\n",
		"line6\n", "line7\n", "line8\n", "line9\n",
	}
	diff := computeUnifiedDiff("test.rb", a, b)

	// Context should include some surrounding lines (context=3).
	if !strings.Contains(diff, " line2\n") || !strings.Contains(diff, " line3\n") {
		t.Error("diff should include context lines before the change")
	}
	if !strings.Contains(diff, " line6\n") || !strings.Contains(diff, " line7\n") {
		t.Error("diff should include context lines after the change")
	}
	if !strings.Contains(diff, "-old_line") {
		t.Error("diff should contain -old_line")
	}
	if !strings.Contains(diff, "+new_line") {
		t.Error("diff should contain +new_line")
	}
}

// ---------------------------------------------------------------------------
// groupHunks
// ---------------------------------------------------------------------------

func TestGroupHunks_NilEdits(t *testing.T) {
	hunks := groupHunks(nil, 0, 0, 3)
	if len(hunks) != 0 {
		t.Errorf("groupHunks(nil) = %d hunks, want 0", len(hunks))
	}
}

func TestGroupHunks_AllEqual(t *testing.T) {
	edits := []edit{
		{op: editEqual, line: "a\n"},
		{op: editEqual, line: "b\n"},
	}
	hunks := groupHunks(edits, 2, 2, 3)
	if len(hunks) != 0 {
		t.Errorf("groupHunks(all equal) = %d hunks, want 0", len(hunks))
	}
}

func TestGroupHunks_SingleChange(t *testing.T) {
	edits := []edit{
		{op: editEqual, line: "a\n"},
		{op: editDelete, line: "b\n"},
		{op: editInsert, line: "c\n"},
		{op: editEqual, line: "d\n"},
	}
	hunks := groupHunks(edits, 3, 3, 3)
	if len(hunks) != 1 {
		t.Fatalf("groupHunks single change = %d hunks, want 1", len(hunks))
	}
	if hunks[0].origCount == 0 || hunks[0].newCount == 0 {
		t.Error("hunk should have non-zero line counts")
	}
}

func TestGroupHunks_FarApartChanges_TwoHunks(t *testing.T) {
	// Two changes separated by more than 2*context equal lines → two hunks.
	var edits []edit
	edits = append(edits, edit{op: editDelete, line: "old1\n"})
	edits = append(edits, edit{op: editInsert, line: "new1\n"})
	for i := 0; i < 10; i++ {
		edits = append(edits, edit{op: editEqual, line: "ctx\n"})
	}
	edits = append(edits, edit{op: editDelete, line: "old2\n"})
	edits = append(edits, edit{op: editInsert, line: "new2\n"})

	hunks := groupHunks(edits, 12, 12, 3)
	if len(hunks) != 2 {
		t.Errorf("groupHunks far apart = %d hunks, want 2", len(hunks))
	}
}

func TestGroupHunks_CloseChanges_OneHunk(t *testing.T) {
	// Two changes separated by fewer than 2*context equal lines → one hunk.
	var edits []edit
	edits = append(edits, edit{op: editDelete, line: "old1\n"})
	edits = append(edits, edit{op: editInsert, line: "new1\n"})
	for i := 0; i < 3; i++ {
		edits = append(edits, edit{op: editEqual, line: "ctx\n"})
	}
	edits = append(edits, edit{op: editDelete, line: "old2\n"})
	edits = append(edits, edit{op: editInsert, line: "new2\n"})

	hunks := groupHunks(edits, 5, 5, 3)
	if len(hunks) != 1 {
		t.Errorf("groupHunks close = %d hunks, want 1", len(hunks))
	}
}

// ---------------------------------------------------------------------------
// copyDirectory + readAllFiles (filesystem tests)
// ---------------------------------------------------------------------------

func TestCopyDirectory_CopiesFiles(t *testing.T) {
	// Create a source directory with some files.
	srcDir := t.TempDir()
	subDir := filepath.Join(srcDir, "recipes")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(srcDir, "metadata.rb"), "name 'test'\n")
	writeTestFile(t, filepath.Join(subDir, "default.rb"), "log 'hello'\n")

	// Copy.
	dstDir, err := copyDirectory(srcDir)
	if err != nil {
		t.Fatalf("copyDirectory error: %v", err)
	}
	defer os.RemoveAll(dstDir)

	// Verify files exist in the copy.
	assertFileContent(t, filepath.Join(dstDir, "metadata.rb"), "name 'test'\n")
	assertFileContent(t, filepath.Join(dstDir, "recipes", "default.rb"), "log 'hello'\n")
}

func TestCopyDirectory_IndependentOfOriginal(t *testing.T) {
	srcDir := t.TempDir()
	writeTestFile(t, filepath.Join(srcDir, "test.rb"), "original\n")

	dstDir, err := copyDirectory(srcDir)
	if err != nil {
		t.Fatalf("copyDirectory error: %v", err)
	}
	defer os.RemoveAll(dstDir)

	// Modify the copy — should not affect original.
	writeTestFile(t, filepath.Join(dstDir, "test.rb"), "modified\n")
	assertFileContent(t, filepath.Join(srcDir, "test.rb"), "original\n")
}

func TestCopyDirectory_EmptyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir, err := copyDirectory(srcDir)
	if err != nil {
		t.Fatalf("copyDirectory error: %v", err)
	}
	defer os.RemoveAll(dstDir)

	// Verify the copy exists and is a directory.
	info, err := os.Stat(dstDir)
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}
	if !info.IsDir() {
		t.Error("copy should be a directory")
	}
}

func TestCopyDirectory_NonexistentSource(t *testing.T) {
	_, err := copyDirectory("/nonexistent/path/that/should/not/exist")
	if err == nil {
		t.Error("expected error for nonexistent source directory")
	}
}

// ---------------------------------------------------------------------------
// readAllFiles
// ---------------------------------------------------------------------------

func TestReadAllFiles_ReadsFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a.rb"), "aaa")
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(subDir, "b.rb"), "bbb")

	files, err := readAllFiles(dir)
	if err != nil {
		t.Fatalf("readAllFiles error: %v", err)
	}

	if files["a.rb"] != "aaa" {
		t.Errorf("a.rb = %q, want %q", files["a.rb"], "aaa")
	}
	if files[filepath.Join("sub", "b.rb")] != "bbb" {
		t.Errorf("sub/b.rb = %q, want %q", files[filepath.Join("sub", "b.rb")], "bbb")
	}
}

func TestReadAllFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := readAllFiles(dir)
	if err != nil {
		t.Fatalf("readAllFiles error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("readAllFiles empty dir = %d files, want 0", len(files))
	}
}

// ---------------------------------------------------------------------------
// copyFile
// ---------------------------------------------------------------------------

func TestCopyFile_CopiesToDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	writeTestFile(t, src, "content here")
	if err := copyFile(src, dst, 0o644); err != nil {
		t.Fatalf("copyFile error: %v", err)
	}
	assertFileContent(t, dst, "content here")
}

func TestCopyFile_NonexistentSource(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "no_such_file"), filepath.Join(dir, "dst"), 0o644)
	if err == nil {
		t.Error("expected error for nonexistent source")
	}
}

// ---------------------------------------------------------------------------
// AutocorrectPreviewResult type sanity
// ---------------------------------------------------------------------------

func TestAutocorrectPreviewResult_SkippedState(t *testing.T) {
	pr := AutocorrectPreviewResult{
		CookbookID: "cb-1",
		Skipped:    true,
		SkipReason: "zero offenses",
	}
	if !pr.Skipped {
		t.Error("expected Skipped = true")
	}
	if pr.SkipReason == "" {
		t.Error("expected non-empty SkipReason")
	}
}

func TestAutocorrectPreviewResult_ErrorState(t *testing.T) {
	pr := AutocorrectPreviewResult{
		CookbookID: "cb-2",
		Error:      os.ErrNotExist,
	}
	if pr.Error == nil {
		t.Error("expected non-nil Error")
	}
}

func TestAutocorrectBatchResult_Defaults(t *testing.T) {
	br := AutocorrectBatchResult{}
	if br.Total != 0 || br.Generated != 0 || br.Skipped != 0 || br.Errors != 0 {
		t.Error("default batch result should have zero counts")
	}
}

// ---------------------------------------------------------------------------
// autocorrectJSONOutput parsing
// ---------------------------------------------------------------------------

func TestAutocorrectJSONOutput_Parse(t *testing.T) {
	// Verify the type can parse CookStyle JSON output.
	jsonStr := `{"summary":{"offense_count":3,"target_file_count":2,"inspected_file_count":5}}`
	var out autocorrectJSONOutput
	if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if out.Summary.OffenseCount != 3 {
		t.Errorf("offense_count = %d, want 3", out.Summary.OffenseCount)
	}
	if out.Summary.TargetFileCount != 2 {
		t.Errorf("target_file_count = %d, want 2", out.Summary.TargetFileCount)
	}
	if out.Summary.InspectedFileCount != 5 {
		t.Errorf("inspected_file_count = %d, want 5", out.Summary.InspectedFileCount)
	}
}

// ---------------------------------------------------------------------------
// Integration-style test: diff of a realistic cookbook change
// ---------------------------------------------------------------------------

func TestGenerateUnifiedDiffs_RealisticCookbookChange(t *testing.T) {
	original := map[string]string{
		"recipes/default.rb": `#
# Cookbook:: my_cookbook
# Recipe:: default
#

node.set['my_cookbook']['port'] = 8080
node.set['my_cookbook']['host'] = 'localhost'

package 'nginx' do
  action :install
end

service 'nginx' do
  action :start
end
`,
	}

	modified := map[string]string{
		"recipes/default.rb": `#
# Cookbook:: my_cookbook
# Recipe:: default
#

node.normal['my_cookbook']['port'] = 8080
node.normal['my_cookbook']['host'] = 'localhost'

package 'nginx' do
  action :install
end

service 'nginx' do
  action :start
end
`,
	}

	diff, changed := generateUnifiedDiffs(original, modified)
	if changed != 1 {
		t.Errorf("changed = %d, want 1", changed)
	}
	if !strings.Contains(diff, "-node.set") {
		t.Error("diff should contain removed node.set lines")
	}
	if !strings.Contains(diff, "+node.normal") {
		t.Error("diff should contain added node.normal lines")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file %s: %v", path, err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("file %s content = %q, want %q", path, string(got), want)
	}
}
