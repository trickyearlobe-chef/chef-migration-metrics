// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package pathsafe provides helpers for safely composing filesystem paths from
// externally-influenced components (cookbook/role names, Chef manifest paths),
// guarding against path-traversal escapes from a base directory.
package pathsafe

import (
	"fmt"
	"path/filepath"
)

// SafeJoin cleans relPath, joins it under baseDir, and returns the resulting
// path only if it stays within baseDir. It rejects absolute paths and any
// path that escapes baseDir via "..". Use it wherever an external value
// (a cookbook/role name, a Chef cookbook manifest path) becomes part of a
// filesystem path.
func SafeJoin(baseDir, relPath string) (string, error) {
	clean := filepath.Clean(relPath)
	if filepath.IsAbs(clean) || clean == ".." || hasParentTraversal(clean) {
		return "", fmt.Errorf("pathsafe: unsafe path %q", relPath)
	}

	full := filepath.Join(baseDir, clean)

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("pathsafe: resolving base directory: %w", err)
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", fmt.Errorf("pathsafe: resolving path: %w", err)
	}
	if !isSubPath(absBase, absFull) {
		return "", fmt.Errorf("pathsafe: path %q escapes base directory", relPath)
	}
	return full, nil
}

func hasParentTraversal(cleanPath string) bool {
	for _, part := range splitPathComponents(cleanPath) {
		if part == ".." {
			return true
		}
	}
	return false
}

func splitPathComponents(p string) []string {
	var parts []string
	for {
		dir, file := filepath.Split(p)
		if file != "" {
			parts = append([]string{file}, parts...)
		}
		if dir == "" || dir == p {
			break
		}
		p = filepath.Clean(dir)
	}
	return parts
}

func isSubPath(parent, child string) bool {
	parentPrefix := parent
	if parentPrefix != "/" && parentPrefix[len(parentPrefix)-1] != filepath.Separator {
		parentPrefix += string(filepath.Separator)
	}
	return child == parent || len(child) > len(parentPrefix) && child[:len(parentPrefix)] == parentPrefix
}
