// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"fmt"
	"strconv"
	"strings"
)

// NewFromConfig creates a Hypervisor client from a type string, driver
// settings map, and pre-resolved secrets map. Returns (nil, nil) when
// hypervisor type is empty (no hypervisor configured). Returns an error
// when required settings are missing for the configured type.
func NewFromConfig(hypType string, settings map[string]any, resolvedSecrets map[string]string) (Hypervisor, error) {
	switch strings.ToLower(hypType) {
	case "":
		return nil, nil
	case "proxmox":
		return newProxmoxFromConfig(settings, resolvedSecrets)
	case "vcenter":
		return newVCenterFromConfig(settings, resolvedSecrets)
	default:
		return nil, fmt.Errorf("hypervisor: unknown type %q", hypType)
	}
}

func newProxmoxFromConfig(settings map[string]any, secrets map[string]string) (Hypervisor, error) {
	baseURL := settingStr(settings, "proxmox_url")
	if baseURL == "" {
		return nil, fmt.Errorf("hypervisor: proxmox requires driver_settings.proxmox_url")
	}
	// Node is optional — the cluster API resolves VM locations automatically.
	node := settingStr(settings, "proxmox_node")
	if node == "" {
		node = settingStr(settings, "node")
	}

	// Prefer API token auth if configured; fall back to username/password.
	insecure := settingBool(settings, "proxmox_insecure")

	tokenID := settingStr(settings, "proxmox_token_id")
	tokenSecret := secrets["proxmox_token_secret"]
	if tokenID != "" && tokenSecret != "" {
		return NewProxmoxClient(baseURL, node, tokenID, tokenSecret, WithInsecureSkipTLSVerify(insecure)), nil
	}

	username := settingStr(settings, "proxmox_username")
	if username == "" {
		return nil, fmt.Errorf("hypervisor: proxmox requires driver_settings.proxmox_username (or proxmox_token_id + proxmox_token_secret)")
	}
	password := secrets["proxmox_password"]
	if password == "" {
		return nil, fmt.Errorf("hypervisor: proxmox requires driver_secrets.proxmox_password")
	}
	return NewProxmoxClientWithPassword(baseURL, node, username, password, WithInsecureSkipTLSVerify(insecure)), nil
}

func newVCenterFromConfig(settings map[string]any, secrets map[string]string) (Hypervisor, error) {
	host := settingStr(settings, "vcenter_host")
	if host == "" {
		return nil, fmt.Errorf("hypervisor: vcenter requires driver_settings.vcenter_host")
	}
	username := settingStr(settings, "vcenter_username")
	if username == "" {
		return nil, fmt.Errorf("hypervisor: vcenter requires driver_settings.vcenter_username")
	}
	password := secrets["vcenter_password"]
	if password == "" {
		return nil, fmt.Errorf("hypervisor: vcenter requires driver_secrets.vcenter_password")
	}
	baseURL := host
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "https://" + host
	}
	datacenter := settingStr(settings, "datacenter")
	// Canonical key is vcenter_disable_ssl_verify (the kitchen-vcenter driver's
	// own setting, shared by the generated overlay). Fall back to the legacy
	// CMM-only vcenter_insecure so existing configs keep working.
	insecure := settingBool(settings, "vcenter_disable_ssl_verify")
	if _, ok := settings["vcenter_disable_ssl_verify"]; !ok {
		insecure = settingBool(settings, "vcenter_insecure")
	}
	return NewVCenterClient(baseURL, username, password, datacenter, WithInsecureSkipTLSVerify(insecure)), nil
}

// settingStr extracts a string value from a map[string]any.
func settingStr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// settingBool extracts a bool value from a map[string]any, defaulting to false
// when the key is absent or unparseable. It accepts a real bool or a string
// "true"/"false" (case-insensitive) — driver settings entered through the UI
// arrive as strings, so a strict bool assertion would silently drop them.
func settingBool(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		b, _ := strconv.ParseBool(strings.TrimSpace(val))
		return b
	default:
		return false
	}
}
