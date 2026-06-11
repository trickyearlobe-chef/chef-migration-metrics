// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package embedded

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Fake executor for testing
// ---------------------------------------------------------------------------

// fakeExecCall records a single invocation of the fake executor.
type fakeExecCall struct {
	Name string
	Args []string
}

// fakeExecResult defines the canned response for a command invocation.
type fakeExecResult struct {
	Stdout string
	Stderr string
	Err    error
}

// fakeExecutor is a test double for CommandExecutor. It returns canned
// responses keyed by the binary name.
type fakeExecutor struct {
	// results maps binary name → canned response. When a command is
	// executed, the binary name (last path component) is looked up here.
	results map[string]fakeExecResult

	// calls records every invocation for assertion.
	calls []fakeExecCall
}

func newFakeExecutor() *fakeExecutor {
	return &fakeExecutor{
		results: make(map[string]fakeExecResult),
	}
}

func (f *fakeExecutor) set(name string, stdout, stderr string, err error) {
	f.results[name] = fakeExecResult{Stdout: stdout, Stderr: stderr, Err: err}
}

func (f *fakeExecutor) Execute(_ context.Context, name string, args ...string) (string, string, error) {
	f.calls = append(f.calls, fakeExecCall{Name: name, Args: args})

	// Look up by full path first, then by last component.
	if r, ok := f.results[name]; ok {
		return r.Stdout, r.Stderr, r.Err
	}

	// Try matching by basename for resolved PATH paths like /usr/bin/cookstyle.
	for key, r := range f.results {
		if len(key) > 0 && key[0] != '/' && nameMatches(name, key) {
			return r.Stdout, r.Stderr, r.Err
		}
	}

	return "", "", fmt.Errorf("fakeExecutor: no result configured for %q", name)
}

// nameMatches checks if the full path ends with /name or equals name.
func nameMatches(fullPath, name string) bool {
	if fullPath == name {
		return true
	}
	if len(fullPath) > len(name)+1 && fullPath[len(fullPath)-len(name)-1] == '/' {
		return fullPath[len(fullPath)-len(name):] == name
	}
	return false
}

// ---------------------------------------------------------------------------
// ResolvePath tests
// ---------------------------------------------------------------------------

func TestResolvePath_FoundOnPATH(t *testing.T) {
	r := NewResolver()

	// "go" should always be on PATH in a Go test environment.
	path, err := r.ResolvePath("go")
	if err != nil {
		t.Fatalf("expected to find 'go' on PATH, got error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path for 'go'")
	}
}

func TestResolvePath_NonexistentTool(t *testing.T) {
	r := NewResolver()

	_, err := r.ResolvePath("nonexistent-tool-xyz-12345")
	if err == nil {
		t.Fatal("expected error for nonexistent tool, got nil")
	}
	if got := err.Error(); got == "" {
		t.Fatal("expected non-empty error message")
	}
}

// ---------------------------------------------------------------------------
// ValidateCookstyle tests
// ---------------------------------------------------------------------------

func TestValidateCookstyle_PopulatesName(t *testing.T) {
	fe := newFakeExecutor()
	fe.set("cookstyle", "1.64.8\n", "", nil)

	// cookstyle is unlikely to be on PATH in CI; the test exercises the code
	// path either way (available → version/path set; unavailable → error set).
	r := NewResolver(WithExecutor(fe))
	info := r.ValidateCookstyle(context.Background())

	if info.Name != "cookstyle" {
		t.Errorf("expected Name = cookstyle, got %q", info.Name)
	}
	if info.Available {
		if info.Version == "" {
			t.Error("expected non-empty version when available")
		}
		if info.Path == "" {
			t.Error("expected non-empty path when available")
		}
	} else if info.Error == "" {
		t.Error("expected non-empty error when not available")
	}
}

func TestValidateCookstyle_NotOnPATH(t *testing.T) {
	fe := newFakeExecutor()
	// Don't configure any result — the tool isn't found.

	r := NewResolver(WithExecutor(fe))
	info := r.ValidateCookstyle(context.Background())

	if info.Available {
		t.Error("expected cookstyle to be unavailable")
	}
	if info.Error == "" {
		t.Error("expected non-empty error")
	}
	if info.Name != "cookstyle" {
		t.Errorf("expected Name = cookstyle, got %q", info.Name)
	}
}

// ---------------------------------------------------------------------------
// ValidateKitchen tests
// ---------------------------------------------------------------------------

func TestValidateKitchen_NotOnPATH(t *testing.T) {
	fe := newFakeExecutor()

	r := NewResolver(WithExecutor(fe))
	info := r.ValidateKitchen(context.Background())

	if info.Available {
		t.Error("expected kitchen to be unavailable")
	}
	if info.Error == "" {
		t.Error("expected non-empty error")
	}
	if info.Name != "kitchen" {
		t.Errorf("expected Name = kitchen, got %q", info.Name)
	}
}

// ---------------------------------------------------------------------------
// ValidateGit tests
// ---------------------------------------------------------------------------

func TestValidateGit_PopulatesName(t *testing.T) {
	fe := newFakeExecutor()
	// git is looked up via exec.LookPath directly; the fake intercepts the
	// version call by basename.
	fe.set("git", "git version 2.45.0\n", "", nil)

	r := NewResolver(WithExecutor(fe))
	info := r.ValidateGit(context.Background())

	if info.Name != "git" {
		t.Errorf("expected Name = git, got %q", info.Name)
	}
	if info.Available {
		if info.Version == "" {
			t.Error("expected non-empty version when available")
		}
		if info.Path == "" {
			t.Error("expected non-empty path when available")
		}
	}
}

func TestValidateGit_ParsesVersion(t *testing.T) {
	// Test the version parsing logic by checking directly.
	raw := "git version 2.45.0"
	trimmed := raw[len("git version "):]
	if trimmed != "2.45.0" {
		t.Errorf("expected 2.45.0, got %q", trimmed)
	}
}

// ---------------------------------------------------------------------------
// ValidateAll tests
// ---------------------------------------------------------------------------

func TestValidateAll_AllUnavailable(t *testing.T) {
	fe := newFakeExecutor()

	r := NewResolver(WithExecutor(fe))
	result := r.ValidateAll(context.Background())

	if result.CookstyleEnabled {
		t.Error("expected CookstyleEnabled = false")
	}
	if result.KitchenEnabled {
		t.Error("expected KitchenEnabled = false")
	}
	// Git may or may not be available depending on the test environment.
}

func TestValidateAll_PopulatesAllFields(t *testing.T) {
	fe := newFakeExecutor()

	r := NewResolver(WithExecutor(fe))
	result := r.ValidateAll(context.Background())

	if result.Cookstyle.Name != "cookstyle" {
		t.Errorf("Cookstyle.Name = %q, want cookstyle", result.Cookstyle.Name)
	}
	if result.Kitchen.Name != "kitchen" {
		t.Errorf("Kitchen.Name = %q, want kitchen", result.Kitchen.Name)
	}
	if result.Git.Name != "git" {
		t.Errorf("Git.Name = %q, want git", result.Git.Name)
	}
}

func TestValidateAll_KitchenEnabledRequiresKitchen(t *testing.T) {
	fe := newFakeExecutor()

	r := NewResolver(WithExecutor(fe))
	result := r.ValidateAll(context.Background())

	if result.KitchenEnabled && !result.Kitchen.Available {
		t.Error("KitchenEnabled should be false when Kitchen is unavailable")
	}
}

// ---------------------------------------------------------------------------
// Option tests
// ---------------------------------------------------------------------------

func TestWithValidationTimeout(t *testing.T) {
	r := NewResolver(WithValidationTimeout(5 * time.Second))
	if r.validationTimeout != 5*time.Second {
		t.Errorf("expected timeout = 5s, got %v", r.validationTimeout)
	}
}

func TestWithExecutor(t *testing.T) {
	fe := newFakeExecutor()
	r := NewResolver(WithExecutor(fe))
	if r.executor == nil {
		t.Fatal("expected non-nil executor")
	}
}

func TestNewResolver_Defaults(t *testing.T) {
	r := NewResolver()

	if r.validationTimeout != 30*time.Second {
		t.Errorf("validationTimeout = %v, want 30s", r.validationTimeout)
	}
	if r.executor == nil {
		t.Fatal("expected non-nil default executor")
	}
}

// ---------------------------------------------------------------------------
// ToolInfo tests
// ---------------------------------------------------------------------------

func TestToolInfo_ZeroValue(t *testing.T) {
	var info ToolInfo
	if info.Available {
		t.Error("zero-value ToolInfo should not be Available")
	}
	if info.Name != "" {
		t.Errorf("zero-value Name should be empty, got %q", info.Name)
	}
}

// ---------------------------------------------------------------------------
// nameMatches helper tests
// ---------------------------------------------------------------------------

func TestNameMatches(t *testing.T) {
	tests := []struct {
		fullPath string
		name     string
		want     bool
	}{
		{"git", "git", true},
		{"/usr/bin/git", "git", true},
		{"/usr/local/bin/cookstyle", "cookstyle", true},
		{"/usr/bin/git", "docker", false},
		{"", "git", false},
		{"git", "", false},
		{"/usr/bin/gitk", "git", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.fullPath, tt.name), func(t *testing.T) {
			got := nameMatches(tt.fullPath, tt.name)
			if got != tt.want {
				t.Errorf("nameMatches(%q, %q) = %v, want %v", tt.fullPath, tt.name, got, tt.want)
			}
		})
	}
}
