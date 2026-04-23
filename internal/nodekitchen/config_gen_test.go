// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package nodekitchen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

func TestGenerateKitchenYML_Basic(t *testing.T) {
	cfg := KitchenGenConfig{
		NodeName:          "web01",
		PlatformName:      "centos",
		PlatformVersion:   "7.9",
		RunList:           []string{"recipe[nginx::default]", "recipe[app::ssl]"},
		TargetChefVersion: "18.4.2",
	}

	out, err := cfg_generate(t, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertContains(t, out, "driver:\n  name: dummy")
	assertContains(t, out, "name: chef_zero")
	assertContains(t, out, `product_version: "18.4.2"`)
	assertContains(t, out, "chef_license: accept")
	assertContains(t, out, "- name: centos-7.9")
	assertContains(t, out, "- name: web01")
	assertContains(t, out, "- recipe[nginx::default]")
	assertContains(t, out, "- recipe[app::ssl]")
	assertNotContains(t, out, "roles_path")
	assertNotContains(t, out, "attributes:")
}

func TestGenerateKitchenYML_ChefIceForV19(t *testing.T) {
	cfg := KitchenGenConfig{
		NodeName:          "db01",
		PlatformName:      "ubuntu",
		PlatformVersion:   "22.04",
		RunList:           []string{"recipe[postgres::default]"},
		TargetChefVersion: "19.1.0",
	}

	out, err := cfg_generate(t, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertContains(t, out, "name: chef_ice")
	assertNotContains(t, out, "chef_zero")
}

func TestGenerateKitchenYML_ChefIceForV20(t *testing.T) {
	cfg := KitchenGenConfig{
		NodeName:          "app01",
		PlatformName:      "rhel",
		PlatformVersion:   "9",
		RunList:           []string{"recipe[base::default]"},
		TargetChefVersion: "20.0.0",
	}

	out, err := cfg_generate(t, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertContains(t, out, "name: chef_ice")
}

func TestGenerateKitchenYML_WithRoles(t *testing.T) {
	cfg := KitchenGenConfig{
		NodeName:          "web02",
		PlatformName:      "centos",
		PlatformVersion:   "7",
		RunList:           []string{"recipe[base::default]"},
		TargetChefVersion: "18.4.2",
		HasRoles:          true,
	}

	out, err := cfg_generate(t, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertContains(t, out, "roles_path: roles")
}

func TestGenerateKitchenYML_WithCustomAttributes(t *testing.T) {
	attrs := json.RawMessage(`{"app":{"port":8080},"env":"production"}`)
	cfg := KitchenGenConfig{
		NodeName:          "app01",
		PlatformName:      "ubuntu",
		PlatformVersion:   "20.04",
		RunList:           []string{"recipe[myapp::default]"},
		TargetChefVersion: "17.0.0",
		CustomAttributes:  attrs,
	}

	out, err := cfg_generate(t, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertContains(t, out, "attributes:")
	assertContains(t, out, "app:")
	assertContains(t, out, "port: 8080")
	assertContains(t, out, "env: production")
}

func TestGenerateKitchenYML_EmptyAttributesOmitted(t *testing.T) {
	cfg := KitchenGenConfig{
		NodeName:          "node1",
		PlatformName:      "centos",
		PlatformVersion:   "7",
		RunList:           []string{"recipe[base::default]"},
		TargetChefVersion: "18.0.0",
		CustomAttributes:  json.RawMessage(`{}`),
	}

	out, err := cfg_generate(t, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertNotContains(t, out, "attributes:")
}

func TestGenerateKitchenYML_NullAttributesOmitted(t *testing.T) {
	cfg := KitchenGenConfig{
		NodeName:          "node1",
		PlatformName:      "centos",
		PlatformVersion:   "7",
		RunList:           []string{"recipe[base::default]"},
		TargetChefVersion: "18.0.0",
		CustomAttributes:  json.RawMessage(`null`),
	}

	out, err := cfg_generate(t, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertNotContains(t, out, "attributes:")
}

func TestGenerateKitchenYML_MissingNodeName(t *testing.T) {
	cfg := KitchenGenConfig{
		PlatformName:      "centos",
		PlatformVersion:   "7",
		TargetChefVersion: "18.0.0",
	}

	_, err := GenerateKitchenYML(cfg)
	if err == nil {
		t.Fatal("expected error for missing NodeName")
	}
	assertContains(t, err.Error(), "NodeName")
}

func TestGenerateKitchenYML_MissingPlatformName(t *testing.T) {
	cfg := KitchenGenConfig{
		NodeName:          "web01",
		TargetChefVersion: "18.0.0",
	}

	_, err := GenerateKitchenYML(cfg)
	if err == nil {
		t.Fatal("expected error for missing PlatformName")
	}
	assertContains(t, err.Error(), "PlatformName")
}

func TestGenerateKitchenYML_PlatformWithoutVersion(t *testing.T) {
	cfg := KitchenGenConfig{
		NodeName:          "web01",
		PlatformName:      "centos-7",
		RunList:           []string{"recipe[base::default]"},
		TargetChefVersion: "18.0.0",
	}

	out, err := cfg_generate(t, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertContains(t, out, "- name: centos-7\n")
}

func TestGenerateKitchenYML_EmptyRunList(t *testing.T) {
	cfg := KitchenGenConfig{
		NodeName:          "empty-node",
		PlatformName:      "centos",
		PlatformVersion:   "7",
		TargetChefVersion: "18.0.0",
	}

	out, err := cfg_generate(t, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertContains(t, out, "- name: empty-node")
	assertNotContains(t, out, "run_list:")
}

func TestGenerateOverlay_MatchingPlatform(t *testing.T) {
	tkConfig := &config.TestKitchenConfig{
		Driver: "vcenter",
		DriverSettings: map[string]any{
			"vcenter_host": "vc.example.com",
		},
		DriverSecrets: map[string]string{
			"vcenter_password": "vc-cred",
		},
		ImageFieldName: "template",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "centos-7", Image: "centos7img"},
		},
		Images: []config.ImageEntry{
			{
				Name: "centos7img",
				ID:   "tmpl-centos-7",
				Transport: &config.PlatformMapTransport{
					Username:           "root",
					PasswordCredential: "ssh_pass",
				},
			},
		},
	}

	out, err := GenerateOverlay(tkConfig, "centos-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertContains(t, out, "driver:")
	assertContains(t, out, "name: vcenter")
	assertContains(t, out, "vcenter_host: vc.example.com")
	assertContains(t, out, "CMM_TK_SECRET_VCENTER_PASSWORD")
	assertContains(t, out, "platforms:")
	assertContains(t, out, "- name: centos-7")
	assertContains(t, out, "template: tmpl-centos-7")
	assertContains(t, out, "transport:")
	assertContains(t, out, "username: root")
	assertContains(t, out, "CMM_TK_TRANSPORT_CENTOS7IMG")
}

func TestGenerateOverlay_NoMatch(t *testing.T) {
	tkConfig := &config.TestKitchenConfig{
		Driver: "vcenter",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "centos-7", Image: "centos7img"},
		},
	}

	out, err := GenerateOverlay(tkConfig, "ubuntu-22.04")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty overlay for non-matching platform, got: %s", out)
	}
}

func TestGenerateOverlay_SkipEntry(t *testing.T) {
	tkConfig := &config.TestKitchenConfig{
		Driver: "vcenter",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "centos-7", Image: "centos7img", Skip: true},
		},
	}

	_, err := GenerateOverlay(tkConfig, "centos-7")
	if err == nil {
		t.Fatal("expected error for skipped platform")
	}
	assertContains(t, err.Error(), "skipped")
}

func TestGenerateOverlay_NilConfig(t *testing.T) {
	out, err := GenerateOverlay(nil, "centos-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty overlay for nil config, got: %s", out)
	}
}

func TestGenerateOverlay_PatternMatch(t *testing.T) {
	tkConfig := &config.TestKitchenConfig{
		Driver: "ec2",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "centos-*", Image: "centos-base", IsPattern: true},
		},
		Images: []config.ImageEntry{
			{Name: "centos-base", ID: "ami-abc123"},
		},
	}

	out, err := GenerateOverlay(tkConfig, "centos-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, out, "platforms:")
	assertContains(t, out, "- name: centos-*")
}

func TestGenerateOverlay_TransportOverride(t *testing.T) {
	tkConfig := &config.TestKitchenConfig{
		Driver: "vcenter",
		PlatformMap: []config.PlatformMapEntry{
			{
				KitchenName: "centos-7",
				Image:       "centos7img",
				Transport: &config.PlatformMapTransport{
					Username:         "admin",
					SSHKeyCredential: "override-key",
				},
			},
		},
		Images: []config.ImageEntry{
			{
				Name: "centos7img",
				ID:   "tmpl-centos-7",
				Transport: &config.PlatformMapTransport{
					Username:           "root",
					PasswordCredential: "img-pass",
				},
			},
		},
	}

	out, err := GenerateOverlay(tkConfig, "centos-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use the platform map entry transport override.
	assertContains(t, out, "username: admin")
	assertContains(t, out, "CMM_TK_KEY_CENTOS7IMG")
	// Should NOT use the image transport.
	assertNotContains(t, out, "username: root")
}

func TestGenerateOverlay_SSHKeyTransport(t *testing.T) {
	tkConfig := &config.TestKitchenConfig{
		Driver: "ec2",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "rhel-9", Image: "rhel9"},
		},
		Images: []config.ImageEntry{
			{
				Name: "rhel9",
				ID:   "ami-12345",
				Transport: &config.PlatformMapTransport{
					Username:         "ec2-user",
					SSHKeyCredential: "ec2-ssh-key",
				},
			},
		},
	}

	out, err := GenerateOverlay(tkConfig, "rhel-9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertContains(t, out, "username: ec2-user")
	assertContains(t, out, "CMM_TK_KEY_RHEL9")
	assertNotContains(t, out, "CMM_TK_TRANSPORT_")
}

func TestWriteKitchenConfigs_BothFiles(t *testing.T) {
	dir := t.TempDir()

	kitchenYML := "---\ndriver:\n  name: dummy\n"
	overlay := "driver:\n  name: vcenter\n"

	if err := WriteKitchenConfigs(dir, kitchenYML, overlay); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".kitchen.yml"))
	if err != nil {
		t.Fatalf("reading .kitchen.yml: %v", err)
	}
	if string(data) != kitchenYML {
		t.Errorf("kitchen.yml content mismatch:\ngot:  %q\nwant: %q", string(data), kitchenYML)
	}

	data, err = os.ReadFile(filepath.Join(dir, ".kitchen.local.yml"))
	if err != nil {
		t.Fatalf("reading .kitchen.local.yml: %v", err)
	}
	if string(data) != overlay {
		t.Errorf("kitchen.local.yml content mismatch:\ngot:  %q\nwant: %q", string(data), overlay)
	}
}

func TestWriteKitchenConfigs_OverlaySkippedWhenEmpty(t *testing.T) {
	dir := t.TempDir()

	kitchenYML := "---\ndriver:\n  name: dummy\n"

	if err := WriteKitchenConfigs(dir, kitchenYML, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".kitchen.yml")); err != nil {
		t.Fatalf(".kitchen.yml should exist: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".kitchen.local.yml")); !os.IsNotExist(err) {
		t.Fatal(".kitchen.local.yml should not exist when overlay is empty")
	}
}

func TestFormatRunListEntry_Recipe(t *testing.T) {
	entry := RunListEntry{Type: "recipe", Name: "nginx", RecipeName: "ssl"}
	got := FormatRunListEntry(entry)
	want := "recipe[nginx::ssl]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatRunListEntry_RecipeDefault(t *testing.T) {
	entry := RunListEntry{Type: "recipe", Name: "nginx", RecipeName: "default"}
	got := FormatRunListEntry(entry)
	want := "recipe[nginx::default]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatRunListEntry_RecipeEmptyRecipeName(t *testing.T) {
	entry := RunListEntry{Type: "recipe", Name: "nginx"}
	got := FormatRunListEntry(entry)
	want := "recipe[nginx::default]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatRunListEntry_Role(t *testing.T) {
	entry := RunListEntry{Type: "role", Name: "webserver"}
	got := FormatRunListEntry(entry)
	want := "role[webserver]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestChefMajorVersion(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"18.4.2", 18},
		{"19.1.0", 19},
		{"17.0.0", 17},
		{"20.0.0-rc1", 20},
		{"", 0},
		{"bad", 0},
		{"3", 0},
	}
	for _, tt := range tests {
		got := chefMajorVersion(tt.input)
		if got != tt.want {
			t.Errorf("chefMajorVersion(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestGenerateKitchenYML_StartsWithYAMLDoc(t *testing.T) {
	cfg := KitchenGenConfig{
		NodeName:          "node1",
		PlatformName:      "centos",
		PlatformVersion:   "7",
		TargetChefVersion: "18.0.0",
	}

	out, err := cfg_generate(t, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(out, "---\n") {
		t.Errorf("expected YAML document marker, got prefix: %q", out[:20])
	}
}

func TestGenerateOverlay_DriverSettingsOrdered(t *testing.T) {
	tkConfig := &config.TestKitchenConfig{
		Driver: "vcenter",
		DriverSettings: map[string]any{
			"zzz_setting": "last",
			"aaa_setting": "first",
		},
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "centos-7", Image: "img1"},
		},
		Images: []config.ImageEntry{
			{Name: "img1", ID: "tmpl-1"},
		},
	}

	out, err := GenerateOverlay(tkConfig, "centos-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	idxA := strings.Index(out, "aaa_setting")
	idxZ := strings.Index(out, "zzz_setting")
	if idxA < 0 || idxZ < 0 {
		t.Fatal("expected both settings in output")
	}
	if idxA >= idxZ {
		t.Error("expected aaa_setting before zzz_setting (sorted order)")
	}
}

// cfg_generate is a test helper that calls GenerateKitchenYML.
func cfg_generate(t *testing.T, cfg KitchenGenConfig) (string, error) {
	t.Helper()
	return GenerateKitchenYML(cfg)
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q, got:\n%s", substr, s)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected output NOT to contain %q, got:\n%s", substr, s)
	}
}
