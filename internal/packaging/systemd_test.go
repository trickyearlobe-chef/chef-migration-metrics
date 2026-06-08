// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package packaging holds tests that guard the shipped deployment artifacts
// (systemd unit, maintainer scripts) against regressions. There is no library
// code here — the artifacts live under deploy/pkg/.
package packaging

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// unitFile is the shipped systemd unit, relative to this test's directory.
const unitFile = "../../deploy/pkg/chef-migration-metrics.service"

func readUnit(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(unitFile)
	if err != nil {
		t.Fatalf("read unit file %s: %v", unitFile, err)
	}
	return string(b)
}

// The non-root service must carry CAP_NET_BIND_SERVICE so it can bind 80/443.
// The capability has to be both granted (AmbientCapabilities) and permitted by
// the bounding set, and it must coexist with NoNewPrivileges=true — the whole
// point is that ambient caps survive the privilege drop where setcap would not.
func TestUnitGrantsNetBindCapability(t *testing.T) {
	unit := readUnit(t)
	for _, want := range []string{
		"AmbientCapabilities=CAP_NET_BIND_SERVICE",
		"CapabilityBoundingSet=CAP_NET_BIND_SERVICE",
		"NoNewPrivileges=true",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit file missing required directive %q", want)
		}
	}
}

// When systemd-analyze is available (CI on Linux), confirm the unit actually
// parses with no errors. Skipped on hosts without it (e.g. dev macOS) so the
// suite stays green everywhere.
func TestUnitParsesWithSystemdAnalyze(t *testing.T) {
	bin, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Skip("systemd-analyze not available; skipping unit-file parse check")
	}
	out, err := exec.Command(bin, "verify", unitFile).CombinedOutput()
	if err != nil {
		t.Fatalf("systemd-analyze verify failed: %v\n%s", err, out)
	}
}
