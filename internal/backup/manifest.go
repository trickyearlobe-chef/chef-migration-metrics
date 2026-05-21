// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WriteManifest writes a manifest as a JSON sidecar file in the backup directory.
func WriteManifest(dir string, m Manifest) error {
	path := manifestPath(dir, m.ID)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("backup: marshal manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("backup: write manifest %s: %w", path, err)
	}
	return nil
}

// ReadManifest reads a manifest from its sidecar JSON file.
func ReadManifest(dir, id string) (Manifest, error) {
	path := manifestPath(dir, id)
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: read manifest %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("backup: unmarshal manifest %s: %w", path, err)
	}
	return m, nil
}

// ListManifests reads all manifest files from the backup directory and returns
// them sorted by creation time (newest first).
func ListManifests(dir string) ([]Manifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("backup: list dir %s: %w", dir, err)
	}

	var manifests []Manifest
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		m, err := ReadManifest(dir, id)
		if err != nil {
			continue // skip unreadable manifests
		}
		manifests = append(manifests, m)
	}

	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].CreatedAt.After(manifests[j].CreatedAt)
	})
	return manifests, nil
}

// DeleteManifest removes the manifest sidecar file for the given backup ID.
func DeleteManifest(dir, id string) error {
	path := manifestPath(dir, id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("backup: delete manifest %s: %w", path, err)
	}
	return nil
}

func manifestPath(dir, id string) string {
	return filepath.Join(dir, id+".json")
}
