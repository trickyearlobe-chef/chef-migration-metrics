// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package syshealth

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// memoryUsage returns the total and available memory in bytes by parsing
// /proc/meminfo. It looks for the "MemTotal" and "MemAvailable" lines.
//
// Example /proc/meminfo lines:
//
//	MemTotal:       16384000 kB
//	MemAvailable:    8192000 kB
func memoryUsage() (total, available uint64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, fmt.Errorf("syshealth: opening /proc/meminfo: %w", err)
	}
	defer f.Close()

	var gotTotal, gotAvail bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "MemTotal:") {
			total, err = parseMemInfoLine(line)
			if err != nil {
				return 0, 0, err
			}
			gotTotal = true
		} else if strings.HasPrefix(line, "MemAvailable:") {
			available, err = parseMemInfoLine(line)
			if err != nil {
				return 0, 0, err
			}
			gotAvail = true
		}

		if gotTotal && gotAvail {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("syshealth: reading /proc/meminfo: %w", err)
	}

	if !gotTotal {
		return 0, 0, fmt.Errorf("syshealth: MemTotal not found in /proc/meminfo")
	}
	if !gotAvail {
		return 0, 0, fmt.Errorf("syshealth: MemAvailable not found in /proc/meminfo")
	}

	return total, available, nil
}

// parseMemInfoLine parses a /proc/meminfo line of the form "Key:  12345 kB"
// and returns the value in bytes. The unit is assumed to be kB (kibibytes).
func parseMemInfoLine(line string) (uint64, error) {
	// "MemTotal:       16384000 kB" → ["MemTotal:", "16384000", "kB"]
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, fmt.Errorf("syshealth: unexpected meminfo line format: %q", line)
	}

	kb, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("syshealth: parsing meminfo value %q: %w", fields[1], err)
	}

	// /proc/meminfo reports in kB (kibibytes = 1024 bytes).
	return kb * 1024, nil
}
