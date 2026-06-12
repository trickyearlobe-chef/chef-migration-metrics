// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"encoding/json"
	"testing"
)

// mb returns kilobytes for a megabyte figure (ohai reports kb_*).
func mbToKB(mb int) int { return mb * 1024 }

func linuxRootFS(availMB, totalMB int) json.RawMessage {
	return linuxFilesystemJSON(map[string]linuxMount{
		"/dev/sda1": {
			KBSize:      mbToKB(totalMB),
			KBAvailable: mbToKB(availMB),
			Mount:       "/",
		},
	})
}

func diskCfg() DiskConfig {
	return DiskConfig{
		InstallPathLinux:        "/hab",
		InstallPathWindows:      `C:\hab`,
		InstallSizeMBLinux:      2048,
		InstallSizeMBWindows:    2048,
		MinRemainingFreePercent: 10,
	}
}

// EvaluateDisk is version-invariant: same filesystem + platform + config yields
// the same verdict regardless of any Chef target version (there is no target
// input). Plenty of headroom → sufficient, with available/required populated.
func TestEvaluateDisk_Sufficient(t *testing.T) {
	v := EvaluateDisk(linuxRootFS(8192, 10240), "ubuntu", diskCfg())
	if v.Sufficient == nil || !*v.Sufficient {
		t.Fatalf("expected sufficient=true, got %v", v.Sufficient)
	}
	if v.AvailableMB == nil || *v.AvailableMB != 8192 {
		t.Errorf("available_mb = %v, want 8192", v.AvailableMB)
	}
	if v.RequiredMB != 2048 {
		t.Errorf("required_mb = %d, want 2048", v.RequiredMB)
	}
}

// Below the absolute install size → insufficient.
func TestEvaluateDisk_InsufficientAbsolute(t *testing.T) {
	v := EvaluateDisk(linuxRootFS(1024, 10240), "ubuntu", diskCfg())
	if v.Sufficient == nil || *v.Sufficient {
		t.Fatalf("expected sufficient=false (1024MB < 2048MB required), got %v", v.Sufficient)
	}
}

// Enough for the absolute install but the post-install free % falls under the
// min-remaining-free-% threshold → insufficient (dual threshold).
func TestEvaluateDisk_InsufficientByMinFreePercent(t *testing.T) {
	cfg := diskCfg()
	cfg.MinRemainingFreePercent = 50 // remaining after install must be >= 50%
	// 2560 available, 2048 required → 512MB remaining of 5120MB total = 10% < 50%.
	v := EvaluateDisk(linuxRootFS(2560, 5120), "ubuntu", cfg)
	if v.Sufficient == nil || *v.Sufficient {
		t.Fatalf("expected sufficient=false (10%% remaining < 50%%), got %v", v.Sufficient)
	}
}

// No usable filesystem data → indeterminate: Sufficient/AvailableMB nil, but the
// required size (a function of platform only) is still reported.
func TestEvaluateDisk_UnknownWhenNoFilesystem(t *testing.T) {
	v := EvaluateDisk(nil, "ubuntu", diskCfg())
	if v.Sufficient != nil {
		t.Errorf("sufficient = %v, want nil (indeterminate)", *v.Sufficient)
	}
	if v.AvailableMB != nil {
		t.Errorf("available_mb = %v, want nil", *v.AvailableMB)
	}
	if v.RequiredMB != 2048 {
		t.Errorf("required_mb = %d, want 2048 (platform-only, always set)", v.RequiredMB)
	}
}

// Windows uses the windows install path/size and drive layout.
func TestEvaluateDisk_Windows(t *testing.T) {
	fs := windowsFilesystemJSON(map[string]windowsDrive{
		"C:": {KBSize: mbToKB(20480), KBAvailable: mbToKB(10240)},
	})
	v := EvaluateDisk(fs, "windows", diskCfg())
	if v.Sufficient == nil || !*v.Sufficient {
		t.Fatalf("expected sufficient=true on windows C:, got %v", v.Sufficient)
	}
}
