// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package remediation_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// TestFunctional_ScanIgnoresObsoleteRubocopTodo is the end-to-end proof that our
// scan invocation does NOT inherit a cookbook's own RuboCop config — neither
// .rubocop.yml nor the .rubocop_todo.yml it chains to. Git-sourced cookbooks
// routinely ship a .rubocop_todo.yml full of renamed/obsolete cops (e.g.
// Metrics/LineLength → Layout/LineLength), which makes CookStyle abort with
// exit 2 ("obsolete configuration found") if loaded. The self-contained sidecar
// must sidestep that so the cookbook is still assessed.
func TestFunctional_ScanIgnoresObsoleteRubocopTodo(t *testing.T) {
	cookstylePath, err := exec.LookPath("cookstyle")
	if err != nil {
		t.Skip("cookstyle binary not found on PATH — skipping")
	}

	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "recipes"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "recipes", "default.rb"),
		[]byte("package 'nginx'\n"), 0o644); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
	// The cookbook's own config chains to a TODO that references an obsolete cop.
	if err := os.WriteFile(filepath.Join(repoDir, ".rubocop.yml"),
		[]byte("inherit_from: .rubocop_todo.yml\n\nrequire:\n  - cookstyle\n"), 0o644); err != nil {
		t.Fatalf("write .rubocop.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".rubocop_todo.yml"),
		[]byte("# Metrics/LineLength was moved to Layout/LineLength.\nMetrics/LineLength:\n  Max: 200\n"), 0o644); err != nil {
		t.Fatalf("write .rubocop_todo.yml: %v", err)
	}

	run := func(args []string) (exitCode int, combined string) {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, cookstylePath, args...)
		var out, errb strings.Builder
		cmd.Stdout = &out
		cmd.Stderr = &errb
		runErr := cmd.Run()
		exitCode = 0
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
		return exitCode, out.String() + errb.String()
	}

	// Baseline: inheriting the cookbook's own config (as the old code did) must
	// abort with exit 2. If this env's cookstyle doesn't consider the cop obsolete
	// (version drift), the scenario doesn't apply — skip.
	baselineExit, baselineOut := run([]string{
		"--config", filepath.Join(repoDir, ".rubocop.yml"), "--format", "json", repoDir,
	})
	if baselineExit != 2 || !strings.Contains(baselineOut, "obsolete") {
		t.Skipf("cookstyle here does not reject this config as obsolete (exit %d) — scenario N/A", baselineExit)
	}

	// The fix: our self-contained sidecar must NOT abort — it scans through.
	args := remediation.BuildCookstyleArgs(repoDir, "18.0", []string{"--format", "json"}, "", nil)
	exitCode, combined := run(args)

	if exitCode == 2 || strings.Contains(combined, "obsolete configuration") {
		t.Fatalf("scan aborted on the cookbook's obsolete .rubocop_todo.yml (exit %d):\n%s", exitCode, combined)
	}
	// A real scan happened → valid JSON on stdout (exit 0 clean or 1 with offences).
	var parsed struct {
		Summary struct {
			TargetFileCount int `json:"target_file_count"`
		} `json:"summary"`
	}
	if jerr := json.Unmarshal([]byte(combined), &parsed); jerr != nil {
		t.Fatalf("expected JSON scan output, got (exit %d):\n%s", exitCode, combined)
	}
}
