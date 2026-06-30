// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This file is the single source of truth for building a cookstyle invocation
// (CLI args + sidecar .rubocop_cmm.yml). Both the scan path
// (internal/analysis) and the autocorrect-preview path (autocorrect.go) build
// on it so the two can never drift. analysis already imports remediation, so
// the shared helper lives here to avoid an import cycle.

// CmmConfigName is the sidecar config file written next to the cookbook's own
// .rubocop.yml (if any). A distinct name avoids overwriting the cookbook's
// configuration.
const CmmConfigName = ".rubocop_cmm.yml"

// BuildCookstyleArgs constructs the cookstyle CLI arguments shared by the scan
// and the autocorrect preview.
//
//   - leadingArgs are the mode-specific flags (e.g. {"--format","json"} for a
//     scan; {"--auto-correct","--format","json"} for an autocorrect run).
//   - When targetChefVersion is non-empty, a sidecar .rubocop_cmm.yml setting
//     AllCops.TargetChefVersion is written into cookbookDir and pointed at via
//     --config. CookStyle has no --target-chef-version CLI flag.
//   - onlyDepartments, when non-empty, restricts the run to those departments
//     via --only. The scan passes "" (full ruleset) so classification — not a
//     department filter — decides the verdict; a Blocker-classified cop outside
//     Chef/Deprecations,Chef/Correctness is no longer silently hidden.
//
// The cookbook directory is always the final argument.
func BuildCookstyleArgs(cookbookDir, targetChefVersion string, leadingArgs []string, onlyDepartments string) []string {
	args := append([]string{}, leadingArgs...)

	if targetChefVersion != "" {
		if configPath := WriteCookstyleTargetConfig(cookbookDir, targetChefVersion); configPath != "" {
			args = append(args, "--config", configPath)
		}
		if onlyDepartments != "" {
			args = append(args, "--only", onlyDepartments)
		}
	}

	args = append(args, cookbookDir)
	return args
}

// WriteCookstyleTargetConfig writes a sidecar .rubocop_cmm.yml into the cookbook
// directory that sets AllCops.TargetChefVersion. If the cookbook already
// contains a .rubocop.yml the sidecar inherits from it so the cookbook's own
// configuration (excludes, custom cops, etc.) is preserved. When no cookbook
// config exists the sidecar explicitly requires cookstyle so that the
// TargetChefVersion parameter is recognised.
//
// Returns the absolute path to the written file, or "" on failure.
func WriteCookstyleTargetConfig(cookbookDir, targetChefVersion string) string {
	var buf strings.Builder

	existingConfig := filepath.Join(cookbookDir, ".rubocop.yml")
	if _, err := os.Stat(existingConfig); err == nil {
		// Cookbook has its own config — inherit from it (which also picks up
		// any `require: cookstyle` it contains).
		buf.WriteString("inherit_from: .rubocop.yml\n\n")
	} else {
		// No cookbook config — require cookstyle ourselves so the
		// TargetChefVersion AllCops parameter is registered.
		buf.WriteString("require:\n  - cookstyle\n\n")
	}

	fmt.Fprintf(&buf, "AllCops:\n  TargetChefVersion: %s\n", targetChefVersion)

	outPath := filepath.Join(cookbookDir, CmmConfigName)
	if err := os.WriteFile(outPath, []byte(buf.String()), 0644); err != nil {
		return ""
	}
	return outPath
}
