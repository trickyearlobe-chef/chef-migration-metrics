// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package gitkitchen

import (
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

func TestGenerateOverlay_ProxmoxDriver(t *testing.T) {
	tkConfig := config.TestKitchenConfig{
		Driver: "proxmox",
		DriverSettings: map[string]any{
			"endpoint": "https://pve.example.com:8006/api2/json",
		},
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-2204", Image: "ubuntu22"},
		},
		Images: []config.ImageEntry{
			{
				Name: "ubuntu22",
				ID:   "local:iso/ubuntu-22.04-server.iso",
				Transport: &config.PlatformMapTransport{
					Username:           "root",
					PasswordCredential: "ubuntu22-pass",
				},
			},
		},
	}

	overlay, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "ubuntu-2204", TargetChefVersion: "18.4.2", CookbookName: "test-cookbook", SuiteName: "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(overlay, "name: proxmox") {
		t.Error("expected driver name proxmox in overlay")
	}
	if !strings.Contains(overlay, "endpoint:") {
		t.Error("expected driver setting endpoint in overlay")
	}
	if !strings.Contains(overlay, "pve.example.com") {
		t.Error("expected endpoint URL value in overlay")
	}
	if !strings.Contains(overlay, "- name: ubuntu-2204") {
		t.Error("expected platform name in overlay")
	}
	if !strings.Contains(overlay, "product_version: \"18.4.2\"") {
		t.Error("expected provisioner product_version in overlay")
	}
	if !strings.Contains(overlay, "product_name: chef") {
		t.Error("expected product_name chef for version < 19")
	}
	if !strings.Contains(overlay, "chef_license: accept-no-persist") {
		t.Error("expected chef_license in overlay")
	}
}

func TestGenerateOverlay_NoMatch(t *testing.T) {
	tkConfig := config.TestKitchenConfig{
		Driver: "proxmox",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "centos-7", Image: "centos7"},
		},
		Images: []config.ImageEntry{
			{Name: "centos7", ID: "tpl-centos7"},
		},
	}

	overlay, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "ubuntu-2204", TargetChefVersion: "18.4.2", CookbookName: "test-cookbook", SuiteName: "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if overlay != "" {
		t.Errorf("expected empty overlay for unmatched platform, got: %s", overlay)
	}
}

func TestGenerateOverlay_SkippedPlatform(t *testing.T) {
	tkConfig := config.TestKitchenConfig{
		Driver: "proxmox",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-2204", Image: "ubuntu22", Skip: true},
		},
		Images: []config.ImageEntry{
			{Name: "ubuntu22", ID: "tpl-ubuntu22"},
		},
	}

	_, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "ubuntu-2204", TargetChefVersion: "18.4.2", CookbookName: "test-cookbook", SuiteName: "default"})
	if err == nil {
		t.Fatal("expected error for skipped platform")
	}
	if !strings.Contains(err.Error(), "skipped") {
		t.Errorf("expected error mentioning 'skipped', got: %v", err)
	}
}

func TestGenerateOverlay_ChefIce(t *testing.T) {
	tkConfig := config.TestKitchenConfig{
		Driver:                   "proxmox",
		ChefLicenseKeyCredential: "my-license",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-2204", Image: "ubuntu22"},
		},
		Images: []config.ImageEntry{
			{Name: "ubuntu22", ID: "tpl-ubuntu22"},
		},
	}

	overlay, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "ubuntu-2204", TargetChefVersion: "19.0.1", CookbookName: "test-cookbook", SuiteName: "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Must use chef-ice provisioner, not chef_zero with product_name.
	if !strings.Contains(overlay, "  name: chef-ice\n") {
		t.Errorf("expected provisioner name chef-ice for version >= 19, got:\n%s", overlay)
	}
	if !strings.Contains(overlay, "product_version: \"19.0.1\"") {
		t.Errorf("expected product_version in overlay, got:\n%s", overlay)
	}
	if !strings.Contains(overlay, "chef_license_key: <%= ENV['CMM_TK_CHEF_LICENSE_KEY'] %>") {
		t.Errorf("expected chef_license_key ERB ref, got:\n%s", overlay)
	}
	if !strings.Contains(overlay, "chef_license: accept") {
		t.Errorf("expected chef_license: accept, got:\n%s", overlay)
	}
	// Must NOT contain chef_zero-style settings.
	if strings.Contains(overlay, "require_chef_omnibus") {
		t.Error("chef-ice provisioner should not set require_chef_omnibus")
	}
	if strings.Contains(overlay, "product_name:") {
		t.Error("chef-ice provisioner should not set product_name")
	}
}

func TestGenerateOverlay_ChefIce_NoLicenseCredential(t *testing.T) {
	tkConfig := config.TestKitchenConfig{
		Driver: "proxmox",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-2204", Image: "ubuntu22"},
		},
		Images: []config.ImageEntry{
			{Name: "ubuntu22", ID: "tpl-ubuntu22"},
		},
	}

	overlay, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "ubuntu-2204", TargetChefVersion: "19.2.12", CookbookName: "test-cookbook", SuiteName: "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Without a license credential, chef_license_key line should be omitted.
	if strings.Contains(overlay, "chef_license_key") {
		t.Error("should not emit chef_license_key when credential is not configured")
	}
	// Should still use chef-ice provisioner.
	if !strings.Contains(overlay, "  name: chef-ice\n") {
		t.Errorf("expected provisioner name chef-ice, got:\n%s", overlay)
	}
}

func TestGenerateOverlay_DriverSecrets(t *testing.T) {
	tkConfig := config.TestKitchenConfig{
		Driver: "proxmox",
		DriverSecrets: map[string]string{
			"token_secret": "proxmox-token-cred",
		},
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-2204", Image: "ubuntu22"},
		},
		Images: []config.ImageEntry{
			{Name: "ubuntu22", ID: "tpl-ubuntu22"},
		},
	}

	overlay, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "ubuntu-2204", TargetChefVersion: "18.4.2", CookbookName: "test-cookbook", SuiteName: "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(overlay, "token_secret: <%= ENV['CMM_TK_SECRET_TOKEN_SECRET'] %>") {
		t.Errorf("expected ERB env ref for driver secret, got:\n%s", overlay)
	}
}

func TestGenerateOverlay_TransportCredentials(t *testing.T) {
	tkConfig := config.TestKitchenConfig{
		Driver: "proxmox",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-2204", Image: "ubuntu22"},
		},
		Images: []config.ImageEntry{
			{
				Name: "ubuntu22",
				ID:   "tpl-ubuntu22",
				Transport: &config.PlatformMapTransport{
					Username:           "admin",
					PasswordCredential: "ubuntu22-pass",
					SSHKeyCredential:   "ubuntu22-key",
				},
			},
		},
	}

	overlay, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "ubuntu-2204", TargetChefVersion: "18.4.2", CookbookName: "test-cookbook", SuiteName: "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(overlay, "username: admin") {
		t.Error("expected transport username in overlay")
	}
	if !strings.Contains(overlay, "password: <%= ENV['CMM_TK_TRANSPORT_UBUNTU22'] %>") {
		t.Errorf("expected transport password ERB ref, got:\n%s", overlay)
	}
	if !strings.Contains(overlay, "ssh_key: <%= ENV['CMM_TK_KEY_PATH_UBUNTU22'] %>") {
		t.Errorf("expected transport ssh_key ERB ref, got:\n%s", overlay)
	}
}

func TestGenerateOverlay_BakedIn_Chef19(t *testing.T) {
	tkConfig := config.TestKitchenConfig{
		Driver:                   "proxmox",
		ChefLicenseKeyCredential: "my-license",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "alma-10", Image: "alma10"},
		},
		Images: []config.ImageEntry{
			{
				Name:           "alma10",
				ID:             "200",
				InstallMethod:  "baked_in",
				ChefClientPath: "/opt/chef/bin/chef-client",
			},
		},
	}

	overlay, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "alma-10", TargetChefVersion: "19.2.12", CookbookName: "test-cookbook", SuiteName: "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(overlay, "  name: chef-ice\n") {
		t.Errorf("expected provisioner name chef-ice, got:\n%s", overlay)
	}
	if !strings.Contains(overlay, "install_strategy: skip") {
		t.Errorf("expected install_strategy: skip, got:\n%s", overlay)
	}
	if strings.Contains(overlay, "require_chef_omnibus") {
		t.Error("chef-ice should not use legacy require_chef_omnibus")
	}
	if !strings.Contains(overlay, "chef_client_path: /opt/chef/bin/chef-client") {
		t.Errorf("expected chef_client_path, got:\n%s", overlay)
	}
	if !strings.Contains(overlay, "chef_license_key: <%= ENV['CMM_TK_CHEF_LICENSE_KEY'] %>") {
		t.Errorf("expected chef_license_key, got:\n%s", overlay)
	}
	// Must NOT contain product_version or product_name (no download).
	if strings.Contains(overlay, "product_version") {
		t.Error("baked_in should not set product_version")
	}
	if strings.Contains(overlay, "product_name") {
		t.Error("baked_in should not set product_name")
	}
}

func TestGenerateOverlay_BakedIn_Chef18(t *testing.T) {
	tkConfig := config.TestKitchenConfig{
		Driver: "proxmox",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "centos-7", Image: "centos7"},
		},
		Images: []config.ImageEntry{
			{
				Name:           "centos7",
				ID:             "101",
				InstallMethod:  "baked_in",
				ChefClientPath: "/usr/bin/chef-client",
			},
		},
	}

	overlay, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "centos-7", TargetChefVersion: "18.4.2", CookbookName: "test-cookbook", SuiteName: "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(overlay, "require_chef_omnibus: false") {
		t.Errorf("expected require_chef_omnibus: false, got:\n%s", overlay)
	}
	if !strings.Contains(overlay, "chef_client_path: /usr/bin/chef-client") {
		t.Errorf("expected chef_client_path, got:\n%s", overlay)
	}
	// Chef <19 baked_in should NOT use chef-ice provisioner.
	if strings.Contains(overlay, "name: chef-ice") {
		t.Error("Chef 18 baked_in should not use chef-ice provisioner")
	}
	if strings.Contains(overlay, "product_version") {
		t.Error("baked_in should not set product_version")
	}
}

func TestGenerateOverlay_ProxmoxVMNamePrefix(t *testing.T) {
	tkConfig := config.TestKitchenConfig{
		Driver: "proxmox",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-2204", Image: "ubuntu22"},
		},
		Images: []config.ImageEntry{
			{Name: "ubuntu22", ID: "local:iso/ubuntu.iso"},
		},
	}

	overlay, err := generateOverlay(tkConfig, OverlayParams{
		PlatformName:      "ubuntu-2204",
		TargetChefVersion: "18.4.2",
		CookbookName:      "chef-client",
		SuiteName:         "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(overlay, `vm_name_prefix: "cmm-"`) {
		t.Errorf("expected vm_name_prefix in proxmox overlay, got:\n%s", overlay)
	}
}

func TestGenerateOverlay_VCenterVMName(t *testing.T) {
	tkConfig := config.TestKitchenConfig{
		Driver: "vcenter",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-2204", Image: "ubuntu22"},
		},
		Images: []config.ImageEntry{
			{Name: "ubuntu22", ID: "ubuntu-22.04-template"},
		},
	}

	overlay, err := generateOverlay(tkConfig, OverlayParams{
		PlatformName:      "ubuntu-2204",
		TargetChefVersion: "18.4.2",
		CookbookName:      "chef-client",
		SuiteName:         "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(overlay, "vm_name: cmm-") {
		t.Errorf("expected vm_name with cmm- prefix in vcenter overlay, got:\n%s", overlay)
	}
}

// ipReleaseConfig builds a minimal proxmox config with one image whose
// IP-release opt-in is controlled by the caller.
func ipReleaseConfig(platform, image string, optIn bool) config.TestKitchenConfig {
	return config.TestKitchenConfig{
		Driver: "proxmox",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: platform, Image: image},
		},
		Images: []config.ImageEntry{
			{Name: image, ID: "tpl-" + image, ReleaseIPOnDestroy: optIn},
		},
	}
}

func TestGenerateOverlay_IPReleaseHook_DisabledByDefault(t *testing.T) {
	tkConfig := ipReleaseConfig("ubuntu-2204", "ubuntu22", false)

	overlay, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "ubuntu-2204", TargetChefVersion: "18.4.2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(overlay, "lifecycle:") || strings.Contains(overlay, "pre_destroy:") {
		t.Errorf("expected no lifecycle hook when opt-in is off, got:\n%s", overlay)
	}
}

func TestGenerateOverlay_IPReleaseHook_LinuxEnabled(t *testing.T) {
	tkConfig := ipReleaseConfig("ubuntu-2204", "ubuntu22", true)

	overlay, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "ubuntu-2204", TargetChefVersion: "18.4.2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"lifecycle:", "pre_destroy:", "remote:", "dhclient"} {
		if !strings.Contains(overlay, want) {
			t.Errorf("expected overlay to contain %q, got:\n%s", want, overlay)
		}
	}
	// Failure isolation: command must always exit 0, be detached (&), and
	// redirect stdio so a severed transport is never seen as a hook failure.
	for _, want := range []string{"exit 0", " &", "/dev/null"} {
		if !strings.Contains(overlay, want) {
			t.Errorf("expected failure-isolated linux command to contain %q, got:\n%s", want, overlay)
		}
	}
	// Must not be a Windows command.
	if strings.Contains(overlay, "ipconfig") {
		t.Errorf("did not expect windows release command for a linux platform, got:\n%s", overlay)
	}
}

func TestGenerateOverlay_IPReleaseHook_Windows(t *testing.T) {
	tkConfig := ipReleaseConfig("windows-2022", "win2022", true)

	overlay, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "windows-2022", TargetChefVersion: "18.4.2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(overlay, "ipconfig /release") {
		t.Errorf("expected windows ipconfig release command, got:\n%s", overlay)
	}
	if !strings.Contains(overlay, "pre_destroy:") {
		t.Errorf("expected pre_destroy hook for windows, got:\n%s", overlay)
	}
	if strings.Contains(overlay, "dhclient") {
		t.Errorf("did not expect linux release command for a windows platform, got:\n%s", overlay)
	}
}

func TestGenerateOverlay_IPReleaseHook_ComposesExistingPreDestroy(t *testing.T) {
	tkConfig := ipReleaseConfig("ubuntu-2204", "ubuntu22", true)

	overlay, err := generateOverlay(tkConfig, OverlayParams{
		PlatformName:      "ubuntu-2204",
		TargetChefVersion: "18.4.2",
		ExistingPreDestroy: []any{
			map[string]any{"remote": "echo repo-setup-hook"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	repoIdx := strings.Index(overlay, "repo-setup-hook")
	cmmIdx := strings.Index(overlay, "dhclient")
	if repoIdx < 0 || cmmIdx < 0 {
		t.Fatalf("expected both repo and CMM pre_destroy commands, got:\n%s", overlay)
	}
	// The cookbook's own hook must be preserved and run before CMM's release.
	if repoIdx > cmmIdx {
		t.Errorf("expected repo pre_destroy hook before the injected release, got:\n%s", overlay)
	}
}

func TestGenerateOverlay_IPReleaseHook_OptInButNoMatch(t *testing.T) {
	// Opt-in image but the platform does not resolve — no overlay, no panic.
	tkConfig := ipReleaseConfig("ubuntu-2204", "ubuntu22", true)

	overlay, err := generateOverlay(tkConfig, OverlayParams{PlatformName: "centos-7", TargetChefVersion: "18.4.2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if overlay != "" {
		t.Errorf("expected empty overlay for unmatched platform, got:\n%s", overlay)
	}
}
