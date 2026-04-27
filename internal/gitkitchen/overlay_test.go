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

	overlay, err := generateOverlay(tkConfig, "ubuntu-2204", "18.4.2")
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

	overlay, err := generateOverlay(tkConfig, "ubuntu-2204", "18.4.2")
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

	_, err := generateOverlay(tkConfig, "ubuntu-2204", "18.4.2")
	if err == nil {
		t.Fatal("expected error for skipped platform")
	}
	if !strings.Contains(err.Error(), "skipped") {
		t.Errorf("expected error mentioning 'skipped', got: %v", err)
	}
}

func TestGenerateOverlay_ChefIce(t *testing.T) {
	tkConfig := config.TestKitchenConfig{
		Driver: "proxmox",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-2204", Image: "ubuntu22"},
		},
		Images: []config.ImageEntry{
			{Name: "ubuntu22", ID: "tpl-ubuntu22"},
		},
	}

	overlay, err := generateOverlay(tkConfig, "ubuntu-2204", "19.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(overlay, "product_name: chef-ice") {
		t.Error("expected product_name chef-ice for version >= 19")
	}
	if strings.Contains(overlay, "product_name: chef\n") {
		t.Error("should not contain bare 'product_name: chef' for version >= 19")
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

	overlay, err := generateOverlay(tkConfig, "ubuntu-2204", "18.4.2")
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

	overlay, err := generateOverlay(tkConfig, "ubuntu-2204", "18.4.2")
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
