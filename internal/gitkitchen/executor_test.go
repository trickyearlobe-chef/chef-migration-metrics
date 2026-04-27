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
}
