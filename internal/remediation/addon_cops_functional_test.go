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

// TestFunctional_AddonCop_LoadedEnabledAndDetected is the end-to-end proof that
// a real operator addon RuboCop cop placed on disk is resolved,
// require:'d, ENABLED, and actually produces an offence when the live cookstyle
// binary scans a cookbook that triggers it — keyed by its cop name like any
// other offence (so classification can later block it).
//
// It drives the same resolution + sidecar/args builder the scanner uses, then
// runs the real cookstyle binary; no datastore is involved.
func TestFunctional_AddonCop_LoadedEnabledAndDetected(t *testing.T) {
	cookstylePath, err := exec.LookPath("cookstyle")
	if err != nil {
		t.Skip("cookstyle binary not found on PATH — skipping functional addon cop test")
	}

	// Operator addon cop: flags `=~` regex matching (the spec's example).
	addonDir := t.TempDir()
	cop := `require 'rubocop'

module RuboCop
  module Cop
    module Cmm
      class NoNodeRegexMatch < RuboCop::Cop::Base
        MSG = 'Avoid =~ regex matching; use a clearer comparison.'

        def on_send(node)
          return unless node.method_name == :=~
          add_offense(node)
        end
      end
    end
  end
end
`
	if err := os.WriteFile(filepath.Join(addonDir, "no_node_regex_match.rb"), []byte(cop), 0o644); err != nil {
		t.Fatalf("write addon cop: %v", err)
	}

	// Cookbook that triggers the addon cop.
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "recipes"), 0o755); err != nil {
		t.Fatalf("mkdir recipes: %v", err)
	}
	recipe := "if node['platform'] =~ /ubuntu/\n  puts 'match'\nend\n"
	if err := os.WriteFile(filepath.Join(repoDir, "recipes", "default.rb"), []byte(recipe), 0o644); err != nil {
		t.Fatalf("write recipe: %v", err)
	}

	// Resolve the addon (directory form) and build the scan invocation exactly
	// as the scanner does.
	cops, problems := remediation.ResolveAddonCopFiles([]string{addonDir})
	if len(problems) != 0 {
		t.Fatalf("addon resolution problems: %v", problems)
	}
	foundName := false
	for _, c := range cops {
		for _, n := range c.CopNames {
			if n == "Cmm/NoNodeRegexMatch" {
				foundName = true
			}
		}
	}
	if !foundName {
		t.Fatalf("expected to parse cop name Cmm/NoNodeRegexMatch, got %+v", cops)
	}

	args := remediation.BuildCookstyleArgs(repoDir, "18.0", []string{"--format", "json"}, "", cops)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cookstylePath, args...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	_ = cmd.Run() // non-zero exit when offences are found is expected

	var parsed struct {
		Files []struct {
			Offenses []struct {
				CopName string `json:"cop_name"`
			} `json:"offenses"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(out.String()), &parsed); err != nil {
		t.Fatalf("parse cookstyle JSON: %v\nstdout: %s\nstderr: %s", err, out.String(), errb.String())
	}

	found := false
	for _, f := range parsed.Files {
		for _, o := range f.Offenses {
			if o.CopName == "Cmm/NoNodeRegexMatch" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("addon cop offence Cmm/NoNodeRegexMatch not produced by the live scan\nstdout: %s\nstderr: %s", out.String(), errb.String())
	}
}
