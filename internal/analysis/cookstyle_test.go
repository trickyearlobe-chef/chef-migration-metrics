// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// ---------------------------------------------------------------------------
// Fake CookStyle executor
// ---------------------------------------------------------------------------

// fakeCookstyleExecCall records a single invocation.
type fakeCookstyleExecCall struct {
	Args []string
}

// fakeCookstyleExecutor implements CookstyleExecutor for testing.
type fakeCookstyleExecutor struct {
	// calls records every invocation for assertion.
	calls []fakeCookstyleExecCall

	// result is the canned response returned for every call.
	stdout   string
	stderr   string
	exitCode int
	err      error

	// perDir allows per-cookbook-directory overrides. When the last argument
	// (the cookbook dir) matches a key here, that result is used instead of
	// the default.
	perDir map[string]fakeCookstyleResult
}

type fakeCookstyleResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func (f *fakeCookstyleExecutor) Run(_ context.Context, args ...string) (string, string, int, error) {
	f.calls = append(f.calls, fakeCookstyleExecCall{Args: args})

	// Check for a per-directory override (last arg is the cookbook dir).
	if len(args) > 0 && f.perDir != nil {
		dir := args[len(args)-1]
		if r, ok := f.perDir[dir]; ok {
			return r.stdout, r.stderr, r.exitCode, r.err
		}
	}

	return f.stdout, f.stderr, f.exitCode, f.err
}

// ---------------------------------------------------------------------------
// Helpers to build CookStyle JSON output
// ---------------------------------------------------------------------------

func makeCookstyleJSON(files []CookstyleFile, offenseCount int) string {
	out := CookstyleOutput{
		Metadata: CookstyleMetadata{
			RubocopVersion: "1.25.0",
			RubyEngine:     "ruby",
			RubyVersion:    "3.1.0",
		},
		Files: files,
		Summary: CookstyleSummary{
			OffenseCount:       offenseCount,
			TargetFileCount:    len(files),
			InspectedFileCount: len(files),
		},
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func makeCleanJSON() string {
	return makeCookstyleJSON(nil, 0)
}

func makeOffenseJSON(copName, severity, message string, corrected bool) string {
	files := []CookstyleFile{
		{
			Path: "recipes/default.rb",
			Offenses: []CookstyleOffense{
				{
					Severity:  severity,
					Message:   message,
					CopName:   copName,
					Corrected: corrected,
					Location: CookstyleOffenseLocation{
						StartLine:   10,
						StartColumn: 1,
						LastLine:    10,
						LastColumn:  40,
					},
				},
			},
		},
	}
	return makeCookstyleJSON(files, 1)
}

func makeMultiOffenseJSON(offenses []CookstyleOffense) string {
	files := []CookstyleFile{
		{
			Path:     "recipes/default.rb",
			Offenses: offenses,
		},
	}
	return makeCookstyleJSON(files, len(offenses))
}

// ---------------------------------------------------------------------------
// CookstyleOutput JSON parsing tests
// ---------------------------------------------------------------------------

func TestCookstyleOutput_ParseClean(t *testing.T) {
	raw := makeCleanJSON()
	var out CookstyleOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("failed to parse clean JSON: %v", err)
	}
	if out.Summary.OffenseCount != 0 {
		t.Errorf("expected 0 offenses, got %d", out.Summary.OffenseCount)
	}
	if out.Metadata.RubocopVersion != "1.25.0" {
		t.Errorf("unexpected rubocop version: %q", out.Metadata.RubocopVersion)
	}
}

func TestCookstyleOutput_ParseWithOffenses(t *testing.T) {
	raw := makeOffenseJSON(
		"Chef/Deprecations/ResourceWithoutUnifiedTrue",
		"warning",
		"Custom resources should set unified_mode true",
		false,
	)
	var out CookstyleOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("failed to parse JSON with offenses: %v", err)
	}
	if out.Summary.OffenseCount != 1 {
		t.Errorf("expected 1 offense, got %d", out.Summary.OffenseCount)
	}
	if len(out.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(out.Files))
	}
	off := out.Files[0].Offenses[0]
	if off.CopName != "Chef/Deprecations/ResourceWithoutUnifiedTrue" {
		t.Errorf("unexpected cop name: %q", off.CopName)
	}
	if off.Severity != "warning" {
		t.Errorf("unexpected severity: %q", off.Severity)
	}
	if off.Corrected {
		t.Error("expected corrected=false")
	}
	if off.Location.StartLine != 10 {
		t.Errorf("expected start_line=10, got %d", off.Location.StartLine)
	}
}

func TestCookstyleOutput_ParseMultipleFiles(t *testing.T) {
	files := []CookstyleFile{
		{
			Path: "recipes/default.rb",
			Offenses: []CookstyleOffense{
				{Severity: "warning", CopName: "Chef/Deprecations/NodeSet", Message: "msg1"},
			},
		},
		{
			Path: "recipes/install.rb",
			Offenses: []CookstyleOffense{
				{Severity: "error", CopName: "Chef/Correctness/InvalidDefaultAction", Message: "msg2"},
				{Severity: "convention", CopName: "Chef/Style/FileMode", Message: "msg3", Corrected: true},
			},
		},
	}
	raw := makeCookstyleJSON(files, 3)

	var out CookstyleOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if out.Summary.OffenseCount != 3 {
		t.Errorf("expected 3 offenses, got %d", out.Summary.OffenseCount)
	}
	if len(out.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(out.Files))
	}
}

// ---------------------------------------------------------------------------
// Namespace helpers
// ---------------------------------------------------------------------------

func TestIsDeprecation(t *testing.T) {
	tests := []struct {
		cop  string
		want bool
	}{
		{"Chef/Deprecations/NodeSet", true},
		{"Chef/Deprecations/ResourceWithoutUnifiedTrue", true},
		{"Chef/Correctness/MetadataMissingName", false},
		{"Chef/Style/FileMode", false},
		{"Chef/Modernize/FoodcriticComments", false},
		{"Layout/EmptyLines", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isDeprecation(tt.cop); got != tt.want {
			t.Errorf("isDeprecation(%q) = %v, want %v", tt.cop, got, tt.want)
		}
	}
}

func TestIsCorrectness(t *testing.T) {
	tests := []struct {
		cop  string
		want bool
	}{
		{"Chef/Correctness/MetadataMissingName", true},
		{"Chef/Correctness/InvalidDefaultAction", true},
		{"Chef/Deprecations/NodeSet", false},
		{"Chef/Style/FileMode", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isCorrectness(tt.cop); got != tt.want {
			t.Errorf("isCorrectness(%q) = %v, want %v", tt.cop, got, tt.want)
		}
	}
}

func TestIsErrorOrFatal(t *testing.T) {
	tests := []struct {
		severity string
		want     bool
	}{
		{"error", true},
		{"fatal", true},
		{"warning", false},
		{"convention", false},
		{"refactor", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isErrorOrFatal(tt.severity); got != tt.want {
			t.Errorf("isErrorOrFatal(%q) = %v, want %v", tt.severity, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// buildCookstyleArgs tests
// ---------------------------------------------------------------------------

func TestBuildCookstyleArgs_NoTargetVersion(t *testing.T) {
	args := buildCookstyleArgs("/path/to/cookbook", "")
	assertContains(t, args, "--format")
	assertContains(t, args, "json")
	assertContains(t, args, "/path/to/cookbook")

	// Should NOT contain --only when no target version.
	for _, a := range args {
		if a == "--only" {
			t.Error("--only should not be present when target version is empty")
		}
	}
}

func TestBuildCookstyleArgs_WithTargetVersion_NoCookbookConfig(t *testing.T) {
	// buildCookstyleArgs writes a sidecar .rubocop_cmm.yml into the
	// cookbook dir, so we need a real temp directory.
	cookbookDir := t.TempDir()

	args := buildCookstyleArgs(cookbookDir, "18.0")

	assertContains(t, args, "--format")
	assertContains(t, args, "json")
	assertContains(t, args, "--config")
	assertContains(t, args, "--only")
	assertContains(t, args, cookbookDir)

	// --target-chef-version is NOT a CLI flag — it is written to .rubocop_cmm.yml.
	for _, a := range args {
		if a == "--target-chef-version" {
			t.Error("--target-chef-version should not be a CLI arg; it belongs in .rubocop_cmm.yml")
		}
	}

	// Find the --only value — should use Chef/Deprecations format.
	for i, a := range args {
		if a == "--only" && i+1 < len(args) {
			val := args[i+1]
			if !strings.Contains(val, "Chef/Deprecations") {
				t.Errorf("--only should include Chef/Deprecations, got %q", val)
			}
			if !strings.Contains(val, "Chef/Correctness") {
				t.Errorf("--only should include Chef/Correctness, got %q", val)
			}
		}
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

func TestBuildCookstyleArgs_WithTargetVersion_WithCookbookConfig(t *testing.T) {
	cookbookDir := t.TempDir()

	// Simulate a cookbook that already has its own .rubocop.yml.
	existingConfig := "require:\n  - cookstyle\n\nChef/Style/TrueClassFalseClassResourceProperties:\n  Enabled: false\n"
	if err := os.WriteFile(filepath.Join(cookbookDir, ".rubocop.yml"), []byte(existingConfig), 0644); err != nil {
		t.Fatal(err)
	}

	args := buildCookstyleArgs(cookbookDir, "17.0")
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

func TestBuildCookstyleArgs_NoSidecarWithoutTarget(t *testing.T) {
	cookbookDir := t.TempDir()
	buildCookstyleArgs(cookbookDir, "")

	if _, err := os.Stat(filepath.Join(cookbookDir, cmmConfigName)); err == nil {
		t.Errorf("%s should not be written when target version is empty", cmmConfigName)
	}
}

func TestBuildCookstyleArgs_CookbookDirIsLast(t *testing.T) {
	cookbookDir := t.TempDir()
	args := buildCookstyleArgs(cookbookDir, "17.0")
	if len(args) == 0 {
		t.Fatal("expected non-empty args")
	}
	last := args[len(args)-1]
	if last != cookbookDir {
		t.Errorf("last arg should be cookbook dir, got %q", last)
	}
}

// ---------------------------------------------------------------------------
// CookstyleScanResult classification tests
// ---------------------------------------------------------------------------

// TestClassifyOffenses_Pass verifies that a scan with only warning-level
// offenses is classified as passed.
func TestClassifyOffenses_Pass(t *testing.T) {
	offenses := []CookstyleOffense{
		{Severity: "warning", CopName: "Chef/Deprecations/NodeSet", Message: "deprecated"},
		{Severity: "convention", CopName: "Chef/Style/FileMode", Message: "style"},
	}
	raw := makeMultiOffenseJSON(offenses)

	var out CookstyleOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	sr := classifyOutput(out)
	if !sr.Passed {
		t.Error("expected Passed=true for warning/convention offenses only")
	}
	if sr.OffenseCount != 2 {
		t.Errorf("expected 2 offenses, got %d", sr.OffenseCount)
	}
	if sr.DeprecationCount != 1 {
		t.Errorf("expected 1 deprecation, got %d", sr.DeprecationCount)
	}
}

// TestClassifyOffenses_Fail verifies that an error-severity offense causes
// the scan to fail.
func TestClassifyOffenses_Fail(t *testing.T) {
	offenses := []CookstyleOffense{
		{Severity: "warning", CopName: "Chef/Deprecations/NodeSet", Message: "deprecated"},
		{Severity: "error", CopName: "Chef/Correctness/InvalidDefaultAction", Message: "bad"},
	}
	raw := makeMultiOffenseJSON(offenses)

	var out CookstyleOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	sr := classifyOutput(out)
	if sr.Passed {
		t.Error("expected Passed=false when error-severity offense is present")
	}
	if sr.CorrectnessCount != 1 {
		t.Errorf("expected 1 correctness offense, got %d", sr.CorrectnessCount)
	}
}

// TestClassifyOffenses_Fatal verifies that a fatal-severity offense fails.
func TestClassifyOffenses_Fatal(t *testing.T) {
	offenses := []CookstyleOffense{
		{Severity: "fatal", CopName: "Chef/Correctness/SomethingBad", Message: "fatal"},
	}
	raw := makeMultiOffenseJSON(offenses)

	var out CookstyleOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	sr := classifyOutput(out)
	if sr.Passed {
		t.Error("expected Passed=false for fatal offense")
	}
}

// TestClassifyOffenses_Correctable verifies the correctable counter.
func TestClassifyOffenses_Correctable(t *testing.T) {
	offenses := []CookstyleOffense{
		{Severity: "warning", CopName: "Chef/Deprecations/NodeSet", Message: "a", Corrected: true},
		{Severity: "warning", CopName: "Chef/Deprecations/EpicFail", Message: "b", Corrected: true},
		{Severity: "convention", CopName: "Chef/Style/FileMode", Message: "c", Corrected: false},
	}
	raw := makeMultiOffenseJSON(offenses)

	var out CookstyleOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	sr := classifyOutput(out)
	if sr.CorrectableCount != 2 {
		t.Errorf("expected 2 correctable, got %d", sr.CorrectableCount)
	}
}

// TestClassifyOffenses_DeprecationWarnings verifies the deprecation subset.
func TestClassifyOffenses_DeprecationWarnings(t *testing.T) {
	offenses := []CookstyleOffense{
		{Severity: "warning", CopName: "Chef/Deprecations/NodeSet", Message: "dep1"},
		{Severity: "error", CopName: "Chef/Correctness/InvalidDefaultAction", Message: "corr"},
		{Severity: "warning", CopName: "Chef/Deprecations/EpicFail", Message: "dep2"},
		{Severity: "convention", CopName: "Chef/Style/FileMode", Message: "style"},
	}
	raw := makeMultiOffenseJSON(offenses)

	var out CookstyleOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	sr := classifyOutput(out)
	if len(sr.DeprecationWarnings) != 2 {
		t.Errorf("expected 2 deprecation warnings, got %d", len(sr.DeprecationWarnings))
	}
	for _, dw := range sr.DeprecationWarnings {
		if !isDeprecation(dw.CopName) {
			t.Errorf("deprecation warning has unexpected cop: %q", dw.CopName)
		}
	}
}

// TestClassifyOffenses_Empty verifies a clean scan.
func TestClassifyOffenses_Empty(t *testing.T) {
	raw := makeCleanJSON()

	var out CookstyleOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	sr := classifyOutput(out)
	if !sr.Passed {
		t.Error("expected Passed=true for clean scan")
	}
	if sr.OffenseCount != 0 {
		t.Errorf("expected 0 offenses, got %d", sr.OffenseCount)
	}
	if sr.DeprecationCount != 0 {
		t.Errorf("expected 0 deprecations, got %d", sr.DeprecationCount)
	}
	if sr.CorrectnessCount != 0 {
		t.Errorf("expected 0 correctness, got %d", sr.CorrectnessCount)
	}
	if sr.CorrectableCount != 0 {
		t.Errorf("expected 0 correctable, got %d", sr.CorrectableCount)
	}
	if len(sr.Offenses) != 0 {
		t.Errorf("expected empty offenses list, got %d", len(sr.Offenses))
	}
}

// classifyOutput is a test helper that replicates the classification logic
// from scanOne so we can test it in isolation without needing a database.
func classifyOutput(output CookstyleOutput) CookstyleScanResult {
	sr := CookstyleScanResult{
		OffenseCount: output.Summary.OffenseCount,
		Passed:       true,
	}
	for _, file := range output.Files {
		for _, off := range file.Offenses {
			sr.Offenses = append(sr.Offenses, off)
			if isDeprecation(off.CopName) {
				sr.DeprecationCount++
				sr.DeprecationWarnings = append(sr.DeprecationWarnings, off)
			}
			if isCorrectness(off.CopName) {
				sr.CorrectnessCount++
			}
			if off.Corrected {
				sr.CorrectableCount++
			}
			if isErrorOrFatal(off.Severity) {
				sr.Passed = false
			}
		}
	}
	return sr
}

// ---------------------------------------------------------------------------
// enrichOffenses tests
// ---------------------------------------------------------------------------

func TestEnrichOffenses_Nil(t *testing.T) {
	result := enrichOffenses(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestEnrichOffenses_Empty(t *testing.T) {
	result := enrichOffenses([]CookstyleOffense{})
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestEnrichOffenses_KnownCop(t *testing.T) {
	// Use a cop name that exists in the embedded mapping table.
	// ChefDeprecations/ResourceWithoutUnifiedTrue is one of the first entries.
	copName := "Chef/Deprecations/ResourceWithoutUnifiedTrue"
	mapping := remediation.LookupCop(copName)
	if mapping == nil {
		t.Skipf("cop %q not in embedded mapping table — cannot test enrichment", copName)
	}

	offenses := []CookstyleOffense{
		{
			Severity:  "warning",
			Message:   "Custom resources should use unified_mode",
			CopName:   copName,
			Corrected: false,
			Location: CookstyleOffenseLocation{
				StartLine:   5,
				StartColumn: 1,
				LastLine:    5,
				LastColumn:  30,
			},
		},
	}

	enriched := enrichOffenses(offenses)
	if len(enriched) != 1 {
		t.Fatalf("expected 1 enriched offense, got %d", len(enriched))
	}

	e := enriched[0]
	if e.CopName != copName {
		t.Errorf("CopName = %q, want %q", e.CopName, copName)
	}
	if e.Severity != "warning" {
		t.Errorf("Severity = %q, want %q", e.Severity, "warning")
	}
	if e.Message != "Custom resources should use unified_mode" {
		t.Errorf("Message = %q, want original message", e.Message)
	}
	if e.Remediation == nil {
		t.Fatal("expected non-nil Remediation for known cop")
	}
	if e.Remediation.CopName != copName {
		t.Errorf("Remediation.CopName = %q, want %q", e.Remediation.CopName, copName)
	}
	if e.Remediation.MigrationURL == "" {
		t.Error("expected non-empty MigrationURL for known cop")
	}
}

func TestEnrichOffenses_UnknownCop(t *testing.T) {
	offenses := []CookstyleOffense{
		{
			Severity:  "convention",
			Message:   "some unknown rule",
			CopName:   "Chef/Style/UnknownCopThatDoesNotExist",
			Corrected: false,
			Location: CookstyleOffenseLocation{
				StartLine:   1,
				StartColumn: 1,
				LastLine:    1,
				LastColumn:  10,
			},
		},
	}

	enriched := enrichOffenses(offenses)
	if len(enriched) != 1 {
		t.Fatalf("expected 1 enriched offense, got %d", len(enriched))
	}

	e := enriched[0]
	if e.CopName != "Chef/Style/UnknownCopThatDoesNotExist" {
		t.Errorf("CopName = %q, want original cop name", e.CopName)
	}
	if e.Remediation != nil {
		t.Errorf("expected nil Remediation for unknown cop, got %+v", e.Remediation)
	}
}

func TestEnrichOffenses_LocationFidelity(t *testing.T) {
	offenses := []CookstyleOffense{
		{
			Severity: "warning",
			Message:  "test",
			CopName:  "Chef/Deprecations/SomeRule",
			Location: CookstyleOffenseLocation{
				StartLine:   42,
				StartColumn: 7,
				LastLine:    50,
				LastColumn:  99,
			},
		},
	}

	enriched := enrichOffenses(offenses)
	if len(enriched) != 1 {
		t.Fatalf("expected 1 enriched offense, got %d", len(enriched))
	}

	loc := enriched[0].Location
	if loc.StartLine != 42 {
		t.Errorf("StartLine = %d, want 42", loc.StartLine)
	}
	if loc.StartColumn != 7 {
		t.Errorf("StartColumn = %d, want 7", loc.StartColumn)
	}
	if loc.LastLine != 50 {
		t.Errorf("LastLine = %d, want 50", loc.LastLine)
	}
	if loc.LastColumn != 99 {
		t.Errorf("LastColumn = %d, want 99", loc.LastColumn)
	}
}

func TestEnrichOffenses_MixedKnownAndUnknown(t *testing.T) {
	// Pick a known cop from the mapping table.
	knownCop := "Chef/Deprecations/ResourceWithoutUnifiedTrue"
	if remediation.LookupCop(knownCop) == nil {
		t.Skipf("cop %q not in embedded mapping table", knownCop)
	}

	offenses := []CookstyleOffense{
		{
			Severity: "warning",
			Message:  "known offense",
			CopName:  knownCop,
			Location: CookstyleOffenseLocation{StartLine: 1, StartColumn: 1, LastLine: 1, LastColumn: 10},
		},
		{
			Severity: "convention",
			Message:  "unknown offense",
			CopName:  "Chef/Style/TotallyMadeUp",
			Location: CookstyleOffenseLocation{StartLine: 20, StartColumn: 5, LastLine: 20, LastColumn: 40},
		},
		{
			Severity: "error",
			Message:  "another known or unknown",
			CopName:  "Chef/Correctness/AlsoMadeUp",
			Location: CookstyleOffenseLocation{StartLine: 30, StartColumn: 1, LastLine: 35, LastColumn: 1},
		},
	}

	enriched := enrichOffenses(offenses)
	if len(enriched) != 3 {
		t.Fatalf("expected 3 enriched offenses, got %d", len(enriched))
	}

	// First should have remediation.
	if enriched[0].Remediation == nil {
		t.Error("expected non-nil Remediation for known cop at index 0")
	}
	// Second should not.
	if enriched[1].Remediation != nil {
		t.Error("expected nil Remediation for unknown cop at index 1")
	}
	// Third should not (also unknown).
	if enriched[2].Remediation != nil {
		t.Error("expected nil Remediation for unknown cop at index 2")
	}

	// Verify ordering and field preservation.
	if enriched[0].Message != "known offense" {
		t.Errorf("enriched[0].Message = %q, want %q", enriched[0].Message, "known offense")
	}
	if enriched[1].Message != "unknown offense" {
		t.Errorf("enriched[1].Message = %q, want %q", enriched[1].Message, "unknown offense")
	}
	if enriched[2].Severity != "error" {
		t.Errorf("enriched[2].Severity = %q, want %q", enriched[2].Severity, "error")
	}
}

func TestEnrichOffenses_JSONRoundTrip(t *testing.T) {
	offenses := []CookstyleOffense{
		{
			Severity:  "warning",
			Message:   "test msg",
			CopName:   "Chef/Deprecations/ResourceWithoutUnifiedTrue",
			Corrected: true,
			Location:  CookstyleOffenseLocation{StartLine: 1, StartColumn: 1, LastLine: 1, LastColumn: 10},
		},
	}

	enriched := enrichOffenses(offenses)
	data, err := json.Marshal(enriched)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded []remediation.EnrichedOffense
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded) != 1 {
		t.Fatalf("expected 1 decoded offense, got %d", len(decoded))
	}
	if decoded[0].CopName != offenses[0].CopName {
		t.Errorf("CopName = %q, want %q", decoded[0].CopName, offenses[0].CopName)
	}
	if decoded[0].Remediation == nil {
		t.Fatal("expected Remediation to survive JSON round-trip")
	}
	if decoded[0].Remediation.CopName != offenses[0].CopName {
		t.Errorf("Remediation.CopName = %q after round-trip, want %q",
			decoded[0].Remediation.CopName, offenses[0].CopName)
	}
}

// ---------------------------------------------------------------------------
// relativeCookstylePath tests
// ---------------------------------------------------------------------------

func TestRelativeCookstylePath_TempDir(t *testing.T) {
	// Simulates the streaming pipeline temp dir path.
	got := relativeCookstylePath(
		"/tmp/cmm-cb-apache2-5.0.1-abc123/recipes/default.rb",
		"/tmp/cmm-cb-apache2-5.0.1-abc123",
	)
	if got != "recipes/default.rb" {
		t.Errorf("got %q, want %q", got, "recipes/default.rb")
	}
}

func TestRelativeCookstylePath_TempDirTrailingSlash(t *testing.T) {
	// cookbookDir already has a trailing separator.
	got := relativeCookstylePath(
		"/tmp/cmm-cb-apache2-5.0.1-abc123/recipes/default.rb",
		"/tmp/cmm-cb-apache2-5.0.1-abc123/",
	)
	if got != "recipes/default.rb" {
		t.Errorf("got %q, want %q", got, "recipes/default.rb")
	}
}

func TestRelativeCookstylePath_GitDir(t *testing.T) {
	// Git cookbook clone path.
	got := relativeCookstylePath(
		"/data/git-cookbooks/nginx/attributes/default.rb",
		"/data/git-cookbooks/nginx",
	)
	if got != "attributes/default.rb" {
		t.Errorf("got %q, want %q", got, "attributes/default.rb")
	}
}

func TestRelativeCookstylePath_NestedFile(t *testing.T) {
	got := relativeCookstylePath(
		"/tmp/cmm-cb-java-2.0.0-xyz/recipes/sub/nested/deep.rb",
		"/tmp/cmm-cb-java-2.0.0-xyz",
	)
	if got != "recipes/sub/nested/deep.rb" {
		t.Errorf("got %q, want %q", got, "recipes/sub/nested/deep.rb")
	}
}

func TestRelativeCookstylePath_TopLevelFile(t *testing.T) {
	got := relativeCookstylePath(
		"/tmp/cmm-cb-mycb-1.0.0-def/metadata.rb",
		"/tmp/cmm-cb-mycb-1.0.0-def",
	)
	if got != "metadata.rb" {
		t.Errorf("got %q, want %q", got, "metadata.rb")
	}
}

func TestRelativeCookstylePath_EmptyCookbookDir(t *testing.T) {
	// When cookbookDir is empty, the path should be returned as-is.
	path := "/tmp/cmm-cb-apache2-5.0.1-abc123/recipes/default.rb"
	got := relativeCookstylePath(path, "")
	if got != path {
		t.Errorf("got %q, want %q (unchanged)", got, path)
	}
}

func TestRelativeCookstylePath_NoMatch(t *testing.T) {
	// Path does not start with cookbookDir — returned unchanged.
	path := "/some/other/path/recipes/default.rb"
	got := relativeCookstylePath(path, "/tmp/cmm-cb-apache2-5.0.1-abc123")
	if got != path {
		t.Errorf("got %q, want %q (unchanged)", got, path)
	}
}

func TestRelativeCookstylePath_PartialDirNameNoFalseMatch(t *testing.T) {
	// Ensure "/tmp/cb" does not falsely match "/tmp/cb-extra/file.rb".
	path := "/tmp/cb-extra/recipes/default.rb"
	got := relativeCookstylePath(path, "/tmp/cb")
	if got != path {
		t.Errorf("got %q, want %q (should not match partial dir name)", got, path)
	}
}

func TestRelativeCookstylePath_RelativeInput(t *testing.T) {
	// If CookStyle somehow returns a relative path already, leave it alone.
	got := relativeCookstylePath("recipes/default.rb", "/tmp/cmm-cb-x-1.0.0-abc")
	if got != "recipes/default.rb" {
		t.Errorf("got %q, want %q", got, "recipes/default.rb")
	}
}

// ---------------------------------------------------------------------------
// NewCookstyleScanner tests
// ---------------------------------------------------------------------------

func TestNewCookstyleScanner_Defaults(t *testing.T) {
	fe := &fakeCookstyleExecutor{}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 0, 0,
		WithCookstyleExecutor(fe))

	if s.concurrency != 1 {
		t.Errorf("expected concurrency=1 for zero input, got %d", s.concurrency)
	}
	if s.timeout != 10*time.Minute {
		t.Errorf("expected timeout=10m for zero input, got %v", s.timeout)
	}
	if s.cookstylePath != "/usr/bin/cookstyle" {
		t.Errorf("unexpected cookstylePath: %q", s.cookstylePath)
	}
}

func TestNewCookstyleScanner_CustomValues(t *testing.T) {
	fe := &fakeCookstyleExecutor{}
	s := NewCookstyleScanner(nil, nil, "/opt/embedded/bin/cookstyle", 4, 15,
		WithCookstyleExecutor(fe))

	if s.concurrency != 4 {
		t.Errorf("expected concurrency=4, got %d", s.concurrency)
	}
	if s.timeout != 15*time.Minute {
		t.Errorf("expected timeout=15m, got %v", s.timeout)
	}
}

func TestNewCookstyleScanner_NegativeDefaults(t *testing.T) {
	fe := &fakeCookstyleExecutor{}
	s := NewCookstyleScanner(nil, nil, "/bin/cookstyle", -5, -1,
		WithCookstyleExecutor(fe))

	if s.concurrency != 1 {
		t.Errorf("expected concurrency=1 for negative, got %d", s.concurrency)
	}
	if s.timeout != 10*time.Minute {
		t.Errorf("expected timeout=10m for negative, got %v", s.timeout)
	}
}

// ---------------------------------------------------------------------------
// Executor call tests (verify correct args are passed)
// ---------------------------------------------------------------------------

func TestExecutor_CalledWithFormatJSON(t *testing.T) {
	fe := &fakeCookstyleExecutor{
		stdout: makeCleanJSON(),
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	s.scanOneNoDB(context.Background(), "test-cookbook", "1.0.0", "", "/cookbooks/test")

	if len(fe.calls) == 0 {
		t.Fatal("expected at least one executor call")
	}

	args := fe.calls[0].Args
	foundFormat := false
	foundJSON := false
	for i, a := range args {
		if a == "--format" && i+1 < len(args) && args[i+1] == "json" {
			foundFormat = true
			foundJSON = true
		}
	}
	if !foundFormat || !foundJSON {
		t.Errorf("expected --format json in args, got %v", args)
	}
}

func TestExecutor_CalledWithOnlyWhenTargetVersion(t *testing.T) {
	cookbookDir := t.TempDir()
	fe := &fakeCookstyleExecutor{
		stdout: makeCleanJSON(),
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	s.scanOneNoDB(context.Background(), "test-cookbook", "1.0.0", "18.0", cookbookDir)

	if len(fe.calls) == 0 {
		t.Fatal("expected at least one executor call")
	}

	args := fe.calls[0].Args
	foundOnly := false
	foundConfig := false
	for i, a := range args {
		if a == "--only" && i+1 < len(args) {
			foundOnly = true
			val := args[i+1]
			if !strings.Contains(val, "Chef/Deprecations") || !strings.Contains(val, "Chef/Correctness") {
				t.Errorf("--only value should contain both namespaces, got %q", val)
			}
		}
		if a == "--config" {
			foundConfig = true
		}
		if a == "--target-chef-version" {
			t.Error("--target-chef-version should not be a CLI arg; it belongs in .rubocop_cmm.yml")
		}
	}
	if !foundOnly {
		t.Errorf("expected --only in args when target version is set, got %v", args)
	}
	if !foundConfig {
		t.Errorf("expected --config in args when target version is set, got %v", args)
	}
}

func TestExecutor_NotCalledWithOnlyWhenNoTarget(t *testing.T) {
	cookbookDir := t.TempDir()
	fe := &fakeCookstyleExecutor{
		stdout: makeCleanJSON(),
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	s.scanOneNoDB(context.Background(), "test-cookbook", "1.0.0", "", cookbookDir)

	if len(fe.calls) == 0 {
		t.Fatal("expected at least one executor call")
	}

	args := fe.calls[0].Args
	for _, a := range args {
		if a == "--only" {
			t.Error("--only should not be present when target version is empty")
		}
		if a == "--config" {
			t.Error("--config should not be present when target version is empty")
		}
	}
}

func TestExecutor_CookbookDirIsLastArg(t *testing.T) {
	cookbookDir := t.TempDir()
	fe := &fakeCookstyleExecutor{
		stdout: makeCleanJSON(),
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	s.scanOneNoDB(context.Background(), "test-cookbook", "1.0.0", "", cookbookDir)

	if len(fe.calls) == 0 {
		t.Fatal("expected at least one executor call")
	}

	args := fe.calls[0].Args
	if args[len(args)-1] != cookbookDir {
		t.Errorf("last arg should be cookbook dir, got %q", args[len(args)-1])
	}
}

// ---------------------------------------------------------------------------
// Scan result parsing tests (via scanOneNoDB helper)
// ---------------------------------------------------------------------------

func TestScanOneNoDB_CleanCookbook(t *testing.T) {
	fe := &fakeCookstyleExecutor{
		stdout: makeCleanJSON(),
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	sr := s.scanOneNoDB(context.Background(), "clean-cb", "1.0.0", "", "/cookbooks/clean")

	if !sr.Passed {
		t.Error("expected Passed=true for clean cookbook")
	}
	if sr.Error != nil {
		t.Errorf("expected nil error, got: %v", sr.Error)
	}
	if sr.OffenseCount != 0 {
		t.Errorf("expected 0 offenses, got %d", sr.OffenseCount)
	}
	if sr.CookbookName != "clean-cb" {
		t.Errorf("expected CookbookName=clean-cb, got %q", sr.CookbookName)
	}
	if sr.CookbookVersion != "1.0.0" {
		t.Errorf("expected CookbookVersion=1.0.0, got %q", sr.CookbookVersion)
	}
}

func TestScanOneNoDB_DeprecationWarnings(t *testing.T) {
	fe := &fakeCookstyleExecutor{
		stdout: makeOffenseJSON(
			"Chef/Deprecations/NodeSet",
			"warning",
			"Do not use node.set. Use node.normal instead.",
			false,
		),
		exitCode: 1, // cookstyle exits non-zero with offenses
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	sr := s.scanOneNoDB(context.Background(), "old-cb", "2.0.0", "", "/cookbooks/old")

	if !sr.Passed {
		t.Error("expected Passed=true — warnings don't cause failure")
	}
	if sr.Error != nil {
		t.Errorf("expected nil error, got: %v", sr.Error)
	}
	if sr.OffenseCount != 1 {
		t.Errorf("expected 1 offense, got %d", sr.OffenseCount)
	}
	if sr.DeprecationCount != 1 {
		t.Errorf("expected 1 deprecation, got %d", sr.DeprecationCount)
	}
	if len(sr.DeprecationWarnings) != 1 {
		t.Errorf("expected 1 deprecation warning, got %d", len(sr.DeprecationWarnings))
	}
	if sr.CorrectnessCount != 0 {
		t.Errorf("expected 0 correctness, got %d", sr.CorrectnessCount)
	}
}

func TestScanOneNoDB_ErrorSeverityFails(t *testing.T) {
	offenses := []CookstyleOffense{
		{Severity: "error", CopName: "Chef/Correctness/InvalidDefaultAction", Message: "bad action"},
	}
	fe := &fakeCookstyleExecutor{
		stdout:   makeMultiOffenseJSON(offenses),
		exitCode: 1,
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	sr := s.scanOneNoDB(context.Background(), "bad-cb", "1.0.0", "", "/cookbooks/bad")

	if sr.Passed {
		t.Error("expected Passed=false for error-severity offense")
	}
	if sr.Error != nil {
		t.Errorf("expected nil error (non-zero exit with valid JSON is normal), got: %v", sr.Error)
	}
	if sr.CorrectnessCount != 1 {
		t.Errorf("expected 1 correctness, got %d", sr.CorrectnessCount)
	}
}

func TestScanOneNoDB_MixedOffenses(t *testing.T) {
	offenses := []CookstyleOffense{
		{Severity: "warning", CopName: "Chef/Deprecations/NodeSet", Message: "dep", Corrected: true},
		{Severity: "error", CopName: "Chef/Correctness/InvalidDefaultAction", Message: "err"},
		{Severity: "convention", CopName: "Chef/Style/FileMode", Message: "style", Corrected: true},
		{Severity: "warning", CopName: "Chef/Modernize/FoodcriticComments", Message: "modernize"},
	}
	fe := &fakeCookstyleExecutor{
		stdout:   makeMultiOffenseJSON(offenses),
		exitCode: 1,
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	sr := s.scanOneNoDB(context.Background(), "mixed-cb", "3.0.0", "", "/cookbooks/mixed")

	if sr.Passed {
		t.Error("expected Passed=false due to error-severity offense")
	}
	if sr.OffenseCount != 4 {
		t.Errorf("expected 4 offenses, got %d", sr.OffenseCount)
	}
	if sr.DeprecationCount != 1 {
		t.Errorf("expected 1 deprecation, got %d", sr.DeprecationCount)
	}
	if sr.CorrectnessCount != 1 {
		t.Errorf("expected 1 correctness, got %d", sr.CorrectnessCount)
	}
	if sr.CorrectableCount != 2 {
		t.Errorf("expected 2 correctable, got %d", sr.CorrectableCount)
	}
	if len(sr.Offenses) != 4 {
		t.Errorf("expected 4 offenses in list, got %d", len(sr.Offenses))
	}
	if len(sr.DeprecationWarnings) != 1 {
		t.Errorf("expected 1 deprecation warning, got %d", len(sr.DeprecationWarnings))
	}
}

func TestScanOneNoDB_WithTargetVersion(t *testing.T) {
	fe := &fakeCookstyleExecutor{
		stdout: makeCleanJSON(),
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	sr := s.scanOneNoDB(context.Background(), "cb", "1.0.0", "18.0", "/cookbooks/cb")

	if sr.TargetChefVersion != "18.0" {
		t.Errorf("expected TargetChefVersion=18.0, got %q", sr.TargetChefVersion)
	}
}

// ---------------------------------------------------------------------------
// Error path tests
// ---------------------------------------------------------------------------

func TestScanOneNoDB_InvalidJSON(t *testing.T) {
	fe := &fakeCookstyleExecutor{
		stdout:   "this is not json",
		exitCode: 0,
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	sr := s.scanOneNoDB(context.Background(), "bad-json-cb", "1.0.0", "", "/cookbooks/bad")

	if sr.Error == nil {
		t.Error("expected error for invalid JSON output")
	}
	if !strings.Contains(sr.Error.Error(), "invalid JSON") {
		t.Errorf("error should mention invalid JSON, got: %v", sr.Error)
	}
}

func TestScanOneNoDB_NonZeroExitInvalidJSON(t *testing.T) {
	fe := &fakeCookstyleExecutor{
		stdout:   "crash trace here",
		stderr:   "FATAL: something broke",
		exitCode: 2,
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	sr := s.scanOneNoDB(context.Background(), "crash-cb", "1.0.0", "", "/cookbooks/crash")

	if sr.Error == nil {
		t.Error("expected error for non-zero exit with invalid JSON")
	}
	if !strings.Contains(sr.Error.Error(), "exit") {
		t.Errorf("error should mention exit code, got: %v", sr.Error)
	}
}

func TestScanOneNoDB_ExecError_NoOutput(t *testing.T) {
	fe := &fakeCookstyleExecutor{
		stdout:   "",
		stderr:   "command not found",
		exitCode: 127,
		err:      fmt.Errorf("exec: cookstyle: not found"),
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	sr := s.scanOneNoDB(context.Background(), "missing-cb", "1.0.0", "", "/cookbooks/missing")

	if sr.Error == nil {
		t.Error("expected error when execution fails with no output")
	}
	if !strings.Contains(sr.Error.Error(), "execution failed") {
		t.Errorf("error should mention execution failure, got: %v", sr.Error)
	}
	if sr.RawStderr != "command not found" {
		t.Errorf("expected stderr to be captured, got %q", sr.RawStderr)
	}
}

func TestScanOneNoDB_ExecError_WithValidJSON(t *testing.T) {
	// Some cookstyle runs exit non-zero but produce valid JSON (offenses found).
	// The fake executor returns both an error AND valid JSON stdout to test
	// the fallthrough path.
	fe := &fakeCookstyleExecutor{
		stdout: makeOffenseJSON(
			"Chef/Deprecations/NodeSet",
			"warning",
			"deprecated",
			false,
		),
		stderr:   "",
		exitCode: 1,
		err:      fmt.Errorf("exit status 1"),
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	sr := s.scanOneNoDB(context.Background(), "exit1-cb", "1.0.0", "", "/cookbooks/exit1")

	// Should fall through to parse JSON successfully.
	if sr.Error != nil {
		t.Errorf("expected nil error (valid JSON despite exec error), got: %v", sr.Error)
	}
	if sr.OffenseCount != 1 {
		t.Errorf("expected 1 offense, got %d", sr.OffenseCount)
	}
}

func TestScanOneNoDB_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	fe := &fakeCookstyleExecutor{
		err: context.Canceled,
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	sr := s.scanOneNoDB(ctx, "cancel-cb", "1.0.0", "", "/cookbooks/cancel")

	// The scan should have an error (either from context or execution).
	// The important thing is it doesn't panic.
	if sr.CookbookName != "cancel-cb" {
		t.Errorf("expected CookbookName=cancel-cb, got %q", sr.CookbookName)
	}
}

// ---------------------------------------------------------------------------
// Duration and timestamp tests
// ---------------------------------------------------------------------------

func TestScanOneNoDB_SetsTimestamps(t *testing.T) {
	fe := &fakeCookstyleExecutor{
		stdout: makeCleanJSON(),
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	before := time.Now()
	sr := s.scanOneNoDB(context.Background(), "ts-cb", "1.0.0", "", "/cookbooks/ts")
	after := time.Now()

	if sr.ScannedAt.Before(before) || sr.ScannedAt.After(after.Add(time.Second)) {
		t.Errorf("ScannedAt %v not in expected range [%v, %v]", sr.ScannedAt, before, after)
	}
	if sr.Duration < 0 {
		t.Errorf("Duration should be non-negative, got %v", sr.Duration)
	}
}

// ---------------------------------------------------------------------------
// Raw output capture tests
// ---------------------------------------------------------------------------

func TestScanOneNoDB_CapturesRawOutput(t *testing.T) {
	jsonOut := makeCleanJSON()
	fe := &fakeCookstyleExecutor{
		stdout: jsonOut,
		stderr: "some warning on stderr",
	}
	s := NewCookstyleScanner(nil, nil, "/usr/bin/cookstyle", 1, 10,
		WithCookstyleExecutor(fe))

	sr := s.scanOneNoDB(context.Background(), "raw-cb", "1.0.0", "", "/cookbooks/raw")

	if sr.RawStdout != jsonOut {
		t.Errorf("expected RawStdout to contain cookstyle JSON output")
	}
	if sr.RawStderr != "some warning on stderr" {
		t.Errorf("expected RawStderr to be captured, got %q", sr.RawStderr)
	}
}

// ---------------------------------------------------------------------------
// scanOneNoDB is a test helper that runs scanOne logic without needing
// a datastore (skips the immutability check and persistence).
// ---------------------------------------------------------------------------

func (s *CookstyleScanner) scanOneNoDB(
	ctx context.Context,
	cookbookName, cookbookVersion, targetChefVersion, cookbookDir string,
) CookstyleScanResult {
	sr := CookstyleScanResult{
		CookbookName:      cookbookName,
		CookbookVersion:   cookbookVersion,
		TargetChefVersion: targetChefVersion,
	}

	args := buildCookstyleArgs(cookbookDir, targetChefVersion)

	scanStart := time.Now()
	scanCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	stdout, stderr, exitCode, execErr := s.executor.Run(scanCtx, args...)
	sr.Duration = time.Since(scanStart)
	sr.ScannedAt = time.Now().UTC()
	sr.RawStdout = stdout
	sr.RawStderr = stderr

	if execErr != nil {
		if scanCtx.Err() == context.DeadlineExceeded {
			sr.Error = fmt.Errorf("timed out after %s", s.timeout)
			return sr
		}
		if stdout == "" {
			sr.Error = fmt.Errorf("execution failed (exit %d): %v; stderr: %s",
				exitCode, execErr, strings.TrimSpace(stderr))
			return sr
		}
		// Fall through — non-zero exit with stdout present.
	}

	var output CookstyleOutput
	if parseErr := json.Unmarshal([]byte(stdout), &output); parseErr != nil {
		if exitCode != 0 {
			sr.Error = fmt.Errorf("exit %d with invalid JSON: %v; stderr: %s",
				exitCode, parseErr, strings.TrimSpace(stderr))
		} else {
			sr.Error = fmt.Errorf("invalid JSON output: %v", parseErr)
		}
		return sr
	}

	sr.OffenseCount = output.Summary.OffenseCount
	sr.Passed = true

	for _, file := range output.Files {
		for _, off := range file.Offenses {
			sr.Offenses = append(sr.Offenses, off)
			if isDeprecation(off.CopName) {
				sr.DeprecationCount++
				sr.DeprecationWarnings = append(sr.DeprecationWarnings, off)
			}
			if isCorrectness(off.CopName) {
				sr.CorrectnessCount++
			}
			if off.Corrected {
				sr.CorrectableCount++
			}
			if isErrorOrFatal(off.Severity) {
				sr.Passed = false
			}
		}
	}

	return sr
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

func assertContains(t *testing.T, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("expected %v to contain %q", slice, want)
}
