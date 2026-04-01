// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"gopkg.in/yaml.v3"
)

// YAMLMigrateResult records the outcome of a YAML-to-config-store migration.
type YAMLMigrateResult struct {
	SectionsMigrated int
	Skipped          bool
	SkipReason       string
}

// IsFullYAML returns true when the parsed Config contains at least one
// organisation — indicating it was loaded from a full YAML config file
// rather than a bootstrap-only file. This is the detection heuristic
// described in the encrypted-config-store specification.
func IsFullYAML(cfg *config.Config) bool {
	return len(cfg.Organisations) > 0
}

// MigrateFromYAML migrates a full YAML configuration into the encrypted
// config store. Each config section is serialised to JSON and encrypted
// into the database. After migration the original YAML file is renamed
// to config.yml.migrated and a minimal bootstrap file is written in its
// place.
//
// The migration is idempotent:
//   - If config_store already has entries, migration is skipped (the DB
//     config takes precedence over the file).
//   - If the YAML file does not exist, migration is skipped silently.
//   - If a .migrated backup already exists, the backup is not overwritten;
//     the original file is still removed and the bootstrap is written.
//
// The cfg parameter should be the already-parsed and validated Config
// from the full YAML file. The yamlPath is the filesystem path to the
// original config.yml.
func MigrateFromYAML(ctx context.Context, store *Store, cfg *config.Config, yamlPath string) (*YAMLMigrateResult, error) {
	result := &YAMLMigrateResult{}

	// If the YAML file doesn't exist, nothing to migrate.
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		result.Skipped = true
		result.SkipReason = "YAML config file does not exist"
		return result, nil
	}

	// Check if config_store already has entries.
	empty, err := store.IsEmpty(ctx)
	if err != nil {
		return nil, fmt.Errorf("yaml migrate: check config_store empty: %w", err)
	}
	if !empty {
		result.Skipped = true
		result.SkipReason = "config_store already has entries; YAML config is ignored"
		return result, nil
	}

	// Serialise each config section to JSON.
	sections, err := ConfigToSections(cfg)
	if err != nil {
		return nil, fmt.Errorf("yaml migrate: serialise config sections: %w", err)
	}

	// Encrypt and store each section.
	for key, value := range sections {
		if err := store.Set(ctx, key, json.RawMessage(value), false, "yaml-migration"); err != nil {
			return nil, fmt.Errorf("yaml migrate: store section %q: %w", key, err)
		}
		result.SectionsMigrated++
	}

	// Rename original YAML to .migrated (preserve existing backup).
	backupPath := yamlPath + ".migrated"
	if _, statErr := os.Stat(backupPath); os.IsNotExist(statErr) {
		if err := os.Rename(yamlPath, backupPath); err != nil {
			return nil, fmt.Errorf("yaml migrate: rename %s to %s: %w", yamlPath, backupPath, err)
		}
	} else {
		// Backup already exists — just remove the original.
		if err := os.Remove(yamlPath); err != nil {
			return nil, fmt.Errorf("yaml migrate: remove %s: %w", yamlPath, err)
		}
	}

	// Write minimal bootstrap YAML.
	if err := writeBootstrapYAML(yamlPath, cfg); err != nil {
		return nil, fmt.Errorf("yaml migrate: write bootstrap: %w", err)
	}

	return result, nil
}

// bootstrapYAML is the struct serialised into the bootstrap config file.
// It contains only the values needed before the database is available.
type bootstrapYAML struct {
	DatabaseURL   string `yaml:"database_url"`
	ListenAddress string `yaml:"listen_address"`
	ListenPort    int    `yaml:"listen_port"`
}

// writeBootstrapYAML writes a minimal bootstrap config.yml containing only
// database_url, listen_address, and listen_port. The file is created with
// 0600 permissions since it may contain a database password.
func writeBootstrapYAML(path string, cfg *config.Config) error {
	bootstrap := bootstrapYAML{
		DatabaseURL:   cfg.Datastore.URL,
		ListenAddress: cfg.Server.ListenAddress,
		ListenPort:    cfg.Server.Port,
	}

	data, err := yaml.Marshal(&bootstrap)
	if err != nil {
		return fmt.Errorf("marshal bootstrap YAML: %w", err)
	}

	header := []byte("# Bootstrap configuration — all other settings are in the database.\n# See config.yml.migrated for the original full configuration.\n")
	content := append(header, data...)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write bootstrap file: %w", err)
	}

	return nil
}
