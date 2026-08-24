// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package embedded

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Where the Chef tools are, when they are not where a shell would find them.
//
// Chef Workstation normally puts cookstyle and kitchen on PATH. Where a
// deployment does not — a service started without the profile that sets it, an
// installation somewhere else — the operator says where they are, and this is
// what reads that. Without it the setting existed on the settings screen and
// reached nothing: an operator typed a path in, saved, was told it worked, and
// scanning stayed switched off.

// binDirWithTool builds a directory holding one executable of the given name.
func binDirWithTool(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, name)
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho from-the-bin-dir\n"), 0o755); err != nil {
		t.Fatalf("writing a stand-in %s: %v", name, err)
	}
	return dir
}

// The baseline: with nothing configured, a tool is found the way it always was.
// Without this the test below would pass against a resolver that had stopped
// looking at PATH altogether.
func TestBinDir_WithNoneSetTheToolIsStillFoundOnPATH(t *testing.T) {
	path, err := NewResolver().ResolvePath("go")
	if err != nil {
		t.Fatalf("a tool on PATH was not found with no bin dir set: %v", err)
	}
	if path == "" {
		t.Error("a tool on PATH resolved to nothing")
	}
}

// The point of the setting: the named directory is looked in first, so a
// deployment whose PATH does not carry the Chef tools can still say where they
// are.
func TestBinDir_TheNamedDirectoryIsPreferredToPATH(t *testing.T) {
	// "go" is certainly on PATH, so a resolver that ignored the bin dir would
	// still answer — with the wrong one. That is what makes this a test of
	// preference rather than of finding anything at all.
	dir := binDirWithTool(t, "go")

	path, err := NewResolver(WithBinDir(dir)).ResolvePath("go")
	if err != nil {
		t.Fatalf("a tool in the configured directory was not found: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("resolved %q, which is not in the configured directory %q — an operator who "+
			"pointed at their own Chef tools got whichever ones the PATH had", path, dir)
	}
}

// The directory is read when a tool is resolved, not when the resolver is
// built.
//
// The directory comes from the database and an operator changes it on a screen,
// so a resolver that answers with the directory it was built with sends every
// scan to the old place until somebody restarts the service.
func TestBinDir_TheDirectoryIsReadAtResolveTimeNotAtBuildTime(t *testing.T) {
	first := binDirWithTool(t, "go")
	second := binDirWithTool(t, "go")

	current := first
	r := NewResolver(WithBinDirFunc(func() string { return current }))

	// The baseline: it honours the directory at all. Without this, a resolver
	// that had stopped reading the accessor entirely would still pass the
	// change below by way of always answering from PATH.
	path, err := r.ResolvePath("go")
	if err != nil {
		t.Fatalf("a tool in the configured directory was not found: %v", err)
	}
	if filepath.Dir(path) != first {
		t.Fatalf("resolved %q, not the configured directory %q — this test cannot tell a live "+
			"read from a stale one until the directory is honoured at all", path, first)
	}

	current = second

	path, err = r.ResolvePath("go")
	if err != nil {
		t.Fatalf("after the directory changed, the tool was not found: %v", err)
	}
	if filepath.Dir(path) != second {
		t.Errorf("resolved %q, still under the old directory %q — an operator who corrected "+
			"where the Chef tools are was told it was saved, and every scan kept running the "+
			"binary from the place they had just changed", path, first)
	}
}

// With no accessor set, the static directory still decides. Tests and defaults
// rely on it, and a live accessor is an addition rather than a replacement.
func TestBinDir_WithoutAnAccessorTheStaticDirectoryStillDecides(t *testing.T) {
	dir := binDirWithTool(t, "go")

	path, err := NewResolver(WithBinDir(dir)).ResolvePath("go")
	if err != nil {
		t.Fatalf("the static directory stopped working: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("resolved %q rather than %q", path, dir)
	}
}

// A directory that does not have the tool is not a dead end. The setting says
// where the tools are when PATH does not have them; where PATH does, it still
// works.
func TestBinDir_ADirectoryWithoutTheToolFallsBackToPATH(t *testing.T) {
	path, err := NewResolver(WithBinDir(t.TempDir())).ResolvePath("go")
	if err != nil {
		t.Fatalf("setting a directory that does not hold the tool stopped it being found "+
			"on PATH, so one wrong setting disables scanning entirely: %v", err)
	}
	if path == "" {
		t.Error("resolved to nothing")
	}
}

// When neither has it, the operator is told both places were looked in.
// Otherwise somebody who mistyped the directory reads "not found in PATH" and
// has no reason to suspect the setting they just changed.
func TestBinDir_WhenNeitherHasItTheRefusalNamesBothPlaces(t *testing.T) {
	dir := t.TempDir()
	_, err := NewResolver(WithBinDir(dir)).ResolvePath("nonexistent-tool-xyz-12345")
	if err == nil {
		t.Fatal("a tool that exists nowhere resolved successfully")
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("the refusal does not name the configured directory: %v — somebody who "+
			"mistyped it is told only that PATH does not have the tool", err)
	}
	if !strings.Contains(err.Error(), "PATH") {
		t.Errorf("the refusal does not say PATH was looked at too: %v", err)
	}
}

// A file that is there but cannot be run is not a resolution. Otherwise a
// directory holding a stray text file called "cookstyle" reports the tool as
// found and every scan fails later, somewhere else.
func TestBinDir_SomethingThatCannotBeRunIsNotTheTool(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte("not a program"), 0o644); err != nil {
		t.Fatalf("writing the file: %v", err)
	}

	path, err := NewResolver(WithBinDir(dir)).ResolvePath("go")
	if err != nil {
		t.Fatalf("an unrunnable file in the directory stopped the real tool being found "+
			"on PATH: %v", err)
	}
	if filepath.Dir(path) == dir {
		t.Errorf("resolved to %q, which is not executable — the tool reads as present and "+
			"every scan fails later, somewhere that does not mention this directory", path)
	}
}
