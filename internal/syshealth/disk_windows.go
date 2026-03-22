// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package syshealth

import "fmt"

// diskUsageWithDevice is a stub on Windows. The syscall.Statfs approach
// used on Unix-like systems is not available. A full implementation would
// use GetDiskFreeSpaceExW via golang.org/x/sys/windows.
func diskUsageWithDevice(path string) (total, free, deviceID uint64, err error) {
	return 0, 0, 0, fmt.Errorf("syshealth: disk usage not supported on Windows")
}
