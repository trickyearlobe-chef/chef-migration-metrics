// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package syshealth

import "syscall"

// diskUsageWithDevice returns the total and free bytes for the filesystem
// containing the given path, along with the device ID (Statfs_t.Dev is
// not available on Linux, so we use syscall.Stat_t.Dev from a stat call)
// used to de-duplicate multiple paths that resolve to the same underlying
// filesystem.
func diskUsageWithDevice(path string) (total, free, deviceID uint64, err error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0, 0, 0, err
	}
	// Bsize is the fundamental block size; Blocks is the total number of
	// blocks; Bavail is the number of free blocks available to unprivileged
	// users (more accurate than Bfree for real available space).
	total = uint64(fs.Bsize) * fs.Blocks
	free = uint64(fs.Bsize) * fs.Bavail

	// Linux Statfs_t does not expose a device ID directly. Use
	// syscall.Stat on the path to obtain the underlying device number
	// (Stat_t.Dev), which uniquely identifies the mounted filesystem.
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return 0, 0, 0, err
	}
	deviceID = st.Dev
	return total, free, deviceID, nil
}
