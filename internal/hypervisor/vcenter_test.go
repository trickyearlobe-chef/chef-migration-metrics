// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func newVCenterMockServer(t *testing.T, vms []vsphereVM) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/session" && r.Method == http.MethodPost:
			user, pass, ok := r.BasicAuth()
			if !ok || user != "admin" || pass != "password" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode("test-session-id")

		case r.URL.Path == "/api/vcenter/vm" && r.Method == http.MethodGet:
			if r.Header.Get("vmware-api-session-id") != "test-session-id" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode(vms)

		case strings.HasSuffix(r.URL.Path, "/power") && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)

		case strings.HasPrefix(r.URL.Path, "/api/vcenter/vm/") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)

		default:
			http.NotFound(w, r)
		}
	}))
}

func newVCenterTestClient(srv *httptest.Server) *VCenterClient {
	client := NewVCenterClient(srv.URL, "admin", "password")
	client.httpClient = srv.Client()
	return client
}

func TestVCenterClient_Session(t *testing.T) {
	vms := []vsphereVM{
		{VM: "vm-1", Name: "test-vm", PowerState: "POWERED_ON", CPUCount: 2, MemorySizeMiB: 4096},
	}
	srv := newVCenterMockServer(t, vms)
	defer srv.Close()

	client := newVCenterTestClient(srv)
	// Session should be created implicitly on first REST request.
	_, err := client.ListManagedVMs(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.sessionID != "test-session-id" {
		t.Errorf("expected session ID 'test-session-id', got %q", client.sessionID)
	}
}

func TestVCenterClient_ListManagedVMs(t *testing.T) {
	vms := []vsphereVM{
		{VM: "vm-200", Name: "cmm-test-1", PowerState: "POWERED_ON", CPUCount: 2, MemorySizeMiB: 4096},
		{VM: "vm-201", Name: "cmm-test-2", PowerState: "POWERED_OFF", CPUCount: 4, MemorySizeMiB: 8192},
		{VM: "vm-300", Name: "other-vm", PowerState: "POWERED_ON", CPUCount: 1, MemorySizeMiB: 1024},
		{VM: "vm-400", Name: "cmm-other", PowerState: "SUSPENDED", CPUCount: 2, MemorySizeMiB: 2048},
	}
	srv := newVCenterMockServer(t, vms)
	defer srv.Close()

	client := newVCenterTestClient(srv)
	managed, err := client.ListManagedVMs(context.Background(), "cmm-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(managed) != 2 {
		t.Fatalf("expected 2 managed VMs, got %d", len(managed))
	}
	if managed[0].HypervisorID != "vm-200" || managed[0].Name != "cmm-test-1" {
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

func TestVCenterClient_DestroyVM(t *testing.T) {
	srv := newVCenterMockServer(t, nil)
	defer srv.Close()

	client := newVCenterTestClient(srv)
	err := client.DestroyVM(context.Background(), "vm-200")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVCenterClient_Session_Unauthorized(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewVCenterClient(srv.URL, "bad", "creds")
	client.httpClient = srv.Client()
	_, err := client.ListManagedVMs(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
}

func TestVCenterClient_Type(t *testing.T) {
	client := NewVCenterClient("https://vcenter.example.com", "u", "p")
	if client.Type() != "vcenter" {
		t.Errorf("expected vcenter, got %s", client.Type())
	}
}

func TestVCenterClient_PowerStateMapping(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"POWERED_ON", "poweredOn"},
		{"POWERED_OFF", "poweredOff"},
		{"SUSPENDED", "suspended"},
		{"UNKNOWN", "UNKNOWN"},
		{"", ""},
	}
	for _, tc := range tests {
		got := mapVSpherePowerState(tc.input)
		if got != tc.want {
			t.Errorf("mapVSpherePowerState(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestVCenterClient_SessionReuse(t *testing.T) {
	var sessionCalls int64
	vms := []vsphereVM{
		{VM: "vm-1", Name: "test", PowerState: "POWERED_ON", CPUCount: 1, MemorySizeMiB: 512},
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/session" && r.Method == http.MethodPost:
			atomic.AddInt64(&sessionCalls, 1)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode("reuse-session-id")
		case r.URL.Path == "/api/vcenter/vm" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(vms)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewVCenterClient(srv.URL, "admin", "password")
	client.httpClient = srv.Client()

	ctx := context.Background()
	if _, err := client.ListManagedVMs(ctx, ""); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := client.ListManagedVMs(ctx, ""); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if n := atomic.LoadInt64(&sessionCalls); n != 1 {
		t.Errorf("expected 1 session POST, got %d", n)
	}
}

func TestVCenterClient_ListManagedVMs_EmptyPrefix(t *testing.T) {
	vms := []vsphereVM{
		{VM: "vm-1", Name: "cmm-a", PowerState: "POWERED_ON", CPUCount: 1, MemorySizeMiB: 512},
		{VM: "vm-2", Name: "other", PowerState: "POWERED_OFF", CPUCount: 2, MemorySizeMiB: 1024},
	}
	srv := newVCenterMockServer(t, vms)
	defer srv.Close()

	client := newVCenterTestClient(srv)
	managed, err := client.ListManagedVMs(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(managed) != 2 {
		t.Fatalf("expected 2 VMs with empty prefix, got %d", len(managed))
	}
}
