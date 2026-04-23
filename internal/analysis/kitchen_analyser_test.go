// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// ---------------------------------------------------------------------------
// Parse Tests
// ---------------------------------------------------------------------------

func TestParseKitchenYAML_Valid(t *testing.T) {
	data := []byte("driver:\n  name: vagrant\nplatforms:\n  - name: ubuntu-22.04\n")
	m, err := ParseKitchenYAML(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	drv, ok := m["driver"]
	if !ok {
		t.Fatal("expected 'driver' key")
	}
	dm, ok := drv.(map[string]any)
	if !ok {
		t.Fatalf("expected driver to be map, got %T", drv)
	}
	if dm["name"] != "vagrant" {
		t.Errorf("expected driver name %q, got %q", "vagrant", dm["name"])
	}
}

func TestParseKitchenYAML_Empty(t *testing.T) {
	m, err := ParseKitchenYAML([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestParseKitchenYAML_Invalid(t *testing.T) {
	data := []byte(":\n  :\n  - [invalid\n")
	_, err := ParseKitchenYAML(data)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// ---------------------------------------------------------------------------
// Merge Tests
// ---------------------------------------------------------------------------

func TestMergeKitchenConfigs_DeepMerge(t *testing.T) {
	base := map[string]any{
		"driver": map[string]any{
			"name":     "vagrant",
			"provider": "virtualbox",
		},
	}
	override := map[string]any{
		"driver": map[string]any{
			"name": "dokken",
		},
	}
	result := MergeKitchenConfigs(base, override)
	drv := result["driver"].(map[string]any)
	if drv["name"] != "dokken" {
		t.Errorf("expected driver name %q, got %q", "dokken", drv["name"])
	}
	if drv["provider"] != "virtualbox" {
		t.Errorf("expected provider %q kept from base, got %q", "virtualbox", drv["provider"])
	}
}

func TestMergeKitchenConfigs_ArrayReplace(t *testing.T) {
	base := map[string]any{
		"platforms": []any{
			map[string]any{"name": "ubuntu-20.04"},
			map[string]any{"name": "centos-7"},
		},
	}
	override := map[string]any{
		"platforms": []any{
			map[string]any{"name": "ubuntu-22.04"},
		},
	}
	result := MergeKitchenConfigs(base, override)
	platforms := result["platforms"].([]any)
	if len(platforms) != 1 {
		t.Fatalf("expected 1 platform (replaced), got %d", len(platforms))
	}
	pm := platforms[0].(map[string]any)
	if pm["name"] != "ubuntu-22.04" {
		t.Errorf("expected platform %q, got %q", "ubuntu-22.04", pm["name"])
	}
}

func TestMergeKitchenConfigs_ScalarReplace(t *testing.T) {
	base := map[string]any{"provisioner": "chef_solo"}
	override := map[string]any{"provisioner": "chef_zero"}
	result := MergeKitchenConfigs(base, override)
	if result["provisioner"] != "chef_zero" {
		t.Errorf("expected %q, got %q", "chef_zero", result["provisioner"])
	}
}

func TestMergeKitchenConfigs_EmptyOverride(t *testing.T) {
	base := map[string]any{"driver": map[string]any{"name": "vagrant"}}
	result := MergeKitchenConfigs(base, map[string]any{})
	drv := result["driver"].(map[string]any)
	if drv["name"] != "vagrant" {
		t.Errorf("expected base preserved, got %v", result)
	}
}

func TestMergeKitchenConfigs_EmptyBase(t *testing.T) {
	override := map[string]any{"driver": map[string]any{"name": "dokken"}}
	result := MergeKitchenConfigs(map[string]any{}, override)
	drv := result["driver"].(map[string]any)
	if drv["name"] != "dokken" {
		t.Errorf("expected override returned, got %v", result)
	}
}

func TestMergeKitchenConfigs_NilMaps(t *testing.T) {
	result := MergeKitchenConfigs(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}

	result2 := MergeKitchenConfigs(nil, map[string]any{"a": 1})
	if result2["a"] != 1 {
		t.Errorf("expected override value, got %v", result2)
	}

	result3 := MergeKitchenConfigs(map[string]any{"b": 2}, nil)
	if result3["b"] != 2 {
		t.Errorf("expected base value, got %v", result3)
	}
}

// ---------------------------------------------------------------------------
// Extract Tests
// ---------------------------------------------------------------------------

func TestExtractKitchenConfig_FullConfig(t *testing.T) {
	raw := map[string]any{
		"driver": map[string]any{
			"name":     "vagrant",
			"provider": "virtualbox",
		},
		"provisioner": map[string]any{
			"name":                 "chef_zero",
			"require_chef_omnibus": false,
		},
		"transport": map[string]any{
			"name":    "ssh",
			"ssh_key": "/tmp/key",
		},
		"platforms": []any{
			map[string]any{"name": "ubuntu-22.04"},
			map[string]any{"name": "centos-7"},
		},
		"suites": []any{
			map[string]any{
				"name":     "default",
				"run_list": []any{"recipe[my_cookbook::default]"},
			},
		},
	}
	cfg := ExtractKitchenConfig(raw)

	if cfg.DriverName != "vagrant" {
		t.Errorf("expected driver %q, got %q", "vagrant", cfg.DriverName)
	}
	if cfg.DriverSettings["provider"] != "virtualbox" {
		t.Errorf("expected provider in settings, got %v", cfg.DriverSettings)
	}
	if cfg.ProvisionerName != "chef_zero" {
		t.Errorf("expected provisioner %q, got %q", "chef_zero", cfg.ProvisionerName)
	}
	if cfg.TransportType != "ssh" {
		t.Errorf("expected transport %q, got %q", "ssh", cfg.TransportType)
	}
	if len(cfg.Platforms) != 2 {
		t.Fatalf("expected 2 platforms, got %d", len(cfg.Platforms))
	}
	if cfg.Platforms[0].NormalisedName != "ubuntu-22.04" {
		t.Errorf("expected normalised name %q, got %q", "ubuntu-22.04", cfg.Platforms[0].NormalisedName)
	}
	if len(cfg.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(cfg.Suites))
	}
	if cfg.Suites[0].RunList[0] != "recipe[my_cookbook::default]" {
		t.Errorf("unexpected run_list: %v", cfg.Suites[0].RunList)
	}
}

func TestExtractKitchenConfig_MinimalConfig(t *testing.T) {
	raw := map[string]any{
		"driver": map[string]any{"name": "dokken"},
		"platforms": []any{
			map[string]any{"name": "centos-7"},
		},
	}
	cfg := ExtractKitchenConfig(raw)
	if cfg.DriverName != "dokken" {
		t.Errorf("expected driver %q, got %q", "dokken", cfg.DriverName)
	}
	if len(cfg.Platforms) != 1 {
		t.Fatalf("expected 1 platform, got %d", len(cfg.Platforms))
	}
	if len(cfg.Suites) != 0 {
		t.Errorf("expected 0 suites, got %d", len(cfg.Suites))
	}
}

func TestExtractKitchenConfig_EmptyConfig(t *testing.T) {
	cfg := ExtractKitchenConfig(map[string]any{})
	if cfg.DriverName != "" {
		t.Errorf("expected empty driver, got %q", cfg.DriverName)
	}
	if len(cfg.Platforms) != 0 {
		t.Errorf("expected 0 platforms, got %d", len(cfg.Platforms))
	}
	if len(cfg.Suites) != 0 {
		t.Errorf("expected 0 suites, got %d", len(cfg.Suites))
	}
}

func TestExtractKitchenConfig_PlatformExtensions(t *testing.T) {
	raw := map[string]any{
		"platforms": []any{
			map[string]any{
				"name":              "rhel7",
				"x-custom-box_type": "stable",
				"x-custom-size":     "small",
			},
		},
	}
	cfg := ExtractKitchenConfig(raw)
	if len(cfg.Platforms) != 1 {
		t.Fatalf("expected 1 platform, got %d", len(cfg.Platforms))
	}
	ext := cfg.Platforms[0].Extensions
	if ext == nil {
		t.Fatal("expected extensions map, got nil")
	}
	if ext["x-custom-box_type"] != "stable" {
		t.Errorf("expected extension %q, got %v", "stable", ext["x-custom-box_type"])
	}
	if ext["x-custom-size"] != "small" {
		t.Errorf("expected extension %q, got %v", "small", ext["x-custom-size"])
	}
}

func TestExtractKitchenConfig_PerPlatformDriver(t *testing.T) {
	raw := map[string]any{
		"platforms": []any{
			map[string]any{
				"name": "ubuntu-22.04",
				"driver": map[string]any{
					"name":     "ec2",
					"image_id": "ami-12345",
				},
			},
		},
	}
	cfg := ExtractKitchenConfig(raw)
	if cfg.Platforms[0].DriverOverrides == nil {
		t.Fatal("expected driver overrides, got nil")
	}
	if cfg.Platforms[0].DriverOverrides["name"] != "ec2" {
		t.Errorf("expected driver override name %q, got %v", "ec2", cfg.Platforms[0].DriverOverrides["name"])
	}
	if cfg.Platforms[0].DriverOverrides["image_id"] != "ami-12345" {
		t.Errorf("expected image_id, got %v", cfg.Platforms[0].DriverOverrides["image_id"])
	}
}

func TestExtractKitchenConfig_PerPlatformTransport(t *testing.T) {
	raw := map[string]any{
		"platforms": []any{
			map[string]any{
				"name": "windows-2019",
				"transport": map[string]any{
					"name":     "winrm",
					"username": "Administrator",
				},
			},
		},
	}
	cfg := ExtractKitchenConfig(raw)
	if cfg.Platforms[0].TransportOverrides == nil {
		t.Fatal("expected transport overrides, got nil")
	}
	if cfg.Platforms[0].TransportOverrides["name"] != "winrm" {
		t.Errorf("expected winrm transport, got %v", cfg.Platforms[0].TransportOverrides["name"])
	}
}

func TestExtractKitchenConfig_Suites(t *testing.T) {
	raw := map[string]any{
		"suites": []any{
			map[string]any{
				"name":     "default",
				"run_list": []any{"recipe[my_cookbook::default]"},
				"excludes": []any{"windows-2019"},
				"includes": []any{"centos-7"},
			},
			map[string]any{
				"name":     "integration",
				"run_list": []any{"recipe[my_cookbook::integration]"},
			},
		},
	}
	cfg := ExtractKitchenConfig(raw)
	if len(cfg.Suites) != 2 {
		t.Fatalf("expected 2 suites, got %d", len(cfg.Suites))
	}
	s := cfg.Suites[0]
	if s.Name != "default" {
		t.Errorf("expected name %q, got %q", "default", s.Name)
	}
	if len(s.RunList) != 1 || s.RunList[0] != "recipe[my_cookbook::default]" {
		t.Errorf("unexpected run_list: %v", s.RunList)
	}
	if len(s.Excludes) != 1 || s.Excludes[0] != "windows-2019" {
		t.Errorf("unexpected excludes: %v", s.Excludes)
	}
	if len(s.Includes) != 1 || s.Includes[0] != "centos-7" {
		t.Errorf("unexpected includes: %v", s.Includes)
	}
}

// ---------------------------------------------------------------------------
// Normaliser Tests
// ---------------------------------------------------------------------------

func TestNormalisePlatformName(t *testing.T) {
	tests := []struct {
		input      string
		normalised string
		osFamily   string
		osVersion  string
	}{
		{"rhel7-chef16", "rhel-7", "rhel", "7"},
		{"RHEL-8", "rhel-8", "rhel", "8"},
		{"centos-7", "centos-7", "rhel", "7"},
		{"centos7", "centos-7", "rhel", "7"},
		{"windows2k12-vanilla", "windows-2012", "windows", "2012"},
		{"win2k16-chef16", "windows-2016", "windows", "2016"},
		{"windows-2019", "windows-2019", "windows", "2019"},
		{"ubuntu-22.04", "ubuntu-22.04", "debian", "22.04"},
		{"ubuntu2204-stable", "ubuntu-22.04", "debian", "22.04"},
		{"debian-11", "debian-11", "debian", "11"},
		{"sles-15", "sles-15", "suse", "15"},
		{"rocky-9", "rocky-9", "rhel", "9"},
		{"alma-8", "alma-8", "rhel", "8"},
		{"amazon-2", "amazon-2", "rhel", "2"},
		{"custom-platform", "other-custom-platform", "other", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			norm, family, ver := NormalisePlatformName(tt.input)
			if norm != tt.normalised {
				t.Errorf("normalised: expected %q, got %q", tt.normalised, norm)
			}
			if family != tt.osFamily {
				t.Errorf("osFamily: expected %q, got %q", tt.osFamily, family)
			}
			if ver != tt.osVersion {
				t.Errorf("osVersion: expected %q, got %q", tt.osVersion, ver)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Transport Detection Tests
// ---------------------------------------------------------------------------

func TestDetectTransportType(t *testing.T) {
	tests := []struct {
		name     string
		raw      map[string]any
		expected string
	}{
		{
			name: "winrm transport",
			raw: map[string]any{
				"transport": map[string]any{"name": "winrm"},
			},
			expected: "winrm",
		},
		{
			name: "ssh transport",
			raw: map[string]any{
				"transport": map[string]any{"name": "ssh"},
			},
			expected: "ssh",
		},
		{
			name:     "no transport defaults to ssh",
			raw:      map[string]any{},
			expected: "ssh",
		},
		{
			name: "dokken transport",
			raw: map[string]any{
				"transport": map[string]any{"name": "dokken"},
			},
			expected: "dokken",
		},
		{
			name: "mixed from platform overrides",
			raw: map[string]any{
				"platforms": []any{
					map[string]any{
						"name": "windows-2019",
						"transport": map[string]any{
							"name": "winrm",
						},
					},
				},
			},
			expected: "mixed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectTransportType(tt.raw)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Discovery Tests
// ---------------------------------------------------------------------------

func TestDiscoverKitchenFiles(t *testing.T) {
	t.Run("primary only", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".kitchen.yml"), "driver:\n  name: vagrant\n")

		primary, local, variants := DiscoverKitchenFiles(dir)
		if filepath.Base(primary) != ".kitchen.yml" {
			t.Errorf("expected .kitchen.yml, got %q", primary)
		}
		if local != "" {
			t.Errorf("expected no local override, got %q", local)
		}
		if len(variants) != 0 {
			t.Errorf("expected no variants, got %v", variants)
		}
	})

	t.Run("primary and local override", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".kitchen.yml"), "driver:\n  name: vagrant\n")
		writeFile(t, filepath.Join(dir, ".kitchen.local.yml"), "driver:\n  name: dokken\n")

		primary, local, variants := DiscoverKitchenFiles(dir)
		if filepath.Base(primary) != ".kitchen.yml" {
			t.Errorf("expected .kitchen.yml, got %q", primary)
		}
		if filepath.Base(local) != ".kitchen.local.yml" {
			t.Errorf("expected .kitchen.local.yml, got %q", local)
		}
		if len(variants) != 0 {
			t.Errorf("expected no variants, got %v", variants)
		}
	})

	t.Run("primary local and variant", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".kitchen.yml"), "driver:\n  name: vagrant\n")
		writeFile(t, filepath.Join(dir, ".kitchen.local.yml"), "driver:\n  name: dokken\n")
		writeFile(t, filepath.Join(dir, ".kitchen.ci.yml"), "driver:\n  name: ec2\n")

		primary, local, variants := DiscoverKitchenFiles(dir)
		if filepath.Base(primary) != ".kitchen.yml" {
			t.Errorf("expected .kitchen.yml, got %q", primary)
		}
		if filepath.Base(local) != ".kitchen.local.yml" {
			t.Errorf("expected .kitchen.local.yml, got %q", local)
		}
		if len(variants) != 1 {
			t.Fatalf("expected 1 variant, got %d", len(variants))
		}
		if filepath.Base(variants[0]) != ".kitchen.ci.yml" {
			t.Errorf("expected .kitchen.ci.yml variant, got %q", variants[0])
		}
	})

	t.Run("no kitchen files", func(t *testing.T) {
		dir := t.TempDir()
		primary, local, variants := DiscoverKitchenFiles(dir)
		if primary != "" {
			t.Errorf("expected no primary, got %q", primary)
		}
		if local != "" {
			t.Errorf("expected no local, got %q", local)
		}
		if len(variants) != 0 {
			t.Errorf("expected no variants, got %v", variants)
		}
	})

	t.Run("kitchen yml without dot prefix", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "kitchen.yml"), "driver:\n  name: vagrant\n")

		primary, _, _ := DiscoverKitchenFiles(dir)
		if filepath.Base(primary) != "kitchen.yml" {
			t.Errorf("expected kitchen.yml, got %q", primary)
		}
	})
}

// ---------------------------------------------------------------------------
// Integration Tests
// ---------------------------------------------------------------------------

func TestAnalyseKitchenDir(t *testing.T) {
	dir := t.TempDir()

	kitchenYML := `driver:
  name: vagrant
  provider: virtualbox

provisioner:
  name: chef_zero
  require_chef_omnibus: false

transport:
  ssh_key: ./.ssh/testkitchen-pem

platforms:
  - name: rhel7-chef16
    x-custom-box_type: stable
    x-custom-size: small
  - name: windows2k12-vanilla
    transport:
      name: winrm
      username: Administrator
      password: P@ssw0rd

suites:
  - name: default
    run_list:
      - recipe[my_cookbook::default]
`
	writeFile(t, filepath.Join(dir, ".kitchen.yml"), kitchenYML)

	// Create InSpec test dir for the "default" suite.
	inspecDir := filepath.Join(dir, "test", "integration", "default")
	if err := os.MkdirAll(inspecDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(inspecDir, "default_test.rb"), "describe true do\n  it { should eq true }\nend\n")

	entry := AnalyseKitchenDir(dir)

	if entry.ErrorMessage != "" {
		t.Fatalf("unexpected error: %s", entry.ErrorMessage)
	}
	if entry.Config.DriverName != "vagrant" {
		t.Errorf("expected driver %q, got %q", "vagrant", entry.Config.DriverName)
	}
	if entry.Config.DriverSettings["provider"] != "virtualbox" {
		t.Errorf("expected provider in settings, got %v", entry.Config.DriverSettings)
	}
	if entry.Config.ProvisionerName != "chef_zero" {
		t.Errorf("expected provisioner %q, got %q", "chef_zero", entry.Config.ProvisionerName)
	}
	if len(entry.Config.Platforms) != 2 {
		t.Fatalf("expected 2 platforms, got %d", len(entry.Config.Platforms))
	}

	// First platform: rhel7-chef16 → rhel-7.
	p0 := entry.Config.Platforms[0]
	if p0.Name != "rhel7-chef16" {
		t.Errorf("expected raw name %q, got %q", "rhel7-chef16", p0.Name)
	}
	if p0.NormalisedName != "rhel-7" {
		t.Errorf("expected normalised %q, got %q", "rhel-7", p0.NormalisedName)
	}
	if p0.OSFamily != "rhel" {
		t.Errorf("expected os_family %q, got %q", "rhel", p0.OSFamily)
	}
	if p0.Extensions["x-custom-box_type"] != "stable" {
		t.Errorf("expected extension value %q, got %v", "stable", p0.Extensions["x-custom-box_type"])
	}

	// Second platform: windows2k12-vanilla → windows-2012.
	p1 := entry.Config.Platforms[1]
	if p1.NormalisedName != "windows-2012" {
		t.Errorf("expected normalised %q, got %q", "windows-2012", p1.NormalisedName)
	}
	if p1.OSFamily != "windows" {
		t.Errorf("expected os_family %q, got %q", "windows", p1.OSFamily)
	}
	if p1.TransportOverrides == nil {
		t.Fatal("expected transport overrides for windows platform")
	}
	if p1.TransportOverrides["name"] != "winrm" {
		t.Errorf("expected winrm transport override, got %v", p1.TransportOverrides["name"])
	}

	// Transport type should be "mixed" (default ssh + platform winrm override).
	if entry.Config.TransportType != "mixed" {
		t.Errorf("expected transport type %q, got %q", "mixed", entry.Config.TransportType)
	}

	// Suite checks.
	if len(entry.Config.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(entry.Config.Suites))
	}
	if entry.Config.Suites[0].Name != "default" {
		t.Errorf("expected suite name %q, got %q", "default", entry.Config.Suites[0].Name)
	}
	if !entry.Config.Suites[0].HasInspecTests {
		t.Error("expected HasInspecTests true for default suite")
	}

	if entry.HasLocalOverride {
		t.Error("expected no local override")
	}
}

func TestAnalyseKitchenDir_WithLocalOverride(t *testing.T) {
	dir := t.TempDir()

	kitchenYML := `driver:
  name: vagrant
  provider: virtualbox

provisioner:
  name: chef_zero

platforms:
  - name: ubuntu-22.04

suites:
  - name: default
    run_list:
      - recipe[my_cookbook::default]
`
	localYML := `driver:
  name: dokken
`
	writeFile(t, filepath.Join(dir, ".kitchen.yml"), kitchenYML)
	writeFile(t, filepath.Join(dir, ".kitchen.local.yml"), localYML)

	entry := AnalyseKitchenDir(dir)

	if entry.ErrorMessage != "" {
		t.Fatalf("unexpected error: %s", entry.ErrorMessage)
	}
	if !entry.HasLocalOverride {
		t.Error("expected HasLocalOverride true")
	}
	if entry.Config.DriverName != "dokken" {
		t.Errorf("expected merged driver %q, got %q", "dokken", entry.Config.DriverName)
	}
	if entry.Config.DriverSettings["provider"] != "virtualbox" {
		t.Errorf("expected provider retained from base, got %v", entry.Config.DriverSettings)
	}

	// Check local override keys.
	sort.Strings(entry.LocalOverrideKeys)
	if len(entry.LocalOverrideKeys) != 1 || entry.LocalOverrideKeys[0] != "driver" {
		t.Errorf("expected local override keys [driver], got %v", entry.LocalOverrideKeys)
	}
}

func TestAnalyseKitchenDir_NoKitchenFile(t *testing.T) {
	dir := t.TempDir()
	entry := AnalyseKitchenDir(dir)
	if entry.ErrorMessage == "" {
		t.Error("expected error message for missing kitchen file")
	}
}

// ---------------------------------------------------------------------------
// InSpec Test Detection
// ---------------------------------------------------------------------------

func TestCheckInspecTests(t *testing.T) {
	dir := t.TempDir()

	// No test dirs yet.
	if CheckInspecTests(dir, "default") {
		t.Error("expected false when no test dirs exist")
	}

	// Create integration test dir with a file.
	inspecDir := filepath.Join(dir, "test", "integration", "default")
	if err := os.MkdirAll(inspecDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(inspecDir, "default_test.rb"), "")
	if !CheckInspecTests(dir, "default") {
		t.Error("expected true when integration test dir has files")
	}

	// Smoke dir.
	dir2 := t.TempDir()
	smokeDir := filepath.Join(dir2, "test", "smoke", "mytest")
	if err := os.MkdirAll(smokeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(smokeDir, "smoke_test.rb"), "")
	if !CheckInspecTests(dir2, "mytest") {
		t.Error("expected true when smoke test dir has files")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
