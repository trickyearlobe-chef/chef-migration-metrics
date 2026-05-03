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
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
)

// VCenterClient implements the Hypervisor interface for VMware vCenter
// using the vSphere REST API (not the SOAP API). This keeps dependencies
// lean — no govmomi required.
type VCenterClient struct {
	baseURL    string // e.g. "https://vcenter.example.com"
	username   string
	password   string
	datacenter string // optional datacenter name for SOAP/govmomi queries
	httpClient *http.Client

	sessionMu  sync.Mutex
	sessionID  string
	sessionExp time.Time
}

// vsphereVM represents a VM entry from the vSphere REST API
// GET /api/vcenter/vm endpoint.
type vsphereVM struct {
	VM            string `json:"vm"`              // MoRef ID like "vm-123"
	Name          string `json:"name"`            // VM display name
	PowerState    string `json:"power_state"`     // "POWERED_ON", "POWERED_OFF", "SUSPENDED"
	CPUCount      int    `json:"cpu_count"`       // number of vCPUs
	MemorySizeMiB int    `json:"memory_size_mib"` // memory in MiB
}

// sessionTTL is how long we consider a cached session valid.
// vSphere sessions last 30 minutes; we refresh 5 minutes early.
const sessionTTL = 25 * time.Minute

// sessionBuffer is the safety margin before expiry at which we re-auth.
const sessionBuffer = 5 * time.Minute

// NewVCenterClient creates a vCenter hypervisor client.
func NewVCenterClient(baseURL, username, password, datacenter string) *VCenterClient {
	jar, _ := cookiejar.New(nil) // cookiejar.New never returns an error with nil options
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Jar:     jar,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // vCenter often uses self-signed certs.
		},
	}
	return &VCenterClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		username:   username,
		password:   password,
		datacenter: datacenter,
		httpClient: httpClient,
	}
}

// ensureSession creates or refreshes the vSphere API session.
// The session ID is cached and reused until near expiry.
func (c *VCenterClient) ensureSession(ctx context.Context) error {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()

	if c.sessionID != "" && time.Now().Add(sessionBuffer).Before(c.sessionExp) {
		return nil
	}

	url := c.baseURL + "/api/session"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("vcenter: create session request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vcenter: session POST: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("vcenter: read session response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return fmt.Errorf("vcenter: session auth failed (HTTP %d): %s", resp.StatusCode, snippet)
	}

	var sessionID string
	if err := json.Unmarshal(body, &sessionID); err != nil {
		return fmt.Errorf("vcenter: unmarshal session ID: %w", err)
	}
	if sessionID == "" {
		return fmt.Errorf("vcenter: empty session ID in response")
	}

	c.sessionID = sessionID
	c.sessionExp = time.Now().Add(sessionTTL)
	return nil
}

// ListTemplates returns classic VM templates from vCenter using the SOAP
// API (via govmomi). The REST API excludes classic templates by design, so
// we use the Finder to discover all VMs then filter by IsTemplate.
func (c *VCenterClient) ListTemplates(ctx context.Context) ([]Template, error) {
	u, err := url.Parse(c.baseURL + "/sdk")
	if err != nil {
		return nil, fmt.Errorf("vcenter: parse URL: %w", err)
	}
	u.User = url.UserPassword(c.username, c.password)

	client, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		return nil, fmt.Errorf("vcenter: SOAP connect: %w", err)
	}
	defer client.Logout(ctx) //nolint:errcheck

	finder := find.NewFinder(client.Client, false)

	// Set datacenter scope — required when vCenter manages multiple datacenters.
	if c.datacenter != "" {
		dc, err := finder.Datacenter(ctx, c.datacenter)
		if err != nil {
			return nil, fmt.Errorf("vcenter: find datacenter %q: %w", c.datacenter, err)
		}
		finder.SetDatacenter(dc)
	} else {
		dc, err := finder.DefaultDatacenter(ctx)
		if err != nil {
			return nil, fmt.Errorf("vcenter: find default datacenter: %w", err)
		}
		finder.SetDatacenter(dc)
	}

	vms, err := finder.VirtualMachineList(ctx, "*")
	if err != nil {
		return nil, fmt.Errorf("vcenter: find VMs: %w", err)
	}

	var templates []Template
	for _, vm := range vms {
		isTmpl, err := vm.IsTemplate(ctx)
		if err != nil {
			continue
		}
		if isTmpl {
			templates = append(templates, Template{
				ID:   vm.Reference().Value,
				Name: vm.Name(),
			})
		}
	}
	return templates, nil
}

// ListManagedVMs returns VMs whose name starts with prefix.
func (c *VCenterClient) ListManagedVMs(ctx context.Context, prefix string) ([]ManagedVM, error) {
	body, _, err := c.doRequest(ctx, http.MethodGet, "/api/vcenter/vm")
	if err != nil {
		return nil, fmt.Errorf("vcenter: list managed VMs: %w", err)
	}

	var vms []vsphereVM
	if err := json.Unmarshal(body, &vms); err != nil {
		return nil, fmt.Errorf("vcenter: unmarshal VM list: %w", err)
	}

	var managed []ManagedVM
	for _, vm := range vms {
		if !matchesKitchenPrefix(vm.Name, prefix) {
			continue
		}
		managed = append(managed, ManagedVM{
			HypervisorID: vm.VM,
			Name:         vm.Name,
			PowerState:   mapVSpherePowerState(vm.PowerState),
			CPUCount:     vm.CPUCount,
			MemoryMB:     vm.MemorySizeMiB,
		})
	}
	return managed, nil
}

// DestroyVM powers off (best-effort) and deletes a VM by its MoRef ID.
func (c *VCenterClient) DestroyVM(ctx context.Context, hypervisorID string) error {
	// Best-effort power off — ignore errors (VM may already be off).
	powerPath := fmt.Sprintf("/api/vcenter/vm/%s/power?action=stop", hypervisorID)
	_, status, _ := c.doRequest(ctx, http.MethodPost, powerPath)
	// 400 is expected if the VM is already powered off.
	_ = status // Non-OK/non-400 status is log-worthy but not fatal — proceed with delete.

	deletePath := fmt.Sprintf("/api/vcenter/vm/%s", hypervisorID)
	_, _, err := c.doRequest(ctx, http.MethodDelete, deletePath)
	if err != nil {
		return fmt.Errorf("vcenter: destroy VM %s: %w", hypervisorID, err)
	}
	return nil
}

// Type returns the hypervisor type name.
func (c *VCenterClient) Type() string { return "vcenter" }

// doRequest performs an authenticated vCenter API request.
func (c *VCenterClient) doRequest(ctx context.Context, method, path string) ([]byte, int, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, 0, err
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("vcenter: create request: %w", err)
	}
	req.Header.Set("vmware-api-session-id", c.sessionID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("vcenter: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("vcenter: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, resp.StatusCode, fmt.Errorf("vcenter: %s %s returned %d: %s", method, path, resp.StatusCode, snippet)
	}

	return body, resp.StatusCode, nil
}

// mapVSpherePowerState converts vSphere power state strings to the CMM
// standard format used across all hypervisor backends.
func mapVSpherePowerState(state string) string {
	switch state {
	case "POWERED_ON":
		return "poweredOn"
	case "POWERED_OFF":
		return "poweredOff"
	case "SUSPENDED":
		return "suspended"
	default:
		return state
	}
}
