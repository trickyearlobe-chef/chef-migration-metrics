// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package gitkitchen

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// KitchenExecutor runs kitchen CLI commands.
type KitchenExecutor interface {
	Run(ctx context.Context, dir string, extraEnv []string, args ...string) (stdout, stderr string, exitCode int, err error)
}

// CredentialResolver resolves kitchen credentials to environment variables.
type CredentialResolver interface {
	ResolveKitchenCredentials(ctx context.Context, tkConfig config.TestKitchenConfig) (envVars map[string][]byte, cleanup func(), err error)
}

// ResultStore abstracts the datastore methods needed by kitchen execution.
type ResultStore interface {
	UpsertGitKitchenResult(ctx context.Context, p datastore.UpsertGitKitchenResultParams) (datastore.GitKitchenResult, error)
}

// RunInstanceParams holds input for a single git kitchen instance run.
type RunInstanceParams struct {
	GitRepoName       string
	GitRepoURL        string
	RepoDir           string // path to the local git clone (read-only)
	InstanceName      string // e.g. "default-ubuntu-2204"
	SuiteName         string
	PlatformName      string
	TargetChefVersion string
	CommitSHA         string
}

// RunInstanceResult holds the output of a single instance run.
type RunInstanceResult struct {
	Passed          *bool
	TimedOut        bool
	NetworkTimeout  bool
	Output          string // combined stdout+stderr
	DurationSeconds *int
	ErrorMessage    string
	DriverUsed      string
}

// RunInstance executes a single Test Kitchen instance for a git-cloned cookbook.
func RunInstance(ctx context.Context, params RunInstanceParams, tkConfig config.TestKitchenConfig,
	executor KitchenExecutor, credResolver CredentialResolver) RunInstanceResult {

	start := time.Now()

	// Create isolated workspace
	workDir, err := copyRepoToWorkspace(params.RepoDir)
	if err != nil {
		return RunInstanceResult{
			ErrorMessage: fmt.Sprintf("gitkitchen: creating workspace: %v", err),
			DriverUsed:   tkConfig.Driver,
		}
	}
	defer os.RemoveAll(workDir)

	// Back up existing .kitchen.local.yml if present
	localOverlayPath := filepath.Join(workDir, ".kitchen.local.yml")
	if _, statErr := os.Stat(localOverlayPath); statErr == nil {
		bakPath := localOverlayPath + ".bak"
		if renameErr := os.Rename(localOverlayPath, bakPath); renameErr != nil {
			return RunInstanceResult{
				ErrorMessage: fmt.Sprintf("gitkitchen: backing up existing overlay: %v", renameErr),
				DriverUsed:   tkConfig.Driver,
			}
		}
	}

	// Generate overlay. Only read the cookbook's own pre_destroy hooks when at
	// least one image opts in to the IP-release hook — otherwise the overlay
	// injects no pre_destroy phase and there is nothing to compose with.
	var existingPreDestroy []any
	if anyImageReleasesIP(tkConfig) {
		existingPreDestroy = readExistingPreDestroy(workDir)
	}
	overlay, err := generateOverlay(tkConfig, OverlayParams{
		PlatformName:       params.PlatformName,
		TargetChefVersion:  params.TargetChefVersion,
		CookbookName:       params.GitRepoName,
		SuiteName:          params.SuiteName,
		ExistingPreDestroy: existingPreDestroy,
		SetupScripts:       discoverSetupScripts(workDir, params.PlatformName, tkConfig),
	})
	if err != nil {
		return RunInstanceResult{
			ErrorMessage: fmt.Sprintf("gitkitchen: generating overlay: %v", err),
			DriverUsed:   tkConfig.Driver,
		}
	}
	if overlay != "" {
		if writeErr := os.WriteFile(localOverlayPath, []byte(overlay), 0644); writeErr != nil {
			return RunInstanceResult{
				ErrorMessage: fmt.Sprintf("gitkitchen: writing overlay: %v", writeErr),
				DriverUsed:   tkConfig.Driver,
			}
		}
	}

	// Resolve credentials
	credEnvVars, cleanup, err := credResolver.ResolveKitchenCredentials(ctx, tkConfig)
	if err != nil {
		return RunInstanceResult{
			ErrorMessage: fmt.Sprintf("gitkitchen: resolving credentials: %v", err),
			DriverUsed:   tkConfig.Driver,
		}
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Build env var slice from credentials
	extraEnv := buildEnvSlice(credEnvVars)

	// Run kitchen test
	stdout, stderr, exitCode, execErr := executor.Run(ctx, workDir, extraEnv,
		"test", params.InstanceName, "--destroy=always", "--no-color")

	duration := int(time.Since(start).Seconds())
	output := fmt.Sprintf("[workdir: %s] [started: %s]\n", workDir, start.UTC().Format(time.RFC3339)) + combineOutput(stdout, stderr)

	result := RunInstanceResult{
		Output:          output,
		DurationSeconds: intPtr(duration),
		DriverUsed:      tkConfig.Driver,
	}

	if execErr != nil {
		result.ErrorMessage = fmt.Sprintf("gitkitchen: executor error: %v", execErr)
		if errors.Is(execErr, context.DeadlineExceeded) {
			result.TimedOut = true
			if !hasConvergeActivity(output) {
				result.NetworkTimeout = true
				result.ErrorMessage = "probable DHCP/network timeout"
			}
		}
		// Passed remains nil — unknown state
	} else {
		passed := exitCode == 0
		result.Passed = boolPtr(passed)
	}

	return result
}

// anyImageReleasesIP reports whether any configured image opts in to the
// IP-release pre_destroy hook.
func anyImageReleasesIP(tkConfig config.TestKitchenConfig) bool {
	for _, img := range tkConfig.Images {
		if img.ReleaseIPOnDestroy {
			return true
		}
	}
	return false
}

// readExistingPreDestroy returns the cookbook's own lifecycle.pre_destroy
// entries from its untouched .kitchen.yml, or nil when absent or unparseable.
// Best-effort: any error yields nil so the IP-release hook is still injected
// (without composition) rather than failing overlay generation.
func readExistingPreDestroy(workDir string) []any {
	primary, _, _ := analysis.DiscoverKitchenFiles(workDir)
	if primary == "" {
		return nil
	}
	data, err := os.ReadFile(primary)
	if err != nil {
		return nil
	}
	raw, err := analysis.ParseKitchenYAML(data)
	if err != nil {
		return nil
	}
	lifecycle, ok := raw["lifecycle"].(map[string]any)
	if !ok {
		return nil
	}
	entries, ok := lifecycle["pre_destroy"].([]any)
	if !ok {
		return nil
	}
	return entries
}

// discoverSetupScripts globs the OS-family setup-script patterns against the
// workspace, reads each matched script body, and returns them sorted by
// repo-relative path. The OS family is derived from the resolved platform
// (linux scripts run over SSH, windows over WinRM). Returns nil when no
// patterns are configured for the family or nothing matches. A script that
// cannot be read is skipped — discovery never aborts overlay generation; the
// hook's own non-zero exit is what fails a run.
func discoverSetupScripts(workDir, platformName string, tkConfig config.TestKitchenConfig) []SetupScript {
	if tkConfig.SetupScripts == nil {
		return nil
	}
	_, osFamily, _ := analysis.NormalisePlatformName(platformName)
	patterns := tkConfig.SetupScripts.Linux
	if osFamily == "windows" {
		patterns = tkConfig.SetupScripts.Windows
	}
	if len(patterns) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var scripts []SetupScript
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(workDir, pattern))
		if err != nil {
			continue // malformed pattern — already surfaced by config validation
		}
		for _, abs := range matches {
			rel, relErr := filepath.Rel(workDir, abs)
			if relErr != nil {
				rel = abs
			}
			if seen[rel] {
				continue
			}
			info, statErr := os.Stat(abs)
			if statErr != nil || info.IsDir() {
				continue
			}
			body, readErr := os.ReadFile(abs)
			if readErr != nil {
				continue
			}
			seen[rel] = true
			scripts = append(scripts, SetupScript{Path: rel, Body: string(body)})
		}
	}
	sort.Slice(scripts, func(i, j int) bool { return scripts[i].Path < scripts[j].Path })
	return scripts
}

// copyRepoToWorkspace creates a temporary directory and copies the repo contents into it.
func copyRepoToWorkspace(repoDir string) (string, error) {
	workDir, err := os.MkdirTemp("", "cmm-gitkitchen-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	// Use cp -a to preserve structure
	cmd := exec.Command("cp", "-a", repoDir+"/.", workDir)
	if out, cpErr := cmd.CombinedOutput(); cpErr != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("copying repo: %s: %w", strings.TrimSpace(string(out)), cpErr)
	}

	return workDir, nil
}

// buildEnvSlice converts a credential map to KEY=VALUE strings.
func buildEnvSlice(creds map[string][]byte) []string {
	if len(creds) == 0 {
		return nil
	}
	keys := make([]string, 0, len(creds))
	for k := range creds {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(creds))
	for _, k := range keys {
		env = append(env, fmt.Sprintf("%s=%s", k, string(creds[k])))
	}
	return env
}

// combineOutput merges stdout and stderr into a single string.
func combineOutput(stdout, stderr string) string {
	var parts []string
	if stdout != "" {
		parts = append(parts, stdout)
	}
	if stderr != "" {
		parts = append(parts, stderr)
	}
	return strings.Join(parts, "")
}

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

// convergePatterns matches lines that indicate Chef converge activity started.
var convergePatterns = regexp.MustCompile(
	`(?i)(Converging \d+ resource|` +
		`\* \S+\[.*\] action |` +
		`Recipe: |` +
		`Starting Chef (Infra )?Client|` +
		`resolving cookbooks)`)

// hasConvergeActivity returns true if the combined output contains evidence
// that Chef actually began converging. Used to distinguish network/DHCP
// timeouts (VM never booted) from real test failures.
func hasConvergeActivity(output string) bool {
	return convergePatterns.MatchString(output)
}
