// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package nodekitchen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Which kitchen runs is decided when a run starts, not when the executor is
// built — the same reason as cookstyle. Where the Chef tools are is a setting
// on a screen, and the path used to be resolved once at startup.

func kitchenPrinting(t *testing.T, word string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "kitchen")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho "+word+"\n"), 0o755); err != nil {
		t.Fatalf("writing a stand-in kitchen: %v", err)
	}
	return script
}

func TestDefaultExecutor_ResolvesTheBinaryWhenItRuns(t *testing.T) {
	first := kitchenPrinting(t, "first-kitchen")
	second := kitchenPrinting(t, "second-kitchen")

	current := first
	e := &DefaultExecutor{PathFn: func() (string, error) { return current, nil }}

	// The baseline: it runs what it is pointed at, so the change below is
	// telling us about staleness rather than about nothing running.
	stdout, _, _, err := e.Run(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("running the first binary: %v", err)
	}
	if !strings.Contains(stdout, "first-kitchen") {
		t.Fatalf("did not run the binary it was pointed at (%q)", stdout)
	}

	current = second

	stdout, _, _, err = e.Run(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("running after the path changed: %v", err)
	}
	if !strings.Contains(stdout, "second-kitchen") {
		t.Errorf("still ran the old binary (%q) — correcting where the Chef tools are did "+
			"nothing until the service was restarted", stdout)
	}
}

// The fixed Path still works: callers holding a resolved path, and tests.
func TestDefaultExecutor_AFixedPathStillRuns(t *testing.T) {
	e := &DefaultExecutor{Path: kitchenPrinting(t, "fixed-kitchen")}

	stdout, _, _, err := e.Run(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("the fixed-path executor stopped working: %v", err)
	}
	if !strings.Contains(stdout, "fixed-kitchen") {
		t.Errorf("ran something else: %q", stdout)
	}
}
