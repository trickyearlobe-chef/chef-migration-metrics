// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package embedded resolves the paths to external tools (cookstyle, kitchen,
// git, docker) used by the analysis and data collection components. It checks
// the configured embedded bin directory first, then falls back to PATH lookup.
//
// Startup validation functions verify that each tool is installed and
// executable, returning version information and any errors encountered.
package embedded

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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

// Resolver locates external tools and validates their availability.
type Resolver struct {
	// embeddedBinDir is checked first for each tool. May be empty, in
	// which case only PATH is consulted.
	embeddedBinDir string

	// executor runs external commands. Defaults to defaultExecutor.
	executor CommandExecutor

	// validationTimeout bounds each startup validation command.
	validationTimeout time.Duration
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

// NewResolver creates a Resolver that checks embeddedBinDir first, then PATH.
func NewResolver(embeddedBinDir string, opts ...Option) *Resolver {
	r := &Resolver{
		embeddedBinDir:    embeddedBinDir,
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

// ResolvePath returns the absolute path to the named binary. It first checks
// embeddedBinDir/<name>, then falls back to exec.LookPath (PATH lookup).
// Returns ("", error) if the binary cannot be found anywhere.
func (r *Resolver) ResolvePath(name string) (string, error) {
	// Try embedded directory first.
	if r.embeddedBinDir != "" {
		candidate := r.embeddedBinDir + "/" + name
		// Use LookPath semantics: the candidate must be an absolute path
		// containing a slash, which it always will be.
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}

	// Fall back to PATH lookup.
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("embedded: %q not found in %q or PATH", name, r.embeddedBinDir)
	}
	return path, nil
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

	// git is never expected in the embedded directory — always use PATH.
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

// dockerInfoResponse is the minimal subset of `docker info --format json`
// that we parse for startup validation.
type dockerInfoResponse struct {
	ServerVersion string `json:"ServerVersion"`
}

// ValidateDocker checks that the Docker daemon is available and responsive.
func (r *Resolver) ValidateDocker(ctx context.Context) ToolInfo {
	info := ToolInfo{Name: "docker"}

	path, err := exec.LookPath("docker")
	if err != nil {
		info.Error = fmt.Sprintf("docker not found in PATH: %v", err)
		return info
	}
	info.Path = path

	vCtx, cancel := context.WithTimeout(ctx, r.validationTimeout)
	defer cancel()

	stdout, stderr, err := r.executor.Execute(vCtx, path, "info", "--format", "json")
	if err != nil {
		info.Error = fmt.Sprintf("docker info failed: %v; stderr: %s", err, strings.TrimSpace(stderr))
		return info
	}

	var di dockerInfoResponse
	if jsonErr := json.Unmarshal([]byte(stdout), &di); jsonErr != nil {
		info.Error = fmt.Sprintf("docker info returned invalid JSON: %v", jsonErr)
		return info
	}

	info.Version = di.ServerVersion
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
	Docker    ToolInfo

	// CookstyleEnabled is true if CookStyle scanning is available.
	CookstyleEnabled bool

	// KitchenEnabled is true if Test Kitchen testing is available.
	// Requires both kitchen and docker.
	KitchenEnabled bool
}

// ValidateAll runs startup validation for all external tools and returns
// a summary. The caller decides how to handle missing tools:
//   - Git unavailable → fatal (refuse to start)
//   - Cookstyle unavailable → disable CookStyle scanning
//   - Kitchen or Docker unavailable → disable Test Kitchen testing
//   - Both cookstyle and kitchen unavailable → warn, no compatibility testing
func (r *Resolver) ValidateAll(ctx context.Context) ValidationResult {
	result := ValidationResult{
		Git:       r.ValidateGit(ctx),
		Cookstyle: r.ValidateCookstyle(ctx),
		Kitchen:   r.ValidateKitchen(ctx),
		Docker:    r.ValidateDocker(ctx),
	}

	result.CookstyleEnabled = result.Cookstyle.Available
	result.KitchenEnabled = result.Kitchen.Available && result.Docker.Available

	return result
}
