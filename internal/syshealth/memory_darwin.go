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

// memoryUsage returns the total and available memory in bytes on macOS.
//
// Total memory is read from sysctl hw.memsize (returns bytes directly).
//
// Available memory is estimated by summing the "free" and "inactive"
// page counts from vm_stat and multiplying by the page size. This is an
// approximation — macOS does not expose a single "MemAvailable" metric
// like Linux does, but free+inactive closely matches what Activity
// Monitor reports as available for use.
func memoryUsage() (total, available uint64, err error) {
	// Total physical memory.
	totalBytes, err := sysctlUint64("hw.memsize")
	if err != nil {
		return 0, 0, fmt.Errorf("syshealth: reading hw.memsize: %w", err)
	}

	// Page size and page counts from vm_stat.
	pageSize, err := sysctlUint64("hw.pagesize")
	if err != nil {
		return 0, 0, fmt.Errorf("syshealth: reading hw.pagesize: %w", err)
	}

	freePages, inactivePages, err := vmStatFreeInactive()
	if err != nil {
		return 0, 0, fmt.Errorf("syshealth: reading vm_stat: %w", err)
	}

	availBytes := (freePages + inactivePages) * pageSize
	return totalBytes, availBytes, nil
}

// sysctlUint64 reads a numeric sysctl value.
func sysctlUint64(name string) (uint64, error) {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(out))
	return strconv.ParseUint(s, 10, 64)
}

// vmStatFreeInactive parses the output of vm_stat to extract the
// "Pages free" and "Pages inactive" counts. vm_stat output looks like:
//
//	Mach Virtual Memory Statistics: (page size of 16384 bytes)
//	Pages free:                               12345.
//	Pages active:                             67890.
//	Pages inactive:                           11111.
//	...
func vmStatFreeInactive() (free, inactive uint64, err error) {
	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0, 0, err
	}

	var gotFree, gotInactive bool
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Pages free:") {
			free, err = parseVMStatLine(line)
			if err != nil {
				return 0, 0, err
			}
			gotFree = true
		} else if strings.HasPrefix(line, "Pages inactive:") {
			inactive, err = parseVMStatLine(line)
			if err != nil {
				return 0, 0, err
			}
			gotInactive = true
		}
		if gotFree && gotInactive {
			break
		}
	}

	// vm_stat is best-effort; if fields are missing return what we have.
	return free, inactive, nil
}

// parseVMStatLine parses a vm_stat line like "Pages free:   12345." and
// returns the numeric value. The trailing period is stripped.
func parseVMStatLine(line string) (uint64, error) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return 0, fmt.Errorf("unexpected vm_stat line: %q", line)
	}
	s := strings.TrimSpace(parts[1])
	s = strings.TrimSuffix(s, ".")
	return strconv.ParseUint(s, 10, 64)
}
