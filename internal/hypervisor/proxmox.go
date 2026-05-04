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
// Queries the cluster-wide API to discover templates and VMs across all nodes.
type ProxmoxClient struct {
	baseURL  string // e.g. "https://pve.example.com:8006"
	node     string // optional — used as hint for destroy if set
	authFunc func(req *http.Request) // injects auth header into requests
	httpClient *http.Client

	// Fields retained for test introspection.
	tokenID     string
	tokenSecret string
	username    string
	password    string
}

// proxmoxResponse wraps the Proxmox API response format.
type proxmoxResponse struct {
	Data json.RawMessage `json:"data"`
}

// proxmoxClusterResource represents a VM entry from /cluster/resources.
type proxmoxClusterResource struct {
	VMID     int    `json:"vmid"`
	Name     string `json:"name"`
	Node     string `json:"node"`
	Status   string `json:"status"`   // "running", "stopped"
	Type     string `json:"type"`     // "qemu", "lxc"
	Template int    `json:"template"` // 1 = template, 0 = regular VM
	MaxCPU   int    `json:"maxcpu"`
	MaxMem   int64  `json:"maxmem"` // bytes
	Uptime   int64  `json:"uptime"`
}

// NewProxmoxClient creates a Proxmox hypervisor client using API token auth.
// The tokenID is in the format "user@realm!tokenname" and tokenSecret is the
// token value.
func NewProxmoxClient(baseURL, node, tokenID, tokenSecret string) *ProxmoxClient {
	httpClient := newProxmoxHTTPClient()
	c := &ProxmoxClient{
		baseURL:     strings.TrimRight(baseURL, "/"),
		node:        node,
		tokenID:     tokenID,
		tokenSecret: tokenSecret,
		httpClient:  httpClient,
	}
	c.authFunc = func(req *http.Request) {
		req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", tokenID, tokenSecret))
	}
	return c
}

// NewProxmoxClientWithPassword creates a Proxmox hypervisor client using
// username/password ticket auth. The username is in "user@realm" format.
func NewProxmoxClientWithPassword(baseURL, node, username, password string) *ProxmoxClient {
	httpClient := newProxmoxHTTPClient()
	c := &ProxmoxClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		node:       node,
		username:   username,
		password:   password,
		httpClient: httpClient,
	}
	c.authFunc = func(req *http.Request) {
		req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", username, password))
	}
	return c
}

func newProxmoxHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // POC — Proxmox often uses self-signed certs.
		},
	}
}

// ListTemplates returns VM templates from all cluster nodes, deduped by name.
// When the same template name exists on multiple nodes, only one entry is returned.
func (c *ProxmoxClient) ListTemplates(ctx context.Context) ([]Template, error) {
	vms, err := c.listClusterVMs(ctx)
	if err != nil {
		return nil, fmt.Errorf("proxmox: list templates: %w", err)
	}

	seen := make(map[string]struct{})
	var templates []Template
	for _, vm := range vms {
		if vm.Template != 1 {
			continue
		}
		if _, dup := seen[vm.Name]; dup {
			continue
		}
		seen[vm.Name] = struct{}{}
		templates = append(templates, Template{
			ID:   strconv.Itoa(vm.VMID),
			Name: vm.Name,
		})
	}
	return templates, nil
}

// ListManagedVMs returns non-template VMs whose name starts with prefix.
func (c *ProxmoxClient) ListManagedVMs(ctx context.Context, prefix string) ([]ManagedVM, error) {
	vms, err := c.listClusterVMs(ctx)
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
			CPUCount:     vm.MaxCPU,
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
// Resolves the node from cluster resources if not configured.
func (c *ProxmoxClient) DestroyVM(ctx context.Context, hypervisorID string) error {
	node, err := c.resolveVMNode(ctx, hypervisorID)
	if err != nil {
		return err
	}
	if node == "" {
		// VM not found in cluster — treat as already destroyed.
		return nil
	}

	// Best-effort stop — ignore errors (VM may already be stopped).
	stopPath := fmt.Sprintf("/api2/json/nodes/%s/qemu/%s/status/stop", node, hypervisorID)
	_, _ = c.doRequest(ctx, http.MethodPost, stopPath)

	// Brief wait for stop to take effect.
	time.Sleep(1 * time.Second)

	deletePath := fmt.Sprintf("/api2/json/nodes/%s/qemu/%s", node, hypervisorID)
	_, err = c.doRequest(ctx, http.MethodDelete, deletePath)
	if err != nil {
		// Treat "does not exist" as already destroyed (stale cluster resource entry).
		if strings.Contains(err.Error(), "does not exist") {
			return nil
		}
		return fmt.Errorf("proxmox: destroy VM %s: %w", hypervisorID, err)
	}
	return nil
}

// resolveVMNode finds which node a VM is on via cluster resources.
// Returns empty string if VM not found.
func (c *ProxmoxClient) resolveVMNode(ctx context.Context, hypervisorID string) (string, error) {
	vms, err := c.listClusterVMs(ctx)
	if err != nil {
		return "", fmt.Errorf("proxmox: resolve VM node: %w", err)
	}
	for _, vm := range vms {
		if strconv.Itoa(vm.VMID) == hypervisorID {
			return vm.Node, nil
		}
	}
	return "", nil
}

// Type returns the hypervisor type name.
func (c *ProxmoxClient) Type() string { return "proxmox" }

// listClusterVMs queries the cluster-wide resource listing.
// Filters to QEMU VMs only (excludes LXC containers).
func (c *ProxmoxClient) listClusterVMs(ctx context.Context) ([]proxmoxClusterResource, error) {
	body, err := c.doRequest(ctx, http.MethodGet, "/api2/json/cluster/resources?type=vm")
	if err != nil {
		return nil, err
	}

	var resp proxmoxResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("proxmox: unmarshal response: %w", err)
	}

	var resources []proxmoxClusterResource
	if err := json.Unmarshal(resp.Data, &resources); err != nil {
		return nil, fmt.Errorf("proxmox: unmarshal cluster resources: %w", err)
	}

	// Filter to QEMU only.
	var qemu []proxmoxClusterResource
	for _, r := range resources {
		if r.Type == "qemu" {
			qemu = append(qemu, r)
		}
	}
	return qemu, nil
}

// doRequest performs an authenticated Proxmox API request.
func (c *ProxmoxClient) doRequest(ctx context.Context, method, path string) ([]byte, error) {
	url := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("proxmox: create request: %w", err)
	}
	c.authFunc(req)

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
