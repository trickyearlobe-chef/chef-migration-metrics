// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"fmt"
	"sync"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// ConfigHolder provides concurrent-safe access to the application's
// configuration. It holds a pointer to the current config.Config that can
// be atomically swapped when the configuration is reloaded from the
// database.
//
// HTTP handlers and background workers call Get() to obtain the current
// config. The collector calls Reload() at the start of each collection
// run to pick up any changes made through the admin UI.
//
// ConfigHolder is safe for concurrent use by multiple goroutines.
type ConfigHolder struct {
	mu    sync.RWMutex
	cfg   *config.Config
	store *Store
}

// NewConfigHolder creates a ConfigHolder with an initial configuration.
// The store is used by Reload() to re-assemble config from the database.
// If store is nil, Reload() will return an error — use Set() instead for
// testing or bootstrap scenarios.
func NewConfigHolder(cfg *config.Config, store *Store) *ConfigHolder {
	return &ConfigHolder{
		cfg:   cfg,
		store: store,
	}
}

// Get returns the current configuration. The returned pointer is safe to
// use without holding any lock — the Config struct is treated as immutable
// once published. Callers MUST NOT modify the returned Config.
func (h *ConfigHolder) Get() *config.Config {
	h.mu.RLock()
	cfg := h.cfg
	h.mu.RUnlock()
	return cfg
}

// Set replaces the current configuration with a new one. This is used
// during initial startup and for testing. For normal operation, prefer
// Reload() which re-assembles from the database.
func (h *ConfigHolder) Set(cfg *config.Config) {
	h.mu.Lock()
	h.cfg = cfg
	h.mu.Unlock()
}

// Reload re-reads all configuration from the config store, assembles a
// new config.Config struct, validates it, and atomically swaps the pointer.
// If assembly or validation fails, the existing config is preserved and
// the error is returned.
//
// Bootstrap values (database_url, listen_address, listen_port) are copied
// from the current config since they are not stored in the database.
func (h *ConfigHolder) Reload(ctx context.Context) error {
	if h.store == nil {
		return fmt.Errorf("configstore: reload: no store configured")
	}

	newCfg, warnings, err := AssembleConfig(ctx, h.store)
	if err != nil {
		return fmt.Errorf("configstore: reload: %w", err)
	}

	// Carry over bootstrap values from the current config. These are not
	// stored in the database — they come from the bootstrap YAML file and
	// environment variables.
	h.mu.RLock()
	current := h.cfg
	h.mu.RUnlock()

	if current != nil {
		newCfg.Datastore.URL = current.Datastore.URL
		newCfg.Server.ListenAddress = current.Server.ListenAddress
		newCfg.Server.Port = current.Server.Port
	}

	// Warnings are non-fatal — log them if a logger is available, but
	// don't prevent the swap.
	_ = warnings

	h.mu.Lock()
	h.cfg = newCfg
	h.mu.Unlock()

	return nil
}

// Store returns the underlying config store, or nil if none was provided.
// This is used by callers that need to read or write individual config
// entries (e.g. API handlers).
func (h *ConfigHolder) Store() *Store {
	return h.store
}
