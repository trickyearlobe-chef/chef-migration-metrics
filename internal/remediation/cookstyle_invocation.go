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
//   - addonCops are resolved operator addon cops (see ResolveAddonCopFiles).
//     Each file is require:'d into the sidecar so cookstyle loads it, and each
//     parsed cop name is enabled explicitly (a required custom cop is otherwise
//     registered but does not run). When addons are present the sidecar is
//     written even with no target version, so --config is emitted in that case.
//
// The cookbook directory is always the final argument.
func BuildCookstyleArgs(cookbookDir, targetChefVersion string, leadingArgs []string, onlyDepartments string, addonCops []AddonCop) []string {
	args := append([]string{}, leadingArgs...)

	if configPath := WriteCookstyleTargetConfig(cookbookDir, targetChefVersion, addonCops); configPath != "" {
		args = append(args, "--config", configPath)
	}
	if targetChefVersion != "" && onlyDepartments != "" {
		args = append(args, "--only", onlyDepartments)
	}

	args = append(args, cookbookDir)
	return args
}

// WriteCookstyleTargetConfig writes a self-contained sidecar .rubocop_cmm.yml
// into the cookbook directory that sets AllCops.TargetChefVersion, requires
// cookstyle, and require:s any addon cops.
//
// The sidecar deliberately does NOT inherit the cookbook's own .rubocop.yml /
// .rubocop_todo.yml. Those files are the team's style configuration and a
// deferred-violations TODO — irrelevant to a migration-readiness scan (honouring
// the TODO would hide the very offences we assess), and frequently referencing
// obsolete/renamed cops (e.g. `Metrics/LineLength` → `Layout/LineLength`) that
// make CookStyle abort with exit 2 ("obsolete configuration found"). Assessing
// every cookbook against a consistent, tool-controlled ruleset for the target
// Chef version is both correct and immune to those stale configs.
//
// addonCops are resolved operator addon cops; each file is added to the
// sidecar's require: list and each parsed cop name is enabled with an explicit
// `<CopName>: { Enabled: true }` entry so the (otherwise registered-but-dormant)
// custom cops actually run.
//
// The sidecar is written when there is anything to configure — a target version
// OR at least one addon cop. With neither, nothing is written (returns "").
//
// Returns the absolute path to the written file, or "" on failure / nothing to do.
func WriteCookstyleTargetConfig(cookbookDir, targetChefVersion string, addonCops []AddonCop) string {
	if targetChefVersion == "" && len(addonCops) == 0 {
		return ""
	}

	var buf strings.Builder

	// Always require cookstyle ourselves so the AllCops.TargetChefVersion
	// parameter and the Chef cops are registered. We never inherit the cookbook's
	// own config (see the doc comment above), so this is unconditional.
	requires := []string{"cookstyle"}
	for _, c := range addonCops {
		requires = append(requires, c.Path)
	}

	if len(requires) > 0 {
		buf.WriteString("require:\n")
		for _, r := range requires {
			fmt.Fprintf(&buf, "  - %s\n", r)
		}
		buf.WriteString("\n")
	}

	if targetChefVersion != "" {
		fmt.Fprintf(&buf, "AllCops:\n  TargetChefVersion: %s\n\n", targetChefVersion)
	}

	// Enable each addon cop explicitly. A required custom cop is registered but
	// dormant until enabled here — neither AllCops.NewCops nor
	// --enable-pending-cops turns required custom cops on.
	for _, c := range addonCops {
		for _, name := range c.CopNames {
			fmt.Fprintf(&buf, "%s:\n  Enabled: true\n", name)
		}
	}

	outPath := filepath.Join(cookbookDir, CmmConfigName)
	if err := os.WriteFile(outPath, []byte(buf.String()), 0644); err != nil {
		return ""
	}
	return outPath
}
