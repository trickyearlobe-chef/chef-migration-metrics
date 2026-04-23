// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package batch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Mock implementations for KitchenRunner tests ---

type mockExecCall struct {
	Dir      string
	ExtraEnv []string
	Args     []string
}

type mockKitchenExec struct {
	mu       sync.Mutex
	calls    []mockExecCall
	resultFn func(call mockExecCall) (string, string, int, error)
}

func (m *mockKitchenExec) Run(_ context.Context, dir string, extraEnv []string, args ...string) (string, string, int, error) {
	call := mockExecCall{Dir: dir, ExtraEnv: extraEnv, Args: args}
	m.mu.Lock()
	m.calls = append(m.calls, call)
	m.mu.Unlock()
	if m.resultFn != nil {
		return m.resultFn(call)
	}
	return "ok", "", 0, nil
}

func (m *mockKitchenExec) getCalls() []mockExecCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mockExecCall, len(m.calls))
	copy(out, m.calls)
	return out
}

type mockCredProvider struct {
	envVars []string
	err     error
	cleaned bool
}

func (m *mockCredProvider) ResolveCredentials(_ context.Context) ([]string, func(), error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return m.envVars, func() { m.cleaned = true }, nil
}

// writeKitchenYAML creates a minimal .kitchen.yml stub in dir.
func writeKitchenYAML(t *testing.T, dir string) {
	t.Helper()
	content := "driver:\n  name: vagrant\n"
	if err := os.WriteFile(filepath.Join(dir, ".kitchen.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("write .kitchen.yml: %v", err)
	}
}

// newTestRunner creates a KitchenRunner with sensible defaults for testing.
func newTestRunner(t *testing.T, exec *mockKitchenExec, cred *mockCredProvider, repoDir string, overlay OverlayConfig) *KitchenRunner {
	t.Helper()
	if cred == nil {
		cred = &mockCredProvider{}
	}
	return NewKitchenRunner(KitchenRunnerConfig{
		Executor:     exec,
		CredProvider: cred,
		Logger:       discardLogger{},
		RepoDir:      func(_ string) string { return repoDir },
		Timeout:      30 * time.Second,
		Overlay:      overlay,
	})
}

func TestKitchenRunner_BasicRun(t *testing.T) {
	dir := t.TempDir()
	writeKitchenYAML(t, dir)

	exec := &mockKitchenExec{}
	runner := newTestRunner(t, exec, nil, dir, OverlayConfig{})

	result := runner.RunInstance(context.Background(), RunInstanceRequest{
		BatchID:           "b1",
		GitRepoName:       "test-cb",
		GitRepoURL:        "https://git.example.com/test-cb.git",
		CommitSHA:         "abc123",
		TargetChefVersion: "18.5.0",
		PlatformName:      "ubuntu-22.04",
		SuiteName:         "default",
	})

	if result.ConvergePassed == nil || !*result.ConvergePassed {
		t.Error("expected ConvergePassed=true")
	}
	if result.TestsPassed == nil || !*result.TestsPassed {
		t.Error("expected TestsPassed=true")
	}
	if result.TimedOut {
		t.Error("expected TimedOut=false")
	}

	calls := exec.getCalls()
	if len(calls) != 3 {
		t.Fatalf("expected 3 executor calls (converge, verify, destroy), got %d", len(calls))
	}

	if result.DurationSeconds == nil || *result.DurationSeconds < 0 {
		t.Error("expected DurationSeconds >= 0")
	}
	if result.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}
	if result.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestKitchenRunner_ConvergeFails(t *testing.T) {
	dir := t.TempDir()
	writeKitchenYAML(t, dir)

	exec := &mockKitchenExec{
		resultFn: func(call mockExecCall) (string, string, int, error) {
			for _, a := range call.Args {
				if a == "converge" {
					return "converge failed", "error output", 1, nil
				}
				if a == "destroy" {
					return "destroyed", "", 0, nil
				}
			}
			return "ok", "", 0, nil
		},
	}
	runner := newTestRunner(t, exec, nil, dir, OverlayConfig{})

	result := runner.RunInstance(context.Background(), RunInstanceRequest{
		BatchID:           "b2",
		GitRepoName:       "fail-cb",
		GitRepoURL:        "https://git.example.com/fail-cb.git",
		CommitSHA:         "def456",
		TargetChefVersion: "18.5.0",
		PlatformName:      "ubuntu-22.04",
		SuiteName:         "default",
	})

	if result.ConvergePassed == nil || *result.ConvergePassed {
		t.Error("expected ConvergePassed=false")
	}
	if result.TestsPassed != nil {
		t.Error("expected TestsPassed=nil (verify not run)")
	}

	calls := exec.getCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 executor calls (converge, destroy), got %d", len(calls))
	}
}

func TestKitchenRunner_LocalOverrideBackupRestore(t *testing.T) {
	dir := t.TempDir()
	writeKitchenYAML(t, dir)

	originalContent := "original content"
	localPath := filepath.Join(dir, ".kitchen.local.yml")
	if err := os.WriteFile(localPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("write .kitchen.local.yml: %v", err)
	}

	exec := &mockKitchenExec{
		resultFn: func(call mockExecCall) (string, string, int, error) {
			for _, a := range call.Args {
				if a == "converge" {
					data, err := os.ReadFile(filepath.Join(call.Dir, ".kitchen.local.yml"))
					if err != nil {
						t.Errorf("read .kitchen.local.yml during converge: %v", err)
						return "", "", 1, err
					}
					if !strings.Contains(string(data), "generated by chef-migration-metrics") {
						t.Errorf("expected overlay content during converge, got: %s", string(data))
					}
					return "ok", "", 0, nil
				}
			}
			return "ok", "", 0, nil
		},
	}
	runner := newTestRunner(t, exec, nil, dir, OverlayConfig{})

	runner.RunInstance(context.Background(), RunInstanceRequest{
		BatchID:           "b3",
		GitRepoName:       "backup-cb",
		GitRepoURL:        "https://git.example.com/backup-cb.git",
		CommitSHA:         "aaa111",
		TargetChefVersion: "18.5.0",
		PlatformName:      "ubuntu-22.04",
		SuiteName:         "default",
	})

	restored, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("expected .kitchen.local.yml to be restored, got error: %v", err)
	}
	if string(restored) != originalContent {
		t.Errorf("expected restored content %q, got %q", originalContent, string(restored))
	}

	bakPath := filepath.Join(dir, ".kitchen.local.yml.bak")
	if _, err := os.Stat(bakPath); !os.IsNotExist(err) {
		t.Error("expected .kitchen.local.yml.bak to be cleaned up")
	}
}

func TestKitchenRunner_NoLocalOverride(t *testing.T) {
	dir := t.TempDir()
	writeKitchenYAML(t, dir)

	exec := &mockKitchenExec{}
	runner := newTestRunner(t, exec, nil, dir, OverlayConfig{})

	runner.RunInstance(context.Background(), RunInstanceRequest{
		BatchID:           "b4",
		GitRepoName:       "nolocal-cb",
		GitRepoURL:        "https://git.example.com/nolocal-cb.git",
		CommitSHA:         "bbb222",
		TargetChefVersion: "18.5.0",
		PlatformName:      "ubuntu-22.04",
		SuiteName:         "default",
	})

	localPath := filepath.Join(dir, ".kitchen.local.yml")
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Error("expected .kitchen.local.yml to NOT exist after run")
	}
}

func TestKitchenRunner_ChefVersionOverlay_Chef18(t *testing.T) {
	dir := t.TempDir()
	writeKitchenYAML(t, dir)

	var capturedOverlay string
	exec := &mockKitchenExec{
		resultFn: func(call mockExecCall) (string, string, int, error) {
			for _, a := range call.Args {
				if a == "converge" {
					data, err := os.ReadFile(filepath.Join(call.Dir, ".kitchen.local.yml"))
					if err == nil {
						capturedOverlay = string(data)
					}
					return "ok", "", 0, nil
				}
			}
			return "ok", "", 0, nil
		},
	}
	runner := newTestRunner(t, exec, nil, dir, OverlayConfig{})

	runner.RunInstance(context.Background(), RunInstanceRequest{
		BatchID:           "b5",
		GitRepoName:       "chef18-cb",
		GitRepoURL:        "https://git.example.com/chef18-cb.git",
		CommitSHA:         "ccc333",
		TargetChefVersion: "18.5.0",
		PlatformName:      "ubuntu-22.04",
		SuiteName:         "default",
	})

	if !strings.Contains(capturedOverlay, `product_version: "18.5.0"`) {
		t.Errorf("expected overlay to contain product_version 18.5.0, got:\n%s", capturedOverlay)
	}
	if !strings.Contains(capturedOverlay, "chef_license: accept") {
		t.Errorf("expected overlay to contain chef_license: accept, got:\n%s", capturedOverlay)
	}
	if strings.Contains(capturedOverlay, "name: chef_ice") {
		t.Errorf("overlay should NOT contain chef_ice for Chef 18, got:\n%s", capturedOverlay)
	}
}

func TestKitchenRunner_ChefVersionOverlay_Chef19(t *testing.T) {
	dir := t.TempDir()
	writeKitchenYAML(t, dir)

	var capturedOverlay string
	exec := &mockKitchenExec{
		resultFn: func(call mockExecCall) (string, string, int, error) {
			for _, a := range call.Args {
				if a == "converge" {
					data, err := os.ReadFile(filepath.Join(call.Dir, ".kitchen.local.yml"))
					if err == nil {
						capturedOverlay = string(data)
					}
					return "ok", "", 0, nil
				}
			}
			return "ok", "", 0, nil
		},
	}
	runner := newTestRunner(t, exec, nil, dir, OverlayConfig{})

	runner.RunInstance(context.Background(), RunInstanceRequest{
		BatchID:           "b6",
		GitRepoName:       "chef19-cb",
		GitRepoURL:        "https://git.example.com/chef19-cb.git",
		CommitSHA:         "ddd444",
		TargetChefVersion: "19.0.0",
		PlatformName:      "ubuntu-22.04",
		SuiteName:         "default",
	})

	if !strings.Contains(capturedOverlay, "chef_ice") {
		t.Errorf("expected overlay to contain chef_ice for Chef 19, got:\n%s", capturedOverlay)
	}
}

func TestKitchenRunner_CredentialInjection(t *testing.T) {
	dir := t.TempDir()
	writeKitchenYAML(t, dir)

	cred := &mockCredProvider{
		envVars: []string{"CMM_TK_SECRET_PASSWORD=secret123"},
	}
	exec := &mockKitchenExec{}
	runner := newTestRunner(t, exec, cred, dir, OverlayConfig{})

	runner.RunInstance(context.Background(), RunInstanceRequest{
		BatchID:           "b7",
		GitRepoName:       "cred-cb",
		GitRepoURL:        "https://git.example.com/cred-cb.git",
		CommitSHA:         "eee555",
		TargetChefVersion: "18.5.0",
		PlatformName:      "ubuntu-22.04",
		SuiteName:         "default",
	})

	calls := exec.getCalls()
	for i, call := range calls {
		found := false
		for _, env := range call.ExtraEnv {
			if env == "CMM_TK_SECRET_PASSWORD=secret123" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("call %d: expected CMM_TK_SECRET_PASSWORD in extraEnv, got %v", i, call.ExtraEnv)
		}
	}

	if !cred.cleaned {
		t.Error("expected credential cleanup to be called")
	}
}

func TestKitchenRunner_EmptyRepoDir(t *testing.T) {
	exec := &mockKitchenExec{}
	runner := NewKitchenRunner(KitchenRunnerConfig{
		Executor:     exec,
		CredProvider: &mockCredProvider{},
		Logger:       discardLogger{},
		RepoDir:      func(_ string) string { return "" },
		Timeout:      30 * time.Second,
	})

	result := runner.RunInstance(context.Background(), RunInstanceRequest{
		BatchID:           "b8",
		GitRepoName:       "empty-dir-cb",
		GitRepoURL:        "https://git.example.com/empty-dir-cb.git",
		CommitSHA:         "fff666",
		TargetChefVersion: "18.5.0",
		PlatformName:      "ubuntu-22.04",
		SuiteName:         "default",
	})

	if result.ErrorMessage == "" || !strings.Contains(strings.ToLower(result.ErrorMessage), "directory") {
		t.Errorf("expected ErrorMessage containing 'directory', got %q", result.ErrorMessage)
	}

	calls := exec.getCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 executor calls, got %d", len(calls))
	}
}

func TestChefMajorVersion(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"18.5.0", 18},
		{"19.0.0", 19},
		{"17.10.3", 17},
		{"", 0},
		{"abc", 0},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := chefMajorVersion(tc.input)
			if got != tc.want {
				t.Errorf("chefMajorVersion(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestBackupLocalOverride(t *testing.T) {
	t.Run("file exists", func(t *testing.T) {
		dir := t.TempDir()
		localPath := filepath.Join(dir, ".kitchen.local.yml")
		if err := os.WriteFile(localPath, []byte("existing"), 0644); err != nil {
			t.Fatal(err)
		}

		had, err := backupLocalOverride(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !had {
			t.Error("expected had=true")
		}

		bakPath := filepath.Join(dir, ".kitchen.local.yml.bak")
		if _, err := os.Stat(bakPath); err != nil {
			t.Error("expected .bak file to exist")
		}
		if _, err := os.Stat(localPath); !os.IsNotExist(err) {
			t.Error("expected original file to be renamed away")
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		dir := t.TempDir()

		had, err := backupLocalOverride(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if had {
			t.Error("expected had=false")
		}
	})
}

func TestRestoreLocalOverride(t *testing.T) {
	t.Run("had override with bak file", func(t *testing.T) {
		dir := t.TempDir()
		bakPath := filepath.Join(dir, ".kitchen.local.yml.bak")
		localPath := filepath.Join(dir, ".kitchen.local.yml")

		if err := os.WriteFile(bakPath, []byte("original"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(localPath, []byte("overlay"), 0644); err != nil {
			t.Fatal(err)
		}

		restoreLocalOverride(dir, true)

		data, err := os.ReadFile(localPath)
		if err != nil {
			t.Fatalf("expected .kitchen.local.yml to exist: %v", err)
		}
		if string(data) != "original" {
			t.Errorf("expected restored content 'original', got %q", string(data))
		}
		if _, err := os.Stat(bakPath); !os.IsNotExist(err) {
			t.Error("expected .bak file to be removed")
		}
	})

	t.Run("had no override", func(t *testing.T) {
		dir := t.TempDir()
		localPath := filepath.Join(dir, ".kitchen.local.yml")

		if err := os.WriteFile(localPath, []byte("overlay"), 0644); err != nil {
			t.Fatal(err)
		}

		restoreLocalOverride(dir, false)

		if _, err := os.Stat(localPath); !os.IsNotExist(err) {
			t.Error("expected .kitchen.local.yml to be removed")
		}
	})
}
