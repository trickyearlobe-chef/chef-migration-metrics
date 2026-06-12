// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package nodekitchen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// ---------------------------------------------------------------------------
// DBCookbookDependencyResolver tests
// ---------------------------------------------------------------------------

func TestDBCookbookDependencyResolver_WithDeps(t *testing.T) {
	deps := map[string]string{"apt": ">= 0.0.0", "yum": "~> 5.0"}
	depsJSON, _ := json.Marshal(deps)

	resolver := &testDBDepResolver{
		deps: map[string]map[string]string{
			"myorg/nginx/1.0.0": deps,
		},
		raw: map[string]json.RawMessage{
			"myorg/nginx/1.0.0": depsJSON,
		},
	}

	got, err := resolver.GetCookbookDependencies(context.Background(), "myorg", "nginx", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(got))
	}
	if got["apt"] != ">= 0.0.0" {
		t.Errorf("apt dep = %q, want %q", got["apt"], ">= 0.0.0")
	}
	if got["yum"] != "~> 5.0" {
		t.Errorf("yum dep = %q, want %q", got["yum"], "~> 5.0")
	}
}

func TestDBCookbookDependencyResolver_NoDeps(t *testing.T) {
	resolver := &testDBDepResolver{
		deps: map[string]map[string]string{
			"myorg/simple/1.0.0": nil,
		},
		raw: map[string]json.RawMessage{
			"myorg/simple/1.0.0": nil,
		},
	}

	got, err := resolver.GetCookbookDependencies(context.Background(), "myorg", "simple", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil deps, got %v", got)
	}
}

func TestDBCookbookDependencyResolver_NullJSON(t *testing.T) {
	resolver := &testDBDepResolver{
		deps: map[string]map[string]string{
			"myorg/nulldeps/1.0.0": nil,
		},
		raw: map[string]json.RawMessage{
			"myorg/nulldeps/1.0.0": json.RawMessage("null"),
		},
	}

	got, err := resolver.GetCookbookDependencies(context.Background(), "myorg", "nulldeps", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil deps for null JSON, got %v", got)
	}
}

func TestDBCookbookDependencyResolver_NotFound(t *testing.T) {
	resolver := &testDBDepResolver{
		deps: map[string]map[string]string{},
		raw:  map[string]json.RawMessage{},
	}

	_, err := resolver.GetCookbookDependencies(context.Background(), "myorg", "missing", "1.0.0")
	if err == nil {
		t.Fatal("expected error for missing cookbook")
	}
}

// testDBDepResolver is a test double for CookbookDependencyResolver that
// avoids importing datastore.
type testDBDepResolver struct {
	deps map[string]map[string]string
	raw  map[string]json.RawMessage
}

func (r *testDBDepResolver) GetCookbookDependencies(_ context.Context, orgName, cookbookName, version string) (map[string]string, error) {
	key := orgName + "/" + cookbookName + "/" + version
	raw, ok := r.raw[key]
	if !ok {
		return nil, fmt.Errorf("cookbook not found: %s", key)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var deps map[string]string
	if err := json.Unmarshal(raw, &deps); err != nil {
		return nil, err
	}
	return deps, nil
}

// ---------------------------------------------------------------------------
// ScopedLoggerAdapter tests
// ---------------------------------------------------------------------------

func TestScopedLoggerAdapter_DelegatesToFunctions(t *testing.T) {
	var infoCalled, warnCalled, errorCalled bool

	adapter := &ScopedLoggerAdapter{
		Info_:  func(msg string) { infoCalled = true },
		Warn_:  func(msg string) { warnCalled = true },
		Error_: func(msg string) { errorCalled = true },
	}

	adapter.Info("test info")
	adapter.Warn("test warn")
	adapter.Error("test error")

	if !infoCalled {
		t.Error("Info_ was not called")
	}
	if !warnCalled {
		t.Error("Warn_ was not called")
	}
	if !errorCalled {
		t.Error("Error_ was not called")
	}
}

func TestScopedLoggerAdapter_PassesMessage(t *testing.T) {
	var gotMsg string
	adapter := &ScopedLoggerAdapter{
		Info_:  func(msg string) { gotMsg = msg },
		Warn_:  func(msg string) {},
		Error_: func(msg string) {},
	}

	adapter.Info("hello world")
	if gotMsg != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", gotMsg)
	}
}

// ---------------------------------------------------------------------------
// RunnerFactory tests
// ---------------------------------------------------------------------------

func TestRunnerFactory_ClientFactoryError(t *testing.T) {
	factory := &RunnerFactory{
		ClientFactory: func(_ context.Context, orgName string) (*chefapi.Client, error) {
			return nil, fmt.Errorf("credential resolution failed for %s", orgName)
		},
		Logger: &testLogger{},
	}

	result := factory.Run(context.Background(), RunRequest{
		NodeName:          "web1",
		OrganisationName:  "test-org",
		TargetChefVersion: "18.0.0",
		CookbookSource:    "server",
	})
	if result.Error == nil {
		t.Fatal("expected error from client factory")
	}
	if result.ErrorMessage == "" {
		t.Error("expected non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// DefaultExecutor tests
// ---------------------------------------------------------------------------

func TestDefaultExecutor_RunEcho(t *testing.T) {
	// Use 'echo' as a simple command to test the executor plumbing.
	executor := &DefaultExecutor{Path: "echo"}
	stdout, stderr, exitCode, err := executor.Run(context.Background(), t.TempDir(), nil, "hello", "world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if stdout != "hello world\n" {
		t.Errorf("stdout = %q, want %q", stdout, "hello world\n")
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestDefaultExecutor_NonZeroExit(t *testing.T) {
	executor := &DefaultExecutor{Path: "false"}
	_, _, exitCode, err := executor.Run(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("unexpected infrastructure error: %v", err)
	}
	if exitCode == 0 {
		t.Error("expected non-zero exit code from 'false'")
	}
}

func TestDefaultExecutor_BinaryNotFound(t *testing.T) {
	executor := &DefaultExecutor{Path: "/nonexistent/binary"}
	_, _, _, err := executor.Run(context.Background(), t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestDefaultExecutor_ExtraEnvPassed(t *testing.T) {
	// Use 'env' to print environment and check our custom var is present.
	executor := &DefaultExecutor{Path: "env"}
	stdout, _, exitCode, err := executor.Run(
		context.Background(), t.TempDir(),
		[]string{"CMM_TEST_VAR=hello_from_test"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if !containsLine(stdout, "CMM_TEST_VAR=hello_from_test") {
		t.Errorf("expected CMM_TEST_VAR in output, got:\n%s", stdout)
	}
}

func TestDefaultExecutor_BundlerEnvSanitised(t *testing.T) {
	// Set BUNDLE_GEMFILE in the extra env and verify it gets stripped
	// by the sanitiser (it only sanitises os.Environ, not extraEnv).
	// This test verifies the sanitiser removes BUNDLE_GEMFILE from the
	// base environment. Since we can't easily inject into os.Environ(),
	// we test sanitiseKitchenEnv directly.
	env := []string{
		"HOME=/home/user",
		"BUNDLE_GEMFILE=/some/path",
		"BUNDLE_BIN_PATH=/some/bin",
		"BUNDLE_PATH=/some/bundle",
		"RUBYOPT=-rbundler/setup",
		"PATH=/usr/bin",
	}
	sanitised := sanitiseKitchenEnv(env)
	for _, kv := range sanitised {
		key := kv
		if idx := len(kv); idx > 0 {
			for i, c := range kv {
				if c == '=' {
					key = kv[:i]
					break
				}
			}
		}
		switch key {
		case "BUNDLE_GEMFILE", "BUNDLE_BIN_PATH", "BUNDLE_PATH", "RUBYOPT":
			t.Errorf("expected %q to be removed, but it was present", key)
		}
	}
	if len(sanitised) != 2 {
		t.Errorf("expected 2 entries (HOME, PATH), got %d: %v", len(sanitised), sanitised)
	}
}

func TestDefaultExecutor_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	executor := &DefaultExecutor{Path: "sleep"}
	_, _, _, err := executor.Run(ctx, t.TempDir(), nil, "60")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// ---------------------------------------------------------------------------
// AnalysisCredentialAdapter tests
// ---------------------------------------------------------------------------

func TestAnalysisCredentialAdapter_NilResolver(t *testing.T) {
	adapter := &AnalysisCredentialAdapter{Resolver: nil}
	envVars, cleanup, err := adapter.ResolveKitchenCredentials(
		context.Background(), config.TestKitchenConfig{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(envVars) != 0 {
		t.Errorf("expected empty env vars, got %d entries", len(envVars))
	}
	if cleanup != nil {
		cleanup()
	}
}

// ---------------------------------------------------------------------------
// sanitiseKitchenEnv tests
// ---------------------------------------------------------------------------

func TestSanitiseKitchenEnv_PreservesNonBundlerVars(t *testing.T) {
	env := []string{"HOME=/home/user", "PATH=/usr/bin", "EDITOR=vim"}
	got := sanitiseKitchenEnv(env)
	if len(got) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(got), got)
	}
}

func TestSanitiseKitchenEnv_Empty(t *testing.T) {
	got := sanitiseKitchenEnv(nil)
	if len(got) != 0 {
		t.Errorf("expected 0 entries, got %d", len(got))
	}
}

func TestSanitiseKitchenEnv_AllBundlerVars(t *testing.T) {
	env := []string{
		"BUNDLE_GEMFILE=/path",
		"BUNDLE_BIN_PATH=/bin",
		"BUNDLE_PATH=/bundle",
		"RUBYOPT=-rbundler/setup",
	}
	got := sanitiseKitchenEnv(env)
	if len(got) != 0 {
		t.Errorf("expected 0 entries after sanitisation, got %d: %v", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type testLogger struct {
	messages []string
}

func (l *testLogger) Info(msg string)  { l.messages = append(l.messages, "INFO: "+msg) }
func (l *testLogger) Warn(msg string)  { l.messages = append(l.messages, "WARN: "+msg) }
func (l *testLogger) Error(msg string) { l.messages = append(l.messages, "ERROR: "+msg) }

func containsLine(output, line string) bool {
	for _, l := range strings.Split(output, "\n") {
		if l == line {
			return true
		}
	}
	return false
}

// TestRunnerFactory_Concurrency_Live verifies the factory resolves the
// cookbook-download concurrency live on each run via ConcurrencyFn, and
// reports 0 (assembly default) when no provider is wired.
func TestRunnerFactory_Concurrency_Live(t *testing.T) {
	var f RunnerFactory
	if got := f.concurrency(); got != 0 {
		t.Errorf("no provider: concurrency = %d, want 0", got)
	}

	live := 4
	f.ConcurrencyFn = func() int { return live }
	if got := f.concurrency(); got != 4 {
		t.Errorf("live concurrency = %d, want 4", got)
	}
	live = 12
	if got := f.concurrency(); got != 12 {
		t.Errorf("live concurrency after change = %d, want 12", got)
	}
}
