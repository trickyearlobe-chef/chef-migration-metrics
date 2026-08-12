// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Which cookstyle runs is decided when a scan runs, not when the scanner is
// built.
//
// Where the Chef tools are is a setting an operator changes on a screen. The
// resolved path used to be worked out at startup and handed here as a string,
// so correcting the setting changed nothing until somebody restarted the
// service — and the screen reported the save as applied.

// toolPrinting builds a directory holding an executable of the given name that
// prints the given word, so which one ran can be told from its output.
func toolPrinting(t *testing.T, name, word string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, name)
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho "+word+"\n"), 0o755); err != nil {
		t.Fatalf("writing a stand-in %s: %v", name, err)
	}
	return dir
}

func TestCookstyleExecutor_ResolvesTheBinaryWhenItRuns(t *testing.T) {
	first := filepath.Join(toolPrinting(t, "cookstyle", "first-binary"), "cookstyle")
	second := filepath.Join(toolPrinting(t, "cookstyle", "second-binary"), "cookstyle")

	current := first
	exec := NewCookstyleExecutorFunc(func() (string, error) { return current, nil })

	// The baseline: it runs the binary it is pointed at. Without this, an
	// executor that ran nothing at all would pass the change below by way of
	// both outputs being equally empty.
	stdout, _, _, err := exec.Run(context.Background(), "")
	if err != nil {
		t.Fatalf("running the first binary: %v", err)
	}
	if !strings.Contains(stdout, "first-binary") {
		t.Fatalf("the executor did not run the binary it was pointed at (%q), so this test "+
			"cannot tell a live path from a stale one", stdout)
	}

	current = second

	stdout, _, _, err = exec.Run(context.Background(), "")
	if err != nil {
		t.Fatalf("running after the path changed: %v", err)
	}
	if !strings.Contains(stdout, "second-binary") {
		t.Errorf("still ran the old binary (%q) — an operator who corrected where the Chef "+
			"tools are was told it was saved, and every scan went on running the one from the "+
			"place they had just changed", stdout)
	}
}

// A path that cannot be resolved is a failure of that run, not of the process.
// Scanning must not be wedged by a directory that was wrong for a while.
func TestCookstyleExecutor_AnUnresolvablePathFailsOnlyThatRun(t *testing.T) {
	dir := toolPrinting(t, "cookstyle", "it-works")
	good := filepath.Join(dir, "cookstyle")

	broken := true
	exec := NewCookstyleExecutorFunc(func() (string, error) {
		if broken {
			return "", os.ErrNotExist
		}
		return good, nil
	})

	if _, _, _, err := exec.Run(context.Background(), ""); err == nil {
		t.Fatal("a path that could not be resolved reported success")
	}

	broken = false

	stdout, _, _, err := exec.Run(context.Background(), "")
	if err != nil {
		t.Fatalf("the next run failed too, so one bad setting stopped scanning until a "+
			"restart: %v", err)
	}
	if !strings.Contains(stdout, "it-works") {
		t.Errorf("recovered but ran the wrong thing: %q", stdout)
	}
}

// The static constructor still works. Tests and any caller that already holds a
// resolved path rely on it, and the accessor is an addition.
func TestCookstyleExecutor_AFixedPathStillRuns(t *testing.T) {
	path := filepath.Join(toolPrinting(t, "cookstyle", "fixed-path"), "cookstyle")

	stdout, _, _, err := NewCookstyleExecutor(path).Run(context.Background(), "")
	if err != nil {
		t.Fatalf("the fixed-path executor stopped working: %v", err)
	}
	if !strings.Contains(stdout, "fixed-path") {
		t.Errorf("ran something else: %q", stdout)
	}
}
