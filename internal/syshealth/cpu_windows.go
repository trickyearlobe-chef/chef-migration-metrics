// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package syshealth

import "fmt"

// loadAvg1 is a stub on Windows. Load average is a Unix concept and is
// not natively available on Windows. A full implementation could use
// performance counters via PDH, but that requires cgo or x/sys/windows.
func loadAvg1() (float64, error) {
	return 0, fmt.Errorf("syshealth: load average not supported on Windows")
}
