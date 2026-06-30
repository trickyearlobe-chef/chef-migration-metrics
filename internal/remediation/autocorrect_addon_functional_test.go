// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package remediation_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation"
)

// TestFunctional_AddonCopAutocorrect_AppearsInDiff is the end-to-end proof for
// Chunk E: an operator addon RuboCop cop that defines an AutoCorrector is
// resolved, require:'d and ENABLED into the shared full-ruleset autocorrect
// invocation, and its fix is actually applied by the live cookstyle binary —
// so it would appear in the whole-cookbook diff. Addon cops have no embedded
// remediation mapping, so this diff is their only preview.
//
// It drives the same resolution + sidecar/args builder the autocorrect preview
// uses (BuildCookstyleArgs with --auto-correct), then runs the real cookstyle
// binary; no datastore is involved.
func TestFunctional_AddonCopAutocorrect_AppearsInDiff(t *testing.T) {
	cookstylePath, err := exec.LookPath("cookstyle")
	if err != nil {
		t.Skip("cookstyle binary not found on PATH — skipping functional addon autocorrect test")
	}

	// Operator addon cop with an AutoCorrector: rewrites the string 'oldvalue'
	// to 'newvalue'.
	addonDir := t.TempDir()
	cop := `require 'rubocop'

module RuboCop
  module Cop
    module Cmm
      class ReplaceOldValue < RuboCop::Cop::Base
        extend AutoCorrector
        MSG = 'Use newvalue instead of oldvalue.'

        def on_str(node)
          return unless node.value == 'oldvalue'
          add_offense(node) do |corrector|
            corrector.replace(node, "'newvalue'")
          end
        end
      end
    end
  end
end
`
	if err := os.WriteFile(filepath.Join(addonDir, "replace_old_value.rb"), []byte(cop), 0o644); err != nil {
		t.Fatalf("write addon cop: %v", err)
	}

	// Cookbook that triggers the addon cop's autocorrect.
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "recipes"), 0o755); err != nil {
		t.Fatalf("mkdir recipes: %v", err)
	}
	recipePath := filepath.Join(repoDir, "recipes", "default.rb")
	if err := os.WriteFile(recipePath, []byte("x = 'oldvalue'\n"), 0o644); err != nil {
		t.Fatalf("write recipe: %v", err)
	}

	// Resolve the addon and build the autocorrect invocation exactly as the
	// preview generator does (--auto-correct, full ruleset, no --only).
	cops, problems := remediation.ResolveAddonCopFiles([]string{addonDir})
	if len(problems) != 0 {
		t.Fatalf("addon resolution problems: %v", problems)
	}
	foundName := false
	for _, c := range cops {
		for _, n := range c.CopNames {
			if n == "Cmm/ReplaceOldValue" {
				foundName = true
			}
		}
	}
	if !foundName {
		t.Fatalf("expected to parse cop name Cmm/ReplaceOldValue, got %+v", cops)
	}

	args := remediation.BuildCookstyleArgs(repoDir, "18.0", []string{"--auto-correct", "--format", "json"}, "", cops)
	if strings.Contains(strings.Join(args, " "), "--only") {
		t.Fatalf("autocorrect invocation must be full ruleset (no --only), got: %v", args)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cookstylePath, args...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	_ = cmd.Run() // --auto-correct modifies in place; non-zero exit is expected

	// The addon cop's AutoCorrector must have rewritten the recipe in place —
	// this is exactly the change the preview's unified diff would render.
	modified, readErr := os.ReadFile(recipePath)
	if readErr != nil {
		t.Fatalf("read modified recipe: %v", readErr)
	}
	if !strings.Contains(string(modified), "newvalue") {
		t.Fatalf("addon cop autocorrect was not applied by the live run\nrecipe: %s\nstdout: %s\nstderr: %s",
			string(modified), out.String(), errb.String())
	}
	if strings.Contains(string(modified), "oldvalue") {
		t.Errorf("expected 'oldvalue' to be replaced, got: %s", string(modified))
	}
}
