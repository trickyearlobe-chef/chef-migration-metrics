// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package batch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// KitchenExec runs the kitchen binary in a directory.
// Same interface shape as analysis.KitchenExecutor, redefined to avoid import coupling.
type KitchenExec interface {
	Run(ctx context.Context, dir string, extraEnv []string, args ...string) (stdout, stderr string, exitCode int, err error)
}

// CredentialProvider resolves credentials for kitchen runs.
type CredentialProvider interface {
	ResolveCredentials(ctx context.Context) (envVars []string, cleanup func(), err error)
}

// OverlayConfig holds the configuration for generating .kitchen.local.yml overlays.
type OverlayConfig struct {
	DriverName     string
	DriverSettings map[string]interface{}
	DriverSecrets  map[string]string
	PlatformMap    []PlatformEntry
	Images         []ImageConfig
	ChefLicenseKey string
}

// PlatformEntry maps a kitchen platform name to an image.
type PlatformEntry struct {
	KitchenName string
	Image       string
}

// ImageConfig holds image details for overlay generation.
type ImageConfig struct {
	Name             string
	ID               string
	DriverSettings   map[string]interface{}
	Transport        *TransportConfig
	ChefDownloadURLs map[string]string
}

// TransportConfig holds transport settings for an image.
type TransportConfig struct {
	Username           string
	PasswordCredential string
	SSHKeyCredential   string
}

// KitchenRunnerConfig bundles constructor parameters.
type KitchenRunnerConfig struct {
	Executor     KitchenExec
	CredProvider CredentialProvider
	Logger       ExecutorLogger
	RepoDir      func(repoName string) string
	Timeout      time.Duration
	Overlay      OverlayConfig
}

// KitchenRunner implements InstanceRunner by shelling out to kitchen.
type KitchenRunner struct {
	executor      KitchenExec
	credProvider  CredentialProvider
	logger        ExecutorLogger
	repoDir       func(repoName string) string
	timeout       time.Duration
	overlayConfig OverlayConfig
}

// NewKitchenRunner creates a KitchenRunner from the given config.
func NewKitchenRunner(cfg KitchenRunnerConfig) *KitchenRunner {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	return &KitchenRunner{
		executor:      cfg.Executor,
		credProvider:  cfg.CredProvider,
		logger:        cfg.Logger,
		repoDir:       cfg.RepoDir,
		timeout:       timeout,
		overlayConfig: cfg.Overlay,
	}
}

// RunInstance executes a single kitchen instance (converge, verify, destroy).
func (r *KitchenRunner) RunInstance(ctx context.Context, req RunInstanceRequest) RunInstanceResult {
	startedAt := time.Now().UTC()
	result := RunInstanceResult{
		StartedAt:  &startedAt,
		DriverUsed: r.overlayConfig.DriverName,
	}

	dir := r.repoDir(req.GitRepoName)
	if dir == "" {
		result.ErrorMessage = fmt.Sprintf("no repo directory for %s", req.GitRepoName)
		now := time.Now().UTC()
		result.CompletedAt = &now
		return result
	}

	// Backup existing .kitchen.local.yml.
	hadOverride, backupErr := backupLocalOverride(dir)
	if backupErr != nil {
		result.ErrorMessage = fmt.Sprintf("backup .kitchen.local.yml: %v", backupErr)
		now := time.Now().UTC()
		result.CompletedAt = &now
		return result
	}
	if hadOverride {
		r.logger.Warn(fmt.Sprintf("displaced existing .kitchen.local.yml in %s", dir))
	}

	// Generate and write overlay.
	overlayContent := r.buildInstanceOverlay(req.TargetChefVersion)
	if overlayContent != "" {
		overlayPath := filepath.Join(dir, ".kitchen.local.yml")
		if err := os.WriteFile(overlayPath, []byte(overlayContent), 0644); err != nil {
			restoreLocalOverride(dir, hadOverride)
			result.ErrorMessage = fmt.Sprintf("writing .kitchen.local.yml: %v", err)
			now := time.Now().UTC()
			result.CompletedAt = &now
			return result
		}
		result.TemplateUsed = "generated-overlay"
	}

	// Defer restore of original override.
	defer restoreLocalOverride(dir, hadOverride)

	// Resolve credentials.
	var credEnv []string
	if r.credProvider != nil {
		envVars, cleanup, credErr := r.credProvider.ResolveCredentials(ctx)
		if credErr != nil {
			r.logger.Warn(fmt.Sprintf("credential resolution failed: %v", credErr))
		} else {
			credEnv = envVars
			if cleanup != nil {
				defer cleanup()
			}
		}
	}

	// Build instance regex: kitchen concatenates suite-platform with hyphens.
	instanceRegex := fmt.Sprintf("%s-%s", req.SuiteName, req.PlatformName)

	// Run converge.
	convergeCtx, convergeCancel := context.WithTimeout(ctx, r.timeout)
	cStdout, cStderr, cExitCode, cErr := r.executor.Run(
		convergeCtx, dir, credEnv,
		"converge", instanceRegex, "--concurrency=1", "--log-level=info",
	)
	convergeCancel()

	result.ConvergeOutput = combineOutput(cStdout, cStderr)

	convergePassed := cExitCode == 0 && cErr == nil
	if convergeCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		f := false
		result.ConvergePassed = &f
		f2 := false
		result.TestsPassed = &f2
	} else if cErr != nil {
		f := false
		result.ConvergePassed = &f
		result.ErrorMessage = fmt.Sprintf("converge execution error: %v", cErr)
		convergePassed = false
	} else {
		result.ConvergePassed = &convergePassed
	}

	// Run verify only if converge passed.
	if convergePassed && result.ConvergePassed != nil && *result.ConvergePassed {
		verifyCtx, verifyCancel := context.WithTimeout(ctx, r.timeout)
		vStdout, vStderr, vExitCode, vErr := r.executor.Run(
			verifyCtx, dir, credEnv,
			"verify", instanceRegex, "--concurrency=1", "--log-level=info",
		)
		verifyCancel()

		result.VerifyOutput = combineOutput(vStdout, vStderr)

		if verifyCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			f := false
			result.TestsPassed = &f
		} else if vErr != nil {
			f := false
			result.TestsPassed = &f
		} else {
			tp := vExitCode == 0
			result.TestsPassed = &tp
		}
	}

	// Destroy always, with a fresh background context.
	destroyCtx, destroyCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	dStdout, dStderr, _, _ := r.executor.Run(
		destroyCtx, dir, credEnv,
		"destroy", instanceRegex, "--concurrency=1",
	)
	destroyCancel()
	result.DestroyOutput = combineOutput(dStdout, dStderr)

	completedAt := time.Now().UTC()
	result.CompletedAt = &completedAt
	duration := int(completedAt.Sub(startedAt).Seconds())
	result.DurationSeconds = &duration

	return result
}

// buildInstanceOverlay generates a .kitchen.local.yml overlay string.
func (r *KitchenRunner) buildInstanceOverlay(targetVersion string) string {
	cfg := r.overlayConfig
	var buf bytes.Buffer

	buf.WriteString("# .kitchen.local.yml — generated by chef-migration-metrics batch runner\n")
	buf.WriteString("# DO NOT EDIT — this file is overwritten on each test run\n")

	hasContent := false

	// Driver block.
	if cfg.DriverName != "" {
		buf.WriteString("\ndriver:\n")
		fmt.Fprintf(&buf, "  name: %s\n", cfg.DriverName)
		for _, k := range sortedOverlayKeys(cfg.DriverSettings) {
			writeOverlaySetting(&buf, k, cfg.DriverSettings[k], 2)
		}
		for _, k := range sortedOverlayStringKeys(cfg.DriverSecrets) {
			envName := "CMM_TK_SECRET_" + normalizeOverlayEnvSuffix(k)
			fmt.Fprintf(&buf, "  %s: <%%= ENV['%s'] %%>\n", k, envName)
		}
		hasContent = true
	}

	// Provisioner block.
	if targetVersion != "" {
		buf.WriteString("\nprovisioner:\n")
		major := chefMajorVersion(targetVersion)
		if major >= 19 {
			buf.WriteString("  name: chef_ice\n")
		}
		fmt.Fprintf(&buf, "  product_version: %q\n", targetVersion)
		if cfg.ChefLicenseKey != "" {
			buf.WriteString("  chef_license_key: <%= ENV['CMM_TK_CHEF_LICENSE_KEY'] %>\n")
		}
		buf.WriteString("  chef_license: accept\n")
		hasContent = true
	}

	// Platforms block.
	if len(cfg.PlatformMap) > 0 {
		imageIndex := buildOverlayImageIndex(cfg.Images)
		buf.WriteString("\nplatforms:\n")
		for _, entry := range cfg.PlatformMap {
			img, ok := imageIndex[entry.Image]
			if !ok {
				continue
			}
			fmt.Fprintf(&buf, "  - name: %s\n", entry.KitchenName)
			buf.WriteString("    driver:\n")
			fmt.Fprintf(&buf, "      image: %s\n", overlayYAMLScalar(img.ID))
			for _, k := range sortedOverlayKeys(img.DriverSettings) {
				writeOverlaySetting(&buf, k, img.DriverSettings[k], 6)
			}

			if targetVersion != "" {
				if url, ok := img.ChefDownloadURLs[targetVersion]; ok && url != "" {
					buf.WriteString("    provisioner:\n")
					fmt.Fprintf(&buf, "      download_url: %s\n", overlayYAMLScalar(url))
				}
			}

			if img.Transport != nil {
				buf.WriteString("    transport:\n")
				if img.Transport.Username != "" {
					fmt.Fprintf(&buf, "      username: %s\n", overlayYAMLScalar(img.Transport.Username))
				}
				if img.Transport.PasswordCredential != "" {
					envName := "CMM_TK_TRANSPORT_" + normalizeOverlayEnvSuffix(img.Name)
					fmt.Fprintf(&buf, "      password: <%%= ENV['%s'] %%>\n", envName)
				}
				if img.Transport.SSHKeyCredential != "" {
					envName := "CMM_TK_KEY_" + normalizeOverlayEnvSuffix(img.Name)
					fmt.Fprintf(&buf, "      ssh_key: <%%= ENV['%s'] %%>\n", envName)
				}
			}
		}
		hasContent = true
	}

	if !hasContent {
		return ""
	}
	return buf.String()
}

// backupLocalOverride renames .kitchen.local.yml to .kitchen.local.yml.bak if it exists.
func backupLocalOverride(dir string) (bool, error) {
	src := filepath.Join(dir, ".kitchen.local.yml")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("checking .kitchen.local.yml: %w", err)
	}
	dst := filepath.Join(dir, ".kitchen.local.yml.bak")
	if err := os.Rename(src, dst); err != nil {
		return false, fmt.Errorf("backing up .kitchen.local.yml: %w", err)
	}
	return true, nil
}

// restoreLocalOverride restores .kitchen.local.yml from .bak, removing the generated overlay.
func restoreLocalOverride(dir string, hadOverride bool) {
	overlayPath := filepath.Join(dir, ".kitchen.local.yml")
	_ = os.Remove(overlayPath)
	if hadOverride {
		bakPath := filepath.Join(dir, ".kitchen.local.yml.bak")
		_ = os.Rename(bakPath, overlayPath)
	}
}

// combineOutput joins stdout and stderr, trimming whitespace.
func combineOutput(stdout, stderr string) string {
	stdout = strings.TrimSpace(stdout)
	stderr = strings.TrimSpace(stderr)
	if stdout == "" {
		return stderr
	}
	if stderr == "" {
		return stdout
	}
	return stdout + "\n" + stderr
}

// chefMajorVersion extracts the major version number from a version string.
func chefMajorVersion(version string) int {
	if idx := strings.IndexByte(version, '.'); idx > 0 {
		n, _ := strconv.Atoi(version[:idx])
		return n
	}
	parts := strings.SplitN(version, ".", 2)
	if len(parts) == 0 {
		return 0
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	return n
}

// buildOverlayImageIndex creates a lookup map from image name to ImageConfig.
func buildOverlayImageIndex(images []ImageConfig) map[string]ImageConfig {
	idx := make(map[string]ImageConfig, len(images))
	for _, img := range images {
		idx[img.Name] = img
	}
	return idx
}

// overlayYAMLScalar quotes a YAML scalar value if it contains special characters.
func overlayYAMLScalar(v string) string {
	if v == "" {
		return `""`
	}
	special := `:{}[]&*#?|!@` + "`" + `"' `
	for _, ch := range v {
		if strings.ContainsRune(special, ch) {
			return fmt.Sprintf("%q", v)
		}
	}
	return v
}

// normalizeOverlayEnvSuffix uppercases and replaces non-alphanumeric characters
// (except underscore) with underscores, for use in environment variable names.
func normalizeOverlayEnvSuffix(s string) string {
	s = strings.ToUpper(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, ch := range s {
		switch {
		case ch >= 'A' && ch <= 'Z':
			b.WriteRune(ch)
		case ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		case ch == '_':
			b.WriteRune(ch)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// writeOverlaySetting writes a single driver setting key/value pair.
func writeOverlaySetting(buf *bytes.Buffer, key string, value any, indent int) {
	prefix := strings.Repeat(" ", indent)
	switch v := value.(type) {
	case map[string]any:
		fmt.Fprintf(buf, "%s%s:\n", prefix, key)
		writeOverlayAttributes(buf, v, indent+2)
	default:
		fmt.Fprintf(buf, "%s%s: %s\n", prefix, key, overlayYAMLScalar(fmt.Sprintf("%v", v)))
	}
}

// writeOverlayAttributes writes a map of attributes as indented YAML.
func writeOverlayAttributes(buf *bytes.Buffer, attrs map[string]interface{}, indent int) {
	prefix := strings.Repeat(" ", indent)
	keys := sortedOverlayKeys(attrs)
	for _, k := range keys {
		v := attrs[k]
		switch val := v.(type) {
		case map[string]interface{}:
			fmt.Fprintf(buf, "%s%s:\n", prefix, k)
			writeOverlayAttributes(buf, val, indent+2)
		default:
			fmt.Fprintf(buf, "%s%s: %s\n", prefix, k, overlayYAMLScalar(fmt.Sprintf("%v", val)))
		}
	}
}

// sortedOverlayKeys returns the keys of a map[string]any in sorted order.
func sortedOverlayKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedOverlayStringKeys returns the keys of a map[string]string in sorted order.
func sortedOverlayStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
