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

// ---------------------------------------------------------------------------
// Contract: partial search delivers ONLY the by_pair section
// ---------------------------------------------------------------------------
//
// The node partial search requests ["filesystem","by_pair"] rather than the
// whole filesystem subtree, because by_device/by_mountpoint roughly triple the
// payload for no benefit (measured on a real Ohai 18 Linux node: by_pair 4496
// bytes, by_mountpoint 3498, by_device 3246).
//
// by_pair is the only section that can be used: its entries carry a "mount"
// field, which findBestMountLinux requires and skips entries without. Real
// by_mountpoint entries key on the mount and omit the field entirely, so
// narrowing to that section would silently make every Linux disk verdict
// unknown. These tests pin that contract — the JSON below is the real shape
// returned by Ohai, not a synthetic fixture.

func TestEvaluateDisk_ByPairOnlyLinux(t *testing.T) {
	// Real Ohai 18 shape: the value delivered under "filesystem" is the
	// by_pair map itself, keyed "device,mount".
	raw := json.RawMessage(`{
		"/dev/mapper/ubuntu--vg-ubuntu--lv,/": {
			"mount": "/", "fs_type": "ext4",
			"kb_size": 50633164, "kb_used": 12000000, "kb_available": 36000000, "percent_used": 25
		},
		"/dev/sda,": {"fs_type": "ext4"},
		"/dev/sda2,/boot": {
			"mount": "/boot", "fs_type": "ext4",
			"kb_size": 1992552, "kb_used": 200000, "kb_available": 1700000, "percent_used": 11
		}
	}`)

	v := EvaluateDisk(raw, "ubuntu", DiskConfig{
		InstallPathLinux:     "/hab",
		InstallSizeMBLinux:   2048,
		InstallPathWindows:   `C:\hab`,
		InstallSizeMBWindows: 2048,
	})

	if v.AvailableMB == nil {
		t.Fatal("expected an available-MB reading from the by_pair section; a nil result means the disk verdict silently broke")
	}
	// / has 36000000 KB available -> ~35156 MB.
	if got := *v.AvailableMB; got < 35000 || got > 35200 {
		t.Errorf("AvailableMB = %d, want ~35156 (from the / mount)", got)
	}
	if v.Sufficient == nil || !*v.Sufficient {
		t.Error("expected sufficient disk space for a 2048MB install on 35GB free")
	}
}

func TestEvaluateDisk_ByPairOnlyWindows(t *testing.T) {
	// Real Ohai shape on Windows: by_pair keys are ",C:" — the device half is
	// empty, so the key never matches the drive letter and the verdict depends
	// entirely on the entry's "mount" field.
	raw := json.RawMessage(`{
		",C:": {"mount": "C:", "kb_size": 41940988, "kb_used": 23648000, "kb_available": 18292989, "percent_used": 56},
		",Z:": {"mount": "Z:", "kb_size": 0, "kb_used": 0, "kb_available": 0},
		"new volume,D:": {"mount": "D:", "kb_size": 52427260, "kb_available": 32937435}
	}`)

	v := EvaluateDisk(raw, "windows", DiskConfig{
		InstallPathLinux:     "/hab",
		InstallSizeMBLinux:   2048,
		InstallPathWindows:   `C:\hab`,
		InstallSizeMBWindows: 2048,
	})

	if v.AvailableMB == nil {
		t.Fatal("expected an available-MB reading for C: from the by_pair section")
	}
	// C: has 18292989 KB available -> ~17864 MB.
	if got := *v.AvailableMB; got < 17800 || got > 17900 {
		t.Errorf("AvailableMB = %d, want ~17864 (from C:)", got)
	}
}

func TestEvaluateDisk_ByMountpointShapeIsUnusable(t *testing.T) {
	// Documents WHY the search requests by_pair and not by_mountpoint: real
	// by_mountpoint entries omit "mount", so no entry can be matched. If a
	// future change points the search at by_mountpoint, this test explains
	// the resulting silent "unknown" verdicts.
	raw := json.RawMessage(`{
		"/":     {"devices": ["/dev/mapper/ubuntu--vg-ubuntu--lv"], "kb_size": 50633164, "kb_available": 36000000},
		"/boot": {"devices": ["/dev/sda2"], "kb_size": 1992552, "kb_available": 1700000}
	}`)

	v := EvaluateDisk(raw, "ubuntu", DiskConfig{
		InstallPathLinux:   "/hab",
		InstallSizeMBLinux: 2048,
	})

	if v.AvailableMB != nil {
		t.Errorf("by_mountpoint entries have no mount field, so no verdict is possible; got AvailableMB = %d", *v.AvailableMB)
	}
}
