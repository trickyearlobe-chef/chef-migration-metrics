// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package syshealth

import "fmt"

// memoryUsage is a stub on Windows. A full implementation would use
// GlobalMemoryStatusEx via golang.org/x/sys/windows.
func memoryUsage() (total, available uint64, err error) {
	return 0, 0, fmt.Errorf("syshealth: memory usage not supported on Windows")
}
