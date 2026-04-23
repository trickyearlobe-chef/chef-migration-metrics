// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"context"
	"testing"
	"time"
)

func TestGenerateVMName(t *testing.T) {
	ts := time.Unix(1700000000, 0)

	tests := []struct {
		name     string
		prefix   string
		cookbook string
		suite    string
		platform string
		want     string
	}{
		{
			name:     "basic",
			prefix:   "cmm",
			cookbook: "my-cookbook",
			suite:    "default",
			platform: "centos-7",
			want:     "cmm-my-cookbook-default-centos-7-1700000000",
		},
		{
			name:     "sanitisation",
			prefix:   "cmm",
			cookbook: "My_Cookbook!",
			suite:    "Default Suite",
			platform: "CentOS 7",
			want:     "cmm-my-cookbook-default-suite-centos-7-1700000000",
		},
		{
			name:     "empty prefix defaults to cmm",
			prefix:   "",
			cookbook: "test",
			suite:    "default",
			platform: "ubuntu",
			want:     "cmm-test-default-ubuntu-1700000000",
		},
		{
			name:     "long cookbook name truncated to fit 80 chars",
			prefix:   "cmm",
			cookbook: "aaaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffff",
			suite:    "default",
			platform: "centos-7",
			want:     "cmm-aaaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeee-default-centos-7-1700000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateVMName(tt.prefix, tt.cookbook, tt.suite, tt.platform, ts)
			if got != tt.want {
				t.Errorf("GenerateVMName() = %q, want %q", got, tt.want)
			}
			if len(got) > maxVMNameLen {
				t.Errorf("GenerateVMName() length = %d, exceeds max %d", len(got), maxVMNameLen)
			}
		})
	}
}

func TestGenerateVMName_Deterministic(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	a := GenerateVMName("cmm", "cookbook", "suite", "platform", ts)
	b := GenerateVMName("cmm", "cookbook", "suite", "platform", ts)
	if a != b {
		t.Errorf("expected deterministic output, got %q and %q", a, b)
	}
}

func TestParseVMName(t *testing.T) {
	tests := []struct {
		name      string
		vmName    string
		prefix    string
		wantOK    bool
		wantTS    int64
		wantPfx   string
		wantCB    string
		wantSuite string
		wantPlat  string
	}{
		{
			name:      "valid full name",
			vmName:    "cmm-my-cookbook-default-centos-7-1700000000",
			prefix:    "cmm",
			wantOK:    true,
			wantTS:    1700000000,
			wantPfx:   "cmm",
			wantCB:    "my",
			wantSuite: "cookbook",
			wantPlat:  "default-centos-7",
		},
		{
			name:   "wrong prefix",
			vmName: "other-cookbook-default-centos-7-1700000000",
			prefix: "cmm",
			wantOK: false,
		},
		{
			name:   "no timestamp",
			vmName: "cmm-cookbook-default-centos",
			prefix: "cmm",
			wantOK: false,
		},
		{
			name:   "timestamp too short",
			vmName: "cmm-cookbook-123",
			prefix: "cmm",
			wantOK: false,
		},
		{
			name:      "short name with minimal components",
			vmName:    "cmm-test-1700000000",
			prefix:    "cmm",
			wantOK:    true,
			wantTS:    1700000000,
			wantPfx:   "cmm",
			wantCB:    "test",
			wantSuite: "",
			wantPlat:  "",
		},
		{
			name:   "empty name",
			vmName: "",
			prefix: "cmm",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp, ok := ParseVMName(tt.vmName, tt.prefix)
			if ok != tt.wantOK {
				t.Fatalf("ParseVMName() ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if comp.Prefix != tt.wantPfx {
				t.Errorf("Prefix = %q, want %q", comp.Prefix, tt.wantPfx)
			}
			if comp.Timestamp != tt.wantTS {
				t.Errorf("Timestamp = %d, want %d", comp.Timestamp, tt.wantTS)
			}
			if comp.CookbookName != tt.wantCB {
				t.Errorf("CookbookName = %q, want %q", comp.CookbookName, tt.wantCB)
			}
			if comp.SuiteName != tt.wantSuite {
				t.Errorf("SuiteName = %q, want %q", comp.SuiteName, tt.wantSuite)
			}
			if comp.PlatformName != tt.wantPlat {
				t.Errorf("PlatformName = %q, want %q", comp.PlatformName, tt.wantPlat)
			}
		})
	}
}

func TestVMStatus_Values(t *testing.T) {
	allStatuses := []VMStatus{
		VMStatusCreating,
		VMStatusRunning,
		VMStatusDestroying,
		VMStatusDestroyed,
		VMStatusOrphaned,
	}

	if len(ValidVMStatuses) != len(allStatuses) {
		t.Fatalf("ValidVMStatuses has %d entries, expected %d", len(ValidVMStatuses), len(allStatuses))
	}

	for _, s := range allStatuses {
		if !ValidVMStatuses[s] {
			t.Errorf("status %q missing from ValidVMStatuses", s)
		}
	}
}

func TestNullHypervisor(t *testing.T) {
	var h NullHypervisor
	ctx := context.Background()

	templates, err := h.ListTemplates(ctx)
	if templates != nil || err != nil {
		t.Errorf("ListTemplates() = (%v, %v), want (nil, nil)", templates, err)
	}

	vms, err := h.ListManagedVMs(ctx, "cmm")
	if vms != nil || err != nil {
		t.Errorf("ListManagedVMs() = (%v, %v), want (nil, nil)", vms, err)
	}

	err = h.DestroyVM(ctx, "vm-123")
	if err == nil {
		t.Error("DestroyVM() should return an error")
	}

	if h.Type() != "none" {
		t.Errorf("Type() = %q, want %q", h.Type(), "none")
	}
}
