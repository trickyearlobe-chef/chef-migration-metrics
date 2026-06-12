// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"encoding/json"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
)

func diskFS(availMB, totalMB int) json.RawMessage {
	b, _ := json.Marshal(map[string]map[string]any{
		"/dev/sda1": {
			"kb_available": availMB * 1024,
			"kb_size":      totalMB * 1024,
			"mount":        "/",
		},
	})
	return b
}

func verdictCfg() analysis.DiskConfig {
	return analysis.DiskConfig{
		InstallPathLinux:        "/hab",
		InstallSizeMBLinux:      2048,
		MinRemainingFreePercent: 10,
	}
}

// A healthy node with plenty of space gets a determinate sufficient verdict,
// with available + required (platform-only) populated.
func TestNodeDiskVerdict_HealthySufficient(t *testing.T) {
	suff, avail, req := nodeDiskVerdict(diskFS(8192, 10240), "ubuntu", false, verdictCfg())
	if suff == nil || !*suff {
		t.Fatalf("sufficient = %v, want true", suff)
	}
	if avail == nil || *avail != 8192 {
		t.Errorf("available = %v, want 8192", avail)
	}
	if req == nil || *req != 2048 {
		t.Errorf("required = %v, want 2048", req)
	}
}

// A stale node's free space is old, so sufficiency + available are indeterminate
// (nil) — matching readiness evaluation — but the required size is still recorded.
func TestNodeDiskVerdict_StaleIsIndeterminate(t *testing.T) {
	suff, avail, req := nodeDiskVerdict(diskFS(8192, 10240), "ubuntu", true, verdictCfg())
	if suff != nil {
		t.Errorf("sufficient = %v, want nil (stale)", *suff)
	}
	if avail != nil {
		t.Errorf("available = %v, want nil (stale)", *avail)
	}
	if req == nil || *req != 2048 {
		t.Errorf("required = %v, want 2048 even when stale", req)
	}
}

// With no filesystem data the verdict is indeterminate, but required is still set.
func TestNodeDiskVerdict_NoFilesystem(t *testing.T) {
	suff, avail, req := nodeDiskVerdict(nil, "ubuntu", false, verdictCfg())
	if suff != nil || avail != nil {
		t.Errorf("sufficient/available = %v/%v, want nil/nil", suff, avail)
	}
	if req == nil || *req != 2048 {
		t.Errorf("required = %v, want 2048", req)
	}
}
