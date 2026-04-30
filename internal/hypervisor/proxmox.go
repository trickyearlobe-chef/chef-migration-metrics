// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ProxmoxClient implements the Hypervisor interface for Proxmox VE.
// This is a minimal proof-of-concept to validate the hypervisor abstraction.
type ProxmoxClient struct {
	baseURL     string // e.g. "https://pve.example.com:8006"
	node        string // Proxmox node name (e.g. "pve1")
	tokenID     string // API token ID (e.g. "user@pam!token-name")
	tokenSecret string // API token secret
	httpClient  *http.Client
}

// proxmoxResponse wraps the Proxmox API response format.
type proxmoxResponse struct {
	Data json.RawMessage `json:"data"`
}

// proxmoxVM represents a VM entry from the Proxmox API.
type proxmoxVM struct {
	VMID     int    `json:"vmid"`
	Name     string `json:"name"`
	Status   string `json:"status"`   // "running", "stopped"
	Template int    `json:"template"` // 1 = template, 0 = regular VM
	CPU      int    `json:"cpus"`
	MaxMem   int64  `json:"maxmem"` // bytes
	Uptime   int64  `json:"uptime"`
}

// NewProxmoxClient creates a Proxmox hypervisor client.
// The tokenID is in the format "user@realm!tokenname" and tokenSecret is the
// token value.
func NewProxmoxClient(baseURL, node, tokenID, tokenSecret string) *ProxmoxClient {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // POC — Proxmox often uses self-signed certs.
		},
	}
	return &ProxmoxClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		node:        node,
		tokenID:     tokenID,
		tokenSecret: tokenSecret,
		httpClient:  httpClient,
	}
}

// ListTemplates returns VM templates from the Proxmox node.
func (c *ProxmoxClient) ListTemplates(ctx context.Context) ([]Template, error) {
	vms, err := c.listAllVMs(ctx)
	if err != nil {
		return nil, fmt.Errorf("proxmox: list templates: %w", err)
	}

	var templates []Template
	for _, vm := range vms {
		if vm.Template != 1 {
			continue
		}
		templates = append(templates, Template{
			ID:   strconv.Itoa(vm.VMID),
			Name: vm.Name,
		})
	}
	return templates, nil
}

// ListManagedVMs returns non-template VMs whose name starts with prefix.
func (c *ProxmoxClient) ListManagedVMs(ctx context.Context, prefix string) ([]ManagedVM, error) {
	vms, err := c.listAllVMs(ctx)
	if err != nil {
		return nil, fmt.Errorf("proxmox: list managed VMs: %w", err)
	}

	var managed []ManagedVM
	for _, vm := range vms {
		if vm.Template == 1 {
			continue
		}
		if !matchesKitchenPrefix(vm.Name, prefix) {
			continue
		}
		managed = append(managed, ManagedVM{
			HypervisorID: strconv.Itoa(vm.VMID),
			Name:         vm.Name,
			PowerState:   proxmoxPowerState(vm.Status),
			CPUCount:     vm.CPU,
			MemoryMB:     int(vm.MaxMem / 1048576),
			Uptime:       time.Duration(vm.Uptime) * time.Second,
		})
	}
	return managed, nil
}

// matchesKitchenPrefix returns true if the VM name matches the configured
// prefix or the legacy "kitchen-" prefix used by Test Kitchen drivers.
// When prefix is empty, all VMs match (inventory mode).
func matchesKitchenPrefix(name, prefix string) bool {
	if prefix == "" {
		return true
	}
	if strings.HasPrefix(name, prefix) {
		return true
	}
	return strings.HasPrefix(name, "kitchen-")
}

// DestroyVM stops (if running) and deletes a VM by its VMID.
func (c *ProxmoxClient) DestroyVM(ctx context.Context, hypervisorID string) error {
	// Best-effort stop — ignore errors (VM may already be stopped).
	stopPath := fmt.Sprintf("/api2/json/nodes/%s/qemu/%s/status/stop", c.node, hypervisorID)
	_, _ = c.doRequest(ctx, http.MethodPost, stopPath)

	// Brief wait for stop to take effect (POC, not production).
	time.Sleep(1 * time.Second)

	deletePath := fmt.Sprintf("/api2/json/nodes/%s/qemu/%s", c.node, hypervisorID)
	_, err := c.doRequest(ctx, http.MethodDelete, deletePath)
	if err != nil {
		return fmt.Errorf("proxmox: destroy VM %s: %w", hypervisorID, err)
	}
	return nil
}

// Type returns the hypervisor type name.
func (c *ProxmoxClient) Type() string { return "proxmox" }

// listAllVMs fetches all QEMU VMs from the configured Proxmox node.
func (c *ProxmoxClient) listAllVMs(ctx context.Context) ([]proxmoxVM, error) {
	path := fmt.Sprintf("/api2/json/nodes/%s/qemu", c.node)
	body, err := c.doRequest(ctx, http.MethodGet, path)
	if err != nil {
		return nil, err
	}

	var resp proxmoxResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("proxmox: unmarshal response: %w", err)
	}

	var vms []proxmoxVM
	if err := json.Unmarshal(resp.Data, &vms); err != nil {
		return nil, fmt.Errorf("proxmox: unmarshal VM list: %w", err)
	}
	return vms, nil
}

// doRequest performs an authenticated Proxmox API request.
func (c *ProxmoxClient) doRequest(ctx context.Context, method, path string) ([]byte, error) {
	url := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("proxmox: create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenID, c.tokenSecret))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("proxmox: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("proxmox: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("proxmox: %s %s returned %d: %s", method, path, resp.StatusCode, snippet)
	}

	return body, nil
}

// proxmoxPowerState maps a Proxmox VM status string to a CMM power state.
func proxmoxPowerState(status string) string {
	switch status {
	case "running":
		return "poweredOn"
	default:
		return "poweredOff"
	}
}
