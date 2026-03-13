// Package config provides configuration loading, defaults, and validation for
// chef-migration-metrics. Configuration is read from a YAML file whose path is
// supplied via the CHEF_MIGRATION_METRICS_CONFIG environment variable or passed
// directly to Load. Environment variable overrides are applied on top of the
// file values.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Top-level configuration
// ---------------------------------------------------------------------------

// Config is the root configuration structure for the application.
type Config struct {
	CredentialEncryptionKeyEnv string              `yaml:"credential_encryption_key_env"`
	Organisations              []Organisation      `yaml:"organisations"`
	TargetChefVersions         []string            `yaml:"target_chef_versions"`
	GitBaseURLs                []string            `yaml:"git_base_urls"`
	Storage                    StorageConfig       `yaml:"storage"`
	Collection                 CollectionConfig    `yaml:"collection"`
	Concurrency                ConcurrencyConfig   `yaml:"concurrency"`
	AnalysisTools              AnalysisToolsConfig `yaml:"analysis_tools"`
	Readiness                  ReadinessConfig     `yaml:"readiness"`
	Notifications              NotificationsConfig `yaml:"notifications"`
	SMTP                       SMTPConfig          `yaml:"smtp"`
	Exports                    ExportsConfig       `yaml:"exports"`
	Elasticsearch              ElasticsearchConfig `yaml:"elasticsearch"`
	Datastore                  DatastoreConfig     `yaml:"datastore"`
	Server                     ServerConfig        `yaml:"server"`
	Frontend                   FrontendConfig      `yaml:"frontend"`
	Logging                    LoggingConfig       `yaml:"logging"`
	Auth                       AuthConfig          `yaml:"auth"`
	Ownership                  OwnershipConfig     `yaml:"ownership"`

	// explicitExportsDir tracks whether the user explicitly set exports.output_directory.
	explicitExportsDir bool
	// explicitESDir tracks whether the user explicitly set elasticsearch.output_directory.
	explicitESDir bool
}

// ---------------------------------------------------------------------------
// Chef server organisations
// ---------------------------------------------------------------------------

// Organisation describes a single Chef Infra Server organisation to collect
// data from.
type Organisation struct {
	Name                string `yaml:"name"`
	ChefServerURL       string `yaml:"chef_server_url"`
	OrgName             string `yaml:"org_name"`
	ClientName          string `yaml:"client_name"`
	ClientKeyPath       string `yaml:"client_key_path"`
	ClientKeyCredential string `yaml:"client_key_credential"`
	SSLVerify           *bool  `yaml:"ssl_verify"`
}

// SSLVerifyEnabled returns whether SSL verification is enabled for this
// organisation. The default is true (verify) when not explicitly set.
func (o *Organisation) SSLVerifyEnabled() bool {
	if o.SSLVerify == nil {
		return true
	}
	return *o.SSLVerify
}

// ---------------------------------------------------------------------------
// Storage paths
// ---------------------------------------------------------------------------

// StorageConfig controls the filesystem paths used for persistent data such
// as downloaded Chef Server cookbooks and cloned git repositories. All paths
// default to subdirectories under DataDir.
//
// For RPM/DEB installs DataDir defaults to /var/lib/chef-migration-metrics
// which is created by the package with correct ownership. For development
// (when the default is not writable) it falls back to $TMPDIR/chef-migration-metrics.
type StorageConfig struct {
	// DataDir is the base directory for all persistent application data.
	// CookbookCacheDir and GitCookbookDir default to subdirectories of
	// this path when not explicitly set.
	DataDir string `yaml:"data_dir"`

	// CookbookCacheDir is the directory where Chef Server cookbook files
	// are extracted after download. Structure:
	//   <cookbook_cache_dir>/<org_id>/<name>/<version>/
	CookbookCacheDir string `yaml:"cookbook_cache_dir"`

	// GitCookbookDir is the directory where git cookbook repositories are
	// cloned and pulled. Structure:
	//   <git_cookbook_dir>/<cookbook_name>/
	GitCookbookDir string `yaml:"git_cookbook_dir"`
}

// ---------------------------------------------------------------------------
// Collection schedule & thresholds
// ---------------------------------------------------------------------------

// CollectionConfig controls the background collection schedule and staleness
// thresholds.
type CollectionConfig struct {
	Schedule                   string `yaml:"schedule"`
	StaleNodeThresholdDays     int    `yaml:"stale_node_threshold_days"`
	StaleCookbookThresholdDays int    `yaml:"stale_cookbook_threshold_days"`
}

// ---------------------------------------------------------------------------
// Concurrency / worker pool sizes
// ---------------------------------------------------------------------------

// ConcurrencyConfig controls worker pool sizes for parallelised tasks.
type ConcurrencyConfig struct {
	OrganisationCollection int `yaml:"organisation_collection"`
	NodePageFetching       int `yaml:"node_page_fetching"`
	GitPull                int `yaml:"git_pull"`
	CookstyleScan          int `yaml:"cookstyle_scan"`
	TestKitchenRun         int `yaml:"test_kitchen_run"`
	ReadinessEvaluation    int `yaml:"readiness_evaluation"`
}

// ---------------------------------------------------------------------------
// Analysis tools (embedded CookStyle / Test Kitchen)
// ---------------------------------------------------------------------------

// AnalysisToolsConfig controls the embedded analysis tool locations and
// timeouts.
type AnalysisToolsConfig struct {
	EmbeddedBinDir            string            `yaml:"embedded_bin_dir"`
	CookstyleEnabled          *bool             `yaml:"cookstyle_enabled"`
	CookstyleTimeoutMinutes   int               `yaml:"cookstyle_timeout_minutes"`
	TestKitchenTimeoutMinutes int               `yaml:"test_kitchen_timeout_minutes"`
	TestKitchen               TestKitchenConfig `yaml:"test_kitchen"`
}

// IsCookstyleEnabled returns true if CookStyle scanning is enabled in the
// configuration. Defaults to true when the field is omitted.
func (a *AnalysisToolsConfig) IsCookstyleEnabled() bool {
	if a.CookstyleEnabled == nil {
		return true
	}
	return *a.CookstyleEnabled
}

// TestKitchenConfig controls driver and platform overrides for Test Kitchen
// runs. These allow the operator to force a specific virtualisation /
// containerisation provider and to test cookbooks against the actual
// platforms that consume them rather than whatever the cookbook author chose.
type TestKitchenConfig struct {
	// Enabled controls whether Test Kitchen testing is active. When set to
	// false, Test Kitchen is disabled even if the kitchen and docker binaries
	// are available. When omitted or set to true (the default), Test Kitchen
	// is enabled automatically if both kitchen and docker are detected at
	// startup.
	//
	// Use this to turn off Test Kitchen without removing docker or kitchen
	// from the system:
	//
	//   analysis_tools:
	//     test_kitchen:
	//       enabled: false
	Enabled *bool `yaml:"enabled"`

	// DriverOverride forces every Test Kitchen run to use the named driver
	// (e.g. "dokken", "vagrant", "ec2", "azurerm") regardless of what the
	// cookbook's .kitchen.yml specifies. When empty the cookbook's own
	// driver setting is left intact.
	DriverOverride string `yaml:"driver_override"`

	// DriverConfig is an arbitrary map of key-value pairs merged into the
	// driver section of the generated .kitchen.local.yml. Useful for
	// setting driver-specific options like privileged mode, network, or
	// instance type. Keys must be valid YAML scalars. Example:
	//   driver_config:
	//     privileged: "true"
	//     instance_type: t3.medium
	DriverConfig map[string]string `yaml:"driver_config"`

	// PlatformOverrides replaces the platforms list in the generated
	// .kitchen.local.yml so cookbooks are tested against the actual OS
	// images consumed in production rather than the cookbook's defaults.
	// Each entry maps directly to a Test Kitchen platform definition.
	// When this list is non-empty, the cookbook's own platform list is
	// completely replaced.
	//
	// Example:
	//   platform_overrides:
	//     - name: ubuntu-22.04
	//       driver:
	//         image: dokken/ubuntu-22.04
	//     - name: centos-8
	//       driver:
	//         image: dokken/centos-8
	PlatformOverrides []TestKitchenPlatform `yaml:"platform_overrides"`

	// ExtraYAML is a raw YAML block merged verbatim into every generated
	// .kitchen.local.yml after the driver, provisioner, and platform
	// overrides. This is the escape hatch for anything not covered by the
	// structured fields above — transport settings, verifier overrides,
	// lifecycle hooks, etc.
	//
	// Example:
	//   extra_yaml: |
	//     transport:
	//       name: ssh
	//     verifier:
	//       name: inspec
	ExtraYAML string `yaml:"extra_yaml"`
}

// TestKitchenPlatform describes a single platform entry for the
// .kitchen.local.yml override. It mirrors Test Kitchen's own platform
// schema so the operator can specify exactly which OS images to test.
type TestKitchenPlatform struct {
	// Name is the Test Kitchen platform name (e.g. "ubuntu-22.04").
	Name string `yaml:"name"`

	// Driver contains driver-specific platform settings (e.g. image name,
	// box URL). The map is written as-is under the platform's driver key.
	Driver map[string]string `yaml:"driver"`

	// Attributes contains platform-level default attributes. Optional.
	Attributes map[string]interface{} `yaml:"attributes"`
}

// IsEnabled returns true if Test Kitchen testing is enabled in the
// configuration. Defaults to true when the field is omitted.
func (tk *TestKitchenConfig) IsEnabled() bool {
	if tk.Enabled == nil {
		return true
	}
	return *tk.Enabled
}

// ---------------------------------------------------------------------------
// Upgrade readiness
// ---------------------------------------------------------------------------

// ReadinessConfig controls the upgrade readiness evaluation parameters.
type ReadinessConfig struct {
	MinFreeDiskMB int `yaml:"min_free_disk_mb"`
}

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------

// NotificationsConfig controls webhook and email notification delivery.
type NotificationsConfig struct {
	Enabled             bool                  `yaml:"enabled"`
	Channels            []NotificationChannel `yaml:"channels"`
	ReadinessMilestones []int                 `yaml:"readiness_milestones"`
	StaleNodeAlertCount int                   `yaml:"stale_node_alert_count"`
}

// NotificationChannel is one configured delivery channel (webhook or email).
type NotificationChannel struct {
	Name       string                    `yaml:"name"`
	Type       string                    `yaml:"type"`
	URL        string                    `yaml:"url"`
	URLEnv     string                    `yaml:"url_env"`
	Recipients []string                  `yaml:"recipients"`
	Events     []string                  `yaml:"events"`
	Filters    NotificationChannelFilter `yaml:"filters"`
}

// NotificationChannelFilter limits which events are delivered through a
// channel.
type NotificationChannelFilter struct {
	Organisations []string `yaml:"organisations"`
	Cookbooks     []string `yaml:"cookbooks"`
}

// ---------------------------------------------------------------------------
// SMTP (email notifications)
// ---------------------------------------------------------------------------

// SMTPConfig holds settings for email notification delivery.
type SMTPConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	UsernameEnv string `yaml:"username_env"`
	PasswordEnv string `yaml:"password_env"`
	FromAddress string `yaml:"from_address"`
	TLS         bool   `yaml:"tls"`
}

// ---------------------------------------------------------------------------
// Data exports
// ---------------------------------------------------------------------------

// ExportsConfig controls the behaviour of data export operations.
type ExportsConfig struct {
	MaxRows         int    `yaml:"max_rows"`
	AsyncThreshold  int    `yaml:"async_threshold"`
	OutputDirectory string `yaml:"output_directory"`
	RetentionHours  int    `yaml:"retention_hours"`
}

// ---------------------------------------------------------------------------
// Elasticsearch NDJSON export
// ---------------------------------------------------------------------------

// ElasticsearchConfig controls the NDJSON export pipeline for Elasticsearch.
type ElasticsearchConfig struct {
	Enabled         bool   `yaml:"enabled"`
	OutputDirectory string `yaml:"output_directory"`
	RetentionHours  int    `yaml:"retention_hours"`
}

// ---------------------------------------------------------------------------
// Datastore
// ---------------------------------------------------------------------------

// DatastoreConfig holds the database connection settings.
type DatastoreConfig struct {
	URL string `yaml:"url"`
}

// ---------------------------------------------------------------------------
// Web server & TLS
// ---------------------------------------------------------------------------

// ServerConfig controls the HTTP/HTTPS listener.
type ServerConfig struct {
	ListenAddress           string          `yaml:"listen_address"`
	Port                    int             `yaml:"port"`
	TLS                     TLSConfig       `yaml:"tls"`
	WebSocket               WebSocketConfig `yaml:"websocket"`
	GracefulShutdownSeconds int             `yaml:"graceful_shutdown_seconds"`
}

// WebSocketConfig controls the real-time event WebSocket endpoint.
type WebSocketConfig struct {
	Enabled             *bool `yaml:"enabled"` // nil means "use default (true)"
	MaxConnections      int   `yaml:"max_connections"`
	SendBufferSize      int   `yaml:"send_buffer_size"`
	WriteTimeoutSeconds int   `yaml:"write_timeout_seconds"`
	PingIntervalSeconds int   `yaml:"ping_interval_seconds"`
	PongTimeoutSeconds  int   `yaml:"pong_timeout_seconds"`
}

// IsEnabled returns whether the WebSocket endpoint is enabled. If the
// Enabled field was not set in configuration, the default is true.
func (ws *WebSocketConfig) IsEnabled() bool {
	if ws.Enabled == nil {
		return true
	}
	return *ws.Enabled
}

// TLSConfig holds all TLS-related settings including static certificate and
// ACME modes.
type TLSConfig struct {
	Mode             string     `yaml:"mode"`
	Enabled          *bool      `yaml:"enabled,omitempty"` // deprecated, backward compat
	CertPath         string     `yaml:"cert_path"`
	KeyPath          string     `yaml:"key_path"`
	CAPath           string     `yaml:"ca_path"`
	MinVersion       string     `yaml:"min_version"`
	HTTPRedirectPort int        `yaml:"http_redirect_port"`
	ACME             ACMEConfig `yaml:"acme"`
}

// ACMEConfig holds Automatic Certificate Management Environment settings.
type ACMEConfig struct {
	Domains           []string          `yaml:"domains"`
	Email             string            `yaml:"email"`
	CAURL             string            `yaml:"ca_url"`
	Challenge         string            `yaml:"challenge"`
	DNSProvider       string            `yaml:"dns_provider"`
	DNSProviderConfig map[string]string `yaml:"dns_provider_config"`
	StoragePath       string            `yaml:"storage_path"`
	RenewBeforeDays   int               `yaml:"renew_before_days"`
	AgreeToTOS        bool              `yaml:"agree_to_tos"`
	TrustedRoots      string            `yaml:"trusted_roots"`
}

// ---------------------------------------------------------------------------
// Frontend
// ---------------------------------------------------------------------------

// FrontendConfig controls the embedded SPA serving behaviour.
type FrontendConfig struct {
	BasePath string `yaml:"base_path"`
}

// ---------------------------------------------------------------------------
// Logging
// ---------------------------------------------------------------------------

// LoggingConfig controls structured logging and log retention.
type LoggingConfig struct {
	Level         string `yaml:"level"`
	RetentionDays int    `yaml:"retention_days"`
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

// AuthConfig holds authentication provider configuration.
type AuthConfig struct {
	Providers         []AuthProvider `yaml:"providers"`
	SessionExpiry     string         `yaml:"session_expiry"`
	MinPasswordLength int            `yaml:"min_password_length"`
	LockoutAttempts   int            `yaml:"lockout_attempts"`
}

// AuthProvider is a single authentication provider (local, LDAP, or SAML).
type AuthProvider struct {
	Type                   string `yaml:"type"`
	Host                   string `yaml:"host,omitempty"`
	Port                   int    `yaml:"port,omitempty"`
	BaseDN                 string `yaml:"base_dn,omitempty"`
	BindDN                 string `yaml:"bind_dn,omitempty"`
	BindPasswordEnv        string `yaml:"bind_password_env,omitempty"`
	BindPasswordCredential string `yaml:"bind_password_credential,omitempty"`
	IDPMetadataURL         string `yaml:"idp_metadata_url,omitempty"`
	SPEntityID             string `yaml:"sp_entity_id,omitempty"`
}

// ---------------------------------------------------------------------------
// Ownership
// ---------------------------------------------------------------------------

// OwnershipConfig controls the ownership tracking feature.
type OwnershipConfig struct {
	Enabled   bool                `yaml:"enabled"`
	AuditLog  OwnershipAuditLog   `yaml:"audit_log"`
	AutoRules []OwnershipAutoRule `yaml:"auto_rules"`
}

// OwnershipAuditLog controls retention of the ownership audit log.
type OwnershipAuditLog struct {
	RetentionDays int `yaml:"retention_days"`
}

// OwnershipAutoRule defines a single auto-derivation rule for ownership.
type OwnershipAutoRule struct {
	Name           string `yaml:"name"`
	Owner          string `yaml:"owner"`
	Type           string `yaml:"type"`
	AttributePath  string `yaml:"attribute_path"`
	MatchValue     string `yaml:"match_value"`
	Pattern        string `yaml:"pattern"`
	PolicyName     string `yaml:"policy_name"`
	Organisation   string `yaml:"organisation"`
	ObjectType     string `yaml:"object_type"`     // cmdb_attribute: one of node, cookbook, profile, role
	OwnerAttribute string `yaml:"owner_attribute"` // cmdb_attribute: field within itil.cmdb.<object_type> holding the owner name (default: "owner")
}

// ValidCMDBObjectTypes lists the allowed values for OwnershipAutoRule.ObjectType
// when Type is "cmdb_attribute". Each corresponds to a Chef normal attribute
// subtree at itil.cmdb.<object_type>.
var ValidCMDBObjectTypes = map[string]bool{
	"node":     true,
	"cookbook": true,
	"profile":  true,
	"role":     true,
}

// CMDBSearchKeys returns the additional partial-search attribute keys and
// paths needed to collect CMDB ownership data from Chef nodes. Each
// configured cmdb_attribute rule contributes a key of the form
// "itil.cmdb.<object_type>" mapped to the Chef attribute path
// ["itil", "cmdb", "<object_type>"]. Duplicate object types are
// deduplicated so each path is requested at most once.
//
// The returned map is intended to be merged into the standard
// NodeSearchAttributes query before executing partial search.
func (c *OwnershipConfig) CMDBSearchKeys() map[string][]string {
	if !c.Enabled {
		return nil
	}
	keys := make(map[string][]string)
	for _, rule := range c.AutoRules {
		if rule.Type != "cmdb_attribute" || rule.ObjectType == "" {
			continue
		}
		key := "itil.cmdb." + rule.ObjectType
		if _, exists := keys[key]; !exists {
			keys[key] = []string{"itil", "cmdb", rule.ObjectType}
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

// setDefaults populates zero-value fields with the specification defaults.
func (c *Config) setDefaults() {
	if c.CredentialEncryptionKeyEnv == "" {
		c.CredentialEncryptionKeyEnv = "CMM_CREDENTIAL_ENCRYPTION_KEY"
	}

	// Storage — resolve DataDir first, then derive subdirectories from it.
	if c.Storage.DataDir == "" {
		// Prefer the standard package install location; fall back to a
		// temp-based path for development where /var/lib may not be writable.
		candidate := "/var/lib/chef-migration-metrics"
		if info, err := os.Stat(candidate); err != nil || !info.IsDir() {
			candidate = filepath.Join(os.TempDir(), "chef-migration-metrics")
		}
		c.Storage.DataDir = candidate
	}
	if c.Storage.CookbookCacheDir == "" {
		c.Storage.CookbookCacheDir = filepath.Join(c.Storage.DataDir, "cookbook-cache")
	}
	if c.Storage.GitCookbookDir == "" {
		c.Storage.GitCookbookDir = filepath.Join(c.Storage.DataDir, "git-cookbooks")
	}

	// Collection
	if c.Collection.Schedule == "" {
		c.Collection.Schedule = "0 * * * *"
	}
	if c.Collection.StaleNodeThresholdDays == 0 {
		c.Collection.StaleNodeThresholdDays = 7
	}
	if c.Collection.StaleCookbookThresholdDays == 0 {
		c.Collection.StaleCookbookThresholdDays = 365
	}

	// Concurrency
	if c.Concurrency.OrganisationCollection == 0 {
		c.Concurrency.OrganisationCollection = 5
	}
	if c.Concurrency.NodePageFetching == 0 {
		c.Concurrency.NodePageFetching = 10
	}
	if c.Concurrency.GitPull == 0 {
		c.Concurrency.GitPull = 10
	}
	if c.Concurrency.CookstyleScan == 0 {
		c.Concurrency.CookstyleScan = 8
	}
	if c.Concurrency.TestKitchenRun == 0 {
		c.Concurrency.TestKitchenRun = 4
	}
	if c.Concurrency.ReadinessEvaluation == 0 {
		c.Concurrency.ReadinessEvaluation = 20
	}

	// Analysis tools
	if c.AnalysisTools.EmbeddedBinDir == "" {
		c.AnalysisTools.EmbeddedBinDir = "/opt/chef-migration-metrics/embedded/bin"
	}
	if c.AnalysisTools.CookstyleTimeoutMinutes == 0 {
		c.AnalysisTools.CookstyleTimeoutMinutes = 10
	}
	if c.AnalysisTools.TestKitchenTimeoutMinutes == 0 {
		c.AnalysisTools.TestKitchenTimeoutMinutes = 30
	}
	if c.AnalysisTools.CookstyleEnabled == nil {
		t := true
		c.AnalysisTools.CookstyleEnabled = &t
	}
	if c.AnalysisTools.TestKitchen.Enabled == nil {
		t := true
		c.AnalysisTools.TestKitchen.Enabled = &t
	}

	// Readiness
	if c.Readiness.MinFreeDiskMB == 0 {
		c.Readiness.MinFreeDiskMB = 2048
	}

	// Notifications
	if c.Notifications.ReadinessMilestones == nil {
		c.Notifications.ReadinessMilestones = []int{50, 75, 90, 100}
	}
	if c.Notifications.StaleNodeAlertCount == 0 {
		c.Notifications.StaleNodeAlertCount = 50
	}

	// SMTP
	if c.SMTP.Port == 0 {
		c.SMTP.Port = 587
	}
	if !c.SMTP.TLS {
		c.SMTP.TLS = true
	}

	// Exports
	if c.Exports.MaxRows == 0 {
		c.Exports.MaxRows = 100000
	}
	if c.Exports.AsyncThreshold == 0 {
		c.Exports.AsyncThreshold = 10000
	}
	if c.Exports.OutputDirectory == "" {
		c.Exports.OutputDirectory = "/var/lib/chef-migration-metrics/exports"
	}
	if c.Exports.RetentionHours == 0 {
		c.Exports.RetentionHours = 24
	}

	// Elasticsearch
	if c.Elasticsearch.OutputDirectory == "" {
		c.Elasticsearch.OutputDirectory = "/var/lib/chef-migration-metrics/elasticsearch"
	}
	if c.Elasticsearch.RetentionHours == 0 {
		c.Elasticsearch.RetentionHours = 48
	}

	// Datastore
	if c.Datastore.URL == "" {
		c.Datastore.URL = "postgres://localhost:5432/chef_migration_metrics"
	}

	// Server
	if c.Server.ListenAddress == "" {
		c.Server.ListenAddress = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.GracefulShutdownSeconds == 0 {
		c.Server.GracefulShutdownSeconds = 30
	}

	// WebSocket defaults — Enabled uses *bool so nil (not set) defaults to
	// true via IsEnabled(), while explicit `enabled: false` is preserved.
	if c.Server.WebSocket.MaxConnections == 0 {
		c.Server.WebSocket.MaxConnections = 100
	}
	if c.Server.WebSocket.SendBufferSize == 0 {
		c.Server.WebSocket.SendBufferSize = 64
	}
	if c.Server.WebSocket.WriteTimeoutSeconds == 0 {
		c.Server.WebSocket.WriteTimeoutSeconds = 10
	}
	if c.Server.WebSocket.PingIntervalSeconds == 0 {
		c.Server.WebSocket.PingIntervalSeconds = 30
	}
	if c.Server.WebSocket.PongTimeoutSeconds == 0 {
		c.Server.WebSocket.PongTimeoutSeconds = 60
	}

	// TLS defaults
	c.resolveTLSMode()
	if c.Server.TLS.MinVersion == "" {
		c.Server.TLS.MinVersion = "1.2"
	}
	if c.Server.TLS.ACME.CAURL == "" {
		c.Server.TLS.ACME.CAURL = "https://acme-v02.api.letsencrypt.org/directory"
	}
	if c.Server.TLS.ACME.Challenge == "" {
		c.Server.TLS.ACME.Challenge = "http-01"
	}
	if c.Server.TLS.ACME.StoragePath == "" {
		c.Server.TLS.ACME.StoragePath = "/var/lib/chef-migration-metrics/acme"
	}
	if c.Server.TLS.ACME.RenewBeforeDays == 0 {
		c.Server.TLS.ACME.RenewBeforeDays = 30
	}

	// Frontend
	if c.Frontend.BasePath == "" {
		c.Frontend.BasePath = "/"
	}

	// Auth
	if c.Auth.SessionExpiry == "" {
		c.Auth.SessionExpiry = "8h"
	}
	if c.Auth.MinPasswordLength == 0 {
		c.Auth.MinPasswordLength = 8
	}
	if c.Auth.LockoutAttempts == 0 {
		c.Auth.LockoutAttempts = 5
	}

	// Logging
	if c.Logging.Level == "" {
		c.Logging.Level = "INFO"
	}
	if c.Logging.RetentionDays == 0 {
		c.Logging.RetentionDays = 90
	}

	// Ownership
	if c.Ownership.AuditLog.RetentionDays == 0 {
		c.Ownership.AuditLog.RetentionDays = 365
	}
	for i := range c.Ownership.AutoRules {
		if c.Ownership.AutoRules[i].Type == "cmdb_attribute" && c.Ownership.AutoRules[i].OwnerAttribute == "" {
			c.Ownership.AutoRules[i].OwnerAttribute = "owner"
		}
	}
}

// resolveTLSMode handles the deprecated tls.enabled boolean and normalises
// the mode field.
func (c *Config) resolveTLSMode() {
	if c.Server.TLS.Mode != "" {
		// Explicit mode always wins.
		return
	}
	if c.Server.TLS.Enabled != nil && *c.Server.TLS.Enabled {
		c.Server.TLS.Mode = "static"
	} else {
		c.Server.TLS.Mode = "off"
	}
}

// ---------------------------------------------------------------------------
// Environment variable overrides
// ---------------------------------------------------------------------------

// applyEnvOverrides applies well-known environment variable overrides on top
// of the loaded configuration.
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		c.Datastore.URL = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Server.Port = p
		}
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_MODE"); v != "" {
		c.Server.TLS.Mode = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_CERT_PATH"); v != "" {
		c.Server.TLS.CertPath = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_KEY_PATH"); v != "" {
		c.Server.TLS.KeyPath = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_CA_PATH"); v != "" {
		c.Server.TLS.CAPath = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_MIN_VERSION"); v != "" {
		c.Server.TLS.MinVersion = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_HTTP_REDIRECT_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Server.TLS.HTTPRedirectPort = p
		}
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_EMAIL"); v != "" {
		c.Server.TLS.ACME.Email = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CA_URL"); v != "" {
		c.Server.TLS.ACME.CAURL = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CHALLENGE"); v != "" {
		c.Server.TLS.ACME.Challenge = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_DNS_PROVIDER"); v != "" {
		c.Server.TLS.ACME.DNSProvider = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_STORAGE_PATH"); v != "" {
		c.Server.TLS.ACME.StoragePath = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_AGREE_TO_TOS"); v != "" {
		c.Server.TLS.ACME.AgreeToTOS = strings.EqualFold(v, "true")
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_ANALYSIS_TOOLS_EMBEDDED_BIN_DIR"); v != "" {
		c.AnalysisTools.EmbeddedBinDir = v
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_ELASTICSEARCH_ENABLED"); v != "" {
		c.Elasticsearch.Enabled = strings.EqualFold(v, "true")
	}
	if v := os.Getenv("CHEF_MIGRATION_METRICS_ELASTICSEARCH_OUTPUT_DIRECTORY"); v != "" {
		c.Elasticsearch.OutputDirectory = v
	}
	if v := os.Getenv("CMM_OWNERSHIP_ENABLED"); v != "" {
		c.Ownership.Enabled = strings.EqualFold(v, "true")
	}
	if v := os.Getenv("CMM_OWNERSHIP_AUDIT_LOG_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			c.Ownership.AuditLog.RetentionDays = n
		}
	}
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// ValidationError collects one or more configuration validation failures.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("configuration validation failed:\n  - %s", strings.Join(e.Errors, "\n  - "))
}

// add appends an error message to the list.
func (e *ValidationError) add(msg string) {
	e.Errors = append(e.Errors, msg)
}

// addf appends a formatted error message.
func (e *ValidationError) addf(format string, args ...any) {
	e.Errors = append(e.Errors, fmt.Sprintf(format, args...))
}

// hasErrors returns true if any errors have been recorded.
func (e *ValidationError) hasErrors() bool {
	return len(e.Errors) > 0
}

// Warnings collects non-fatal issues detected during validation.
type Warnings struct {
	Messages []string
}

func (w *Warnings) add(msg string) {
	w.Messages = append(w.Messages, msg)
}

func (w *Warnings) addf(format string, args ...any) {
	w.Messages = append(w.Messages, fmt.Sprintf(format, args...))
}

// Validate checks the configuration against the specification rules. It
// returns a non-nil *ValidationError if fatal problems are found and a
// Warnings struct for non-fatal issues.
func (c *Config) Validate() (*Warnings, error) {
	ve := &ValidationError{}
	w := &Warnings{}

	c.validateOrganisations(ve)
	c.validateTargetVersions(ve)
	c.validateCollection(ve)
	c.validateConcurrency(ve)
	c.validateAnalysisTools(ve, w)
	c.validateNotifications(ve, w)
	c.validateExports(ve, w)
	c.validateElasticsearch(ve, w)
	c.validateServer(ve, w)
	c.validateLogging(ve)
	c.validateAuth(ve)
	c.validateOwnership(ve)

	if ve.hasErrors() {
		return w, ve
	}
	return w, nil
}

// --- per-section validators ---

func (c *Config) validateOrganisations(ve *ValidationError) {
	if len(c.Organisations) == 0 {
		ve.add("at least one organisation must be configured")
		return
	}
	seen := make(map[string]bool)
	for i, org := range c.Organisations {
		prefix := fmt.Sprintf("organisations[%d]", i)
		if org.Name == "" {
			ve.addf("%s: name is required", prefix)
		} else if seen[org.Name] {
			ve.addf("%s: duplicate organisation name %q", prefix, org.Name)
		} else {
			seen[org.Name] = true
		}
		if org.ChefServerURL == "" {
			ve.addf("%s: chef_server_url is required", prefix)
		}
		if org.OrgName == "" {
			ve.addf("%s: org_name is required", prefix)
		}
		if org.ClientName == "" {
			ve.addf("%s: client_name is required", prefix)
		}
		if org.ClientKeyPath == "" && org.ClientKeyCredential == "" {
			ve.addf("%s: one of client_key_path or client_key_credential is required", prefix)
		}
		if org.ClientKeyPath != "" {
			if _, err := os.Stat(org.ClientKeyPath); err != nil {
				ve.addf("%s: client_key_path %q: %v", prefix, org.ClientKeyPath, err)
			}
		}
	}
}

// semverRe is a simple check for major.minor.patch format.
var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func (c *Config) validateTargetVersions(ve *ValidationError) {
	for i, v := range c.TargetChefVersions {
		if !semverRe.MatchString(v) {
			ve.addf("target_chef_versions[%d]: %q is not a valid semver string (expected MAJOR.MINOR.PATCH)", i, v)
		}
	}
}

// cronFieldRe is a deliberately permissive check — a cron expression has 5
// space-separated fields. Full parsing is left to the scheduler library.
var cronFieldRe = regexp.MustCompile(`^(\S+\s+){4}\S+$`)

func (c *Config) validateCollection(ve *ValidationError) {
	if !cronFieldRe.MatchString(c.Collection.Schedule) {
		ve.addf("collection.schedule: %q is not a valid cron expression", c.Collection.Schedule)
	}
	if c.Collection.StaleNodeThresholdDays < 1 {
		ve.add("collection.stale_node_threshold_days must be >= 1")
	}
	if c.Collection.StaleCookbookThresholdDays < 1 {
		ve.add("collection.stale_cookbook_threshold_days must be >= 1")
	}
}

func (c *Config) validateConcurrency(ve *ValidationError) {
	check := func(name string, val int) {
		if val < 1 {
			ve.addf("concurrency.%s must be >= 1", name)
		}
	}
	check("organisation_collection", c.Concurrency.OrganisationCollection)
	check("node_page_fetching", c.Concurrency.NodePageFetching)
	check("git_pull", c.Concurrency.GitPull)
	check("cookstyle_scan", c.Concurrency.CookstyleScan)
	check("test_kitchen_run", c.Concurrency.TestKitchenRun)
	check("readiness_evaluation", c.Concurrency.ReadinessEvaluation)
}

func (c *Config) validateAnalysisTools(ve *ValidationError, w *Warnings) {
	if c.AnalysisTools.CookstyleTimeoutMinutes < 1 {
		ve.add("analysis_tools.cookstyle_timeout_minutes must be >= 1")
	}
	if c.AnalysisTools.TestKitchenTimeoutMinutes < 1 {
		ve.add("analysis_tools.test_kitchen_timeout_minutes must be >= 1")
	}
	if c.AnalysisTools.EmbeddedBinDir != "" {
		if info, err := os.Stat(c.AnalysisTools.EmbeddedBinDir); err != nil || !info.IsDir() {
			w.addf("analysis_tools.embedded_bin_dir %q does not exist or is not a directory; falling back to PATH lookup", c.AnalysisTools.EmbeddedBinDir)
		}
	}

	// Validate Test Kitchen overrides.
	tk := c.AnalysisTools.TestKitchen
	if tk.DriverOverride != "" {
		validDrivers := map[string]bool{
			"dokken": true, "vagrant": true, "ec2": true,
			"azurerm": true, "gce": true, "hyperv": true,
			"docker": true, "digitalocean": true, "openstack": true,
		}
		if !validDrivers[tk.DriverOverride] {
			w.addf("analysis_tools.test_kitchen.driver_override %q is not a recognised Test Kitchen driver; proceeding anyway", tk.DriverOverride)
		}
	}
	for i, p := range tk.PlatformOverrides {
		if p.Name == "" {
			ve.addf("analysis_tools.test_kitchen.platform_overrides[%d].name is required", i)
		}
	}
}

func (c *Config) validateNotifications(ve *ValidationError, w *Warnings) {
	if !c.Notifications.Enabled {
		return
	}

	validEvents := map[string]bool{
		"cookbook_status_change":        true,
		"readiness_milestone":           true,
		"new_incompatible_cookbook":     true,
		"collection_failure":            true,
		"stale_node_threshold_exceeded": true,
		"certificate_expiry_warning":    true,
	}

	seenNames := make(map[string]bool)
	hasEmailChannel := false

	for i, ch := range c.Notifications.Channels {
		prefix := fmt.Sprintf("notifications.channels[%d]", i)
		if ch.Name == "" {
			ve.addf("%s: name is required", prefix)
		} else if seenNames[ch.Name] {
			ve.addf("%s: duplicate channel name %q", prefix, ch.Name)
		} else {
			seenNames[ch.Name] = true
		}

		switch ch.Type {
		case "webhook":
			if ch.URL == "" && ch.URLEnv == "" {
				ve.addf("%s: webhook channel requires url or url_env", prefix)
			}
			if ch.URLEnv != "" {
				if os.Getenv(ch.URLEnv) == "" {
					ve.addf("%s: url_env %q references an unset environment variable", prefix, ch.URLEnv)
				}
			}
		case "email":
			hasEmailChannel = true
			if len(ch.Recipients) == 0 {
				ve.addf("%s: email channel requires at least one recipient", prefix)
			}
		default:
			ve.addf("%s: type must be 'webhook' or 'email', got %q", prefix, ch.Type)
		}

		for j, ev := range ch.Events {
			if !validEvents[ev] {
				ve.addf("%s.events[%d]: unknown event type %q", prefix, j, ev)
			}
		}
	}

	for i, m := range c.Notifications.ReadinessMilestones {
		if m < 0 || m > 100 {
			ve.addf("notifications.readiness_milestones[%d]: %d must be between 0 and 100", i, m)
		}
	}

	if hasEmailChannel {
		if c.SMTP.Host == "" {
			ve.add("smtp.host is required when email notification channels are configured")
		}
		if c.SMTP.FromAddress == "" {
			ve.add("smtp.from_address is required when email notification channels are configured")
		}
	}
}

func (c *Config) validateExports(ve *ValidationError, w *Warnings) {
	if c.Exports.RetentionHours < 1 {
		ve.add("exports.retention_hours must be >= 1")
	}
	if c.Exports.MaxRows < 1 {
		ve.add("exports.max_rows must be >= 1")
	}
	if c.Exports.AsyncThreshold < 1 {
		ve.add("exports.async_threshold must be >= 1")
	}
	// Only validate the output directory if the user explicitly configured it.
	// The default path (/var/lib/...) may not exist in dev/test environments;
	// the application will create it at runtime if needed.
	if c.explicitExportsDir {
		if err := checkDirWritable(c.Exports.OutputDirectory); err != nil {
			ve.addf("exports.output_directory %q: %v", c.Exports.OutputDirectory, err)
		}
	}
}

func (c *Config) validateElasticsearch(ve *ValidationError, w *Warnings) {
	if !c.Elasticsearch.Enabled {
		return
	}
	if c.Elasticsearch.RetentionHours < 1 {
		ve.add("elasticsearch.retention_hours must be >= 1")
	}
	// Only validate the output directory if the user explicitly configured it
	// or if elasticsearch is enabled (user opted in, so the dir matters).
	if c.explicitESDir {
		if err := checkDirWritable(c.Elasticsearch.OutputDirectory); err != nil {
			ve.addf("elasticsearch.output_directory %q: %v", c.Elasticsearch.OutputDirectory, err)
		}
	}
}

func (c *Config) validateServer(ve *ValidationError, w *Warnings) {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		ve.addf("server.port: %d is not a valid port number (1-65535)", c.Server.Port)
	}

	// WebSocket validation
	if c.Server.WebSocket.MaxConnections < 1 {
		ve.addf("server.websocket.max_connections: must be at least 1, got %d", c.Server.WebSocket.MaxConnections)
	}
	if c.Server.WebSocket.SendBufferSize < 1 {
		ve.addf("server.websocket.send_buffer_size: must be at least 1, got %d", c.Server.WebSocket.SendBufferSize)
	}
	if c.Server.WebSocket.WriteTimeoutSeconds < 1 {
		ve.addf("server.websocket.write_timeout_seconds: must be at least 1, got %d", c.Server.WebSocket.WriteTimeoutSeconds)
	}
	if c.Server.WebSocket.PingIntervalSeconds < 1 {
		ve.addf("server.websocket.ping_interval_seconds: must be at least 1, got %d", c.Server.WebSocket.PingIntervalSeconds)
	}
	if c.Server.WebSocket.PongTimeoutSeconds <= c.Server.WebSocket.PingIntervalSeconds {
		ve.addf("server.websocket.pong_timeout_seconds: must be greater than ping_interval_seconds (%d), got %d",
			c.Server.WebSocket.PingIntervalSeconds, c.Server.WebSocket.PongTimeoutSeconds)
	}

	// Backward compatibility warning
	if c.Server.TLS.Enabled != nil && c.Server.TLS.Mode != "" {
		w.add("server.tls.enabled is deprecated; server.tls.mode takes precedence")
	}

	switch c.Server.TLS.Mode {
	case "off":
		// nothing to validate
	case "static":
		c.validateTLSStatic(ve, w)
	case "acme":
		c.validateTLSACME(ve)
	default:
		ve.addf("server.tls.mode: must be 'off', 'static', or 'acme', got %q", c.Server.TLS.Mode)
	}

	if c.Server.TLS.Mode == "static" || c.Server.TLS.Mode == "acme" {
		switch c.Server.TLS.MinVersion {
		case "1.2", "1.3":
			// ok
		default:
			ve.addf("server.tls.min_version: must be '1.2' or '1.3', got %q", c.Server.TLS.MinVersion)
		}
	}

	if c.Server.TLS.HTTPRedirectPort != 0 {
		if c.Server.TLS.HTTPRedirectPort < 1 || c.Server.TLS.HTTPRedirectPort > 65535 {
			ve.addf("server.tls.http_redirect_port: %d is not a valid port number (1-65535)", c.Server.TLS.HTTPRedirectPort)
		}
	}
}

func (c *Config) validateTLSStatic(ve *ValidationError, w *Warnings) {
	if c.Server.TLS.CertPath == "" {
		ve.add("server.tls.cert_path is required when tls.mode is 'static'")
	} else if _, err := os.Stat(c.Server.TLS.CertPath); err != nil {
		ve.addf("server.tls.cert_path %q: %v", c.Server.TLS.CertPath, err)
	}
	if c.Server.TLS.KeyPath == "" {
		ve.add("server.tls.key_path is required when tls.mode is 'static'")
	} else if info, err := os.Stat(c.Server.TLS.KeyPath); err != nil {
		ve.addf("server.tls.key_path %q: %v", c.Server.TLS.KeyPath, err)
	} else if info.Mode().Perm()&0o077 != 0 {
		w.addf("server.tls.key_path %q has permissions %o; recommended 0600", c.Server.TLS.KeyPath, info.Mode().Perm())
	}
	if c.Server.TLS.CAPath != "" {
		if _, err := os.Stat(c.Server.TLS.CAPath); err != nil {
			ve.addf("server.tls.ca_path %q: %v", c.Server.TLS.CAPath, err)
		}
	}
}

func (c *Config) validateTLSACME(ve *ValidationError) {
	if len(c.Server.TLS.ACME.Domains) == 0 {
		ve.add("server.tls.acme.domains is required when tls.mode is 'acme'")
	}
	if c.Server.TLS.ACME.Email == "" {
		ve.add("server.tls.acme.email is required when tls.mode is 'acme'")
	}
	if !c.Server.TLS.ACME.AgreeToTOS {
		ve.add("server.tls.acme.agree_to_tos must be true when tls.mode is 'acme'")
	}

	switch c.Server.TLS.ACME.Challenge {
	case "http-01":
		if c.Server.TLS.HTTPRedirectPort == 0 {
			ve.add("server.tls.http_redirect_port must be set when acme.challenge is 'http-01'")
		}
	case "tls-alpn-01":
		// no extra config needed
	case "dns-01":
		if c.Server.TLS.ACME.DNSProvider == "" {
			ve.add("server.tls.acme.dns_provider is required when acme.challenge is 'dns-01'")
		}
	default:
		ve.addf("server.tls.acme.challenge: must be 'http-01', 'tls-alpn-01', or 'dns-01', got %q", c.Server.TLS.ACME.Challenge)
	}

	if c.Server.TLS.ACME.RenewBeforeDays < 1 || c.Server.TLS.ACME.RenewBeforeDays > 89 {
		ve.addf("server.tls.acme.renew_before_days: must be between 1 and 89, got %d", c.Server.TLS.ACME.RenewBeforeDays)
	}

	if c.Server.TLS.ACME.CAURL != "" {
		if _, err := url.ParseRequestURI(c.Server.TLS.ACME.CAURL); err != nil {
			ve.addf("server.tls.acme.ca_url: not a valid URL: %v", err)
		}
	}

	if c.Server.TLS.ACME.TrustedRoots != "" {
		if _, err := os.Stat(c.Server.TLS.ACME.TrustedRoots); err != nil {
			ve.addf("server.tls.acme.trusted_roots %q: %v", c.Server.TLS.ACME.TrustedRoots, err)
		}
	}
}

func (c *Config) validateLogging(ve *ValidationError) {
	switch strings.ToUpper(c.Logging.Level) {
	case "DEBUG", "INFO", "WARN", "ERROR":
		// ok
	default:
		ve.addf("logging.level: must be one of DEBUG, INFO, WARN, ERROR, got %q", c.Logging.Level)
	}
}

func (c *Config) validateAuth(ve *ValidationError) {
	for i, p := range c.Auth.Providers {
		prefix := fmt.Sprintf("auth.providers[%d]", i)
		switch p.Type {
		case "local":
			// no additional config required
		case "ldap":
			if p.Host == "" {
				ve.addf("%s: host is required for ldap provider", prefix)
			}
			if p.BaseDN == "" {
				ve.addf("%s: base_dn is required for ldap provider", prefix)
			}
		case "saml":
			if p.IDPMetadataURL == "" {
				ve.addf("%s: idp_metadata_url is required for saml provider", prefix)
			}
			if p.SPEntityID == "" {
				ve.addf("%s: sp_entity_id is required for saml provider", prefix)
			}
		default:
			ve.addf("%s: unknown provider type %q (expected local, ldap, or saml)", prefix, p.Type)
		}
	}
}

func (c *Config) validateOwnership(ve *ValidationError) {
	if !c.Ownership.Enabled {
		return
	}

	names := make(map[string]bool)
	for i, rule := range c.Ownership.AutoRules {
		prefix := fmt.Sprintf("ownership.auto_rules[%d]", i)
		if rule.Name == "" {
			ve.addf("%s.name is required", prefix)
		} else if names[rule.Name] {
			ve.addf("%s.name %q is duplicated", prefix, rule.Name)
		} else {
			names[rule.Name] = true
		}
		// cmdb_attribute rules derive the owner from node attributes, so
		// the Owner field is not required (and should not be set).
		if rule.Type != "cmdb_attribute" {
			if rule.Owner == "" {
				ve.addf("%s.owner is required", prefix)
			}
		}
		switch rule.Type {
		case "node_attribute":
			if rule.AttributePath == "" {
				ve.addf("%s.attribute_path is required for node_attribute rules", prefix)
			}
			if rule.MatchValue == "" {
				ve.addf("%s.match_value is required for node_attribute rules", prefix)
			}
		case "node_name_pattern", "cookbook_name_pattern", "git_repo_url_pattern", "role_match":
			if rule.Pattern == "" {
				ve.addf("%s.pattern is required for %s rules", prefix, rule.Type)
			}
		case "policy_match":
			if rule.PolicyName == "" && rule.Pattern == "" {
				ve.addf("%s.policy_name or pattern is required for policy_match rules", prefix)
			}
		case "cmdb_attribute":
			if rule.ObjectType == "" {
				ve.addf("%s.object_type is required for cmdb_attribute rules", prefix)
			} else if !ValidCMDBObjectTypes[rule.ObjectType] {
				ve.addf("%s.object_type %q is not valid (must be one of: node, cookbook, profile, role)", prefix, rule.ObjectType)
			}
			if rule.Owner != "" {
				ve.addf("%s.owner must not be set for cmdb_attribute rules (owner is derived from the attribute value)", prefix)
			}
		case "":
			ve.addf("%s.type is required", prefix)
		default:
			ve.addf("%s.type %q is not valid (must be one of: node_attribute, node_name_pattern, policy_match, cookbook_name_pattern, git_repo_url_pattern, role_match, cmdb_attribute)", prefix, rule.Type)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// checkDirWritable checks that the given path exists and is a writable
// directory. It tries to create it if it doesn't exist.
func checkDirWritable(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist")
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}

	// Quick writability probe — try creating a temp file.
	tmp := filepath.Join(path, ".config-write-probe")
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	f.Close()
	os.Remove(tmp)
	return nil
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

// Load reads configuration from the given YAML file path, applies defaults,
// applies environment variable overrides, and validates the result.
// If path is empty, it checks the CHEF_MIGRATION_METRICS_CONFIG environment
// variable.
func Load(path string) (*Config, *Warnings, error) {
	if path == "" {
		path = os.Getenv("CHEF_MIGRATION_METRICS_CONFIG")
	}
	if path == "" {
		return nil, nil, fmt.Errorf("no configuration file path provided (set CHEF_MIGRATION_METRICS_CONFIG or pass path to Load)")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading configuration file: %w", err)
	}

	cfg, warnings, err := Parse(data)
	if err != nil {
		return cfg, warnings, err
	}
	return cfg, warnings, nil
}

// Parse unmarshals YAML bytes into a Config, applies defaults, applies
// environment variable overrides, and validates.
func Parse(data []byte) (*Config, *Warnings, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parsing configuration YAML: %w", err)
	}

	// Track which directory fields were explicitly set before defaults fill them in.
	cfg.explicitExportsDir = cfg.Exports.OutputDirectory != ""
	cfg.explicitESDir = cfg.Elasticsearch.OutputDirectory != ""

	cfg.setDefaults()
	cfg.applyEnvOverrides()

	// Environment overrides count as explicit.
	if os.Getenv("CHEF_MIGRATION_METRICS_ELASTICSEARCH_OUTPUT_DIRECTORY") != "" {
		cfg.explicitESDir = true
	}

	warnings, err := cfg.Validate()
	if err != nil {
		return &cfg, warnings, err
	}
	return &cfg, warnings, nil
}
