// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package gitkitchen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

type mockExecutor struct {
	stdout   string
	stderr   string
	exitCode int
	err      error

	// Captured invocation
	dir      string
	extraEnv []string
	args     []string

	// Files captured during Run (before workspace cleanup)
	capturedFiles map[string]string
}

func (m *mockExecutor) Run(_ context.Context, dir string, extraEnv []string, args ...string) (string, string, int, error) {
	m.dir = dir
	m.extraEnv = extraEnv
	m.args = args
	// Capture files from workspace before returning (workspace is cleaned after RunInstance)
	if m.capturedFiles == nil {
		m.capturedFiles = make(map[string]string)
	}
	for _, name := range []string{".kitchen.local.yml", ".kitchen.local.yml.bak"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			m.capturedFiles[name] = string(data)
		}
	}
	return m.stdout, m.stderr, m.exitCode, m.err
}

type mockCredResolver struct {
	envVars map[string][]byte
	err     error
	cleaned bool
}

func (m *mockCredResolver) ResolveKitchenCredentials(_ context.Context, _ config.TestKitchenConfig) (map[string][]byte, func(), error) {
	cleanup := func() { m.cleaned = true }
	return m.envVars, cleanup, m.err
}

func baseTKConfig() config.TestKitchenConfig {
	return config.TestKitchenConfig{
		Driver: "proxmox",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-2204", Image: "ubuntu22"},
		},
		Images: []config.ImageEntry{
			{Name: "ubuntu22", ID: "tpl-ubuntu22"},
		},
	}
}

func baseParams(repoDir string) RunInstanceParams {
	return RunInstanceParams{
		GitRepoName:       "example-cookbook",
		GitRepoURL:        "https://git.example.com/org/example-cookbook.git",
		RepoDir:           repoDir,
		InstanceName:      "default-ubuntu-2204",
		SuiteName:         "default",
		PlatformName:      "ubuntu-2204",
		TargetChefVersion: "18.4.2",
		CommitSHA:         "abc123",
	}
}

func setupRepoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Create a marker file to verify isolation
	if err := os.WriteFile(filepath.Join(dir, "metadata.rb"), []byte("name 'example'"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunInstance_Success(t *testing.T) {
	repoDir := setupRepoDir(t)
	exec := &mockExecutor{
		stdout:   "Kitchen converge passed\n",
		stderr:   "some warnings\n",
		exitCode: 0,
	}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	result := RunInstance(context.Background(), baseParams(repoDir), baseTKConfig(), exec, cred)

	if result.Passed == nil {
		t.Fatal("expected Passed to be non-nil")
	}
	if !*result.Passed {
		t.Error("expected Passed to be true")
	}
	if !strings.Contains(result.Output, "Kitchen converge passed") {
		t.Error("expected stdout in output")
	}
	if !strings.Contains(result.Output, "some warnings") {
		t.Error("expected stderr in output")
	}
	if result.ErrorMessage != "" {
		t.Errorf("unexpected error message: %s", result.ErrorMessage)
	}
}

func TestRunInstance_KitchenFailure(t *testing.T) {
	repoDir := setupRepoDir(t)
	exec := &mockExecutor{
		stdout:   "Converging...\n",
		stderr:   "ERROR: Chef run failed\n",
		exitCode: 1,
	}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	result := RunInstance(context.Background(), baseParams(repoDir), baseTKConfig(), exec, cred)

	if result.Passed == nil {
		t.Fatal("expected Passed to be non-nil")
	}
	if *result.Passed {
		t.Error("expected Passed to be false")
	}
	if !strings.Contains(result.Output, "ERROR: Chef run failed") {
		t.Error("expected stderr in output")
	}
}

func TestRunInstance_ExecutorError(t *testing.T) {
	repoDir := setupRepoDir(t)
	exec := &mockExecutor{
		err: errors.New("failed to start kitchen process"),
	}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	result := RunInstance(context.Background(), baseParams(repoDir), baseTKConfig(), exec, cred)

	if result.Passed != nil {
		t.Error("expected Passed to be nil on executor error")
	}
	if result.ErrorMessage == "" {
		t.Error("expected ErrorMessage to be set")
	}
	if !strings.Contains(result.ErrorMessage, "failed to start kitchen process") {
		t.Errorf("expected error message to contain executor error, got: %s", result.ErrorMessage)
	}
}

func TestRunInstance_OverlayGenerated(t *testing.T) {
	repoDir := setupRepoDir(t)
	exec := &mockExecutor{exitCode: 0}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	RunInstance(context.Background(), baseParams(repoDir), baseTKConfig(), exec, cred)

	// The executor should have been called with a workspace dir (not the original repo)
	if exec.dir == "" {
		t.Fatal("executor was not called")
	}
	if exec.dir == repoDir {
		t.Error("executor should run in isolated workspace, not original repo dir")
	}

	// Verify overlay was captured from workspace during execution
	overlay, ok := exec.capturedFiles[".kitchen.local.yml"]
	if !ok {
		t.Fatal("overlay was not written in workspace before kitchen ran")
	}
	if !strings.Contains(overlay, "name: proxmox") {
		t.Error("overlay does not contain expected driver config")
	}
}

func TestRunInstance_IPReleaseHook_ComposesRepoPreDestroy(t *testing.T) {
	repoDir := setupRepoDir(t)
	// Cookbook ships its own pre_destroy hook in .kitchen.yml.
	kitchenYML := "lifecycle:\n  pre_destroy:\n    - remote: echo repo-teardown\n"
	if err := os.WriteFile(filepath.Join(repoDir, ".kitchen.yml"), []byte(kitchenYML), 0644); err != nil {
		t.Fatal(err)
	}

	tkConfig := baseTKConfig()
	tkConfig.Images[0].ReleaseIPOnDestroy = true

	exec := &mockExecutor{exitCode: 0}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	RunInstance(context.Background(), baseParams(repoDir), tkConfig, exec, cred)

	overlay, ok := exec.capturedFiles[".kitchen.local.yml"]
	if !ok {
		t.Fatal("overlay was not written in workspace")
	}
	repoIdx := strings.Index(overlay, "repo-teardown")
	cmmIdx := strings.Index(overlay, "dhclient")
	if repoIdx < 0 || cmmIdx < 0 {
		t.Fatalf("expected composed pre_destroy with repo hook + release command, got:\n%s", overlay)
	}
	if repoIdx > cmmIdx {
		t.Errorf("expected repo hook before injected release, got:\n%s", overlay)
	}
}

// A cookbook ships hooks in several phases. CMM reserves pre_destroy only, so
// every other phase must be absent from the overlay — TK's untouched .kitchen.yml
// plus its array-replace merge then preserves them. Asserted with IP-release both
// on and off so the reserved pre_destroy hook does not leak into other phases.
func TestRunInstance_PreservesRepoLifecyclePhases(t *testing.T) {
	kitchenYML := "lifecycle:\n" +
		"  pre_create:\n    - remote: echo repo-pre-create\n" +
		"  pre_converge:\n    - remote: echo repo-pre-converge\n" +
		"  post_converge:\n    - remote: echo repo-post-converge\n" +
		"  pre_destroy:\n    - remote: echo repo-pre-destroy\n"

	for _, tc := range []struct {
		name      string
		releaseIP bool
	}{
		{"IPReleaseOff", false},
		{"IPReleaseOn", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoDir := setupRepoDir(t)
			if err := os.WriteFile(filepath.Join(repoDir, ".kitchen.yml"), []byte(kitchenYML), 0644); err != nil {
				t.Fatal(err)
			}

			tkConfig := baseTKConfig()
			tkConfig.Images[0].ReleaseIPOnDestroy = tc.releaseIP

			exec := &mockExecutor{exitCode: 0}
			cred := &mockCredResolver{envVars: map[string][]byte{}}

			RunInstance(context.Background(), baseParams(repoDir), tkConfig, exec, cred)

			overlay := exec.capturedFiles[".kitchen.local.yml"]

			// The overlay must never echo a repo-owned phase: it leaves them in the
			// untouched .kitchen.yml for the merge to preserve.
			for _, phase := range []string{"pre_create", "pre_converge", "post_converge"} {
				if strings.Contains(overlay, phase) {
					t.Errorf("overlay must not write repo-owned phase %q, got:\n%s", phase, overlay)
				}
			}
			for _, body := range []string{"repo-pre-create", "repo-pre-converge", "repo-post-converge"} {
				if strings.Contains(overlay, body) {
					t.Errorf("overlay must not echo repo hook %q from a non-reserved phase, got:\n%s", body, overlay)
				}
			}

			if tc.releaseIP {
				// CMM owns pre_destroy: it composes (repo hook preserved, run first).
				if !strings.Contains(overlay, "pre_destroy:") || !strings.Contains(overlay, "repo-pre-destroy") {
					t.Errorf("expected composed pre_destroy carrying the repo hook, got:\n%s", overlay)
				}
			} else {
				// No reserved phase written → no lifecycle block at all.
				if strings.Contains(overlay, "lifecycle:") {
					t.Errorf("expected no lifecycle block when IP-release is off, got:\n%s", overlay)
				}
			}
		})
	}
}

func TestRunInstance_SetupScripts_InlinedForOSFamily(t *testing.T) {
	repoDir := setupRepoDir(t)
	setupDir := filepath.Join(repoDir, "test", "setup")
	if err := os.MkdirAll(setupDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Two linux scripts (matched, sorted) + one windows script (not matched
	// on a linux platform).
	for name, body := range map[string]string{
		"a-users.sh": "useradd -m svc\n",
		"b-dirs.sh":  "mkdir -p /opt/app\n",
		"win.ps1":    "New-LocalUser svc\n",
	} {
		if err := os.WriteFile(filepath.Join(setupDir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	tkConfig := baseTKConfig()
	tkConfig.SetupScripts = &config.SetupScriptsConfig{
		Linux:   []string{"test/setup/*.sh"},
		Windows: []string{"test/setup/*.ps1"},
	}

	exec := &mockExecutor{exitCode: 0}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	// baseParams resolves to ubuntu-2204 (linux).
	RunInstance(context.Background(), baseParams(repoDir), tkConfig, exec, cred)

	overlay := exec.capturedFiles[".kitchen.local.yml"]
	if !strings.Contains(overlay, "pre_converge:") {
		t.Fatalf("expected setup scripts in pre_converge, got:\n%s", overlay)
	}
	aIdx := strings.Index(overlay, "useradd -m svc")
	bIdx := strings.Index(overlay, "mkdir -p /opt/app")
	if aIdx < 0 || bIdx < 0 {
		t.Fatalf("expected both linux setup scripts inlined, got:\n%s", overlay)
	}
	if aIdx > bIdx {
		t.Errorf("expected scripts in sorted-by-path order (a before b), got:\n%s", overlay)
	}
	// Windows script must not leak onto a linux platform.
	if strings.Contains(overlay, "New-LocalUser") {
		t.Errorf("windows setup script must not be inlined on a linux platform, got:\n%s", overlay)
	}
}

func TestRunInstance_SetupScripts_NoneWhenNoMatch(t *testing.T) {
	repoDir := setupRepoDir(t)

	tkConfig := baseTKConfig()
	tkConfig.SetupScripts = &config.SetupScriptsConfig{
		Linux: []string{"test/setup/*.sh"}, // no such files in the repo
	}

	exec := &mockExecutor{exitCode: 0}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	RunInstance(context.Background(), baseParams(repoDir), tkConfig, exec, cred)

	overlay := exec.capturedFiles[".kitchen.local.yml"]
	if strings.Contains(overlay, "pre_converge:") {
		t.Errorf("expected no pre_converge block when no script matches, got:\n%s", overlay)
	}
}

func TestRunInstance_IPReleaseHook_OffByDefault(t *testing.T) {
	repoDir := setupRepoDir(t)
	exec := &mockExecutor{exitCode: 0}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	RunInstance(context.Background(), baseParams(repoDir), baseTKConfig(), exec, cred)

	overlay := exec.capturedFiles[".kitchen.local.yml"]
	if strings.Contains(overlay, "pre_destroy:") {
		t.Errorf("expected no pre_destroy hook when opt-in is off, got:\n%s", overlay)
	}
}

func TestRunInstance_ExistingLocalOverride_Backed(t *testing.T) {
	repoDir := setupRepoDir(t)
	// Place an existing .kitchen.local.yml in the repo
	existingOverlay := "driver:\n  name: vagrant\n"
	if err := os.WriteFile(filepath.Join(repoDir, ".kitchen.local.yml"), []byte(existingOverlay), 0644); err != nil {
		t.Fatal(err)
	}

	exec := &mockExecutor{exitCode: 0}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	RunInstance(context.Background(), baseParams(repoDir), baseTKConfig(), exec, cred)

	if exec.dir == "" {
		t.Fatal("executor was not called")
	}

	// In the workspace, the original should be backed up (captured during Run)
	bakContent, ok := exec.capturedFiles[".kitchen.local.yml.bak"]
	if !ok {
		t.Fatal("backup file not found in workspace during execution")
	}
	if bakContent != existingOverlay {
		t.Errorf("backup content mismatch: got %q", bakContent)
	}

	// The new overlay should exist
	newOverlay, ok := exec.capturedFiles[".kitchen.local.yml"]
	if !ok {
		t.Fatal("new overlay not found in workspace during execution")
	}
	if !strings.Contains(newOverlay, "name: proxmox") {
		t.Error("new overlay should contain proxmox driver")
	}
}

func TestRunInstance_IsolatedWorkspace(t *testing.T) {
	repoDir := setupRepoDir(t)
	exec := &mockExecutor{exitCode: 0}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	RunInstance(context.Background(), baseParams(repoDir), baseTKConfig(), exec, cred)

	// Original repo dir should not have .kitchen.local.yml written by us
	// (it may have one if it existed before, but the content should be unchanged)
	entries, _ := os.ReadDir(repoDir)
	for _, e := range entries {
		if e.Name() == ".kitchen.local.yml" {
			// File existed before — that's OK, but no .bak should appear here
			bakPath := filepath.Join(repoDir, ".kitchen.local.yml.bak")
			if _, err := os.Stat(bakPath); err == nil {
				t.Error("original repo dir should not be modified — .bak file found")
			}
		}
	}

	// Marker file should still be intact
	data, err := os.ReadFile(filepath.Join(repoDir, "metadata.rb"))
	if err != nil {
		t.Fatal("marker file missing from original repo dir")
	}
	if string(data) != "name 'example'" {
		t.Error("marker file content changed in original repo dir")
	}
}

func TestRunInstance_CredentialEnvVars(t *testing.T) {
	repoDir := setupRepoDir(t)
	exec := &mockExecutor{exitCode: 0}
	cred := &mockCredResolver{
		envVars: map[string][]byte{
			"CMM_TK_SECRET_TOKEN":    []byte("secret-value"),
			"CMM_TK_TRANSPORT_IMG1":  []byte("password123"),
		},
	}

	RunInstance(context.Background(), baseParams(repoDir), baseTKConfig(), exec, cred)

	if len(exec.extraEnv) == 0 {
		t.Fatal("expected credential env vars to be passed to executor")
	}

	envMap := make(map[string]string)
	for _, e := range exec.extraEnv {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	if envMap["CMM_TK_SECRET_TOKEN"] != "secret-value" {
		t.Errorf("expected CMM_TK_SECRET_TOKEN=secret-value, got %q", envMap["CMM_TK_SECRET_TOKEN"])
	}
	if envMap["CMM_TK_TRANSPORT_IMG1"] != "password123" {
		t.Errorf("expected CMM_TK_TRANSPORT_IMG1=password123, got %q", envMap["CMM_TK_TRANSPORT_IMG1"])
	}
}

func TestRunInstance_ContextCancelled(t *testing.T) {
	repoDir := setupRepoDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	exec := &mockExecutor{
		err: context.Canceled,
	}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	result := RunInstance(ctx, baseParams(repoDir), baseTKConfig(), exec, cred)

	if result.Passed != nil {
		t.Error("expected Passed to be nil on cancelled context")
	}
	if result.ErrorMessage == "" {
		t.Error("expected ErrorMessage to be set")
	}
	if result.TimedOut {
		t.Error("context.Canceled should NOT set TimedOut")
	}
}

func TestRunInstance_DeadlineExceeded_SetsTimedOut(t *testing.T) {
	repoDir := setupRepoDir(t)
	exec := &mockExecutor{
		err:    context.DeadlineExceeded,
		stdout: "Waiting for SSH access...\n",
	}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	result := RunInstance(context.Background(), baseParams(repoDir), baseTKConfig(), exec, cred)

	if !result.TimedOut {
		t.Error("expected TimedOut to be true on DeadlineExceeded")
	}
	if result.Passed != nil {
		t.Error("expected Passed to be nil on timeout")
	}
	if result.ErrorMessage == "" {
		t.Error("expected ErrorMessage to be set")
	}
}

func TestRunInstance_NetworkTimeout_NoConvergeActivity(t *testing.T) {
	repoDir := setupRepoDir(t)
	// Timeout with output that has no converge activity — should be classified as network timeout.
	exec := &mockExecutor{
		err:    context.DeadlineExceeded,
		stdout: "Waiting for SSH access on 10.0.0.1\nConnection timed out\n",
	}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	result := RunInstance(context.Background(), baseParams(repoDir), baseTKConfig(), exec, cred)

	if !result.TimedOut {
		t.Error("expected TimedOut to be true")
	}
	if !result.NetworkTimeout {
		t.Error("expected NetworkTimeout to be true when timed out with no converge output")
	}
	if !strings.Contains(result.ErrorMessage, "DHCP/network timeout") {
		t.Errorf("expected network timeout error message, got: %s", result.ErrorMessage)
	}
}

func TestRunInstance_Timeout_WithConvergeActivity_NotNetworkTimeout(t *testing.T) {
	repoDir := setupRepoDir(t)
	// Timeout but output DOES contain converge activity — not a network timeout.
	exec := &mockExecutor{
		err:    context.DeadlineExceeded,
		stdout: "Converging 5 resources\n  * package[curl] action install\n  * file[/tmp/test] action create\n",
	}
	cred := &mockCredResolver{envVars: map[string][]byte{}}

	result := RunInstance(context.Background(), baseParams(repoDir), baseTKConfig(), exec, cred)

	if !result.TimedOut {
		t.Error("expected TimedOut to be true")
	}
	if result.NetworkTimeout {
		t.Error("expected NetworkTimeout to be false when converge activity is present")
	}
}

func TestIsConvergeOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"empty", "", false},
		{"ssh waiting only", "Waiting for SSH access...\n", false},
		{"converge header", "Converging 3 resources\n", true},
		{"resource action", "  * package[curl] action install\n", true},
		{"recipe compile", "  Recipe: my_cookbook::default\n", true},
		{"chef client run", "Starting Chef Infra Client, version 18.4.2\n", true},
		{"chef run start", "Starting Chef Client, version 17.0.0\n", true},
		{"resolving cookbooks", "resolving cookbooks for run list\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasConvergeActivity(tt.output)
			if got != tt.want {
				t.Errorf("hasConvergeActivity(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}
