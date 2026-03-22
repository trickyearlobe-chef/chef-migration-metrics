// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package syshealth

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// loadAvg1 returns the 1-minute load average on macOS by running
// sysctl -n vm.loadavg. The output format is: "{ 1.23 4.56 7.89 }"
func loadAvg1() (float64, error) {
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return 0, fmt.Errorf("syshealth: running sysctl vm.loadavg: %w", err)
	}

	// Strip braces and split: "{ 1.23 4.56 7.89 }" → ["1.23", "4.56", "7.89"]
	s := strings.TrimSpace(string(out))
	s = strings.Trim(s, "{ }")
	fields := strings.Fields(s)
	if len(fields) < 1 {
		return 0, fmt.Errorf("syshealth: unexpected sysctl vm.loadavg output: %q", string(out))
	}

	load, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("syshealth: parsing load average %q: %w", fields[0], err)
	}

	return load, nil
}
