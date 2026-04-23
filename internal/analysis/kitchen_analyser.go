// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// KitchenConfig represents a fully parsed and merged Test Kitchen configuration.
type KitchenConfig struct {
	DriverName          string            `json:"driver_name,omitempty"`
	DriverSettings      map[string]any    `json:"driver_settings,omitempty"`
	ProvisionerName     string            `json:"provisioner_name,omitempty"`
	ProvisionerSettings map[string]any    `json:"provisioner_settings,omitempty"`
	Platforms           []KitchenPlatform `json:"platforms"`
	Suites              []KitchenSuite    `json:"suites"`
	TransportType       string            `json:"transport_type,omitempty"`
	TransportSettings   map[string]any    `json:"transport_settings,omitempty"`
}

// KitchenPlatform represents a single platform entry from a kitchen config.
type KitchenPlatform struct {
	Name               string         `json:"name"`
	NormalisedName     string         `json:"normalised_name"`
	OSFamily           string         `json:"os_family"`
	OSVersion          string         `json:"os_version,omitempty"`
	Extensions         map[string]any `json:"extensions,omitempty"`
	DriverOverrides    map[string]any `json:"driver_overrides,omitempty"`
	TransportOverrides map[string]any `json:"transport_overrides,omitempty"`
}

// KitchenSuite represents a single suite entry from a kitchen config.
type KitchenSuite struct {
	Name           string   `json:"name"`
	RunList        []string `json:"run_list,omitempty"`
	Excludes       []string `json:"excludes,omitempty"`
	Includes       []string `json:"includes,omitempty"`
	HasInspecTests bool     `json:"has_inspec_tests"`
}

// KitchenAnalysisEntry is the complete analysis result for a single repo.
type KitchenAnalysisEntry struct {
	KitchenFiles      []string      `json:"kitchen_files"`
	HasLocalOverride  bool          `json:"has_local_override"`
	LocalOverrideKeys []string      `json:"local_override_keys,omitempty"`
	VariantFiles      []string      `json:"variant_files,omitempty"`
	Config            KitchenConfig `json:"config"`
	ErrorMessage      string        `json:"error_message,omitempty"`
}

// ---------------------------------------------------------------------------
// YAML Parsing
// ---------------------------------------------------------------------------

// ParseKitchenYAML parses raw YAML bytes into a generic map.
func ParseKitchenYAML(data []byte) (map[string]any, error) {
	var result map[string]any
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("analysis: parse kitchen yaml: %w", err)
	}
	if result == nil {
		return map[string]any{}, nil
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Config Merging (Test Kitchen semantics)
// ---------------------------------------------------------------------------

// MergeKitchenConfigs merges override on top of base using TK merge semantics.
// Maps are deep-merged recursively; arrays and scalars from override replace base.
func MergeKitchenConfigs(base, override map[string]any) map[string]any {
	if base == nil && override == nil {
		return map[string]any{}
	}
	if base == nil {
		return copyMap(override)
	}
	if override == nil {
		return copyMap(base)
	}

	result := copyMap(base)
	for k, ov := range override {
		bv, exists := result[k]
		if !exists {
			result[k] = ov
			continue
		}
		bMap, bIsMap := toStringMap(bv)
		oMap, oIsMap := toStringMap(ov)
		if bIsMap && oIsMap {
			result[k] = MergeKitchenConfigs(bMap, oMap)
			continue
		}
		// Arrays and scalars: override replaces base.
		result[k] = ov
	}
	return result
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func toStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprint(k)] = val
		}
		return out, true
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// Config Extraction
// ---------------------------------------------------------------------------

// ExtractKitchenConfig extracts structured fields from a raw parsed YAML map.
func ExtractKitchenConfig(raw map[string]any) KitchenConfig {
	var cfg KitchenConfig

	// Driver
	if drv, ok := toStringMap(raw["driver"]); ok {
		cfg.DriverName = stringVal(drv, "name")
		settings := make(map[string]any, len(drv))
		for k, v := range drv {
			if k != "name" {
				settings[k] = v
			}
		}
		if len(settings) > 0 {
			cfg.DriverSettings = settings
		}
	}
	if cfg.DriverName == "" {
		if dp, ok := raw["driver_plugin"].(string); ok {
			cfg.DriverName = dp
		}
	}

	// Provisioner
	if prov, ok := toStringMap(raw["provisioner"]); ok {
		cfg.ProvisionerName = stringVal(prov, "name")
		settings := make(map[string]any, len(prov))
		for k, v := range prov {
			if k != "name" {
				settings[k] = v
			}
		}
		if len(settings) > 0 {
			cfg.ProvisionerSettings = settings
		}
	}

	// Transport
	if tr, ok := toStringMap(raw["transport"]); ok {
		cfg.TransportType = stringVal(tr, "name")
		settings := make(map[string]any, len(tr))
		for k, v := range tr {
			if k != "name" {
				settings[k] = v
			}
		}
		if len(settings) > 0 {
			cfg.TransportSettings = settings
		}
	}

	// Platforms
	if platforms, ok := raw["platforms"].([]any); ok {
		for _, p := range platforms {
			pm, ok := toStringMap(p)
			if !ok {
				continue
			}
			plat := extractPlatform(pm)
			cfg.Platforms = append(cfg.Platforms, plat)
		}
	}

	// Suites
	if suites, ok := raw["suites"].([]any); ok {
		for _, s := range suites {
			sm, ok := toStringMap(s)
			if !ok {
				continue
			}
			cfg.Suites = append(cfg.Suites, extractSuite(sm))
		}
	}

	// Infer transport if not set explicitly
	if cfg.TransportType == "" {
		cfg.TransportType = DetectTransportType(raw)
	}

	if cfg.Platforms == nil {
		cfg.Platforms = []KitchenPlatform{}
	}
	if cfg.Suites == nil {
		cfg.Suites = []KitchenSuite{}
	}

	return cfg
}

func extractPlatform(pm map[string]any) KitchenPlatform {
	name := stringVal(pm, "name")
	norm, family, ver := NormalisePlatformName(name)

	plat := KitchenPlatform{
		Name:           name,
		NormalisedName: norm,
		OSFamily:       family,
		OSVersion:      ver,
	}

	// Driver overrides
	for _, key := range []string{"driver", "driver_config"} {
		if d, ok := toStringMap(pm[key]); ok {
			plat.DriverOverrides = d
			break
		}
	}

	// Transport overrides
	if t, ok := toStringMap(pm["transport"]); ok {
		plat.TransportOverrides = t
	}

	// Extensions (x-* keys)
	for k, v := range pm {
		if strings.HasPrefix(k, "x-") {
			if plat.Extensions == nil {
				plat.Extensions = make(map[string]any)
			}
			plat.Extensions[k] = v
		}
	}

	return plat
}

func extractSuite(sm map[string]any) KitchenSuite {
	suite := KitchenSuite{
		Name: stringVal(sm, "name"),
	}
	if rl, ok := sm["run_list"].([]any); ok {
		for _, item := range rl {
			if s, ok := item.(string); ok {
				suite.RunList = append(suite.RunList, s)
			}
		}
	}
	if ex, ok := sm["excludes"].([]any); ok {
		for _, item := range ex {
			if s, ok := item.(string); ok {
				suite.Excludes = append(suite.Excludes, s)
			}
		}
	}
	if inc, ok := sm["includes"].([]any); ok {
		for _, item := range inc {
			if s, ok := item.(string); ok {
				suite.Includes = append(suite.Includes, s)
			}
		}
	}
	return suite
}

func stringVal(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Platform Normaliser
// ---------------------------------------------------------------------------

var (
	knownSuffixes = []string{
		"-chef16", "-chef15", "-chef14", "-chef13", "-chef12",
		"-x86_64", "-amd64",
		"-stable", "-testing",
		"-small", "-medium", "-large",
		"-vanilla",
	}

	versionReplacements = map[string]string{
		"2k12": "2012",
		"2k16": "2016",
		"2k19": "2019",
		"2k22": "2022",
	}

	rhelFamilyPrefixes = []string{"rhel", "centos", "redhat", "oracle", "rocky", "alma", "amazon"}
	windowsPrefixes    = []string{"windows", "win"}
	debianPrefixes     = []string{"ubuntu", "debian"}
	susePrefixes       = []string{"sles", "suse", "opensuse"}

	// Matches a numeric segment optionally containing dots (e.g. "22.04", "7", "2012").
	versionRe = regexp.MustCompile(`(\d+(?:\.\d+)*)`)

	// For detecting windows shorthand prefixes.
	winPrefixRe = regexp.MustCompile(`^win(\d|-)`)

	// OS prefixes that need a hyphen inserted if missing before a digit.
	hyphenPrefixes = []string{"centos", "rhel", "ubuntu", "debian", "sles", "opensuse", "windows"}
)

// NormalisePlatformName normalises a Test Kitchen platform name and extracts
// the OS family and version.
func NormalisePlatformName(name string) (normalised string, osFamily string, osVersion string) {
	s := strings.ToLower(strings.TrimSpace(name))

	// Strip known suffixes.
	for _, suffix := range knownSuffixes {
		for strings.HasSuffix(s, suffix) {
			s = s[:len(s)-len(suffix)]
		}
	}

	// Normalise version formats (2k12 → 2012 etc.) — must happen before prefix normalisation.
	for old, repl := range versionReplacements {
		s = strings.ReplaceAll(s, old, repl)
	}

	// Normalise Windows prefixes.
	if strings.HasPrefix(s, "windows2k") {
		s = "windows-" + s[len("windows2k"):]
	} else if strings.HasPrefix(s, "win2k") {
		s = "windows-" + s[len("win2k"):]
	} else if winPrefixRe.MatchString(s) && !strings.HasPrefix(s, "windows") {
		s = "windows-" + s[len("win"):]
	}

	// Insert hyphen between OS prefix and digit if missing.
	for _, prefix := range hyphenPrefixes {
		if strings.HasPrefix(s, prefix) && len(s) > len(prefix) {
			next := s[len(prefix)]
			if next != '-' && (next >= '0' && next <= '9') {
				s = prefix + "-" + s[len(prefix):]
			}
		}
	}

	// Special handling for Ubuntu 4-digit versions (e.g. ubuntu-2204 → ubuntu-22.04).
	if strings.HasPrefix(s, "ubuntu-") {
		rest := s[len("ubuntu-"):]
		if len(rest) == 4 && isAllDigits(rest) {
			s = "ubuntu-" + rest[:2] + "." + rest[2:]
		}
	}

	// Detect OS family.
	osFamily = detectOSFamily(s)

	// Extract version.
	osVersion = extractVersion(s)

	// Build normalised name.
	if osFamily == "other" {
		normalised = "other-" + s
	} else {
		// Use the prefix from s up to the version.
		osPart := extractOSPrefix(s)
		if osVersion != "" {
			normalised = osPart + "-" + osVersion
		} else {
			normalised = "other-" + s
		}
	}

	return normalised, osFamily, osVersion
}

func detectOSFamily(s string) string {
	for _, p := range rhelFamilyPrefixes {
		if strings.HasPrefix(s, p) {
			return "rhel"
		}
	}
	for _, p := range windowsPrefixes {
		if strings.HasPrefix(s, p) {
			return "windows"
		}
	}
	for _, p := range debianPrefixes {
		if strings.HasPrefix(s, p) {
			return "debian"
		}
	}
	for _, p := range susePrefixes {
		if strings.HasPrefix(s, p) {
			return "suse"
		}
	}
	return "other"
}

func extractVersion(s string) string {
	// Find the first numeric segment.
	loc := versionRe.FindStringIndex(s)
	if loc == nil {
		return ""
	}
	return versionRe.FindString(s)
}

func extractOSPrefix(s string) string {
	loc := versionRe.FindStringIndex(s)
	if loc == nil {
		return s
	}
	prefix := s[:loc[0]]
	prefix = strings.TrimRight(prefix, "-")
	return prefix
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// ---------------------------------------------------------------------------
// Transport Detection
// ---------------------------------------------------------------------------

// DetectTransportType detects the transport type from a raw kitchen config.
func DetectTransportType(raw map[string]any) string {
	if tr, ok := toStringMap(raw["transport"]); ok {
		if name := stringVal(tr, "name"); name != "" {
			switch strings.ToLower(name) {
			case "winrm":
				return "winrm"
			case "ssh":
				return "ssh"
			case "dokken":
				return "dokken"
			default:
				return name
			}
		}
	}

	// Check if any platform has winrm transport overrides.
	if platforms, ok := raw["platforms"].([]any); ok {
		for _, p := range platforms {
			pm, ok := toStringMap(p)
			if !ok {
				continue
			}
			if tr, ok := toStringMap(pm["transport"]); ok {
				if n := stringVal(tr, "name"); strings.EqualFold(n, "winrm") {
					return "mixed"
				}
			}
		}
	}

	return "ssh"
}

// ---------------------------------------------------------------------------
// File Discovery
// ---------------------------------------------------------------------------

// DiscoverKitchenFiles scans a directory for kitchen config files.
func DiscoverKitchenFiles(dir string) (primary string, localOverride string, variants []string) {
	primaryCandidates := []string{
		".kitchen.yml",
		".kitchen.yaml",
		"kitchen.yml",
		"kitchen.yaml",
	}
	for _, name := range primaryCandidates {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			primary = path
			break
		}
	}

	localCandidates := []string{
		".kitchen.local.yml",
		".kitchen.local.yaml",
	}
	for _, name := range localCandidates {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			localOverride = path
			break
		}
	}

	// Discover variant files.
	for _, ext := range []string{"yml", "yaml"} {
		pattern := filepath.Join(dir, ".kitchen.*."+ext)
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			if m == primary || m == localOverride {
				continue
			}
			// Skip .kitchen.local.* files (already handled).
			base := filepath.Base(m)
			if strings.HasPrefix(base, ".kitchen.local.") {
				continue
			}
			variants = append(variants, m)
		}
	}

	return primary, localOverride, variants
}

// ---------------------------------------------------------------------------
// InSpec Test Detection
// ---------------------------------------------------------------------------

// CheckInspecTests checks if InSpec tests exist for a suite.
func CheckInspecTests(dir string, suiteName string) bool {
	testDirs := []string{
		filepath.Join(dir, "test", "integration", suiteName),
		filepath.Join(dir, "test", "smoke", suiteName),
	}
	for _, d := range testDirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		if len(entries) > 0 {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Full Directory Analysis
// ---------------------------------------------------------------------------

// AnalyseKitchenDir orchestrates the full analysis of a single repo directory.
func AnalyseKitchenDir(dir string) KitchenAnalysisEntry {
	primary, localOverride, variants := DiscoverKitchenFiles(dir)

	entry := KitchenAnalysisEntry{
		VariantFiles: variants,
	}

	if primary == "" {
		entry.Config = KitchenConfig{
			Platforms: []KitchenPlatform{},
			Suites:    []KitchenSuite{},
		}
		entry.ErrorMessage = "analysis: no kitchen config file found"
		return entry
	}

	entry.KitchenFiles = append(entry.KitchenFiles, primary)

	data, err := os.ReadFile(primary)
	if err != nil {
		entry.ErrorMessage = fmt.Sprintf("analysis: read primary config: %v", err)
		entry.Config = KitchenConfig{Platforms: []KitchenPlatform{}, Suites: []KitchenSuite{}}
		return entry
	}

	baseMap, err := ParseKitchenYAML(data)
	if err != nil {
		entry.ErrorMessage = fmt.Sprintf("analysis: parse primary config: %v", err)
		entry.Config = KitchenConfig{Platforms: []KitchenPlatform{}, Suites: []KitchenSuite{}}
		return entry
	}

	merged := baseMap

	if localOverride != "" {
		entry.HasLocalOverride = true
		entry.KitchenFiles = append(entry.KitchenFiles, localOverride)

		overrideData, err := os.ReadFile(localOverride)
		if err != nil {
			entry.ErrorMessage = fmt.Sprintf("analysis: read local override: %v", err)
			entry.Config = ExtractKitchenConfig(merged)
			return entry
		}

		overrideMap, err := ParseKitchenYAML(overrideData)
		if err != nil {
			entry.ErrorMessage = fmt.Sprintf("analysis: parse local override: %v", err)
			entry.Config = ExtractKitchenConfig(merged)
			return entry
		}

		// Record which top-level keys the override touches.
		for k := range overrideMap {
			entry.LocalOverrideKeys = append(entry.LocalOverrideKeys, k)
		}

		merged = MergeKitchenConfigs(baseMap, overrideMap)
	}

	cfg := ExtractKitchenConfig(merged)

	// Check for InSpec tests per suite.
	for i := range cfg.Suites {
		cfg.Suites[i].HasInspecTests = CheckInspecTests(dir, cfg.Suites[i].Name)
	}

	entry.Config = cfg
	return entry
}
