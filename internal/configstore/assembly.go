// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"gopkg.in/yaml.v3"
)

// HighestVersion returns the highest "MAJOR.MINOR.PATCH" string from a list,
// or "" if the list is empty. Used only in transitional adapters: migrating a
// legacy target-version list to the single active target, and the list-shaped
// admin config PUT shim.
func HighestVersion(versions []string) string {
	best := ""
	var bestParts [3]int
	for _, v := range versions {
		var parts [3]int
		for i, seg := range strings.SplitN(v, ".", 3) {
			if i >= 3 {
				break
			}
			n, _ := strconv.Atoi(seg)
			parts[i] = n
		}
		if best == "" || parts[0] > bestParts[0] ||
			(parts[0] == bestParts[0] && parts[1] > bestParts[1]) ||
			(parts[0] == bestParts[0] && parts[1] == bestParts[1] && parts[2] > bestParts[2]) {
			best = v
			bestParts = parts
		}
	}
	return best
}

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
	KeyOrganisations     = "organisations"
	KeyTargetChefVersion = "target_chef_version"
	KeyGitBaseURLs       = "git_base_urls"

	// KeyTargetChefVersionsLegacy is the pre-single-target key that stored a
	// list of target Chef versions. Retained only so config assembly can read
	// and migrate existing stores (the highest version becomes the single
	// active target). Never written by ConfigToSections.
	KeyTargetChefVersionsLegacy   = "target_chef_versions"
	KeyCollection                 = "collection"
	KeyConcurrency                = "concurrency"
	KeyAnalysisTools              = "analysis_tools"
	KeyTestKitchen                = "analysis_tools.test_kitchen"
	KeyReadiness                  = "readiness"
	KeyExports                    = "exports"
	KeyServerListen               = "server.listen"
	KeyServerTLS                  = "server.tls"
	KeyServerWebSocket            = "server.websocket"
	KeyServerGracefulShutdown     = "server.graceful_shutdown_seconds"
	KeyServerTrustedProxy         = "server.trusted_proxy"
	KeyFrontend                   = "frontend"
	KeyLogging                    = "logging"
	KeyAuth                       = "auth"
	KeyOwnership                  = "ownership"
	KeyStorage                    = "storage"
	KeySystemHealth               = "system_health"
	KeyPerformance                = "performance"
	KeyBackup                     = "backup"
	KeyIngest                     = "ingest"
	KeyCredentialEncryptionKeyEnv = "credential_encryption_key_env"

	// DB cert_source TLS material. These are standalone config-store entries,
	// NOT config sections: they are written and read directly by the admin TLS
	// save path and the static-mode listener wiring, and are deliberately
	// excluded from AllConfigKeys/ConfigToSections so config assembly never
	// tries to fold PEM material into a config struct. The certificate is
	// stored non-secret (public); the private key is stored secret (encrypted,
	// never returned by any API).7.
	KeyServerTLSCertificate       = "server.tls.certificate"
	KeyServerTLSPrivateKey        = "server.tls.private_key"
	KeyServerTLSPrivateKeyPending = "server.tls.private_key.pending"

	// ACME state material. Like the static cert keys above, these are
	// standalone config-store entries written and read directly by the ACME
	// engine (internal/acme), NOT config sections — they are excluded from
	// AllConfigKeys/ConfigToSections so config assembly never folds key/cert
	// material into a config struct. The account key and issued private key are
	// secret (encrypted, never returned by any API); the issued cert is public.
	//5.
	KeyServerTLSACMEAccountKey = "server.tls.acme.account_key"
	KeyServerTLSACMECert       = "server.tls.acme.cert"
	KeyServerTLSACMEKey        = "server.tls.acme.key"

	// Route 53 DNS-01 solver settings. Like the ACME state keys above, these are
	// standalone config-store entries read directly by the Route 53 solver
	// (internal/acme), NOT config sections, so they are excluded from
	// AllConfigKeys/ConfigToSections. The access key ID and secret access key are
	// secret (encrypted, never returned by any API); region and hosted zone ID
	// are public. They are the highest-priority source in the AWS credential and
	// region/zone resolution order (env vars and the IAM instance role follow).
	//4 / § 3.5.
	KeyServerTLSACMERoute53AccessKeyID     = "server.tls.acme.route53.access_key_id"
	KeyServerTLSACMERoute53SecretAccessKey = "server.tls.acme.route53.secret_access_key"
	KeyServerTLSACMERoute53Region          = "server.tls.acme.route53.region"
	KeyServerTLSACMERoute53HostedZoneID    = "server.tls.acme.route53.hosted_zone_id"

	// ACME operator status. A standalone, non-secret,
	// non-key entry written only by the renewer (last renewal time, last
	// renewal error, last hostname-registration error) and read by the admin
	// config GET to populate the Server & TLS status panel. Excluded from
	// AllConfigKeys/ConfigToSections like the other acme.* state keys.
	KeyServerTLSACMEStatus = "server.tls.acme.status"
)

// ServerListenSection is the JSON/YAML shape of the `server.listen` config
// store section. It holds the listen address and port, which are DB-managed
// and UI-editable (the bootstrap YAML keeps a copy only as the bind-failure
// fallback —).
type ServerListenSection struct {
	ListenAddress string `yaml:"listen_address" json:"listen_address"`
	Port          int    `yaml:"port" json:"port"`
}

// AnalysisToolsSection is the shape of the `analysis_tools` section: the
// analysis tool settings WITHOUT Test Kitchen, which keeps a section of its own.
//
// Two screens write these settings, and a screen replaces the whole section with
// what it was sent. Sharing one section therefore lets either screen silently
// drop everything the other owns, and report a successful save.
//
// Kept apart rather than merged on write, because merging makes "not sent" and
// "cleared" the same thing and a setting can then never be cleared.
type AnalysisToolsSection struct {
	EmbeddedBinDir            string   `yaml:"embedded_bin_dir" json:"embedded_bin_dir"`
	CookstyleEnabled          *bool    `yaml:"cookstyle_enabled" json:"cookstyle_enabled"`
	CookstyleTimeoutMinutes   int      `yaml:"cookstyle_timeout_minutes" json:"cookstyle_timeout_minutes"`
	CookstyleAddonCopPaths    []string `yaml:"cookstyle_addon_cop_paths" json:"cookstyle_addon_cop_paths"`
	TestKitchenTimeoutMinutes int      `yaml:"test_kitchen_timeout_minutes" json:"test_kitchen_timeout_minutes"`
}

// analysisToolsSectionOf is the analysis tool settings without Test Kitchen.
func analysisToolsSectionOf(a config.AnalysisToolsConfig) AnalysisToolsSection {
	return AnalysisToolsSection{
		EmbeddedBinDir:            a.EmbeddedBinDir,
		CookstyleEnabled:          a.CookstyleEnabled,
		CookstyleTimeoutMinutes:   a.CookstyleTimeoutMinutes,
		CookstyleAddonCopPaths:    a.CookstyleAddonCopPaths,
		TestKitchenTimeoutMinutes: a.TestKitchenTimeoutMinutes,
	}
}

// AllConfigKeys returns the complete list of known config section keys in
// the order they should be processed. This is useful for YAML auto-migration
// to ensure all sections are captured.
func AllConfigKeys() []string {
	return []string{
		KeyOrganisations,
		KeyTargetChefVersion,
		KeyGitBaseURLs,
		KeyCollection,
		KeyConcurrency,
		KeyAnalysisTools,
		// After the analysis tools, always: a store written before the split
		// still carries Test Kitchen nested inside that section, and this is
		// what puts the record of its own on top of the old copy.
		KeyTestKitchen,
		KeyReadiness,
		KeyExports,
		KeyServerListen,
		KeyServerTLS,
		KeyServerWebSocket,
		KeyServerGracefulShutdown,
		KeyServerTrustedProxy,
		KeyFrontend,
		KeyLogging,
		KeyAuth,
		KeyOwnership,
		KeyStorage,
		KeySystemHealth,
		KeyPerformance,
		KeyBackup,
		KeyIngest,
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
// Applied in the order AllConfigKeys gives, not the order a map happens to
// hand back. Two keys can write the same field — a store written before Test
// Kitchen was split out still carries it nested inside the analysis tools
// section — and which one lands has to be settled here rather than by whichever
// order the map iterated this time.
func assembleFields(cfg *config.Config, values map[string]json.RawMessage) error {
	applied := make(map[string]bool, len(values))
	for _, key := range assemblyOrder() {
		raw, ok := values[key]
		if !ok {
			continue
		}
		applied[key] = true
		if err := assembleOneField(cfg, key, raw); err != nil {
			return fmt.Errorf("key %q: %w", key, err)
		}
	}

	// Anything the order does not name, in a settled order of its own. Nothing
	// should reach here, and a key that does would otherwise be dropped
	// silently — which is how a setting stops being read without anybody
	// noticing.
	rest := make([]string, 0)
	for key := range values {
		if !applied[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	for _, key := range rest {
		if err := assembleOneField(cfg, key, values[key]); err != nil {
			return fmt.Errorf("key %q: %w", key, err)
		}
	}
	return nil
}

// assemblyOrder is the order stored sections are applied in.
//
// AllConfigKeys is what this service WRITES. Reading has to cope with what an
// older one wrote too, and two of those keys write a field another key also
// writes — so which lands is settled here rather than by whichever order a map
// iterated this time:
//
//   - the pre-single-target list of versions, which must come after the scalar
//     it falls back to, because it steps aside when that one is set;
//   - Test Kitchen, which a store written before the split still carries
//     nested inside the analysis tools section.
func assemblyOrder() []string {
	order := make([]string, 0, len(AllConfigKeys())+1)
	for _, key := range AllConfigKeys() {
		order = append(order, key)
		if key == KeyTargetChefVersion {
			order = append(order, KeyTargetChefVersionsLegacy)
		}
	}
	return order
}

// assembleOneField unmarshals a single config key's JSON value into the
// appropriate field on the Config struct. Returns nil for unknown keys.
func assembleOneField(cfg *config.Config, key string, raw json.RawMessage) error {
	switch key {
	case KeyOrganisations:
		return yamlUnmarshalInto(&cfg.Organisations, raw, key)
	case KeyTargetChefVersion:
		return yamlUnmarshalInto(&cfg.TargetChefVersion, raw, key)
	case KeyTargetChefVersionsLegacy:
		// Back-compat: an existing store may hold the pre-single-target list.
		// Migrate it to the single active target (the highest version, matching
		// the prior live behaviour) without clobbering a newer scalar value.
		if cfg.TargetChefVersion != "" {
			return nil
		}
		var legacy []string
		if err := yamlUnmarshalInto(&legacy, raw, key); err != nil {
			return err
		}
		cfg.TargetChefVersion = HighestVersion(legacy)
		return nil
	case KeyGitBaseURLs:
		return yamlUnmarshalInto(&cfg.GitBaseURLs, raw, key)
	case KeyCollection:
		return yamlUnmarshalInto(&cfg.Collection, raw, key)
	case KeyConcurrency:
		return yamlUnmarshalInto(&cfg.Concurrency, raw, key)
	case KeyAnalysisTools:
		// The whole struct on purpose, Test Kitchen included. A store written
		// before the split has it nested here, and reading it is what keeps an
		// upgrade from losing the settings before anything moves them.
		return yamlUnmarshalInto(&cfg.AnalysisTools, raw, key)
	case KeyTestKitchen:
		return yamlUnmarshalInto(&cfg.AnalysisTools.TestKitchen, raw, key)
	case KeyReadiness:
		return yamlUnmarshalInto(&cfg.Readiness, raw, key)
	case KeyExports:
		return yamlUnmarshalInto(&cfg.Exports, raw, key)
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
	case KeyServerTrustedProxy:
		return yamlUnmarshalInto(&cfg.Server.TrustedProxy, raw, key)
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
	case KeyIngest:
		return yamlUnmarshalInto(&cfg.Ingest, raw, key)
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
		KeyOrganisations:     cfg.Organisations,
		KeyTargetChefVersion: cfg.TargetChefVersion,
		KeyGitBaseURLs:       cfg.GitBaseURLs,
		KeyCollection:        cfg.Collection,
		KeyConcurrency:       cfg.Concurrency,
		KeyAnalysisTools:     analysisToolsSectionOf(cfg.AnalysisTools),
		KeyTestKitchen:       cfg.AnalysisTools.TestKitchen,
		KeyReadiness:         cfg.Readiness,
		KeyExports:           cfg.Exports,
		KeyServerListen: ServerListenSection{
			ListenAddress: cfg.Server.ListenAddress,
			Port:          cfg.Server.Port,
		},
		KeyServerTLS:              cfg.Server.TLS,
		KeyServerWebSocket:        cfg.Server.WebSocket,
		KeyServerGracefulShutdown: cfg.Server.GracefulShutdownSeconds,
		KeyServerTrustedProxy:     cfg.Server.TrustedProxy,
		KeyFrontend:               cfg.Frontend,
		KeyLogging:                cfg.Logging,
		KeyAuth:                   cfg.Auth,
		KeyOwnership:              cfg.Ownership,
		KeyStorage:                cfg.Storage,
		KeySystemHealth:           cfg.SystemHealth,
		KeyPerformance:            cfg.Performance,
		KeyBackup:                 cfg.Backup,
		KeyIngest:                 cfg.Ingest,
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
