// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// ConfigSectionKey constants define the config_store keys for each
// configuration section. These match the key naming convention in the
// encrypted-config-store specification.
const (
	KeyOrganisations              = "organisations"
	KeyTargetChefVersions         = "target_chef_versions"
	KeyGitBaseURLs                = "git_base_urls"
	KeyCollection                 = "collection"
	KeyConcurrency                = "concurrency"
	KeyAnalysisTools              = "analysis_tools"
	KeyReadiness                  = "readiness"
	KeyNotifications              = "notifications"
	KeySMTP                       = "smtp"
	KeyExports                    = "exports"
	KeyElasticsearch              = "elasticsearch"
	KeyServerTLS                  = "server.tls"
	KeyServerWebSocket            = "server.websocket"
	KeyServerGracefulShutdown     = "server.graceful_shutdown_seconds"
	KeyFrontend                   = "frontend"
	KeyLogging                    = "logging"
	KeyAuth                       = "auth"
	KeyOwnership                  = "ownership"
	KeyStorage                    = "storage"
	KeySystemHealth               = "system_health"
	KeyPerformance                = "performance"
	KeyCredentialEncryptionKeyEnv = "credential_encryption_key_env"
)

// AllConfigKeys returns the complete list of known config section keys in
// the order they should be processed. This is useful for YAML auto-migration
// to ensure all sections are captured.
func AllConfigKeys() []string {
	return []string{
		KeyOrganisations,
		KeyTargetChefVersions,
		KeyGitBaseURLs,
		KeyCollection,
		KeyConcurrency,
		KeyAnalysisTools,
		KeyReadiness,
		KeyNotifications,
		KeySMTP,
		KeyExports,
		KeyElasticsearch,
		KeyServerTLS,
		KeyServerWebSocket,
		KeyServerGracefulShutdown,
		KeyFrontend,
		KeyLogging,
		KeyAuth,
		KeyOwnership,
		KeyStorage,
		KeySystemHealth,
		KeyPerformance,
		KeyCredentialEncryptionKeyEnv,
	}
}

// AssembleConfig builds a config.Config struct from the decrypted entries in
// the config store. Each known key is unmarshalled into the corresponding
// Config field. After assembly, defaults are applied and the config is
// validated using the same rules as the YAML loading path.
//
// Bootstrap values (database_url, listen_address, listen_port) are NOT
// populated by this function — they come from the bootstrap YAML file and
// must be set on the returned Config by the caller.
//
// Missing optional sections are silently skipped — defaults will be applied.
// Unknown keys in the store are ignored.
func AssembleConfig(ctx context.Context, store *Store) (*config.Config, *config.Warnings, error) {
	values, err := store.GetAll(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("configstore: assemble: get all entries: %w", err)
	}

	cfg := &config.Config{}

	if err := assembleFields(cfg, values); err != nil {
		return nil, nil, fmt.Errorf("configstore: assemble: %w", err)
	}

	// Apply defaults and validate — same path as config.Parse().
	cfg.ApplyDefaults()

	warnings, valErr := cfg.Validate()
	if valErr != nil {
		return cfg, warnings, valErr
	}

	return cfg, warnings, nil
}

// AssembleConfigRaw builds a config.Config from a pre-fetched map of
// decrypted values without calling the store. This is useful for testing
// and for the YAML migration path where values are already in memory.
//
// Unlike AssembleConfig, this does NOT apply defaults or validate — the
// caller is responsible for that.
func AssembleConfigRaw(values map[string]json.RawMessage) (*config.Config, error) {
	cfg := &config.Config{}
	if err := assembleFields(cfg, values); err != nil {
		return nil, err
	}
	return cfg, nil
}

// assembleFields unmarshals each known config key from the values map into
// the corresponding field on cfg. Unknown keys are silently ignored.
func assembleFields(cfg *config.Config, values map[string]json.RawMessage) error {
	for key, raw := range values {
		if err := assembleOneField(cfg, key, raw); err != nil {
			return fmt.Errorf("key %q: %w", key, err)
		}
	}
	return nil
}

// assembleOneField unmarshals a single config key's JSON value into the
// appropriate field on the Config struct. Returns nil for unknown keys.
func assembleOneField(cfg *config.Config, key string, raw json.RawMessage) error {
	switch key {
	case KeyOrganisations:
		return unmarshalInto(&cfg.Organisations, raw, key)
	case KeyTargetChefVersions:
		return unmarshalInto(&cfg.TargetChefVersions, raw, key)
	case KeyGitBaseURLs:
		return unmarshalInto(&cfg.GitBaseURLs, raw, key)
	case KeyCollection:
		return unmarshalInto(&cfg.Collection, raw, key)
	case KeyConcurrency:
		return unmarshalInto(&cfg.Concurrency, raw, key)
	case KeyAnalysisTools:
		return unmarshalInto(&cfg.AnalysisTools, raw, key)
	case KeyReadiness:
		return unmarshalInto(&cfg.Readiness, raw, key)
	case KeyNotifications:
		return unmarshalInto(&cfg.Notifications, raw, key)
	case KeySMTP:
		return unmarshalInto(&cfg.SMTP, raw, key)
	case KeyExports:
		return unmarshalInto(&cfg.Exports, raw, key)
	case KeyElasticsearch:
		return unmarshalInto(&cfg.Elasticsearch, raw, key)
	case KeyServerTLS:
		return unmarshalInto(&cfg.Server.TLS, raw, key)
	case KeyServerWebSocket:
		return unmarshalInto(&cfg.Server.WebSocket, raw, key)
	case KeyServerGracefulShutdown:
		return unmarshalInto(&cfg.Server.GracefulShutdownSeconds, raw, key)
	case KeyFrontend:
		return unmarshalInto(&cfg.Frontend, raw, key)
	case KeyLogging:
		return unmarshalInto(&cfg.Logging, raw, key)
	case KeyAuth:
		return unmarshalInto(&cfg.Auth, raw, key)
	case KeyOwnership:
		return unmarshalInto(&cfg.Ownership, raw, key)
	case KeyStorage:
		return unmarshalInto(&cfg.Storage, raw, key)
	case KeySystemHealth:
		return unmarshalInto(&cfg.SystemHealth, raw, key)
	case KeyPerformance:
		return unmarshalInto(&cfg.Performance, raw, key)
	case KeyCredentialEncryptionKeyEnv:
		return unmarshalInto(&cfg.CredentialEncryptionKeyEnv, raw, key)
	default:
		// Unknown key — silently ignore. This allows forward compatibility
		// when new config sections are added.
		return nil
	}
}

// unmarshalInto is a typed helper that unmarshals JSON into a target pointer,
// wrapping the error with the config key name for diagnostics.
func unmarshalInto[T any](target *T, raw json.RawMessage, key string) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("unmarshal %q: %w", key, err)
	}
	return nil
}

// ConfigToSections serialises a config.Config struct into a map of config
// store key → JSON value. This is the inverse of assembleFields and is used
// during YAML auto-migration to populate the config store from a parsed
// Config struct.
//
// Bootstrap-only fields (Datastore.URL, Server.ListenAddress, Server.Port)
// are excluded — they remain in the bootstrap YAML file.
func ConfigToSections(cfg *config.Config) (map[string]json.RawMessage, error) {
	sections := map[string]any{
		KeyOrganisations:          cfg.Organisations,
		KeyTargetChefVersions:     cfg.TargetChefVersions,
		KeyGitBaseURLs:            cfg.GitBaseURLs,
		KeyCollection:             cfg.Collection,
		KeyConcurrency:            cfg.Concurrency,
		KeyAnalysisTools:          cfg.AnalysisTools,
		KeyReadiness:              cfg.Readiness,
		KeyNotifications:          cfg.Notifications,
		KeySMTP:                   cfg.SMTP,
		KeyExports:                cfg.Exports,
		KeyElasticsearch:          cfg.Elasticsearch,
		KeyServerTLS:              cfg.Server.TLS,
		KeyServerWebSocket:        cfg.Server.WebSocket,
		KeyServerGracefulShutdown: cfg.Server.GracefulShutdownSeconds,
		KeyFrontend:               cfg.Frontend,
		KeyLogging:                cfg.Logging,
		KeyAuth:                   cfg.Auth,
		KeyOwnership:              cfg.Ownership,
		KeyStorage:                cfg.Storage,
		KeySystemHealth:           cfg.SystemHealth,
		KeyPerformance:            cfg.Performance,
	}

	// Only include credential_encryption_key_env if it was explicitly set
	// to a non-default value.
	if cfg.CredentialEncryptionKeyEnv != "" && cfg.CredentialEncryptionKeyEnv != "CMM_CREDENTIAL_ENCRYPTION_KEY" {
		sections[KeyCredentialEncryptionKeyEnv] = cfg.CredentialEncryptionKeyEnv
	}

	result := make(map[string]json.RawMessage, len(sections))
	for key, value := range sections {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("configstore: marshal section %q: %w", key, err)
		}
		result[key] = json.RawMessage(data)
	}

	return result, nil
}
