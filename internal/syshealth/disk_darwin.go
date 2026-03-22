// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package syshealth

import "syscall"

// diskUsageWithDevice returns the total and free bytes for the filesystem
// containing the given path, along with a device identifier derived from
// Fsid used to de-duplicate multiple paths that resolve to the same
// underlying filesystem.
func diskUsageWithDevice(path string) (total, free, deviceID uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, err
	}
	// Bsize is the fundamental block size; Blocks is the total number of
	// blocks; Bavail is the number of free blocks available to unprivileged
	// users (more accurate than Bfree for real available space).
	total = uint64(stat.Bsize) * stat.Blocks
	free = uint64(stat.Bsize) * stat.Bavail
	// macOS Statfs_t does not have a Dev field. Use Fsid which uniquely
	// identifies the mounted filesystem. Combine its two int32 values
	// into a single uint64 for use as a map key.
	deviceID = uint64(stat.Fsid.Val[0]) | uint64(stat.Fsid.Val[1])<<32
	return total, free, deviceID, nil
}
