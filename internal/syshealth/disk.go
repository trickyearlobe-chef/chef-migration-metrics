// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package syshealth

import "syscall"

// diskUsage returns the total and free bytes for the filesystem containing
// the given path. It uses syscall.Statfs which is available on Linux,
// macOS, and other Unix-like systems.
func diskUsage(path string) (total, free uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	// Bsize is the fundamental block size; Blocks is the total number of
	// blocks; Bavail is the number of free blocks available to unprivileged
	// users (more accurate than Bfree for real available space).
	total = uint64(stat.Bsize) * stat.Blocks
	free = uint64(stat.Bsize) * stat.Bavail
	return total, free, nil
}
