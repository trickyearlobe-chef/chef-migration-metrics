// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"gopkg.in/yaml.v3"
)

// HasKey reports whether the given config key exists in the store. It is used
// to decide whether a config section came from the database (authoritative)
// or is absent and should fall back to a bootstrap/default value — notably for
// server.listen, where the DB copy overrides the bootstrap YAML when present.
func HasKey(ctx context.Context, store *Store, key string) (bool, error) {
	_, err := store.Get(ctx, key)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}

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
	KeyExports                    = "exports"
	KeyElasticsearch              = "elasticsearch"
	KeyServerListen               = "server.listen"
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
	KeyBackup                     = "backup"
	KeyCredentialEncryptionKeyEnv = "credential_encryption_key_env"

	// DB cert_source TLS material. These are standalone config-store entries,
	// NOT config sections: they are written and read directly by the admin TLS
	// save path and the static-mode listener wiring, and are deliberately
	// excluded from AllConfigKeys/ConfigToSections so config assembly never
	// tries to fold PEM material into a config struct. The certificate is
	// stored non-secret (public); the private key is stored secret (encrypted,
	// never returned by any API). See tls-static.md § 2.7.
	KeyServerTLSCertificate       = "server.tls.certificate"
	KeyServerTLSPrivateKey        = "server.tls.private_key"
	KeyServerTLSPrivateKeyPending = "server.tls.private_key.pending"

	// ACME state material. Like the static cert keys above, these are
	// standalone config-store entries written and read directly by the ACME
	// engine (internal/acme), NOT config sections — they are excluded from
	// AllConfigKeys/ConfigToSections so config assembly never folds key/cert
	// material into a config struct. The account key and issued private key are
	// secret (encrypted, never returned by any API); the issued cert is public.
	// See tls-acme.md § 3.5.
	KeyServerTLSACMEAccountKey = "server.tls.acme.account_key"
	KeyServerTLSACMECert       = "server.tls.acme.cert"
	KeyServerTLSACMEKey        = "server.tls.acme.key"
)

// ServerListenSection is the JSON/YAML shape of the `server.listen` config
// store section. It holds the listen address and port, which are DB-managed
// and UI-editable (the bootstrap YAML keeps a copy only as the bind-failure
// fallback — see encrypted-config-store.md).
type ServerListenSection struct {
	ListenAddress string `yaml:"listen_address" json:"listen_address"`
	Port          int    `yaml:"port" json:"port"`
}

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
		KeyExports,
		KeyElasticsearch,
		KeyServerListen,
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
		KeyBackup,
		KeyCredentialEncryptionKeyEnv,
	}
}

// AssembleConfig builds a config.Config struct from the decrypted entries in
// the config store. Each known key is unmarshalled into the corresponding
// Config field. After assembly, defaults are applied and the config is
// validated using the same rules as the YAML loading path.
//
// The config structs use `yaml` struct tags (not `json`), so we use YAML
// unmarshal for the JSON→struct step. JSON is a subset of YAML so this
// works correctly and honours the snake_case field names.
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
//
// We use yaml.Unmarshal because the config structs have `yaml` struct tags
// (e.g. `yaml:"chef_server_url"`) rather than `json` tags. Since JSON is
// valid YAML, the stored JSON values unmarshal correctly via the YAML decoder.
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
		return yamlUnmarshalInto(&cfg.Organisations, raw, key)
	case KeyTargetChefVersions:
		return yamlUnmarshalInto(&cfg.TargetChefVersions, raw, key)
	case KeyGitBaseURLs:
		return yamlUnmarshalInto(&cfg.GitBaseURLs, raw, key)
	case KeyCollection:
		return yamlUnmarshalInto(&cfg.Collection, raw, key)
	case KeyConcurrency:
		return yamlUnmarshalInto(&cfg.Concurrency, raw, key)
	case KeyAnalysisTools:
		return yamlUnmarshalInto(&cfg.AnalysisTools, raw, key)
	case KeyReadiness:
		return yamlUnmarshalInto(&cfg.Readiness, raw, key)
	case KeyExports:
		return yamlUnmarshalInto(&cfg.Exports, raw, key)
	case KeyElasticsearch:
		return yamlUnmarshalInto(&cfg.Elasticsearch, raw, key)
	case KeyServerListen:
		var listen ServerListenSection
		if err := yamlUnmarshalInto(&listen, raw, key); err != nil {
			return err
		}
		cfg.Server.ListenAddress = listen.ListenAddress
		cfg.Server.Port = listen.Port
		return nil
	case KeyServerTLS:
		return yamlUnmarshalInto(&cfg.Server.TLS, raw, key)
	case KeyServerWebSocket:
		return yamlUnmarshalInto(&cfg.Server.WebSocket, raw, key)
	case KeyServerGracefulShutdown:
		return yamlUnmarshalInto(&cfg.Server.GracefulShutdownSeconds, raw, key)
	case KeyFrontend:
		return yamlUnmarshalInto(&cfg.Frontend, raw, key)
	case KeyLogging:
		return yamlUnmarshalInto(&cfg.Logging, raw, key)
	case KeyAuth:
		return yamlUnmarshalInto(&cfg.Auth, raw, key)
	case KeyOwnership:
		return yamlUnmarshalInto(&cfg.Ownership, raw, key)
	case KeyStorage:
		return yamlUnmarshalInto(&cfg.Storage, raw, key)
	case KeySystemHealth:
		return yamlUnmarshalInto(&cfg.SystemHealth, raw, key)
	case KeyPerformance:
		return yamlUnmarshalInto(&cfg.Performance, raw, key)
	case KeyBackup:
		return yamlUnmarshalInto(&cfg.Backup, raw, key)
	case KeyCredentialEncryptionKeyEnv:
		return yamlUnmarshalInto(&cfg.CredentialEncryptionKeyEnv, raw, key)
	default:
		// Unknown key — silently ignore. This allows forward compatibility
		// when new config sections are added.
		return nil
	}
}

// yamlUnmarshalInto unmarshals a JSON value into a target pointer using the
// YAML decoder. This is necessary because config structs use `yaml` struct
// tags. JSON is a subset of YAML so the YAML decoder handles it correctly.
func yamlUnmarshalInto[T any](target *T, raw json.RawMessage, key string) error {
	if err := yaml.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("unmarshal %q: %w", key, err)
	}
	return nil
}

// ConfigToSections serialises a config.Config struct into a map of config
// store key → JSON value. This is the inverse of assembleFields and is used
// during YAML auto-migration to populate the config store from a parsed
// Config struct.
//
// We serialise via JSON (not YAML) because the stored values in config_store
// are JSON. The config structs have `yaml` tags but json.Marshal falls back
// to using field names when no `json` tag is present. To get consistent
// snake_case keys, we round-trip through YAML marshal → JSON-compatible map
// via yamlToJSON.
//
// Datastore.URL remains a bootstrap-only field (it is excluded — it stays in
// the bootstrap YAML). Server.ListenAddress/Port are written to the
// `server.listen` section (DB is authoritative for UI editing; the bootstrap
// YAML keeps a copy only as the bind-failure fallback).
func ConfigToSections(cfg *config.Config) (map[string]json.RawMessage, error) {
	sections := map[string]any{
		KeyOrganisations:      cfg.Organisations,
		KeyTargetChefVersions: cfg.TargetChefVersions,
		KeyGitBaseURLs:        cfg.GitBaseURLs,
		KeyCollection:         cfg.Collection,
		KeyConcurrency:        cfg.Concurrency,
		KeyAnalysisTools:      cfg.AnalysisTools,
		KeyReadiness:          cfg.Readiness,
		KeyExports:            cfg.Exports,
		KeyElasticsearch:      cfg.Elasticsearch,
		KeyServerListen: ServerListenSection{
			ListenAddress: cfg.Server.ListenAddress,
			Port:          cfg.Server.Port,
		},
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
		KeyBackup:                 cfg.Backup,
	}

	// Only include credential_encryption_key_env if it was explicitly set
	// to a non-default value.
	if cfg.CredentialEncryptionKeyEnv != "" && cfg.CredentialEncryptionKeyEnv != "CMM_CREDENTIAL_ENCRYPTION_KEY" {
		sections[KeyCredentialEncryptionKeyEnv] = cfg.CredentialEncryptionKeyEnv
	}

	result := make(map[string]json.RawMessage, len(sections))
	for key, value := range sections {
		data, err := yamlToJSON(value)
		if err != nil {
			return nil, fmt.Errorf("configstore: marshal section %q: %w", key, err)
		}
		result[key] = json.RawMessage(data)
	}

	return result, nil
}

// SerializeValue serialises any config value to a JSON RawMessage using yaml
// struct tags for field names (snake_case). Use this when you need to
// serialise a config sub-struct (e.g. TestKitchenConfig, ServerConfig) to
// JSON without going through ConfigToSections.
func SerializeValue(v any) (json.RawMessage, error) {
	data, err := yamlToJSON(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// DeserializeValue unmarshals a stored config-store JSON value into v using the
// struct's yaml tags (snake_case), the inverse of SerializeValue. JSON is a
// subset of YAML so the YAML decoder reads the stored JSON value correctly. Use
// this to decode a single section (e.g. server.tls) into its config sub-struct
// without assembling the whole config.
func DeserializeValue(raw json.RawMessage, v any) error {
	if err := yaml.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("configstore: deserialize value: %w", err)
	}
	return nil
}

// yamlToJSON serialises a value to JSON by first marshalling to YAML (which
// uses the struct's yaml tags for field names), then unmarshalling to a
// generic interface, then marshalling to JSON. This ensures the JSON output
// uses the snake_case field names from yaml tags rather than Go PascalCase.
func yamlToJSON(v any) ([]byte, error) {
	yamlBytes, err := yaml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("yaml marshal: %w", err)
	}

	var generic any
	if err := yaml.Unmarshal(yamlBytes, &generic); err != nil {
		return nil, fmt.Errorf("yaml unmarshal to generic: %w", err)
	}

	jsonBytes, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}

	return jsonBytes, nil
}
