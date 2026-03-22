// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package syshealth

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// loadAvg1 returns the 1-minute load average by reading /proc/loadavg.
// The file format is: "0.52 0.58 0.59 2/1234 56789"
// where the first three fields are the 1, 5, and 15 minute load averages.
func loadAvg1() (float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, fmt.Errorf("syshealth: reading /proc/loadavg: %w", err)
	}

	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("syshealth: unexpected /proc/loadavg format")
	}

	load, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("syshealth: parsing load average %q: %w", fields[0], err)
	}

	return load, nil
}
