// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func newProxmoxMockServer(t *testing.T, vms []proxmoxVM) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "PVEAPIToken=") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/qemu"):
			resp := proxmoxResponse{Data: mustMarshal(vms)}
			json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/status/stop"):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(proxmoxResponse{})

		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(proxmoxResponse{})

		default:
			http.NotFound(w, r)
		}
	}))
}

func newProxmoxTestClient(srv *httptest.Server) *ProxmoxClient {
	client := NewProxmoxClient(srv.URL, "pve1", "test@pam!token", "secret")
	client.httpClient = srv.Client()
	return client
}

func TestProxmoxClient_ListTemplates(t *testing.T) {
	vms := []proxmoxVM{
		{VMID: 100, Name: "tmpl-ubuntu", Status: "stopped", Template: 1, CPU: 2, MaxMem: 2147483648},
		{VMID: 101, Name: "tmpl-rhel", Status: "stopped", Template: 1, CPU: 4, MaxMem: 4294967296},
		{VMID: 200, Name: "worker-1", Status: "running", Template: 0, CPU: 4, MaxMem: 8589934592},
	}
	srv := newProxmoxMockServer(t, vms)
	defer srv.Close()

	client := newProxmoxTestClient(srv)
	templates, err := client.ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(templates))
	}
	if templates[0].ID != "100" || templates[0].Name != "tmpl-ubuntu" {
		t.Errorf("unexpected first template: %+v", templates[0])
	}
	if templates[1].ID != "101" || templates[1].Name != "tmpl-rhel" {
		t.Errorf("unexpected second template: %+v", templates[1])
	}
}

func TestProxmoxClient_ListManagedVMs(t *testing.T) {
	vms := []proxmoxVM{
		{VMID: 100, Name: "tmpl-ubuntu", Status: "stopped", Template: 1, CPU: 2, MaxMem: 2147483648},
		{VMID: 200, Name: "cmm-test-1", Status: "running", Template: 0, CPU: 2, MaxMem: 4294967296},
		{VMID: 201, Name: "cmm-test-2", Status: "stopped", Template: 0, CPU: 4, MaxMem: 8589934592},
		{VMID: 300, Name: "other-vm", Status: "running", Template: 0, CPU: 1, MaxMem: 1073741824},
	}
	srv := newProxmoxMockServer(t, vms)
	defer srv.Close()

	client := newProxmoxTestClient(srv)
	managed, err := client.ListManagedVMs(context.Background(), "cmm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(managed) != 2 {
		t.Fatalf("expected 2 managed VMs, got %d", len(managed))
	}
	if managed[0].HypervisorID != "200" || managed[0].Name != "cmm-test-1" {
		t.Errorf("unexpected first VM: %+v", managed[0])
	}
	if managed[0].PowerState != "poweredOn" {
		t.Errorf("expected poweredOn, got %s", managed[0].PowerState)
	}
	if managed[0].CPUCount != 2 {
		t.Errorf("expected 2 CPUs, got %d", managed[0].CPUCount)
	}
	if managed[0].MemoryMB != 4096 {
		t.Errorf("expected 4096 MB, got %d", managed[0].MemoryMB)
	}
	if managed[1].PowerState != "poweredOff" {
		t.Errorf("expected poweredOff, got %s", managed[1].PowerState)
	}
}

func TestProxmoxClient_DestroyVM(t *testing.T) {
	srv := newProxmoxMockServer(t, nil)
	defer srv.Close()

	client := newProxmoxTestClient(srv)
	err := client.DestroyVM(context.Background(), "200")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProxmoxClient_ListTemplates_Unauthorized(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewProxmoxClient(srv.URL, "pve1", "bad", "creds")
	client.httpClient = srv.Client()
	_, err := client.ListTemplates(context.Background())
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
}

func TestProxmoxClient_Type(t *testing.T) {
	client := NewProxmoxClient("https://pve.example.com:8006", "pve1", "u@pam!t", "s")
	if client.Type() != "proxmox" {
		t.Errorf("expected proxmox, got %s", client.Type())
	}
}

func TestProxmoxClient_ListManagedVMs_EmptyPrefix(t *testing.T) {
	vms := []proxmoxVM{
		{VMID: 100, Name: "tmpl-ubuntu", Status: "stopped", Template: 1, CPU: 2, MaxMem: 2147483648},
		{VMID: 200, Name: "cmm-test-1", Status: "running", Template: 0, CPU: 2, MaxMem: 4294967296},
		{VMID: 201, Name: "cmm-test-2", Status: "stopped", Template: 0, CPU: 4, MaxMem: 8589934592},
		{VMID: 300, Name: "other-vm", Status: "running", Template: 0, CPU: 1, MaxMem: 1073741824},
	}
	srv := newProxmoxMockServer(t, vms)
	defer srv.Close()

	client := newProxmoxTestClient(srv)
	managed, err := client.ListManagedVMs(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(managed) != 3 {
		t.Fatalf("expected 3 managed VMs with empty prefix, got %d", len(managed))
	}
}
