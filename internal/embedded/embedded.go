// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package embedded resolves the paths to external tools (cookstyle, kitchen,
// git) used by the analysis and data collection components by looking them up
// on PATH. cookstyle and kitchen are provided by Chef Workstation — they are
// not bundled with the application.
//
// Startup validation functions verify that each tool is installed and
// executable, returning version information and any errors encountered.
package embedded

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CommandExecutor abstracts os/exec for testability. The default
// implementation shells out to the real binary; tests can inject a fake.
type CommandExecutor interface {
	// Execute runs the named command with the given arguments and returns
	// combined stdout, stderr, and any error. The context controls timeout.
	Execute(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)
}

// defaultExecutor shells out to the real binary.
type defaultExecutor struct{}

func (defaultExecutor) Execute(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), err
}

// ToolInfo describes a resolved external tool.
type ToolInfo struct {
	// Name is the short name of the tool (e.g. "cookstyle", "kitchen").
	Name string

	// Path is the absolute filesystem path to the resolved binary.
	Path string

	// Version is the version string reported by the tool, or empty if
	// version detection failed.
	Version string

	// Available is true if the tool was found and responded to its
	// version/info command with exit code 0.
	Available bool

	// Error describes why the tool is unavailable, or is empty on success.
	Error string
}

// Resolver locates external tools on PATH and validates their availability.
type Resolver struct {
	// executor runs external commands. Defaults to defaultExecutor.
	executor CommandExecutor

	// validationTimeout bounds each startup validation command.
	validationTimeout time.Duration

	// binDir is where the Chef tools are, when they are not somewhere a shell
	// would find them. Empty means PATH alone, which is the ordinary case:
	// Chef Workstation puts them there. See WithBinDir.
	binDir string
}

// Option configures a Resolver.
type Option func(*Resolver)

// WithExecutor overrides the command executor (useful for testing).
func WithExecutor(e CommandExecutor) Option {
	return func(r *Resolver) { r.executor = e }
}

// WithValidationTimeout overrides the default 30-second timeout for each
// startup validation command.
func WithValidationTimeout(d time.Duration) Option {
	return func(r *Resolver) { r.validationTimeout = d }
}

// WithBinDir names the directory holding the Chef tools, for deployments where
// they are not on PATH — a service started without the profile that sets it, or
// a Chef Workstation installed somewhere unusual. Empty means PATH alone.
//
// Only cookstyle and kitchen. Git is resolved from PATH regardless: it is not
// a Chef tool and this setting is about where Chef Workstation put its own.
func WithBinDir(dir string) Option {
	return func(r *Resolver) { r.binDir = strings.TrimSpace(dir) }
}

// NewResolver creates a Resolver that resolves tools from PATH.
func NewResolver(opts ...Option) *Resolver {
	r := &Resolver{
		executor:          defaultExecutor{},
		validationTimeout: 30 * time.Second,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// ---------------------------------------------------------------------------
// Path resolution
// ---------------------------------------------------------------------------

// ResolvePath returns the absolute path to the named binary: the configured
// directory first, then PATH.
//
// The directory is a preference rather than a replacement. A deployment that
// set one and then had the tools appear on PATH keeps working, and so does one
// whose directory holds only some of them — otherwise a single wrong setting
// switches scanning off, and the message says nothing about the setting.
func (r *Resolver) ResolvePath(name string) (string, error) {
	if r.binDir != "" {
		// Runnable, not merely present. A stray file of the right name would
		// otherwise report the tool as found, and every scan would fail later
		// somewhere that never mentions this directory.
		candidate := filepath.Join(r.binDir, name)
		if info, err := os.Stat(candidate); err == nil &&
			info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}

	path, err := exec.LookPath(name)
	if err == nil {
		return path, nil
	}
	if r.binDir != "" {
		return "", fmt.Errorf("embedded: %q is not in %s and not in PATH", name, r.binDir)
	}
	return "", fmt.Errorf("embedded: %q not found in PATH", name)
}

// ---------------------------------------------------------------------------
// Startup validation
// ---------------------------------------------------------------------------

// ValidateCookstyle checks that cookstyle is available and returns its
// version. CookStyle exits 0 for --version even though it uses --format json
// for scan output; the --version flag produces a plain version string.
func (r *Resolver) ValidateCookstyle(ctx context.Context) ToolInfo {
	info := ToolInfo{Name: "cookstyle"}

	path, err := r.ResolvePath("cookstyle")
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Path = path

	vCtx, cancel := context.WithTimeout(ctx, r.validationTimeout)
	defer cancel()

	stdout, stderr, err := r.executor.Execute(vCtx, path, "--version")
	if err != nil {
		info.Error = fmt.Sprintf("cookstyle --version failed: %v; stderr: %s", err, strings.TrimSpace(stderr))
		return info
	}

	info.Version = strings.TrimSpace(stdout)
	info.Available = true
	return info
}

// ValidateKitchen checks that Test Kitchen is available and returns its
// version.
func (r *Resolver) ValidateKitchen(ctx context.Context) ToolInfo {
	info := ToolInfo{Name: "kitchen"}

	path, err := r.ResolvePath("kitchen")
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Path = path

	vCtx, cancel := context.WithTimeout(ctx, r.validationTimeout)
	defer cancel()

	stdout, stderr, err := r.executor.Execute(vCtx, path, "version")
	if err != nil {
		info.Error = fmt.Sprintf("kitchen version failed: %v; stderr: %s", err, strings.TrimSpace(stderr))
		return info
	}

	info.Version = strings.TrimSpace(stdout)
	info.Available = true
	return info
}

// ValidateGit checks that git is available and returns its version.
// Git is mandatory — the caller should treat a failure as fatal.
func (r *Resolver) ValidateGit(ctx context.Context) ToolInfo {
	info := ToolInfo{Name: "git"}

	path, err := exec.LookPath("git")
	if err != nil {
		info.Error = fmt.Sprintf("git not found in PATH: %v", err)
		return info
	}
	info.Path = path

	vCtx, cancel := context.WithTimeout(ctx, r.validationTimeout)
	defer cancel()

	stdout, stderr, err := r.executor.Execute(vCtx, path, "version")
	if err != nil {
		info.Error = fmt.Sprintf("git version failed: %v; stderr: %s", err, strings.TrimSpace(stderr))
		return info
	}

	// Output format: "git version X.Y.Z"
	raw := strings.TrimSpace(stdout)
	info.Version = strings.TrimPrefix(raw, "git version ")
	info.Available = true
	return info
}

// ---------------------------------------------------------------------------
// Bulk validation
// ---------------------------------------------------------------------------

// ValidationResult holds the outcome of validating all required tools.
type ValidationResult struct {
	Cookstyle ToolInfo
	Kitchen   ToolInfo
	Git       ToolInfo

	// CookstyleEnabled is true if CookStyle scanning is available.
	CookstyleEnabled bool

	// KitchenEnabled is true if the kitchen binary is available.
	// Docker is not required — VM-based drivers (vcenter, proxmox) manage
	// their own infrastructure.
	KitchenEnabled bool
}

// ValidateAll runs startup validation for all external tools and returns
// a summary. The caller decides how to handle missing tools:
//   - Git unavailable → fatal (refuse to start)
//   - Cookstyle unavailable → disable CookStyle scanning
//   - Kitchen unavailable → disable Test Kitchen testing
//   - Both cookstyle and kitchen unavailable → warn, no compatibility testing
func (r *Resolver) ValidateAll(ctx context.Context) ValidationResult {
	result := ValidationResult{
		Git:       r.ValidateGit(ctx),
		Cookstyle: r.ValidateCookstyle(ctx),
		Kitchen:   r.ValidateKitchen(ctx),
	}

	result.CookstyleEnabled = result.Cookstyle.Available
	result.KitchenEnabled = result.Kitchen.Available

	return result
}
