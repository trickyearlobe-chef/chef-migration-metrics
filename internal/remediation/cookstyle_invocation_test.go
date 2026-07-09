// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readSidecar reads the .rubocop_cmm.yml written into dir, failing the test if
// it is absent.
func readSidecar(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, CmmConfigName))
	if err != nil {
		t.Fatalf("reading sidecar: %v", err)
	}
	return string(data)
}

// TestWriteCookstyleTargetConfig_InjectsAddonRequires proves each resolved
// addon .rb is require:'d in the sidecar alongside cookstyle, and each cop name
// is enabled explicitly (a required custom cop is otherwise dormant).
func TestWriteCookstyleTargetConfig_InjectsAddonRequires(t *testing.T) {
	dir := t.TempDir()
	addons := []AddonCop{
		{Path: "/opt/cops/no_eval.rb", CopNames: []string{"Cmm/NoEval"}},
		{Path: "/opt/cops/no_legacy.rb", CopNames: []string{"Cmm/NoLegacy"}},
	}

	path := WriteCookstyleTargetConfig(dir, "18.0", addons)
	if path == "" {
		t.Fatal("expected sidecar to be written")
	}
	got := readSidecar(t, dir)

	for _, a := range addons {
		if !strings.Contains(got, "- "+a.Path) {
			t.Errorf("sidecar missing require for addon %q:\n%s", a.Path, got)
		}
	}
	if !strings.Contains(got, "- cookstyle") {
		t.Errorf("sidecar must still require cookstyle:\n%s", got)
	}
	if !strings.Contains(got, "Cmm/NoEval:\n  Enabled: true") {
		t.Errorf("sidecar must enable each addon cop explicitly:\n%s", got)
	}
	if !strings.Contains(got, "Cmm/NoLegacy:\n  Enabled: true") {
		t.Errorf("sidecar must enable each addon cop explicitly:\n%s", got)
	}
	if !strings.Contains(got, "TargetChefVersion: 18.0") {
		t.Errorf("sidecar must keep TargetChefVersion:\n%s", got)
	}
}

// TestWriteCookstyleTargetConfig_AddonsWithoutTargetVersion proves the sidecar
// is written (so addons load) even when no target version is set.
func TestWriteCookstyleTargetConfig_AddonsWithoutTargetVersion(t *testing.T) {
	dir := t.TempDir()
	path := WriteCookstyleTargetConfig(dir, "", []AddonCop{{Path: "/opt/cops/x.rb", CopNames: []string{"Cmm/X"}}})
	if path == "" {
		t.Fatal("expected sidecar to be written for addons even with no target version")
	}
	got := readSidecar(t, dir)
	if !strings.Contains(got, "- /opt/cops/x.rb") {
		t.Errorf("sidecar must require the addon:\n%s", got)
	}
	if strings.Contains(got, "TargetChefVersion") {
		t.Errorf("no target version was set; TargetChefVersion must not appear:\n%s", got)
	}
}

// TestWriteCookstyleTargetConfig_NoSidecarWithoutTargetOrAddons keeps the
// pre-existing behaviour: with neither a target version nor addons, no sidecar.
func TestWriteCookstyleTargetConfig_NoSidecarWithoutTargetOrAddons(t *testing.T) {
	dir := t.TempDir()
	path := WriteCookstyleTargetConfig(dir, "", nil)
	if path != "" {
		t.Errorf("no sidecar expected when there is nothing to configure, got %q", path)
	}
	if _, err := os.Stat(filepath.Join(dir, CmmConfigName)); err == nil {
		t.Errorf("sidecar file must not be created")
	}
}

// TestWriteCookstyleTargetConfig_IgnoresCookbookConfig proves the sidecar is
// self-contained even when the cookbook ships its own .rubocop.yml: it requires
// cookstyle itself and does NOT inherit the cookbook config (which would drag in
// an obsolete .rubocop_todo.yml and make CookStyle abort with exit 2). Addons
// are still require:'d and enabled.
func TestWriteCookstyleTargetConfig_IgnoresCookbookConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".rubocop.yml"), []byte("inherit_from: .rubocop_todo.yml\nrequire:\n  - cookstyle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	WriteCookstyleTargetConfig(dir, "18.0", []AddonCop{{Path: "/opt/cops/x.rb", CopNames: []string{"Cmm/X"}}})
	got := readSidecar(t, dir)
	if strings.Contains(got, "inherit_from") {
		t.Errorf("must NOT inherit the cookbook config:\n%s", got)
	}
	if !strings.Contains(got, "require:") || !strings.Contains(got, "- cookstyle") {
		t.Errorf("must require cookstyle itself:\n%s", got)
	}
	if !strings.Contains(got, "- /opt/cops/x.rb") {
		t.Errorf("must require the addon:\n%s", got)
	}
	if !strings.Contains(got, "Cmm/X:\n  Enabled: true") {
		t.Errorf("must enable the addon cop explicitly:\n%s", got)
	}
}

// TestBuildCookstyleArgs_AddonSidecarConfig proves BuildCookstyleArgs points
// cookstyle at the sidecar via --config when addons are present even with no
// target version.
func TestBuildCookstyleArgs_AddonSidecarConfig(t *testing.T) {
	dir := t.TempDir()
	args := BuildCookstyleArgs(dir, "", []string{"--format", "json"}, "", []AddonCop{{Path: filepath.Join(dir, "cop.rb"), CopNames: []string{"Cmm/Cop"}}})

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--config") {
		t.Errorf("expected --config pointing at the addon sidecar, got %v", args)
	}
	if strings.Contains(joined, "--only") {
		t.Errorf("addon scan must not narrow with --only, got %v", args)
	}
	if args[len(args)-1] != dir {
		t.Errorf("cookbook dir must be last, got %v", args)
	}
}
