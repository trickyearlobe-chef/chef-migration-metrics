// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// ---------------------------------------------------------------------------
// Kitchen executor interface (for testing)
// ---------------------------------------------------------------------------

// KitchenExecutor abstracts running the `kitchen` CLI so tests can inject a
// fake without touching the filesystem or requiring Docker / Vagrant.
type KitchenExecutor interface {
	// Run executes the kitchen binary with the given arguments in the
	// specified working directory. extraEnv is a list of additional
	// KEY=VALUE pairs (e.g. resolved credential env vars) to append to the
	// process environment. It returns stdout, stderr, the exit code, and
	// any execution error. A non-zero exit code is NOT returned as an error
	// when the process ran to completion — Test Kitchen exits non-zero when
	// converge or verify fails. An error is returned only for failures to
	// start the process, context cancellation, or signal-based termination.
	Run(ctx context.Context, dir string, extraEnv []string, args ...string) (stdout, stderr string, exitCode int, err error)
}

// ---------------------------------------------------------------------------
// Credential environment variable naming
// ---------------------------------------------------------------------------

// driverSecretEnvVar returns the environment variable name for a driver
// secret key. The key is uppercased with hyphens replaced by underscores,
// prefixed with CMM_TK_SECRET_.
func driverSecretEnvVar(key string) string {
	return "CMM_TK_SECRET_" + normalizeEnvVarSuffix(key)
}

// transportPasswordEnvVar returns the environment variable name for a
// platform's transport password. The platform name is normalised.
func transportPasswordEnvVar(kitchenName string) string {
	return "CMM_TK_TRANSPORT_" + normalizeEnvVarSuffix(kitchenName)
}

// transportKeyEnvVar returns the environment variable name for a
// platform's SSH key. The platform name is normalised.
func transportKeyEnvVar(kitchenName string) string {
	return "CMM_TK_KEY_" + normalizeEnvVarSuffix(kitchenName)
}

// normalizeEnvVarSuffix uppercases and replaces any character that is not
// alphanumeric or underscore with an underscore, for use in environment
// variable names.
func normalizeEnvVarSuffix(s string) string {
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

// ---------------------------------------------------------------------------
// .kitchen.yml detection and driver parsing
// ---------------------------------------------------------------------------

// findKitchenYML returns the path to the kitchen configuration file in the
// given directory. It checks for .kitchen.yml, .kitchen.yaml, kitchen.yml,
// and kitchen.yaml in that order. Returns empty string if none found.
func findKitchenYML(dir string) string {
	candidates := []string{
		".kitchen.yml",
		".kitchen.yaml",
		"kitchen.yml",
		"kitchen.yaml",
	}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

// detectDriver reads a .kitchen.yml file and extracts the driver name.
// Returns the driver name (e.g. "dokken", "vagrant") or empty string if
// the driver cannot be determined.
//
// This is a best-effort parser — it looks for the `driver:` top-level key
// and the `name:` sub-key. It does NOT use a full YAML parser because the
// only information we need is the driver name for overlay generation, and
// a simple line-scan avoids importing a YAML dependency into the analysis
// package.
func detectDriver(kitchenYMLPath string) string {
	data, err := os.ReadFile(kitchenYMLPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	inDriverBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Check for top-level `driver:` key (no leading whitespace).
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if strings.HasPrefix(trimmed, "driver:") {
				// Inline form: `driver: { name: dokken }` or `driver:`
				after := strings.TrimPrefix(trimmed, "driver:")
				after = strings.TrimSpace(after)
				if after != "" && after != "{" {
					// Could be `driver: dokken` (shorthand, not valid TK but handle it)
					return ""
				}
				inDriverBlock = true
				continue
			}
			inDriverBlock = false
			continue
		}

		if inDriverBlock {
			if strings.HasPrefix(trimmed, "name:") {
				val := strings.TrimPrefix(trimmed, "name:")
				val = strings.TrimSpace(val)
				val = strings.Trim(val, `"'`)
				return val
			}
			// If we hit another top-level key (no extra indentation relative
			// to driver), stop.
			indent := countLeadingSpaces(line)
			if indent == 0 {
				inDriverBlock = false
			}
		}
	}
	return ""
}

// countLeadingSpaces returns the number of leading space characters in s.
func countLeadingSpaces(s string) int {
	count := 0
	for _, ch := range s {
		switch ch {
		case ' ':
			count++
		case '\t':
			count += 2 // Treat tabs as 2 spaces.
		default:
			return count
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// YAML helpers (minimal — avoids importing a full YAML library)
// ---------------------------------------------------------------------------

// yamlScalar formats a string as a YAML scalar value. If the value contains
// characters that are special in YAML, it is double-quoted. Otherwise it is
// written bare.
func yamlScalar(v string) string {
	if v == "" {
		return `""`
	}
	// Only quote if the value contains characters that are genuinely
	// ambiguous or special as YAML *values*. Hyphens, dots, slashes, and
	// equals signs are safe inside scalar values — they only cause trouble
	// as leading characters in flow/block contexts that don't apply here.
	special := `:{}[]&*#?|!@` + "`" + `"' `
	for _, ch := range v {
		if strings.ContainsRune(special, ch) {
			return fmt.Sprintf("%q", v)
		}
	}
	return v
}

// writeAttributes writes a map of attributes as indented YAML. The indent
// parameter is the number of leading spaces for each key.
func writeAttributes(buf *bytes.Buffer, attrs map[string]interface{}, indent int) {
	prefix := strings.Repeat(" ", indent)
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := attrs[k]
		switch val := v.(type) {
		case map[string]interface{}:
			fmt.Fprintf(buf, "%s%s:\n", prefix, k)
			writeAttributes(buf, val, indent+2)
		default:
			fmt.Fprintf(buf, "%s%s: %s\n", prefix, k, yamlScalar(fmt.Sprintf("%v", val)))
		}
	}
}

// truncSHA returns the first 8 characters of a SHA string for log messages.
// If the SHA is shorter than 8 characters, it is returned as-is.
func truncSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// sanitiseKitchenEnv returns a copy of the given environment with Bundler-
// related variables removed. This prevents the cookbook's own Gemfile from
// interfering with the system-installed Test Kitchen binary.
//
// Variables removed:
//   - BUNDLE_GEMFILE — tells Bundler to use a specific Gemfile
//   - BUNDLE_BIN_PATH — overrides the Bundler binary location
//   - BUNDLE_PATH — overrides the gem installation path
//   - RUBYOPT — may contain "-rbundler/setup" injected by Bundler
//
// GEM_HOME and GEM_PATH are preserved so that the system kitchen gem
// (and its dependencies) remain discoverable.
func sanitiseKitchenEnv(environ []string) []string {
	drop := map[string]bool{
		"BUNDLE_GEMFILE":  true,
		"BUNDLE_BIN_PATH": true,
		"BUNDLE_PATH":     true,
		"RUBYOPT":         true,
	}

	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		if idx := strings.IndexByte(kv, '='); idx > 0 {
			key := kv[:idx]
			// Environment variable names are case-sensitive on Unix but
			// case-insensitive on Windows. Normalise to upper for the check.
			if drop[strings.Map(unicode.ToUpper, key)] {
				continue
			}
		}
		out = append(out, kv)
	}
	return out
}
