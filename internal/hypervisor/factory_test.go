// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"net/http"
	"strings"
	"testing"
)

func TestNewFromConfig_EmptyType(t *testing.T) {
	h, err := NewFromConfig("", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h != nil {
		t.Fatal("expected nil hypervisor for empty type")
	}
}

func TestNewFromConfig_UnknownType(t *testing.T) {
	_, err := NewFromConfig("xen", nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if !strings.Contains(err.Error(), "unknown type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewFromConfig_Proxmox(t *testing.T) {
	settings := map[string]any{
		"proxmox_url":      "https://pve.example.com:8006",
		"node":             "pve1",
		"proxmox_token_id": "user@pam!cmm",
	}
	secrets := map[string]string{
		"proxmox_token_secret": "secret-value",
	}

	h, err := NewFromConfig("proxmox", settings, secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("expected non-nil hypervisor")
	}
	pc, ok := h.(*ProxmoxClient)
	if !ok {
		t.Fatalf("expected *ProxmoxClient, got %T", h)
	}
	if pc.baseURL != "https://pve.example.com:8006" {
		t.Errorf("baseURL = %q", pc.baseURL)
	}
	if pc.node != "pve1" {
		t.Errorf("node = %q", pc.node)
	}
	if pc.tokenID != "user@pam!cmm" {
		t.Errorf("tokenID = %q", pc.tokenID)
	}
	if pc.tokenSecret != "secret-value" {
		t.Errorf("tokenSecret = %q", pc.tokenSecret)
	}
}

func TestNewFromConfig_Proxmox_TokenAuth(t *testing.T) {
	settings := map[string]any{
		"proxmox_url":      "https://pve.example.com:8006",
		"proxmox_node":     "pve1",
		"proxmox_token_id": "user@pam!cmm",
	}
	secrets := map[string]string{
		"proxmox_token_secret": "token-secret-value",
	}

	h, err := NewFromConfig("proxmox", settings, secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pc := h.(*ProxmoxClient)
	if pc.tokenID != "user@pam!cmm" {
		t.Errorf("tokenID = %q", pc.tokenID)
	}
	if pc.tokenSecret != "token-secret-value" {
		t.Errorf("tokenSecret = %q", pc.tokenSecret)
	}
}

func TestNewFromConfig_Proxmox_MissingFields(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
		secrets  map[string]string
		wantErr  string
	}{
		{
			name:     "missing url",
			settings: map[string]any{"node": "n", "proxmox_token_id": "t"},
			secrets:  map[string]string{"proxmox_token_secret": "s"},
			wantErr:  "proxmox_url",
		},
		{
			name:     "missing auth",
			settings: map[string]any{"proxmox_url": "u", "node": "n"},
			secrets:  map[string]string{},
			wantErr:  "proxmox_username",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFromConfig("proxmox", tt.settings, tt.secrets)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNewFromConfig_Proxmox_NodeOptional(t *testing.T) {
	settings := map[string]any{
		"proxmox_url":      "https://pve.example.com:8006",
		"proxmox_token_id": "user@pam!cmm",
	}
	secrets := map[string]string{
		"proxmox_token_secret": "secret-value",
	}

	h, err := NewFromConfig("proxmox", settings, secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("expected non-nil hypervisor")
	}
	pc := h.(*ProxmoxClient)
	if pc.node != "" {
		t.Errorf("expected empty node, got %q", pc.node)
	}
}

func TestNewFromConfig_VCenter(t *testing.T) {
	settings := map[string]any{
		"vcenter_host":     "vcenter.example.com",
		"vcenter_username": "admin@vsphere.local",
	}
	secrets := map[string]string{
		"vcenter_password": "pass123",
	}

	h, err := NewFromConfig("vcenter", settings, secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("expected non-nil hypervisor")
	}
	vc, ok := h.(*VCenterClient)
	if !ok {
		t.Fatalf("expected *VCenterClient, got %T", h)
	}
	if vc.baseURL != "https://vcenter.example.com" {
		t.Errorf("baseURL = %q", vc.baseURL)
	}
	if vc.username != "admin@vsphere.local" {
		t.Errorf("username = %q", vc.username)
	}
	if vc.password != "pass123" {
		t.Errorf("password = %q", vc.password)
	}
}

func TestNewFromConfig_VCenter_WithDatacenter(t *testing.T) {
	settings := map[string]any{
		"vcenter_host":     "vcenter.example.com",
		"vcenter_username": "admin@vsphere.local",
		"datacenter":       "DC-01",
	}
	secrets := map[string]string{
		"vcenter_password": "pass123",
	}

	h, err := NewFromConfig("vcenter", settings, secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vc := h.(*VCenterClient)
	if vc.datacenter != "DC-01" {
		t.Errorf("datacenter = %q, want %q", vc.datacenter, "DC-01")
	}
}

func TestNewFromConfig_VCenter_WithScheme(t *testing.T) {
	settings := map[string]any{
		"vcenter_host":     "https://vc.example.com",
		"vcenter_username": "admin",
	}
	secrets := map[string]string{
		"vcenter_password": "pass",
	}

	h, err := NewFromConfig("vcenter", settings, secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vc := h.(*VCenterClient)
	if vc.baseURL != "https://vc.example.com" {
		t.Errorf("baseURL = %q, want no double-scheme", vc.baseURL)
	}
}

func TestNewFromConfig_VCenter_MissingFields(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
		secrets  map[string]string
		wantErr  string
	}{
		{
			name:     "missing host",
			settings: map[string]any{"vcenter_username": "u"},
			secrets:  map[string]string{"vcenter_password": "p"},
			wantErr:  "vcenter_host",
		},
		{
			name:     "missing username",
			settings: map[string]any{"vcenter_host": "h"},
			secrets:  map[string]string{"vcenter_password": "p"},
			wantErr:  "vcenter_username",
		},
		{
			name:     "missing password",
			settings: map[string]any{"vcenter_host": "h", "vcenter_username": "u"},
			secrets:  map[string]string{},
			wantErr:  "vcenter_password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFromConfig("vcenter", tt.settings, tt.secrets)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNewFromConfig_CaseInsensitive(t *testing.T) {
	settings := map[string]any{
		"proxmox_url":      "https://pve.example.com:8006",
		"node":             "pve1",
		"proxmox_token_id": "user@pam!t",
	}
	secrets := map[string]string{"proxmox_token_secret": "s"}

	h, err := NewFromConfig("Proxmox", settings, secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Fatal("expected non-nil for uppercase type")
	}
}

// tlsInsecure reports the InsecureSkipVerify setting of a client's transport.
func tlsInsecure(t *testing.T, hc *http.Client) bool {
	t.Helper()
	tr, ok := hc.Transport.(*http.Transport)
	if !ok || tr.TLSClientConfig == nil {
		return false
	}
	return tr.TLSClientConfig.InsecureSkipVerify
}

func TestHypervisorClients_TLSVerificationSecureByDefault(t *testing.T) {
	if tlsInsecure(t, NewVCenterClient("https://vc", "u", "p", "").httpClient) {
		t.Error("vCenter: TLS verification should be ON by default")
	}
	if tlsInsecure(t, NewProxmoxClient("https://pve", "n", "id", "s").httpClient) {
		t.Error("Proxmox token client: TLS verification should be ON by default")
	}
	if tlsInsecure(t, NewProxmoxClientWithPassword("https://pve", "n", "u", "p").httpClient) {
		t.Error("Proxmox password client: TLS verification should be ON by default")
	}
}

func TestHypervisorClients_InsecureOptIn(t *testing.T) {
	if !tlsInsecure(t, NewVCenterClient("https://vc", "u", "p", "", WithInsecureSkipTLSVerify(true)).httpClient) {
		t.Error("vCenter: expected InsecureSkipVerify when opted in")
	}
	if !tlsInsecure(t, NewProxmoxClient("https://pve", "n", "id", "s", WithInsecureSkipTLSVerify(true)).httpClient) {
		t.Error("Proxmox: expected InsecureSkipVerify when opted in")
	}
}

func TestNewFromConfig_VCenterInsecureSetting(t *testing.T) {
	secrets := map[string]string{"vcenter_password": "p"}
	base := map[string]any{"vcenter_host": "vc.example.com", "vcenter_username": "u"}

	h, err := NewFromConfig("vcenter", base, secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsInsecure(t, h.(*VCenterClient).httpClient) {
		t.Error("default config should verify TLS")
	}

	withInsecure := map[string]any{"vcenter_host": "vc.example.com", "vcenter_username": "u", "vcenter_insecure": true}
	h2, err := NewFromConfig("vcenter", withInsecure, secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tlsInsecure(t, h2.(*VCenterClient).httpClient) {
		t.Error("vcenter_insecure: true should disable TLS verification")
	}
}
